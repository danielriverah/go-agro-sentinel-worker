package processing

import (
	"strings"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

func iaTestInput() IARequestInput {
	planted := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

	return IARequestInput{
		Production: &domain.Production{
			ProduccionID: 42,
			Folio:        "CSJ2601-18-A",
			Rancho:       "EL CARMEN",
			// La columna del ERP se llama "cosecha" pero guarda la especie.
			Cosecha:   "LECHUGA",
			Vaiedades: "ROMANA",
			// Valor deliberadamente improbable: el test comprueba que este
			// plazo administrativo no aparezca por ningún lado del payload.
			MaxDiasMonitoring: 9999,
			FechaPlantacion:   &planted,
			FechaFin:          &end,
		},
		Scene: &domain.Scene{SceneName: "S2B_TEST_L2A"},
		Params: &Params{
			SceneDate:           "2026-08-01",
			DiasDesdePlantacion: 65,
			CloudCoverBBox:      3.2,
			Coverage:            CoverageStats{VegetationPct: 88, SoilPct: 10, WaterPct: 2},
			Indices:             map[string]IndexStats{"ndvi": {Mean: 0.74}},
		},
	}
}

// Los plazos de seguimiento llevan márgenes administrativos — en lechuga, 95
// días frente a un ciclo real de 60-75 — así que el modelo los leía como fecha
// de cosecha y avisaba fuera de tiempo. No deben viajar en el payload.
func TestBuildIARequestOmitsMonitoringDeadlines(t *testing.T) {
	req := BuildIARequest(iaTestInput())

	raw, err := MarshalIARequest(req)
	if err != nil {
		t.Fatalf("marshal ia request: %v", err)
	}
	got := string(raw)

	if strings.Contains(got, "9999") {
		t.Errorf("el payload filtró max_dias_monitoring:\n%s", got)
	}
	for _, forbidden := range []string{"dias_ciclo_aprox", "fecha_fin", "max_dias"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("el payload contiene el campo administrativo %q:\n%s", forbidden, got)
		}
	}
}

func TestBuildIARequestCarriesCrop(t *testing.T) {
	req := BuildIARequest(iaTestInput())

	if req.Produccion.Cultivo != "LECHUGA" {
		t.Errorf("cultivo = %q, want LECHUGA", req.Produccion.Cultivo)
	}
	if req.Produccion.DiasDesdePlantacion != 65 {
		t.Errorf("dias_desde_plantacion = %d, want 65", req.Produccion.DiasDesdePlantacion)
	}

	raw, err := MarshalIARequest(req)
	if err != nil {
		t.Fatalf("marshal ia request: %v", err)
	}
	// El modelo necesita la especie bajo una clave que signifique lo que dice:
	// "cosecha" se confunde con la temporada.
	if !strings.Contains(string(raw), `"cultivo": "LECHUGA"`) {
		t.Errorf("falta la clave cultivo en el payload:\n%s", raw)
	}
}
