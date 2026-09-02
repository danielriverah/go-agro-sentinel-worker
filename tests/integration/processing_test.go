//go:build integration

// Package integration contains end-to-end tests for the processing pipeline
// that exercise real GDAL command-line tools (never mocked). These tests are
// gated behind the "integration" build tag and skip automatically when GDAL
// is not available on PATH, per the project's constraint that GDAL is only
// ever invoked as an external process.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/gdal"
	"agro-sentinel-worker/internal/processing"
	"agro-sentinel-worker/internal/storage"
)

// testSRS matches processing.TargetSRS so the synthetic bands need no
// reprojection, only cropping/resampling, when built into multiband.tif.
const testSRS = "EPSG:32614"

// requireGDAL skips the test if gdalinfo (and the other GDAL CLI tools) are
// not available on PATH.
func requireGDAL(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gdalinfo"); err != nil {
		t.Skip("gdalinfo not found in PATH; skipping GDAL integration test")
	}
	for _, tool := range []string{"gdal_create", "gdal_translate", "gdalwarp", "gdalbuildvrt", "gdaldem"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found in PATH; skipping GDAL integration test", tool)
		}
	}
}

// createTestBand creates a small 10x10 pixel, single-band GeoTIFF at path,
// georeferenced to testSRS covering [ulx,uly]-[lrx,lry], filled with a
// constant value. This simulates one Sentinel-2 band.
func createTestBand(t *testing.T, path string, ulx, uly, lrx, lry float64, value int) {
	t.Helper()
	args := []string{
		"-outsize", "10", "10",
		"-bands", "1",
		"-ot", "UInt16",
		"-burn", fmt.Sprintf("%d", value),
		"-a_srs", testSRS,
		"-a_ullr", fmt.Sprintf("%v", ulx), fmt.Sprintf("%v", uly), fmt.Sprintf("%v", lrx), fmt.Sprintf("%v", lry),
		path,
	}
	cmd := exec.Command("gdal_create", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gdal_create failed for %s: %v\n%s", path, err, out)
	}
}

