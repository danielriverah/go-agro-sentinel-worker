diff --git a/cmd/worker/main.go b/cmd/worker/main.go
index 9c7097c..e3e9ca3 100644
--- a/cmd/worker/main.go
+++ b/cmd/worker/main.go
@@ -7,20 +7,21 @@ import (
 	"log"
 	"os"
 
 	awssdk "github.com/aws/aws-sdk-go-v2/config"
 
 	"agro-sentinel-worker/internal/config"
 	"agro-sentinel-worker/internal/domain"
 	"agro-sentinel-worker/internal/infrastructure/aws"
 	"agro-sentinel-worker/internal/infrastructure/database"
 	"agro-sentinel-worker/internal/infrastructure/gdal"
+	"agro-sentinel-worker/internal/infrastructure/ia"
 	"agro-sentinel-worker/internal/logger"
 	"agro-sentinel-worker/internal/worker"
 )
 
 // stacBandResolver is a placeholder worker.BandResolver: full STAC/COG
 // discovery is not implemented yet. It fails clearly rather than silently
 // producing an empty band set.
 type stacBandResolver struct{}
 
 func (stacBandResolver) ResolveBands(ctx context.Context, produccionID int64, sceneID string) ([]domain.BandInfo, string, error) {
@@ -69,21 +70,21 @@ func main() {
 	executor := gdal.NewExecutor(cfg.GDAL.TimeoutSeconds)
 	s3Client := aws.NewS3Client(awsCfg)
 
 	deps := worker.WorkerDeps{
 		Productions: database.NewProductionRepo(db),
 		Scenes:      database.NewSceneRepo(db),
 		Files:       database.NewFileRepo(db),
 		S3:          s3Client,
 		Executor:    executor,
 		Bands:       stacBandResolver{},
-		IA:          nil, // wired in Task 13
+		IA:          ia.New(cfg.IA),
 		Processing:  cfg.Processing,
 		Sentinel:    cfg.Sentinel,
 		S3Config:    cfg.S3,
 		Logger:      l,
 	}
 
 	w := worker.New(deps)
 
 	if err := w.ProcessScene(ctx, *produccionID, *sceneID); err != nil {
 		l.Error("scene processing failed", "produccion_id", *produccionID, "scene_id", *sceneID, "error", err)
diff --git a/internal/infrastructure/ia/client.go b/internal/infrastructure/ia/client.go
new file mode 100644
index 0000000..2384a91
--- /dev/null
+++ b/internal/infrastructure/ia/client.go
@@ -0,0 +1,98 @@
+// Package ia implements the HTTP client for the external IA (image analysis)
+// service. The worker calls it after generating params.json for a scene; a
+// failure here is a partial failure per the spec (IA_ERROR), so the scene
+// still completes without analisis.json.
+package ia
+
+import (
+	"bytes"
+	"context"
+	"encoding/json"
+	"fmt"
+	"net/http"
+	"time"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/domain"
+	"agro-sentinel-worker/internal/processing"
+	"agro-sentinel-worker/internal/worker"
+)
+
+// Client is an HTTP client for the IA analysis service.
+type Client struct {
+	cfg        config.IAConfig
+	httpClient *http.Client
+}
+
+// New creates a Client configured from cfg. The underlying HTTP client's
+// timeout is set from cfg.TimeoutSeconds.
+func New(cfg config.IAConfig) *Client {
+	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
+	if timeout <= 0 {
+		timeout = 30 * time.Second
+	}
+
+	return &Client{
+		cfg: cfg,
+		httpClient: &http.Client{
+			Timeout: timeout,
+		},
+	}
+}
+
+// analyzeRequest is the JSON body POSTed to the IA service's /analyze
+// endpoint.
+type analyzeRequest struct {
+	ProduccionID  int64              `json:"produccion_id"`
+	SceneID       string             `json:"scene_id"`
+	MultibandPath string             `json:"multiband_path"`
+	Params        *processing.Params `json:"params,omitempty"`
+}
+
+// Analyze POSTs the given input to {ServiceURL}/analyze and parses the
+// response body as a domain.AnalysisResult. If the IA service is disabled
+// (cfg.Enabled == false), it returns nil, nil immediately without making a
+// request. Any HTTP-level failure (non-2xx status, timeout, connection
+// refused, malformed response) is wrapped as a
+// domain.ProcessingError{Type: domain.ErrIA}.
+func (c *Client) Analyze(ctx context.Context, input worker.IAInput) (*domain.AnalysisResult, error) {
+	if !c.cfg.Enabled {
+		return nil, nil
+	}
+
+	reqBody := analyzeRequest{
+		ProduccionID:  input.ProduccionID,
+		SceneID:       input.SceneID,
+		MultibandPath: input.MultibandPath,
+		Params:        input.Params,
+	}
+
+	payload, err := json.Marshal(reqBody)
+	if err != nil {
+		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "encoding IA request", Wrapped: err}
+	}
+
+	url := fmt.Sprintf("%s/analyze", c.cfg.ServiceURL)
+	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
+	if err != nil {
+		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "building IA request", Wrapped: err}
+	}
+	req.Header.Set("Content-Type", "application/json")
+
+	resp, err := c.httpClient.Do(req)
+	if err != nil {
+		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "calling IA service", Wrapped: err}
+	}
+	defer resp.Body.Close()
+
+	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
+		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: fmt.Sprintf("IA service returned status %d", resp.StatusCode)}
+	}
+
+	var result domain.AnalysisResult
+	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
+		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "parsing IA response", Wrapped: err}
+	}
+
+	return &result, nil
+}
diff --git a/internal/infrastructure/ia/client_test.go b/internal/infrastructure/ia/client_test.go
new file mode 100644
index 0000000..8466815
--- /dev/null
+++ b/internal/infrastructure/ia/client_test.go
@@ -0,0 +1,113 @@
+package ia
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"net/http"
+	"net/http/httptest"
+	"testing"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/domain"
+	"agro-sentinel-worker/internal/worker"
+)
+
+func TestAnalyze(t *testing.T) {
+	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.Method != "POST" || r.URL.Path != "/analyze" {
+			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
+		}
+		if err := json.NewEncoder(w).Encode(domain.AnalysisResult{
+			EstadoGeneral:  "bueno",
+			PosibleCosecha: false,
+			Confianza:      0.85,
+		}); err != nil {
+			t.Fatalf("encode: %v", err)
+		}
+	}))
+	defer server.Close()
+
+	client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
+	result, err := client.Analyze(context.Background(), worker.IAInput{
+		ProduccionID:  1,
+		SceneID:       "scene-1",
+		MultibandPath: "/tmp/multiband.tif",
+	})
+	if err != nil {
+		t.Fatalf("Analyze failed: %v", err)
+	}
+	if result == nil {
+		t.Fatal("result is nil")
+	}
+	if result.EstadoGeneral != "bueno" {
+		t.Errorf("estado = %q, want bueno", result.EstadoGeneral)
+	}
+}
+
+func TestAnalyzeDisabled(t *testing.T) {
+	client := New(config.IAConfig{Enabled: false})
+	result, err := client.Analyze(context.Background(), worker.IAInput{})
+	if err != nil || result != nil {
+		t.Error("disabled IA should return nil, nil")
+	}
+}
+
+func TestAnalyzePosibleCosecha(t *testing.T) {
+	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if err := json.NewEncoder(w).Encode(domain.AnalysisResult{
+			EstadoGeneral:  "maduro",
+			PosibleCosecha: true,
+			Confianza:      0.92,
+		}); err != nil {
+			t.Fatalf("encode: %v", err)
+		}
+	}))
+	defer server.Close()
+
+	client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
+	result, err := client.Analyze(context.Background(), worker.IAInput{ProduccionID: 1, SceneID: "scene-1"})
+	if err != nil {
+		t.Fatalf("Analyze failed: %v", err)
+	}
+	if !result.PosibleCosecha {
+		t.Error("expected PosibleCosecha = true")
+	}
+}
+
+func TestAnalyzeHTTPError(t *testing.T) {
+	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		w.WriteHeader(http.StatusInternalServerError)
+	}))
+	defer server.Close()
+
+	client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
+	_, err := client.Analyze(context.Background(), worker.IAInput{ProduccionID: 1, SceneID: "scene-1"})
+	if err == nil {
+		t.Fatal("expected error for non-2xx response")
+	}
+
+	var procErr *domain.ProcessingError
+	if !errors.As(err, &procErr) {
+		t.Fatalf("expected *domain.ProcessingError, got %T: %v", err, err)
+	}
+	if procErr.Type != domain.ErrIA {
+		t.Errorf("Type = %q, want %q", procErr.Type, domain.ErrIA)
+	}
+}
+
+func TestAnalyzeConnectionRefused(t *testing.T) {
+	client := New(config.IAConfig{Enabled: true, ServiceURL: "http://127.0.0.1:1", TimeoutSeconds: 1})
+	_, err := client.Analyze(context.Background(), worker.IAInput{ProduccionID: 1, SceneID: "scene-1"})
+	if err == nil {
+		t.Fatal("expected error for connection refused")
+	}
+
+	var procErr *domain.ProcessingError
+	if !errors.As(err, &procErr) {
+		t.Fatalf("expected *domain.ProcessingError, got %T: %v", err, err)
+	}
+	if procErr.Type != domain.ErrIA {
+		t.Errorf("Type = %q, want %q", procErr.Type, domain.ErrIA)
+	}
+}
diff --git a/internal/worker/worker.go b/internal/worker/worker.go
index 832b66f..982282d 100644
--- a/internal/worker/worker.go
+++ b/internal/worker/worker.go
@@ -33,20 +33,21 @@ const maxRetries = 3
 // GDALExecutor is the subset of gdal.Executor's behavior the worker's
 // processing calls depend on. It matches processing.GDALExecutor
 // structurally so any *gdal.Executor (or test double) satisfies both.
 type GDALExecutor interface {
 	Run(ctx context.Context, command string, args []string) (stdout string, stderr string, err error)
 }
 
 // ProductionRepository is the subset of database.ProductionRepo the worker needs.
 type ProductionRepository interface {
 	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
+	SetBloqueado(ctx context.Context, produccionID int64, motivo string) error
 }
 
 // SceneRepository is the subset of database.SceneRepo the worker needs.
 type SceneRepository interface {
 	GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error)
 	UpdateStatus(ctx context.Context, id int64, status domain.JobStatus) error
 	SetError(ctx context.Context, id int64, errType string, errMsg string) error
 	Upsert(ctx context.Context, s *domain.Scene) error
 	GetPreviousValidScene(ctx context.Context, produccionID int64, beforeDate time.Time) (*domain.Scene, error)
 }
