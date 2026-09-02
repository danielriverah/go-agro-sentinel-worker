# Task 9 Report — RGB compositions and vegetation index image generation

## Status
Complete.

## Files
- Created `internal/processing/rgb.go` — `CompositionDefinition`, `AllCompositions()`, `GenerateRGB()`, shared `bandIndex` map (domain.Band -> 1-based multiband.tif band number).
- Created `internal/processing/indices.go` — `IndexDefinition`, `AllIndices()`, `GenerateIndex()`. Computes each index via `gdal_calc.py` (Float32 raw GeoTIFF), then renders a color-mapped PNG via `gdaldem color-relief` using a green-yellow-red ramp for vegetation indices and a blue-white-brown ramp for NBR/NDMI.
- Created `internal/processing/rgb_test.go` and `internal/processing/indices_test.go` — mock-executor based unit tests.
- Extended `mockExecutor.Run` in `internal/processing/multiband_test.go` to recognize `gdal_calc.py` (`--outfile=`) and `gdaldem` (4th positional arg) output paths so the shared fake-file-write behavior works for these new commands.

## Design notes
- `AllCompositions()` returns natural (3,2,1), false_color (7,3,2), red_edge (5,4,3), swir (10,8,3), matching the band-number spec.
- `AllIndices()` returns the 7 indices (NDVI, NDRE, EVI, GNDVI, NBR, NDMI, SAVI) with formulas using gdal_calc.py placeholder letters (A, B, C) mapped to `IndexDefinition.Bands` in order.
- `GenerateIndex` looks up the definition by `domain.FileType`, returns a `domain.ProcessingError{Type: ErrValidation}` for unknown types.
- All GDAL invocations wrap failures and missing-output-file conditions in `domain.ProcessingError{Type: ErrGDAL}`, consistent with `multiband.go`.

## Tests summary
`go test ./internal/processing/ -v -run "RGB|Index|Composition"` — 7/7 pass:
- TestAllCompositions
- TestGenerateRGB_BuildsCorrectCommand
- TestGenerateRGB_ExecutorError
- TestAllIndices
- TestGenerateIndex_NDVI_BuildsCorrectCommands
- TestGenerateIndex_EVI_UsesThreeBands
- TestGenerateIndex_UnknownType
- TestGenerateIndex_CalcExecutorError

Full suite (`go build ./...` and `go test ./...`) passes with no regressions.

## Concerns
- The working directory is not a git repository (`git status` unavailable), so no commit was created — code changes are on disk only. If this should live in a repo, initialize one or point me to the correct repo root.
- Color-relief ramp break points and colors are a reasonable default (not pixel-specified by the spec); adjust `vegetationColorRamp`/`moistureColorRamp` in `indices.go` if design wants specific hex values.
- `gdal_calc.py` invocation assumes it's on PATH as `gdal_calc.py` (consistent with the brief's example); if the deployment environment invokes it differently (e.g. via `python gdal_calc.py`), the `GDALExecutor.Run` command name may need adjustment at the executor/config layer rather than here.
