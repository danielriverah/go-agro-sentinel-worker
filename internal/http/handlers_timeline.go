package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/processing"
)

// TimelineRepository is the subset of database.SceneRepo the timeline needs.
type TimelineRepository interface {
	ListTimelineRows(ctx context.Context, monitoringProduccionID uint) ([]*domain.TimelineRow, error)
}

// Index keys plotted by the timeline. NDVI and NDMI are shown by default;
// the rest are opt-in series the user can enable.
const (
	idxNDVI = "ndvi"
	idxNDMI = "ndmi"
)

var timelineSeries = []timelineSerie{
	{Clave: idxNDVI, Principal: true, Grupo: "vegetacion"},
	{Clave: idxNDMI, Principal: true, Grupo: "humedad"},
	{Clave: "evi", Grupo: "vegetacion"},
	{Clave: "savi", Grupo: "vegetacion"},
	{Clave: "ndre", Grupo: "vegetacion"},
	{Clave: "gndvi", Grupo: "vegetacion"},
	{Clave: "nbr", Grupo: "humedad"},
}

// Quality thresholds. A scene whose polygon is this cloudy has index means
// contaminated by cloud pixels, which would draw a dip that reads as crop
// stress, so it is excluded from the line.
const (
	maxUsableCloud = 40.0
	noDataCloud    = 101.0
)

// Qualitative cuts for the two headline indices. Generic starting values:
// real thresholds vary by crop and growth stage, so they live here in one
// place to be tuned against field data later.
const (
	ndviBajo      = 0.30
	ndviModerado  = 0.55
	ndmiSeco      = 0.10
	ndmiHumedo    = 0.40
	trendWindow   = 3
	trendMinDelta = 0.02
)

type timelineSerie struct {
	Clave     string `json:"clave"`
	Principal bool   `json:"principal"`
	Grupo     string `json:"grupo"`
}

type timelineProduccion struct {
	ID              uint    `json:"id"`
	Folio           string  `json:"folio"`
	Rancho          string  `json:"rancho"`
	Cosecha         string  `json:"cosecha"`
	Variedades      string  `json:"variedades"`
	FechaPlantacion *string `json:"fecha_plantacion"`
	FechaFin        *string `json:"fecha_fin"`
}

type timelineExtremo struct {
	Fecha string  `json:"fecha"`
	Valor float64 `json:"valor"`
}

type timelineResumen struct {
	PuntosTotales    int              `json:"puntos_totales"`
	PuntosConfiables int              `json:"puntos_confiables"`
	MejorDia         *timelineExtremo `json:"mejor_dia"`
	PeorDia          *timelineExtremo `json:"peor_dia"`
	Tendencia        string           `json:"tendencia"`
}

type timelineIA struct {
	Estado string `json:"estado"`
	Riesgo string `json:"riesgo"`
	Motivo string `json:"motivo,omitempty"`
}

type timelinePunto struct {
	EscenaID          uint64             `json:"escena_id"`
	SceneName         string             `json:"scene_name"`
	Fecha             string             `json:"fecha"`
	DiaCultivo        *int               `json:"dia_cultivo"`
	DiasACosecha      *int               `json:"dias_a_cosecha"`
	Nubosidad         *float64           `json:"nubosidad"`
	Confiable         bool               `json:"confiable"`
	MotivoNoConfiable string             `json:"motivo_no_confiable,omitempty"`
	Valores           map[string]float64 `json:"valores,omitempty"`
	Delta             map[string]float64 `json:"delta,omitempty"`
	Estado            map[string]string  `json:"estado,omitempty"`
	IA                *timelineIA        `json:"ia,omitempty"`
}

type timelineFase struct {
	Nombre    string `json:"nombre"`
	DiaInicio int    `json:"dia_inicio"`
	DiaFin    int    `json:"dia_fin"`
	Color     string `json:"color"`
}

type timelineResponse struct {
	Produccion timelineProduccion `json:"produccion"`
	Resumen    timelineResumen    `json:"resumen"`
	Series     []timelineSerie    `json:"series"`
	Puntos     []timelinePunto    `json:"puntos"`
	Fases      []timelineFase     `json:"fases"`
}

