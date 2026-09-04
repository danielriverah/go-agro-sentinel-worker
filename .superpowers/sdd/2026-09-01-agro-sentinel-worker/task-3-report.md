# Task 3 Report: MySQL migrations and repositories

## Status
Complete.

## Files created
- `internal/infrastructure/database/mysql.go` — `NewConnection(cfg config.MySQLConfig) (*sql.DB, error)`, pools via go-sql-driver/mysql with `parseTime=true`.
- `internal/infrastructure/database/migrations.go` — `RunMigrations(db *sql.DB) error`, `CREATE TABLE IF NOT EXISTS` for `s3_monitoring_producciones`, `s3_monitoring_escenas`, `s3_monitoring_escena_archivos` matching the spec's column definitions, including the unique keys (`produccion_id` on producciones, `(produccion_id, scene_id)` on escenas).
- `internal/infrastructure/database/production_repo.go` — `ProductionRepo` with `Upsert` (INSERT ... ON DUPLICATE KEY UPDATE), `GetByProduccionID`, `ListActive`, `UpdateMonitoring`, `UpdateBBox`, `SetBloqueado`, `Desbloquear`, `IncrementEscenas`.
- `internal/infrastructure/database/scene_repo.go` — `SceneRepo` with `Upsert`, `GetByID`, `GetByProduccionAndSceneID`, `ListByProduccion`, `UpdateStatus`, `SetError` (increments `retry_count`), `SetCompleted`, `GetPreviousValidScene` (most recent scene before a date with `passes_quality=1`).
- `internal/infrastructure/database/file_repo.go` — `FileRepo` with `Create` (sets `f.ID` from `LastInsertId`), `ListByEscena`, `GetByType`.
- `internal/infrastructure/database/testdb_test.go` — shared test helper `testDB(t)` that skips when `MYSQL_TEST_DSN` is unset, otherwise connects and runs `RunMigrations`; `randomID()` helper for unique test IDs.
- `internal/infrastructure/database/production_repo_test.go`, `scene_repo_test.go`, `file_repo_test.go` — cover insert/get, upsert-no-duplicate, status updates, `SetBloqueado`/`Desbloquear`, `GetPreviousValidScene`, `ListActive`, `IncrementEscenas`, file create/list/GetByType.

All repo methods take `context.Context` as the first parameter and use `*sql.DB` (no external transaction manager needed at this layer). NULL-able domain fields (`BBox`, `BloqueadoAt`, `FechaPlantacion`, `FechaFinMonitoreo`, `LastSyncAt`, `CloudCoverBBox`, string "empty means NULL" fields) are handled via `sql.Null*` scanning and a `nullString` helper on write.

## Dependency
Added `github.com/go-sql-driver/mysql v1.10.1` (pulls in `filippo.io/edwards25519`) via `go get`; `go.mod`/`go.sum` updated.

## Commit(s)
Not committed — per project instructions, commits are only created when explicitly requested by the user. All files above are staged in the working tree, uncommitted.

## Tests summary
- `go build ./...` and `go vet ./...` pass cleanly.
- `go test ./internal/infrastructure/database/... -v`: all 13 tests run and **SKIP** (`MYSQL_TEST_DSN not set`), which is the expected/designed behavior per the brief ("skip if no test DB available").
- No MySQL server was reachable on this machine (port 3306 not listening; XAMPP's MySQL service was not running and was not started, since starting services was out of scope). Integration behavior (upsert dedup, `SetBloqueado`/`Desbloquear`, `GetPreviousValidScene` ordering, etc.) is therefore **unverified against a real database** in this session — only compiled/reviewed for correctness.

To verify against a real MySQL later:
```
MYSQL_TEST_DSN="user:pass@tcp(localhost:3306)/agro_test" go test ./internal/infrastructure/database/ -v
```

## Concerns
- Integration tests could not be executed against a live MySQL instance in this environment; recommend running them in CI or against XAMPP's MySQL (start the service, create an `agro_test` schema) before relying on this layer in production.
- `NewConnection` uses `loc=UTC` in the DSN so `parseTime` returns UTC times consistently with `time.Now().UTC()` used in repo writes; if the target MySQL server's `sql_mode`/timezone tables differ, this should still be safe since conversion happens driver-side.
- DECIMAL columns for bbox/cloud cover are scanned into `float64`/`sql.NullFloat64` for simplicity; this is fine for the coordinate/percentage precision used here but is worth a note if exact decimal semantics are ever required elsewhere.
