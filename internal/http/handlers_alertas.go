package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"agro-sentinel-worker/internal/auth"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/database"
)

// AlertaRepository es el subconjunto de database.AlertaRepo que usa la API.
type AlertaRepository interface {
	List(ctx context.Context, f database.FiltroAlertas) ([]*domain.Alerta, error)
	MarcarVista(ctx context.Context, id uint64, usuario string) error
	MarcarResuelta(ctx context.Context, id uint64, usuario string) error
	Disponible() bool
}

// ListAlertas maneja GET /api/v1/alertas.
//
// Filtros por query string: estado, severidad (ambos admiten lista separada
// por comas), produccion_id y limite. Sin filtros devuelve las más recientes.
func (h *Handlers) ListAlertas(w http.ResponseWriter, r *http.Request) {
	if !h.Alertas.Disponible() {
		Error(w, http.StatusServiceUnavailable,
			"la tabla monitoring_alertas no existe todavía")
		return
	}

	q := r.URL.Query()
	filtro := database.FiltroAlertas{
		Estados:     listaCSV(q.Get("estado")),
		Severidades: listaCSV(q.Get("severidad")),
	}
	if v := q.Get("produccion_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filtro.ProduccionID = n
		}
	}
	if v := q.Get("limite"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filtro.Limite = n
		}
	}

	alertas, err := h.Alertas.List(r.Context(), filtro)
	if err != nil {
		Error(w, http.StatusInternalServerError, "listando alertas: "+err.Error())
		return
	}
	if alertas == nil {
		alertas = []*domain.Alerta{}
	}
	JSON(w, http.StatusOK, alertas)
}

// MarcarAlertaVista maneja POST /api/v1/alertas/{id}/vista.
func (h *Handlers) MarcarAlertaVista(w http.ResponseWriter, r *http.Request) {
	h.cambiarEstadoAlerta(w, r, false)
}

// MarcarAlertaResuelta maneja POST /api/v1/alertas/{id}/resuelta.
func (h *Handlers) MarcarAlertaResuelta(w http.ResponseWriter, r *http.Request) {
	h.cambiarEstadoAlerta(w, r, true)
}

func (h *Handlers) cambiarEstadoAlerta(w http.ResponseWriter, r *http.Request, resolver bool) {
	if !h.Alertas.Disponible() {
		Error(w, http.StatusServiceUnavailable, "la tabla monitoring_alertas no existe todavía")
		return
	}

	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		Error(w, http.StatusBadRequest, "id de alerta inválido")
		return
	}

	// Quién la atendió sale de la sesión, no del cuerpo: así no se puede
	// atribuir la acción a otra persona.
	var usuario string
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		usuario = claims.Username
	}

	if resolver {
		err = h.Alertas.MarcarResuelta(r.Context(), id, usuario)
	} else {
		err = h.Alertas.MarcarVista(r.Context(), id, usuario)
	}
	if err != nil {
		Error(w, http.StatusInternalServerError, "actualizando alerta: "+err.Error())
		return
	}

	estado := domain.AlertaVista
	if resolver {
		estado = domain.AlertaResuelta
	}
	JSON(w, http.StatusOK, map[string]any{"monitoring_alerta_id": id, "estado": estado})
}

// listaCSV parte "nueva,vista" en sus elementos, descartando los vacíos.
func listaCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
