package database

import (
	"context"
	"testing"

	"agro-sentinel-worker/internal/domain"
)

func TestProductionRepo_UpsertAndGet(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	p := &domain.Production{
		ProduccionID:     produccionID,
		Cultivo:          "Maiz",
		Ciclo:            "2026-A",
		BBox:             &domain.BBox{MinX: -60.1, MinY: -34.5, MaxX: -60.0, MaxY: -34.4},
		Monitoring:       true,
		TargetResolution: 10,
		CloudCoverMax:    23.0,
	}

	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got == nil {
		t.Fatal("expected production, got nil")
	}
	if got.Cultivo != "Maiz" || got.Ciclo != "2026-A" {
		t.Errorf("unexpected fields: %+v", got)
	}
	if got.BBox == nil || got.BBox.MinX != -60.1 {
		t.Errorf("unexpected bbox: %+v", got.BBox)
	}
	if !got.Monitoring {
		t.Error("expected monitoring true")
	}
}

func TestProductionRepo_UpsertNoDuplicate(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	p := &domain.Production{ProduccionID: produccionID, Cultivo: "Soja", Monitoring: true}

	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}

	p.Cultivo = "Soja actualizada"
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM s3_monitoring_producciones WHERE produccion_id = ?", produccionID).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got.Cultivo != "Soja actualizada" {
		t.Errorf("expected updated cultivo, got %q", got.Cultivo)
	}
}

func TestProductionRepo_SetBloqueadoAndDesbloquear(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	p := &domain.Production{ProduccionID: produccionID, Cultivo: "Trigo", Monitoring: true}
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := repo.SetBloqueado(ctx, produccionID, "manual hold"); err != nil {
		t.Fatalf("SetBloqueado: %v", err)
	}

	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if !got.Bloqueado {
		t.Fatal("expected bloqueado true")
	}
	if got.BloqueadoMotivo != "manual hold" {
		t.Errorf("unexpected motivo: %q", got.BloqueadoMotivo)
	}

	if err := repo.Desbloquear(ctx, produccionID, "operator1"); err != nil {
		t.Fatalf("Desbloquear: %v", err)
	}

	got, err = repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got.Bloqueado {
		t.Fatal("expected bloqueado false after desbloquear")
	}
	if got.DesbloqueadoPor != "operator1" {
		t.Errorf("unexpected desbloqueado_por: %q", got.DesbloqueadoPor)
	}
}

func TestProductionRepo_ListActive(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	activeID := randomID()
	inactiveID := randomID()

	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: activeID, Cultivo: "Activo", Monitoring: true}); err != nil {
		t.Fatalf("Upsert active: %v", err)
	}
	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: inactiveID, Cultivo: "Inactivo", Monitoring: false}); err != nil {
		t.Fatalf("Upsert inactive: %v", err)
	}

	list, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	var foundActive, foundInactive bool
	for _, p := range list {
		if p.ProduccionID == activeID {
			foundActive = true
		}
		if p.ProduccionID == inactiveID {
			foundInactive = true
		}
	}
	if !foundActive {
		t.Error("expected active production in list")
	}
	if foundInactive {
		t.Error("did not expect inactive production in list")
	}
}

func TestProductionRepo_IncrementEscenas(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Monitoring: true}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := repo.IncrementEscenas(ctx, produccionID, true); err != nil {
		t.Fatalf("IncrementEscenas valid: %v", err)
	}
	if err := repo.IncrementEscenas(ctx, produccionID, false); err != nil {
		t.Fatalf("IncrementEscenas invalid: %v", err)
	}

	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got.TotalEscenas != 2 {
		t.Errorf("expected total_escenas=2, got %d", got.TotalEscenas)
	}
	if got.TotalEscenasValidas != 1 {
		t.Errorf("expected total_escenas_validas=1, got %d", got.TotalEscenasValidas)
	}
}

func TestProductionRepo_UpsertWithNewFields(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	p := &domain.Production{
		ProduccionID:     produccionID,
		ArticuloID:       12345,
		CentroCostoID:    67890,
		NombreRancho:     "Rancho El Dorado",
		Cultivo:          "Maiz",
		Ciclo:            "2026-A",
		BBox:             &domain.BBox{MinX: -60.1, MinY: -34.5, MaxX: -60.0, MaxY: -34.4},
		Monitoring:       true,
		TargetResolution: 10,
		CloudCoverMax:    23.0,
	}

	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got == nil {
		t.Fatal("expected production, got nil")
	}
	if got.ArticuloID != 12345 {
		t.Errorf("unexpected articulo_id: got %d, want %d", got.ArticuloID, 12345)
	}
	if got.CentroCostoID != 67890 {
		t.Errorf("unexpected centro_costo_id: got %d, want %d", got.CentroCostoID, 67890)
	}
	if got.NombreRancho != "Rancho El Dorado" {
		t.Errorf("unexpected nombre_rancho: got %q, want %q", got.NombreRancho, "Rancho El Dorado")
	}
}