@@ -275,20 +276,26 @@ func (w *Worker) process(ctx context.Context, production *domain.Production, sce
 
 		iaAnalysis = w.runIA(ctx, production, scene, multibandPath, params)
 		if iaAnalysis != nil {
 			analisisPath := filepath.Join(jobDir.Output(), "analisis.json")
 			if err := writeJSON(analisisPath, iaAnalysis); err != nil {
 				w.log.Error("failed to write analisis.json", "scene_id", scene.SceneID, "error", err)
 				iaAnalysis = nil
 			} else {
 				outputs = append(outputs, outputFile{fileType: domain.FileAnalisis, path: analisisPath, name: "analisis.json"})
 			}
+
+			if iaAnalysis != nil && iaAnalysis.PosibleCosecha {
+				if err := w.deps.Productions.SetBloqueado(ctx, production.ProduccionID, "posible_cosecha detectada por IA"); err != nil {
+					w.log.Error("failed to block production after posible_cosecha detection", "produccion_id", production.ProduccionID, "scene_id", scene.SceneID, "error", err)
+				}
+			}
 		}
 	} else {
 		naturalPath, err := w.generateNaturalOnly(ctx, multibandPath, jobDir)
 		if err != nil {
 			return err
 		}
 		outputs = append(outputs, outputFile{fileType: domain.FileNatural, path: naturalPath, name: "natural.png"})
 	}
 
 	if err := w.uploadAndRegister(ctx, production, scene, outputs); err != nil {
diff --git a/internal/worker/worker_test.go b/internal/worker/worker_test.go
index 3ddcd0f..5bc5d9b 100644
--- a/internal/worker/worker_test.go
+++ b/internal/worker/worker_test.go
@@ -10,31 +10,45 @@ import (
 	"time"
 
 	"agro-sentinel-worker/internal/config"
 	"agro-sentinel-worker/internal/domain"
 	"agro-sentinel-worker/internal/processing"
 )
 
 // ---- mocks ----
 
 type mockProductionRepo struct {
+	mu sync.Mutex
+
 	production *domain.Production
 	err        error
+
+	bloqueadoCalled bool
+	bloqueadoMotivo string
+	bloqueadoErr    error
 }
 
 func (m *mockProductionRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
 	if m.err != nil {
 		return nil, m.err
 	}
 	return m.production, nil
 }
 
+func (m *mockProductionRepo) SetBloqueado(ctx context.Context, produccionID int64, motivo string) error {
+	m.mu.Lock()
+	defer m.mu.Unlock()
+	m.bloqueadoCalled = true
+	m.bloqueadoMotivo = motivo
+	return m.bloqueadoErr
+}
+
 type mockSceneRepo struct {
 	mu sync.Mutex
 
 	scene *domain.Scene
 
 	prevScene *domain.Scene
 
 	statusUpdates []domain.JobStatus
 	errorType     string
 	errorMessage  string
@@ -516,20 +530,72 @@ func TestProcessScene_IAError_StillCompletes(t *testing.T) {
 		t.Error("expected HasAnalisis = false after IA error")
 	}
 
 	for _, f := range h.fileRepo.created {
 		if f.FileType == domain.FileAnalisis {
 			t.Error("analisis.json should not be registered when IA fails")
 		}
 	}
 }
 
+func TestProcessScene_PosibleCosecha_BlocksProduction(t *testing.T) {
+	h := newHarness(t, cloudCoverBuckets(12))
+	h.ia = &mockIA{result: &domain.AnalysisResult{
+		EstadoGeneral:  "maduro",
+		PosibleCosecha: true,
+		Confianza:      0.9,
+	}}
+	h.deps.IA = h.ia
+	w := New(h.deps)
+
+	if err := w.ProcessScene(context.Background(), 1234, "S2A_test_scene"); err != nil {
+		t.Fatalf("ProcessScene failed: %v", err)
+	}
+
+	if !h.prodRepo.bloqueadoCalled {
+		t.Error("expected SetBloqueado to be called when posible_cosecha is true")
+	}
+	if h.prodRepo.bloqueadoMotivo == "" {
+		t.Error("expected a non-empty motivo for SetBloqueado")
+	}
+
+	final := h.sceneRepo.upserted
+	if final == nil {
+		t.Fatal("expected scene finalized")
+	}
+	if final.Status != domain.StatusCompleted {
+		t.Errorf("status = %v, want COMPLETED", final.Status)
+	}
+	if !final.HasAnalisis {
+		t.Error("expected HasAnalisis = true")
+	}
+}
+
+func TestProcessScene_NoCosecha_DoesNotBlockProduction(t *testing.T) {
+	h := newHarness(t, cloudCoverBuckets(12))
+	h.ia = &mockIA{result: &domain.AnalysisResult{
+		EstadoGeneral:  "bueno",
+		PosibleCosecha: false,
+		Confianza:      0.9,
+	}}
+	h.deps.IA = h.ia
+	w := New(h.deps)
+
+	if err := w.ProcessScene(context.Background(), 1234, "S2A_test_scene"); err != nil {
+		t.Fatalf("ProcessScene failed: %v", err)
+	}
+
+	if h.prodRepo.bloqueadoCalled {
+		t.Error("expected SetBloqueado not to be called when posible_cosecha is false")
+	}
+}
+
 func TestProcessScene_ValidationError_ProductionNotMonitoring(t *testing.T) {
 	h := newHarness(t, cloudCoverBuckets(12))
 	h.prodRepo.production.Monitoring = false
 	w := New(h.deps)
 
 	err := w.ProcessScene(context.Background(), 1234, "S2A_test_scene")
 	if err == nil {
 		t.Fatal("expected validation error")
 	}
 
