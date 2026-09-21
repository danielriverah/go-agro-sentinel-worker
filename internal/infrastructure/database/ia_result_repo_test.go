package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// setupSceneForIA creates a production + scene and returns (monitoringProduccionID, sceneID uint64)
func setupSceneForIA(t *testing.T, ctx context.Context, prodRepo *ProductionRepo, sceneRepo *SceneRepo) (uint, uint64) {
	t.Helper()
	produccionID := randomID()
	if err := prodRepo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Cosecha: "Maiz", Monitoring: true}); err != nil {
		t.Fatalf("setup production: %v", err)
	}
	prod, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil || prod == nil {
		t.Fatalf("get production: %v", err)
	}

	sceneName := fmt.Sprintf("SCENE_%d", randomID())
	fecha := time.Now().UTC().Add(-24 * time.Hour)
	s := &domain.Scene{
		MonitoringProduccionID: prod.ID,
		SceneName:              sceneName,
		Fecha:                  &fecha,
		Status:                 domain.StatusPending,
	}
	if err := sceneRepo.Upsert(ctx, s); err != nil {
		t.Fatalf("setup scene: %v", err)
	}
	got, err := sceneRepo.GetByProduccionAndSceneName(ctx, produccionID, sceneName)
	if err != nil || got == nil {
		t.Fatalf("fetch scene: %v", err)
	}
	return prod.ID, got.ID
}

func TestIAResultRepository_UpsertAndGet(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	_, sceneID := setupSceneForIA(t, ctx, prodRepo, sceneRepo)

	fechaAnalisis := time.Now().UTC()
	result := &domain.IAResultSummary{
		S3MonitoringEscenaID: sceneID,
		EstadoClave:          "normal",
		EstadoGeneral:        "Cultivo en buen estado",
		RiesgoNivel:          "bajo",
		RiesgoMotivo:         "Sin riesgos detectados",
		FechaAnalisis:        &fechaAnalisis,
		JSONOriginal:         `{"status":"ok"}`,
	}

	if err := iaRepo.Upsert(ctx, result); err != nil {
		t.Fatalf("Upsert ia result: %v", err)
	}

	got, err := iaRepo.GetByEscenaID(ctx, sceneID)
	if err != nil {
		t.Fatalf("GetByEscenaID: %v", err)
	}
	if got == nil {
		t.Fatal("expected IA result, got nil")
	}
	if got.S3MonitoringEscenaID != sceneID {
		t.Errorf("unexpected escena id: got %d, want %d", got.S3MonitoringEscenaID, sceneID)
	}
	if got.EstadoClave != "normal" {
		t.Errorf("unexpected estado clave: got %q, want %q", got.EstadoClave, "normal")
	}
	if got.RiesgoNivel != "bajo" {
		t.Errorf("unexpected riesgo nivel: got %q, want %q", got.RiesgoNivel, "bajo")
	}
}

func TestIAResultRepository_UpsertUpdate(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	_, sceneID := setupSceneForIA(t, ctx, prodRepo, sceneRepo)

	result := &domain.IAResultSummary{
		S3MonitoringEscenaID: sceneID,
		EstadoClave:          "normal",
		RiesgoNivel:          "bajo",
	}
	if err := iaRepo.Upsert(ctx, result); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}

	result.EstadoClave = "alerta"
	result.RiesgoNivel = "medio"
	if err := iaRepo.Upsert(ctx, result); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	got, err := iaRepo.GetByEscenaID(ctx, sceneID)
	if err != nil {
		t.Fatalf("GetByEscenaID: %v", err)
	}
	if got.EstadoClave != "alerta" {
		t.Errorf("expected updated estado clave, got %q", got.EstadoClave)
	}
	if got.RiesgoNivel != "medio" {
		t.Errorf("expected updated riesgo nivel, got %q", got.RiesgoNivel)
	}
}

func TestIAResultRepository_GetByEscenaID_NotFound(t *testing.T) {
	db := testDB(t)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	got, err := iaRepo.GetByEscenaID(ctx, uint64(randomID()))
	if err != nil {
		t.Fatalf("GetByEscenaID: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for non-existent escena")
	}
}

func TestIAResultRepository_ListByMonitoringProduccion(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	monitoringID, _ := setupSceneForIA(t, ctx, prodRepo, sceneRepo)

	// Add two more scenes for same production
	var sceneIDs []uint64
	for i := 0; i < 2; i++ {
		sceneName := fmt.Sprintf("SCENE_EXTRA_%d_%d", randomID(), i)
		fecha := time.Now().UTC().Add(-time.Duration(i+1) * time.Hour)
		s := &domain.Scene{
			MonitoringProduccionID: monitoringID,
			SceneName:              sceneName,
			Fecha:                  &fecha,
			Status:                 domain.StatusPending,
		}
		if err := sceneRepo.Upsert(ctx, s); err != nil {
			t.Fatalf("Upsert scene %d: %v", i, err)
		}
		// Get the ID
		got, err := sceneRepo.GetByID(ctx, s.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil {
			// id was set by Upsert if LastInsertId worked
			got = s
		}
		sceneIDs = append(sceneIDs, s.ID)

		result := &domain.IAResultSummary{
			S3MonitoringEscenaID: s.ID,
			EstadoClave:          "normal",
			RiesgoNivel:          "bajo",
		}
		if err := iaRepo.Upsert(ctx, result); err != nil {
			t.Fatalf("Upsert ia result %d: %v", i, err)
		}
	}

	results, err := iaRepo.ListByMonitoringProduccion(ctx, monitoringID)
	if err != nil {
		t.Fatalf("ListByMonitoringProduccion: %v", err)
	}
	// At least the 2 extra scenes we added have IA results
	if len(results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(results))
	}
}

func TestIAResultRepository_DeleteByEscenaID(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	_, sceneID := setupSceneForIA(t, ctx, prodRepo, sceneRepo)

	result := &domain.IAResultSummary{
		S3MonitoringEscenaID: sceneID,
		EstadoClave:          "normal",
		RiesgoNivel:          "bajo",
	}
	if err := iaRepo.Upsert(ctx, result); err != nil {
		t.Fatalf("Upsert ia result: %v", err)
	}

	got, err := iaRepo.GetByEscenaID(ctx, sceneID)
	if err != nil {
		t.Fatalf("GetByEscenaID before delete: %v", err)
	}
	if got == nil {
		t.Fatal("expected result to exist before delete")
	}

	if err := iaRepo.DeleteByEscenaID(ctx, sceneID); err != nil {
		t.Fatalf("DeleteByEscenaID: %v", err)
	}

	got, err = iaRepo.GetByEscenaID(ctx, sceneID)
	if err != nil {
		t.Fatalf("GetByEscenaID after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected result to be deleted")
	}
}

func TestIAResultRepository_DeleteByEscenaID_NotFound(t *testing.T) {
	db := testDB(t)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	if err := iaRepo.DeleteByEscenaID(ctx, uint64(randomID())); err != nil {
		t.Fatalf("DeleteByEscenaID for non-existent: %v", err)
	}
}
