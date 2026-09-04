### Task 3: MySQL migrations and repositories

**Files:**
- Create: `internal/infrastructure/database/mysql.go`
- Create: `internal/infrastructure/database/migrations.go`
- Create: `internal/infrastructure/database/production_repo.go`
- Create: `internal/infrastructure/database/scene_repo.go`
- Create: `internal/infrastructure/database/file_repo.go`
- Test: `internal/infrastructure/database/production_repo_test.go`
- Test: `internal/infrastructure/database/scene_repo_test.go`
- Test: `internal/infrastructure/database/file_repo_test.go`

**Interfaces:**
- Consumes: `domain.Production`, `domain.Scene`, `domain.SceneFile`, `domain.BBox`, `config.MySQLConfig`
- Produces:
  - `database.NewConnection(cfg config.MySQLConfig) (*sql.DB, error)`
  - `database.RunMigrations(db *sql.DB) error` — creates the 3 tables if they don't exist
  - `database.ProductionRepo` struct with methods:
    - `Upsert(ctx, production *domain.Production) error`
    - `GetByProduccionID(ctx, produccionID int64) (*domain.Production, error)`
    - `ListActive(ctx) ([]*domain.Production, error)`
    - `UpdateMonitoring(ctx, produccionID int64, monitoring bool, motivo string) error`
    - `UpdateBBox(ctx, produccionID int64, bbox domain.BBox) error`
    - `SetBloqueado(ctx, produccionID int64, motivo string) error`
    - `Desbloquear(ctx, produccionID int64, usuario string) error`
    - `IncrementEscenas(ctx, produccionID int64, valid bool) error`
  - `database.SceneRepo` struct with methods:
    - `Upsert(ctx, scene *domain.Scene) error`
    - `GetByID(ctx, id int64) (*domain.Scene, error)`
    - `GetByProduccionAndSceneID(ctx, produccionID int64, sceneID string) (*domain.Scene, error)`
    - `ListByProduccion(ctx, produccionID int64) ([]*domain.Scene, error)`
    - `UpdateStatus(ctx, id int64, status domain.JobStatus) error`
    - `SetError(ctx, id int64, errType string, errMsg string) error`
    - `SetCompleted(ctx, id int64, passesQuality bool, cloudCoverBBox float64) error`
    - `GetPreviousValidScene(ctx, produccionID int64, beforeDate time.Time) (*domain.Scene, error)`
  - `database.FileRepo` struct with methods:
    - `Create(ctx, file *domain.SceneFile) error`
    - `ListByEscena(ctx, escenaID int64) ([]*domain.SceneFile, error)`
    - `GetByType(ctx, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error)`

- [ ] **Step 1: Write migration SQL constants and test**

Create `internal/infrastructure/database/migrations.go` with the CREATE TABLE statements for all 3 tables matching the spec's column definitions exactly. Include `IF NOT EXISTS` for idempotency.

Create `internal/infrastructure/database/mysql.go` with `NewConnection` that opens a MySQL connection using `go-sql-driver/mysql`.

Write tests in `internal/infrastructure/database/production_repo_test.go` etc. that use `sqltest` or a test MySQL (skip if `MYSQL_TEST_DSN` env is not set). For each repo, test CRUD operations: insert, get, update, list.

The key test patterns:
- Insert a production, get it back, verify all fields
- Upsert same produccion_id, verify no duplicate (UNIQUE constraint)
- Insert scene, update status, verify change
- SetBloqueado then Desbloquear, verify bloqueado=0
- GetPreviousValidScene returns the most recent scene before a given date with passes_quality=1

- [ ] **Step 2: Implement repos**

Each repo takes `*sql.DB` in its constructor. Use prepared statements. All methods take `context.Context` as first parameter.

- [ ] **Step 3: Add go-sql-driver dependency**

```bash
go get github.com/go-sql-driver/mysql
```

- [ ] **Step 4: Run tests (skip if no test DB available)**

```bash
MYSQL_TEST_DSN="user:pass@tcp(localhost:3306)/agro_test" go test ./internal/infrastructure/database/ -v
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: MySQL migrations and repositories for producciones, escenas, archivos"
```

---
