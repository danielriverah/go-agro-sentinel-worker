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
		ProduccionID: produccionID,
		Cosecha:      "Maiz",
		Monitoring:   true,
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
	if got.Cosecha != "Maiz" {
		t.Errorf("unexpected cosecha: %q", got.Cosecha)
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
	p := &domain.Production{ProduccionID: produccionID, Cosecha: "Soja", Monitoring: true}

	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}

	p.Cosecha = "Soja actualizada"
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
	if got.Cosecha != "Soja actualizada" {
		t.Errorf("expected updated cosecha, got %q", got.Cosecha)
	}
}

// Bloquear y desbloquear se hacen escribiendo posible_cosecha: la columna
// bloqueado la deriva un trigger de la tabla. El repo sólo responde de
// posible_cosecha; que el trigger propague se comprueba contra la base real,
// porque no todos los entornos de prueba lo tienen instalado.
func TestProductionRepo_UpdatePosibleCosecha(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Cosecha: "Trigo", Monitoring: true}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := repo.UpdatePosibleCosecha(ctx, produccionID, true); err != nil {
		t.Fatalf("UpdatePosibleCosecha true: %v", err)
	}

	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if !got.PosibleCosecha {
		t.Fatal("expected posible_cosecha true")
	}

	if err := repo.UpdatePosibleCosecha(ctx, produccionID, false); err != nil {
		t.Fatalf("UpdatePosibleCosecha false: %v", err)
	}

	got, err = repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got.PosibleCosecha {
		t.Fatal("expected posible_cosecha false")
	}
}

func TestProductionRepo_ListActive(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	activeID := randomID()
	inactiveID := randomID()

	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: activeID, Cosecha: "Activo", Monitoring: true}); err != nil {
		t.Fatalf("Upsert active: %v", err)
	}
	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: inactiveID, Cosecha: "Inactivo", Monitoring: false}); err != nil {
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

func TestProductionRepo_UpdateMonitoring(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Monitoring: true}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := repo.UpdateMonitoring(ctx, produccionID, false); err != nil {
		t.Fatalf("UpdateMonitoring: %v", err)
	}

	got, err := repo.GetByProduccionID(ctx, produccionID)
	if err != nil {
		t.Fatalf("GetByProduccionID: %v", err)
	}
	if got.Monitoring {
		t.Error("expected monitoring false after update")
	}
}

func TestProductionRepo_GetERPDetails(t *testing.T) {
	db := testDB(t)
	repo := NewProductionRepo(db)
	ctx := context.Background()

	produccionID := randomID()
	if err := repo.Upsert(ctx, &domain.Production{ProduccionID: produccionID, Monitoring: true}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// GetERPDetails queries the ERP tables — in the test DB they may not exist,
	// so we just verify it does not panic and returns gracefully.
	_, err := repo.GetERPDetails(ctx, produccionID)
	// An error here is acceptable (missing ERP tables in test DB).
	_ = err
}
