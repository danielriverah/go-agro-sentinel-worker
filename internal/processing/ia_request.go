package processing

import (
	"bytes"
	"encoding/json"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// recentHistoricoWindow is the number of most-recent scenes included in the
// ia_req.json "recientes" array. Kept small so the IA payload stays compact.
const recentHistoricoWindow = 3

// IARequestProduccion carries the production-level context sent to the IA.
type IARequestProduccion struct {
	ID                  int64  `json:"id"`
	Folio               string `json:"folio,omitempty"`
	Rancho              string `json:"rancho,omitempty"`
	Cosecha             string `json:"cosecha"`
	Variedades          string `json:"variedades,omitempty"`
	FechaPlantacion     string `json:"fecha_plantacion,omitempty"`
	DiasDesdePlantacion int    `json:"dias_desde_plantacion"`
	DiasCicloAprox      int    `json:"dias_ciclo_aprox,omitempty"`
}

// IARequestEscena carries the scene-level context sent to the IA.
type IARequestEscena struct {
	ID            string  `json:"id"`
	Fecha         string  `json:"fecha"`
	CloudCoverPct float64 `json:"cloud_cover_pct"`
}

// IARequestCobertura carries the coverage percentages.
type IARequestCobertura struct {
	VegetacionPct float64 `json:"vegetacion_pct"`
	SueloPct      float64 `json:"suelo_pct"`
	AguaPct       float64 `json:"agua_pct"`
}

// IARequestEstado carries the current vegetation state.
type IARequestEstado struct {
	Cobertura IARequestCobertura `json:"cobertura"`
	Indices   map[string]float64 `json:"indices"`
}

// IARequestHistoricoReciente holds one row in the "recientes" array.
type IARequestHistoricoReciente struct {
	Fecha    string             `json:"fecha"`
	Indices  map[string]float64 `json:"indices"`
	CloudPct float64            `json:"cloud_pct"`
}

// IARequestHistorico holds the compact historical summary.
type IARequestHistorico struct {
	NTotal    int                          `json:"n_total"`
	Recientes []IARequestHistoricoReciente `json:"recientes,omitempty"`
	Tendencia map[string]float64           `json:"tendencia,omitempty"`
}

// IARequestUltimoAnalisis holds the previous IA analysis summary.
type IARequestUltimoAnalisis struct {
	Fecha         string `json:"fecha,omitempty"`
	EstadoClave   string `json:"estado_clave"`
	EstadoGeneral string `json:"estado_general"`
	RiesgoNivel   string `json:"riesgo_nivel"`
	RiesgoMotivo  string `json:"riesgo_motivo,omitempty"`
}

// IARequest is the compact JSON payload written to multiband.ia_req.json and
// sent to the IA service. It contains only what the model needs — no raw band
// data, no full statistical tables.
type IARequest struct {
	Produccion     IARequestProduccion      `json:"produccion"`
	Escena         IARequestEscena          `json:"escena"`
	FechaAnalisis  string                   `json:"fecha_analisis"`
	Estado         IARequestEstado          `json:"estado"`
	Historico      IARequestHistorico       `json:"historico"`
	UltimoAnalisis *IARequestUltimoAnalisis `json:"ultimo_analisis,omitempty"`
}

// IARequestInput bundles everything BuildIARequest needs.
type IARequestInput struct {
	Production    *domain.Production
	Scene         *domain.Scene
	Params        *Params
	FullHistorico []HistoricoEntry
	UltimoAnalisis *domain.IAResultSummary
}

// BuildIARequest constructs the compact IA payload from the current params
// and the production/scene context.
func BuildIARequest(in IARequestInput) *IARequest {
	prod := in.Production
	scene := in.Scene
	p := in.Params

	var plantacionStr string
	if prod.FechaPlantacion != nil {
		plantacionStr = prod.FechaPlantacion.Format(dateLayout)
	}

	req := &IARequest{
		Produccion: IARequestProduccion{
			ID:                  prod.ProduccionID,
			Folio:               prod.Folio,
			Rancho:              prod.Rancho,
			Cosecha:             prod.Cosecha,
			Variedades:          prod.Vaiedades,
			FechaPlantacion:     plantacionStr,
			DiasDesdePlantacion: p.DiasDesdePlantacion,
			DiasCicloAprox:      prod.MaxDiasMonitoring,
		},
		FechaAnalisis: time.Now().UTC().Format(dateLayout),
		Escena: IARequestEscena{
			ID:            scene.SceneName,
			Fecha:         p.SceneDate,
			CloudCoverPct: roundTo(p.CloudCoverBBox, 2),
		},
		Estado: IARequestEstado{
			Cobertura: IARequestCobertura{
				VegetacionPct: roundTo(p.Coverage.VegetationPct, 2),
				SueloPct:      roundTo(p.Coverage.SoilPct, 2),
				AguaPct:       roundTo(p.Coverage.WaterPct, 2),
			},
			Indices: indexMeans(p.Indices),
		},
		Historico: buildHistorico(in.FullHistorico),
	}

	if in.UltimoAnalisis != nil {
		ua := in.UltimoAnalisis
		var fechaStr string
		if ua.FechaAnalisis != nil {
			fechaStr = ua.FechaAnalisis.Format(dateLayout)
		}
		req.UltimoAnalisis = &IARequestUltimoAnalisis{
			Fecha:         fechaStr,
			EstadoClave:   ua.EstadoClave,
			EstadoGeneral: ua.EstadoGeneral,
			RiesgoNivel:   ua.RiesgoNivel,
			RiesgoMotivo:  ua.RiesgoMotivo,
		}
	}

	return req
}

// indexValidRange is the expected value range for normalized indices.
// Values outside [-1, 1] indicate nodata or division-by-zero artifacts and
// are excluded from the IA payload to avoid misleading the model.
const indexValidMin, indexValidMax = -1.0, 1.0

// indexSaturationThreshold excludes means that are suspiciously close to the
// clip boundaries (±1), which indicates that most pixels were clamped to nodata
// or sensor saturation rather than real reflectance values.
const indexSaturationThreshold = 0.98

// indexMeans returns a map of index name → rounded mean, keeping only the
// mean value (the IA doesn't need std/min/max/percentiles).
// Means outside the valid index range, or saturated at ±1 (≥ 0.98 in abs),
// are omitted — they indicate GDAL nodata or division-by-zero artifacts.
func indexMeans(indices map[string]IndexStats) map[string]float64 {
	out := make(map[string]float64, len(indices))
	for name, s := range indices {
		m := s.Mean
		if m < indexValidMin || m > indexValidMax {
			continue
		}
		if m >= indexSaturationThreshold || m <= -indexSaturationThreshold {
			continue
		}
		out[name] = roundTo(m, 4)
	}
	return out
}

// buildHistorico builds the compact historical summary from the full historico
// chain stored in params (entries[0] = most recent, since BuildParams prepends).
func buildHistorico(entries []HistoricoEntry) IARequestHistorico {
	n := len(entries)
	h := IARequestHistorico{NTotal: n}
	if n == 0 {
		return h
	}

	end := recentHistoricoWindow
	if end > n {
		end = n
	}
	h.Recientes = make([]IARequestHistoricoReciente, 0, end)
	for _, e := range entries[:end] {
		h.Recientes = append(h.Recientes, IARequestHistoricoReciente{
			Fecha:    e.SceneDate,
			Indices:  indexMeans(e.Indices),
			CloudPct: roundTo(e.CloudCoverBBox, 2),
		})
	}

	// Season trend: average mean-change per scene for each key index.
	keyIndices := []string{"ndvi", "evi", "gndvi", "ndre", "nbr", "ndmi", "savi"}
	tendencia := make(map[string]float64)
	for _, key := range keyIndices {
		deltaKey := key + "_mean_change"
		var sum float64
		var cnt int
		for _, e := range entries {
			if v, ok := e.Delta[deltaKey]; ok {
				sum += v
				cnt++
			}
		}
		if cnt > 0 {
			tendencia[key] = roundTo(sum/float64(cnt), 4)
		}
	}
	if len(tendencia) > 0 {
		h.Tendencia = tendencia
	}

	return h
}

// MarshalIARequest serializes req to indented JSON with UTF-8 preserved
// (no HTML escaping so accented characters like á, é, ó stay readable).
func MarshalIARequest(req *IARequest) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(req); err != nil {
		return nil, err
	}
	// Encode appends a trailing newline — trim it for consistency with writeJSON.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
