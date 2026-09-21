package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

// --- mocks ---

type mockProductionRepo struct {
	listActive    []*domain.Production
	listActiveErr error
	byID          map[int64]*domain.Production
	getErr        error
	setBloqueado  bool
	setBloqID     int64
	setBloqErr    error
}

func (m *mockProductionRepo) ListActive(ctx context.Context) ([]*domain.Production, error) {
	return m.listActive, m.listActiveErr
}

func (m *mockProductionRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.byID[produccionID], nil
}

func (m *mockProductionRepo) GetByMonitoringID(ctx context.Context, monitoringID uint) (*domain.Production, error) {
	return nil, nil
}

// GetByID looks up by the monitoring PK. The fixtures are keyed by ERP
// produccion_id, so scan for the matching record.
func (m *mockProductionRepo) GetByID(ctx context.Context, id uint) (*domain.Production, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, p := range m.byID {
		if p != nil && p.ID == id {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockProductionRepo) GetStatsForActive(ctx context.Context) (map[uint]*domain.ProductionStats, error) {
	return map[uint]*domain.ProductionStats{}, nil
}

func (m *mockProductionRepo) UpdateIAuto(ctx context.Context, produccionID int64, iaAuto bool) error {
	if p, ok := m.byID[produccionID]; ok && p != nil {
		p.IAuto = iaAuto
	}
	return nil
}

func (m *mockProductionRepo) UpdatePolygon(ctx context.Context, monitoringID uint, poligono, pbox []byte) error {
	for _, p := range m.byID {
		if p != nil && p.ID == monitoringID {
			p.PoligonoJSON = poligono
			p.PBoxJSON = pbox
			p.PolygonBBoxJSON = pbox
			return nil
		}
	}
	return nil
}

func (m *mockProductionRepo) SetBloqueado(ctx context.Context, produccionID int64, bloqueado bool) error {
	if m.setBloqErr != nil {
		return m.setBloqErr
	}
	m.setBloqueado = bloqueado
	m.setBloqID = produccionID
	return nil
}

type mockSceneRepo struct {
	byMonitoring map[uint][]*domain.Scene
	err          error
}

func (m *mockSceneRepo) ListByMonitoringProduccion(ctx context.Context, monitoringProduccionID uint) ([]*domain.Scene, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.byMonitoring[monitoringProduccionID], nil
}

func (m *mockSceneRepo) GetByID(ctx context.Context, id uint64) (*domain.Scene, error) {
	return nil, nil
}

type mockFileRepo struct {
	byEscena map[uint64]*domain.SceneFile
	err      error
}

func (m *mockFileRepo) ListByEscena(ctx context.Context, escenaID uint64) ([]*domain.SceneFile, error) {
	return nil, nil
}

func (m *mockFileRepo) GetByTipo(ctx context.Context, escenaID uint64, tipo string) (*domain.SceneFile, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.byEscena[escenaID], nil
}

type mockPresigner struct {
	url string
	err error
}

func (m *mockPresigner) PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.url, nil
}

type mockSyncer struct {
	called chan struct{}
	err    error
}

func (m *mockSyncer) RunOnce(ctx context.Context) error {
	if m.called != nil {
		close(m.called)
	}
	return m.err
}

func (m *mockSyncer) Status() any {
	return nil
}

type mockDBPinger struct {
	err error
}

func (m *mockDBPinger) PingContext(ctx context.Context) error {
	return m.err
}

type mockS3Checker struct {
	err error
}

func (m *mockS3Checker) HeadBucket(ctx context.Context, bucket string) error {
	return m.err
}

type mockDynamoDBChecker struct {
	err error
}

func (m *mockDynamoDBChecker) DescribeTable(ctx context.Context, tableName string) error {
	return m.err
}

type mockGDALExecutor struct {
	version string
	err     error
}

func (m *mockGDALExecutor) Run(ctx context.Context) (string, error) {
	return m.version, m.err
}

// --- tests ---

func TestListProducciones(t *testing.T) {
	prodRepo := &mockProductionRepo{
		listActive: []*domain.Production{
			{ProduccionID: 1, Cosecha: "soja"},
			{ProduccionID: 2, Cosecha: "maiz"},
		},
	}
	h := &Handlers{Productions: prodRepo}

	req := httptest.NewRequest("GET", "/api/v1/producciones", nil)
	w := httptest.NewRecorder()

	h.ListProducciones(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data []domain.Production `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("got %d producciones, want 2", len(body.Data))
	}
}

func TestGetProduccion(t *testing.T) {
	prodRepo := &mockProductionRepo{
		byID: map[int64]*domain.Production{
			42: {ID: 42, ProduccionID: 42, Cosecha: "soja"},
		},
	}
	sceneRepo := &mockSceneRepo{
		byMonitoring: map[uint][]*domain.Scene{
			42: {{ID: 1, MonitoringProduccionID: 42, SceneName: "s1"}},
		},
	}
	h := &Handlers{Productions: prodRepo, Scenes: sceneRepo}

	req := httptest.NewRequest("GET", "/api/v1/producciones/42", nil)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	h.GetProduccion(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data struct {
			ProduccionID int64          `json:"ProduccionID"`
			Escenas      []domain.Scene `json:"escenas"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.ProduccionID != 42 {
		t.Errorf("produccion_id = %d, want 42", body.Data.ProduccionID)
	}
	if len(body.Data.Escenas) != 1 {
		t.Fatalf("got %d escenas, want 1", len(body.Data.Escenas))
	}
}

func TestGetProduccionNotFound(t *testing.T) {
	h := &Handlers{Productions: &mockProductionRepo{byID: map[int64]*domain.Production{}}}

	req := httptest.NewRequest("GET", "/api/v1/producciones/99", nil)
	req.SetPathValue("id", "99")
	w := httptest.NewRecorder()

	h.GetProduccion(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Result().StatusCode)
	}
}

// The {id} in the URL is the monitoring PK, while SetBloqueado takes the ERP
// produccion_id. The fixture keeps them different on purpose so a regression
// that mixes the two identifiers fails here.
func TestDesbloquearProduccion(t *testing.T) {
	prodRepo := &mockProductionRepo{
		byID: map[int64]*domain.Production{
			2007: {ID: 7, ProduccionID: 2007, Bloqueado: true},
		},
	}
	h := &Handlers{Productions: prodRepo}

	req := httptest.NewRequest("POST", "/api/v1/producciones/7/desbloquear", nil)
	req.SetPathValue("id", "7")
	w := httptest.NewRecorder()

	h.DesbloquearProduccion(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if prodRepo.setBloqueado != false {
		t.Error("expected SetBloqueado(false) to be called")
	}
	if prodRepo.setBloqID != 2007 {
		t.Errorf("SetBloqueado got produccion_id %d, want the ERP id 2007", prodRepo.setBloqID)
	}
}

func TestGetEscenaArchivo(t *testing.T) {
	fileRepo := &mockFileRepo{
		byEscena: map[uint64]*domain.SceneFile{
			5: {ID: 1, EscenaID: 5, Tipo: "ndvi", S3Key: "key/ndvi.tif", S3Uri: "s3://bucket/key/ndvi.tif"},
		},
	}
	presigner := &mockPresigner{url: "https://example.com/presigned"}
	h := &Handlers{Files: fileRepo, S3: presigner, S3Bucket: "default-bucket"}

	req := httptest.NewRequest("GET", "/api/v1/escenas/5/archivos/ndvi", nil)
	req.SetPathValue("id", "5")
	req.SetPathValue("tipo", "ndvi")
	w := httptest.NewRecorder()

	h.GetEscenaArchivo(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data archivoResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.URL != "https://example.com/presigned" {
		t.Errorf("url = %q, want presigned url", body.Data.URL)
	}
}

func TestGetEscenaArchivoNotFound(t *testing.T) {
	h := &Handlers{Files: &mockFileRepo{byEscena: map[uint64]*domain.SceneFile{}}}

	req := httptest.NewRequest("GET", "/api/v1/escenas/5/archivos/ndvi", nil)
	req.SetPathValue("id", "5")
	req.SetPathValue("tipo", "ndvi")
	w := httptest.NewRecorder()

	h.GetEscenaArchivo(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Result().StatusCode)
	}
}

func TestTriggerSync(t *testing.T) {
	called := make(chan struct{})
	h := &Handlers{Sync: &mockSyncer{called: called}}

	req := httptest.NewRequest("POST", "/api/v1/sync/trigger", nil)
	w := httptest.NewRecorder()

	h.TriggerSync(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Error("expected RunOnce to be called")
	}
}

func TestTriggerSyncNotConfigured(t *testing.T) {
	h := &Handlers{}

	req := httptest.NewRequest("POST", "/api/v1/sync/trigger", nil)
	w := httptest.NewRecorder()

	h.TriggerSync(w, req)

	if w.Result().StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Result().StatusCode)
	}
}

