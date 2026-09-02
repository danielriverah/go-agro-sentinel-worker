# Task 11 Report: Processing worker orchestration

## Status: Complete

## Summary

Implemented `internal/worker/worker.go` with `Worker.ProcessScene`, wiring together every processing package (multiband, cloud cover, RGB compositions, indices, statistics, params) plus the database repos and S3 client behind local interfaces (`ProductionRepository`, `SceneRepository`, `FileRepository`, `S3Client`, `GDALExecutor`, `BandResolver`, `IAClient`) so the pipeline is fully mockable.

Flow implemented matches the brief's 1-13 steps: fetch production/scene, validate (`monitoring=true`, bbox present, not bloqueado — failure sets scene FAILED/VALIDATION_ERROR without ever marking PROCESSING), mark PROCESSING, create a `storage.JobDir` (deferred `Cleanup`), build `multiband.tif`, compute SCL cloud cover, branch on the 23% threshold (full compositions+indices+stats+params.json below, natural.png only at/above), best-effort IA analysis (nil-safe, errors logged and tolerated — `has_analisis=false`), upload every output file to S3 and register it via `FileRepo.Create`, then finalize the scene via `SceneRepo.Upsert` (status COMPLETED, `passes_quality`, `cloud_cover_bbox`, `has_multiband`/`has_params`/`has_rgb`/`has_analisis`). Any error is classified through `*domain.ProcessingError` (defaulting unclassified errors to `VALIDATION_ERROR`) and recorded via `SceneRepo.SetError`, which also increments `retry_count`.

One dependency the brief didn't already provide: how COG band hrefs are discovered for a scene. No STAC/COG-discovery client exists yet anywhere in the codebase, so I added a local `BandResolver` interface (`ResolveBands(ctx, produccionID, sceneID) ([]domain.BandInfo, sclHref string, err error)`) per the brief's instruction to define local interfaces for each dependency. `cmd/worker/main.go` wires a `stacBandResolver` placeholder that returns a clear `VALIDATION_ERROR` until STAC discovery is implemented in a future task — everything else in `main.go` (MySQL, S3, GDAL executor, all repos) is wired to real implementations.

Also added `storage.JobDir.Root()` (additive, non-breaking) since `processing.MultibandBuilder.Build` needs the job's root directory path, which `JobDir` didn't previously expose.

## Commits

- (pending — see below) `feat: processing worker orchestration with full scene pipeline`

## Tests

`go test ./internal/worker/... -v` — 6/6 pass:
- `TestProcessScene_HappyPath_FullProcessing` — 12% cloud cover, verifies PROCESSING status update, all 13 output files (multiband + 4 compositions + 7 indices + params.json) uploaded/registered, scene finalized COMPLETED with correct flags.
- `TestProcessScene_AboveCloudThreshold_NaturalOnly` — 40% cloud cover, verifies only multiband.tif + natural.png are produced, `passes_quality=false`, `has_params=false`.
- `TestProcessScene_GDALError_MarksFailed` — gdalwarp failure propagates as `GDAL_ERROR`, scene never finalized as completed, `SetError` called once with the right type.
- `TestProcessScene_IAError_StillCompletes` — IA client returns an error; ProcessScene still returns nil, scene COMPLETED, `has_analisis=false`, no `analisis.json` registered.
- `TestProcessScene_ValidationError_ProductionNotMonitoring` — production.Monitoring=false short-circuits before any PROCESSING status update; scene marked FAILED/VALIDATION_ERROR.
- `TestProcessScene_HistoricalChain_UsesPreviousParams` — previous valid scene has a registered params.json in S3; verifies it's downloaded/parsed and a new params.json is produced and registered.

Full repo suite (`go test ./...`) passes; `go build ./...` and `go vet ./...` are clean; `gofmt -l` clean.

## Concerns

- `BandResolver` (COG href discovery) is new scope beyond the brief's explicit interface list — necessary because no STAC client exists yet in the codebase. `cmd/worker/main.go` wires a placeholder that errors clearly; a future task must implement real STAC/COG discovery and satisfy this interface for the worker to be end-to-end functional.
- `SceneRepo.Upsert` (rather than a dedicated "complete" method) is used to persist the final status + all `has_*` flags together, since the existing `SetCompleted` method doesn't cover `has_analisis`/`has_params`/`has_rgb`. This matches the existing repo surface without requiring schema/repo changes.
- IA client interface and wiring are stubs per the brief — Task 13 provides the real implementation; `cmd/worker/main.go` passes `IA: nil`.
