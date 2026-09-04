diff --git a/cmd/sync/main.go b/cmd/sync/main.go
index e92c51e..6677b78 100644
--- a/cmd/sync/main.go
+++ b/cmd/sync/main.go
@@ -1,26 +1,75 @@
 package main
 
 import (
-	"fmt"
+	"context"
 	"log"
 	"os"
+	"os/signal"
+	"syscall"
 
 	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/infrastructure/aws"
+	"agro-sentinel-worker/internal/infrastructure/database"
 	"agro-sentinel-worker/internal/logger"
+	"agro-sentinel-worker/internal/sync"
 )
 
 func main() {
 	cfgPath := "configs/config.yaml"
 	if p := os.Getenv("CONFIG_PATH"); p != "" {
 		cfgPath = p
 	}
 
 	cfg, err := config.Load(cfgPath)
 	if err != nil {
 		log.Fatalf("loading config: %v", err)
 	}
 
 	l := logger.New(cfg.Logging)
 	l.Info("sync starting", "name", cfg.App.Name, "interval_minutes", cfg.Sync.IntervalMinutes)
-	fmt.Println("sync: no DynamoDB configured yet")
+
+	db, err := database.NewConnection(cfg.MySQL)
+	if err != nil {
+		l.Error("connecting to mysql failed", "error", err)
+		os.Exit(1)
+	}
+	defer db.Close()
+
+	if err := database.RunMigrations(db); err != nil {
+		l.Error("running migrations failed", "error", err)
+		os.Exit(1)
+	}
+
+	awsCfg, err := aws.NewSession(cfg.AWS)
+	if err != nil {
+		l.Error("creating aws session failed", "error", err)
+		os.Exit(1)
+	}
+
+	dynamoClient := aws.NewDynamoDBClient(awsCfg)
+	prodRepo := database.NewProductionRepo(db)
+	sceneRepo := database.NewSceneRepo(db)
+	polygonRepo := database.NewPolygonRepo(db, sync.CalculateBBoxFromWKT)
+
+	svc := sync.New(
+		dynamoClient,
+		prodRepo,
+		sceneRepo,
+		polygonRepo,
+		cfg.Sync,
+		cfg.Sentinel,
+		cfg.DynamoDB.TableProducciones,
+		cfg.DynamoDB.TableEscenas,
+		l,
+	)
+
+	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
+	defer stop()
+
+	if err := svc.RunLoop(ctx); err != nil && ctx.Err() == nil {
+		l.Error("sync loop exited with error", "error", err)
+		os.Exit(1)
+	}
+
+	l.Info("sync stopped")
 }
