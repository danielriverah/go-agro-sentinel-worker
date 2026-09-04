package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// TestIntegration_ProductionSyncCycle tests the complete production sync cycle:
// Create production -> sync DynamoDB -> get production
func TestIntegration_ProductionSyncCycle(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	ctx := context.Background()

	// Step 1: Create a production (simulating sync from DynamoDB)
	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID:     produccionID,
		ArticuloID:       1001,
		CentroCostoID:    2001,
		NombreRancho:     "Estancia Los Andes",
		Cultivo:          "Maiz",
		Ciclo:            "2026-A",
		BBox:             &domain.BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
		Monitoring:       true,
		TargetResolution: 10,
		CloudCoverMax:    20.0,
	}

	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Step 1 - Upsert production: %v", err)
	}

	// Step 2: Retrieve the production to verify sync
	got, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("Step 2 - GetByProduccionID: %v", err)
	}
	if got == nil {
		t.Fatal("Step 2 - production should exist")
	}

	// Verify all critical fields
	if got.ProduccionID != produccionID {
		t.Errorf("ProduccionID mismatch: got %d, want %d", got.ProduccionID, produccionID)
	}
	if got.ArticuloID != 1001 {
		t.Errorf("ArticuloID mismatch: got %d, want %d", got.ArticuloID, 1001)
	}
	if got.CentroCostoID != 2001 {
		t.Errorf("CentroCostoID mismatch: got %d, want %d", got.CentroCostoID, 2001)
	}
	if got.NombreRancho != "Estancia Los Andes" {
		t.Errorf("NombreRancho mismatch: got %q, want %q", got.NombreRancho, "Estancia Los Andes")
	}
	if got.Cultivo != "Maiz" {
		t.Errorf("Cultivo mismatch: got %q, want %q", got.Cultivo, "Maiz")
	}
	if !got.Monitoring {
		t.Error("Monitoring should be true")
	}
	if got.BBox == nil {
		t.Error("BBox should not be nil")
	} else {
		if got.BBox.MinX != -102.35 || got.BBox.MaxY != 21.85 {
			t.Errorf("BBox mismatch: got %+v", got.BBox)
		}
	}

	// Step 3: List by articulo_id
	results, err := prodRepo.GetByArticuloID(ctx, 1001)
	if err != nil {
		t.Fatalf("Step 3 - GetByArticuloID: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Step 3 - should find production by articulo_id")
	}

	// Step 4: List by centro_costo_id
	results, err = prodRepo.GetByCentroCostoID(ctx, 2001)
	if err != nil {
		t.Fatalf("Step 4 - GetByCentroCostoID: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Step 4 - should find production by centro_costo_id")
	}

	t.Log("Integration test: ProductionSyncCycle passed")
}

// TestIntegration_SceneProcessingAndIAAnalysis tests the complete processing cycle:
// Create escena -> process and get result -> save IA analysis -> retrieve results
func TestIntegration_SceneProcessingAndIAAnalysis(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// Step 1: Create production
	produccionID := randomID()
	prod := &domain.Production{
		ProduccionID:     produccionID,
		ArticuloID:       1002,
		CentroCostoID:    2002,
		NombreRancho:     "Hacienda Central",
		Cultivo:          "Soja",
		Ciclo:            "2026-B",
		BBox:             &domain.BBox{MinX: -60.1, MinY: -34.5, MaxX: -60.0, MaxY: -34.4},
		Monitoring:       true,
		TargetResolution: 10,
		CloudCoverMax:    15.0,
	}
	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Step 1 - Upsert production: %v", err)
	}

	// Step 2: Create escena (scene capture)
	sceneIDStr := fmt.Sprintf("SCENE_%d", randomID())
	escena := &domain.Scene{
		ProduccionID:    produccionID,
		SceneID:         sceneIDStr,
		SceneDate:       time.Now().UTC().Add(-24 * time.Hour),
		CloudCoverScene: 8.0,
		PassesQuality:   true,
		HasMultiband:    true,
		HasParams:       true,
		HasRGB:          true,
		HasAnalisis:     false,
		Status:          domain.StatusPending,
	}
	if err := sceneRepo.Upsert(ctx, escena); err != nil {
		t.Fatalf("Step 2 - Upsert scene: %v", err)
	}

	// Step 3: Get escena to verify creation
	gotEscena, err := sceneRepo.GetByProduccionAndSceneID(ctx, produccionID, sceneIDStr)
	if err != nil {
		t.Fatalf("Step 3 - GetByProduccionAndSceneID: %v", err)
	}
	if gotEscena == nil {
		t.Fatal("Step 3 - escena should exist")
	}

	// Step 4: Process escena and generate IA analysis result
	fechaAnalisis := time.Now().UTC()
	iaResult := &domain.IAResultSummary{
		S3MonitoringEscenaID: escena.ID,
		EstadoClave:          "alerta",
		EstadoGeneral:        "Se detectó potencial estrés hídrico",
		RiesgoNivel:          "medio",
		RiesgoMotivo:         "NDVI indica déficit de agua",
		FechaAnalisis:        &fechaAnalisis,
		JSONOriginal:         `{"ndvi": 0.65, "ndbi": -0.05, "estado": "alerta"}`,
	}
	if err := iaRepo.Upsert(ctx, iaResult); err != nil {
		t.Fatalf("Step 4 - Upsert IA result: %v", err)
	}

	// Step 5: Retrieve IA result by escena
	gotIA, err := iaRepo.GetByEscenaID(ctx, escena.ID)
	if err != nil {
		t.Fatalf("Step 5 - GetByEscenaID: %v", err)
	}
	if gotIA == nil {
		t.Fatal("Step 5 - IA result should exist")
	}
	if gotIA.EstadoClave != "alerta" {
		t.Errorf("EstadoClave mismatch: got %q, want %q", gotIA.EstadoClave, "alerta")
	}
	if gotIA.RiesgoNivel != "medio" {
		t.Errorf("RiesgoNivel mismatch: got %q, want %q", gotIA.RiesgoNivel, "medio")
	}

	// Step 6: List all IA results for production
	results, err := iaRepo.ListByProduccion(ctx, produccionID)
	if err != nil {
		t.Fatalf("Step 6 - ListByProduccion: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Step 6 - should find IA results for production")
	}

	// Step 7: Increment escena counters
	if err := prodRepo.IncrementEscenas(ctx, produccionID, true); err != nil {
		t.Fatalf("Step 7 - IncrementEscenas: %v", err)
	}

	// Verify counters
	updatedProd, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("Step 7 - GetByProduccionID: %v", err)
	}
	if updatedProd.TotalEscenas != 1 {
		t.Errorf("TotalEscenas mismatch: got %d, want 1", updatedProd.TotalEscenas)
	}
	if updatedProd.TotalEscenasValidas != 1 {
		t.Errorf("TotalEscenasValidas mismatch: got %d, want 1", updatedProd.TotalEscenasValidas)
	}

	t.Log("Integration test: SceneProcessingAndIAAnalysis passed")
}

