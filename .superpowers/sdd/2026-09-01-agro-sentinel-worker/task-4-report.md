# Task 4 Report: GDAL Executor

## Status
Complete.

## Files created
- `internal/infrastructure/gdal/executor.go` — `Executor` struct, `NewExecutor(timeoutSeconds)`, `Run(ctx, command, args)` using `exec.CommandContext`, captures stdout/stderr, applies default timeout when ctx has no deadline, wraps failures (non-zero exit, timeout, start failure) as `domain.ProcessingError{Type: domain.ErrGDAL}`.
- `internal/infrastructure/gdal/info.go` — `GDALInfo` struct and `Info(ctx, executor, inputPath)`; checks input file exists, runs `gdalinfo -json`, parses into `gdalInfoJSON` (size, bands, coordinateSystem.wkt, cornerCoordinates), maps to `GDALInfo{Width, Height, Bands, Projection, BoundsMinX/Y, BoundsMaxX/Y}`.
- `internal/infrastructure/gdal/translate.go` — `TranslateOpts` and `Translate(ctx, executor, opts)`; runs `gdal_translate -of <format> [-projwin ulx uly lrx lry] input output`, verifies output file exists after success.
- `internal/infrastructure/gdal/warp.go` — `WarpOpts` and `Warp(ctx, executor, opts)`; runs `gdalwarp [-tr res res] [-r method] [-t_srs srs] [-te minx miny maxx maxy] input output`, verifies output file exists after success.
- `internal/infrastructure/gdal/vrt.go` — `VRTOpts` and `BuildVRT(ctx, executor, opts)`; runs `gdalbuildvrt [-separate] output input...`, verifies output file exists after success.
- `internal/infrastructure/gdal/executor_test.go` — tests `Run` for successful version check, non-zero exit, context-deadline timeout, and default-timeout-applied-without-explicit-deadline. GDAL-dependent tests skip if `gdalinfo` is not in PATH; timeout tests use a cross-platform sleep helper (`sleep` on Unix, `ping -n` on Windows) and skip if that command is unavailable.
- `internal/infrastructure/gdal/info_test.go` — `TestInfoParsesTinyTif` (creates `testdata/tiny.tif` via `gdal_create` if missing, skips if `gdal_create`/`gdalinfo` unavailable), `TestInfoNonexistentFile`, and `TestGDALInfoJSONParsing` (pure JSON-parsing unit test against a mock `gdalinfo -json` payload, no GDAL binary required).
- `testdata/tiny.tif` — NOT committed. GDAL is not installed in this dev environment (`gdalinfo`, `gdal_create`, etc. are absent from PATH), so the fixture could not be generated. The test helper `ensureTinyTif` creates it on demand via `gdal_create` when GDAL is present, and skips gracefully otherwise.

## Tests summary
```
go build ./...        -> OK
go vet ./...           -> OK
go test ./internal/infrastructure/gdal/... -v
=== RUN   TestExecutorRun                              SKIP (gdalinfo not found in PATH)
=== RUN   TestExecutorRunNonZeroExit                   SKIP (gdalinfo not found in PATH)
=== RUN   TestExecutorRunTimeout                       PASS
=== RUN   TestExecutorRunDefaultTimeoutAppliedWithoutDeadline  PASS
=== RUN   TestInfoParsesTinyTif                        SKIP (gdalinfo not found in PATH)
=== RUN   TestInfoNonexistentFile                      PASS
=== RUN   TestGDALInfoJSONParsing                       PASS
PASS
ok agro-sentinel-worker/internal/infrastructure/gdal 2.452s
```

## Concerns
- GDAL command-line tools (`gdalinfo`, `gdal_translate`, `gdalwarp`, `gdalbuildvrt`, `gdal_create`) are not installed on this development machine, so the GDAL-dependent tests (real command execution, real JSON parsing against a live `gdalinfo`, and `testdata/tiny.tif` generation) could not be exercised end-to-end here — they are written to skip cleanly and should be re-run in an environment (or CI image) with GDAL installed to get full coverage, including generating and committing `testdata/tiny.tif`.
- `TranslateOpts.OutputFormat` defaults to `GTiff` when empty; adjust if a different default is desired.
- Timeout tests use `sleep`/`ping` as a stand-in long-running process rather than an actual GDAL binary, since no GDAL command reliably blocks for a fixed duration; this still exercises the same `exec.CommandContext` timeout path used for real GDAL invocations.
