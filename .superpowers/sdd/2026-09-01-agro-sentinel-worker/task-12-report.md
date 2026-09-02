# Task 12 Report: REST API with Scalar

## Status
Complete.

## Commit(s)
See `git log -1` after commit created by this task (message: "feat: REST API with all endpoints and Scalar documentation").

## What was built
- `internal/http/responses.go`: `JSON(w, status, data)` and `Error(w, status, message)` helpers using a consistent `{"data": ...}` / `{"error": ...}` envelope.
- `internal/http/middleware.go`: `LoggingMiddleware(logger, next)` logging method, path, status, duration.
- `internal/http/handlers.go`: `Handlers` struct with local interfaces (`ProductionRepository`, `SceneRepository`, `FileRepository`, `Presigner`, `Syncer`) satisfied by `database.ProductionRepo`, `database.SceneRepo`, `database.FileRepo`, `aws.S3Client`, and `sync.Service` respectively. Implements:
  - `ListProducciones` — `GET /api/v1/producciones`
  - `GetProduccion` — `GET /api/v1/producciones/{id}` (production + its escenas)
  - `DesbloquearProduccion` — `POST /api/v1/producciones/{id}/desbloquear` (optional `{"usuario": "..."}` body)
  - `GetEscenaArchivo` — `GET /api/v1/escenas/{id}/archivos/{tipo}` (presigned S3 URL, 15 min expiry)
  - `TriggerSync` — `POST /api/v1/sync/trigger` (runs sync in background goroutine, returns 202 immediately)
  - `HealthHandler` — `GET /health` (preserved)
- `internal/http/docs.go`: embedded OpenAPI 3.0 YAML spec (`openapiSpec`) and Scalar UI HTML (`scalarHTML`, CDN-loaded from `@scalar/api-reference`).
- `internal/http/router.go`: `NewRouter(logger, *Handlers)` wires `/health`, `/docs`, `/api/v1/openapi.yaml`, and all API routes using Go 1.22+ method-pattern routing, wrapped in `LoggingMiddleware`.
- `cmd/api/main.go`: wires MySQL connection, AWS session, S3 client, DynamoDB client, all repos, and the `sync.Service` (used both as the sync trigger dependency and built the same way `cmd/sync/main.go` builds it) into `Handlers`, then serves on `cfg.Server.Host:cfg.Server.Port` (configured via `configs/config.yaml`, expected to be set to port 6000).
- `internal/http/handlers_test.go`: expanded with mock repos/presigner/syncer and tests for every endpoint, including not-found and 202/503 paths for sync trigger.

## Tests summary
`go test ./internal/http/... -v` — 9/9 passed:
TestHealthHandler, TestListProducciones, TestGetProduccion, TestGetProduccionNotFound, TestDesbloquearProduccion, TestGetEscenaArchivo, TestGetEscenaArchivoNotFound, TestTriggerSync, TestTriggerSyncNotConfigured.

`go build ./...` succeeds for the whole module (including `cmd/api`, `cmd/sync`, `cmd/worker`).

## Concerns
- Port 6000: the brief says "API on port 6000" but the app reads the port from `configs/config.yaml` (`server.port`), consistent with the existing pattern in this codebase (no hardcoded ports elsewhere). Confirm `configs/config.yaml` sets `server.port: 6000` in the deployed environment; not modified here since no existing config file was found in the repo to edit.
- `TriggerSync` fires the sync cycle in a detached goroutine with `context.Background()` (not tied to the request context), so it keeps running after the HTTP response is sent — this matches the "returns 202 Accepted" spec but means the caller gets no result/error from that cycle; errors are only visible in server logs (via `sync.Service`'s own logger).
- The Scalar `<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference">` tag has no Subresource Integrity (SRI) hash, since Scalar's CDN bundle is unpinned/versionless in the brief's example; consider pinning a version and adding an `integrity` attribute if this is exposed publicly.
- `GetProduccion` embeds `*domain.Production` by pointer in `produccionDetail`, which flattens the production's fields alongside `escenas` in the JSON response (matches spec's "production detail with all its scenes").