// TestIntegration_CompleteMonitoringWorkflow tests the complete monitoring workflow
// with multiple escenas and IA analyses
func TestIntegration_CompleteMonitoringWorkflow(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	sceneRepo := NewSceneRepo(db)
	iaRepo := NewIAResultRepository(db)
	ctx := context.Background()

	// Step 1: Create production
	produccionID := randomID()
	articulo := int64(2001)
	centroCosto := int64(3001)
	ranchName := "Rancho Productivo"

	prod := &domain.Production{
		ProduccionID:     produccionID,
		ArticuloID:       articulo,
		CentroCostoID:    centroCosto,
		NombreRancho:     ranchName,
		Cultivo:          "Trigo",
		Ciclo:            "2026-C",
		BBox:             &domain.BBox{MinX: -65.0, MinY: -35.0, MaxX: -64.9, MaxY: -34.9},
		Monitoring:       true,
		TargetResolution: 10,
		CloudCoverMax:    25.0,
	}
	if err := prodRepo.Upsert(ctx, prod); err != nil {
		t.Fatalf("Upsert production: %v", err)
	}

	// Step 2: Create multiple escenas with IA analyses
	numEscenas := 5
	var escenaIDs []int64

	for i := 0; i < numEscenas; i++ {
		sceneIDStr := fmt.Sprintf("SCENE_%d_%d", randomID(), i)
		// Create escena
		escena := &domain.Scene{
			ProduccionID:    produccionID,
			SceneID:         sceneIDStr,
			SceneDate:       time.Now().UTC().Add(-time.Duration(24-i) * time.Hour),
			CloudCoverScene: float64(5 + i),
			PassesQuality:   true,
			HasMultiband:    true,
			HasParams:       true,
			HasRGB:          true,
			HasAnalisis:     false,
			Status:          domain.StatusPending,
		}
		if err := sceneRepo.Upsert(ctx, escena); err != nil {
			t.Fatalf("Upsert scene %d: %v", i, err)
		}

		escenaIDs = append(escenaIDs, escena.ID)

		// Generate and save IA analysis
		estadoClave := "normal"
		riesgoNivel := "bajo"
		if i%3 == 1 {
			estadoClave = "alerta"
			riesgoNivel = "medio"
		} else if i%3 == 2 {
			estadoClave = "crítico"
			riesgoNivel = "alto"
		}

		fechaAnalisis := time.Now().UTC().Add(-time.Duration(23-i) * time.Hour)
		iaResult := &domain.IAResultSummary{
			S3MonitoringEscenaID: escena.ID,
			EstadoClave:          estadoClave,
			EstadoGeneral:        "Análisis de escena " + string(rune('0'+i)),
			RiesgoNivel:          riesgoNivel,
			RiesgoMotivo:         "Prueba de integración",
			FechaAnalisis:        &fechaAnalisis,
			JSONOriginal:         `{"test": true}`,
		}
		if err := iaRepo.Upsert(ctx, iaResult); err != nil {
			t.Fatalf("Upsert IA result %d: %v", i, err)
		}

		// Increment scene counters
		valid := i%2 == 0 // Mark every other scene as valid
		if err := prodRepo.IncrementEscenas(ctx, produccionID, valid); err != nil {
			t.Fatalf("IncrementEscenas %d: %v", i, err)
		}
	}

	// Step 3: Verify all escenas were created
	allResults, err := iaRepo.ListByProduccion(ctx, produccionID)
	if err != nil {
		t.Fatalf("ListByProduccion: %v", err)
	}
	if len(allResults) != numEscenas {
		t.Errorf("expected %d IA results, got %d", numEscenas, len(allResults))
	}

	// Step 4: Verify scene counters
	updatedProd, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if updatedProd.TotalEscenas != numEscenas {
		t.Errorf("expected total_escenas=%d, got %d", numEscenas, updatedProd.TotalEscenas)
	}
	expectedValidas := (numEscenas + 1) / 2 // 3 out of 5 (0, 2, 4)
	if updatedProd.TotalEscenasValidas != expectedValidas {
		t.Errorf("expected total_escenas_validas=%d, got %d", expectedValidas, updatedProd.TotalEscenasValidas)
	}

	// Step 5: Update production metadata (articulo and centro costo)
	newArticulo := int64(2002)
	newCentroCosto := int64(3002)
	newRanchName := "Rancho Productivo Mejorado"
	if err := prodRepo.UpdateArticuloAndCentro(ctx, produccionID, newArticulo, newCentroCosto, newRanchName); err != nil {
		t.Fatalf("UpdateArticuloAndCentro: %v", err)
	}

	// Verify update
	finalProd, err := prodRepo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID after update: %v", err)
	}
	if finalProd.ArticuloID != newArticulo {
		t.Errorf("ArticuloID not updated: got %d, want %d", finalProd.ArticuloID, newArticulo)
	}
	if finalProd.CentroCostoID != newCentroCosto {
		t.Errorf("CentroCostoID not updated: got %d, want %d", finalProd.CentroCostoID, newCentroCosto)
	}

	// Step 6: Verify search by updated articulo and centro
	byArticulo, err := prodRepo.GetByArticuloID(ctx, newArticulo)
	if err != nil {
		t.Fatalf("GetByArticuloID: %v", err)
	}
	if len(byArticulo) == 0 {
		t.Fatal("should find production by updated articulo_id")
	}

	byCentro, err := prodRepo.GetByCentroCostoID(ctx, newCentroCosto)
	if err != nil {
		t.Fatalf("GetByCentroCostoID: %v", err)
	}
	if len(byCentro) == 0 {
		t.Fatal("should find production by updated centro_costo_id")
	}

	// Step 7: Delete an IA result
	if err := iaRepo.DeleteByEscenaID(ctx, escenaIDs[0]); err != nil {
		t.Fatalf("DeleteByEscenaID: %v", err)
	}

	// Verify deletion
	deleted, err := iaRepo.GetByEscenaID(ctx, escenaIDs[0])
	if err != nil {
		t.Fatalf("GetByEscenaID after delete: %v", err)
	}
	if deleted != nil {
		t.Fatal("IA result should be deleted")
	}

	// Verify count decreased
	remainingResults, err := iaRepo.ListByProduccion(ctx, produccionID)
	if err != nil {
		t.Fatalf("ListByProduccion after delete: %v", err)
	}
	if len(remainingResults) != numEscenas-1 {
		t.Errorf("expected %d IA results after delete, got %d", numEscenas-1, len(remainingResults))
	}

	t.Log("Integration test: CompleteMonitoringWorkflow passed")
}

