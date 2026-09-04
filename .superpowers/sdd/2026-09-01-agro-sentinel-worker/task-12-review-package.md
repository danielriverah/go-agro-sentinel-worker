diff --git a/cmd/api/main.go b/cmd/api/main.go
index 9a1dd0b..b919702 100644
--- a/cmd/api/main.go
+++ b/cmd/api/main.go
@@ -1,35 +1,81 @@
 package main
 
 import (
 	"fmt"
 	"log"
 	"net/http"
 	"os"
 
 	"agro-sentinel-worker/internal/config"
 	apphttp "agro-sentinel-worker/internal/http"
+	"agro-sentinel-worker/internal/infrastructure/aws"
+	"agro-sentinel-worker/internal/infrastructure/database"
 	"agro-sentinel-worker/internal/logger"
+	"agro-sentinel-worker/internal/sync"
 )
 
 func main() {
 	cfgPath := "configs/config.yaml"
 	if p := os.Getenv("CONFIG_PATH"); p != "" {
 		cfgPath = p
 	}
 
 	cfg, err := config.Load(cfgPath)
 	if err != nil {
 		log.Fatalf("loading config: %v", err)
 	}
 
 	l := logger.New(cfg.Logging)
-	router := apphttp.NewRouter(l)
+
+	db, err := database.NewConnection(cfg.MySQL)
+	if err != nil {
+		l.Error("connecting to mysql failed", "error", err)
+		os.Exit(1)
+	}
+	defer db.Close()
+
+	awsCfg, err := aws.NewSession(cfg.AWS)
+	if err != nil {
+		l.Error("creating aws session failed", "error", err)
+		os.Exit(1)
+	}
+
+	prodRepo := database.NewProductionRepo(db)
+	sceneRepo := database.NewSceneRepo(db)
+	fileRepo := database.NewFileRepo(db)
+	s3Client := aws.NewS3Client(awsCfg)
+
+	var syncer apphttp.Syncer
+	dynamoClient := aws.NewDynamoDBClient(awsCfg)
+	polygonRepo := database.NewPolygonRepo(db, sync.CalculateBBoxFromWKT)
+	syncer = sync.New(
+		dynamoClient,
+		prodRepo,
+		sceneRepo,
+		polygonRepo,
+		cfg.Sync,
+		cfg.Sentinel,
+		cfg.DynamoDB.TableProducciones,
+		cfg.DynamoDB.TableEscenas,
+		l,
+	)
+
+	h := &apphttp.Handlers{
+		Productions: prodRepo,
+		Scenes:      sceneRepo,
+		Files:       fileRepo,
+		S3:          s3Client,
+		S3Bucket:    cfg.S3.Bucket,
+		Sync:        syncer,
+	}
+
+	router := apphttp.NewRouter(l, h)
 
 	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
 	l.Info("API server starting", "addr", addr)
 
 	if err := http.ListenAndServe(addr, router); err != nil {
 		l.Error("server failed", "error", err)
 		os.Exit(1)
 	}
 }
