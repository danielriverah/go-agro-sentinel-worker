package sync

import (
	"context"
	"testing"
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/database"
)

func TestCalculateFinMonitoreo(t *testing.T) {
	plantacion := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	fin := CalculateFinMonitoreo(plantacion, 150, 30)
	expected := time.Date(2027, 1, 11, 0, 0, 0, 0, time.UTC)
	if !fin.Equal(expected) {
		t.Errorf("fin = %v, want %v", fin, expected)
	}
}

// ---- mocks ----

type mockDynamo struct {
	producciones []aws.DynamoProduction
	escenas      map[int64][]aws.DynamoScene
	listErr      error
	escenasErr   error
}

func (m *mockDynamo) ListActiveProducciones(ctx context.Context, tableName string) ([]aws.DynamoProduction, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.producciones, nil
}

func (m *mockDynamo) ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]aws.DynamoScene, error) {
	if m.escenasErr != nil {
		return nil, m.escenasErr
	}
	return m.escenas[produccionID], nil
}

func (m *mockDynamo) CloseProduccion(ctx context.Context, tableName string, produccionID int64, folio string) error {
	return nil
}

type mockProdRepo struct {
	byID   map[int64]*domain.Production
	upsert []domain.Production
	erp    map[int64]*database.ERPProduccion
}

func newMockProdRepo() *mockProdRepo {
	return &mockProdRepo{
		byID: map[int64]*domain.Production{},
		erp:  map[int64]*database.ERPProduccion{},
	}
}

func (m *mockProdRepo) Upsert(ctx context.Context, p *domain.Production) error {
	cp := *p
	m.byID[p.ProduccionID] = &cp
	m.upsert = append(m.upsert, cp)
	return nil
}

func (m *mockProdRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
	p, ok := m.byID[produccionID]
	if !ok {
		return nil, nil
	}
	cp := *p
	return &cp, nil
}

func (m *mockProdRepo) ListActive(ctx context.Context) ([]*domain.Production, error) {
	var result []*domain.Production
	for _, p := range m.byID {
		if p.Monitoring {
			cp := *p
			result = append(result, &cp)
		}
	}
	return result, nil
}

// UpdateSyncState mirrors the columns the real UPDATE writes, including the
// geometry ones. Leaving those out would let a regression that overwrites an
// edited polygon slip past TestRunOnce_DoesNotOverwriteEditedPolygon.
func (m *mockProdRepo) UpdateSyncState(ctx context.Context, p *domain.Production) error {
	if existing, ok := m.byID[p.ProduccionID]; ok {
		existing.Monitoring = p.Monitoring
		existing.UltimaSincronizacion = p.UltimaSincronizacion
		existing.FechaFin = p.FechaFin
		existing.MaxDiasMonitoring = p.MaxDiasMonitoring
		existing.PBoxJSON = p.PBoxJSON
		existing.TileBBoxJSON = p.TileBBoxJSON
		existing.PoligonoJSON = p.PoligonoJSON
		existing.TileCenterLat = p.TileCenterLat
		existing.TileCenterLon = p.TileCenterLon
	}
	return nil
}

func (m *mockProdRepo) UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool) error {
	return nil
}

func (m *mockProdRepo) GetERPDetails(ctx context.Context, produccionID int64) (*database.ERPProduccion, error) {
	if erp, ok := m.erp[produccionID]; ok {
		return erp, nil
	}
	return nil, nil
}

func (m *mockProdRepo) GetVariedades(ctx context.Context, produccionID int64) (string, error) {
	return "", nil
}

func (m *mockProdRepo) ExistsInERP(ctx context.Context, produccionID int64) (bool, error) {
	return true, nil
}

type mockSceneRepo struct {
	byKey  map[string]*domain.Scene
	upsert []domain.Scene
}

func newMockSceneRepo() *mockSceneRepo {
	return &mockSceneRepo{byKey: map[string]*domain.Scene{}}
}

func sceneKey(monitoringProdID uint, sceneName string) string {
	return string(rune(monitoringProdID)) + "|" + sceneName
}

func (m *mockSceneRepo) Upsert(ctx context.Context, s *domain.Scene) error {
	cp := *s
	m.byKey[sceneKey(s.MonitoringProduccionID, s.SceneName)] = &cp
	m.upsert = append(m.upsert, cp)
	return nil
}

