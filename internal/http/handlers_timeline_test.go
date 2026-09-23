package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/processing"
)

func tlDate(day int) *time.Time {
	t := time.Date(2024, 6, day, 0, 0, 0, 0, time.UTC)
	return &t
}

func tlFloat(v float64) *float64 { return &v }

func tlParams(t *testing.T, indices map[string]float64) string {
	t.Helper()
	stats := make(map[string]processing.IndexStats, len(indices))
	for k, v := range indices {
		stats[k] = processing.IndexStats{Mean: v}
	}
	raw, err := json.Marshal(processing.Params{Indices: stats})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return string(raw)
}

func tlProduction() *domain.Production {
	planted := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 9, 30, 0, 0, 0, 0, time.UTC)
	return &domain.Production{
		ID:              7,
		Folio:           "F-001",
		Rancho:          "El Milagro",
		Cosecha:         "2024",
		FechaPlantacion: &planted,
		FechaFin:        &end,
	}
}

func TestBuildTimelineNoScenes(t *testing.T) {
	resp := buildTimeline(tlProduction(), nil)

	if len(resp.Puntos) != 0 {
		t.Errorf("puntos = %d, want 0", len(resp.Puntos))
	}
	if resp.Resumen.PuntosConfiables != 0 {
		t.Errorf("puntos_confiables = %d, want 0", resp.Resumen.PuntosConfiables)
	}
	if resp.Resumen.MejorDia != nil || resp.Resumen.PeorDia != nil {
		t.Error("mejor/peor dia should be nil with no scenes")
	}
	if resp.Fases == nil {
		t.Error("fases must serialize as [], not null")
	}
}

func TestBuildTimelineUnreliableScenes(t *testing.T) {
	tests := []struct {
		name       string
		row        *domain.TimelineRow
		wantMotivo string
	}{
		{
			name: "sensor acquired no data",
			row: &domain.TimelineRow{
				EscenaID: 1, Fecha: tlDate(1), Usable: true,
				ProductionCloud: tlFloat(101),
				ParamsJSON:      tlParams(t, map[string]float64{"ndvi": 0.5}),
			},
			wantMotivo: "sin_dato_sensor",
		},
		{
			name: "params file missing",
			row: &domain.TimelineRow{
				EscenaID: 2, Fecha: tlDate(2), Usable: true,
				ProductionCloud: tlFloat(5),
			},
			wantMotivo: "sin_params",
		},
		{
			name: "params content corrupt",
			row: &domain.TimelineRow{
				EscenaID: 3, Fecha: tlDate(3), Usable: true,
				ProductionCloud: tlFloat(5),
				ParamsJSON:      "{not valid json",
			},
			wantMotivo: "sin_params",
		},
		{
			name: "too cloudy over the polygon",
			row: &domain.TimelineRow{
				EscenaID: 4, Fecha: tlDate(4), Usable: true,
				ProductionCloud: tlFloat(78),
				ParamsJSON:      tlParams(t, map[string]float64{"ndvi": 0.2}),
			},
			wantMotivo: "nubosidad_alta",
		},
		{
			name: "flagged unusable",
			row: &domain.TimelineRow{
				EscenaID: 5, Fecha: tlDate(5), Usable: false,
				ProductionCloud: tlFloat(5),
				ParamsJSON:      tlParams(t, map[string]float64{"ndvi": 0.5}),
			},
			wantMotivo: "escena_no_usable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := buildTimeline(tlProduction(), []*domain.TimelineRow{tc.row})

			if len(resp.Puntos) != 1 {
				t.Fatalf("puntos = %d, want 1", len(resp.Puntos))
			}
			pt := resp.Puntos[0]

			if pt.Confiable {
				t.Error("punto should not be marked confiable")
			}
			if pt.MotivoNoConfiable != tc.wantMotivo {
				t.Errorf("motivo = %q, want %q", pt.MotivoNoConfiable, tc.wantMotivo)
			}
			// Values are withheld so the chart cannot plot a misleading dip.
			if pt.Valores != nil {
				t.Errorf("valores = %v, want nil for an unreliable point", pt.Valores)
			}
			if resp.Resumen.PuntosConfiables != 0 {
				t.Errorf("puntos_confiables = %d, want 0", resp.Resumen.PuntosConfiables)
			}
		})
	}
}

