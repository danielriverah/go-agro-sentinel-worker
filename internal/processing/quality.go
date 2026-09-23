package processing

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
)

//go:embed raster_quality.py
var rasterQualityScript string

const MaxNoDataPct = 15.0

// Quality describes only pixels inside the destination production polygon.
// A pointer in Params distinguishes legacy (not evaluated) from zero NoData.
type Quality struct {
	NoDataPct   float64 `json:"nodata_pct"`
	ValidPct    float64 `json:"valid_pct"`
	TotalPixels int64   `json:"total_pixels"`
	ValidPixels int64   `json:"valid_pixels"`
	HasSCL      bool    `json:"has_scl"`
	Source      string  `json:"source"`
	Usable      bool    `json:"usable"`
	Reason      string  `json:"reason,omitempty"`
}

func (q *Quality) Evaluate(cloud, limit float64, cloudKnown bool) {
	q.Usable = false
	switch {
	case q.NoDataPct > MaxNoDataPct:
		q.Reason = "cobertura_insuficiente"
	case !cloudKnown:
		q.Reason = "calidad_no_verificada"
	case cloud > limit:
		q.Reason = "nubosidad_alta"
	default:
		q.Usable, q.Reason = true, ""
	}
}

// PrepareQuality measures the actual reused/new raster and makes a masked work
// copy. No source bands are downloaded, including for legacy ten-band rasters.
func PrepareQuality(ctx context.Context, executor GDALExecutor, source, polygon, workDir string) (string, *Quality, CoverageStats, error) {
	output := filepath.Join(workDir, "multiband_valid.tif")
	stdout, stderr, err := executor.Run(ctx, "python3", []string{"-c", rasterQualityScript, "prepare", source, polygon, output})
	if err != nil {
		return "", nil, CoverageStats{}, fmt.Errorf("evaluating raster coverage: %s: %w", stderr, err)
	}
	var result struct {
		Quality
		CoverageStats
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return "", nil, CoverageStats{}, fmt.Errorf("decoding raster coverage: %w", err)
	}
	if result.TotalPixels <= 0 || result.ValidPixels < 0 || result.ValidPixels > result.TotalPixels {
		return "", nil, CoverageStats{}, fmt.Errorf("invalid raster coverage counts")
	}
	return output, &result.Quality, result.CoverageStats, nil
}

func ApplyNoDataAlpha(ctx context.Context, executor GDALExecutor, source, png string) error {
	_, stderr, err := executor.Run(ctx, "python3", []string{"-c", rasterQualityScript, "alpha", source, png})
	if err != nil {
		return fmt.Errorf("applying NoData transparency: %s: %w", stderr, err)
	}
	return nil
}
