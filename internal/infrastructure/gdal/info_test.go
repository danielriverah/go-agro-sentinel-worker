package gdal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ensureTinyTif returns the path to testdata/tiny.tif, creating it with
// `gdal_create` if it doesn't already exist. It skips the calling test if
// gdal_create is not available.
func ensureTinyTif(t *testing.T) string {
	t.Helper()

	path, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "tiny.tif"))
	if err != nil {
		t.Fatalf("resolving testdata path: %v", err)
	}

	if _, statErr := os.Stat(path); statErr == nil {
		return path
	}

	if _, err := exec.LookPath("gdal_create"); err != nil {
		t.Skip("gdal_create not found in PATH; cannot create testdata/tiny.tif")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating testdata dir: %v", err)
	}

	cmd := exec.Command("gdal_create", "-of", "GTiff", "-outsize", "10", "10", "-bands", "1", "-burn", "128", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("gdal_create failed, skipping: %v (%s)", err, out)
	}

	return path
}

func TestInfoParsesTinyTif(t *testing.T) {
	if _, err := exec.LookPath("gdalinfo"); err != nil {
		t.Skip("gdalinfo not found in PATH")
	}

	path := ensureTinyTif(t)
	e := NewExecutor(300)

	info, err := Info(context.Background(), e, path)
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}

	if info.Width != 10 || info.Height != 10 {
		t.Errorf("expected 10x10, got %dx%d", info.Width, info.Height)
	}
	if info.Bands != 1 {
		t.Errorf("expected 1 band, got %d", info.Bands)
	}
}

func TestInfoNonexistentFile(t *testing.T) {
	e := NewExecutor(300)
	_, err := Info(context.Background(), e, filepath.Join(t.TempDir(), "does-not-exist.tif"))
	if err == nil {
		t.Fatal("expected error for nonexistent input file")
	}
}

// TestGDALInfoJSONParsing verifies the JSON-parsing logic against a mock
// gdalinfo -json response, without requiring GDAL to be installed.
func TestGDALInfoJSONParsing(t *testing.T) {
	mock := []byte(`{
		"size": [512, 256],
		"bands": [{}, {}, {}],
		"coordinateSystem": {"wkt": "PROJCS[\"WGS 84 / UTM zone 33N\"]"},
		"cornerCoordinates": {
			"upperLeft": [499980.0, 4200000.0],
			"lowerLeft": [499980.0, 4100000.0],
			"upperRight": [600000.0, 4200000.0],
			"lowerRight": [600000.0, 4100000.0]
		}
	}`)

	var parsed gdalInfoJSON
	if err := json.Unmarshal(mock, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	info := &GDALInfo{
		Width:      parsed.Size[0],
		Height:     parsed.Size[1],
		Bands:      len(parsed.Bands),
		Projection: parsed.CoordinateSystem.Wkt,
		BoundsMinX: parsed.CornerCoordinates.LowerLeft[0],
		BoundsMinY: parsed.CornerCoordinates.LowerLeft[1],
		BoundsMaxX: parsed.CornerCoordinates.UpperRight[0],
		BoundsMaxY: parsed.CornerCoordinates.UpperRight[1],
	}

	if info.Width != 512 || info.Height != 256 {
		t.Errorf("expected 512x256, got %dx%d", info.Width, info.Height)
	}
	if info.Bands != 3 {
		t.Errorf("expected 3 bands, got %d", info.Bands)
	}
	if info.BoundsMinX != 499980.0 || info.BoundsMaxX != 600000.0 {
		t.Errorf("unexpected X bounds: min=%v max=%v", info.BoundsMinX, info.BoundsMaxX)
	}
	if info.BoundsMinY != 4100000.0 || info.BoundsMaxY != 4200000.0 {
		t.Errorf("unexpected Y bounds: min=%v max=%v", info.BoundsMinY, info.BoundsMaxY)
	}
}