diff --git a/internal/http/docs.go b/internal/http/docs.go
new file mode 100644
index 0000000..83e2875
--- /dev/null
+++ b/internal/http/docs.go
@@ -0,0 +1,196 @@
+package http
+
+// openapiSpec is the embedded OpenAPI 3.0 description of the API, served at
+// GET /api/v1/openapi.yaml and rendered by Scalar at GET /docs.
+const openapiSpec = `openapi: 3.0.3
+info:
+  title: Agro Sentinel Worker API
+  description: REST API for querying producciones, escenas and their processed files.
+  version: "1.0.0"
+servers:
+  - url: /api/v1
+paths:
+  /producciones:
+    get:
+      summary: List active producciones
+      operationId: listProducciones
+      tags: [producciones]
+      responses:
+        "200":
+          description: List of active producciones
+          content:
+            application/json:
+              schema:
+                type: object
+                properties:
+                  data:
+                    type: array
+                    items:
+                      $ref: "#/components/schemas/Production"
+  /producciones/{id}:
+    get:
+      summary: Get a produccion with its escenas
+      operationId: getProduccion
+      tags: [producciones]
+      parameters:
+        - $ref: "#/components/parameters/ProduccionID"
+      responses:
+        "200":
+          description: Produccion detail
+          content:
+            application/json:
+              schema:
+                type: object
+                properties:
+                  data:
+                    $ref: "#/components/schemas/ProductionDetail"
+        "404":
+          $ref: "#/components/responses/NotFound"
+  /producciones/{id}/desbloquear:
+    post:
+      summary: Unblock a produccion
+      operationId: desbloquearProduccion
+      tags: [producciones]
+      parameters:
+        - $ref: "#/components/parameters/ProduccionID"
+      requestBody:
+        required: false
+        content:
+          application/json:
+            schema:
+              type: object
+              properties:
+                usuario:
+                  type: string
+      responses:
+        "200":
+          description: Updated produccion
+          content:
+            application/json:
+              schema:
+                type: object
+                properties:
+                  data:
+                    $ref: "#/components/schemas/Production"
+        "404":
+          $ref: "#/components/responses/NotFound"
+  /escenas/{id}/archivos/{tipo}:
+    get:
+      summary: Get a presigned URL for a scene file
+      operationId: getEscenaArchivo
+      tags: [escenas]
+      parameters:
+        - name: id
+          in: path
+          required: true
+          schema:
+            type: integer
+            format: int64
+        - name: tipo
+          in: path
+          required: true
+          schema:
+            type: string
+          description: File type, e.g. ndvi, natural, multiband, params, analisis
+      responses:
+        "200":
+          description: Presigned URL
+          content:
+            application/json:
+              schema:
+                type: object
+                properties:
+                  data:
+                    $ref: "#/components/schemas/ArchivoResponse"
+        "404":
+          $ref: "#/components/responses/NotFound"
+  /sync/trigger:
+    post:
+      summary: Trigger a sync cycle
+      operationId: triggerSync
+      tags: [sync]
+      responses:
+        "202":
+          description: Sync cycle triggered
+          content:
+            application/json:
+              schema:
+                type: object
+                properties:
+                  data:
+                    type: object
+                    properties:
+                      status:
+                        type: string
+                        example: triggered
+components:
+  parameters:
+    ProduccionID:
+      name: id
+      in: path
+      required: true
+      schema:
+        type: integer
+        format: int64
+  responses:
+    NotFound:
+      description: Resource not found
+      content:
+        application/json:
+          schema:
+            type: object
+            properties:
+              error:
+                type: string
+  schemas:
+    Production:
+      type: object
+      properties:
+        ID: { type: integer, format: int64 }
+        ProduccionID: { type: integer, format: int64 }
+        Cultivo: { type: string }
+        Ciclo: { type: string }
+        Monitoring: { type: boolean }
+        Bloqueado: { type: boolean }
+        TotalEscenas: { type: integer }
+        TotalEscenasValidas: { type: integer }
+    ProductionDetail:
+      allOf:
+        - $ref: "#/components/schemas/Production"
+        - type: object
+          properties:
+            escenas:
+              type: array
+              items:
+                $ref: "#/components/schemas/Scene"
+    Scene:
+      type: object
+      properties:
+        ID: { type: integer, format: int64 }
+        ProduccionID: { type: integer, format: int64 }
+        SceneID: { type: string }
+        SceneDate: { type: string, format: date-time }
+        Status: { type: string }
+    ArchivoResponse:
+      type: object
+      properties:
+        file_name: { type: string }
+        file_type: { type: string }
+        url: { type: string }
+        expires_in_seconds: { type: integer }
+`
+
+// scalarHTML renders the Scalar API reference UI, pointed at the embedded
+// OpenAPI spec served from GET /api/v1/openapi.yaml.
+const scalarHTML = `<!doctype html>
+<html>
+  <head>
+    <title>Agro Sentinel Worker API</title>
+    <meta charset="utf-8" />
+    <meta name="viewport" content="width=device-width, initial-scale=1" />
+  </head>
+  <body>
+    <script id="api-reference" data-url="/api/v1/openapi.yaml"></script>
+    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
+  </body>
+</html>`
diff --git a/internal/http/handlers.go b/internal/http/handlers.go
index 38c9c86..250ab8c 100644
--- a/internal/http/handlers.go
+++ b/internal/http/handlers.go
@@ -1,11 +1,221 @@
 package http
 
 import (
+	"context"
 	"encoding/json"
 	"net/http"
+	"strconv"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
 )
 
