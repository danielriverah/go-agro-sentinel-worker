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
