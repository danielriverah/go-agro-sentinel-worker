### Task 7: Processing core — multiband.tif generation

**Files:**
- Create: `internal/processing/bands.go`
- Create: `internal/processing/multiband.go`
- Create: `internal/storage/filesystem.go`
- Test: `internal/processing/multiband_test.go`
- Test: `internal/storage/filesystem_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor.Run(ctx, command, args) (stdout, stderr, err)`
  - `gdal.Warp(ctx, executor, WarpOpts) error`
  - `gdal.BuildVRT(ctx, executor, VRTOpts) error`
  - `gdal.Translate(ctx, executor, TranslateOpts) error`
  - `domain.BBox`, `domain.Band`, `domain.BandInfo`, `domain.AllSpectralBands()`
  - `aws.S3Client.Upload`, `aws.S3Client.HeadObject`
  - `config.ProcessingConfig`
- Produces:
  - `processing.MultibandBuilder` struct with:
    - `New(executor GDALExecutor, s3 S3Uploader, cfg config.ProcessingConfig, logger *slog.Logger) *MultibandBuilder`
    - `Build(ctx, jobDir string, bbox domain.BBox, bands []domain.BandInfo, targetResolution int) (outputPath string, err error)`
  - `processing.GDALExecutor` interface wrapping gdal executor methods
  - `processing.S3Uploader` interface `{Upload(ctx, bucket, key, filePath) error; HeadObject(ctx, bucket, key) (bool, int64, error)}`
  - `storage.JobDir` struct with:
    - `New(baseDir string, jobID string) *JobDir`
    - `Input() string` — returns `{baseDir}/jobs/{jobID}/input/`
    - `Work() string` — returns `{baseDir}/jobs/{jobID}/work/`
    - `Output() string` — returns `{baseDir}/jobs/{jobID}/output/`
    - `Create() error` — creates all subdirectories
    - `Cleanup() error` — removes the entire job directory

- [ ] **Step 1: Write filesystem test**

```go
func TestJobDir(t *testing.T) {
	base := t.TempDir()
	jd := New(base, "test-job-123")

	if err := jd.Create(); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	for _, dir := range []string{jd.Input(), jd.Work(), jd.Output()} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("directory %s should exist", dir)
		}
	}

	if err := jd.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if _, err := os.Stat(jd.Input()); !os.IsNotExist(err) {
		t.Error("job directory should be removed after cleanup")
	}
}
```

- [ ] **Step 2: Implement filesystem.go**

- [ ] **Step 3: Write multiband build test**

Test with mock GDALExecutor that records command calls. Verify:
- Warp is called once per band with correct BBOX and target resolution
- 20m bands get resampling_method=bilinear
- BuildVRT is called with all warped bands, `separate=true`
- Translate is called to convert VRT to GeoTIFF
- Output path is in the job output directory

- [ ] **Step 4: Implement multiband.go**

The flow:
1. For each band: `gdalwarp` with `-te bbox -tr resolution -r bilinear` on the COG href → cropped/resampled band in work dir
2. `gdalbuildvrt -separate` all warped bands → composite.vrt
3. `gdal_translate` composite.vrt → multiband.tif in output dir

- [ ] **Step 5: Run tests**

```bash
go test ./internal/processing/ -v
go test ./internal/storage/ -v
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: multiband.tif generation from COG bands via GDAL"
```

---