+// PresignExpiry is how long presigned S3 URLs served by the API remain valid.
+const PresignExpiry = 15 * time.Minute
+
+// ProductionRepository is the subset of database.ProductionRepo the API needs.
+type ProductionRepository interface {
+	ListActive(ctx context.Context) ([]*domain.Production, error)
+	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
+	Desbloquear(ctx context.Context, produccionID int64, usuario string) error
+}
+
+// SceneRepository is the subset of database.SceneRepo the API needs.
+type SceneRepository interface {
+	ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.Scene, error)
+	GetByID(ctx context.Context, id int64) (*domain.Scene, error)
+}
+
+// FileRepository is the subset of database.FileRepo the API needs.
+type FileRepository interface {
+	ListByEscena(ctx context.Context, escenaID int64) ([]*domain.SceneFile, error)
+	GetByType(ctx context.Context, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error)
+}
+
+// Presigner is the subset of aws.S3Client the API needs.
+type Presigner interface {
+	PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
+}
+
+// Syncer triggers a sync cycle. Implemented by *sync.Service.
+type Syncer interface {
+	RunOnce(ctx context.Context) error
+}
+
+// HealthHandler reports basic liveness.
 func HealthHandler(w http.ResponseWriter, r *http.Request) {
 	w.Header().Set("Content-Type", "application/json")
 	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
 }