func TestProductionRepo_GetByArticuloID(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	articuloID := int64(12345)

	// Create multiple productions with the same articulo_id
	prodID1 := randomID()
	if err := repo.Upsert(ctx, &domain.Production{
		ProduccionID: prodID1,
		ArticuloID:   articuloID,
		Cultivo:      "Maiz",
		Ciclo:        "2026-A",
	}); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}

	prodID2 := randomID()
	if err := repo.Upsert(ctx, &domain.Production{
		ProduccionID: prodID2,
		ArticuloID:   articuloID,
		Cultivo:      "Soja",
		Ciclo:        "2026-B",
	}); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	// Create production with different articulo_id
	prodID3 := randomID()
	if err := repo.Upsert(ctx, &domain.Production{
		ProduccionID: prodID3,
		ArticuloID:   99999,
		Cultivo:      "Trigo",
		Ciclo:        "2026-C",
	}); err != nil {
		t.Fatalf("Upsert 3: %v", err)
	}

	// List by articulo_id
	results, err := repo.GetByArticuloID(ctx, articuloID)
	if err != nil {
		t.Fatalf("GetByArticuloID: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Verify correct productions returned
	var found1, found2 bool
	for _, p := range results {
		if p.ProduccionID == prodID1 {
			found1 = true
		}
		if p.ProduccionID == prodID2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Error("expected both productions in results")
	}
}

func TestProductionRepo_GetByArticuloID_Empty(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	nonExistentArticuloID := int64(999999)
	results, err := repo.GetByArticuloID(ctx, nonExistentArticuloID)
	if err != nil {
		t.Fatalf("GetByArticuloID: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestProductionRepo_GetByCentroCostoID(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	centroCostoID := int64(67890)

	// Create multiple productions with the same centro_costo_id
	prodID1 := randomID()
	if err := repo.Upsert(ctx, &domain.Production{
		ProduccionID:  prodID1,
		CentroCostoID: centroCostoID,
		Cultivo:       "Maiz",
		Ciclo:         "2026-A",
	}); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}

	prodID2 := randomID()
	if err := repo.Upsert(ctx, &domain.Production{
		ProduccionID:  prodID2,
		CentroCostoID: centroCostoID,
		Cultivo:       "Soja",
		Ciclo:         "2026-B",
	}); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	// Create production with different centro_costo_id
	prodID3 := randomID()
	if err := repo.Upsert(ctx, &domain.Production{
		ProduccionID:  prodID3,
		CentroCostoID: 11111,
		Cultivo:       "Trigo",
		Ciclo:         "2026-C",
	}); err != nil {
		t.Fatalf("Upsert 3: %v", err)
	}

	// List by centro_costo_id
	results, err := repo.GetByCentroCostoID(ctx, centroCostoID)
	if err != nil {
		t.Fatalf("GetByCentroCostoID: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Verify correct productions returned
	var found1, found2 bool
	for _, p := range results {
		if p.ProduccionID == prodID1 {
			found1 = true
		}
		if p.ProduccionID == prodID2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Error("expected both productions in results")
	}
}

func TestProductionRepo_GetByCentroCostoID_Empty(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	nonExistentCentroCostoID := int64(999999)
	results, err := repo.GetByCentroCostoID(ctx, nonExistentCentroCostoID)
	if err != nil {
		t.Fatalf("GetByCentroCostoID: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestProductionRepo_UpdateArticuloAndCentro(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	p := &domain.Production{
		ProduccionID:  produccionID,
		ArticuloID:    100,
		CentroCostoID: 200,
		NombreRancho:  "Rancho A",
		Cultivo:       "Maiz",
		Ciclo:         "2026-A",
	}

	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Update articulo and centro
	if err := repo.UpdateArticuloAndCentro(ctx, produccionID, 500, 600, "Rancho B"); err != nil {
		t.Fatalf("UpdateArticuloAndCentro: %v", err)
	}

	// Verify update
	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got.ArticuloID != 500 {
		t.Errorf("expected updated articulo_id=500, got %d", got.ArticuloID)
	}
	if got.CentroCostoID != 600 {
		t.Errorf("expected updated centro_costo_id=600, got %d", got.CentroCostoID)
	}
	if got.NombreRancho != "Rancho B" {
		t.Errorf("expected updated nombre_rancho='Rancho B', got %q", got.NombreRancho)
	}
}
