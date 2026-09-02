package database

import (
	"context"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

func setupProduction(t *testing.T, ctx context.Context, prodRepo *ProductionRepo) int64 {
	t.Helper()
	produccionID := randomID()
	if err := prodRepo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Cultivo: "Maiz", Monitoring: true}); err != nil {
		t.Fatalf("setting up production: %v", err)
	}
	return produccionID
}

func TestSceneRepo_UpsertAndGet(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	produccionID := setupProduction(t, ctx, prodRepo)
	sceneID := "S2A_TEST_SCENE_1"

	s := &domain.Scene{
		ProduccionID:    produccionID,
		SceneID:         sceneID,
		SceneDate:       time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		CloudCoverScene: 5.5,
		Status:          domain.StatusPending,
	}

	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByProduccionAndSceneID(ctx, produccionID, sceneID)
	if err != nil {
		t.Fatalf("GetByProduccionAndSceneID: %v", err)
	}
	if got == nil {
		t.Fatal("expected scene, got nil")
	}
	if got.Status != domain.StatusPending {
		t.Errorf("expected status PENDING, got %s", got.Status)
	}
	if got.CloudCoverScene != 5.5 {
		t.Errorf("unexpected cloud_cover_scene: %v", got.CloudCoverScene)
	}

	byID, err := repo.GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID == nil || byID.SceneID != sceneID {
		t.Errorf("unexpected GetByID result: %+v", byID)
	}
}

func TestSceneRepo_UpsertNoDuplicate(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	produccionID := setupProduction(t, ctx, prodRepo)
	sceneID := "S2A_TEST_SCENE_DUP"

	s := &domain.Scene{ProduccionID: produccionID, SceneID: sceneID, SceneDate: time.Now().UTC(), Status: domain.StatusPending}
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}
	s.CloudCoverScene = 10.0
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM s3_monitoring_escenas WHERE produccion_id = ? AND scene_id = ?", produccionID, sceneID).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

func TestSceneRepo_UpdateStatusAndError(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	produccionID := setupProduction(t, ctx, prodRepo)
	s := &domain.Scene{ProduccionID: produccionID, SceneID: "S2A_STATUS_TEST", SceneDate: time.Now().UTC(), Status: domain.StatusPending}
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByProduccionAndSceneID(ctx, produccionID, s.SceneID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if err := repo.UpdateStatus(ctx, got.ID, domain.StatusProcessing); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, err = repo.GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.StatusProcessing {
		t.Errorf("expected PROCESSING, got %s", got.Status)
	}

	if err := repo.SetError(ctx, got.ID, "DOWNLOAD_ERROR", "timeout"); err != nil {
		t.Fatalf("SetError: %v", err)
	}
	got, err = repo.GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.StatusFailed {
		t.Errorf("expected FAILED, got %s", got.Status)
	}
	if got.ErrorType != "DOWNLOAD_ERROR" || got.ErrorMessage != "timeout" {
		t.Errorf("unexpected error fields: %+v", got)
	}
	if got.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", got.RetryCount)
	}

	if err := repo.SetCompleted(ctx, got.ID, true, 3.2); err != nil {
		t.Fatalf("SetCompleted: %v", err)
	}
	got, err = repo.GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.StatusCompleted || !got.PassesQuality {
		t.Errorf("unexpected completed state: %+v", got)
	}
	if got.CloudCoverBBox == nil || *got.CloudCoverBBox != 3.2 {
		t.Errorf("unexpected cloud_cover_bbox: %+v", got.CloudCoverBBox)
	}
}

func TestSceneRepo_GetPreviousValidScene(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	produccionID := setupProduction(t, ctx, prodRepo)

	older := &domain.Scene{ProduccionID: produccionID, SceneID: "OLDER", SceneDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Status: domain.StatusCompleted, PassesQuality: true}
	newerInvalid := &domain.Scene{ProduccionID: produccionID, SceneID: "NEWER_INVALID", SceneDate: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), Status: domain.StatusCompleted, PassesQuality: false}
	target := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	if err := repo.Upsert(ctx, older); err != nil {
		t.Fatalf("Upsert older: %v", err)
	}
	if err := repo.Upsert(ctx, newerInvalid); err != nil {
		t.Fatalf("Upsert newerInvalid: %v", err)
	}

	got, err := repo.GetPreviousValidScene(ctx, produccionID, target)
	if err != nil {
		t.Fatalf("GetPreviousValidScene: %v", err)
	}
	if got == nil {
		t.Fatal("expected a previous valid scene, got nil")
	}
	if got.SceneID != "OLDER" {
		t.Errorf("expected OLDER scene (only passes_quality=1 one), got %s", got.SceneID)
	}
}

func TestSceneRepo_ListByProduccion(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	produccionID := setupProduction(t, ctx, prodRepo)

	for i, id := range []string{"LIST_A", "LIST_B"} {
		s := &domain.Scene{
			ProduccionID: produccionID,
			SceneID:      id,
			SceneDate:    time.Date(2026, 8, i+1, 0, 0, 0, 0, time.UTC),
			Status:       domain.StatusPending,
		}
		if err := repo.Upsert(ctx, s); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	list, err := repo.ListByProduccion(ctx, produccionID)
	if err != nil {
		t.Fatalf("ListByProduccion: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(list))
	}
}
