package processing

import (
	"time"

	"agro-sentinel-worker/internal/domain"
)

// dateLayout is the date-only format used for scene dates in params.json,
// matching the spec's params.json schema.
const dateLayout = "2006-01-02"

// maxHistoricoEntries caps the historical chain carried in params.json.
const maxHistoricoEntries = 20

// ParamsInput carries the current scene's computed statistics and metadata
// needed to build a new Params document.
type ParamsInput struct {
	ProduccionID     int64
	SceneID          string
	SceneDate        time.Time
	FechaPlantacion  time.Time
	CloudCoverBBox   float64
	Indices          map[domain.FileType]IndexStats
	BandStats        map[domain.Band]BandStats
	Coverage         CoverageStats
}

// HistoricoEntry summarizes one previous scene within the historical chain,
// along with the deltas from that scene to the one immediately after it.
type HistoricoEntry struct {
	SceneID        string               `json:"scene_id"`
	SceneDate      string               `json:"scene_date"`
	CloudCoverBBox float64              `json:"cloud_cover_bbox"`
	Indices        map[string]IndexStats `json:"indices,omitempty"`
	Delta          map[string]float64   `json:"delta,omitempty"`
}

// Params is the JSON-serializable structure written to params.json,
// matching the spec's schema.
type Params struct {
	ProduccionID        int64                  `json:"produccion_id"`
	SceneID             string                 `json:"scene_id"`
	SceneDate           string                 `json:"scene_date"`
	DiasDesdePlantacion int                    `json:"dias_desde_plantacion"`
	CloudCoverBBox      float64                `json:"cloud_cover_bbox"`
	Coverage            CoverageStats          `json:"coverage"`
	Indices             map[string]IndexStats  `json:"indices"`
	BandStats           map[string]BandStats   `json:"band_stats,omitempty"`
	Historico           []HistoricoEntry       `json:"historico,omitempty"`
}

// fileTypeIndexKeys maps domain.FileType index identifiers to the lowercase
// string keys used in params.json.
func indexKey(t domain.FileType) string {
	return string(t)
}

func bandKey(b domain.Band) string {
	return string(b)
}

// BuildParams constructs the new Params document for the current scene,
// prepending a summary of the previous scene (if any) to the historical
// chain, with per-index mean deltas from the previous scene to the current
// one. The historical chain is capped at maxHistoricoEntries.
func BuildParams(current ParamsInput, previousParams *Params) *Params {
	indices := make(map[string]IndexStats, len(current.Indices))
	for t, stats := range current.Indices {
		indices[indexKey(t)] = stats
	}

	var bandStats map[string]BandStats
	if len(current.BandStats) > 0 {
		bandStats = make(map[string]BandStats, len(current.BandStats))
		for b, stats := range current.BandStats {
			bandStats[bandKey(b)] = stats
		}
	}

	params := &Params{
		ProduccionID:        current.ProduccionID,
		SceneID:             current.SceneID,
		SceneDate:           current.SceneDate.Format(dateLayout),
		DiasDesdePlantacion: daysBetween(current.FechaPlantacion, current.SceneDate),
		CloudCoverBBox:      current.CloudCoverBBox,
		Coverage:            current.Coverage,
		Indices:             indices,
		BandStats:           bandStats,
	}

	if previousParams == nil {
		return params
	}

	historico := make([]HistoricoEntry, 0, len(previousParams.Historico)+1)

	prevEntry := HistoricoEntry{
		SceneID:        previousParams.SceneID,
		SceneDate:      previousParams.SceneDate,
		CloudCoverBBox: previousParams.CloudCoverBBox,
		Indices:        previousParams.Indices,
		Delta:          computeDeltas(previousParams.Indices, indices),
	}
	historico = append(historico, prevEntry)
	historico = append(historico, previousParams.Historico...)

	params.Historico = TrimHistorico(historico, maxHistoricoEntries)

	return params
}

// computeDeltas returns a map of "<index>_mean_change" -> current mean -
// previous mean, for every index present in both the previous and current
// index maps.
func computeDeltas(previous, current map[string]IndexStats) map[string]float64 {
	if len(previous) == 0 || len(current) == 0 {
		return nil
	}

	deltas := make(map[string]float64)
	for name, prevStats := range previous {
		curStats, ok := current[name]
		if !ok {
			continue
		}
		deltas[name+"_mean_change"] = roundTo(curStats.Mean-prevStats.Mean, 6)
	}
	if len(deltas) == 0 {
		return nil
	}
	return deltas
}

// roundTo rounds v to the given number of decimal places, avoiding
// floating-point noise in delta comparisons (e.g. 0.72-0.60 == 0.12).
func roundTo(v float64, decimals int) float64 {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	return float64(int64(v*mult+sign(v)*0.5)) / mult
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// daysBetween returns the number of whole days between fechaPlantacion and
// sceneDate. Negative if sceneDate precedes fechaPlantacion.
func daysBetween(fechaPlantacion, sceneDate time.Time) int {
	d := sceneDate.Sub(fechaPlantacion)
	return int(d.Hours() / 24)
}

// TrimHistorico keeps at most maxEntries entries from historico, preferring
// the entries at the front of the slice (the most recent scenes, since
// BuildParams always prepends).
func TrimHistorico(historico []HistoricoEntry, maxEntries int) []HistoricoEntry {
	if len(historico) <= maxEntries {
		return historico
	}
	return historico[:maxEntries]
}
