# Review Package — Task 3

## Commits
3a386cb feat: MySQL migrations and repositories for producciones, escenas, archivos

## Stat
 go.mod                                             |   6 +-
 go.sum                                             |   4 +
 internal/infrastructure/database/file_repo.go      | 113 +++++++++
 internal/infrastructure/database/file_repo_test.go | 107 ++++++++
 internal/infrastructure/database/migrations.go     |  94 +++++++
 internal/infrastructure/database/mysql.go          |  33 +++
 .../infrastructure/database/production_repo.go     | 270 +++++++++++++++++++++
 .../database/production_repo_test.go               | 188 ++++++++++++++
 internal/infrastructure/database/scene_repo.go     | 229 +++++++++++++++++
 .../infrastructure/database/scene_repo_test.go     | 209 ++++++++++++++++
 internal/infrastructure/database/testdb_test.go    |  42 ++++
 11 files changed, 1294 insertions(+), 1 deletion(-)

## Diff
diff --git a/go.mod b/go.mod
index c590812..2017b41 100644
--- a/go.mod
+++ b/go.mod
@@ -1,5 +1,9 @@
 module agro-sentinel-worker
 
 go 1.26.5
 
-require gopkg.in/yaml.v3 v3.0.1 // indirect
+require (
+	filippo.io/edwards25519 v1.2.0 // indirect
+	github.com/go-sql-driver/mysql v1.10.1 // indirect
+	gopkg.in/yaml.v3 v3.0.1 // indirect
+)
diff --git a/go.sum b/go.sum
index 4bc0337..6eedd30 100644
--- a/go.sum
+++ b/go.sum
@@ -1,3 +1,7 @@
+filippo.io/edwards25519 v1.2.0 h1:crnVqOiS4jqYleHd9vaKZ+HKtHfllngJIiOpNpoJsjo=
+filippo.io/edwards25519 v1.2.0/go.mod h1:xzAOLCNug/yB62zG1bQ8uziwrIqIuxhctzJT18Q77mc=
+github.com/go-sql-driver/mysql v1.10.1 h1:arlSnNLq6a5yxGxV7qg9lF4j0C+KwD6NbQyKr9QL6ME=
+github.com/go-sql-driver/mysql v1.10.1/go.mod h1:M+cqaI7+xxXGG9swrdeUIoPG3Y3KCkF0pZej+SK+nWk=
 gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
 gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
 gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
