### Task 4: GDAL executor

**Files:**
- Create: `internal/infrastructure/gdal/executor.go`
- Create: `internal/infrastructure/gdal/translate.go`
- Create: `internal/infrastructure/gdal/warp.go`
- Create: `internal/infrastructure/gdal/info.go`
- Create: `internal/infrastructure/gdal/vrt.go`
- Test: `internal/infrastructure/gdal/executor_test.go`
- Test: `internal/infrastructure/gdal/info_test.go`
- Test: `testdata/tiny.tif` (small GeoTIFF for testing)

**Interfaces:**
- Consumes: `config.GDALConfig`, `domain.BBox`, `domain.Band`
- Produces:
  - `gdal.Executor` struct with:
    - `Run(ctx context.Context, command string, args []string) (stdout string, stderr string, err error)` — runs a GDAL command with timeout, validates exit code
  - `gdal.Info(ctx, executor *Executor, inputPath string) (*GDALInfo, error)` — runs gdalinfo -json, parses output into struct with size, bands, projection, bounds
  - `gdal.Translate(ctx, executor *Executor, opts TranslateOpts) error` — runs gdal_translate with projwin for BBOX crop
  - `gdal.Warp(ctx, executor *Executor, opts WarpOpts) error` — runs gdalwarp with target resolution, resampling method, target SRS
  - `gdal.BuildVRT(ctx, executor *Executor, opts VRTOpts) error` — runs gdalbuildvrt to compose multiple bands into VRT
  - `TranslateOpts{Input, Output string, BBox *domain.BBox, OutputFormat string}`
  - `WarpOpts{Input, Output string, TargetResolution int, ResamplingMethod string, TargetSRS string, BBox *domain.BBox}`
  - `VRTOpts{Inputs []string, Output string, Separate bool}`
  - `GDALInfo{Width, Height int, Bands int, Projection string, BoundsMinX, BoundsMinY, BoundsMaxX, BoundsMaxY float64}`

- [ ] **Step 1: Create a tiny test GeoTIFF**

Use GDAL to create a minimal test file (can be done in test setup):

```bash
gdal_create -of GTiff -outsize 10 10 -bands 1 -burn 128 testdata/tiny.tif
```

Or generate one programmatically in the test setup using gdal_translate from a VRT.

- [ ] **Step 2: Write executor test**

Test that `executor.Run` correctly captures stdout, stderr, and returns error on non-zero exit. Test timeout behavior. Skip tests if `gdalinfo` is not found in PATH.

```go
func TestExecutorRun(t *testing.T) {
	if _, err := exec.LookPath("gdalinfo"); err != nil {
		t.Skip("gdalinfo not found in PATH")
	}
	e := NewExecutor(300)
	stdout, stderr, err := e.Run(context.Background(), "gdalinfo", []string{"--version"})
	if err != nil {
		t.Fatalf("gdalinfo --version failed: %v (stderr: %s)", err, stderr)
	}
	if !strings.Contains(stdout, "GDAL") {
		t.Errorf("expected GDAL in output, got: %s", stdout)
	}
}

func TestExecutorRunTimeout(t *testing.T) {
	e := NewExecutor(1) // 1 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, _, err := e.Run(ctx, "sleep", []string{"10"})
	if err == nil {
		t.Error("expected timeout error")
	}
}
```

- [ ] **Step 3: Implement executor, info, translate, warp, vrt**

The executor runs commands via `exec.CommandContext`, captures stdout/stderr, checks exit code, and wraps errors in `domain.ProcessingError{Type: domain.ErrGDAL}`.

- [ ] **Step 4: Write info parsing test with tiny.tif**

Test that `Info` returns correct width, height, band count from the test file.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/infrastructure/gdal/ -v
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: GDAL executor with translate, warp, vrt, info commands"
```

---
