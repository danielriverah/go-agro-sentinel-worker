package http

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"agro-sentinel-worker/internal/daemon"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/database"
)

// Los mocks anotan el orden real de las llamadas: en una operación que no
// puede ser atómica entre tres sistemas, el orden ES la garantía.
type deleteSpy struct {
	orden      []string
	s3Err      error
	dynamoErr  error
	mysqlErr   error
	sinBBox    bool
}

func (s *deleteSpy) DeletePrefix(ctx context.Context, bucket, prefix string) (int, error) {
	s.orden = append(s.orden, "s3")
	if s.s3Err != nil {
		return 0, s.s3Err
	}
	return 7, nil
}

func (s *deleteSpy) DeleteEscenas(ctx context.Context, table string, id int64) (int, error) {
	s.orden = append(s.orden, "dynamo_escenas")
	if s.dynamoErr != nil {
		return 0, s.dynamoErr
	}
	return 3, nil
}

func (s *deleteSpy) DeleteProduccion(ctx context.Context, table string, id int64, folio string) error {
	s.orden = append(s.orden, "dynamo_produccion")
	return s.dynamoErr
}

func (s *deleteSpy) EliminarMonitoreo(ctx context.Context, mid uint, pid int64) (database.ResumenBorradoMySQL, error) {
	s.orden = append(s.orden, "mysql")
	if s.mysqlErr != nil {
		return database.ResumenBorradoMySQL{}, s.mysqlErr
	}
	return database.ResumenBorradoMySQL{Escenas: 3, Dependientes: 1, ERPApagado: true}, nil
}

func (s *deleteSpy) HasImageBBox() bool { return !s.sinBBox }

func deleteHandlers(spy *deleteSpy) *Handlers {
	return deleteHandlersCon(spy, true)
}

// bloqueada refleja que la producción esté marcada como posible cosecha: sólo
// entonces se permite borrar su monitoreo.
func deleteHandlersCon(spy *deleteSpy, bloqueada bool) *Handlers {
	return &Handlers{
		Productions: &mockProductionRepo{
			byID: map[int64]*domain.Production{
				7: {ID: 7, ProduccionID: 4242, Folio: "CSJ-001", Prefix: "produccion/4242",
					Bloqueado: bloqueada, PosibleCosecha: bloqueada},
			},
		},
		S3Delete:                spy,
		DynamoDelete:            spy,
		Monitoreo:               spy,
		S3Bucket:                "bucket-test",
		DynamoTablaProducciones: "monitoring_producciones",
		DynamoTablaEscenas:      "monitoring_escenas",
		Log:                     slog.Default(),
	}
}

func borrar(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("DELETE", "/api/v1/producciones/7/monitoreo", bytes.NewBufferString(body))
	req.SetPathValue("id", "7")
	w := httptest.NewRecorder()
	h.EliminarMonitoreo(w, req)
	return w
}

// requiereLocks salta la prueba si el directorio de locks no está disponible
// en esta máquina: el borrado toma un lock real de producción.
func requiereLocks(t *testing.T) {
	t.Helper()
	lock, err := daemon.AcquireProduction(999999)
	if err != nil || lock == nil {
		t.Skip("los locks de producción no están disponibles en este entorno")
	}
	lock.Release()
}

func TestEliminarMonitoreoRechazaFolioIncorrecto(t *testing.T) {
	spy := &deleteSpy{}
	h := deleteHandlers(spy)

	w := borrar(t, h, `{"folio":"OTRO-FOLIO"}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if len(spy.orden) != 0 {
		t.Errorf("no debió tocar ningún sistema, pero llamó a %v", spy.orden)
	}
}

// Una producción en seguimiento activo no se borra: sus datos siguen en uso.
// bloqueado lo deriva un trigger de posible_cosecha, así que exigirlo equivale
// a pedir que el lote esté marcado como cosechado.
func TestEliminarMonitoreoRechazaProduccionNoBloqueada(t *testing.T) {
	spy := &deleteSpy{}
	h := deleteHandlersCon(spy, false)

	w := borrar(t, h, `{"folio":"CSJ-001"}`)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
	if len(spy.orden) != 0 {
		t.Errorf("no debió tocar ningún sistema, pero llamó a %v", spy.orden)
	}
}

func TestEliminarMonitoreoRechazaSinImageBBox(t *testing.T) {
	spy := &deleteSpy{sinBBox: true}
	h := deleteHandlers(spy)

	w := borrar(t, h, `{"folio":"CSJ-001"}`)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	// Sin la columna, las escenas dependientes perderían su georreferencia.
	if len(spy.orden) != 0 {
		t.Errorf("no debió borrar nada sin image_bbox, pero llamó a %v", spy.orden)
	}
}

func TestEliminarMonitoreoOrdenS3AntesQueMySQL(t *testing.T) {
	requiereLocks(t)

	spy := &deleteSpy{}
	h := deleteHandlers(spy)

	w := borrar(t, h, `{"folio":"CSJ-001"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	quiero := []string{"s3", "dynamo_escenas", "dynamo_produccion", "mysql"}
	if len(spy.orden) != len(quiero) {
		t.Fatalf("orden = %v, want %v", spy.orden, quiero)
	}
	for i := range quiero {
		if spy.orden[i] != quiero[i] {
			t.Fatalf("orden = %v, want %v", spy.orden, quiero)
		}
	}
}

// Si S3 falla, MySQL no debe tocarse: los registros son la única pista para
// localizar y reintentar lo que quedó a medias.
func TestEliminarMonitoreoFalloS3NoTocaMySQL(t *testing.T) {
	requiereLocks(t)

	spy := &deleteSpy{s3Err: errors.New("s3 caído")}
	h := deleteHandlers(spy)

	w := borrar(t, h, `{"folio":"CSJ-001"}`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	for _, paso := range spy.orden {
		if paso == "mysql" {
			t.Fatalf("MySQL no debió ejecutarse tras fallar S3: %v", spy.orden)
		}
	}
}
