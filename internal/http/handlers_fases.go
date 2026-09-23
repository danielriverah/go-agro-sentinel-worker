package http

import (
	"context"
	"encoding/json"
	"net/http"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/database"
)

// FaseRepository es el subconjunto de database.FaseRepo que usa la API.
type FaseRepository interface {
	ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.FaseCultivo, error)
	ListPlantillas(ctx context.Context) ([]*database.PlantillaFases, error)
	ReplaceForProduccion(ctx context.Context, produccionID int64, fases []*domain.FaseCultivo) error
	Disponible() bool
}

// ListPlantillasFases maneja GET /api/v1/fases/plantillas.
//
// Devuelve únicamente las producciones que ya tienen fases, con sus fases
// incluidas: así el selector de "copiar de" no ofrece opciones vacías y se
// pueden revisar antes de aplicarlas.
func (h *Handlers) ListPlantillasFases(w http.ResponseWriter, r *http.Request) {
	plantillas, err := h.Fases.ListPlantillas(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "listando plantillas de fases: "+err.Error())
		return
	}
	if plantillas == nil {
		plantillas = []*database.PlantillaFases{}
	}
	JSON(w, http.StatusOK, plantillas)
}

// faseInput es una fase tal como la manda el editor. El id no viaja: el PUT
// reemplaza el conjunto completo.
type faseInput struct {
	Nombre    string `json:"nombre"`
	DiaInicio int    `json:"dia_inicio"`
	DiaFin    int    `json:"dia_fin"`
}

type putFasesRequest struct {
	Fases []faseInput `json:"fases"`
}

// GetProduccionFases maneja GET /api/v1/producciones/{id}/fases.
// {id} es el s3_monitoring_produccion_id, igual que el resto de rutas de
// producción, aunque las fases se guarden contra el produccion_id del ERP.
func (h *Handlers) GetProduccionFases(w http.ResponseWriter, r *http.Request) {
	prod, ok := h.produccionDesdeRuta(w, r)
	if !ok {
		return
	}

	fases, err := h.Fases.ListByProduccion(r.Context(), prod.ProduccionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "listando fases: "+err.Error())
		return
	}

	// Siempre un arreglo: el frontend no debe distinguir entre "sin fases" y
	// "la tabla aún no existe" para dibujar la gráfica.
	if fases == nil {
		fases = []*domain.FaseCultivo{}
	}
	JSON(w, http.StatusOK, fases)
}

// PutProduccionFases maneja PUT /api/v1/producciones/{id}/fases y reemplaza el
// conjunto completo de fases de la producción.
func (h *Handlers) PutProduccionFases(w http.ResponseWriter, r *http.Request) {
	prod, ok := h.produccionDesdeRuta(w, r)
	if !ok {
		return
	}

	// En lectura la ausencia de tabla se disimula con una lista vacía, pero en
	// escritura hay que decirlo: aceptar en silencio algo que no se guarda es
	// peor que rechazarlo.
	if !h.Fases.Disponible() {
		Error(w, http.StatusServiceUnavailable,
			"la tabla produccion_fases_cultivo no existe todavía; aplica scripts/phase3-create-produccion-fases-cultivo.sql")
		return
	}

	var req putFasesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "cuerpo inválido: "+err.Error())
		return
	}

	fases := make([]*domain.FaseCultivo, 0, len(req.Fases))
	for _, in := range req.Fases {
		f := &domain.FaseCultivo{
			ProduccionID: prod.ProduccionID,
			Nombre:       in.Nombre,
			DiaInicio:    in.DiaInicio,
			DiaFin:       in.DiaFin,
		}
		if err := f.Validate(); err != nil {
			Error(w, http.StatusBadRequest, err.Error())
			return
		}
		fases = append(fases, f)
	}

	if err := solapan(fases); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Fases.ReplaceForProduccion(r.Context(), prod.ProduccionID, fases); err != nil {
		Error(w, http.StatusInternalServerError, "guardando fases: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, fases)
}

// solapan rechaza tramos que se pisan entre sí. Dos fases activas el mismo día
// pintarían franjas superpuestas y harían ambigua la lectura de la gráfica.
func solapan(fases []*domain.FaseCultivo) error {
	for i := 0; i < len(fases); i++ {
		for j := i + 1; j < len(fases); j++ {
			a, b := fases[i], fases[j]
			if a.DiaInicio < b.DiaFin && b.DiaInicio < a.DiaFin {
				return &domain.ProcessingError{
					Type:    domain.ErrValidation,
					Message: "las fases " + a.Nombre + " y " + b.Nombre + " se solapan",
				}
			}
		}
	}
	return nil
}

// produccionDesdeRuta resuelve {id} y responde el error apropiado.
func (h *Handlers) produccionDesdeRuta(w http.ResponseWriter, r *http.Request) (*domain.Production, bool) {
	id, ok := parseUintParam(w, r, "id")
	if !ok {
		return nil, false
	}

	prod, err := h.Productions.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
		return nil, false
	}
	if prod == nil {
		Error(w, http.StatusNotFound, "produccion not found")
		return nil, false
	}
	return prod, true
}
