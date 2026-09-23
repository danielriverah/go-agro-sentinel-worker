package domain

import (
	"encoding/json"
	"testing"
)

func TestEffectiveBBoxPrefersOwnImageBBox(t *testing.T) {
	prod := &Production{TileBBoxJSON: json.RawMessage(
		`{"min_lon":-100,"min_lat":20,"max_lon":-99,"max_lat":21}`)}

	// Escena que reutilizó el multiband de otra producción: su extensión real
	// es la heredada, no la de su propia producción.
	heredado := BBox{MinX: -105, MinY: 25, MaxX: -104, MaxY: 26}
	scene := &Scene{ImageBBox: heredado.MarshalJSONColumn()}

	got := scene.EffectiveBBox(prod)
	if got == nil {
		t.Fatal("EffectiveBBox devolvió nil")
	}
	if *got != heredado {
		t.Errorf("bbox = %+v, want %+v (el heredado, no el de su producción)", *got, heredado)
	}
}

func TestEffectiveBBoxFallsBackToProduction(t *testing.T) {
	prod := &Production{TileBBoxJSON: json.RawMessage(
		`{"min_lon":-100,"min_lat":20,"max_lon":-99,"max_lat":21}`)}
	scene := &Scene{} // sin image_bbox: construyó su propio multiband

	got := scene.EffectiveBBox(prod)
	if got == nil {
		t.Fatal("EffectiveBBox devolvió nil")
	}
	want := BBox{MinX: -100, MinY: 20, MaxX: -99, MaxY: 21}
	if *got != want {
		t.Errorf("bbox = %+v, want %+v", *got, want)
	}
}

func TestEffectiveBBoxIgnoresCorruptImageBBox(t *testing.T) {
	prod := &Production{TileBBoxJSON: json.RawMessage(
		`{"min_lon":-100,"min_lat":20,"max_lon":-99,"max_lat":21}`)}
	scene := &Scene{ImageBBox: json.RawMessage(`{no es json`)}

	// Un valor ilegible no debe dejar la escena sin georreferencia.
	if got := scene.EffectiveBBox(prod); got == nil {
		t.Error("con image_bbox corrupto debería recurrir al tile_bbox de la producción")
	}
}

// El relleno del DBA copia tile_bbox tal cual, así que EffectiveBBox tiene que
// entender también esa forma, no sólo la que escribe el worker.
func TestEffectiveBBoxReadsPboxArrayForm(t *testing.T) {
	scene := &Scene{ImageBBox: json.RawMessage(`{"pbox":[-105,25,-104,26]}`)}

	got := scene.EffectiveBBox(nil)
	if got == nil {
		t.Fatal("EffectiveBBox devolvió nil para la forma pbox")
	}
	want := BBox{MinX: -105, MinY: 25, MaxX: -104, MaxY: 26}
	if *got != want {
		t.Errorf("bbox = %+v, want %+v", *got, want)
	}
}
