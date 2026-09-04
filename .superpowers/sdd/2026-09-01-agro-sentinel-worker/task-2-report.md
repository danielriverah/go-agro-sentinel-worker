# Task 2 Report: Domain Entities

## Status
Complete — all steps in the brief followed verbatim.

## Files Created
- `internal/domain/errors.go` — `ErrType`, `ProcessingError` (implements `error`, `Unwrap`)
- `internal/domain/band.go` — `Band`, `BandInfo`, `AllSpectralBands()`, `BandsAtResolution()`, `Band.Resolution()`
- `internal/domain/production.go` — `BBox` (+ `Validate()`), `Production` (+ `ShouldProcess()`)
- `internal/domain/scene.go` — `JobStatus`, `Scene` (+ `NeedsProcessing()`, `CanRetry()`), `SceneFile`
- `internal/domain/product.go` — `FileType` constants, `AllImageTypes()`
- `internal/domain/analysis.go` — `AnalysisResult`
- `internal/domain/production_test.go`
- `internal/domain/band_test.go`
- `internal/domain/scene_test.go`

Note: the brief's file list mentioned `internal/domain/job.go`, but `JobStatus` was specified in Step 5 to live in `scene.go` (no separate content was given for `job.go`), so no separate file was created for it — all required types exist in the package.

## Commit(s)
`60fb6ca` — "feat: domain entities — production, scene, band, errors" (9 files changed, 346 insertions)

## Tests Summary
- Pre-implementation: `go test ./internal/domain/ -v` failed to build (undefined symbols), as expected.
- Post-implementation: `go build ./...` succeeded; `go test ./internal/domain/ -v` — all tests PASS:
  - TestAllSpectralBands
  - TestBandsAtResolution
  - TestBandResolution
  - TestBBoxValidate (3 subtests)
  - TestProductionShouldMonitor
  - TestSceneNeedsProcessing

## Concerns
- None functionally. `job.go` was not created as a standalone file since the brief provided no distinct content for it beyond what's in `scene.go`; flagging this in case a later task expects that exact filename.
- Git line-ending warnings (LF → CRLF) appeared on commit; cosmetic only, no `.gitattributes` configured in this repo.