func TestHealthDependenciesAllOk(t *testing.T) {
	h := &Handlers{
		DB:        &mockDBPinger{err: nil},
		GDAL:      &mockGDALExecutor{version: "GDAL 3.9.3", err: nil},
		S3Health:  &mockS3Checker{err: nil},
		DynamoDB:  &mockDynamoDBChecker{err: nil},
		S3Bucket:  "test-bucket",
	}

	req := httptest.NewRequest("GET", "/health/dependencies", nil)
	w := httptest.NewRecorder()

	h.HealthDependencies(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data map[string]healthDependencyStatus `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	deps := body.Data
	if deps["mysql"].Status != "ok" {
		t.Errorf("mysql status = %q, want ok", deps["mysql"].Status)
	}
	if deps["gdal"].Status != "ok" {
		t.Errorf("gdal status = %q, want ok", deps["gdal"].Status)
	}
	if deps["gdal"].Version != "GDAL 3.9.3" {
		t.Errorf("gdal version = %q, want GDAL 3.9.3", deps["gdal"].Version)
	}
	if deps["s3"].Status != "ok" {
		t.Errorf("s3 status = %q, want ok", deps["s3"].Status)
	}
	if deps["dynamodb"].Status != "ok" {
		t.Errorf("dynamodb status = %q, want ok", deps["dynamodb"].Status)
	}
}

func TestHealthDependenciesWithErrors(t *testing.T) {
	h := &Handlers{
		DB:        &mockDBPinger{err: nil},
		GDAL:      &mockGDALExecutor{version: "", err: context.DeadlineExceeded},
		S3Health:  &mockS3Checker{err: nil},
		DynamoDB:  &mockDynamoDBChecker{err: context.DeadlineExceeded},
		S3Bucket:  "test-bucket",
	}

	req := httptest.NewRequest("GET", "/health/dependencies", nil)
	w := httptest.NewRecorder()

	h.HealthDependencies(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 even with errors", resp.StatusCode)
	}

	var body struct {
		Data map[string]healthDependencyStatus `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	deps := body.Data
	if deps["gdal"].Status != "error" {
		t.Errorf("gdal status = %q, want error", deps["gdal"].Status)
	}
	if deps["dynamodb"].Status != "error" {
		t.Errorf("dynamodb status = %q, want error", deps["dynamodb"].Status)
	}
}

func TestHealthDependenciesNotConfigured(t *testing.T) {
	h := &Handlers{}

	req := httptest.NewRequest("GET", "/health/dependencies", nil)
	w := httptest.NewRecorder()

	h.HealthDependencies(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data map[string]healthDependencyStatus `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	deps := body.Data
	if deps["mysql"].Status != "error" {
		t.Errorf("mysql status = %q, want error when not configured", deps["mysql"].Status)
	}
	if !strings.Contains(deps["mysql"].Message, "not configured") {
		t.Errorf("mysql message = %q, want 'not configured'", deps["mysql"].Message)
	}
}