diff --git a/internal/infrastructure/database/polygon_repo.go b/internal/infrastructure/database/polygon_repo.go
new file mode 100644
index 0000000..1a872a8
--- /dev/null
+++ b/internal/infrastructure/database/polygon_repo.go
@@ -0,0 +1,53 @@
+package database
+
+import (
+	"context"
+	"database/sql"
+	"errors"
+	"fmt"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// PolygonRepo looks up the monitored polygon assigned to a production and
+// extracts its bounding box. It is the concrete implementation of
+// sync.PolygonRepository.
+type PolygonRepo struct {
+	db          *sql.DB
+	bboxFromWKT func(wkt string) (*domain.BBox, error)
+}
+
+// NewPolygonRepo creates a PolygonRepo. bboxFromWKT parses the WKT polygon
+// text into a domain.BBox (typically sync.CalculateBBoxFromWKT) — it is
+// injected here so the database package does not need to depend on sync.
+func NewPolygonRepo(db *sql.DB, bboxFromWKT func(wkt string) (*domain.BBox, error)) *PolygonRepo {
+	return &PolygonRepo{db: db, bboxFromWKT: bboxFromWKT}
+}
+
+// GetPolygonBBox fetches the monitored polygon for produccionID from
+// asignaciones_zonas_producciones and returns its bounding box. It returns
+// (nil, nil) when the production has no assigned polygon.
+func (r *PolygonRepo) GetPolygonBBox(ctx context.Context, produccionID int64) (*domain.BBox, error) {
+	const q = `
+SELECT ST_AsText(poligono)
+FROM asignaciones_zonas_producciones
+WHERE produccion_id = ?
+LIMIT 1
+`
+
+	var wkt string
+	err := r.db.QueryRowContext(ctx, q, produccionID).Scan(&wkt)
+	if err != nil {
+		if errors.Is(err, sql.ErrNoRows) {
+			return nil, nil
+		}
+		return nil, fmt.Errorf("getting polygon for produccion %d: %w", produccionID, err)
+	}
+
+	bbox, err := r.bboxFromWKT(wkt)
+	if err != nil {
+		return nil, fmt.Errorf("parsing polygon for produccion %d: %w", produccionID, err)
+	}
+
+	return bbox, nil
+}
diff --git a/internal/sync/bbox.go b/internal/sync/bbox.go
new file mode 100644
index 0000000..0b47db8
--- /dev/null
+++ b/internal/sync/bbox.go
@@ -0,0 +1,72 @@
+package sync
+
+import (
+	"fmt"
+	"regexp"
+	"strconv"
+	"strings"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// coordPairRe matches "x y" coordinate pairs inside a WKT geometry string.
+var coordPairRe = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)`)
+
+// CalculateBBoxFromWKT parses a WKT POLYGON (or MULTIPOLYGON) string, as
+// returned by MySQL spatial columns via ST_AsText, and returns the envelope
+// (bounding box) of all its coordinates. It does not depend on GDAL — this
+// project uses pure Go WKT parsing for BBOX extraction.
+func CalculateBBoxFromWKT(wkt string) (*domain.BBox, error) {
+	trimmed := strings.TrimSpace(wkt)
+	if trimmed == "" {
+		return nil, fmt.Errorf("empty WKT string")
+	}
+
+	// Strip an optional "SRID=4326;" prefix.
+	if idx := strings.Index(trimmed, ";"); idx != -1 {
+		trimmed = trimmed[idx+1:]
+	}
+
+	upper := strings.ToUpper(trimmed)
+	if !strings.HasPrefix(upper, "POLYGON") && !strings.HasPrefix(upper, "MULTIPOLYGON") {
+		return nil, fmt.Errorf("unsupported WKT geometry type: %s", trimmed)
+	}
+
+	matches := coordPairRe.FindAllStringSubmatch(trimmed, -1)
+	if len(matches) == 0 {
+		return nil, fmt.Errorf("no coordinates found in WKT: %s", trimmed)
+	}
+
+	var bbox domain.BBox
+	for i, m := range matches {
+		x, err := strconv.ParseFloat(m[1], 64)
+		if err != nil {
+			return nil, fmt.Errorf("parsing x coordinate %q: %w", m[1], err)
+		}
+		y, err := strconv.ParseFloat(m[2], 64)
+		if err != nil {
+			return nil, fmt.Errorf("parsing y coordinate %q: %w", m[2], err)
+		}
+
+		if i == 0 {
+			bbox.MinX, bbox.MaxX = x, x
+			bbox.MinY, bbox.MaxY = y, y
+			continue
+		}
+
+		if x < bbox.MinX {
+			bbox.MinX = x
+		}
+		if x > bbox.MaxX {
+			bbox.MaxX = x
+		}
+		if y < bbox.MinY {
+			bbox.MinY = y
+		}
+		if y > bbox.MaxY {
+			bbox.MaxY = y
+		}
+	}
+
+	return &bbox, nil
+}
diff --git a/internal/sync/bbox_test.go b/internal/sync/bbox_test.go
new file mode 100644
index 0000000..5f2365e
--- /dev/null
+++ b/internal/sync/bbox_test.go
@@ -0,0 +1,37 @@
+package sync
+
+import "testing"
+
+func TestCalculateBBoxFromWKT(t *testing.T) {
+	wkt := "POLYGON((-102.35 21.80, -102.30 21.80, -102.30 21.85, -102.35 21.85, -102.35 21.80))"
+	bbox, err := CalculateBBoxFromWKT(wkt)
+	if err != nil {
+		t.Fatalf("CalculateBBoxFromWKT failed: %v", err)
+	}
+	if bbox.MinX != -102.35 || bbox.MaxX != -102.30 || bbox.MinY != 21.80 || bbox.MaxY != 21.85 {
+		t.Errorf("bbox = %+v, unexpected values", bbox)
+	}
+}
+
+func TestCalculateBBoxFromWKT_WithSRID(t *testing.T) {
+	wkt := "SRID=4326;POLYGON((-100 20, -99 20, -99 21, -100 21, -100 20))"
+	bbox, err := CalculateBBoxFromWKT(wkt)
+	if err != nil {
+		t.Fatalf("CalculateBBoxFromWKT failed: %v", err)
+	}
+	if bbox.MinX != -100 || bbox.MaxX != -99 || bbox.MinY != 20 || bbox.MaxY != 21 {
+		t.Errorf("bbox = %+v, unexpected values", bbox)
+	}
+}
+
+func TestCalculateBBoxFromWKT_Empty(t *testing.T) {
+	if _, err := CalculateBBoxFromWKT(""); err == nil {
+		t.Fatal("expected error for empty WKT")
+	}
+}
+
+func TestCalculateBBoxFromWKT_Invalid(t *testing.T) {
+	if _, err := CalculateBBoxFromWKT("POINT(1 2)"); err == nil {
+		t.Fatal("expected error for unsupported geometry type")
+	}
+}
diff --git a/internal/sync/sync.go b/internal/sync/sync.go
new file mode 100644
index 0000000..a18f451
--- /dev/null
+++ b/internal/sync/sync.go
@@ -0,0 +1,292 @@
+// Package sync implements the periodic sync cycle that pulls active
+// producciones and their escenas from DynamoDB and reconciles them into
+// MySQL, deciding which producciones should be actively monitored.
+package sync
+
+import (
+	"context"
+	"log/slog"
+	"time"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/domain"
+	"agro-sentinel-worker/internal/infrastructure/aws"
+)
+
+// Monitoring motivo codes recorded on s3_monitoring_producciones.
+const (
+	MotivoOK                 = "OK"
+	MotivoSinFechaPlantacion = "SIN_FECHA_PLANTACION"
+	MotivoFinMonitoreo       = "FIN_MONITOREO_ALCANZADO"
+	MotivoSinPoligono        = "SIN_POLIGONO"
+)
+
+// DynamoReader is the subset of aws.DynamoDBClient the sync service needs.
+type DynamoReader interface {
+	ListActiveProducciones(ctx context.Context, tableName string) ([]aws.DynamoProduction, error)
+	ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]aws.DynamoScene, error)
+}
+
+// ProductionRepository is the subset of database.ProductionRepo the sync
+// service needs.
+type ProductionRepository interface {
+	Upsert(ctx context.Context, p *domain.Production) error
+	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
+	ListActive(ctx context.Context) ([]*domain.Production, error)
+	UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool, motivo string) error
+	UpdateBBox(ctx context.Context, produccionID int64, bbox domain.BBox) error
+	SetBloqueado(ctx context.Context, produccionID int64, motivo string) error
+	Desbloquear(ctx context.Context, produccionID int64, usuario string) error
+	IncrementEscenas(ctx context.Context, produccionID int64, valid bool) error
+}
+
+// SceneRepository is the subset of database.SceneRepo the sync service needs.
+type SceneRepository interface {
+	Upsert(ctx context.Context, s *domain.Scene) error
+	GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error)
+}
+
+// PolygonRepository looks up the monitored polygon for a produccion and
+// returns its bounding box. The concrete implementation queries the
+// asignaciones_zonas_producciones table and extracts the BBOX from the
+// polygon column using CalculateBBoxFromWKT.
+type PolygonRepository interface {
+	GetPolygonBBox(ctx context.Context, produccionID int64) (*domain.BBox, error)
+}
+
+// Service runs the DynamoDB -> MySQL sync cycle.
+type Service struct {
+	dynamo      DynamoReader
+	prodRepo    ProductionRepository
+	sceneRepo   SceneRepository
+	polygonRepo PolygonRepository
+
+	cfg      config.SyncConfig
+	sentinel config.SentinelConfig
+
+	tableProducciones string
+	tableEscenas      string
+
+	logger *slog.Logger
+
+	// now is overridable in tests.
+	now func() time.Time
+}
+
+// New builds a Service with the given dependencies.
+//
+// tableProducciones and tableEscenas name the DynamoDB tables scanned each
+// cycle (config.DynamoDBConfig.TableProducciones / TableEscenas).
+func New(
+	dynamo DynamoReader,
+	prodRepo ProductionRepository,
+	sceneRepo SceneRepository,
+	polygonRepo PolygonRepository,
+	cfg config.SyncConfig,
+	sentinel config.SentinelConfig,
+	tableProducciones string,
+	tableEscenas string,
+	logger *slog.Logger,
+) *Service {
+	if logger == nil {
+		logger = slog.Default()
+	}
+
+	return &Service{
+		dynamo:            dynamo,
+		prodRepo:          prodRepo,
+		sceneRepo:         sceneRepo,
+		polygonRepo:       polygonRepo,
+		cfg:               cfg,
+		sentinel:          sentinel,
+		tableProducciones: tableProducciones,
+		tableEscenas:      tableEscenas,
+		logger:            logger,
+		now:               time.Now,
+	}
+}
+
+// CalculateFinMonitoreo returns the date monitoring should stop for a
+// production planted on plantacion, given its production cycle length and
+// the configured monitoring margin (both in days).
+func CalculateFinMonitoreo(plantacion time.Time, diasProduccion, diasMargen int) time.Time {
+	return plantacion.AddDate(0, 0, diasProduccion+diasMargen)
+}
+
+// RunOnce executes a single sync cycle: it fetches active producciones from
+// DynamoDB, reconciles each into MySQL (deciding whether it should be
+// monitored), and — for producciones that end up monitored — fetches and
+// upserts their new escenas.
+func (s *Service) RunOnce(ctx context.Context) error {
+	producciones, err := s.dynamo.ListActiveProducciones(ctx, s.tableProducciones)
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "listing active producciones", Wrapped: err}
+	}
+
+	for _, dp := range producciones {
+		if err := s.syncProduccion(ctx, dp); err != nil {
+			s.logger.Error("syncing produccion failed", "produccion_id", dp.ProduccionID, "error", err)
+		}
+	}
+
+	return nil
+}
+
+// RunLoop calls RunOnce immediately and then every cfg.IntervalMinutes,
+// until ctx is cancelled.
+func (s *Service) RunLoop(ctx context.Context) error {
+	interval := time.Duration(s.cfg.IntervalMinutes) * time.Minute
+	if interval <= 0 {
+		interval = time.Minute
+	}
+
+	if err := s.RunOnce(ctx); err != nil {
+		s.logger.Error("sync cycle failed", "error", err)
+	}
+
+	ticker := time.NewTicker(interval)
+	defer ticker.Stop()
+
+	for {
+		select {
+		case <-ctx.Done():
+			return ctx.Err()
+		case <-ticker.C:
+			if err := s.RunOnce(ctx); err != nil {
+				s.logger.Error("sync cycle failed", "error", err)
+			}
+		}
+	}
+}
+
+func (s *Service) syncProduccion(ctx context.Context, dp aws.DynamoProduction) error {
+	existing, err := s.prodRepo.GetByProduccionID(ctx, dp.ProduccionID)
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "getting production", Wrapped: err}
+	}
+
+	if existing != nil && existing.Bloqueado {
+		s.logger.Info("skipping blocked produccion", "produccion_id", dp.ProduccionID)
+		return nil
+	}
+
+	prod := buildProduction(dp, existing)
+
+	monitoring, motivo := s.evaluateMonitoring(ctx, &prod)
+	prod.Monitoring = monitoring
+	prod.MonitoringMotivo = motivo
+
+	now := s.now()
+	prod.LastSyncAt = &now
+
+	if err := s.prodRepo.Upsert(ctx, &prod); err != nil {
+		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "upserting production", Wrapped: err}
+	}
+
+	if !monitoring {
+		return nil
+	}
+
+	return s.syncEscenas(ctx, prod.ProduccionID)
+}
+
+// buildProduction merges a DynamoDB production item onto the existing MySQL
+// row (if any), preserving fields DynamoDB doesn't own.
+func buildProduction(dp aws.DynamoProduction, existing *domain.Production) domain.Production {
+	var prod domain.Production
+	if existing != nil {
+		prod = *existing
+	}
+
+	prod.ProduccionID = dp.ProduccionID
+	prod.Cultivo = dp.Cultivo
+	prod.Ciclo = dp.Ciclo
+	prod.DiasProduccion = dp.DiasProduccion
+
+	if dp.FechaPlantacion != "" {
+		if t, err := time.Parse("2006-01-02", dp.FechaPlantacion); err == nil {
+			prod.FechaPlantacion = &t
+		}
+	}
+
+	return prod
+}
+
+// evaluateMonitoring decides whether prod should be actively monitored,
+// returning the monitoring flag and the motivo to record. On success it also
+// populates prod.BBox and prod.FechaFinMonitoreo.
+func (s *Service) evaluateMonitoring(ctx context.Context, prod *domain.Production) (bool, string) {
+	if prod.FechaPlantacion == nil {
+		return false, MotivoSinFechaPlantacion
+	}
+
+	fin := CalculateFinMonitoreo(*prod.FechaPlantacion, prod.DiasProduccion, s.cfg.DiasMargenMonitoreo)
+	prod.FechaFinMonitoreo = &fin
+
+	if s.now().After(fin) {
+		return false, MotivoFinMonitoreo
+	}
+
+	bbox, err := s.polygonRepo.GetPolygonBBox(ctx, prod.ProduccionID)
+	if err != nil {
+		s.logger.Error("getting polygon bbox failed", "produccion_id", prod.ProduccionID, "error", err)
+		return false, MotivoSinPoligono
+	}
+	if bbox == nil {
+		return false, MotivoSinPoligono
+	}
+
+	prod.BBox = bbox
+
+	return true, MotivoOK
+}
+
+func (s *Service) syncEscenas(ctx context.Context, produccionID int64) error {
+	escenas, err := s.dynamo.ListEscenas(ctx, s.tableEscenas, produccionID)
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "listing escenas", Wrapped: err}
+	}
+
+	for _, de := range escenas {
+		if de.CloudCover > s.sentinel.CloudCoverSceneMax {
+			continue
+		}
+
+		existing, err := s.sceneRepo.GetByProduccionAndSceneID(ctx, produccionID, de.SceneID)
+		if err != nil {
+			s.logger.Error("getting scene failed", "produccion_id", produccionID, "scene_id", de.SceneID, "error", err)
+			continue
+		}
+		if existing != nil {
+			continue
+		}
+
+		sceneDate, err := parseSceneDate(de.Date)
+		if err != nil {
+			s.logger.Error("parsing scene date failed", "scene_id", de.SceneID, "date", de.Date, "error", err)
+			continue
+		}
+
+		scene := &domain.Scene{
+			ProduccionID:    produccionID,
+			SceneID:         de.SceneID,
+			SceneDate:       sceneDate,
+			CloudCoverScene: de.CloudCover,
+			Status:          domain.StatusPending,
+		}
+
+		if err := s.sceneRepo.Upsert(ctx, scene); err != nil {
+			s.logger.Error("upserting scene failed", "produccion_id", produccionID, "scene_id", de.SceneID, "error", err)
+			continue
+		}
+	}
+
+	return nil
+}
+
+func parseSceneDate(date string) (time.Time, error) {
+	if t, err := time.Parse("2006-01-02", date); err == nil {
+		return t, nil
+	}
+	return time.Parse(time.RFC3339, date)
+}
diff --git a/internal/sync/sync_test.go b/internal/sync/sync_test.go
new file mode 100644
index 0000000..8dad1b3
--- /dev/null
+++ b/internal/sync/sync_test.go
@@ -0,0 +1,383 @@
+package sync
+
+import (
+	"context"
+	"errors"
+	"testing"
+	"time"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/domain"
+	"agro-sentinel-worker/internal/infrastructure/aws"
+)
+
+func TestCalculateFinMonitoreo(t *testing.T) {
+	plantacion := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
+	fin := CalculateFinMonitoreo(plantacion, 150, 30)
+	expected := time.Date(2027, 1, 11, 0, 0, 0, 0, time.UTC)
+	if !fin.Equal(expected) {
+		t.Errorf("fin = %v, want %v", fin, expected)
+	}
+}
+
+// ---- mocks ----
+
+type mockDynamo struct {
+	producciones []aws.DynamoProduction
+	escenas      map[int64][]aws.DynamoScene
+	listErr      error
+	escenasErr   error
+}
+
+func (m *mockDynamo) ListActiveProducciones(ctx context.Context, tableName string) ([]aws.DynamoProduction, error) {
+	if m.listErr != nil {
+		return nil, m.listErr
+	}
+	return m.producciones, nil
+}
+
+func (m *mockDynamo) ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]aws.DynamoScene, error) {
+	if m.escenasErr != nil {
+		return nil, m.escenasErr
+	}
+	return m.escenas[produccionID], nil
+}
+
+type mockProdRepo struct {
+	byID   map[int64]*domain.Production
+	upsert []domain.Production
+}
+
+func newMockProdRepo() *mockProdRepo {
+	return &mockProdRepo{byID: map[int64]*domain.Production{}}
+}
+
+func (m *mockProdRepo) Upsert(ctx context.Context, p *domain.Production) error {
+	cp := *p
+	m.byID[p.ProduccionID] = &cp
+	m.upsert = append(m.upsert, cp)
+	return nil
+}
+
+func (m *mockProdRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
+	p, ok := m.byID[produccionID]
+	if !ok {
+		return nil, nil
+	}
+	cp := *p
+	return &cp, nil
+}
+
+func (m *mockProdRepo) ListActive(ctx context.Context) ([]*domain.Production, error) { return nil, nil }
+func (m *mockProdRepo) UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool, motivo string) error {
+	return nil
+}
+func (m *mockProdRepo) UpdateBBox(ctx context.Context, produccionID int64, bbox domain.BBox) error {
+	return nil
+}
+func (m *mockProdRepo) SetBloqueado(ctx context.Context, produccionID int64, motivo string) error {
+	return nil
+}
+func (m *mockProdRepo) Desbloquear(ctx context.Context, produccionID int64, usuario string) error {
+	return nil
+}
+func (m *mockProdRepo) IncrementEscenas(ctx context.Context, produccionID int64, valid bool) error {
+	return nil
+}
+
+type mockSceneRepo struct {
+	byKey  map[string]*domain.Scene
+	upsert []domain.Scene
+}
+
+func newMockSceneRepo() *mockSceneRepo {
+	return &mockSceneRepo{byKey: map[string]*domain.Scene{}}
+}
+
+func sceneKey(produccionID int64, sceneID string) string {
+	return string(rune(produccionID)) + "|" + sceneID
+}
+
+func (m *mockSceneRepo) Upsert(ctx context.Context, s *domain.Scene) error {
+	cp := *s
+	m.byKey[sceneKey(s.ProduccionID, s.SceneID)] = &cp
+	m.upsert = append(m.upsert, cp)
+	return nil
+}
+
+func (m *mockSceneRepo) GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error) {
+	s, ok := m.byKey[sceneKey(produccionID, sceneID)]
+	if !ok {
+		return nil, nil
+	}
+	cp := *s
+	return &cp, nil
+}
+
+type mockPolygonRepo struct {
+	byProduccion map[int64]*domain.BBox
+	err          error
+}
+
+func (m *mockPolygonRepo) GetPolygonBBox(ctx context.Context, produccionID int64) (*domain.BBox, error) {
+	if m.err != nil {
+		return nil, m.err
+	}
+	bbox, ok := m.byProduccion[produccionID]
+	if !ok {
+		return nil, nil
+	}
+	return bbox, nil
+}
+
+// ---- RunOnce tests ----
+
+func testCfg() (config.SyncConfig, config.SentinelConfig) {
+	return config.SyncConfig{IntervalMinutes: 15, DiasMargenMonitoreo: 30},
+		config.SentinelConfig{CloudCoverSceneMax: 70, CloudCoverProductionMax: 70}
+}
+
+func TestRunOnce_NewProductionWithPolygon_MonitoringEnabled(t *testing.T) {
+	dynamo := &mockDynamo{
+		producciones: []aws.DynamoProduction{
+			{ProduccionID: 1, Activa: true, Cultivo: "Maiz", Ciclo: "PV", FechaPlantacion: "2026-07-15", DiasProduccion: 150},
+		},
+		escenas: map[int64][]aws.DynamoScene{
+			1: {{SceneID: "S1", ProduccionID: 1, Date: "2026-08-01", CloudCover: 10}},
+		},
+	}
+	prodRepo := newMockProdRepo()
+	sceneRepo := newMockSceneRepo()
+	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
+		1: {MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
+	}}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
+
+	if err := svc.RunOnce(context.Background()); err != nil {
+		t.Fatalf("RunOnce failed: %v", err)
+	}
+
+	p := prodRepo.byID[1]
+	if p == nil {
+		t.Fatal("expected production to be upserted")
+	}
+	if !p.Monitoring {
+		t.Errorf("expected monitoring=true, motivo=%s", p.MonitoringMotivo)
+	}
+	if p.MonitoringMotivo != MotivoOK {
+		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoOK)
+	}
+	if p.BBox == nil || p.BBox.MinX != -102.35 {
+		t.Errorf("bbox not set correctly: %+v", p.BBox)
+	}
+
+	scene := sceneRepo.byKey[sceneKey(1, "S1")]
+	if scene == nil {
+		t.Fatal("expected scene to be upserted")
+	}
+	if scene.Status != domain.StatusPending {
+		t.Errorf("scene status = %s, want %s", scene.Status, domain.StatusPending)
+	}
+}
+
+func TestRunOnce_NoFechaPlantacion_MonitoringDisabled(t *testing.T) {
+	dynamo := &mockDynamo{
+		producciones: []aws.DynamoProduction{
+			{ProduccionID: 2, Activa: true, DiasProduccion: 150},
+		},
+	}
+	prodRepo := newMockProdRepo()
+	sceneRepo := newMockSceneRepo()
+	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{}}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+
+	if err := svc.RunOnce(context.Background()); err != nil {
+		t.Fatalf("RunOnce failed: %v", err)
+	}
+
+	p := prodRepo.byID[2]
+	if p == nil {
+		t.Fatal("expected production to be upserted")
+	}
+	if p.Monitoring {
+		t.Error("expected monitoring=false")
+	}
+	if p.MonitoringMotivo != MotivoSinFechaPlantacion {
+		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoSinFechaPlantacion)
+	}
+}
+
+func TestRunOnce_PastFinMonitoreo_MonitoringDisabled(t *testing.T) {
+	dynamo := &mockDynamo{
+		producciones: []aws.DynamoProduction{
+			{ProduccionID: 3, Activa: true, FechaPlantacion: "2020-01-01", DiasProduccion: 150},
+		},
+	}
+	prodRepo := newMockProdRepo()
+	sceneRepo := newMockSceneRepo()
+	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
+		3: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
+	}}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+
+	if err := svc.RunOnce(context.Background()); err != nil {
+		t.Fatalf("RunOnce failed: %v", err)
+	}
+
+	p := prodRepo.byID[3]
+	if p == nil {
+		t.Fatal("expected production to be upserted")
+	}
+	if p.Monitoring {
+		t.Error("expected monitoring=false")
+	}
+	if p.MonitoringMotivo != MotivoFinMonitoreo {
+		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoFinMonitoreo)
+	}
+}
+
+func TestRunOnce_NoPolygon_MonitoringDisabled(t *testing.T) {
+	dynamo := &mockDynamo{
+		producciones: []aws.DynamoProduction{
+			{ProduccionID: 4, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
+		},
+	}
+	prodRepo := newMockProdRepo()
+	sceneRepo := newMockSceneRepo()
+	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{}}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
+
+	if err := svc.RunOnce(context.Background()); err != nil {
+		t.Fatalf("RunOnce failed: %v", err)
+	}
+
+	p := prodRepo.byID[4]
+	if p == nil {
+		t.Fatal("expected production to be upserted")
+	}
+	if p.Monitoring {
+		t.Error("expected monitoring=false")
+	}
+	if p.MonitoringMotivo != MotivoSinPoligono {
+		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoSinPoligono)
+	}
+}
+
+func TestRunOnce_BlockedProduction_Skipped(t *testing.T) {
+	dynamo := &mockDynamo{
+		producciones: []aws.DynamoProduction{
+			{ProduccionID: 5, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
+		},
+	}
+	prodRepo := newMockProdRepo()
+	prodRepo.byID[5] = &domain.Production{
+		ProduccionID: 5, Bloqueado: true, BloqueadoMotivo: "manual hold",
+	}
+	sceneRepo := newMockSceneRepo()
+	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
+		5: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
+	}}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+
+	if err := svc.RunOnce(context.Background()); err != nil {
+		t.Fatalf("RunOnce failed: %v", err)
+	}
+
+	if len(prodRepo.upsert) != 0 {
+		t.Errorf("expected blocked production to not be upserted, got %d upserts", len(prodRepo.upsert))
+	}
+	if len(sceneRepo.upsert) != 0 {
+		t.Errorf("expected no scenes synced for blocked production")
+	}
+}
+
+func TestRunOnce_SceneAboveCloudCoverThreshold_Filtered(t *testing.T) {
+	dynamo := &mockDynamo{
+		producciones: []aws.DynamoProduction{
+			{ProduccionID: 6, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
+		},
+		escenas: map[int64][]aws.DynamoScene{
+			6: {
+				{SceneID: "GOOD", ProduccionID: 6, Date: "2026-08-01", CloudCover: 10},
+				{SceneID: "BAD", ProduccionID: 6, Date: "2026-08-02", CloudCover: 90},
+			},
+		},
+	}
+	prodRepo := newMockProdRepo()
+	sceneRepo := newMockSceneRepo()
+	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
+		6: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
+	}}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
+
+	if err := svc.RunOnce(context.Background()); err != nil {
+		t.Fatalf("RunOnce failed: %v", err)
+	}
+
+	if sceneRepo.byKey[sceneKey(6, "GOOD")] == nil {
+		t.Error("expected GOOD scene to be synced")
+	}
+	if sceneRepo.byKey[sceneKey(6, "BAD")] != nil {
+		t.Error("expected BAD scene (cloud cover above threshold) to be filtered out")
+	}
+}
+
+func TestRunOnce_ExistingScene_NotReUpserted(t *testing.T) {
+	dynamo := &mockDynamo{
+		producciones: []aws.DynamoProduction{
+			{ProduccionID: 7, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
+		},
+		escenas: map[int64][]aws.DynamoScene{
+			7: {{SceneID: "EXISTS", ProduccionID: 7, Date: "2026-08-01", CloudCover: 10}},
+		},
+	}
+	prodRepo := newMockProdRepo()
+	sceneRepo := newMockSceneRepo()
+	sceneRepo.byKey[sceneKey(7, "EXISTS")] = &domain.Scene{
+		ProduccionID: 7, SceneID: "EXISTS", Status: domain.StatusCompleted,
+	}
+	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
+		7: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
+	}}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
+
+	if err := svc.RunOnce(context.Background()); err != nil {
+		t.Fatalf("RunOnce failed: %v", err)
+	}
+
+	if len(sceneRepo.upsert) != 0 {
+		t.Errorf("expected no upserts for already-existing scene, got %d", len(sceneRepo.upsert))
+	}
+}
+
+func TestRunOnce_DynamoListError_Propagates(t *testing.T) {
+	dynamo := &mockDynamo{listErr: errors.New("boom")}
+	prodRepo := newMockProdRepo()
+	sceneRepo := newMockSceneRepo()
+	polygonRepo := &mockPolygonRepo{}
+
+	cfg, sentinel := testCfg()
+	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
+
+	if err := svc.RunOnce(context.Background()); err == nil {
+		t.Fatal("expected error from RunOnce when dynamo list fails")
+	}
+}