// TestIntegration_DataValidation tests that validation works across the workflow
func TestIntegration_DataValidation(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	// Test Production validation
	invalidProd := &domain.Production{
		ProduccionID: 0, // Invalid
		Cultivo:      "Maiz",
		Ciclo:        "2026-A",
	}
	if err := invalidProd.Validate(); err == nil {
		t.Error("expected validation error for production with ProduccionID=0")
	}

	// Test IAResultSummary validation
	invalidIA := &domain.IAResultSummary{
		ID:                   0, // Invalid
		S3MonitoringEscenaID: 100,
		EstadoClave:          "normal",
		RiesgoNivel:          "bajo",
	}
	if err := invalidIA.Validate(); err == nil {
		t.Error("expected validation error for IA result with ID=0")
	}

	// Test with invalid EstadoClave
	invalidIA2 := &domain.IAResultSummary{
		ID:                   1,
		S3MonitoringEscenaID: 100,
		EstadoClave:          "invalid_state",
		RiesgoNivel:          "bajo",
	}
	if err := invalidIA2.Validate(); err == nil {
		t.Error("expected validation error for invalid EstadoClave")
	}

	// Test String methods
	validIA := &domain.IAResultSummary{
		ID:                   1,
		S3MonitoringEscenaID: 100,
		EstadoClave:          "normal",
		RiesgoNivel:          "bajo",
	}
	str := validIA.String()
	if str == "" {
		t.Error("String() should not return empty string")
	}

	t.Log("Integration test: DataValidation passed")
}
