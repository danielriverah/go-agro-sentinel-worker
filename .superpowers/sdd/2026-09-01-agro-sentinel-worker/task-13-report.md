# Task 13 Report: IA service client and integration

## Status
Complete.

## What was done
- `internal/infrastructure/ia/client.go`: new `ia.Client` with `New(cfg config.IAConfig) *Client` and `Analyze(ctx, worker.IAInput) (*domain.AnalysisResult, error)`. Signature matches the pre-existing `worker.IAClient` interface (which takes `worker.IAInput`, not `json.RawMessage` as the brief's illustrative snippet showed) so the client satisfies it directly.
  - Returns `nil, nil` immediately when `cfg.Enabled == false`.
  - POSTs a JSON body (produccion_id, scene_id, multiband_path, params) to `{ServiceURL}/analyze` using an `http.Client` with timeout from `cfg.TimeoutSeconds` (default 30s if unset).
  - Non-2xx status, request/transport errors (timeout, connection refused), and response-decode failures are all wrapped as `&domain.ProcessingError{Type: domain.ErrIA, ...}`.
- `internal/infrastructure/ia/client_test.go`: httptest-based tests — success path, disabled client, `posible_cosecha=true` passthrough, non-2xx response wrapping, and connection-refused wrapping. All assert the concrete `*domain.ProcessingError{Type: domain.ErrIA}` on failure paths.
- `internal/worker/worker.go`:
  - `ProductionRepository` interface extended with `SetBloqueado(ctx, produccionID int64, motivo string) error` (worker-side subset interface; `database.ProductionRepo` already implemented this method from Task 3, so no change needed there).
  - In `process()`, after `analisis.json` is written from a successful IA result, if `iaAnalysis.PosibleCosecha == true` the worker now calls `w.deps.Productions.SetBloqueado(ctx, production.ProduccionID, "posible_cosecha detectada por IA")`. A `SetBloqueado` failure is logged but does not fail the scene (blocking is best-effort; the analysis result and scene completion are unaffected).
  - The existing `runIA`/error-tolerance path (IA_ERROR → partial, scene still COMPLETED without `analisis.json`) was already implemented in a prior task and required no changes; verified via existing `TestProcessScene_IAError_StillCompletes`.
- `cmd/worker/main.go`: wired the real `ia.New(cfg.IA)` client into `WorkerDeps.IA` (previously `nil` with a "wired in Task 13" comment).
- `internal/worker/worker_test.go`: `mockProductionRepo` extended with a `SetBloqueado` implementation (records call/motivo) to satisfy the widened interface; added two new tests:
  - `TestProcessScene_PosibleCosecha_BlocksProduction` — asserts `SetBloqueado` is called with a non-empty motivo and the scene still completes with `HasAnalisis=true`.
  - `TestProcessScene_NoCosecha_DoesNotBlockProduction` — asserts `SetBloqueado` is NOT called when `PosibleCosecha=false`.

## Commit(s)
- `feat: IA service client with harvest detection and blocking`

## Tests summary
- `go build ./...` — passes.
- `go test ./internal/infrastructure/ia/... ./internal/worker/...` — all pass (5 new IA client tests, 2 new worker tests, all pre-existing worker tests including the IA-error-tolerance test still pass).
- `go test ./...` — full suite passes.
- `gofmt -l` — clean on the two new files (pre-existing formatting drift in `internal/processing/params.go` and `statistics.go` is unrelated/untouched).

## Concerns
- The brief's illustrative test snippet used `Analyze(ctx, json.RawMessage(...))`, but the actual `worker.IAClient` interface (already defined in Task 11/12) uses `Analyze(ctx, IAInput)`. Followed the brief's own instruction to "check the actual interface definition" and implemented against `worker.IAInput` instead — this is the version that type-checks against the worker's dependency wiring.
- The real IA service's request/response contract (exact JSON field names/shape) is not yet specified anywhere in the repo; the request body shape (`produccion_id`, `scene_id`, `multiband_path`, `params`) and response shape (`domain.AnalysisResult`'s existing JSON tags) are my best inference and may need adjustment once the actual IA service API is known.
- Not verified against a real/live IA service — only via `httptest` mocks, consistent with how other infrastructure clients in this repo have been tested to date.