func (m *mockSceneRepo) GetByProduccionAndSceneName(ctx context.Context, produccionID int64, sceneName string) (*domain.Scene, error) {
	for _, s := range m.byKey {
		if s.SceneName == sceneName {
			cp := *s
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockSceneRepo) ListByMonitoringProduccion(ctx context.Context, id uint) ([]*domain.Scene, error) {
	var result []*domain.Scene
	for _, s := range m.byKey {
		if s.MonitoringProduccionID == id {
			cp := *s
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (m *mockSceneRepo) UpdateExistsFlags(ctx context.Context, escenaID uint64, truthTif, renderTif, params, ia bool) error {
	return nil
}

func (m *mockSceneRepo) UpdateFromParams(ctx context.Context, escenaID uint64, productionCloud *float64, usable, analysis *bool) error {
	return nil
}

func (m *mockSceneRepo) UpdateStatus(ctx context.Context, id uint64, status string) error {
	return nil
}

type mockPolygonRepo struct {
	byProduccion map[int64]*domain.BBox
	err          error
}

func (m *mockPolygonRepo) GetPolygon(ctx context.Context, produccionID int64) (string, *domain.BBox, error) {
	if m.err != nil {
		return "", nil, m.err
	}
	bbox, ok := m.byProduccion[produccionID]
	if !ok {
		return "", nil, nil
	}
	return "21.80,-102.35|21.80,-102.30|21.85,-102.30|21.85,-102.35", bbox, nil
}

// mockS3 implements S3Indexer — returns no objects by default.
type mockS3 struct{}

func (m *mockS3) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return true, nil
}
func (m *mockS3) ListObjects(ctx context.Context, bucket, prefix string) ([]aws.S3ObjectInfo, error) {
	return nil, nil
}
func (m *mockS3) GetObjectContent(ctx context.Context, bucket, key string) ([]byte, error) {
	return nil, nil
}

// mockFileRepo implements SceneFileRepository — no-ops by default.
type mockFileRepo struct{}

func (m *mockFileRepo) ExistsByKeyHash(ctx context.Context, escenaID uint64, hash string) (bool, error) {
	return false, nil
}
func (m *mockFileRepo) Create(ctx context.Context, f *domain.SceneFile) error { return nil }

// mockIARepo implements IAResultRepository — no-ops by default.
type mockIARepo struct{}

func (m *mockIARepo) Upsert(ctx context.Context, result *domain.IAResultSummary) error { return nil }

// ---- RunOnce tests ----

func testCfg() (config.SyncConfig, config.SentinelConfig) {
	return config.SyncConfig{IntervalMinutes: 15, DiasMargenMonitoreo: 30},
		config.SentinelConfig{CloudCoverSceneMax: 70, CloudCoverProductionMax: 70}
}

func TestRunOnce_NewProductionWithPolygon_MonitoringEnabled(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 1, Estatus: "OPEN", FechaPlantacion: "15/07/2026", DiasProduccion: 150},
		},
		escenas: map[int64][]aws.DynamoScene{
			1: {{SceneID: "S1", Date: "2026-08-01", CloudCover: 10}},
		},
	}
	prodRepo := newMockProdRepo()
	prodRepo.erp[1] = &database.ERPProduccion{Folio: "F001", Cultivo: "Maiz", NombreRancho: "Rancho1"}
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
		1: {MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
	}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, &mockS3{}, &mockFileRepo{}, &mockIARepo{}, cfg, sentinel, "producciones", "escenas", "test-bucket", nil)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	p := prodRepo.byID[1]
	if p == nil {
		t.Fatal("expected production to be upserted")
	}
	if !p.Monitoring {
		t.Errorf("expected monitoring=true for produccion with fecha_plantacion and polygon")
	}
	if p.Cosecha != "Maiz" {
		t.Errorf("cosecha = %q, want %q", p.Cosecha, "Maiz")
	}

	// Scene should be upserted via syncEscenas using the reloaded production.
	// The mock GetByProduccionID returns the same record that was upserted.
	// SceneName comes from DynamoScene.SceneID = "S1"
	found := false
	for _, s := range sceneRepo.upsert {
		if s.SceneName == "S1" {
			found = true
			if s.Status != domain.StatusPending {
				t.Errorf("scene status = %s, want %s", s.Status, domain.StatusPending)
			}
		}
	}
	if !found {
		t.Error("expected scene S1 to be upserted")
	}
}

