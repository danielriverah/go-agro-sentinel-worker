package database

import (
	"context"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

func setupScene(t *testing.T, ctx context.Context, prodRepo *ProductionRepo, sceneRepo *SceneRepo) int64 {
	t.Helper()
	produccionID := setupProduction(t, ctx, prodRepo)
	s := &domain.Scene{
		ProduccionID: produccionID,
		SceneID:      "S2A_FILE_TEST",
		SceneDate:    time.Now().UTC(),
		Status:       domain.StatusPending,
	}
	if err := sceneRepo.Upsert(ctx, s); err != nil {
		t.Fatalf("setting up scene: %v", err)
	}
	got, err := sceneRepo.GetByProduccionAndSceneID(ctx, produccionID, s.SceneID)
	if err != nil {
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
		EscenaID:      escenaID,
		FileType:      domain.FileMultiband,
		FileName:      "multiband.tif",
		S3Key:         "path/to/multiband.tif",
		S3Bucket:      "test-bucket",
		FileSizeBytes: 12345,
		ResolutionM:   10,
		WidthPx:       1024,
		HeightPx:      1024,
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
	if list[0].FileName != "multiband.tif" {
		t.Errorf("unexpected file name: %s", list[0].FileName)
	}
}

func TestFileRepo_GetByType(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	repo := NewFileRepo(db)
	ctx := context.Background()

	escenaID := setupScene(t, ctx, prodRepo, sceneRepo)

	multiband := &domain.SceneFile{EscenaID: escenaID, FileType: domain.FileMultiband, FileName: "m.tif", S3Key: "k1", S3Bucket: "b"}
	ndvi := &domain.SceneFile{EscenaID: escenaID, FileType: domain.FileNDVI, FileName: "ndvi.png", S3Key: "k2", S3Bucket: "b"}

	if err := repo.Create(ctx, multiband); err != nil {
		t.Fatalf("Create multiband: %v", err)
	}
	if err := repo.Create(ctx, ndvi); err != nil {
		t.Fatalf("Create ndvi: %v", err)
	}

	got, err := repo.GetByType(ctx, escenaID, domain.FileNDVI)
	if err != nil {
		t.Fatalf("GetByType: %v", err)
	}
	if got == nil {
		t.Fatal("expected file, got nil")
	}
	if got.FileName != "ndvi.png" {
		t.Errorf("unexpected file: %+v", got)
	}

	missing, err := repo.GetByType(ctx, escenaID, domain.FileSWIR)
	if err != nil {
		t.Fatalf("GetByType missing: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing type, got %+v", missing)
	}
}
