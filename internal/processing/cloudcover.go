package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"agro-sentinel-worker/internal/domain"
)

// CoverageStats holds the percentage of SCL-classified pixels belonging to
// each land-cover category of interest, computed over the same BBOX-cropped
// SCL raster used for cloud cover.
type CoverageStats struct {
	VegetationPct float64
	SoilPct       float64
	WaterPct      float64
	CloudPct      float64
}

// sclHistogramBuckets is the number of buckets gdalinfo's histogram
// produces for the SCL band, whose values range 0-11 inclusive.
const sclHistogramBuckets = 12

// gdalInfoHistJSON mirrors the subset of `gdalinfo -json -hist` output this
// function needs: the first band's histogram.
type gdalInfoHistJSON struct {
	Bands []struct {
		Histogram struct {
			Count   int     `json:"count"`
			Min     float64 `json:"min"`
			Max     float64 `json:"max"`
			Buckets []int64 `json:"buckets"`
		} `json:"histogram"`
	} `json:"bands"`
}

// CalculateCloudCover warps the SCL band at sclHref to bbox at its native
// 20m resolution (nearest-neighbor, since SCL values are categorical class
// codes rather than continuous data), then runs `gdalinfo -json -hist` on
// the warped raster to obtain per-class pixel counts. It returns the SCL
// cloud cover percentage (spec formula: classes 3, 8, 9, 10 over all valid
// pixels, excluding class 0/no-data) along with vegetation/soil/water/cloud
// coverage percentages for the same area.
func CalculateCloudCover(ctx context.Context, executor GDALExecutor, sclHref string, bbox domain.BBox, workDir string) (float64, CoverageStats, error) {
	if err := bbox.Validate(); err != nil {
		return 0, CoverageStats{}, &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: "invalid bbox",
			Wrapped: err,
		}
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return 0, CoverageStats{}, &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating work dir", Wrapped: err}
	}

	warpedPath := filepath.Join(workDir, fmt.Sprintf("%s.tif", domain.BandSCL))
	res := fmt.Sprintf("%d", domain.BandSCL.Resolution())

	warpArgs := []string{
		"-t_srs", TargetSRS,
		"-te", fmt.Sprintf("%v", bbox.MinX), fmt.Sprintf("%v", bbox.MinY), fmt.Sprintf("%v", bbox.MaxX), fmt.Sprintf("%v", bbox.MaxY),
		"-tr", res, res,
		"-r", "near",
		sclHref,
		warpedPath,
	}

	_, warpStderr, err := executor.Run(ctx, "gdalwarp", warpArgs)
	if err != nil {
		return 0, CoverageStats{}, err
	}

	if _, statErr := os.Stat(warpedPath); statErr != nil {
		return 0, CoverageStats{}, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalwarp did not produce output file: " + warpedPath + " (" + warpStderr + ")",
			Wrapped: statErr,
		}
	}

	infoArgs := []string{"-json", "-hist", warpedPath}
	stdout, stderr, err := executor.Run(ctx, "gdalinfo", infoArgs)
	if err != nil {
		return 0, CoverageStats{}, err
	}

	var parsed gdalInfoHistJSON
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		return 0, CoverageStats{}, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "failed to parse gdalinfo histogram output: " + stderr,
			Wrapped: err,
		}
	}

	if len(parsed.Bands) == 0 {
		return 0, CoverageStats{}, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalinfo output did not contain any bands: " + stderr,
		}
	}

	buckets := parsed.Bands[0].Histogram.Buckets
	if len(buckets) < sclHistogramBuckets {
		return 0, CoverageStats{}, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: fmt.Sprintf("gdalinfo histogram has %d buckets, expected at least %d for SCL classes 0-11", len(buckets), sclHistogramBuckets),
		}
	}

	// buckets[i] holds the pixel count for SCL class i.
	classCount := func(class int) int64 {
		if class < 0 || class >= len(buckets) {
			return 0
		}
		return buckets[class]
	}

	var totalPixels int64
	for i := 0; i < sclHistogramBuckets; i++ {
		totalPixels += classCount(i)
	}
	totalValid := totalPixels - classCount(0)

	if totalValid <= 0 {
		return 0, CoverageStats{}, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "SCL histogram has no valid (non-no-data) pixels",
		}
	}

	cloudPixels := classCount(3) + classCount(8) + classCount(9) + classCount(10)
	cloudCoverPct := float64(cloudPixels) / float64(totalValid) * 100

	coverage := CoverageStats{
		VegetationPct: float64(classCount(4)) / float64(totalValid) * 100,
		SoilPct:       float64(classCount(5)) / float64(totalValid) * 100,
		WaterPct:      float64(classCount(6)) / float64(totalValid) * 100,
		CloudPct:      cloudCoverPct,
	}

	return cloudCoverPct, coverage, nil
}