// TestEndToEndProcessingPipeline exercises the full processing pipeline
// against real GDAL: multiband.tif -> cloud cover -> natural.png -> NDVI ->
// statistics -> params.json.
func TestEndToEndProcessingPipeline(t *testing.T) {
	requireGDAL(t)

	ctx := context.Background()
	baseDir := t.TempDir()
	jobDir := storage.New(baseDir, "test-job-1")
	if err := jobDir.Create(); err != nil {
		t.Fatalf("creating job dir: %v", err)
	}

	// A 100x100m bbox, matching a 10m target resolution -> 10x10 output
	// pixels for the multiband raster.
	bbox := domain.BBox{MinX: 500000, MinY: 2000000, MaxX: 500100, MaxY: 2000100}
	const targetResolution = 10

	// Create synthetic source bands slightly larger than the bbox so
	// gdalwarp has real cropping to do, with distinct reflectance-like
	// values so NDVI comes out non-degenerate.
	inputDir := jobDir.Input()
	bandValues := map[domain.Band]int{
		domain.BandB02: 800,  // blue
		domain.BandB03: 1000, // green
		domain.BandB04: 1200, // red
		domain.BandB08: 3000, // NIR (vegetation reflects strongly here)
	}

	var bandInfos []domain.BandInfo
	for _, name := range []domain.Band{domain.BandB02, domain.BandB03, domain.BandB04, domain.BandB08} {
		path := filepath.Join(inputDir, string(name)+".tif")
		createTestBand(t, path, 499950, 2000150, 500150, 1999950, bandValues[name])
		bandInfos = append(bandInfos, domain.BandInfo{
			Name:       name,
			Resolution: name.Resolution(),
			Href:       path,
		})
	}

	// SCL band at its native 20m resolution, filled mostly with class 4
	// (vegetation). Real Sentinel-2 scenes mix classes; a uniform fill is
	// sufficient here since this test verifies pipeline wiring, not
	// classification accuracy.
	sclPath := filepath.Join(inputDir, "SCL.tif")
	createTestBand(t, sclPath, 499950, 2000150, 500150, 1999950, 4)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	executor := gdal.NewExecutor(60)
	processingCfg := config.ProcessingConfig{ResamplingMethod: "bilinear"}

	// Step 1: build multiband.tif.
	builder := processing.New(executor, nil, processingCfg, logger)
	multibandPath, err := builder.Build(ctx, jobDir.Root(), bbox, bandInfos, targetResolution)
	if err != nil {
		t.Fatalf("MultibandBuilder.Build failed: %v", err)
	}
	assertFileExists(t, multibandPath, 100)

	// Step 2: cloud cover from SCL.
	cloudCoverPct, coverage, err := processing.CalculateCloudCover(ctx, executor, sclPath, bbox, jobDir.Work())
	if err != nil {
		t.Fatalf("CalculateCloudCover failed: %v", err)
	}
	if cloudCoverPct < 0 || cloudCoverPct > 100 {
		t.Errorf("cloud cover out of range: %v", cloudCoverPct)
	}
	if coverage.VegetationPct <= 0 {
		t.Errorf("expected mostly-vegetation SCL fixture to report vegetation coverage, got %+v", coverage)
	}

	// Step 3: natural color composite.
	naturalPath := filepath.Join(jobDir.Output(), string(domain.FileNatural)+".png")
	if err := processing.GenerateRGB(ctx, executor, multibandPath, naturalPath, 3, 2, 1); err != nil {
		t.Fatalf("GenerateRGB failed: %v", err)
	}
	assertFileExists(t, naturalPath, 50)

	// Step 4: NDVI index image.
	ndviPath := filepath.Join(jobDir.Output(), string(domain.FileNDVI)+".png")
	if err := processing.GenerateIndex(ctx, executor, multibandPath, ndviPath, domain.FileNDVI); err != nil {
		t.Fatalf("GenerateIndex(NDVI) failed: %v", err)
	}
	assertFileExists(t, ndviPath, 50)

	// Step 5: band + index statistics.
	bandStats, err := processing.CalculateBandStatistics(ctx, executor, multibandPath)
	if err != nil {
		t.Fatalf("CalculateBandStatistics failed: %v", err)
	}
	if len(bandStats) == 0 {
		t.Fatal("expected non-empty band statistics")
	}

	ndviDef := processing.IndexDefinition{Type: domain.FileNDVI, Name: "NDVI", Bands: []domain.Band{domain.BandB08, domain.BandB04}}
	indexStats, err := processing.CalculateIndexStatistics(ctx, executor, multibandPath, []processing.IndexDefinition{ndviDef})
	if err != nil {
		t.Fatalf("CalculateIndexStatistics failed: %v", err)
	}
	ndviStats, ok := indexStats[domain.FileNDVI]
	if !ok {
		t.Fatal("expected NDVI statistics to be present")
	}
	// NIR (3000) >> Red (1200) should give a strongly positive NDVI.
	if ndviStats.Mean <= 0 {
		t.Errorf("expected positive NDVI mean given NIR > Red fixture values, got %v", ndviStats.Mean)
	}

	// Step 6: build params.json, simulating a previous scene for the
	// historical chain / delta computation.
	previousParams := &processing.Params{
		ProduccionID: 42,
		SceneID:      "S2A_PREV_SCENE",
		SceneDate:    "2026-08-01",
		Indices: map[string]processing.IndexStats{
			"ndvi": {Mean: 0.55, Std: 0.05, Min: 0.1, Max: 0.9, P25: 0.4, P50: 0.55, P75: 0.7},
		},
	}

	paramsInput := processing.ParamsInput{
		ProduccionID:    42,
		SceneID:         "S2A_TEST_SCENE",
		SceneDate:       time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		FechaPlantacion: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		CloudCoverBBox:  cloudCoverPct,
		Indices:         indexStats,
		BandStats:       bandStats,
		Coverage:        coverage,
	}

	params := processing.BuildParams(paramsInput, previousParams)
	if params.SceneID != "S2A_TEST_SCENE" {
		t.Errorf("unexpected scene id in params: %s", params.SceneID)
	}
	if len(params.Historico) != 1 || params.Historico[0].SceneID != "S2A_PREV_SCENE" {
		t.Errorf("expected params.Historico to carry forward the previous scene, got %+v", params.Historico)
	}

	paramsPath := filepath.Join(jobDir.Output(), "params.json")
	data, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		t.Fatalf("marshaling params.json: %v", err)
	}
	if err := os.WriteFile(paramsPath, data, 0o644); err != nil {
		t.Fatalf("writing params.json: %v", err)
	}
	assertFileExists(t, paramsPath, 10)

	// Sanity check: params.json round-trips as valid JSON with the fields
	// the downstream API/DB layer expects.
	var reloaded map[string]interface{}
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("params.json is not valid JSON: %v", err)
	}
	for _, field := range []string{"produccion_id", "scene_id", "scene_date", "cloud_cover_bbox", "coverage", "indices"} {
		if _, ok := reloaded[field]; !ok {
			t.Errorf("params.json missing expected field %q", field)
		}
	}
}

// assertFileExists fails the test unless path exists and is at least
// minSizeBytes, a coarse "reasonable size" sanity check for generated
// artifacts.
func assertFileExists(t *testing.T, path string, minSizeBytes int64) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected output file %s to exist: %v", path, err)
	}
	if info.Size() < minSizeBytes {
		t.Errorf("output file %s is suspiciously small: %d bytes (expected at least %d)", path, info.Size(), minSizeBytes)
	}
}
