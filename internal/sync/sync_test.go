package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
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

type mockProdRepo struct {
	byID   map[int64]*domain.Production
	upsert []domain.Production
}

func newMockProdRepo() *mockProdRepo {
	return &mockProdRepo{byID: map[int64]*domain.Production{}}
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

func (m *mockProdRepo) ListActive(ctx context.Context) ([]*domain.Production, error) { return nil, nil }
func (m *mockProdRepo) UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool, motivo string) error {
	return nil
}
func (m *mockProdRepo) UpdateBBox(ctx context.Context, produccionID int64, bbox domain.BBox) error {
	return nil
}
func (m *mockProdRepo) SetBloqueado(ctx context.Context, produccionID int64, motivo string) error {
	return nil
}
func (m *mockProdRepo) Desbloquear(ctx context.Context, produccionID int64, usuario string) error {
	return nil
}
func (m *mockProdRepo) IncrementEscenas(ctx context.Context, produccionID int64, valid bool) error {
	return nil
}

type mockSceneRepo struct {
	byKey  map[string]*domain.Scene
	upsert []domain.Scene
}

func newMockSceneRepo() *mockSceneRepo {
	return &mockSceneRepo{byKey: map[string]*domain.Scene{}}
}

func sceneKey(produccionID int64, sceneID string) string {
	return string(rune(produccionID)) + "|" + sceneID
}

func (m *mockSceneRepo) Upsert(ctx context.Context, s *domain.Scene) error {
	cp := *s
	m.byKey[sceneKey(s.ProduccionID, s.SceneID)] = &cp
	m.upsert = append(m.upsert, cp)
	return nil
}

func (m *mockSceneRepo) GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error) {
	s, ok := m.byKey[sceneKey(produccionID, sceneID)]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

type mockPolygonRepo struct {
	byProduccion map[int64]*domain.BBox
	err          error
}

func (m *mockPolygonRepo) GetPolygonBBox(ctx context.Context, produccionID int64) (*domain.BBox, error) {
	if m.err != nil {
		return nil, m.err
	}
	bbox, ok := m.byProduccion[produccionID]
	if !ok {
		return nil, nil
	}
	return bbox, nil
}

// ---- RunOnce tests ----

func testCfg() (config.SyncConfig, config.SentinelConfig) {
	return config.SyncConfig{IntervalMinutes: 15, DiasMargenMonitoreo: 30},
		config.SentinelConfig{CloudCoverSceneMax: 70, CloudCoverProductionMax: 70}
}

func TestRunOnce_NewProductionWithPolygon_MonitoringEnabled(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 1, Activa: true, Cultivo: "Maiz", Ciclo: "PV", FechaPlantacion: "2026-07-15", DiasProduccion: 150},
		},
		escenas: map[int64][]aws.DynamoScene{
			1: {{SceneID: "S1", ProduccionID: 1, Date: "2026-08-01", CloudCover: 10}},
		},
	}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
		1: {MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
	}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	p := prodRepo.byID[1]
	if p == nil {
		t.Fatal("expected production to be upserted")
	}
	if !p.Monitoring {
		t.Errorf("expected monitoring=true, motivo=%s", p.MonitoringMotivo)
	}
	if p.MonitoringMotivo != MotivoOK {
		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoOK)
	}
	if p.BBox == nil || p.BBox.MinX != -102.35 {
		t.Errorf("bbox not set correctly: %+v", p.BBox)
	}

	scene := sceneRepo.byKey[sceneKey(1, "S1")]
	if scene == nil {
		t.Fatal("expected scene to be upserted")
	}
	if scene.Status != domain.StatusPending {
		t.Errorf("scene status = %s, want %s", scene.Status, domain.StatusPending)
	}
}

func TestRunOnce_NoFechaPlantacion_MonitoringDisabled(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 2, Activa: true, DiasProduccion: 150},
		},
	}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	p := prodRepo.byID[2]
	if p == nil {
		t.Fatal("expected production to be upserted")
	}
	if p.Monitoring {
		t.Error("expected monitoring=false")
	}
	if p.MonitoringMotivo != MotivoSinFechaPlantacion {
		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoSinFechaPlantacion)
	}
}