// TestRunOnce_DoesNotOverwriteEditedPolygon guards the assumption the polygon
// editor depends on: once a production has a pbox, evaluateMonitoring must not
// look the polygon up again, so a polygon edited through
// PUT /producciones/{id}/poligono survives every later sync cycle.
//
// The ERP polygon in this fixture is deliberately somewhere else entirely. If
// evaluateMonitoring ever stops returning early when pbox is present, the sync
// would silently replace the user's edit with the ERP geometry and this test
// fails.
func TestRunOnce_DoesNotOverwriteEditedPolygon(t *testing.T) {
	const editedPolygon = `[[-100.894830555556,21.1226694444444],[-100.893566666667,21.1234916666667],[-100.893208333333,21.1232583333333],[-100.892888888889,21.1220944444444],[-100.893888888889,21.1215333333333],[-100.894769444444,21.1221472222222]]`
	const editedPBox = `{"min_lon":-100.894830555556,"min_lat":21.1215333333333,"max_lon":-100.892888888889,"max_lat":21.1234916666667,"pbox":[-100.894830555556,21.1215333333333,-100.892888888889,21.1234916666667]}`
	const editedTileBBox = `{"min_lon":-100.9048,"min_lat":21.1125,"max_lon":-100.8848,"max_lat":21.1325,"pbox":[-100.9048,21.1125,-100.8848,21.1325]}`

	plantacion := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	lat, lon := 21.1225, -100.8938

	prodRepo := newMockProdRepo()
	prodRepo.byID[2004] = &domain.Production{
		ID:                1,
		ProduccionID:      2004,
		Monitoring:        true,
		MaxDiasMonitoring: 150,
		FechaPlantacion:   &plantacion,
		PBoxJSON:          []byte(editedPBox),
		PolygonBBoxJSON:   []byte(editedPBox),
		TileBBoxJSON:      []byte(editedTileBBox),
		PoligonoJSON:      []byte(editedPolygon),
		TileCenterLat:     &lat,
		TileCenterLon:     &lon,
	}
	prodRepo.erp[2004] = &database.ERPProduccion{Folio: "F2004", Cultivo: "Maiz", NombreRancho: "Rancho1"}

	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 2004, Estatus: "OPEN", FechaPlantacion: "15/07/2026", DiasProduccion: 150},
		},
	}
	// The ERP still holds a completely different polygon, far from the edit.
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
		2004: {MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
	}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, newMockSceneRepo(), polygonRepo, &mockS3{}, &mockFileRepo{}, &mockIARepo{}, cfg, sentinel, "producciones", "escenas", "test-bucket", nil)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	p := prodRepo.byID[2004]
	if got := string(p.PoligonoJSON); got != editedPolygon {
		t.Errorf("sync overwrote the edited polygon:\n got: %s\nwant: %s", got, editedPolygon)
	}
	if got := string(p.PBoxJSON); got != editedPBox {
		t.Errorf("sync overwrote the edited pbox:\n got: %s\nwant: %s", got, editedPBox)
	}
	if got := string(p.TileBBoxJSON); got != editedTileBBox {
		t.Errorf("sync moved the download tile:\n got: %s\nwant: %s", got, editedTileBBox)
	}
	if p.TileCenterLat == nil || *p.TileCenterLat != lat {
		t.Errorf("tile_center_lat changed: %v, want %v", p.TileCenterLat, lat)
	}
	if !p.Monitoring {
		t.Error("production should stay monitored")
	}
}

func TestRunOnce_NoFechaPlantacion_MonitoringDisabled(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 2, Estatus: "OPEN", DiasProduccion: 150},
		},
	}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, &mockS3{}, &mockFileRepo{}, &mockIARepo{}, cfg, sentinel, "producciones", "escenas", "test-bucket", nil)

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	p := prodRepo.byID[2]
	if p == nil {
		t.Fatal("expected production to be upserted")
	}
	if p.Monitoring {
		t.Errorf("expected monitoring=false when no FechaPlantacion")
	}

	if len(sceneRepo.upsert) != 0 {
		t.Errorf("expected no scenes when monitoring=false, got %d", len(sceneRepo.upsert))
	}
}

func TestRunOnce_DynamoError_ReturnsError(t *testing.T) {
	dynamo := &mockDynamo{listErr: context.DeadlineExceeded}
	svc := New(dynamo, newMockProdRepo(), newMockSceneRepo(),
		&mockPolygonRepo{}, &mockS3{}, &mockFileRepo{}, &mockIARepo{}, testCfgSimple(), config.SentinelConfig{},
		"producciones", "escenas", "test-bucket", nil)

	err := svc.RunOnce(context.Background())
	if err == nil {
		t.Error("expected error when dynamo fails")
	}
}

func testCfgSimple() config.SyncConfig {
	cfg, _ := testCfg()
	return cfg
}

func TestRunOnce_BloqueadoProduccion_Skipped(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 3, Estatus: "OPEN", FechaPlantacion: "15/07/2026", DiasProduccion: 150},
		},
	}
	prodRepo := newMockProdRepo()
	// Pre-seed a blocked production
	prodRepo.byID[3] = &domain.Production{ProduccionID: 3, Bloqueado: true}

	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, &mockS3{}, &mockFileRepo{}, &mockIARepo{}, cfg, sentinel, "producciones", "escenas", "test-bucket", nil)

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Should not have upserted anything new
	if len(prodRepo.upsert) != 0 {
		t.Errorf("expected no upsert for blocked produccion, got %d", len(prodRepo.upsert))
	}
}
