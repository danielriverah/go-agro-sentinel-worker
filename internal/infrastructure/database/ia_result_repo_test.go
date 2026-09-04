package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

func TestIAResultRepository_UpsertAndGet(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// First create a production
	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID:     produccionID,
		Cultivo:          "Maiz",
		Ciclo:            "2026-A",
		BBox:             &domain.BBox{MinX: -60.1, MinY: -34.5, MaxX: -60.0, MaxY: -34.4},
		Monitoring:       true,
		TargetResolution: 10,
		CloudCoverMax:    23.0,
	}
	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}

	// Create an escena for this production
	sceneIDStr := fmt.Sprintf("SCENE_%d", randomID())
	scene := &domain.Scene{
		ProduccionID:    produccionID,
		SceneID:         sceneIDStr,
		SceneDate:       time.Now().UTC().Add(-24 * time.Hour),
		CloudCoverScene: 5.0,
		PassesQuality:   true,
		HasMultiband:    true,
		HasParams:       true,
		HasRGB:          true,
		HasAnalisis:     false,
		Status:          domain.StatusPending,
	}
	if err := sceneRepo.Upsert(ctx, scene); err != nil {
		t.Fatalf("Upsert scene: %v", err)
	}

	// Upsert an IA result for this escena
	fechaAnalisis := time.Now().UTC()
	result := &domain.IAResultSummary{
		S3MonitoringEscenaID: scene.ID,
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

	// Get the result back
	got, err := iaRepo.GetByEscenaID(ctx, scene.ID)
	if err != nil {
		t.Fatalf("GetByEscenaID: %v", err)
	}
	if got == nil {
		t.Fatal("expected IA result, got nil")
	}
	if got.S3MonitoringEscenaID != scene.ID {
		t.Errorf("unexpected escena id: got %d, want %d", got.S3MonitoringEscenaID, scene.ID)
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

	// Create production and scene
	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID: produccionID,
		Cultivo:      "Soja",
		Ciclo:        "2026-B",
		Monitoring:   true,
	}
	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}

	sceneIDStr := fmt.Sprintf("SCENE_%d", randomID())
	scene := &domain.Scene{
		ProduccionID:    produccionID,
		SceneID:         sceneIDStr,
		SceneDate:       time.Now().UTC(),
		CloudCoverScene: 5.0,
		Status:          domain.StatusPending,
	}
	if err := sceneRepo.Upsert(ctx, scene); err != nil {
		t.Fatalf("Upsert scene: %v", err)
	}

	// First insert
	result := &domain.IAResultSummary{
		S3MonitoringEscenaID: scene.ID,
		EstadoClave:          "normal",
		RiesgoNivel:          "bajo",
	}
	if err := iaRepo.Upsert(ctx, result); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}

	// Update
	result.EstadoClave = "alerta"
	result.RiesgoNivel = "medio"
	if err := iaRepo.Upsert(ctx, result); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	// Verify update
	got, err := iaRepo.GetByEscenaID(ctx, scene.ID)
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

	nonExistentEscenaID := randomID()
	got, err := iaRepo.GetByEscenaID(ctx, nonExistentEscenaID)
	if err != nil {
		t.Fatalf("GetByEscenaID: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for non-existent escena")
	}
}

func TestIAResultRepository_ListByProduccion(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// Create production
	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID: produccionID,
		Cultivo:      "Trigo",
		Ciclo:        "2026-C",
		Monitoring:   true,
	}
	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}

	// Create multiple escenas with IA results
	var escenaIDs []int64
	for i := 0; i < 3; i++ {
		sceneIDStr := fmt.Sprintf("SCENE_%d_%d", randomID(), i)
		scene := &domain.Scene{
			ProduccionID:    produccionID,
			SceneID:         sceneIDStr,
			SceneDate:       time.Now().UTC().Add(-time.Duration(i) * time.Hour),
			CloudCoverScene: 5.0,
			Status:          domain.StatusPending,
		}
		if err := sceneRepo.Upsert(ctx, scene); err != nil {
			t.Fatalf("Upsert scene %d: %v", i, err)
		}

		escenaIDs = append(escenaIDs, scene.ID)

		result := &domain.IAResultSummary{
			S3MonitoringEscenaID: scene.ID,
			EstadoClave:          "normal",
			RiesgoNivel:          "bajo",
		}
		if err := iaRepo.Upsert(ctx, result); err != nil {
			t.Fatalf("Upsert ia result %d: %v", i, err)
		}
	}

	// List results for this production
	results, err := iaRepo.ListByProduccion(ctx, produccionID)
	if err != nil {
		t.Fatalf("ListByProduccion: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Verify all escenas are present
	foundEscenas := make(map[int64]bool)
	for _, r := range results {
		foundEscenas[r.S3MonitoringEscenaID] = true
	}
	for _, escenaID := range escenaIDs {
		if !foundEscenas[escenaID] {
			t.Errorf("expected escena %d in results", escenaID)
		}
	}
}

func TestIAResultRepository_ListByProduccion_Empty(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// Create production without any escenas
	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID: produccionID,
		Cultivo:      "Cebada",
		Ciclo:        "2026-D",
		Monitoring:   true,
	}
	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}

	results, err := iaRepo.ListByProduccion(ctx, produccionID)
	if err != nil {
		t.Fatalf("ListByProduccion: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestIAResultRepository_DeleteByEscenaID(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// Setup
	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID: produccionID,
		Cultivo:      "Avena",
		Ciclo:        "2026-E",
		Monitoring:   true,
	}
	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}

	sceneIDStr := fmt.Sprintf("SCENE_%d", randomID())
	scene := &domain.Scene{
		ProduccionID:    produccionID,
		SceneID:         sceneIDStr,
		SceneDate:       time.Now().UTC(),
		CloudCoverScene: 5.0,
		Status:          domain.StatusPending,
	}
	if err := sceneRepo.Upsert(ctx, scene); err != nil {
		t.Fatalf("Upsert scene: %v", err)
	}

	result := &domain.IAResultSummary{
		S3MonitoringEscenaID: scene.ID,
		EstadoClave:          "normal",
		RiesgoNivel:          "bajo",
	}
	if err := iaRepo.Upsert(ctx, result); err != nil {
		t.Fatalf("Upsert ia result: %v", err)
	}

	// Verify it exists
	got, err := iaRepo.GetByEscenaID(ctx, scene.ID)
	if err != nil {
		t.Fatalf("GetByEscenaID before delete: %v", err)
	}
	if got == nil {
		t.Fatal("expected result to exist before delete")
	}

	// Delete
	if err := iaRepo.DeleteByEscenaID(ctx, scene.ID); err != nil {
		t.Fatalf("DeleteByEscenaID: %v", err)
	}

	// Verify it's deleted
	got, err = iaRepo.GetByEscenaID(ctx, scene.ID)
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

	// Delete non-existent result should not error
	nonExistentEscenaID := randomID()
	if err := iaRepo.DeleteByEscenaID(ctx, nonExistentEscenaID); err != nil {
		t.Fatalf("DeleteByEscenaID for non-existent: %v", err)
	}
}