+
+// Handlers implements the API endpoints. It holds the dependencies needed to
+// serve them; each is defined as a narrow local interface so tests can mock
+// them independently.
+type Handlers struct {
+	Productions ProductionRepository
+	Scenes      SceneRepository
+	Files       FileRepository
+	S3          Presigner
+	S3Bucket    string
+	Sync        Syncer
+}
+
+// desbloquearRequest is the optional body for POST .../desbloquear.
+type desbloquearRequest struct {
+	Usuario string `json:"usuario"`
+}
+
+// ListProducciones handles GET /api/v1/producciones.
+func (h *Handlers) ListProducciones(w http.ResponseWriter, r *http.Request) {
+	productions, err := h.Productions.ListActive(r.Context())
+	if err != nil {
+		Error(w, http.StatusInternalServerError, "listing producciones: "+err.Error())
+		return
+	}
+
+	JSON(w, http.StatusOK, productions)
+}
+
+// GetProduccion handles GET /api/v1/producciones/{id}.
+func (h *Handlers) GetProduccion(w http.ResponseWriter, r *http.Request) {
+	id, ok := parseInt64Param(w, r, "id")
+	if !ok {
+		return
+	}
+
+	production, err := h.Productions.GetByProduccionID(r.Context(), id)
+	if err != nil {
+		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
+		return
+	}
+	if production == nil {
+		Error(w, http.StatusNotFound, "produccion not found")
+		return
+	}
+
+	scenes, err := h.Scenes.ListByProduccion(r.Context(), id)
+	if err != nil {
+		Error(w, http.StatusInternalServerError, "listing escenas: "+err.Error())
+		return
+	}
+
+	JSON(w, http.StatusOK, produccionDetail{Production: production, Escenas: scenes})
+}
+
+type produccionDetail struct {
+	*domain.Production
+	Escenas []*domain.Scene `json:"escenas"`
+}
+
+// DesbloquearProduccion handles POST /api/v1/producciones/{id}/desbloquear.
+func (h *Handlers) DesbloquearProduccion(w http.ResponseWriter, r *http.Request) {
+	id, ok := parseInt64Param(w, r, "id")
+	if !ok {
+		return
+	}
+
+	var req desbloquearRequest
+	if r.Body != nil {
+		// Body is optional; ignore decode errors for an empty body.
+		_ = json.NewDecoder(r.Body).Decode(&req)
+	}
+
+	if err := h.Productions.Desbloquear(r.Context(), id, req.Usuario); err != nil {
+		Error(w, http.StatusInternalServerError, "desbloqueando produccion: "+err.Error())
+		return
+	}
+
+	production, err := h.Productions.GetByProduccionID(r.Context(), id)
+	if err != nil {
+		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
+		return
+	}
+	if production == nil {
+		Error(w, http.StatusNotFound, "produccion not found")
+		return
+	}
+
+	JSON(w, http.StatusOK, production)
+}
+
+// GetEscenaArchivo handles GET /api/v1/escenas/{id}/archivos/{tipo}.
+func (h *Handlers) GetEscenaArchivo(w http.ResponseWriter, r *http.Request) {
+	id, ok := parseInt64Param(w, r, "id")
+	if !ok {
+		return
+	}
+
+	tipo := r.PathValue("tipo")
+	if tipo == "" {
+		Error(w, http.StatusBadRequest, "tipo is required")
+		return
+	}
+
+	file, err := h.Files.GetByType(r.Context(), id, domain.FileType(tipo))
+	if err != nil {
+		Error(w, http.StatusInternalServerError, "getting archivo: "+err.Error())
+		return
+	}
+	if file == nil {
+		Error(w, http.StatusNotFound, "archivo not found")
+		return
+	}
+
+	bucket := file.S3Bucket
+	if bucket == "" {
+		bucket = h.S3Bucket
+	}
+
+	url, err := h.S3.PresignGetObject(r.Context(), bucket, file.S3Key, PresignExpiry)
+	if err != nil {
+		Error(w, http.StatusInternalServerError, "presigning url: "+err.Error())
+		return
+	}
+
+	JSON(w, http.StatusOK, archivoResponse{
+		FileName:  file.FileName,
+		FileType:  string(file.FileType),
+		URL:       url,
+		ExpiresIn: int(PresignExpiry.Seconds()),
+	})
+}
+
+type archivoResponse struct {
+	FileName  string `json:"file_name"`
+	FileType  string `json:"file_type"`
+	URL       string `json:"url"`
+	ExpiresIn int    `json:"expires_in_seconds"`
+}
+
+// TriggerSync handles POST /api/v1/sync/trigger. It runs the sync cycle in
+// the background and immediately returns 202 Accepted.
+func (h *Handlers) TriggerSync(w http.ResponseWriter, r *http.Request) {
+	if h.Sync == nil {
+		Error(w, http.StatusServiceUnavailable, "sync service not configured")
+		return
+	}
+
+	go func() {
+		_ = h.Sync.RunOnce(context.Background())
+	}()
+
+	JSON(w, http.StatusAccepted, map[string]string{"status": "triggered"})
+}
+
+// parseInt64Param parses the named path value as an int64, writing a 400
+// error response and returning ok=false if it is missing or invalid.
+func parseInt64Param(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
+	raw := r.PathValue(name)
+	if raw == "" {
+		Error(w, http.StatusBadRequest, name+" is required")
+		return 0, false
+	}
+
+	v, err := strconv.ParseInt(raw, 10, 64)
+	if err != nil {
+		Error(w, http.StatusBadRequest, "invalid "+name)
+		return 0, false
+	}
+
+	return v, true
+}
diff --git a/internal/http/handlers_test.go b/internal/http/handlers_test.go
index b0be8e0..e684305 100644
--- a/internal/http/handlers_test.go
+++ b/internal/http/handlers_test.go
@@ -1,17 +1,21 @@
 package http
 
 import (
+	"context"
 	"encoding/json"
 	"net/http"
 	"net/http/httptest"
 	"testing"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
 )
 
 func TestHealthHandler(t *testing.T) {
 	req := httptest.NewRequest("GET", "/health", nil)
 	w := httptest.NewRecorder()
 
 	HealthHandler(w, req)
 
 	resp := w.Result()
 	if resp.StatusCode != http.StatusOK {
@@ -20,10 +24,289 @@ func TestHealthHandler(t *testing.T) {
 
 	var body map[string]string
 	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
 		t.Fatalf("decode response: %v", err)
 	}
 
 	if body["status"] != "ok" {
 		t.Errorf("status = %q, want ok", body["status"])
 	}
 }
+
+// --- mocks ---
+
+type mockProductionRepo struct {
+	listActive     []*domain.Production
+	listActiveErr  error
+	byID           map[int64]*domain.Production
+	getErr         error
+	desbloquearErr error
+	desbloqueado   bool
+	desbloqUsuario string
+}
+
+func (m *mockProductionRepo) ListActive(ctx context.Context) ([]*domain.Production, error) {
+	return m.listActive, m.listActiveErr
+}
+
+func (m *mockProductionRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
+	if m.getErr != nil {
+		return nil, m.getErr
+	}
+	return m.byID[produccionID], nil
+}
+
+func (m *mockProductionRepo) Desbloquear(ctx context.Context, produccionID int64, usuario string) error {
+	if m.desbloquearErr != nil {
+		return m.desbloquearErr
+	}
+	m.desbloqueado = true
+	m.desbloqUsuario = usuario
+	return nil
+}
+
+type mockSceneRepo struct {
+	byProduccion map[int64][]*domain.Scene
+	err          error
+}
+
+func (m *mockSceneRepo) ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.Scene, error) {
+	if m.err != nil {
+		return nil, m.err
+	}
+	return m.byProduccion[produccionID], nil
+}
+
+func (m *mockSceneRepo) GetByID(ctx context.Context, id int64) (*domain.Scene, error) {
+	return nil, nil
+}
+
+type mockFileRepo struct {
+	byType map[int64]*domain.SceneFile
+	err    error
+}
+
+func (m *mockFileRepo) ListByEscena(ctx context.Context, escenaID int64) ([]*domain.SceneFile, error) {
+	return nil, nil
+}
+
+func (m *mockFileRepo) GetByType(ctx context.Context, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error) {
+	if m.err != nil {
+		return nil, m.err
+	}
+	return m.byType[escenaID], nil
+}
+
+type mockPresigner struct {
+	url string
+	err error
+}
+
+func (m *mockPresigner) PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
+	if m.err != nil {
+		return "", m.err
+	}
+	return m.url, nil
+}
+
+type mockSyncer struct {
+	called chan struct{}
+	err    error
+}
+
+func (m *mockSyncer) RunOnce(ctx context.Context) error {
+	if m.called != nil {
+		close(m.called)
+	}
+	return m.err
+}
+
+// --- tests ---
+
+func TestListProducciones(t *testing.T) {
+	prodRepo := &mockProductionRepo{
+		listActive: []*domain.Production{
+			{ProduccionID: 1, Cultivo: "soja"},
+			{ProduccionID: 2, Cultivo: "maiz"},
+		},
+	}
+	h := &Handlers{Productions: prodRepo}
+
+	req := httptest.NewRequest("GET", "/api/v1/producciones", nil)
+	w := httptest.NewRecorder()
+
+	h.ListProducciones(w, req)
+
+	resp := w.Result()
+	if resp.StatusCode != http.StatusOK {
+		t.Fatalf("status = %d, want 200", resp.StatusCode)
+	}
+
+	var body struct {
+		Data []domain.Production `json:"data"`
+	}
+	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
+		t.Fatalf("decode: %v", err)
+	}
+	if len(body.Data) != 2 {
+		t.Fatalf("got %d producciones, want 2", len(body.Data))
+	}
+}
+
+func TestGetProduccion(t *testing.T) {
+	prodRepo := &mockProductionRepo{
+		byID: map[int64]*domain.Production{
+			42: {ProduccionID: 42, Cultivo: "soja"},
+		},
+	}
+	sceneRepo := &mockSceneRepo{
+		byProduccion: map[int64][]*domain.Scene{
+			42: {{ID: 1, ProduccionID: 42, SceneID: "s1"}},
+		},
+	}
+	h := &Handlers{Productions: prodRepo, Scenes: sceneRepo}
+
+	req := httptest.NewRequest("GET", "/api/v1/producciones/42", nil)
+	req.SetPathValue("id", "42")
+	w := httptest.NewRecorder()
+
+	h.GetProduccion(w, req)
+
+	resp := w.Result()
+	if resp.StatusCode != http.StatusOK {
+		t.Fatalf("status = %d, want 200", resp.StatusCode)
+	}
+
+	var body struct {
+		Data struct {
+			ProduccionID int64          `json:"ProduccionID"`
+			Escenas      []domain.Scene `json:"escenas"`
+		} `json:"data"`
+	}
+	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
+		t.Fatalf("decode: %v", err)
+	}
+	if body.Data.ProduccionID != 42 {
+		t.Errorf("produccion_id = %d, want 42", body.Data.ProduccionID)
+	}
+	if len(body.Data.Escenas) != 1 {
+		t.Fatalf("got %d escenas, want 1", len(body.Data.Escenas))
+	}
+}
+
+func TestGetProduccionNotFound(t *testing.T) {
+	h := &Handlers{Productions: &mockProductionRepo{byID: map[int64]*domain.Production{}}}
+
+	req := httptest.NewRequest("GET", "/api/v1/producciones/99", nil)
+	req.SetPathValue("id", "99")
+	w := httptest.NewRecorder()
+
+	h.GetProduccion(w, req)
+
+	if w.Result().StatusCode != http.StatusNotFound {
+		t.Errorf("status = %d, want 404", w.Result().StatusCode)
+	}
+}
+
+func TestDesbloquearProduccion(t *testing.T) {
+	prodRepo := &mockProductionRepo{
+		byID: map[int64]*domain.Production{
+			7: {ProduccionID: 7, Bloqueado: false},
+		},
+	}
+	h := &Handlers{Productions: prodRepo}
+
+	req := httptest.NewRequest("POST", "/api/v1/producciones/7/desbloquear", nil)
+	req.SetPathValue("id", "7")
+	w := httptest.NewRecorder()
+
+	h.DesbloquearProduccion(w, req)
+
+	resp := w.Result()
+	if resp.StatusCode != http.StatusOK {
+		t.Fatalf("status = %d, want 200", resp.StatusCode)
+	}
+	if !prodRepo.desbloqueado {
+		t.Error("expected Desbloquear to be called")
+	}
+}
+
+func TestGetEscenaArchivo(t *testing.T) {
+	fileRepo := &mockFileRepo{
+		byType: map[int64]*domain.SceneFile{
+			5: {ID: 1, EscenaID: 5, FileType: domain.FileNDVI, FileName: "ndvi.tif", S3Key: "key", S3Bucket: "bucket"},
+		},
+	}
+	presigner := &mockPresigner{url: "https://example.com/presigned"}
+	h := &Handlers{Files: fileRepo, S3: presigner, S3Bucket: "default-bucket"}
+
+	req := httptest.NewRequest("GET", "/api/v1/escenas/5/archivos/ndvi", nil)
+	req.SetPathValue("id", "5")
+	req.SetPathValue("tipo", "ndvi")
+	w := httptest.NewRecorder()
+
+	h.GetEscenaArchivo(w, req)
+
+	resp := w.Result()
+	if resp.StatusCode != http.StatusOK {
+		t.Fatalf("status = %d, want 200", resp.StatusCode)
+	}
+
+	var body struct {
+		Data archivoResponse `json:"data"`
+	}
+	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
+		t.Fatalf("decode: %v", err)
+	}
+	if body.Data.URL != "https://example.com/presigned" {
+		t.Errorf("url = %q, want presigned url", body.Data.URL)
+	}
+}
+
+func TestGetEscenaArchivoNotFound(t *testing.T) {
+	h := &Handlers{Files: &mockFileRepo{byType: map[int64]*domain.SceneFile{}}}
+
+	req := httptest.NewRequest("GET", "/api/v1/escenas/5/archivos/ndvi", nil)
+	req.SetPathValue("id", "5")
+	req.SetPathValue("tipo", "ndvi")
+	w := httptest.NewRecorder()
+
+	h.GetEscenaArchivo(w, req)
+
+	if w.Result().StatusCode != http.StatusNotFound {
+		t.Errorf("status = %d, want 404", w.Result().StatusCode)
+	}
+}
+
+func TestTriggerSync(t *testing.T) {
+	called := make(chan struct{})
+	h := &Handlers{Sync: &mockSyncer{called: called}}
+
+	req := httptest.NewRequest("POST", "/api/v1/sync/trigger", nil)
+	w := httptest.NewRecorder()
+
+	h.TriggerSync(w, req)
+
+	resp := w.Result()
+	if resp.StatusCode != http.StatusAccepted {
+		t.Fatalf("status = %d, want 202", resp.StatusCode)
+	}
+
+	select {
+	case <-called:
+	case <-time.After(time.Second):
+		t.Error("expected RunOnce to be called")
+	}
+}
+
+func TestTriggerSyncNotConfigured(t *testing.T) {
+	h := &Handlers{}
+
+	req := httptest.NewRequest("POST", "/api/v1/sync/trigger", nil)
+	w := httptest.NewRecorder()
+
+	h.TriggerSync(w, req)
+
+	if w.Result().StatusCode != http.StatusServiceUnavailable {
+		t.Errorf("status = %d, want 503", w.Result().StatusCode)
+	}
+}
diff --git a/internal/http/middleware.go b/internal/http/middleware.go
new file mode 100644
index 0000000..8c77ade
--- /dev/null
+++ b/internal/http/middleware.go
@@ -0,0 +1,36 @@
+package http
+
+import (
+	"log/slog"
+	"net/http"
+	"time"
+)
+
+// statusRecorder captures the status code written by downstream handlers.
+type statusRecorder struct {
+	http.ResponseWriter
+	status int
+}
+
+func (r *statusRecorder) WriteHeader(status int) {
+	r.status = status
+	r.ResponseWriter.WriteHeader(status)
+}
+
+// LoggingMiddleware logs each request's method, path, status code and
+// duration using the given logger.
+func LoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
+	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		start := time.Now()
+		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
+
+		next.ServeHTTP(rec, r)
+
+		logger.Info("request",
+			"method", r.Method,
+			"path", r.URL.Path,
+			"status", rec.status,
+			"duration_ms", time.Since(start).Milliseconds(),
+		)
+	})
+}
diff --git a/internal/http/responses.go b/internal/http/responses.go
new file mode 100644
index 0000000..ce7b145
--- /dev/null
+++ b/internal/http/responses.go
@@ -0,0 +1,26 @@
+package http
+
+import (
+	"encoding/json"
+	"net/http"
+)
+
+// envelope is the consistent JSON response shape used by all endpoints.
+type envelope struct {
+	Data  any    `json:"data,omitempty"`
+	Error string `json:"error,omitempty"`
+}
+
+// JSON writes data wrapped in {"data": ...} with the given status code.
+func JSON(w http.ResponseWriter, status int, data any) {
+	w.Header().Set("Content-Type", "application/json")
+	w.WriteHeader(status)
+	_ = json.NewEncoder(w).Encode(envelope{Data: data})
+}
+
+// Error writes {"error": message} with the given status code.
+func Error(w http.ResponseWriter, status int, message string) {
+	w.Header().Set("Content-Type", "application/json")
+	w.WriteHeader(status)
+	_ = json.NewEncoder(w).Encode(envelope{Error: message})
+}
diff --git a/internal/http/router.go b/internal/http/router.go
index b353f54..e916161 100644
--- a/internal/http/router.go
+++ b/internal/http/router.go
@@ -1,12 +1,31 @@
 package http
 
 import (
 	"log/slog"
 	"net/http"
 )
 
-func NewRouter(logger *slog.Logger) *http.ServeMux {
+// NewRouter builds the API's http.ServeMux, wiring health, docs and all
+// versioned API routes, wrapped in the logging middleware.
+func NewRouter(logger *slog.Logger, h *Handlers) http.Handler {
 	mux := http.NewServeMux()
+
 	mux.HandleFunc("GET /health", HealthHandler)
-	return mux
+
+	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
+		w.Header().Set("Content-Type", "text/html")
+		_, _ = w.Write([]byte(scalarHTML))
+	})
+	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
+		w.Header().Set("Content-Type", "application/yaml")
+		_, _ = w.Write([]byte(openapiSpec))
+	})
+
+	mux.HandleFunc("GET /api/v1/producciones", h.ListProducciones)
+	mux.HandleFunc("GET /api/v1/producciones/{id}", h.GetProduccion)
+	mux.HandleFunc("POST /api/v1/producciones/{id}/desbloquear", h.DesbloquearProduccion)
+	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos/{tipo}", h.GetEscenaArchivo)
+	mux.HandleFunc("POST /api/v1/sync/trigger", h.TriggerSync)
+
+	return LoggingMiddleware(logger, mux)
 }
