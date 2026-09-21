package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// TestIntegration_ProductionSyncCycle tests the complete production sync cycle.
func TestIntegration_ProductionSyncCycle(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID: produccionID,
		Cosecha:      "Maiz",
		Monitoring:   true,
	}

	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}

	got, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got == nil {
		t.Fatal("production should exist")
	}
	if got.ProduccionID != produccionID {
		t.Errorf("ProduccionID mismatch: got %d, want %d", got.ProduccionID, produccionID)
	}
	if !got.Monitoring {
		t.Error("Monitoring should be true")
	}

	t.Log("Integration test: ProductionSyncCycle passed")
}

// TestIntegration_SceneProcessingAndIAAnalysis tests the complete processing cycle.
func TestIntegration_SceneProcessingAndIAAnalysis(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// Step 1: Create production
	produccionID := randomID()
	if err := prodRepo.Upsert(ctx, &domain.Production{
		ProduccionID: produccionID,
		Cosecha:      "Soja",
		Monitoring:   true,
	}); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}
	prod, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil || prod == nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}

	// Step 2: Create escena
	sceneName := fmt.Sprintf("SCENE_%d", randomID())
	fecha := time.Now().UTC().Add(-24 * time.Hour)
	escena := &domain.Scene{
		MonitoringProduccionID: prod.ID,
		SceneName:              sceneName,
		Fecha:                  &fecha,
		Status:                 domain.StatusPending,
	}
	if err := sceneRepo.Upsert(ctx, escena); err != nil {
		t.Fatalf("Upsert scene: %v", err)
	}

	// Step 3: Get escena to verify creation
	gotEscena, err := sceneRepo.GetByProduccionAndSceneName(ctx, produccionID, sceneName)
	if err != nil {
		t.Fatalf("GetByProduccionAndSceneName: %v", err)
	}
	if gotEscena == nil {
		t.Fatal("escena should exist")
	}

	// Step 4: Upsert IA analysis result
	fechaAnalisis := time.Now().UTC()
	iaResult := &domain.IAResultSummary{
		S3MonitoringEscenaID: gotEscena.ID,
		EstadoClave:          "alerta",
		EstadoGeneral:        "Se detectó potencial estrés hídrico",
		RiesgoNivel:          "medio",
		RiesgoMotivo:         "NDVI indica déficit de agua",
		FechaAnalisis:        &fechaAnalisis,
		JSONOriginal:         `{"ndvi": 0.65}`,
	}
	if err := iaRepo.Upsert(ctx, iaResult); err != nil {
		t.Fatalf("Upsert IA result: %v", err)
	}

	// Step 5: Retrieve IA result by escena
	gotIA, err := iaRepo.GetByEscenaID(ctx, gotEscena.ID)
	if err != nil {
		t.Fatalf("GetByEscenaID: %v", err)
	}
	if gotIA == nil {
		t.Fatal("IA result should exist")
	}
	if gotIA.EstadoClave != "alerta" {
		t.Errorf("EstadoClave mismatch: got %q, want alerta", gotIA.EstadoClave)
	}

	// Step 6: List IA results for production
	results, err := iaRepo.ListByMonitoringProduccion(ctx, prod.ID)
	if err != nil {
		t.Fatalf("ListByMonitoringProduccion: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("should find IA results for production")
	}

	t.Log("Integration test: SceneProcessingAndIAAnalysis passed")
}

// TestIntegration_CompleteMonitoringWorkflow tests the complete monitoring workflow.
func TestIntegration_CompleteMonitoringWorkflow(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// Create production
	produccionID := randomID()
	if err := prodRepo.Upsert(ctx, &domain.Production{
		ProduccionID: produccionID,
		Cosecha:      "Trigo",
		Monitoring:   true,
	}); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}
	prod, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil || prod == nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	monitoringID := prod.ID

	numEscenas := 5
	var escenaIDs []uint64

	for i := 0; i < numEscenas; i++ {
		sceneName := fmt.Sprintf("SCENE_%d_%d", randomID(), i)
		fecha := time.Now().UTC().Add(-time.Duration(24-i) * time.Hour)
		escena := &domain.Scene{
			MonitoringProduccionID: monitoringID,
			SceneName:              sceneName,
			Fecha:                  &fecha,
			Status:                 domain.StatusPending,
		}
		if err := sceneRepo.Upsert(ctx, escena); err != nil {
			t.Fatalf("Upsert scene %d: %v", i, err)
		}

		got, err := sceneRepo.GetByProduccionAndSceneName(ctx, produccionID, sceneName)
		if err != nil || got == nil {
			t.Fatalf("GetByProduccionAndSceneName scene %d: %v", i, err)
		}
		escenaIDs = append(escenaIDs, got.ID)

		estadoClave := "normal"
		riesgoNivel := "bajo"
		if i%3 == 1 {
			estadoClave = "alerta"
			riesgoNivel = "medio"
		} else if i%3 == 2 {
			estadoClave = "critico"
			riesgoNivel = "alto"
		}

		fechaAnalisis := time.Now().UTC()
		if err := iaRepo.Upsert(ctx, &domain.IAResultSummary{
			S3MonitoringEscenaID: got.ID,
			EstadoClave:          estadoClave,
			EstadoGeneral:        "Análisis " + fmt.Sprintf("%d", i),
			RiesgoNivel:          riesgoNivel,
			RiesgoMotivo:         "Prueba de integración",
			FechaAnalisis:        &fechaAnalisis,
			JSONOriginal:         `{"test": true}`,
		}); err != nil {
			t.Fatalf("Upsert IA result %d: %v", i, err)
		}
	}

	// Verify all IA results were created
	allResults, err := iaRepo.ListByMonitoringProduccion(ctx, monitoringID)
	if err != nil {
		t.Fatalf("ListByMonitoringProduccion: %v", err)
	}
	if len(allResults) != numEscenas {
		t.Errorf("expected %d IA results, got %d", numEscenas, len(allResults))
	}

	// Delete an IA result
	if err := iaRepo.DeleteByEscenaID(ctx, escenaIDs[0]); err != nil {
		t.Fatalf("DeleteByEscenaID: %v", err)
	}

	deleted, err := iaRepo.GetByEscenaID(ctx, escenaIDs[0])
	if err != nil {
		t.Fatalf("GetByEscenaID after delete: %v", err)
	}
	if deleted != nil {
		t.Fatal("IA result should be deleted")
	}

	remaining, err := iaRepo.ListByMonitoringProduccion(ctx, monitoringID)
	if err != nil {
		t.Fatalf("ListByMonitoringProduccion after delete: %v", err)
	}
	if len(remaining) != numEscenas-1 {
		t.Errorf("expected %d IA results after delete, got %d", numEscenas-1, len(remaining))
	}

	t.Log("Integration test: CompleteMonitoringWorkflow passed")
}

// TestIntegration_DataValidation tests that validation works across the workflow.
func TestIntegration_DataValidation(t *testing.T) {
	if err := (&domain.Production{ProduccionID: 0}).Validate(); err == nil {
		t.Error("expected validation error for ProduccionID=0")
	}

	if err := (&domain.IAResultSummary{ID: 0, S3MonitoringEscenaID: 100, EstadoClave: "normal"}).Validate(); err == nil {
		t.Error("expected validation error for IA result with ID=0")
	}

	if err := (&domain.IAResultSummary{ID: 1, S3MonitoringEscenaID: 100, EstadoClave: "invalid_state"}).Validate(); err == nil {
		t.Error("expected validation error for invalid EstadoClave")
	}

	t.Log("Integration test: DataValidation passed")
}
