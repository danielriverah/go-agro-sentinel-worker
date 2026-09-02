package processing

import (
	"context"
	"encoding/json"
	"path/filepath"

	"agro-sentinel-worker/internal/domain"
)

// BandStats holds descriptive statistics for a single spectral band or
// index raster, as computed from gdalinfo -stats/-hist output.
type BandStats struct {
	Mean float64 `json:"mean"`
	Std  float64 `json:"std"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	P25  float64 `json:"p25"`
	P50  float64 `json:"p50"`
	P75  float64 `json:"p75"`
}

// IndexStats holds descriptive statistics for a computed vegetation/moisture
// index raster. Structurally identical to BandStats.
type IndexStats struct {
	Mean float64 `json:"mean"`
	Std  float64 `json:"std"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	P25  float64 `json:"p25"`
	P50  float64 `json:"p50"`
	P75  float64 `json:"p75"`
}

// gdalInfoStatsJSON mirrors the subset of `gdalinfo -json -stats -hist`
// output this package needs: per-band statistics and histogram buckets used
// to approximate percentiles.
type gdalInfoStatsJSON struct {
	Bands []struct {
		Band      int `json:"band"`
		Metadata  map[string]json.RawMessage `json:"metadata"`
		ComputedStatistics struct {
			Minimum   float64 `json:"minimum"`
			Maximum   float64 `json:"maximum"`
			Mean      float64 `json:"mean"`
			StdDev    float64 `json:"stdDev"`
		} `json:"computedStatistics"`
		Histogram struct {
			Count   int       `json:"count"`
			Min     float64   `json:"min"`
			Max     float64   `json:"max"`
			Buckets []int64   `json:"buckets"`
		} `json:"histogram"`
	} `json:"bands"`
}

// CalculateBandStatistics runs `gdalinfo -json -stats -hist` on
// multibandPath and returns per-band descriptive statistics keyed by
// domain.Band, using the fixed band order documented in multiband.go
// (B02=1, B03=2, B04=3, B05=4, B06=5, B07=6, B08=7, B8A=8, B11=9, B12=10).
func CalculateBandStatistics(ctx context.Context, executor GDALExecutor, multibandPath string) (map[domain.Band]BandStats, error) {
	stdout, stderr, err := executor.Run(ctx, "gdalinfo", []string{"-json", "-stats", "-hist", multibandPath})
	if err != nil {
		return nil, err
	}

	var parsed gdalInfoStatsJSON
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		return nil, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "failed to parse gdalinfo stats output: " + stderr,
			Wrapped: err,
		}
	}

	if len(parsed.Bands) == 0 {
		return nil, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalinfo output did not contain any bands: " + stderr,
		}
	}

	bands := domain.AllSpectralBands()
	result := make(map[domain.Band]BandStats, len(bands))

	for i, band := range bands {
		if i >= len(parsed.Bands) {
			break
		}
		b := parsed.Bands[i]
		stats := BandStats{
			Mean: b.ComputedStatistics.Mean,
			Std:  b.ComputedStatistics.StdDev,
			Min:  b.ComputedStatistics.Minimum,
			Max:  b.ComputedStatistics.Maximum,
		}
		p25, p50, p75 := percentilesFromHistogram(b.Histogram.Min, b.Histogram.Max, b.Histogram.Buckets, stats.Mean)
		stats.P25, stats.P50, stats.P75 = p25, p50, p75
		result[band] = stats
	}

	return result, nil
}

// CalculateIndexStatistics runs gdalinfo -json -stats -hist on the raw
// (uncolored) raster for each index computed from multibandPath and returns
// per-index descriptive statistics. It expects the raw index rasters to
// already exist alongside multibandPath, named "<indexType>_raw.tif", as
// produced by GenerateIndex.
func CalculateIndexStatistics(ctx context.Context, executor GDALExecutor, multibandPath string, indices []IndexDefinition) (map[domain.FileType]IndexStats, error) {
	result := make(map[domain.FileType]IndexStats, len(indices))

	for _, def := range indices {
		rawPath := indexRawPath(multibandPath, def.Type)
		stdout, stderr, err := executor.Run(ctx, "gdalinfo", []string{"-json", "-stats", "-hist", rawPath})
		if err != nil {
			return nil, err
		}

		var parsed gdalInfoStatsJSON
		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
			return nil, &domain.ProcessingError{
				Type:    domain.ErrGDAL,
				Message: "failed to parse gdalinfo stats output for " + def.Name + ": " + stderr,
				Wrapped: err,
			}
		}

		if len(parsed.Bands) == 0 {
			return nil, &domain.ProcessingError{
				Type:    domain.ErrGDAL,
				Message: "gdalinfo output for " + def.Name + " did not contain any bands: " + stderr,
			}
		}

		b := parsed.Bands[0]
		stats := IndexStats{
			Mean: b.ComputedStatistics.Mean,
			Std:  b.ComputedStatistics.StdDev,
			Min:  b.ComputedStatistics.Minimum,
			Max:  b.ComputedStatistics.Maximum,
		}
		p25, p50, p75 := percentilesFromHistogram(b.Histogram.Min, b.Histogram.Max, b.Histogram.Buckets, stats.Mean)
		stats.P25, stats.P50, stats.P75 = p25, p50, p75
		result[def.Type] = stats
	}

	return result, nil
}

// indexRawPath derives the path to an index's raw (uncolored) raster from
// the multiband.tif path, matching the naming used by GenerateIndex.
func indexRawPath(multibandPath string, indexType domain.FileType) string {
	dir := filepath.Dir(multibandPath)
	return filepath.Join(dir, string(indexType)+"_raw.tif")
}

// percentilesFromHistogram approximates the 25th, 50th and 75th percentiles
// from a gdalinfo histogram. If no histogram buckets are available, it
// falls back to the mean for all three percentiles (a conservative
// approximation preferable to failing statistics calculation entirely).
func percentilesFromHistogram(min, max float64, buckets []int64, fallbackMean float64) (p25, p50, p75 float64) {
	var total int64
	for _, c := range buckets {
		total += c
	}
	if total <= 0 || len(buckets) == 0 || max <= min {
		return fallbackMean, fallbackMean, fallbackMean
	}

	bucketWidth := (max - min) / float64(len(buckets))

	percentile := func(fraction float64) float64 {
		target := fraction * float64(total)
		var cumulative int64
		for i, c := range buckets {
			cumulative += c
			if float64(cumulative) >= target {
				// Value at the midpoint of the bucket containing the target.
				return min + (float64(i)+0.5)*bucketWidth
			}
		}
		return max
	}

	return percentile(0.25), percentile(0.50), percentile(0.75)
}

