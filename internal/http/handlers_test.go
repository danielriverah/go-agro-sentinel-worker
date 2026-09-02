package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	listActive     []*domain.Production
	listActiveErr  error
	byID           map[int64]*domain.Production
	getErr         error
	desbloquearErr error
	desbloqueado   bool
	desbloqUsuario string
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

func (m *mockProductionRepo) Desbloquear(ctx context.Context, produccionID int64, usuario string) error {
	if m.desbloquearErr != nil {
		return m.desbloquearErr
	}
	m.desbloqueado = true
	m.desbloqUsuario = usuario
	return nil
}

type mockSceneRepo struct {
	byProduccion map[int64][]*domain.Scene
	err          error
}

func (m *mockSceneRepo) ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.Scene, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.byProduccion[produccionID], nil
}

func (m *mockSceneRepo) GetByID(ctx context.Context, id int64) (*domain.Scene, error) {
	return nil, nil
}

type mockFileRepo struct {
	byType map[int64]*domain.SceneFile
	err    error
}

func (m *mockFileRepo) ListByEscena(ctx context.Context, escenaID int64) ([]*domain.SceneFile, error) {
	return nil, nil
}

func (m *mockFileRepo) GetByType(ctx context.Context, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.byType[escenaID], nil
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

// --- tests ---

func TestListProducciones(t *testing.T) {
	prodRepo := &mockProductionRepo{
		listActive: []*domain.Production{
			{ProduccionID: 1, Cultivo: "soja"},
			{ProduccionID: 2, Cultivo: "maiz"},
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
			42: {ProduccionID: 42, Cultivo: "soja"},
		},
	}
	sceneRepo := &mockSceneRepo{
		byProduccion: map[int64][]*domain.Scene{
			42: {{ID: 1, ProduccionID: 42, SceneID: "s1"}},
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

func TestDesbloquearProduccion(t *testing.T) {
	prodRepo := &mockProductionRepo{
		byID: map[int64]*domain.Production{
			7: {ProduccionID: 7, Bloqueado: false},
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
	if !prodRepo.desbloqueado {
		t.Error("expected Desbloquear to be called")
	}
}

func TestGetEscenaArchivo(t *testing.T) {
	fileRepo := &mockFileRepo{
		byType: map[int64]*domain.SceneFile{
			5: {ID: 1, EscenaID: 5, FileType: domain.FileNDVI, FileName: "ndvi.tif", S3Key: "key", S3Bucket: "bucket"},
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
	h := &Handlers{Files: &mockFileRepo{byType: map[int64]*domain.SceneFile{}}}

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