func TestBuildTimelineDeltaSkipsCloudyScenes(t *testing.T) {
	rows := []*domain.TimelineRow{
		{
			EscenaID: 1, Fecha: tlDate(1), Usable: true, ProductionCloud: tlFloat(3),
			ParamsJSON: tlParams(t, map[string]float64{"ndvi": 0.60}),
		},
		{
			EscenaID: 2, Fecha: tlDate(5), Usable: true, ProductionCloud: tlFloat(85),
			ParamsJSON: tlParams(t, map[string]float64{"ndvi": 0.11}),
		},
		{
			EscenaID: 3, Fecha: tlDate(9), Usable: true, ProductionCloud: tlFloat(4),
			ParamsJSON: tlParams(t, map[string]float64{"ndvi": 0.70}),
		},
	}

	resp := buildTimeline(tlProduction(), rows)

	if len(resp.Puntos) != 3 {
		t.Fatalf("puntos = %d, want 3", len(resp.Puntos))
	}

	last := resp.Puntos[2]
	if !last.Confiable {
		t.Fatal("third point should be confiable")
	}

	// 0.70 - 0.60: measured against the last trustworthy reading, not against
	// the cloudy 0.11 in between, which would report a fictitious +0.59 jump.
	got := last.Delta["ndvi"]
	if got != 0.10 {
		t.Errorf("delta ndvi = %v, want 0.10 (compared against the last reliable point)", got)
	}

	if resp.Resumen.PuntosConfiables != 2 {
		t.Errorf("puntos_confiables = %d, want 2", resp.Resumen.PuntosConfiables)
	}
	// The cloudy 0.11 must not win "worst day".
	if resp.Resumen.PeorDia == nil || resp.Resumen.PeorDia.Valor != 0.60 {
		t.Errorf("peor_dia = %+v, want value 0.60", resp.Resumen.PeorDia)
	}
}

func TestBuildTimelineReliablePoint(t *testing.T) {
	rows := []*domain.TimelineRow{{
		EscenaID: 1, SceneName: "S2A_TEST", Fecha: tlDate(15), Usable: true,
		ProductionCloud: tlFloat(5),
		ParamsJSON:      tlParams(t, map[string]float64{"ndvi": 0.68, "ndmi": 0.05}),
	}}

	resp := buildTimeline(tlProduction(), rows)
	pt := resp.Puntos[0]

	if !pt.Confiable {
		t.Fatalf("punto should be confiable, motivo=%q", pt.MotivoNoConfiable)
	}
	if pt.Valores["ndvi"] != 0.68 {
		t.Errorf("ndvi = %v, want 0.68", pt.Valores["ndvi"])
	}
	if pt.Estado["ndvi"] != "vigoroso" {
		t.Errorf("estado ndvi = %q, want vigoroso", pt.Estado["ndvi"])
	}
	if pt.Estado["ndmi"] != "seco" {
		t.Errorf("estado ndmi = %q, want seco", pt.Estado["ndmi"])
	}
	// 2024-05-01 planted, scene on 2024-06-15.
	if pt.DiaCultivo == nil || *pt.DiaCultivo != 45 {
		t.Errorf("dia_cultivo = %v, want 45", pt.DiaCultivo)
	}
	if pt.DiasACosecha == nil || *pt.DiasACosecha != 107 {
		t.Errorf("dias_a_cosecha = %v, want 107", pt.DiasACosecha)
	}
	if pt.Delta != nil {
		t.Errorf("delta = %v, want nil for the first point", pt.Delta)
	}
}

func TestBuildTimelineOmitsNestedHistorico(t *testing.T) {
	// params.json carries up to 20 previous scenes inline. Echoing that per
	// point would multiply the payload for data the caller already has.
	full := processing.Params{
		Indices: map[string]processing.IndexStats{"ndvi": {Mean: 0.5}},
		Historico: []processing.HistoricoEntry{
			{SceneID: "old-1", Indices: map[string]processing.IndexStats{"ndvi": {Mean: 0.4}}},
			{SceneID: "old-2", Indices: map[string]processing.IndexStats{"ndvi": {Mean: 0.3}}},
		},
	}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	resp := buildTimeline(tlProduction(), []*domain.TimelineRow{{
		EscenaID: 1, Fecha: tlDate(1), Usable: true,
		ProductionCloud: tlFloat(2), ParamsJSON: string(raw),
	}})

	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if got := string(out); strings.Contains(got, "old-1") || strings.Contains(got, "historico") {
		t.Error("response leaked the nested historico from params.json")
	}
}