// GetProduccionTimeline handles GET /api/v1/producciones/{id}/timeline.
// {id} is the s3_monitoring_produccion_id, matching GetProduccion.
//
// Every value comes from the params.json already stored in json_content, so
// this serves the whole history from one query without touching S3.
func (h *Handlers) GetProduccionTimeline(w http.ResponseWriter, r *http.Request) {
	production, ok := h.produccionDesdeRuta(w, r)
	if !ok {
		return
	}

	ctx := r.Context()

	rows, err := h.Timeline.ListTimelineRows(ctx, production.ID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "listing timeline: "+err.Error())
		return
	}

	resp := buildTimeline(production, rows)

	// Las fases son opcionales: si la tabla aún no existe o la producción no
	// tiene ninguna configurada, la gráfica se dibuja sin franjas.
	if h.Fases == nil {
		JSON(w, http.StatusOK, resp)
		return
	}
	if fases, err := h.Fases.ListByProduccion(ctx, production.ProduccionID); err != nil {
		h.Log.Warn("no se pudieron cargar las fases de cultivo",
			"produccion_id", production.ProduccionID, "error", err)
	} else {
		resp.Fases = aFasesTimeline(fases)
	}

	JSON(w, http.StatusOK, resp)
}

// faseColors se recorre en orden para que las franjas se distingan entre sí.
// Son tonos suaves: el protagonista de la gráfica es la línea, no el fondo.
var faseColors = []string{"#E5E7EB", "#D1FAE5", "#FEF3C7", "#FED7AA", "#FECACA", "#E9D5FF"}

func aFasesTimeline(fases []*domain.FaseCultivo) []timelineFase {
	out := make([]timelineFase, 0, len(fases))
	for i, f := range fases {
		out = append(out, timelineFase{
			Nombre:    f.Nombre,
			DiaInicio: f.DiaInicio,
			DiaFin:    f.DiaFin,
			Color:     faseColors[i%len(faseColors)],
		})
	}
	return out
}

// buildTimeline turns raw scene rows into the chart payload. Kept separate
// from the handler so it can be tested without HTTP plumbing.
func buildTimeline(prod *domain.Production, rows []*domain.TimelineRow) timelineResponse {
	resp := timelineResponse{
		Produccion: timelineProduccion{
			ID:              prod.ID,
			Folio:           prod.Folio,
			Rancho:          prod.Rancho,
			Cosecha:         prod.Cosecha,
			Variedades:      prod.Vaiedades,
			FechaPlantacion: formatDatePtr(prod.FechaPlantacion),
			FechaFin:        formatDatePtr(prod.FechaFin),
		},
		Series: timelineSeries,
		Puntos: make([]timelinePunto, 0, len(rows)),
		Fases:  []timelineFase{},
	}

	// Deltas compare against the last *trustworthy* reading, not the immediately
	// previous one: chaining through a cloudy scene would report a swing that
	// never happened in the field.
	var lastGood map[string]float64
	var confiables []float64
	var mejor, peor *timelineExtremo

	for _, row := range rows {
		if row.Fecha == nil {
			continue
		}

		pt := timelinePunto{
			EscenaID:  row.EscenaID,
			SceneName: row.SceneName,
			Fecha:     row.Fecha.Format(dateOnly),
			Nubosidad: row.ProductionCloud,
		}
		if pt.Nubosidad == nil {
			pt.Nubosidad = row.CloudCover
		}

		pt.DiaCultivo = daysSince(prod.FechaPlantacion, *row.Fecha)
		pt.DiasACosecha = daysUntil(*row.Fecha, prod.FechaFin)

		valores := parseIndexMeans(row.ParamsJSON)
		pt.Confiable, pt.MotivoNoConfiable = classifyRow(row, valores)

		if !pt.Confiable {
			// Values from an unreliable scene are withheld on purpose: showing
			// them invites reading cloud noise as a real change in the crop.
			resp.Puntos = append(resp.Puntos, pt)
			continue
		}

		pt.Valores = valores
		pt.Estado = map[string]string{
			idxNDVI: classifyNDVI(valores[idxNDVI]),
			idxNDMI: classifyNDMI(valores[idxNDMI]),
		}

		if lastGood != nil {
			delta := make(map[string]float64, len(valores))
			for k, v := range valores {
				if prev, found := lastGood[k]; found {
					delta[k] = round4(v - prev)
				}
			}
			if len(delta) > 0 {
				pt.Delta = delta
			}
		}
		lastGood = valores

		if ndvi, found := valores[idxNDVI]; found {
			confiables = append(confiables, ndvi)
			if mejor == nil || ndvi > mejor.Valor {
				mejor = &timelineExtremo{Fecha: pt.Fecha, Valor: ndvi}
			}
			if peor == nil || ndvi < peor.Valor {
				peor = &timelineExtremo{Fecha: pt.Fecha, Valor: ndvi}
			}
		}

		resp.Puntos = append(resp.Puntos, pt)
	}

	resp.Resumen = timelineResumen{
		PuntosTotales:    len(resp.Puntos),
		PuntosConfiables: len(confiables),
		MejorDia:         mejor,
		PeorDia:          peor,
		Tendencia:        trend(confiables),
	}

	return resp
}

