package database

import (
	"context"
	"testing"

	"agro-sentinel-worker/internal/domain"
)

func setupScene(t *testing.T, ctx context.Context, prodRepo *ProductionRepo, sceneRepo *SceneRepo) uint64 {
	t.Helper()
	produccionID := setupProduction(t, ctx, prodRepo)

	// We need the s3_monitoring_produccion_id, so upsert and reload.
	s := &domain.Scene{
		SceneName: "S2A_FILE_TEST",
		Status:    domain.StatusPending,
	}

	// Upsert requires MonitoringProduccionID — get it from the upserted production.
	prod, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil || prod == nil {
		t.Fatalf("fetching production for scene setup: %v", err)
	}
	s.MonitoringProduccionID = prod.ID

	if err := sceneRepo.Upsert(ctx, s); err != nil {
		t.Fatalf("setting up scene: %v", err)
	}

	got, err := sceneRepo.GetByProduccionAndSceneName(ctx, produccionID, s.SceneName)
	if err != nil || got == nil {
		t.Fatalf("fetching scene: %v", err)
	}
	return got.ID
}

func TestFileRepo_CreateAndList(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	repo := NewFileRepo(db)
	ctx := context.Background()

	escenaID := setupScene(t, ctx, prodRepo, sceneRepo)

	f := &domain.SceneFile{
		EscenaID:  escenaID,
		Tipo:      "multiband",
		S3Key:     "path/to/multiband.tif",
		S3Uri:     "s3://test-bucket/path/to/multiband.tif",
		SizeBytes: 12345,
		Existe:    true,
	}

	if err := repo.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.ID == 0 {
		t.Error("expected file ID to be set after Create")
	}

	list, err := repo.ListByEscena(ctx, escenaID)
	if err != nil {
		t.Fatalf("ListByEscena: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 file, got %d", len(list))
	}
	if list[0].Tipo != "multiband" {
		t.Errorf("unexpected tipo: %s", list[0].Tipo)
	}
}

func TestFileRepo_GetByTipo(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	repo := NewFileRepo(db)
	ctx := context.Background()

	escenaID := setupScene(t, ctx, prodRepo, sceneRepo)

	multiband := &domain.SceneFile{EscenaID: escenaID, Tipo: "multiband", S3Key: "k1", S3Uri: "s3://b/k1", Existe: true}
	ndvi := &domain.SceneFile{EscenaID: escenaID, Tipo: "ndvi", S3Key: "k2", S3Uri: "s3://b/k2", Existe: true}

	if err := repo.Create(ctx, multiband); err != nil {
		t.Fatalf("Create multiband: %v", err)
	}
	if err := repo.Create(ctx, ndvi); err != nil {
		t.Fatalf("Create ndvi: %v", err)
	}

	got, err := repo.GetByTipo(ctx, escenaID, "ndvi")
	if err != nil {
		t.Fatalf("GetByTipo: %v", err)
	}
	if got == nil {
		t.Fatal("expected file, got nil")
	}
	if got.Tipo != "ndvi" {
		t.Errorf("unexpected tipo: %+v", got)
	}

	missing, err := repo.GetByTipo(ctx, escenaID, "swir")
	if err != nil {
		t.Fatalf("GetByTipo missing: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing tipo, got %+v", missing)
	}
}
