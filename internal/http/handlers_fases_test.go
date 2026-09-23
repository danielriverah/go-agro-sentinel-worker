package http

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/database"
)

type mockFaseRepo struct {
	fases      []*domain.FaseCultivo
	plantillas []*database.PlantillaFases
	disponible bool
	guardadas  []*domain.FaseCultivo
	replaceErr error
}

func (m *mockFaseRepo) ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.FaseCultivo, error) {
	return m.fases, nil
}

func (m *mockFaseRepo) ReplaceForProduccion(ctx context.Context, produccionID int64, fases []*domain.FaseCultivo) error {
	if m.replaceErr != nil {
		return m.replaceErr
	}
	m.guardadas = fases
	return nil
}

func (m *mockFaseRepo) ListPlantillas(ctx context.Context) ([]*database.PlantillaFases, error) {
	return m.plantillas, nil
}

func (m *mockFaseRepo) Disponible() bool { return m.disponible }

func fasesHandlers(fr *mockFaseRepo) *Handlers {
	return &Handlers{
		Productions: &mockProductionRepo{
			byID: map[int64]*domain.Production{7: {ID: 7, ProduccionID: 4242}},
		},
		Fases: fr,
		Log:   slog.Default(),
	}
}

func putFases(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("PUT", "/api/v1/producciones/7/fases", bytes.NewBufferString(body))
	req.SetPathValue("id", "7")
	w := httptest.NewRecorder()
	h.PutProduccionFases(w, req)
	return w
}

// Aceptar en silencio una escritura que no se guarda es peor que rechazarla:
// el usuario configuraría las fases y las perdería sin enterarse.
func TestPutFasesRechazaSinTabla(t *testing.T) {
	h := fasesHandlers(&mockFaseRepo{disponible: false})

	w := putFases(t, h, `{"fases":[{"nombre":"Crecimiento","dia_inicio":10,"dia_fin":60}]}`)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestPutFasesRechazaSolapamiento(t *testing.T) {
	fr := &mockFaseRepo{disponible: true}
	h := fasesHandlers(fr)

	w := putFases(t, h, `{"fases":[
		{"nombre":"Crecimiento","dia_inicio":10,"dia_fin":60},
		{"nombre":"Floracion","dia_inicio":50,"dia_fin":90}
	]}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if fr.guardadas != nil {
		t.Error("no debería haber guardado nada tras rechazar el solapamiento")
	}
}

func TestPutFasesRechazaRangoInvertido(t *testing.T) {
	fr := &mockFaseRepo{disponible: true}
	h := fasesHandlers(fr)

	w := putFases(t, h, `{"fases":[{"nombre":"Madurez","dia_inicio":90,"dia_fin":60}]}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPutFasesGuardaContraProduccionIDdelERP(t *testing.T) {
	fr := &mockFaseRepo{disponible: true}
	h := fasesHandlers(fr)

	w := putFases(t, h, `{"fases":[
		{"nombre":"Siembra","dia_inicio":0,"dia_fin":10},
		{"nombre":"Crecimiento","dia_inicio":10,"dia_fin":60}
	]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if len(fr.guardadas) != 2 {
		t.Fatalf("guardadas = %d, want 2", len(fr.guardadas))
	}
	// La ruta lleva el s3_monitoring_produccion_id (7), pero las fases cuelgan
	// del produccion_id del ERP (4242) para sobrevivir al borrado del monitoreo.
	if got := fr.guardadas[0].ProduccionID; got != 4242 {
		t.Errorf("produccion_id = %d, want 4242 (el del ERP, no el del monitoreo)", got)
	}
}

// Tramos contiguos (fin de una = inicio de la siguiente) son válidos: no se
// solapan, sólo se tocan.
func TestPutFasesAceptaTramosContiguos(t *testing.T) {
	fr := &mockFaseRepo{disponible: true}
	h := fasesHandlers(fr)

	w := putFases(t, h, `{"fases":[
		{"nombre":"A","dia_inicio":0,"dia_fin":30},
		{"nombre":"B","dia_inicio":30,"dia_fin":60}
	]}`)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}
