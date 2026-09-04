### Task 8: SCL cloud cover calculation

**Files:**
- Create: `internal/processing/cloudcover.go`
- Test: `internal/processing/cloudcover_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor`, `gdal.Warp`, `gdal.Info`
  - `domain.BBox`, `domain.BandSCL`
- Produces:
  - `processing.CalculateCloudCover(ctx, executor GDALExecutor, sclHref string, bbox domain.BBox, workDir string) (cloudCoverPct float64, coverage CoverageStats, err error)`
  - `processing.CoverageStats` struct `{VegetationPct, SoilPct, WaterPct, CloudPct float64}`

The function:
1. Warp SCL band to BBOX at 20m (native resolution, no resampling — nearest neighbor for categorical data)
2. Run `gdal_translate -of AAIGrid` to get ASCII grid (or use gdalinfo with -stats and histogram)
3. Parse pixel value counts per SCL class
4. Calculate cloud_cover_bbox per spec formula: `(pixels_3 + pixels_8 + pixels_9 + pixels_10) / total_valid * 100`
5. Calculate coverage stats from SCL classes for params.json

- [ ] **Step 1: Write test with known pixel distributions**

Use a mock executor that returns a pre-defined histogram from gdalinfo.

- [ ] **Step 2: Implement cloudcover.go**

- [ ] **Step 3: Run tests and commit**

```bash
go test ./internal/processing/ -v -run CloudCover
git add .
git commit -m "feat: SCL-based cloud cover calculation and coverage stats"
```

---