func TestRunOnce_PastFinMonitoreo_MonitoringDisabled(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 3, Activa: true, FechaPlantacion: "2020-01-01", DiasProduccion: 150},
		},
	}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
		3: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
	}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	p := prodRepo.byID[3]
	if p == nil {
		t.Fatal("expected production to be upserted")
	}
	if p.Monitoring {
		t.Error("expected monitoring=false")
	}
	if p.MonitoringMotivo != MotivoFinMonitoreo {
		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoFinMonitoreo)
	}
}

func TestRunOnce_NoPolygon_MonitoringDisabled(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 4, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
		},
	}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	p := prodRepo.byID[4]
	if p == nil {
		t.Fatal("expected production to be upserted")
	}
	if p.Monitoring {
		t.Error("expected monitoring=false")
	}
	if p.MonitoringMotivo != MotivoSinPoligono {
		t.Errorf("motivo = %s, want %s", p.MonitoringMotivo, MotivoSinPoligono)
	}
}

func TestRunOnce_BlockedProduction_Skipped(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 5, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
		},
	}
	prodRepo := newMockProdRepo()
	prodRepo.byID[5] = &domain.Production{
		ProduccionID: 5, Bloqueado: true, BloqueadoMotivo: "manual hold",
	}
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
		5: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
	}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	if len(prodRepo.upsert) != 0 {
		t.Errorf("expected blocked production to not be upserted, got %d upserts", len(prodRepo.upsert))
	}
	if len(sceneRepo.upsert) != 0 {
		t.Errorf("expected no scenes synced for blocked production")
	}
}

func TestRunOnce_SceneAboveCloudCoverThreshold_Filtered(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 6, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
		},
		escenas: map[int64][]aws.DynamoScene{
			6: {
				{SceneID: "GOOD", ProduccionID: 6, Date: "2026-08-01", CloudCover: 10},
				{SceneID: "BAD", ProduccionID: 6, Date: "2026-08-02", CloudCover: 90},
			},
		},
	}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
		6: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
	}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	if sceneRepo.byKey[sceneKey(6, "GOOD")] == nil {
		t.Error("expected GOOD scene to be synced")
	}
	if sceneRepo.byKey[sceneKey(6, "BAD")] != nil {
		t.Error("expected BAD scene (cloud cover above threshold) to be filtered out")
	}
}

func TestRunOnce_ExistingScene_NotReUpserted(t *testing.T) {
	dynamo := &mockDynamo{
		producciones: []aws.DynamoProduction{
			{ProduccionID: 7, Activa: true, FechaPlantacion: "2026-07-15", DiasProduccion: 150},
		},
		escenas: map[int64][]aws.DynamoScene{
			7: {{SceneID: "EXISTS", ProduccionID: 7, Date: "2026-08-01", CloudCover: 10}},
		},
	}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	sceneRepo.byKey[sceneKey(7, "EXISTS")] = &domain.Scene{
		ProduccionID: 7, SceneID: "EXISTS", Status: domain.StatusCompleted,
	}
	polygonRepo := &mockPolygonRepo{byProduccion: map[int64]*domain.BBox{
		7: {MinX: 0, MinY: 0, MaxX: 1, MaxY: 1},
	}}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	if len(sceneRepo.upsert) != 0 {
		t.Errorf("expected no upserts for already-existing scene, got %d", len(sceneRepo.upsert))
	}
}

func TestRunOnce_DynamoListError_Propagates(t *testing.T) {
	dynamo := &mockDynamo{listErr: errors.New("boom")}
	prodRepo := newMockProdRepo()
	sceneRepo := newMockSceneRepo()
	polygonRepo := &mockPolygonRepo{}

	cfg, sentinel := testCfg()
	svc := New(dynamo, prodRepo, sceneRepo, polygonRepo, cfg, sentinel, "producciones", "escenas", nil)

	if err := svc.RunOnce(context.Background()); err == nil {
		t.Fatal("expected error from RunOnce when dynamo list fails")
	}
}
