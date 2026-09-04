### Task 10: Statistics and params.json generation

**Files:**
- Create: `internal/processing/statistics.go`
- Create: `internal/processing/params.go`
- Test: `internal/processing/statistics_test.go`
- Test: `internal/processing/params_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor`, `gdal.Info`
  - `domain.Band`, `domain.AllSpectralBands()`
  - `processing.CoverageStats`
  - `aws.S3Client.Download` (to fetch previous params.json)
  - `database.SceneRepo.GetPreviousValidScene`
- Produces:
  - `processing.CalculateBandStatistics(ctx, executor GDALExecutor, multibandPath string) (map[domain.Band]BandStats, error)`
  - `processing.BandStats` struct `{Mean, Std, Min, Max, P25, P50, P75 float64}`
  - `processing.CalculateIndexStatistics(ctx, executor GDALExecutor, multibandPath string, indices []IndexDefinition) (map[domain.FileType]IndexStats, error)`
  - `processing.IndexStats` struct `{Mean, Std, Min, Max, P25, P50, P75 float64}`
  - `processing.BuildParams(current ParamsInput, previousParams *Params) *Params`
  - `processing.Params` struct matching the params.json schema from the spec
  - `processing.ParamsInput` struct `{ProduccionID int64, SceneID string, SceneDate, FechaPlantacion time.Time, CloudCoverBBox float64, Indices map[FileType]IndexStats, BandStats map[Band]BandStats, Coverage CoverageStats}`
  - `processing.TrimHistorico(historico []HistoricoEntry, maxEntries int) []HistoricoEntry` — keeps last N entries

- [ ] **Step 1: Write BuildParams test**

```go
func TestBuildParams(t *testing.T) {
	current := ParamsInput{
		ProduccionID:   1234,
		SceneID:        "S2A_scene2",
		SceneDate:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		FechaPlantacion: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		CloudCoverBBox: 12.5,
		Indices: map[FileType]IndexStats{
			FileNDVI: {Mean: 0.72, Std: 0.08},
		},
		Coverage: CoverageStats{VegetationPct: 78.5, SoilPct: 15.2},
	}

	previous := &Params{
		ProduccionID:      1234,
		SceneID:           "S2A_scene1",
		SceneDate:         "2026-08-22",
		DiasDesdePlantacion: 38,
		Indices: map[string]IndexStats{
			"ndvi": {Mean: 0.60},
		},
		Historico: nil, // first scene had no history
	}

	params := BuildParams(current, previous)

	if params.DiasDesdePlantacion != 48 {
		t.Errorf("dias = %d, want 48", params.DiasDesdePlantacion)
	}
	if len(params.Historico) != 1 {
		t.Fatalf("historico length = %d, want 1", len(params.Historico))
	}
	if params.Historico[0].SceneID != "S2A_scene1" {
		t.Error("historico[0] should be previous scene")
	}
	if params.Historico[0].Delta["ndvi_mean_change"] != 0.12 {
		t.Errorf("ndvi delta = %v, want 0.12", params.Historico[0].Delta["ndvi_mean_change"])
	}
}

func TestTrimHistorico(t *testing.T) {
	entries := make([]HistoricoEntry, 25)
	for i := range entries {
		entries[i] = HistoricoEntry{SceneID: fmt.Sprintf("scene_%d", i)}
	}

	trimmed := TrimHistorico(entries, 20)
	if len(trimmed) != 20 {
		t.Errorf("trimmed length = %d, want 20", len(trimmed))
	}
}
```

- [ ] **Step 2: Implement statistics.go and params.go**

Statistics use `gdalinfo -stats -json` on each band of multiband.tif to extract min, max, mean, std. Percentiles can be calculated via `gdal_translate` with histogram analysis.

params.go builds the JSON structure, calculates `dias_desde_plantacion`, builds the historical chain with deltas.

- [ ] **Step 3: Run tests and commit**

```bash
go test ./internal/processing/ -v -run "Params|Statistics|Trim"
git add .
git commit -m "feat: statistics calculation and params.json with historical chain"
```

---