diff --git a/internal/infrastructure/database/file_repo.go b/internal/infrastructure/database/file_repo.go
new file mode 100644
index 0000000..535f76d
--- /dev/null
+++ b/internal/infrastructure/database/file_repo.go
@@ -0,0 +1,113 @@
+package database
+
+import (
+	"context"
+	"database/sql"
+	"errors"
+	"fmt"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// FileRepo provides CRUD access to s3_monitoring_escena_archivos.
+type FileRepo struct {
+	db *sql.DB
+}
+
+// NewFileRepo creates a new FileRepo.
+func NewFileRepo(db *sql.DB) *FileRepo {
+	return &FileRepo{db: db}
+}
+
+// Create inserts a new scene file record.
+func (r *FileRepo) Create(ctx context.Context, f *domain.SceneFile) error {
+	createdAt := f.CreatedAt
+	if createdAt.IsZero() {
+		createdAt = time.Now().UTC()
+	}
+
+	const q = `
+INSERT INTO s3_monitoring_escena_archivos (
+	escena_id, file_type, file_name, s3_key, s3_bucket,
+	file_size_bytes, resolution_m, width_px, height_px, created_at
+) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
+`
+
+	res, err := r.db.ExecContext(ctx, q,
+		f.EscenaID, f.FileType, f.FileName, f.S3Key, f.S3Bucket,
+		f.FileSizeBytes, f.ResolutionM, f.WidthPx, f.HeightPx, createdAt,
+	)
+	if err != nil {
+		return fmt.Errorf("creating scene file: %w", err)
+	}
+
+	if id, err := res.LastInsertId(); err == nil {
+		f.ID = id
+	}
+
+	return nil
+}
+
+const fileSelectCols = `
+id, escena_id, file_type, file_name, s3_key, s3_bucket,
+file_size_bytes, resolution_m, width_px, height_px, created_at
+`
+
+// ListByEscena returns all files for a given scene.
+func (r *FileRepo) ListByEscena(ctx context.Context, escenaID int64) ([]*domain.SceneFile, error) {
+	q := "SELECT " + fileSelectCols + " FROM s3_monitoring_escena_archivos WHERE escena_id = ?"
+
+	rows, err := r.db.QueryContext(ctx, q, escenaID)
+	if err != nil {
+		return nil, fmt.Errorf("listing files by escena: %w", err)
+	}
+	defer rows.Close()
+
+	var results []*domain.SceneFile
+	for rows.Next() {
+		f, err := scanSceneFile(rows)
+		if err != nil {
+			return nil, fmt.Errorf("scanning scene file row: %w", err)
+		}
+		results = append(results, f)
+	}
+	if err := rows.Err(); err != nil {
+		return nil, fmt.Errorf("iterating scene file rows: %w", err)
+	}
+
+	return results, nil
+}
+
+// GetByType returns the file of a given type for a scene, if present.
+func (r *FileRepo) GetByType(ctx context.Context, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error) {
+	q := "SELECT " + fileSelectCols + " FROM s3_monitoring_escena_archivos WHERE escena_id = ? AND file_type = ? LIMIT 1"
+
+	row := r.db.QueryRowContext(ctx, q, escenaID, fileType)
+	f, err := scanSceneFile(row)
+	if err != nil {
+		if errors.Is(err, sql.ErrNoRows) {
+			return nil, nil
+		}
+		return nil, fmt.Errorf("getting file by type: %w", err)
+	}
+
+	return f, nil
+}
+
+func scanSceneFile(row rowScanner) (*domain.SceneFile, error) {
+	var f domain.SceneFile
+	var fileType string
+
+	err := row.Scan(
+		&f.ID, &f.EscenaID, &fileType, &f.FileName, &f.S3Key, &f.S3Bucket,
+		&f.FileSizeBytes, &f.ResolutionM, &f.WidthPx, &f.HeightPx, &f.CreatedAt,
+	)
+	if err != nil {
+		return nil, err
+	}
+
+	f.FileType = domain.FileType(fileType)
+
+	return &f, nil
+}
diff --git a/internal/infrastructure/database/file_repo_test.go b/internal/infrastructure/database/file_repo_test.go
new file mode 100644
index 0000000..c28f305
--- /dev/null
+++ b/internal/infrastructure/database/file_repo_test.go
@@ -0,0 +1,107 @@
+package database
+
+import (
+	"context"
+	"testing"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+func setupScene(t *testing.T, ctx context.Context, prodRepo *ProductionRepo, sceneRepo *SceneRepo) int64 {
+	t.Helper()
+	produccionID := setupProduction(t, ctx, prodRepo)
+	s := &domain.Scene{
+		ProduccionID: produccionID,
+		SceneID:      "S2A_FILE_TEST",
+		SceneDate:    time.Now().UTC(),
+		Status:       domain.StatusPending,
+	}
+	if err := sceneRepo.Upsert(ctx, s); err != nil {
+		t.Fatalf("setting up scene: %v", err)
+	}
+	got, err := sceneRepo.GetByProduccionAndSceneID(ctx, produccionID, s.SceneID)
+	if err != nil {
+		t.Fatalf("fetching scene: %v", err)
+	}
+	return got.ID
+}
+
+func TestFileRepo_CreateAndList(t *testing.T) {
+	db := testDB(t)
+	prodRepo := NewProductionRepo(db)
+	sceneRepo := NewSceneRepo(db)
+	repo := NewFileRepo(db)
+	ctx := context.Background()
+
+	escenaID := setupScene(t, ctx, prodRepo, sceneRepo)
+
+	f := &domain.SceneFile{
+		EscenaID:      escenaID,
+		FileType:      domain.FileMultiband,
+		FileName:      "multiband.tif",
+		S3Key:         "path/to/multiband.tif",
+		S3Bucket:      "test-bucket",
+		FileSizeBytes: 12345,
+		ResolutionM:   10,
+		WidthPx:       1024,
+		HeightPx:      1024,
+	}
+
+	if err := repo.Create(ctx, f); err != nil {
+		t.Fatalf("Create: %v", err)
+	}
+	if f.ID == 0 {
+		t.Error("expected file ID to be set after Create")
+	}
+
+	list, err := repo.ListByEscena(ctx, escenaID)
+	if err != nil {
+		t.Fatalf("ListByEscena: %v", err)
+	}
+	if len(list) != 1 {
+		t.Fatalf("expected 1 file, got %d", len(list))
+	}
+	if list[0].FileName != "multiband.tif" {
+		t.Errorf("unexpected file name: %s", list[0].FileName)
+	}
+}
+
+func TestFileRepo_GetByType(t *testing.T) {
+	db := testDB(t)
+	prodRepo := NewProductionRepo(db)
+	sceneRepo := NewSceneRepo(db)
+	repo := NewFileRepo(db)
+	ctx := context.Background()
+
+	escenaID := setupScene(t, ctx, prodRepo, sceneRepo)
+
+	multiband := &domain.SceneFile{EscenaID: escenaID, FileType: domain.FileMultiband, FileName: "m.tif", S3Key: "k1", S3Bucket: "b"}
+	ndvi := &domain.SceneFile{EscenaID: escenaID, FileType: domain.FileNDVI, FileName: "ndvi.png", S3Key: "k2", S3Bucket: "b"}
+
+	if err := repo.Create(ctx, multiband); err != nil {
+		t.Fatalf("Create multiband: %v", err)
+	}
+	if err := repo.Create(ctx, ndvi); err != nil {
+		t.Fatalf("Create ndvi: %v", err)
+	}
+
+	got, err := repo.GetByType(ctx, escenaID, domain.FileNDVI)
+	if err != nil {
+		t.Fatalf("GetByType: %v", err)
+	}
+	if got == nil {
+		t.Fatal("expected file, got nil")
+	}
+	if got.FileName != "ndvi.png" {
+		t.Errorf("unexpected file: %+v", got)
+	}
+
+	missing, err := repo.GetByType(ctx, escenaID, domain.FileSWIR)
+	if err != nil {
+		t.Fatalf("GetByType missing: %v", err)
+	}
+	if missing != nil {
+		t.Errorf("expected nil for missing type, got %+v", missing)
+	}
+}
diff --git a/internal/infrastructure/database/migrations.go b/internal/infrastructure/database/migrations.go
new file mode 100644
index 0000000..7967324
--- /dev/null
+++ b/internal/infrastructure/database/migrations.go
@@ -0,0 +1,94 @@
+package database
+
+import (
+	"database/sql"
+	"fmt"
+)
+
+const createProduccionesTable = `
+CREATE TABLE IF NOT EXISTS s3_monitoring_producciones (
+	id BIGINT AUTO_INCREMENT PRIMARY KEY,
+	produccion_id BIGINT NOT NULL,
+	cultivo VARCHAR(100),
+	ciclo VARCHAR(50),
+	bbox_minx DECIMAL(10,6) NULL,
+	bbox_miny DECIMAL(10,6) NULL,
+	bbox_maxx DECIMAL(10,6) NULL,
+	bbox_maxy DECIMAL(10,6) NULL,
+	monitoring TINYINT(1) DEFAULT 1,
+	monitoring_motivo VARCHAR(100),
+	bloqueado TINYINT(1) DEFAULT 0,
+	bloqueado_motivo VARCHAR(255),
+	bloqueado_at DATETIME,
+	desbloqueado_por VARCHAR(100),
+	target_resolution INT DEFAULT 10,
+	cloud_cover_max DECIMAL(5,2) DEFAULT 23.00,
+	fecha_plantacion DATE,
+	dias_produccion INT,
+	fecha_fin_monitoreo DATE,
+	total_escenas INT DEFAULT 0,
+	total_escenas_validas INT DEFAULT 0,
+	last_sync_at DATETIME,
+	created_at DATETIME,
+	updated_at DATETIME,
+	UNIQUE KEY uq_produccion_id (produccion_id)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
+`
+
+const createEscenasTable = `
+CREATE TABLE IF NOT EXISTS s3_monitoring_escenas (
+	id BIGINT AUTO_INCREMENT PRIMARY KEY,
+	produccion_id BIGINT NOT NULL,
+	scene_id VARCHAR(100) NOT NULL,
+	scene_date DATE,
+	cloud_cover_scene DECIMAL(5,2),
+	cloud_cover_bbox DECIMAL(5,2),
+	passes_quality TINYINT(1) DEFAULT 0,
+	has_multiband TINYINT(1) DEFAULT 0,
+	has_params TINYINT(1) DEFAULT 0,
+	has_rgb TINYINT(1) DEFAULT 0,
+	has_analisis TINYINT(1) DEFAULT 0,
+	status VARCHAR(20) DEFAULT 'PENDING',
+	error_type VARCHAR(50),
+	error_message TEXT,
+	retry_count INT DEFAULT 0,
+	processed_at DATETIME,
+	created_at DATETIME,
+	updated_at DATETIME,
+	UNIQUE KEY uq_produccion_scene (produccion_id, scene_id)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
+`
+
+const createArchivosTable = `
+CREATE TABLE IF NOT EXISTS s3_monitoring_escena_archivos (
+	id BIGINT AUTO_INCREMENT PRIMARY KEY,
+	escena_id BIGINT NOT NULL,
+	file_type VARCHAR(50),
+	file_name VARCHAR(255),
+	s3_key VARCHAR(500),
+	s3_bucket VARCHAR(100),
+	file_size_bytes BIGINT,
+	resolution_m INT,
+	width_px INT,
+	height_px INT,
+	created_at DATETIME,
+	KEY idx_escena_id (escena_id)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
+`
+
+// RunMigrations creates the required tables if they do not already exist.
+func RunMigrations(db *sql.DB) error {
+	statements := []string{
+		createProduccionesTable,
+		createEscenasTable,
+		createArchivosTable,
+	}
+
+	for _, stmt := range statements {
+		if _, err := db.Exec(stmt); err != nil {
+			return fmt.Errorf("running migration: %w", err)
+		}
+	}
+
+	return nil
+}
diff --git a/internal/infrastructure/database/mysql.go b/internal/infrastructure/database/mysql.go
new file mode 100644
index 0000000..1e6ea78
--- /dev/null
+++ b/internal/infrastructure/database/mysql.go
@@ -0,0 +1,33 @@
+package database
+
+import (
+	"database/sql"
+	"fmt"
+	"time"
+
+	_ "github.com/go-sql-driver/mysql"
+
+	"agro-sentinel-worker/internal/config"
+)
+
+// NewConnection opens a MySQL connection pool using the given configuration.
+func NewConnection(cfg config.MySQLConfig) (*sql.DB, error) {
+	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
+		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
+
+	db, err := sql.Open("mysql", dsn)
+	if err != nil {
+		return nil, fmt.Errorf("opening mysql connection: %w", err)
+	}
+
+	db.SetMaxOpenConns(25)
+	db.SetMaxIdleConns(10)
+	db.SetConnMaxLifetime(5 * time.Minute)
+
+	if err := db.Ping(); err != nil {
+		db.Close()
+		return nil, fmt.Errorf("pinging mysql: %w", err)
+	}
+
+	return db, nil
+}
diff --git a/internal/infrastructure/database/production_repo.go b/internal/infrastructure/database/production_repo.go
new file mode 100644
index 0000000..e26e078
--- /dev/null
+++ b/internal/infrastructure/database/production_repo.go
@@ -0,0 +1,270 @@
+package database
+
+import (
+	"context"
+	"database/sql"
+	"errors"
+	"fmt"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// ProductionRepo provides CRUD access to s3_monitoring_producciones.
+type ProductionRepo struct {
+	db *sql.DB
+}
+
+// NewProductionRepo creates a new ProductionRepo.
+func NewProductionRepo(db *sql.DB) *ProductionRepo {
+	return &ProductionRepo{db: db}
+}
+
+// Upsert inserts a production or updates it if produccion_id already exists.
+func (r *ProductionRepo) Upsert(ctx context.Context, p *domain.Production) error {
+	var minx, miny, maxx, maxy any
+	if p.BBox != nil {
+		minx, miny, maxx, maxy = p.BBox.MinX, p.BBox.MinY, p.BBox.MaxX, p.BBox.MaxY
+	}
+
+	now := time.Now().UTC()
+
+	const q = `
+INSERT INTO s3_monitoring_producciones (
+	produccion_id, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
+	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
+	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
+	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
+	last_sync_at, created_at, updated_at
+) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
+ON DUPLICATE KEY UPDATE
+	cultivo = VALUES(cultivo),
+	ciclo = VALUES(ciclo),
+	bbox_minx = VALUES(bbox_minx),
+	bbox_miny = VALUES(bbox_miny),
+	bbox_maxx = VALUES(bbox_maxx),
+	bbox_maxy = VALUES(bbox_maxy),
+	monitoring = VALUES(monitoring),
+	monitoring_motivo = VALUES(monitoring_motivo),
+	bloqueado = VALUES(bloqueado),
+	bloqueado_motivo = VALUES(bloqueado_motivo),
+	bloqueado_at = VALUES(bloqueado_at),
+	desbloqueado_por = VALUES(desbloqueado_por),
+	target_resolution = VALUES(target_resolution),
+	cloud_cover_max = VALUES(cloud_cover_max),
+	fecha_plantacion = VALUES(fecha_plantacion),
+	dias_produccion = VALUES(dias_produccion),
+	fecha_fin_monitoreo = VALUES(fecha_fin_monitoreo),
+	total_escenas = VALUES(total_escenas),
+	total_escenas_validas = VALUES(total_escenas_validas),
+	last_sync_at = VALUES(last_sync_at),
+	updated_at = VALUES(updated_at)
+`
+
+	createdAt := p.CreatedAt
+	if createdAt.IsZero() {
+		createdAt = now
+	}
+
+	_, err := r.db.ExecContext(ctx, q,
+		p.ProduccionID, p.Cultivo, p.Ciclo, minx, miny, maxx, maxy,
+		p.Monitoring, nullString(p.MonitoringMotivo), p.Bloqueado, nullString(p.BloqueadoMotivo), p.BloqueadoAt,
+		nullString(p.DesbloqueadoPor), p.TargetResolution, p.CloudCoverMax, p.FechaPlantacion,
+		p.DiasProduccion, p.FechaFinMonitoreo, p.TotalEscenas, p.TotalEscenasValidas,
+		p.LastSyncAt, createdAt, now,
+	)
+	if err != nil {
+		return fmt.Errorf("upserting production: %w", err)
+	}
+
+	return nil
+}
+
+// GetByProduccionID fetches a production by its external produccion_id.
+func (r *ProductionRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
+	const q = `
+SELECT id, produccion_id, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
+	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
+	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
+	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
+	last_sync_at, created_at, updated_at
+FROM s3_monitoring_producciones
+WHERE produccion_id = ?
+`
+
+	row := r.db.QueryRowContext(ctx, q, produccionID)
+	p, err := scanProduction(row)
+	if err != nil {
+		if errors.Is(err, sql.ErrNoRows) {
+			return nil, nil
+		}
+		return nil, fmt.Errorf("getting production by produccion_id: %w", err)
+	}
+
+	return p, nil
+}
+
+// ListActive returns all productions with monitoring=1 and bloqueado=0.
+func (r *ProductionRepo) ListActive(ctx context.Context) ([]*domain.Production, error) {
+	const q = `
+SELECT id, produccion_id, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
+	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
+	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
+	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
+	last_sync_at, created_at, updated_at
+FROM s3_monitoring_producciones
+WHERE monitoring = 1 AND bloqueado = 0
+`
+
+	rows, err := r.db.QueryContext(ctx, q)
+	if err != nil {
+		return nil, fmt.Errorf("listing active productions: %w", err)
+	}
+	defer rows.Close()
+
+	var results []*domain.Production
+	for rows.Next() {
+		p, err := scanProduction(rows)
+		if err != nil {
+			return nil, fmt.Errorf("scanning production row: %w", err)
+		}
+		results = append(results, p)
+	}
+	if err := rows.Err(); err != nil {
+		return nil, fmt.Errorf("iterating production rows: %w", err)
+	}
+
+	return results, nil
+}
+
+// UpdateMonitoring updates the monitoring flag and motivo for a production.
+func (r *ProductionRepo) UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool, motivo string) error {
+	const q = `
+UPDATE s3_monitoring_producciones
+SET monitoring = ?, monitoring_motivo = ?, updated_at = ?
+WHERE produccion_id = ?
+`
+	_, err := r.db.ExecContext(ctx, q, monitoring, nullString(motivo), time.Now().UTC(), produccionID)
+	if err != nil {
+		return fmt.Errorf("updating monitoring: %w", err)
+	}
+	return nil
+}
+
+// UpdateBBox updates the bounding box for a production.
+func (r *ProductionRepo) UpdateBBox(ctx context.Context, produccionID int64, bbox domain.BBox) error {
+	const q = `
+UPDATE s3_monitoring_producciones
+SET bbox_minx = ?, bbox_miny = ?, bbox_maxx = ?, bbox_maxy = ?, updated_at = ?
+WHERE produccion_id = ?
+`
+	_, err := r.db.ExecContext(ctx, q, bbox.MinX, bbox.MinY, bbox.MaxX, bbox.MaxY, time.Now().UTC(), produccionID)
+	if err != nil {
+		return fmt.Errorf("updating bbox: %w", err)
+	}
+	return nil
+}
+
+// SetBloqueado marks a production as blocked with the given motivo.
+func (r *ProductionRepo) SetBloqueado(ctx context.Context, produccionID int64, motivo string) error {
+	now := time.Now().UTC()
+	const q = `
+UPDATE s3_monitoring_producciones
+SET bloqueado = 1, bloqueado_motivo = ?, bloqueado_at = ?, updated_at = ?
+WHERE produccion_id = ?
+`
+	_, err := r.db.ExecContext(ctx, q, nullString(motivo), now, now, produccionID)
+	if err != nil {
+		return fmt.Errorf("setting bloqueado: %w", err)
+	}
+	return nil
+}
+
+// Desbloquear unblocks a production, recording who unblocked it.
+func (r *ProductionRepo) Desbloquear(ctx context.Context, produccionID int64, usuario string) error {
+	const q = `
+UPDATE s3_monitoring_producciones
+SET bloqueado = 0, bloqueado_motivo = NULL, bloqueado_at = NULL, desbloqueado_por = ?, updated_at = ?
+WHERE produccion_id = ?
+`
+	_, err := r.db.ExecContext(ctx, q, nullString(usuario), time.Now().UTC(), produccionID)
+	if err != nil {
+		return fmt.Errorf("desbloqueando production: %w", err)
+	}
+	return nil
+}
+
+// IncrementEscenas increments total_escenas, and total_escenas_validas if valid.
+func (r *ProductionRepo) IncrementEscenas(ctx context.Context, produccionID int64, valid bool) error {
+	q := `
+UPDATE s3_monitoring_producciones
+SET total_escenas = total_escenas + 1, updated_at = ?
+WHERE produccion_id = ?
+`
+	if valid {
+		q = `
+UPDATE s3_monitoring_producciones
+SET total_escenas = total_escenas + 1, total_escenas_validas = total_escenas_validas + 1, updated_at = ?
+WHERE produccion_id = ?
+`
+	}
+
+	_, err := r.db.ExecContext(ctx, q, time.Now().UTC(), produccionID)
+	if err != nil {
+		return fmt.Errorf("incrementing escenas: %w", err)
+	}
+	return nil
+}
+
+// rowScanner abstracts *sql.Row / *sql.Rows for shared scan logic.
+type rowScanner interface {
+	Scan(dest ...any) error
+}
+
+func scanProduction(row rowScanner) (*domain.Production, error) {
+	var p domain.Production
+	var minx, miny, maxx, maxy sql.NullFloat64
+	var monitoringMotivo, bloqueadoMotivo, desbloqueadoPor sql.NullString
+	var bloqueadoAt, fechaPlantacion, fechaFinMonitoreo, lastSyncAt sql.NullTime
+	var diasProduccion sql.NullInt64
+
+	err := row.Scan(
+		&p.ID, &p.ProduccionID, &p.Cultivo, &p.Ciclo, &minx, &miny, &maxx, &maxy,
+		&p.Monitoring, &monitoringMotivo, &p.Bloqueado, &bloqueadoMotivo, &bloqueadoAt,
+		&desbloqueadoPor, &p.TargetResolution, &p.CloudCoverMax, &fechaPlantacion,
+		&diasProduccion, &fechaFinMonitoreo, &p.TotalEscenas, &p.TotalEscenasValidas,
+		&lastSyncAt, &p.CreatedAt, &p.UpdatedAt,
+	)
+	if err != nil {
+		return nil, err
+	}
+
+	if minx.Valid && miny.Valid && maxx.Valid && maxy.Valid {
+		p.BBox = &domain.BBox{MinX: minx.Float64, MinY: miny.Float64, MaxX: maxx.Float64, MaxY: maxy.Float64}
+	}
+	p.MonitoringMotivo = monitoringMotivo.String
+	p.BloqueadoMotivo = bloqueadoMotivo.String
+	p.DesbloqueadoPor = desbloqueadoPor.String
+	if bloqueadoAt.Valid {
+		p.BloqueadoAt = &bloqueadoAt.Time
+	}
+	if fechaPlantacion.Valid {
+		p.FechaPlantacion = &fechaPlantacion.Time
+	}
+	if fechaFinMonitoreo.Valid {
+		p.FechaFinMonitoreo = &fechaFinMonitoreo.Time
+	}
+	if lastSyncAt.Valid {
+		p.LastSyncAt = &lastSyncAt.Time
+	}
+	p.DiasProduccion = int(diasProduccion.Int64)
+
+	return &p, nil
+}
+
+func nullString(s string) any {
+	if s == "" {
+		return nil
+	}
+	return s
+}
diff --git a/internal/infrastructure/database/production_repo_test.go b/internal/infrastructure/database/production_repo_test.go
new file mode 100644
index 0000000..5998a1e
--- /dev/null
+++ b/internal/infrastructure/database/production_repo_test.go
@@ -0,0 +1,188 @@
+package database
+
+import (
+	"context"
+	"testing"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+func TestProductionRepo_UpsertAndGet(t *testing.T) {
+	db := testDB(t)
+	repo := NewProductionRepo(db)
+	ctx := context.Background()
+
+	produccionID := randomID()
+	p := &domain.Production{
+		ProduccionID:     produccionID,
+		Cultivo:          "Maiz",
+		Ciclo:            "2026-A",
+		BBox:             &domain.BBox{MinX: -60.1, MinY: -34.5, MaxX: -60.0, MaxY: -34.4},
+		Monitoring:       true,
+		TargetResolution: 10,
+		CloudCoverMax:    23.0,
+	}
+
+	if err := repo.Upsert(ctx, p); err != nil {
+		t.Fatalf("Upsert: %v", err)
+	}
+
+	got, err := repo.GetByProduccionID(ctx, produccionID)
+	if err != nil {
+		t.Fatalf("GetByProduccionID: %v", err)
+	}
+	if got == nil {
+		t.Fatal("expected production, got nil")
+	}
+	if got.Cultivo != "Maiz" || got.Ciclo != "2026-A" {
+		t.Errorf("unexpected fields: %+v", got)
+	}
+	if got.BBox == nil || got.BBox.MinX != -60.1 {
+		t.Errorf("unexpected bbox: %+v", got.BBox)
+	}
+	if !got.Monitoring {
+		t.Error("expected monitoring true")
+	}
+}
+
+func TestProductionRepo_UpsertNoDuplicate(t *testing.T) {
+	db := testDB(t)
+	repo := NewProductionRepo(db)
+	ctx := context.Background()
+
+	produccionID := randomID()
+	p := &domain.Production{ProduccionID: produccionID, Cultivo: "Soja", Monitoring: true}
+
+	if err := repo.Upsert(ctx, p); err != nil {
+		t.Fatalf("Upsert 1: %v", err)
+	}
+
+	p.Cultivo = "Soja actualizada"
+	if err := repo.Upsert(ctx, p); err != nil {
+		t.Fatalf("Upsert 2: %v", err)
+	}
+
+	var count int
+	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM s3_monitoring_producciones WHERE produccion_id = ?", produccionID).Scan(&count); err != nil {
+		t.Fatalf("counting rows: %v", err)
+	}
+	if count != 1 {
+		t.Fatalf("expected 1 row, got %d", count)
+	}
+
+	got, err := repo.GetByProduccionID(ctx, produccionID)
+	if err != nil {
+		t.Fatalf("GetByProduccionID: %v", err)
+	}
+	if got.Cultivo != "Soja actualizada" {
+		t.Errorf("expected updated cultivo, got %q", got.Cultivo)
+	}
+}
+
+func TestProductionRepo_SetBloqueadoAndDesbloquear(t *testing.T) {
+	db := testDB(t)
+	repo := NewProductionRepo(db)
+	ctx := context.Background()
+
+	produccionID := randomID()
+	p := &domain.Production{ProduccionID: produccionID, Cultivo: "Trigo", Monitoring: true}
+	if err := repo.Upsert(ctx, p); err != nil {
+		t.Fatalf("Upsert: %v", err)
+	}
+
+	if err := repo.SetBloqueado(ctx, produccionID, "manual hold"); err != nil {
+		t.Fatalf("SetBloqueado: %v", err)
+	}
+
+	got, err := repo.GetByProduccionID(ctx, produccionID)
+	if err != nil {
+		t.Fatalf("GetByProduccionID: %v", err)
+	}
+	if !got.Bloqueado {
+		t.Fatal("expected bloqueado true")
+	}
+	if got.BloqueadoMotivo != "manual hold" {
+		t.Errorf("unexpected motivo: %q", got.BloqueadoMotivo)
+	}
+
+	if err := repo.Desbloquear(ctx, produccionID, "operator1"); err != nil {
+		t.Fatalf("Desbloquear: %v", err)
+	}
+
+	got, err = repo.GetByProduccionID(ctx, produccionID)
+	if err != nil {
+		t.Fatalf("GetByProduccionID: %v", err)
+	}
+	if got.Bloqueado {
+		t.Fatal("expected bloqueado false after desbloquear")
+	}
+	if got.DesbloqueadoPor != "operator1" {
+		t.Errorf("unexpected desbloqueado_por: %q", got.DesbloqueadoPor)
+	}
+}
+
+func TestProductionRepo_ListActive(t *testing.T) {
+	db := testDB(t)
+	repo := NewProductionRepo(db)
+	ctx := context.Background()
+
+	activeID := randomID()
+	inactiveID := randomID()
+
+	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: activeID, Cultivo: "Activo", Monitoring: true}); err != nil {
+		t.Fatalf("Upsert active: %v", err)
+	}
+	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: inactiveID, Cultivo: "Inactivo", Monitoring: false}); err != nil {
+		t.Fatalf("Upsert inactive: %v", err)
+	}
+
+	list, err := repo.ListActive(ctx)
+	if err != nil {
+		t.Fatalf("ListActive: %v", err)
+	}
+
+	var foundActive, foundInactive bool
+	for _, p := range list {
+		if p.ProduccionID == activeID {
+			foundActive = true
+		}
+		if p.ProduccionID == inactiveID {
+			foundInactive = true
+		}
+	}
+	if !foundActive {
+		t.Error("expected active production in list")
+	}
+	if foundInactive {
+		t.Error("did not expect inactive production in list")
+	}
+}
+
+func TestProductionRepo_IncrementEscenas(t *testing.T) {
+	db := testDB(t)
+	repo := NewProductionRepo(db)
+	ctx := context.Background()
+
+	produccionID := randomID()
+	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Monitoring: true}); err != nil {
+		t.Fatalf("Upsert: %v", err)
+	}
+
+	if err := repo.IncrementEscenas(ctx, produccionID, true); err != nil {
+		t.Fatalf("IncrementEscenas valid: %v", err)
+	}
+	if err := repo.IncrementEscenas(ctx, produccionID, false); err != nil {
+		t.Fatalf("IncrementEscenas invalid: %v", err)
+	}
+
+	got, err := repo.GetByProduccionID(ctx, produccionID)
+	if err != nil {
+		t.Fatalf("GetByProduccionID: %v", err)
+	}
+	if got.TotalEscenas != 2 {
+		t.Errorf("expected total_escenas=2, got %d", got.TotalEscenas)
+	}
+	if got.TotalEscenasValidas != 1 {
+		t.Errorf("expected total_escenas_validas=1, got %d", got.TotalEscenasValidas)
+	}
+}
diff --git a/internal/infrastructure/database/scene_repo.go b/internal/infrastructure/database/scene_repo.go
new file mode 100644
index 0000000..5a4bb07
--- /dev/null
+++ b/internal/infrastructure/database/scene_repo.go
@@ -0,0 +1,229 @@
+package database
+
+import (
+	"context"
+	"database/sql"
+	"errors"
+	"fmt"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// SceneRepo provides CRUD access to s3_monitoring_escenas.
+type SceneRepo struct {
+	db *sql.DB
+}
+
+// NewSceneRepo creates a new SceneRepo.
+func NewSceneRepo(db *sql.DB) *SceneRepo {
+	return &SceneRepo{db: db}
+}
+
+// Upsert inserts a scene, or updates it if (produccion_id, scene_id) already exists.
+func (r *SceneRepo) Upsert(ctx context.Context, s *domain.Scene) error {
+	now := time.Now().UTC()
+	createdAt := s.CreatedAt
+	if createdAt.IsZero() {
+		createdAt = now
+	}
+
+	var cloudCoverBBox any
+	if s.CloudCoverBBox != nil {
+		cloudCoverBBox = *s.CloudCoverBBox
+	}
+
+	status := s.Status
+	if status == "" {
+		status = domain.StatusPending
+	}
+
+	const q = `
+INSERT INTO s3_monitoring_escenas (
+	produccion_id, scene_id, scene_date, cloud_cover_scene, cloud_cover_bbox,
+	passes_quality, has_multiband, has_params, has_rgb, has_analisis,
+	status, error_type, error_message, retry_count, processed_at,
+	created_at, updated_at
+) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
+ON DUPLICATE KEY UPDATE
+	scene_date = VALUES(scene_date),
+	cloud_cover_scene = VALUES(cloud_cover_scene),
+	cloud_cover_bbox = VALUES(cloud_cover_bbox),
+	passes_quality = VALUES(passes_quality),
+	has_multiband = VALUES(has_multiband),
+	has_params = VALUES(has_params),
+	has_rgb = VALUES(has_rgb),
+	has_analisis = VALUES(has_analisis),
+	status = VALUES(status),
+	error_type = VALUES(error_type),
+	error_message = VALUES(error_message),
+	retry_count = VALUES(retry_count),
+	processed_at = VALUES(processed_at),
+	updated_at = VALUES(updated_at)
+`
+
+	_, err := r.db.ExecContext(ctx, q,
+		s.ProduccionID, s.SceneID, s.SceneDate, s.CloudCoverScene, cloudCoverBBox,
+		s.PassesQuality, s.HasMultiband, s.HasParams, s.HasRGB, s.HasAnalisis,
+		status, nullString(s.ErrorType), nullString(s.ErrorMessage), s.RetryCount, s.ProcessedAt,
+		createdAt, now,
+	)
+	if err != nil {
+		return fmt.Errorf("upserting scene: %w", err)
+	}
+
+	return nil
+}
+
+const sceneSelectCols = `
+id, produccion_id, scene_id, scene_date, cloud_cover_scene, cloud_cover_bbox,
+passes_quality, has_multiband, has_params, has_rgb, has_analisis,
+status, error_type, error_message, retry_count, processed_at,
+created_at, updated_at
+`
+
+// GetByID fetches a scene by its primary key.
+func (r *SceneRepo) GetByID(ctx context.Context, id int64) (*domain.Scene, error) {
+	q := "SELECT " + sceneSelectCols + " FROM s3_monitoring_escenas WHERE id = ?"
+
+	row := r.db.QueryRowContext(ctx, q, id)
+	s, err := scanScene(row)
+	if err != nil {
+		if errors.Is(err, sql.ErrNoRows) {
+			return nil, nil
+		}
+		return nil, fmt.Errorf("getting scene by id: %w", err)
+	}
+
+	return s, nil
+}
+
+// GetByProduccionAndSceneID fetches a scene by its production and external scene id.
+func (r *SceneRepo) GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error) {
+	q := "SELECT " + sceneSelectCols + " FROM s3_monitoring_escenas WHERE produccion_id = ? AND scene_id = ?"
+
+	row := r.db.QueryRowContext(ctx, q, produccionID, sceneID)
+	s, err := scanScene(row)
+	if err != nil {
+		if errors.Is(err, sql.ErrNoRows) {
+			return nil, nil
+		}
+		return nil, fmt.Errorf("getting scene by produccion and scene_id: %w", err)
+	}
+
+	return s, nil
+}
+
+// ListByProduccion returns all scenes for a production, ordered by scene_date descending.
+func (r *SceneRepo) ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.Scene, error) {
+	q := "SELECT " + sceneSelectCols + " FROM s3_monitoring_escenas WHERE produccion_id = ? ORDER BY scene_date DESC"
+
+	rows, err := r.db.QueryContext(ctx, q, produccionID)
+	if err != nil {
+		return nil, fmt.Errorf("listing scenes by produccion: %w", err)
+	}
+	defer rows.Close()
+
+	var results []*domain.Scene
+	for rows.Next() {
+		s, err := scanScene(rows)
+		if err != nil {
+			return nil, fmt.Errorf("scanning scene row: %w", err)
+		}
+		results = append(results, s)
+	}
+	if err := rows.Err(); err != nil {
+		return nil, fmt.Errorf("iterating scene rows: %w", err)
+	}
+
+	return results, nil
+}
+
+// UpdateStatus updates the status field of a scene.
+func (r *SceneRepo) UpdateStatus(ctx context.Context, id int64, status domain.JobStatus) error {
+	const q = `UPDATE s3_monitoring_escenas SET status = ?, updated_at = ? WHERE id = ?`
+	_, err := r.db.ExecContext(ctx, q, status, time.Now().UTC(), id)
+	if err != nil {
+		return fmt.Errorf("updating scene status: %w", err)
+	}
+	return nil
+}
+
+// SetError marks a scene as failed with the given error type and message, incrementing retry_count.
+func (r *SceneRepo) SetError(ctx context.Context, id int64, errType string, errMsg string) error {
+	const q = `
+UPDATE s3_monitoring_escenas
+SET status = ?, error_type = ?, error_message = ?, retry_count = retry_count + 1, updated_at = ?
+WHERE id = ?
+`
+	_, err := r.db.ExecContext(ctx, q, domain.StatusFailed, nullString(errType), nullString(errMsg), time.Now().UTC(), id)
+	if err != nil {
+		return fmt.Errorf("setting scene error: %w", err)
+	}
+	return nil
+}
+
+// SetCompleted marks a scene as completed, recording quality and cloud cover over the bbox.
+func (r *SceneRepo) SetCompleted(ctx context.Context, id int64, passesQuality bool, cloudCoverBBox float64) error {
+	now := time.Now().UTC()
+	const q = `
+UPDATE s3_monitoring_escenas
+SET status = ?, passes_quality = ?, cloud_cover_bbox = ?, processed_at = ?, updated_at = ?
+WHERE id = ?
+`
+	_, err := r.db.ExecContext(ctx, q, domain.StatusCompleted, passesQuality, cloudCoverBBox, now, now, id)
+	if err != nil {
+		return fmt.Errorf("setting scene completed: %w", err)
+	}
+	return nil
+}
+
+// GetPreviousValidScene returns the most recent scene before beforeDate that passes quality.
+func (r *SceneRepo) GetPreviousValidScene(ctx context.Context, produccionID int64, beforeDate time.Time) (*domain.Scene, error) {
+	q := "SELECT " + sceneSelectCols + ` FROM s3_monitoring_escenas
+WHERE produccion_id = ? AND scene_date < ? AND passes_quality = 1
+ORDER BY scene_date DESC
+LIMIT 1`
+
+	row := r.db.QueryRowContext(ctx, q, produccionID, beforeDate)
+	s, err := scanScene(row)
+	if err != nil {
+		if errors.Is(err, sql.ErrNoRows) {
+			return nil, nil
+		}
+		return nil, fmt.Errorf("getting previous valid scene: %w", err)
+	}
+
+	return s, nil
+}
+
+func scanScene(row rowScanner) (*domain.Scene, error) {
+	var s domain.Scene
+	var cloudCoverBBox sql.NullFloat64
+	var errorType, errorMessage sql.NullString
+	var processedAt sql.NullTime
+	var status string
+
+	err := row.Scan(
+		&s.ID, &s.ProduccionID, &s.SceneID, &s.SceneDate, &s.CloudCoverScene, &cloudCoverBBox,
+		&s.PassesQuality, &s.HasMultiband, &s.HasParams, &s.HasRGB, &s.HasAnalisis,
+		&status, &errorType, &errorMessage, &s.RetryCount, &processedAt,
+		&s.CreatedAt, &s.UpdatedAt,
+	)
+	if err != nil {
+		return nil, err
+	}
+
+	s.Status = domain.JobStatus(status)
+	if cloudCoverBBox.Valid {
+		v := cloudCoverBBox.Float64
+		s.CloudCoverBBox = &v
+	}
+	s.ErrorType = errorType.String
+	s.ErrorMessage = errorMessage.String
+	if processedAt.Valid {
+		s.ProcessedAt = &processedAt.Time
+	}
+
+	return &s, nil
+}
diff --git a/internal/infrastructure/database/scene_repo_test.go b/internal/infrastructure/database/scene_repo_test.go
new file mode 100644
index 0000000..fc05574
--- /dev/null
+++ b/internal/infrastructure/database/scene_repo_test.go
@@ -0,0 +1,209 @@
+package database
+
+import (
+	"context"
+	"testing"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+func setupProduction(t *testing.T, ctx context.Context, prodRepo *ProductionRepo) int64 {
+	t.Helper()
+	produccionID := randomID()
+	if err := prodRepo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Cultivo: "Maiz", Monitoring: true}); err != nil {
+		t.Fatalf("setting up production: %v", err)
+	}
+	return produccionID
+}
+
+func TestSceneRepo_UpsertAndGet(t *testing.T) {
+	db := testDB(t)
+	prodRepo := NewProductionRepo(db)
+	repo := NewSceneRepo(db)
+	ctx := context.Background()
+
+	produccionID := setupProduction(t, ctx, prodRepo)
+	sceneID := "S2A_TEST_SCENE_1"
+
+	s := &domain.Scene{
+		ProduccionID:    produccionID,
+		SceneID:         sceneID,
+		SceneDate:       time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
+		CloudCoverScene: 5.5,
+		Status:          domain.StatusPending,
+	}
+
+	if err := repo.Upsert(ctx, s); err != nil {
+		t.Fatalf("Upsert: %v", err)
+	}
+
+	got, err := repo.GetByProduccionAndSceneID(ctx, produccionID, sceneID)
+	if err != nil {
+		t.Fatalf("GetByProduccionAndSceneID: %v", err)
+	}
+	if got == nil {
+		t.Fatal("expected scene, got nil")
+	}
+	if got.Status != domain.StatusPending {
+		t.Errorf("expected status PENDING, got %s", got.Status)
+	}
+	if got.CloudCoverScene != 5.5 {
+		t.Errorf("unexpected cloud_cover_scene: %v", got.CloudCoverScene)
+	}
+
+	byID, err := repo.GetByID(ctx, got.ID)
+	if err != nil {
+		t.Fatalf("GetByID: %v", err)
+	}
+	if byID == nil || byID.SceneID != sceneID {
+		t.Errorf("unexpected GetByID result: %+v", byID)
+	}
+}
+
+func TestSceneRepo_UpsertNoDuplicate(t *testing.T) {
+	db := testDB(t)
+	prodRepo := NewProductionRepo(db)
+	repo := NewSceneRepo(db)
+	ctx := context.Background()
+
+	produccionID := setupProduction(t, ctx, prodRepo)
+	sceneID := "S2A_TEST_SCENE_DUP"
+
+	s := &domain.Scene{ProduccionID: produccionID, SceneID: sceneID, SceneDate: time.Now().UTC(), Status: domain.StatusPending}
+	if err := repo.Upsert(ctx, s); err != nil {
+		t.Fatalf("Upsert 1: %v", err)
+	}
+	s.CloudCoverScene = 10.0
+	if err := repo.Upsert(ctx, s); err != nil {
+		t.Fatalf("Upsert 2: %v", err)
+	}
+
+	var count int
+	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM s3_monitoring_escenas WHERE produccion_id = ? AND scene_id = ?", produccionID, sceneID).Scan(&count); err != nil {
+		t.Fatalf("counting rows: %v", err)
+	}
+	if count != 1 {
+		t.Fatalf("expected 1 row, got %d", count)
+	}
+}
+
+func TestSceneRepo_UpdateStatusAndError(t *testing.T) {
+	db := testDB(t)
+	prodRepo := NewProductionRepo(db)
+	repo := NewSceneRepo(db)
+	ctx := context.Background()
+
+	produccionID := setupProduction(t, ctx, prodRepo)
+	s := &domain.Scene{ProduccionID: produccionID, SceneID: "S2A_STATUS_TEST", SceneDate: time.Now().UTC(), Status: domain.StatusPending}
+	if err := repo.Upsert(ctx, s); err != nil {
+		t.Fatalf("Upsert: %v", err)
+	}
+
+	got, err := repo.GetByProduccionAndSceneID(ctx, produccionID, s.SceneID)
+	if err != nil {
+		t.Fatalf("get: %v", err)
+	}
+
+	if err := repo.UpdateStatus(ctx, got.ID, domain.StatusProcessing); err != nil {
+		t.Fatalf("UpdateStatus: %v", err)
+	}
+	got, err = repo.GetByID(ctx, got.ID)
+	if err != nil {
+		t.Fatalf("GetByID: %v", err)
+	}
+	if got.Status != domain.StatusProcessing {
+		t.Errorf("expected PROCESSING, got %s", got.Status)
+	}
+
+	if err := repo.SetError(ctx, got.ID, "DOWNLOAD_ERROR", "timeout"); err != nil {
+		t.Fatalf("SetError: %v", err)
+	}
+	got, err = repo.GetByID(ctx, got.ID)
+	if err != nil {
+		t.Fatalf("GetByID: %v", err)
+	}
+	if got.Status != domain.StatusFailed {
+		t.Errorf("expected FAILED, got %s", got.Status)
+	}
+	if got.ErrorType != "DOWNLOAD_ERROR" || got.ErrorMessage != "timeout" {
+		t.Errorf("unexpected error fields: %+v", got)
+	}
+	if got.RetryCount != 1 {
+		t.Errorf("expected retry_count=1, got %d", got.RetryCount)
+	}
+
+	if err := repo.SetCompleted(ctx, got.ID, true, 3.2); err != nil {
+		t.Fatalf("SetCompleted: %v", err)
+	}
+	got, err = repo.GetByID(ctx, got.ID)
+	if err != nil {
+		t.Fatalf("GetByID: %v", err)
+	}
+	if got.Status != domain.StatusCompleted || !got.PassesQuality {
+		t.Errorf("unexpected completed state: %+v", got)
+	}
+	if got.CloudCoverBBox == nil || *got.CloudCoverBBox != 3.2 {
+		t.Errorf("unexpected cloud_cover_bbox: %+v", got.CloudCoverBBox)
+	}
+}
+
+func TestSceneRepo_GetPreviousValidScene(t *testing.T) {
+	db := testDB(t)
+	prodRepo := NewProductionRepo(db)
+	repo := NewSceneRepo(db)
+	ctx := context.Background()
+
+	produccionID := setupProduction(t, ctx, prodRepo)
+
+	older := &domain.Scene{ProduccionID: produccionID, SceneID: "OLDER", SceneDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Status: domain.StatusCompleted, PassesQuality: true}
+	newerInvalid := &domain.Scene{ProduccionID: produccionID, SceneID: "NEWER_INVALID", SceneDate: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), Status: domain.StatusCompleted, PassesQuality: false}
+	target := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
+
+	if err := repo.Upsert(ctx, older); err != nil {
+		t.Fatalf("Upsert older: %v", err)
+	}
+	if err := repo.Upsert(ctx, newerInvalid); err != nil {
+		t.Fatalf("Upsert newerInvalid: %v", err)
+	}
+
+	got, err := repo.GetPreviousValidScene(ctx, produccionID, target)
+	if err != nil {
+		t.Fatalf("GetPreviousValidScene: %v", err)
+	}
+	if got == nil {
+		t.Fatal("expected a previous valid scene, got nil")
+	}
+	if got.SceneID != "OLDER" {
+		t.Errorf("expected OLDER scene (only passes_quality=1 one), got %s", got.SceneID)
+	}
+}
+
+func TestSceneRepo_ListByProduccion(t *testing.T) {
+	db := testDB(t)
+	prodRepo := NewProductionRepo(db)
+	repo := NewSceneRepo(db)
+	ctx := context.Background()
+
+	produccionID := setupProduction(t, ctx, prodRepo)
+
+	for i, id := range []string{"LIST_A", "LIST_B"} {
+		s := &domain.Scene{
+			ProduccionID: produccionID,
+			SceneID:      id,
+			SceneDate:    time.Date(2026, 8, i+1, 0, 0, 0, 0, time.UTC),
+			Status:       domain.StatusPending,
+		}
+		if err := repo.Upsert(ctx, s); err != nil {
+			t.Fatalf("Upsert: %v", err)
+		}
+	}
+
+	list, err := repo.ListByProduccion(ctx, produccionID)
+	if err != nil {
+		t.Fatalf("ListByProduccion: %v", err)
+	}
+	if len(list) != 2 {
+		t.Fatalf("expected 2 scenes, got %d", len(list))
+	}
+}
diff --git a/internal/infrastructure/database/testdb_test.go b/internal/infrastructure/database/testdb_test.go
new file mode 100644
index 0000000..f96916c
--- /dev/null
+++ b/internal/infrastructure/database/testdb_test.go
@@ -0,0 +1,42 @@
+package database
+
+import (
+	"database/sql"
+	"math/rand"
+	"os"
+	"testing"
+
+	_ "github.com/go-sql-driver/mysql"
+)
+
+// testDB opens a connection to MYSQL_TEST_DSN and runs migrations, skipping
+// the calling test when that env var is not set.
+func testDB(t *testing.T) *sql.DB {
+	t.Helper()
+
+	dsn := os.Getenv("MYSQL_TEST_DSN")
+	if dsn == "" {
+		t.Skip("MYSQL_TEST_DSN not set, skipping database test")
+	}
+
+	db, err := sql.Open("mysql", dsn)
+	if err != nil {
+		t.Fatalf("opening test db: %v", err)
+	}
+	t.Cleanup(func() { db.Close() })
+
+	if err := db.Ping(); err != nil {
+		t.Fatalf("pinging test db: %v", err)
+	}
+
+	if err := RunMigrations(db); err != nil {
+		t.Fatalf("running migrations: %v", err)
+	}
+
+	return db
+}
+
+// randomID returns a pseudo-random int64 usable as a unique produccion_id/scene id in tests.
+func randomID() int64 {
+	return int64(rand.Intn(1_000_000_000)) + 1
+}
