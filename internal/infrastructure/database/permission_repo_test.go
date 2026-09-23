package database

import "strings"
import "testing"

func TestCentrosCostosSelectorQueryFiltraRanchosCuandoExisteTipo(t *testing.T) {
	q := centrosCostosSelectorQuery(true, false)
	if !strings.Contains(q, "LOWER(tipo) = 'rancho'") {
		t.Fatalf("expected rancho filter, got %s", q)
	}
}

func TestCentrosCostosSelectorQueryFiltraRanchosConCatalogoTipo(t *testing.T) {
	q := centrosCostosSelectorQuery(false, true)
	if !strings.Contains(q, "JOIN tipos_centros_costos") {
		t.Fatalf("expected tipos_centros_costos join, got %s", q)
	}
	if !strings.Contains(q, "tcc.tipo_centro_costo_id = cc.tipo_centro_costo_id") {
		t.Fatalf("expected tipo_centro_costo_id relation, got %s", q)
	}
	if !strings.Contains(q, "UPPER(tcc.nombre) = 'RANCHOS'") {
		t.Fatalf("expected rancho catalog filter, got %s", q)
	}
}

func TestCentrosCostosSelectorQueryCompatibilidadSinTipo(t *testing.T) {
	q := centrosCostosSelectorQuery(false, false)
	if strings.Contains(q, "tipo") || strings.Contains(q, "WHERE") {
		t.Fatalf("expected unfiltered legacy query, got %s", q)
	}
}
