package domain

import (
	"math"
	"testing"
)

// produccion2004 is the real polygon that made gdalwarp fail with
// "Cutline polygon is invalid": vertex 7 sits ~2.2 m from vertex 1 but just
// past the line from 1 to 2, so side 6→7 crosses side 1→2.
var produccion2004 = Ring{
	{-100.894830555556, 21.1226694444444},
	{-100.893566666667, 21.1234916666667},
	{-100.893208333333, 21.1232583333333},
	{-100.892888888889, 21.1220944444444},
	{-100.893888888889, 21.1215333333333},
	{-100.894769444444, 21.1221472222222},
	{-100.894819444444, 21.1226861111111},
}

func TestValidate_Produccion2004_IsRejected(t *testing.T) {
	err := produccion2004.Validate()
	if err == nil {
		t.Fatal("expected the polygon to be rejected")
	}
	t.Logf("rejected with: %v", err)
}

func TestCrossing_Produccion2004_MatchesGDALPoint(t *testing.T) {
	// GDAL reported the self-intersection at this point.
	const wantLon, wantLat = -100.89481861857169, 21.12267721004298

	c := produccion2004.Crossing()
	if c == nil {
		t.Fatal("expected a crossing between sides 6→7 and 1→2")
	}
	if math.Abs(c.Lon-wantLon) > 1e-8 || math.Abs(c.Lat-wantLat) > 1e-8 {
		t.Errorf("crossing at (%.11f, %.11f), want (%.11f, %.11f)",
			c.Lon, c.Lat, wantLon, wantLat)
	}
	// Sides are zero-indexed by their first vertex: 0 is 1→2, 5 is 6→7.
	if c.SideA != 0 || c.SideB != 5 {
		t.Errorf("crossing between sides %d and %d, want 0 and 5", c.SideA, c.SideB)
	}
}

func TestValidate_Produccion2004_WithoutLastVertex_IsValid(t *testing.T) {
	fixed := produccion2004[:6]
	if err := fixed.Validate(); err != nil {
		t.Fatalf("dropping vertex 7 should make it valid, got: %v", err)
	}
	if c := fixed.Crossing(); c != nil {
		t.Errorf("unexpected crossing: %v", c)
	}
}

func TestValidate_TooFewVertices(t *testing.T) {
	if err := (Ring{{0, 0}, {1, 1}}).Validate(); err == nil {
		t.Error("expected two vertices to be rejected")
	}
}

func TestValidate_ClosedRingIsAccepted(t *testing.T) {
	// A ring that repeats its first point as the last must not be read as a
	// zero-length side.
	closed := Ring{
		{-100.8948, 21.1226}, {-100.8935, 21.1234},
		{-100.8928, 21.1220}, {-100.8938, 21.1215},
		{-100.8948, 21.1226},
	}
	if err := closed.Validate(); err != nil {
		t.Fatalf("closed ring should be valid, got: %v", err)
	}
}

func TestValidate_BowtieIsRejected(t *testing.T) {
	bowtie := Ring{{0, 0}, {1, 1}, {1, 0}, {0, 1}}
	if err := bowtie.Validate(); err == nil {
		t.Error("expected a bowtie to be rejected")
	}
}

func TestNormalized_ReversesClockwiseRing(t *testing.T) {
	cw := Ring{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
	if cw.IsCounterClockwise() {
		t.Fatal("fixture should be clockwise")
	}
	ccw := cw.Normalized()
	if !ccw.IsCounterClockwise() {
		t.Error("Normalized should wind counter-clockwise")
	}
	if len(ccw) != len(cw) {
		t.Errorf("got %d vertices, want %d", len(ccw), len(cw))
	}
	// Reversing must preserve the shape, so the area is unchanged.
	if math.Abs(math.Abs(cw.SignedArea())-ccw.SignedArea()) > 1e-12 {
		t.Error("reversing changed the area")
	}
}

func TestNormalized_KeepsCounterClockwiseRing(t *testing.T) {
	ccw := Ring{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	got := ccw.Normalized()
	for i := range ccw {
		if got[i] != ccw[i] {
			t.Fatalf("vertex %d changed: got %v, want %v", i, got[i], ccw[i])
		}
	}
}

func TestWithinBBox(t *testing.T) {
	tile := &BBox{MinX: -100.90, MinY: 21.11, MaxX: -100.88, MaxY: 21.13}
	if !produccion2004.WithinBBox(tile) {
		t.Error("polygon should fit inside the tile")
	}
	tight := &BBox{MinX: -100.8945, MinY: 21.122, MaxX: -100.893, MaxY: 21.123}
	if produccion2004.WithinBBox(tight) {
		t.Error("polygon should not fit inside the tight bbox")
	}
}

func TestAreaHectares(t *testing.T) {
	area := produccion2004[:6].AreaHectares()
	// Roughly 200 x 200 m of plot — sanity bounds, not an exact figure.
	if area < 1 || area > 50 {
		t.Errorf("area %.2f ha is outside the plausible range", area)
	}
}
