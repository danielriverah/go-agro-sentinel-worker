package processing

import (
	"fmt"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

func TestBuildParams(t *testing.T) {
	current := ParamsInput{
		ProduccionID:    1234,
		SceneID:         "S2A_scene2",
		SceneDate:       time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		FechaPlantacion: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		CloudCoverBBox:  12.5,
		Indices: map[domain.FileType]IndexStats{
			domain.FileNDVI: {Mean: 0.72, Std: 0.08},
		},
		Coverage: CoverageStats{VegetationPct: 78.5, SoilPct: 15.2},
	}

	previous := &Params{
		ProduccionID:        1234,
		SceneID:             "S2A_scene1",
		SceneDate:           "2026-08-22",
		DiasDesdePlantacion: 38,
		Indices: map[string]IndexStats{
			"ndvi": {Mean: 0.60},
		},
		Historico: nil, // first scene had no history
	}

	params := BuildParams(current, previous)

	if params.DiasDesdePlantacion != 48 {
		t.Errorf("dias = %d, want 48", params.DiasDesdePlantacion)
	}
	if len(params.Historico) != 1 {
		t.Fatalf("historico length = %d, want 1", len(params.Historico))
	}
	if params.Historico[0].SceneID != "S2A_scene1" {
		t.Error("historico[0] should be previous scene")
	}
	if params.Historico[0].Delta["ndvi_mean_change"] != 0.12 {
		t.Errorf("ndvi delta = %v, want 0.12", params.Historico[0].Delta["ndvi_mean_change"])
	}
}

func TestBuildParams_NoPrevious(t *testing.T) {
	current := ParamsInput{
		ProduccionID:    1,
		SceneID:         "S2A_scene1",
		SceneDate:       time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		FechaPlantacion: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		Indices: map[domain.FileType]IndexStats{
			domain.FileNDVI: {Mean: 0.60},
		},
	}

	params := BuildParams(current, nil)

	if params.DiasDesdePlantacion != 38 {
		t.Errorf("dias = %d, want 38", params.DiasDesdePlantacion)
	}
	if len(params.Historico) != 0 {
		t.Errorf("historico length = %d, want 0", len(params.Historico))
	}
	if params.Indices["ndvi"].Mean != 0.60 {
		t.Errorf("ndvi mean = %v, want 0.60", params.Indices["ndvi"].Mean)
	}
}

func TestBuildParams_ChainsPreviousHistorico(t *testing.T) {
	current := ParamsInput{
		SceneID:   "scene3",
		SceneDate: time.Now(),
		Indices: map[domain.FileType]IndexStats{
			domain.FileNDVI: {Mean: 0.5},
		},
	}
	previous := &Params{
		SceneID:   "scene2",
		SceneDate: "2026-08-22",
		Indices: map[string]IndexStats{
			"ndvi": {Mean: 0.4},
		},
		Historico: []HistoricoEntry{
			{SceneID: "scene1"},
		},
	}

	params := BuildParams(current, previous)

	if len(params.Historico) != 2 {
		t.Fatalf("historico length = %d, want 2", len(params.Historico))
	}
	if params.Historico[0].SceneID != "scene2" {
		t.Errorf("historico[0] = %s, want scene2", params.Historico[0].SceneID)
	}
	if params.Historico[1].SceneID != "scene1" {
		t.Errorf("historico[1] = %s, want scene1", params.Historico[1].SceneID)
	}
}

func TestTrimHistorico(t *testing.T) {
	entries := make([]HistoricoEntry, 25)
	for i := range entries {
		entries[i] = HistoricoEntry{SceneID: fmt.Sprintf("scene_%d", i)}
	}

	trimmed := TrimHistorico(entries, 20)
	if len(trimmed) != 20 {
		t.Errorf("trimmed length = %d, want 20", len(trimmed))
	}
	if trimmed[0].SceneID != "scene_0" {
		t.Errorf("trimmed[0] = %s, want scene_0", trimmed[0].SceneID)
	}
}

func TestTrimHistorico_UnderLimit(t *testing.T) {
	entries := []HistoricoEntry{{SceneID: "a"}, {SceneID: "b"}}
	trimmed := TrimHistorico(entries, 20)
	if len(trimmed) != 2 {
		t.Errorf("trimmed length = %d, want 2", len(trimmed))
	}
}

func TestDaysBetween(t *testing.T) {
	fecha := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	scene := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if got := daysBetween(fecha, scene); got != 48 {
		t.Errorf("daysBetween = %d, want 48", got)
	}
}
