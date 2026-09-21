package processing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agro-sentinel-worker/internal/domain"
)

// cloudCoverMockExecutor simulates gdalwarp (creating the warped output
// file) and gdalinfo -json -hist (returning a pre-built histogram) without
// shelling out to real GDAL.
type cloudCoverMockExecutor struct {
	buckets   [12]int64
	infoErr   error
	warpErr   error
	infoCalls int
	warpCalls int
}

func (m *cloudCoverMockExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
	switch command {
	case "gdalwarp":
		m.warpCalls++
		if m.warpErr != nil {
			return "", "mock warp error", m.warpErr
		}
		outputPath := args[len(args)-1]
		if err := os.WriteFile(outputPath, []byte("fake"), 0o644); err != nil {
			return "", "", err
		}
		return "", "", nil
	case "gdalinfo":
		m.infoCalls++
		if m.infoErr != nil {
			return "", "mock info error", m.infoErr
		}
		buckets := make([]int64, len(m.buckets))
		copy(buckets, m.buckets[:])
		resp := gdalInfoHistJSON{
			Bands: []struct {
				Histogram struct {
					Count   int     `json:"count"`
					Min     float64 `json:"min"`
					Max     float64 `json:"max"`
					Buckets []int64 `json:"buckets"`
				} `json:"histogram"`
			}{
				{
					Histogram: struct {
						Count   int     `json:"count"`
						Min     float64 `json:"min"`
						Max     float64 `json:"max"`
						Buckets []int64 `json:"buckets"`
					}{
						Count:   len(buckets),
						Min:     -0.5,
						Max:     11.5,
						Buckets: buckets,
					},
				},
			},
		}
		out, err := json.Marshal(resp)
		if err != nil {
			return "", "", err
		}
		return string(out), "", nil
	}
	return "", "", nil
}

func testCloudBBox() domain.BBox {
	return domain.BBox{MinX: -100, MinY: 20, MaxX: -99, MaxY: 21}
}

func TestCalculateCloudCover_KnownDistribution(t *testing.T) {
	workDir := t.TempDir()

	// class index: 0    1  2  3   4    5    6  7  8   9   10  11
	// pixels:       0    0  0  0  500  0    0  0  100 100 0   0
	// total=700 (excluding class0 which is 0 anyway), veg=500/700, cloud=(0+100+100+0)/700
	mock := &cloudCoverMockExecutor{
		buckets: [12]int64{0, 0, 0, 0, 500, 0, 0, 0, 100, 100, 0, 0},
	}

	cloudPct, coverage, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCloud := float64(200) / float64(700) * 100
	if diff := cloudPct - wantCloud; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("cloudPct = %v, want %v", cloudPct, wantCloud)
	}

	wantVeg := float64(500) / float64(700) * 100
	if diff := coverage.VegetationPct - wantVeg; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("VegetationPct = %v, want %v", coverage.VegetationPct, wantVeg)
	}

	if coverage.CloudPct != cloudPct {
		t.Errorf("coverage.CloudPct = %v, want it to equal returned cloudPct %v", coverage.CloudPct, cloudPct)
	}

	if mock.warpCalls != 1 {
		t.Errorf("expected 1 gdalwarp call, got %d", mock.warpCalls)
	}
	if mock.infoCalls != 1 {
		t.Errorf("expected 1 gdalinfo call, got %d", mock.infoCalls)
	}

	warpedPath := filepath.Join(workDir, "scl_cloud.tif")
	if _, err := os.Stat(warpedPath); err != nil {
		t.Errorf("expected warped SCL file at %s: %v", warpedPath, err)
	}
}

func TestCalculateCloudCover_50PctVeg20PctCloud(t *testing.T) {
	workDir := t.TempDir()

	// total valid = 1000. vegetation(4)=500 (50%), cloud classes sum to 200 (20%).
	mock := &cloudCoverMockExecutor{
		buckets: [12]int64{0, 0, 100, 50, 500, 100, 100, 0, 100, 30, 20, 0},
	}
	// valid total excludes class0: 100+50+500+100+100+0+100+30+20+0 = 1000
	// cloud = class3(50)+class8(100)+class9(30)+class10(20) = 200 -> 20%

	cloudPct, coverage, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cloudPct != 20.0 {
		t.Errorf("cloudPct = %v, want 20.0", cloudPct)
	}
	if coverage.VegetationPct != 50.0 {
		t.Errorf("VegetationPct = %v, want 50.0", coverage.VegetationPct)
	}
}

func TestCalculateCloudCover_ExcludesNoData(t *testing.T) {
	workDir := t.TempDir()

	// class0 (no data) = 1000, should not count toward total_valid.
	// vegetation(4) = 100, rest 0 -> vegetation should be 100%.
	mock := &cloudCoverMockExecutor{
		buckets: [12]int64{1000, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0},
	}

	cloudPct, coverage, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cloudPct != 0.0 {
		t.Errorf("cloudPct = %v, want 0.0", cloudPct)
	}
	if coverage.VegetationPct != 100.0 {
		t.Errorf("VegetationPct = %v, want 100.0", coverage.VegetationPct)
	}
}

func TestCalculateCloudCover_InvalidBBox(t *testing.T) {
	workDir := t.TempDir()
	mock := &cloudCoverMockExecutor{}

	badBBox := domain.BBox{MinX: 10, MinY: 20, MaxX: 5, MaxY: 21}

	_, _, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", badBBox, workDir, "")
	if err == nil {
		t.Fatal("expected error for invalid bbox, got nil")
	}

	var procErr *domain.ProcessingError
	if !isProcessingError(err, &procErr) {
		t.Fatalf("expected *domain.ProcessingError, got %T", err)
	}
	if procErr.Type != domain.ErrValidation {
		t.Errorf("Type = %v, want %v", procErr.Type, domain.ErrValidation)
	}
}

func TestCalculateCloudCover_AllNoData(t *testing.T) {
	workDir := t.TempDir()

	mock := &cloudCoverMockExecutor{
		buckets: [12]int64{1000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}

	_, _, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir, "")
	if err == nil {
		t.Fatal("expected error when all pixels are no-data, got nil")
	}
}

func TestCalculateCloudCover_GDALInfoError(t *testing.T) {
	workDir := t.TempDir()

	mock := &cloudCoverMockExecutor{
		infoErr: &domain.ProcessingError{Type: domain.ErrGDAL, Message: "gdalinfo failed"},
	}

	_, _, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir, "")
	if err == nil {
		t.Fatal("expected error when gdalinfo fails, got nil")
	}
}

// isProcessingError checks whether err is a *domain.ProcessingError and, if
// so, assigns it to *out.
func isProcessingError(err error, out **domain.ProcessingError) bool {
	pe, ok := err.(*domain.ProcessingError)
	if !ok {
		return false
	}
	*out = pe
	return true
}
