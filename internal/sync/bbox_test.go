package sync

import "testing"

func TestCalculateBBoxFromWKT(t *testing.T) {
	wkt := "POLYGON((-102.35 21.80, -102.30 21.80, -102.30 21.85, -102.35 21.85, -102.35 21.80))"
	bbox, err := CalculateBBoxFromWKT(wkt)
	if err != nil {
		t.Fatalf("CalculateBBoxFromWKT failed: %v", err)
	}
	if bbox.MinX != -102.35 || bbox.MaxX != -102.30 || bbox.MinY != 21.80 || bbox.MaxY != 21.85 {
		t.Errorf("bbox = %+v, unexpected values", bbox)
	}
}

func TestCalculateBBoxFromWKT_WithSRID(t *testing.T) {
	wkt := "SRID=4326;POLYGON((-100 20, -99 20, -99 21, -100 21, -100 20))"
	bbox, err := CalculateBBoxFromWKT(wkt)
	if err != nil {
		t.Fatalf("CalculateBBoxFromWKT failed: %v", err)
	}
	if bbox.MinX != -100 || bbox.MaxX != -99 || bbox.MinY != 20 || bbox.MaxY != 21 {
		t.Errorf("bbox = %+v, unexpected values", bbox)
	}
}

func TestCalculateBBoxFromWKT_Empty(t *testing.T) {
	if _, err := CalculateBBoxFromWKT(""); err == nil {
		t.Fatal("expected error for empty WKT")
	}
}

func TestCalculateBBoxFromWKT_Invalid(t *testing.T) {
	if _, err := CalculateBBoxFromWKT("POINT(1 2)"); err == nil {
		t.Fatal("expected error for unsupported geometry type")
	}
}

func TestCalculateBBoxFromWKT_PipeFormat(t *testing.T) {
	// Format stored in asignaciones_zonas_producciones: lat,lon|lat,lon|...
	pipe := "21.1224472222222,-100.891313888889|21.1214361111111,-100.891147222222|21.1203416666667,-100.888763888889|21.1198611111111,-100.888955555556"
	bbox, err := CalculateBBoxFromWKT(pipe)
	if err != nil {
		t.Fatalf("PipeFormat failed: %v", err)
	}
	// lon (X): min=-100.891313888889, max=-100.888763888889  (more negative = smaller)
	// lat (Y): min=21.1198611111111, max=21.1224472222222
	if bbox.MinX > -100.8913 || bbox.MaxX < -100.8888 {
		t.Errorf("unexpected X range: MinX=%v MaxX=%v", bbox.MinX, bbox.MaxX)
	}
	if bbox.MinY > 21.1199 || bbox.MaxY < 21.1224 {
		t.Errorf("unexpected Y range: MinY=%v MaxY=%v", bbox.MinY, bbox.MaxY)
	}
}

func TestCalculateBBoxFromWKT_PipeFormat_RealCase(t *testing.T) {
	// Real example from produccion 2066 logs
	pipe := "21.1224472222222,-100.891313888889|21.1214361111111,-100.891147222222|21.1203416666667,-100.888763888889|21.1198611111111,-100.888955555556|21.1203833333333,-100.891366666667|21.1216666666667,-100.892475"
	bbox, err := CalculateBBoxFromWKT(pipe)
	if err != nil {
		t.Fatalf("RealCase failed: %v", err)
	}
	if bbox.MinX == 0 || bbox.MinY == 0 {
		t.Errorf("unexpected zero bbox: %+v", bbox)
	}
}