const dateOnly = "2006-01-02"

// parseIndexMeans pulls just the per-index means out of a params.json blob.
// The nested historico[] is deliberately ignored: every scene carries its own
// copy of the previous twenty, so honouring it would multiply the payload for
// data the caller already has point by point.
func parseIndexMeans(raw string) map[string]float64 {
	if raw == "" {
		return nil
	}

	var params processing.Params
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return nil
	}
	if len(params.Indices) == 0 {
		return nil
	}

	means := make(map[string]float64, len(params.Indices))
	for name, stats := range params.Indices {
		means[name] = round4(stats.Mean)
	}
	return means
}

// classifyRow decides whether a scene's readings can be trusted, and why not
// when they cannot.
func classifyRow(row *domain.TimelineRow, valores map[string]float64) (bool, string) {
	if row.ProductionCloud != nil && *row.ProductionCloud == noDataCloud {
		return false, "sin_dato_sensor"
	}
	if len(valores) == 0 {
		return false, "sin_params"
	}
	if row.ProductionCloud != nil && *row.ProductionCloud >= maxUsableCloud {
		return false, "nubosidad_alta"
	}
	if !row.Usable {
		return false, "escena_no_usable"
	}
	return true, ""
}

func classifyNDVI(v float64) string {
	switch {
	case v < ndviBajo:
		return "bajo"
	case v < ndviModerado:
		return "moderado"
	default:
		return "vigoroso"
	}
}

func classifyNDMI(v float64) string {
	switch {
	case v < ndmiSeco:
		return "seco"
	case v < ndmiHumedo:
		return "normal"
	default:
		return "humedo"
	}
}

// trend compares the mean of the last few readings against the ones before
// them. A plain first-to-last comparison would call a recovered crop
// "declining" just because it dipped once in the middle.
func trend(values []float64) string {
	if len(values) < trendWindow*2 {
		return "estable"
	}

	recent := mean(values[len(values)-trendWindow:])
	earlier := mean(values[len(values)-trendWindow*2 : len(values)-trendWindow])

	switch {
	case recent-earlier > trendMinDelta:
		return "mejorando"
	case earlier-recent > trendMinDelta:
		return "declinando"
	default:
		return "estable"
	}
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func daysSince(from *time.Time, to time.Time) *int {
	if from == nil {
		return nil
	}
	d := int(to.Sub(*from).Hours() / 24)
	return &d
}

func daysUntil(from time.Time, to *time.Time) *int {
	if to == nil {
		return nil
	}
	d := int(to.Sub(from).Hours() / 24)
	return &d
}

func formatDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateOnly)
	return &s
}

func round4(v float64) float64 {
	return float64(int64(v*10000+copySign(0.5, v))) / 10000
}

func copySign(magnitude, sign float64) float64 {
	if sign < 0 {
		return -magnitude
	}
	return magnitude
}
