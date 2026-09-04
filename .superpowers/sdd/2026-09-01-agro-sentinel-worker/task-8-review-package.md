diff --git a/internal/processing/cloudcover.go b/internal/processing/cloudcover.go
new file mode 100644
index 0000000..e365e5f
--- /dev/null
+++ b/internal/processing/cloudcover.go
@@ -0,0 +1,147 @@
+package processing
+
+import (
+	"context"
+	"encoding/json"
+	"fmt"
+	"os"
+	"path/filepath"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// CoverageStats holds the percentage of SCL-classified pixels belonging to
+// each land-cover category of interest, computed over the same BBOX-cropped
+// SCL raster used for cloud cover.
+type CoverageStats struct {
+	VegetationPct float64
+	SoilPct       float64
+	WaterPct      float64
+	CloudPct      float64
+}
+
+// sclHistogramBuckets is the number of buckets gdalinfo's histogram
+// produces for the SCL band, whose values range 0-11 inclusive.
+const sclHistogramBuckets = 12
+
+// gdalInfoHistJSON mirrors the subset of `gdalinfo -json -hist` output this
+// function needs: the first band's histogram.
+type gdalInfoHistJSON struct {
+	Bands []struct {
+		Histogram struct {
+			Count   int     `json:"count"`
+			Min     float64 `json:"min"`
+			Max     float64 `json:"max"`
+			Buckets []int64 `json:"buckets"`
+		} `json:"histogram"`
+	} `json:"bands"`
+}
+
+// CalculateCloudCover warps the SCL band at sclHref to bbox at its native
+// 20m resolution (nearest-neighbor, since SCL values are categorical class
+// codes rather than continuous data), then runs `gdalinfo -json -hist` on
+// the warped raster to obtain per-class pixel counts. It returns the SCL
+// cloud cover percentage (spec formula: classes 3, 8, 9, 10 over all valid
+// pixels, excluding class 0/no-data) along with vegetation/soil/water/cloud
+// coverage percentages for the same area.
+func CalculateCloudCover(ctx context.Context, executor GDALExecutor, sclHref string, bbox domain.BBox, workDir string) (float64, CoverageStats, error) {
+	if err := bbox.Validate(); err != nil {
+		return 0, CoverageStats{}, &domain.ProcessingError{
+			Type:    domain.ErrValidation,
+			Message: "invalid bbox",
+			Wrapped: err,
+		}
+	}
+
+	if err := os.MkdirAll(workDir, 0o755); err != nil {
+		return 0, CoverageStats{}, &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating work dir", Wrapped: err}
+	}
+
+	warpedPath := filepath.Join(workDir, fmt.Sprintf("%s.tif", domain.BandSCL))
+	res := fmt.Sprintf("%d", domain.BandSCL.Resolution())
+
+	warpArgs := []string{
+		"-t_srs", TargetSRS,
+		"-te", fmt.Sprintf("%v", bbox.MinX), fmt.Sprintf("%v", bbox.MinY), fmt.Sprintf("%v", bbox.MaxX), fmt.Sprintf("%v", bbox.MaxY),
+		"-tr", res, res,
+		"-r", "near",
+		sclHref,
+		warpedPath,
+	}
+
+	_, warpStderr, err := executor.Run(ctx, "gdalwarp", warpArgs)
+	if err != nil {
+		return 0, CoverageStats{}, err
+	}
+
+	if _, statErr := os.Stat(warpedPath); statErr != nil {
+		return 0, CoverageStats{}, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalwarp did not produce output file: " + warpedPath + " (" + warpStderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	infoArgs := []string{"-json", "-hist", warpedPath}
+	stdout, stderr, err := executor.Run(ctx, "gdalinfo", infoArgs)
+	if err != nil {
+		return 0, CoverageStats{}, err
+	}
+
+	var parsed gdalInfoHistJSON
+	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
+		return 0, CoverageStats{}, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "failed to parse gdalinfo histogram output: " + stderr,
+			Wrapped: err,
+		}
+	}
+
+	if len(parsed.Bands) == 0 {
+		return 0, CoverageStats{}, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalinfo output did not contain any bands: " + stderr,
+		}
+	}
+
+	buckets := parsed.Bands[0].Histogram.Buckets
+	if len(buckets) < sclHistogramBuckets {
+		return 0, CoverageStats{}, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: fmt.Sprintf("gdalinfo histogram has %d buckets, expected at least %d for SCL classes 0-11", len(buckets), sclHistogramBuckets),
+		}
+	}
+
+	// buckets[i] holds the pixel count for SCL class i.
+	classCount := func(class int) int64 {
+		if class < 0 || class >= len(buckets) {
+			return 0
+		}
+		return buckets[class]
+	}
+
+	var totalPixels int64
+	for i := 0; i < sclHistogramBuckets; i++ {
+		totalPixels += classCount(i)
+	}
+	totalValid := totalPixels - classCount(0)
+
+	if totalValid <= 0 {
+		return 0, CoverageStats{}, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "SCL histogram has no valid (non-no-data) pixels",
+		}
+	}
+
+	cloudPixels := classCount(3) + classCount(8) + classCount(9) + classCount(10)
+	cloudCoverPct := float64(cloudPixels) / float64(totalValid) * 100
+
+	coverage := CoverageStats{
+		VegetationPct: float64(classCount(4)) / float64(totalValid) * 100,
+		SoilPct:       float64(classCount(5)) / float64(totalValid) * 100,
+		WaterPct:      float64(classCount(6)) / float64(totalValid) * 100,
+		CloudPct:      cloudCoverPct,
+	}
+
+	return cloudCoverPct, coverage, nil
+}
diff --git a/internal/processing/cloudcover_test.go b/internal/processing/cloudcover_test.go
new file mode 100644
index 0000000..4f96b30
--- /dev/null
+++ b/internal/processing/cloudcover_test.go
@@ -0,0 +1,222 @@
+package processing
+
+import (
+	"context"
+	"encoding/json"
+	"os"
+	"path/filepath"
+	"testing"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// cloudCoverMockExecutor simulates gdalwarp (creating the warped output
+// file) and gdalinfo -json -hist (returning a pre-built histogram) without
+// shelling out to real GDAL.
+type cloudCoverMockExecutor struct {
+	buckets   [12]int64
+	infoErr   error
+	warpErr   error
+	infoCalls int
+	warpCalls int
+}
+
+func (m *cloudCoverMockExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
+	switch command {
+	case "gdalwarp":
+		m.warpCalls++
+		if m.warpErr != nil {
+			return "", "mock warp error", m.warpErr
+		}
+		outputPath := args[len(args)-1]
+		if err := os.WriteFile(outputPath, []byte("fake"), 0o644); err != nil {
+			return "", "", err
+		}
+		return "", "", nil
+	case "gdalinfo":
+		m.infoCalls++
+		if m.infoErr != nil {
+			return "", "mock info error", m.infoErr
+		}
+		buckets := make([]int64, len(m.buckets))
+		copy(buckets, m.buckets[:])
+		resp := gdalInfoHistJSON{
+			Bands: []struct {
+				Histogram struct {
+					Count   int     `json:"count"`
+					Min     float64 `json:"min"`
+					Max     float64 `json:"max"`
+					Buckets []int64 `json:"buckets"`
+				} `json:"histogram"`
+			}{
+				{
+					Histogram: struct {
+						Count   int     `json:"count"`
+						Min     float64 `json:"min"`
+						Max     float64 `json:"max"`
+						Buckets []int64 `json:"buckets"`
+					}{
+						Count:   len(buckets),
+						Min:     -0.5,
+						Max:     11.5,
+						Buckets: buckets,
+					},
+				},
+			},
+		}
+		out, err := json.Marshal(resp)
+		if err != nil {
+			return "", "", err
+		}
+		return string(out), "", nil
+	}
+	return "", "", nil
+}
+
+func testCloudBBox() domain.BBox {
+	return domain.BBox{MinX: -100, MinY: 20, MaxX: -99, MaxY: 21}
+}
+
+func TestCalculateCloudCover_KnownDistribution(t *testing.T) {
+	workDir := t.TempDir()
+
+	// class index: 0    1  2  3   4    5    6  7  8   9   10  11
+	// pixels:       0    0  0  0  500  0    0  0  100 100 0   0
+	// total=700 (excluding class0 which is 0 anyway), veg=500/700, cloud=(0+100+100+0)/700
+	mock := &cloudCoverMockExecutor{
+		buckets: [12]int64{0, 0, 0, 0, 500, 0, 0, 0, 100, 100, 0, 0},
+	}
+
+	cloudPct, coverage, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir)
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	wantCloud := float64(200) / float64(700) * 100
+	if diff := cloudPct - wantCloud; diff > 1e-9 || diff < -1e-9 {
+		t.Errorf("cloudPct = %v, want %v", cloudPct, wantCloud)
+	}
+
+	wantVeg := float64(500) / float64(700) * 100
+	if diff := coverage.VegetationPct - wantVeg; diff > 1e-9 || diff < -1e-9 {
+		t.Errorf("VegetationPct = %v, want %v", coverage.VegetationPct, wantVeg)
+	}
+
+	if coverage.CloudPct != cloudPct {
+		t.Errorf("coverage.CloudPct = %v, want it to equal returned cloudPct %v", coverage.CloudPct, cloudPct)
+	}
+
+	if mock.warpCalls != 1 {
+		t.Errorf("expected 1 gdalwarp call, got %d", mock.warpCalls)
+	}
+	if mock.infoCalls != 1 {
+		t.Errorf("expected 1 gdalinfo call, got %d", mock.infoCalls)
+	}
+
+	warpedPath := filepath.Join(workDir, "SCL.tif")
+	if _, err := os.Stat(warpedPath); err != nil {
+		t.Errorf("expected warped SCL file at %s: %v", warpedPath, err)
+	}
+}
+
+func TestCalculateCloudCover_50PctVeg20PctCloud(t *testing.T) {
+	workDir := t.TempDir()
+
+	// total valid = 1000. vegetation(4)=500 (50%), cloud classes sum to 200 (20%).
+	mock := &cloudCoverMockExecutor{
+		buckets: [12]int64{0, 0, 100, 50, 500, 100, 100, 0, 100, 30, 20, 0},
+	}
+	// valid total excludes class0: 100+50+500+100+100+0+100+30+20+0 = 1000
+	// cloud = class3(50)+class8(100)+class9(30)+class10(20) = 200 -> 20%
+
+	cloudPct, coverage, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir)
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	if cloudPct != 20.0 {
+		t.Errorf("cloudPct = %v, want 20.0", cloudPct)
+	}
+	if coverage.VegetationPct != 50.0 {
+		t.Errorf("VegetationPct = %v, want 50.0", coverage.VegetationPct)
+	}
+}
+
+func TestCalculateCloudCover_ExcludesNoData(t *testing.T) {
+	workDir := t.TempDir()
+
+	// class0 (no data) = 1000, should not count toward total_valid.
+	// vegetation(4) = 100, rest 0 -> vegetation should be 100%.
+	mock := &cloudCoverMockExecutor{
+		buckets: [12]int64{1000, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0},
+	}
+
+	cloudPct, coverage, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir)
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	if cloudPct != 0.0 {
+		t.Errorf("cloudPct = %v, want 0.0", cloudPct)
+	}
+	if coverage.VegetationPct != 100.0 {
+		t.Errorf("VegetationPct = %v, want 100.0", coverage.VegetationPct)
+	}
+}
+
+func TestCalculateCloudCover_InvalidBBox(t *testing.T) {
+	workDir := t.TempDir()
+	mock := &cloudCoverMockExecutor{}
+
+	badBBox := domain.BBox{MinX: 10, MinY: 20, MaxX: 5, MaxY: 21}
+
+	_, _, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", badBBox, workDir)
+	if err == nil {
+		t.Fatal("expected error for invalid bbox, got nil")
+	}
+
+	var procErr *domain.ProcessingError
+	if !isProcessingError(err, &procErr) {
+		t.Fatalf("expected *domain.ProcessingError, got %T", err)
+	}
+	if procErr.Type != domain.ErrValidation {
+		t.Errorf("Type = %v, want %v", procErr.Type, domain.ErrValidation)
+	}
+}
+
+func TestCalculateCloudCover_AllNoData(t *testing.T) {
+	workDir := t.TempDir()
+
+	mock := &cloudCoverMockExecutor{
+		buckets: [12]int64{1000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
+	}
+
+	_, _, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir)
+	if err == nil {
+		t.Fatal("expected error when all pixels are no-data, got nil")
+	}
+}
+
+func TestCalculateCloudCover_GDALInfoError(t *testing.T) {
+	workDir := t.TempDir()
+
+	mock := &cloudCoverMockExecutor{
+		infoErr: &domain.ProcessingError{Type: domain.ErrGDAL, Message: "gdalinfo failed"},
+	}
+
+	_, _, err := CalculateCloudCover(context.Background(), mock, "/vsis3/bucket/SCL.tif", testCloudBBox(), workDir)
+	if err == nil {
+		t.Fatal("expected error when gdalinfo fails, got nil")
+	}
+}
+
+// isProcessingError checks whether err is a *domain.ProcessingError and, if
+// so, assigns it to *out.
+func isProcessingError(err error, out **domain.ProcessingError) bool {
+	pe, ok := err.(*domain.ProcessingError)
+	if !ok {
+		return false
+	}
+	*out = pe
+	return true
+}
