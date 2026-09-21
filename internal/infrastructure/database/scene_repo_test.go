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
	if err := prodRepo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Cosecha: "Maiz", Monitoring: true}); err != nil {
		t.Fatalf("setting up production: %v", err)
	}
	return produccionID
}

// setupProductionWithMonitoringID creates a production and returns (produccionID, monitoringID).
func setupProductionWithMonitoringID(t *testing.T, ctx context.Context, prodRepo *ProductionRepo) (int64, uint) {
	t.Helper()
	produccionID := setupProduction(t, ctx, prodRepo)
	prod, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil || prod == nil {
		t.Fatalf("get production for monitoringID: %v", err)
	}
	return produccionID, prod.ID
}

func TestSceneRepo_UpsertAndGet(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	produccionID, monitoringID := setupProductionWithMonitoringID(t, ctx, prodRepo)
	sceneName := "S2A_TEST_SCENE_1"
	fecha := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	s := &domain.Scene{
		MonitoringProduccionID: monitoringID,
		SceneName:              sceneName,
		Fecha:                  &fecha,
		Status:                 domain.StatusPending,
	}

	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByProduccionAndSceneName(ctx, produccionID, sceneName)
	if err != nil {
		t.Fatalf("GetByProduccionAndSceneName: %v", err)
	}
	if got == nil {
		t.Fatal("expected scene, got nil")
	}
	if got.Status != domain.StatusPending {
		t.Errorf("expected status PENDING, got %s", got.Status)
	}

	byID, err := repo.GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID == nil || byID.SceneName != sceneName {
		t.Errorf("unexpected GetByID result: %+v", byID)
	}
}

func TestSceneRepo_UpsertNoDuplicate(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	produccionID, monitoringID := setupProductionWithMonitoringID(t, ctx, prodRepo)
	sceneName := "S2A_TEST_SCENE_DUP"
	fecha := time.Now().UTC()

	s := &domain.Scene{MonitoringProduccionID: monitoringID, SceneName: sceneName, Fecha: &fecha, Status: domain.StatusPending}
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM s3_monitoring_escenas WHERE s3_monitoring_produccion_id = ? AND scene_name = ?", monitoringID, sceneName).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	_ = produccionID // used to derive monitoringID
}

func TestSceneRepo_UpdateStatusAndFailed(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	_, monitoringID := setupProductionWithMonitoringID(t, ctx, prodRepo)
	fecha := time.Now().UTC()
	s := &domain.Scene{MonitoringProduccionID: monitoringID, SceneName: "S2A_STATUS_TEST", Fecha: &fecha, Status: domain.StatusPending}
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByID(ctx, s.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID after upsert: %v", err)
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

	if err := repo.SetFailed(ctx, got.ID); err != nil {
		t.Fatalf("SetFailed: %v", err)
	}
	got, err = repo.GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.StatusFailed {
		t.Errorf("expected FAILED, got %s", got.Status)
	}
}

func TestSceneRepo_GetPreviousUsableScene(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	_, monitoringID := setupProductionWithMonitoringID(t, ctx, prodRepo)

	olderFecha := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	newerFecha := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	older := &domain.Scene{MonitoringProduccionID: monitoringID, SceneName: "OLDER", Fecha: &olderFecha, Status: domain.StatusCompleted, Usable: true}
	newerInvalid := &domain.Scene{MonitoringProduccionID: monitoringID, SceneName: "NEWER_INVALID", Fecha: &newerFecha, Status: domain.StatusCompleted, Usable: false}
	target := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	if err := repo.Upsert(ctx, older); err != nil {
		t.Fatalf("Upsert older: %v", err)
	}
	if err := repo.Upsert(ctx, newerInvalid); err != nil {
		t.Fatalf("Upsert newerInvalid: %v", err)
	}

	got, err := repo.GetPreviousUsableScene(ctx, monitoringID, target)
	if err != nil {
		t.Fatalf("GetPreviousUsableScene: %v", err)
	}
	if got == nil {
		t.Fatal("expected a previous usable scene, got nil")
	}
	if got.SceneName != "OLDER" {
		t.Errorf("expected OLDER scene, got %s", got.SceneName)
	}
}

func TestSceneRepo_ListByMonitoringProduccion(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	_, monitoringID := setupProductionWithMonitoringID(t, ctx, prodRepo)

	for i, name := range []string{"LIST_A", "LIST_B"} {
		fecha := time.Date(2026, 8, i+1, 0, 0, 0, 0, time.UTC)
		s := &domain.Scene{
			MonitoringProduccionID: monitoringID,
			SceneName:              name,
			Fecha:                  &fecha,
			Status:                 domain.StatusPending,
		}
		if err := repo.Upsert(ctx, s); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	list, err := repo.ListByMonitoringProduccion(ctx, monitoringID)
	if err != nil {
		t.Fatalf("ListByMonitoringProduccion: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(list))
	}
}
