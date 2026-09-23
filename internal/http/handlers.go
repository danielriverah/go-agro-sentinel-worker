package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"agro-sentinel-worker/internal/auth"
	"agro-sentinel-worker/internal/daemon"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/ia"
)

// PresignExpiry is how long presigned S3 URLs served by the API remain valid.
const PresignExpiry = 15 * time.Minute

// ProductionRepository is the subset of database.ProductionRepo the API needs.
type ProductionRepository interface {
	ListActive(ctx context.Context) ([]*domain.Production, error)
	GetByID(ctx context.Context, id uint) (*domain.Production, error)
	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
	GetByMonitoringID(ctx context.Context, monitoringID uint) (*domain.Production, error)
	// Bloquear y desbloquear se hacen a través de posible_cosecha: un trigger
	// de la tabla deriva bloqueado de ese campo.
	UpdatePosibleCosecha(ctx context.Context, produccionID int64, posible bool) error
	UpdateIAuto(ctx context.Context, produccionID int64, iaAuto bool) error
	GetStatsForActive(ctx context.Context) (map[uint]*domain.ProductionStats, error)
	UpdatePolygon(ctx context.Context, monitoringID uint, poligono, pbox []byte) error
	// GetCentroCostoByMonitoringID resuelve un s3_monitoring_produccion_id a su
	// centro_costo_id. Lo usa auth.RequirePermission para comprobar permisos
	// per-rancho sin que el middleware tenga que conocer el repo completo.
	GetCentroCostoByMonitoringID(ctx context.Context, monitoringID uint) (*int64, error)
}

// SceneRepository is the subset of database.SceneRepo the API needs.
type SceneRepository interface {
	ListByMonitoringProduccion(ctx context.Context, monitoringProduccionID uint) ([]*domain.Scene, error)
	GetByID(ctx context.Context, id uint64) (*domain.Scene, error)
}

// FileRepository is the subset of database.FileRepo the API needs.
type FileRepository interface {
	ListByEscena(ctx context.Context, escenaID uint64) ([]*domain.SceneFile, error)
	GetByTipo(ctx context.Context, escenaID uint64, tipo string) (*domain.SceneFile, error)
}

// Presigner is the subset of aws.S3Client the API needs.
type Presigner interface {
	PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// S3Getter streams S3 objects directly. Implemented by *aws.S3Client.
type S3Getter interface {
	// GetObjectStream opens bucket/key for streaming. Returns nil body when the object does not exist.
	GetObjectStream(ctx context.Context, bucket, key string) (body io.ReadCloser, contentType string, contentLength int64, err error)
}

// Syncer triggers a sync cycle and exposes schedule status. Implemented by *sync.Service.
type Syncer interface {
	RunOnce(ctx context.Context) error
	// Status returns last/next run times and schedule info as a JSON-serializable value.
	Status() any
}

// IAResultRepository fetches persisted IA results.
type IAResultRepository interface {
	GetByEscenaID(ctx context.Context, escenaID uint64) (*domain.IAResultSummary, error)
}

// IATriggerer runs the IA analysis pipeline for a scene.
// Implemented by *ia.Analyzer.
type IATriggerer interface {
	Analyze(ctx context.Context, escenaID uint64, s3Key string, produccionID int64) (*domain.IAResultSummary, error)
	IsRunning(escenaID uint64) bool
	DryRun(ctx context.Context, s3Key string) (*ia.DryRunResult, error)
}

// DBPinger checks database connectivity. Implemented by *sql.DB.
type DBPinger interface {
	PingContext(ctx context.Context) error
}

// S3Checker checks S3 bucket accessibility. Implemented by *aws.S3Client.
type S3Checker interface {
	HeadBucket(ctx context.Context, bucket string) error
}

// DynamoDBChecker checks DynamoDB table accessibility. Implemented by *aws.DynamoDBClient.
type DynamoDBChecker interface {
	DescribeTable(ctx context.Context, tableName string) error
}

// PermissionChecker provides permission evaluation for handlers.
// Nil or unavailable = permissive mode (no filtering).
type PermissionChecker interface {
	Disponible() bool
	CargarPermisos(ctx context.Context, usuarioID int64) (*auth.PermisosUsuario, error)
	ObtenerPermisosCacheados(usuarioID int64) *auth.PermisosUsuario
	InvalidarCache(usuarioID int64)
}

// GDALExecutor runs GDAL commands. Implemented by *GDALCommand.
type GDALExecutor interface {
	Run(ctx context.Context) (string, error)
}

// GDALCommand wraps exec.CommandContext to run GDAL version check.
type GDALCommand struct {
	// NewCmd is a function that creates an exec.Cmd. Can be overridden in tests.
	NewCmd func(ctx context.Context, name string, args ...string) interface {
		Output() ([]byte, error)
	}
}

// NewGDALCommand creates a GDAL executor that runs `gdalinfo --version`.
func NewGDALCommand() *GDALCommand {
	return &GDALCommand{
		NewCmd: func(ctx context.Context, name string, args ...string) interface {
			Output() ([]byte, error)
		} {
			return exec.CommandContext(ctx, name, args...)
		},
	}
}

// Run executes `gdalinfo --version` and returns the version output.
func (g *GDALCommand) Run(ctx context.Context) (string, error) {
	cmd := g.NewCmd(ctx, "gdalinfo", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running gdalinfo: %w", err)
	}
	return string(output), nil
}

// HealthHandler reports basic liveness.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Handlers implements the API endpoints. It holds the dependencies needed to
// serve them; each is defined as a narrow local interface so tests can mock
// them independently.
type Handlers struct {
	Productions ProductionRepository
	Scenes      SceneRepository
	Timeline    TimelineRepository
	Fases       FaseRepository
	Alertas     AlertaRepository
	Files       FileRepository
	IAResults   IAResultRepository
	IA          IATriggerer // nil = IA not configured
	S3          Presigner
	S3Stream    S3Getter // streams objects directly (proxy endpoint)
	S3Bucket    string
	S3Prefix    string // used to build ia_req.json S3 keys
	Sync        Syncer
	// Borrado de monitoreo: toca los tres sistemas, por eso viaja aparte.
	S3Delete                S3PrefixDeleter
	DynamoDelete            DynamoDeleter
	Monitoreo               MonitoreoDeleter
	DynamoTablaProducciones string
	DynamoTablaEscenas      string
	Log                  *slog.Logger
	DB                   DBPinger
	S3Health             S3Checker
	DynamoDB             DynamoDBChecker
	GDAL                 GDALExecutor
	// Permisos habilita el filtrado por rancho en endpoints de listado. nil o
	// Disponible()==false = modo permisivo (sin filtrar), para no romper
	// despliegues donde las tablas de permisos aún no existen.
	Permisos PermissionChecker
	// PermisosRepo respalda los endpoints de administración de roles/permisos
	// (internal/http/handlers_permisos.go). Es el mismo *database.PermissionRepo
	// que Permisos, expuesto con una interfaz más amplia para el CRUD.
	PermisosRepo PermissionRepository
}

// desbloquearRequest is the optional body for POST .../desbloquear.
type desbloquearRequest struct {
	Usuario string `json:"usuario"`
}

// ListProducciones handles GET /api/v1/producciones.
// produccionSummary embeds a Production with its scene-level stats.
type produccionSummary struct {
	*domain.Production
	Stats *domain.ProductionStats `json:"stats,omitempty"`
}

func (h *Handlers) ListProducciones(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	productions, err := h.Productions.ListActive(ctx)
	if err != nil {
		Error(w, http.StatusInternalServerError, "listing producciones: "+err.Error())
		return
	}

	if h.Permisos != nil && h.Permisos.Disponible() {
		productions, err = h.filterProduccionesPorPermiso(ctx, productions)
		if err != nil {
			Error(w, http.StatusInternalServerError, "cargando permisos: "+err.Error())
			return
		}
	}

	// Fetch scene stats in a single query — non-fatal if it fails.
	stats, _ := h.Productions.GetStatsForActive(ctx)

	summaries := make([]produccionSummary, len(productions))
	for i, p := range productions {
		summaries[i] = produccionSummary{Production: p, Stats: stats[p.ID]}
	}

	JSON(w, http.StatusOK, summaries)
}

// filterProduccionesPorPermiso restringe la lista a las producciones cuyo
// centro_costo_id está entre los ranchos donde el usuario autenticado tiene
// el permiso "producciones.ver". Sin claims en el contexto (no debería pasar
// detrás del middleware de auth) devuelve la lista sin filtrar. nil de
// RanchosConPermiso significa "todos los ranchos" — no se filtra.
func (h *Handlers) filterProduccionesPorPermiso(ctx context.Context, productions []*domain.Production) ([]*domain.Production, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return productions, nil
	}

	perms := h.Permisos.ObtenerPermisosCacheados(claims.UserID)
	if perms == nil {
		var err error
		perms, err = h.Permisos.CargarPermisos(ctx, claims.UserID)
		if err != nil {
			return nil, err
		}
	}

	ranchos := perms.RanchosConPermiso("producciones.ver")
	if ranchos == nil {
		// Todos los ranchos — no filtrar.
		return productions, nil
	}

	allowed := make(map[int64]bool, len(ranchos))
	for _, id := range ranchos {
		allowed[id] = true
	}

	filtered := make([]*domain.Production, 0, len(productions))
	for _, p := range productions {
		if allowed[p.CentroCostoID] {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// GetProduccion handles GET /api/v1/producciones/{id}.
// {id} is the s3_monitoring_produccion_id (PK), which is the ID the frontend receives
// from ListProducciones and uses to navigate to a specific production.
func (h *Handlers) GetProduccion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUintParam(w, r, "id")
	if !ok {
		return
	}

	production, err := h.Productions.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
		return
	}
	if production == nil {
		Error(w, http.StatusNotFound, "produccion not found")
		return
	}

	scenes, err := h.Scenes.ListByMonitoringProduccion(r.Context(), production.ID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "listing escenas: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, produccionDetail{Production: production, Escenas: scenes})
}

type produccionDetail struct {
	*domain.Production
	Escenas []*domain.Scene `json:"escenas"`
}

// DesbloquearProduccion handles POST /api/v1/producciones/{id}/desbloquear.
// {id} is the s3_monitoring_produccion_id (PK).
//
// Escribe posible_cosecha = 0, no bloqueado. Un trigger de la tabla deriva
// bloqueado de posible_cosecha cuando éste cambia, así que tocar bloqueado
// directamente dejaría ambos campos desincronizados.
func (h *Handlers) DesbloquearProduccion(w http.ResponseWriter, r *http.Request) {
	h.cambiarPosibleCosecha(w, r, false)
}

// BloquearProduccion handles POST /api/v1/producciones/{id}/bloquear.
//
// Marca posible_cosecha = 1; el trigger la bloquea. Es la vía manual para
// señalar que un lote está listo para cosecha.
func (h *Handlers) BloquearProduccion(w http.ResponseWriter, r *http.Request) {
	h.cambiarPosibleCosecha(w, r, true)
}

func (h *Handlers) cambiarPosibleCosecha(w http.ResponseWriter, r *http.Request, posible bool) {
	production, ok := h.produccionDesdeRuta(w, r)
	if !ok {
		return
	}

	if err := h.Productions.UpdatePosibleCosecha(r.Context(), production.ProduccionID, posible); err != nil {
		Error(w, http.StatusInternalServerError, "actualizando posible_cosecha: "+err.Error())
		return
	}

	// El trigger mantiene bloqueado == posible_cosecha; se refleja aquí para
	// que la respuesta coincida con lo que quedó en la base.
	production.PosibleCosecha = posible
	production.Bloqueado = posible
	JSON(w, http.StatusOK, production)
}

// PatchIAAuto handles PATCH /api/v1/producciones/{id}/ia-auto.
// Idempotent: if the production already has the requested ia_auto value the
// handler returns 200 without touching the database.
func (h *Handlers) PatchIAAuto(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUintParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		IAAuto bool `json:"ia_auto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		Error(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	production, err := h.Productions.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
		return
	}
	if production == nil {
		Error(w, http.StatusNotFound, "produccion not found")
		return
	}

	// Idempotency: skip the DB write if value is already as requested.
	if production.IAuto != body.IAAuto {
		if err := h.Productions.UpdateIAuto(r.Context(), production.ProduccionID, body.IAAuto); err != nil {
			Error(w, http.StatusInternalServerError, "updating ia_auto: "+err.Error())
			return
		}
		production.IAuto = body.IAAuto
	}

	JSON(w, http.StatusOK, production)
}

// PutProduccionPoligono handles PUT /api/v1/producciones/{id}/poligono.
//
// It rewrites only the monitoring polygon and its tight bbox. The download tile
// is left in place so the multiband rasters already generated stay aligned, and
// the ERP tables keep the original polygon. Scenes already processed are not
// re-queued: the new polygon applies to whatever is processed from now on.
func (h *Handlers) PutProduccionPoligono(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUintParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Poligono domain.Ring `json:"poligono"` // [[lon,lat],...]
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		Error(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	production, err := h.Productions.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
		return
	}
	if production == nil {
		Error(w, http.StatusNotFound, "produccion not found")
		return
	}

	// Never trust the client's own validation — GDAL refuses invalid cutlines
	// and a bad polygon would fail every scene from here on.
	ring := body.Poligono.Normalized()
	if err := ring.Validate(); err != nil {
		Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// The polygon must stay inside the tile already being downloaded, otherwise
	// the area outside it would have no pixels.
	if tile := production.ParseTileBBox(); tile != nil && !ring.WithinBBox(tile) {
		Error(w, http.StatusUnprocessableEntity,
			"el polígono sale del tile de descarga — mantén todos los vértices dentro del recuadro")
		return
	}

	bbox := ring.BBox()
	if bbox == nil {
		Error(w, http.StatusUnprocessableEntity, "el polígono no tiene vértices")
		return
	}

	poligonoJSON, err := json.Marshal(ring)
	if err != nil {
		Error(w, http.StatusInternalServerError, "marshaling poligono: "+err.Error())
		return
	}
	pboxJSON, err := json.Marshal(map[string]any{
		"min_lon": bbox.MinX, "min_lat": bbox.MinY,
		"max_lon": bbox.MaxX, "max_lat": bbox.MaxY,
		"pbox": []float64{bbox.MinX, bbox.MinY, bbox.MaxX, bbox.MaxY},
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "marshaling pbox: "+err.Error())
		return
	}

	if err := h.Productions.UpdatePolygon(r.Context(), production.ID, poligonoJSON, pboxJSON); err != nil {
		Error(w, http.StatusInternalServerError, "updating poligono: "+err.Error())
		return
	}

	production.PoligonoJSON = poligonoJSON
	production.PBoxJSON = pboxJSON
	production.PolygonBBoxJSON = pboxJSON

	JSON(w, http.StatusOK, map[string]any{
		"produccion":     production,
		"vertices":       len(ring),
		"area_hectareas": ring.AreaHectares(),
	})
}

// GetEscena handles GET /api/v1/escenas/{id}.
func (h *Handlers) GetEscena(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}
	scene, err := h.Scenes.GetByID(r.Context(), uint64(id))
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting escena: "+err.Error())
		return
	}
	if scene == nil {
		Error(w, http.StatusNotFound, "escena not found")
		return
	}
	JSON(w, http.StatusOK, scene)
}

// ListEscenaArchivos handles GET /api/v1/escenas/{id}/archivos.
// Returns all indexed files for the scene with presigned URLs.
func (h *Handlers) ListEscenaArchivos(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}
	files, err := h.Files.ListByEscena(r.Context(), uint64(id))
	if err != nil {
		Error(w, http.StatusInternalServerError, "listing archivos: "+err.Error())
		return
	}

	type fileItem struct {
		Tipo      string  `json:"tipo"`
		S3Key     string  `json:"s3_key"`
		Extension string  `json:"extension"`
		SizeBytes int64   `json:"size_bytes"`
		URL       *string `json:"url,omitempty"`
	}

	items := make([]fileItem, 0, len(files))
	for _, f := range files {
		item := fileItem{
			Tipo:      tipoFromKey(f.Tipo, f.S3Key),
			S3Key:     f.S3Key,
			Extension: f.Extension,
			SizeBytes: f.SizeBytes,
		}
		if f.S3Key != "" {
			if url, err := h.S3.PresignGetObject(r.Context(), h.S3Bucket, f.S3Key, PresignExpiry); err == nil {
				item.URL = &url
			}
		}
		items = append(items, item)
	}

	JSON(w, http.StatusOK, items)
}

// GetEscenaArchivo handles GET /api/v1/escenas/{id}/archivos/{tipo}.
func (h *Handlers) GetEscenaArchivo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}

	tipo := r.PathValue("tipo")
	if tipo == "" {
		Error(w, http.StatusBadRequest, "tipo is required")
		return
	}

	file, err := h.Files.GetByTipo(r.Context(), uint64(id), tipo)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting archivo: "+err.Error())
		return
	}

	// PNG bands (ndvi, rgb, natural, evi, etc.) are stored with tipo="image".
	// ia_result is stored as tipo="ia".
	// Fall back to scanning all files and matching by inferred tipo.
	if file == nil && (isImageBandType(tipo) || tipo == "ia_result") {
		allFiles, ferr := h.Files.ListByEscena(r.Context(), uint64(id))
		if ferr == nil {
			for _, f := range allFiles {
				if tipoFromKey(f.Tipo, f.S3Key) == tipo {
					file = f
					break
				}
			}
		}
	}

	if file == nil {
		Error(w, http.StatusNotFound, "archivo not found")
		return
	}

	bucket := h.S3Bucket

	url, err := h.S3.PresignGetObject(r.Context(), bucket, file.S3Key, PresignExpiry)
	if err != nil {
		Error(w, http.StatusInternalServerError, "presigning url: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, archivoResponse{
		FileName:  file.S3Key,
		FileType:  file.Tipo,
		URL:       url,
		ExpiresIn: int(PresignExpiry.Seconds()),
	})
}

type archivoResponse struct {
	FileName  string `json:"file_name"`
	FileType  string `json:"file_type"`
	URL       string `json:"url"`
	ExpiresIn int    `json:"expires_in_seconds"`
}

// ProxyEscenaArchivo handles GET /api/v1/escenas/{id}/archivos/{tipo}/stream.
// It fetches the S3 object through the API server and streams the bytes directly
// to the browser, so the browser never needs to reach S3 / LocalStack directly.
func (h *Handlers) ProxyEscenaArchivo(w http.ResponseWriter, r *http.Request) {
	if h.S3Stream == nil {
		Error(w, http.StatusServiceUnavailable, "streaming not configured")
		return
	}

	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}

	tipo := r.PathValue("tipo")
	if tipo == "" {
		Error(w, http.StatusBadRequest, "tipo is required")
		return
	}

	file, err := h.Files.GetByTipo(r.Context(), uint64(id), tipo)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting archivo: "+err.Error())
		return
	}

	if file == nil && (isImageBandType(tipo) || tipo == "ia_result") {
		allFiles, ferr := h.Files.ListByEscena(r.Context(), uint64(id))
		if ferr == nil {
			for _, f := range allFiles {
				if tipoFromKey(f.Tipo, f.S3Key) == tipo {
					file = f
					break
				}
			}
		}
	}

	if file == nil {
		Error(w, http.StatusNotFound, "archivo not found")
		return
	}

	body, ct, cl, streamErr := h.S3Stream.GetObjectStream(r.Context(), h.S3Bucket, file.S3Key)
	if streamErr != nil {
		Error(w, http.StatusInternalServerError, "streaming from S3: "+streamErr.Error())
		return
	}
	if body == nil {
		Error(w, http.StatusNotFound, fmt.Sprintf("object not found in S3: bucket=%s key=%s", h.S3Bucket, file.S3Key))
		return
	}
	defer body.Close()

	if ct == "" {
		ct = contentTypeFromKey(file.S3Key)
	}
	w.Header().Set("Content-Type", ct)
	if cl > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(cl, 10))
	}
	w.Header().Set("Cache-Control", "private, max-age=900")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// contentTypeFromKey guesses Content-Type from the S3 key extension.
func contentTypeFromKey(key string) string {
	if strings.HasSuffix(key, ".tif") || strings.HasSuffix(key, ".tiff") {
		return "image/tiff"
	}
	if strings.HasSuffix(key, ".png") {
		return "image/png"
	}
	if strings.HasSuffix(key, ".json") {
		return "application/json"
	}
	return "application/octet-stream"
}

// SyncRunner extends Syncer with an IsRunning check for idempotent triggers.
type SyncRunner interface {
	Syncer
	IsRunning() bool
}

// TriggerSync handles POST /api/v1/sync/trigger. It runs the sync cycle in
// the background and immediately returns 202 Accepted.
// Returns 409 Conflict when a sync cycle is already in progress.
func (h *Handlers) TriggerSync(w http.ResponseWriter, r *http.Request) {
	if h.Sync == nil {
		Error(w, http.StatusServiceUnavailable, "sync service not configured")
		return
	}

	if sr, ok := h.Sync.(SyncRunner); ok && sr.IsRunning() {
		JSON(w, http.StatusConflict, map[string]string{"status": "already_running"})
		return
	}

	log := h.Log
	go func() {
		if err := h.Sync.RunOnce(context.Background()); err != nil {
			if log != nil {
				log.Error("manual sync trigger failed", "error", err)
			}
		}
	}()

	JSON(w, http.StatusAccepted, map[string]string{"status": "triggered"})
}

// SyncStatus handles GET /api/v1/sync/status. Returns the last execution time,
// next scheduled execution, active cron expression and timezone.
func (h *Handlers) SyncStatus(w http.ResponseWriter, r *http.Request) {
	if h.Sync == nil {
		Error(w, http.StatusServiceUnavailable, "sync service not configured")
		return
	}
	JSON(w, http.StatusOK, h.Sync.Status())
}

// SyncEvents handles GET /api/v1/sync/events.
// Sends the current sync status as a single SSE event and instructs the
// browser to reconnect in 15 s.  Simple, stateless, no long-lived connection.
func (h *Handlers) SyncEvents(w http.ResponseWriter, r *http.Request) {
	if h.Sync == nil {
		Error(w, http.StatusServiceUnavailable, "sync service not configured")
		return
	}

	b, _ := json.Marshal(h.Sync.Status())

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// retry: tells EventSource to reconnect after 15 s.
	fmt.Fprintf(w, "retry: 15000\ndata: %s\n\n", b)
}

// healthDependencyStatus represents the status of a single dependency.
type healthDependencyStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`
}

// HealthDependencies handles GET /health/dependencies. It checks the status
// of GDAL, MySQL, S3, and DynamoDB and returns 200 with status info for each.
func (h *Handlers) HealthDependencies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	deps := map[string]healthDependencyStatus{}

	// Check MySQL
	if h.DB != nil {
		if err := h.DB.PingContext(ctx); err != nil {
			deps["mysql"] = healthDependencyStatus{Status: "error", Message: err.Error()}
		} else {
			deps["mysql"] = healthDependencyStatus{Status: "ok"}
		}
	} else {
		deps["mysql"] = healthDependencyStatus{Status: "error", Message: "database not configured"}
	}

	// Check GDAL
	if h.GDAL != nil {
		if version, err := h.GDAL.Run(ctx); err != nil {
			deps["gdal"] = healthDependencyStatus{Status: "error", Message: err.Error()}
		} else {
			deps["gdal"] = healthDependencyStatus{Status: "ok", Version: strings.TrimSpace(version)}
		}
	} else {
		deps["gdal"] = healthDependencyStatus{Status: "error", Message: "GDAL executor not configured"}
	}

	// Check S3
	if h.S3Health != nil {
		if err := h.S3Health.HeadBucket(ctx, h.S3Bucket); err != nil {
			deps["s3"] = healthDependencyStatus{Status: "error", Message: err.Error()}
		} else {
			deps["s3"] = healthDependencyStatus{Status: "ok"}
		}
	} else {
		deps["s3"] = healthDependencyStatus{Status: "error", Message: "S3 client not configured"}
	}

	// Check DynamoDB
	if h.DynamoDB != nil {
		if err := h.DynamoDB.DescribeTable(ctx, ""); err != nil {
			deps["dynamodb"] = healthDependencyStatus{Status: "error", Message: err.Error()}
		} else {
			deps["dynamodb"] = healthDependencyStatus{Status: "ok"}
		}
	} else {
		deps["dynamodb"] = healthDependencyStatus{Status: "error", Message: "DynamoDB client not configured"}
	}

	JSON(w, http.StatusOK, deps)
}

// GetEscenaAnalisis handles GET /api/v1/escenas/{id}/analisis.
// Returns the latest IA result for the scene, or 404 if none exists yet.
func (h *Handlers) GetEscenaAnalisis(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}
	if h.IAResults == nil {
		Error(w, http.StatusServiceUnavailable, "IA results not configured")
		return
	}
	result, err := h.IAResults.GetByEscenaID(r.Context(), uint64(id))
	if err != nil {
		Error(w, http.StatusInternalServerError, "fetching IA result: "+err.Error())
		return
	}
	if result == nil {
		Error(w, http.StatusNotFound, "no IA result found for this scene")
		return
	}
	JSON(w, http.StatusOK, result)
}

// TriggerEscenaAnalisis handles POST /api/v1/escenas/{id}/analizar.
//
// Normal mode: starts IA analysis in the background, returns 202 immediately.
//   - 409 if an analysis for this scene is already running.
//   - 422 if ia_req.json has not been generated yet.
//   - 503 if Bedrock is not configured.
//
// Dry-run mode (?dry_run=true): downloads ia_req.json from S3 and returns the
// fully assembled Bedrock prompt without making any API call. Useful for
// inspecting the prompt before spending tokens.
func (h *Handlers) TriggerEscenaAnalisis(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}
	if h.IA == nil {
		Error(w, http.StatusServiceUnavailable, "IA analyzer no configurado — define IA_AWS_ACCESS_KEY_ID y IA_AWS_SECRET_ACCESS_KEY")
		return
	}

	escenaID := uint64(id)
	dryRun := r.URL.Query().Get("dry_run") == "true"

	// Fetch scene — needed in both modes to validate and build the S3 key.
	scene, err := h.Scenes.GetByID(r.Context(), escenaID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "fetching scene: "+err.Error())
		return
	}
	if scene == nil {
		Error(w, http.StatusNotFound, "escena no encontrada")
		return
	}
	// Verify ia_req.json exists by checking the scene files table.
	if !scene.Usable {
		Error(w, http.StatusUnprocessableEntity, "escena no apta para análisis: revisa nubosidad y cobertura de datos")
		return
	}
	iaReqFile, err := h.Files.GetByTipo(r.Context(), escenaID, string(domain.FileIAReq))
	if err != nil || iaReqFile == nil {
		Error(w, http.StatusUnprocessableEntity, "ia_req.json aun no generado para esta escena — procesa la escena primero")
		return
	}

	// Build S3 key matching the path the worker wrote:
	// {production.Prefix}/{scene_name}/multiband.ia_req.json
	prod, err := h.Productions.GetByMonitoringID(r.Context(), scene.MonitoringProduccionID)
	if err != nil || prod == nil {
		Error(w, http.StatusInternalServerError, "produccion no encontrada para la escena")
		return
	}
	s3Key := strings.TrimRight(prod.Prefix, "/") + "/" + scene.SceneName + "/multiband.ia_req.json"

	// --- Dry-run: return assembled prompt, no Bedrock call ---
	if dryRun {
		result, err := h.IA.DryRun(r.Context(), s3Key)
		if err != nil {
			Error(w, http.StatusInternalServerError, "dry-run failed: "+err.Error())
			return
		}
		JSON(w, http.StatusOK, result)
		return
	}

	// --- Normal mode: reject double execution, trigger async ---
	if h.IA.IsRunning(escenaID) {
		Error(w, http.StatusConflict, "analisis IA ya en progreso para esta escena")
		return
	}

	go func() {
		ctx := context.Background()
		if _, err := h.IA.Analyze(ctx, escenaID, s3Key, prod.ProduccionID); err != nil {
			if !errors.Is(err, ia.ErrAlreadyRunning) {
				slog.Error("ia analysis failed", "escena_id", escenaID, "error", err)
			}
		}
	}()

	JSON(w, http.StatusAccepted, map[string]any{
		"status":    "triggered",
		"escena_id": escenaID,
		"s3_key":    s3Key,
	})
}

// TriggerWorkerProduction handles POST /api/v1/worker/run-production/{id}.
// {id} is the s3_monitoring_produccion_id (PK). The handler resolves it to the
// ERP produccion_id that the worker uses internally.
// Returns 409 if a global or per-production run is already active.
func (h *Handlers) TriggerWorkerProduction(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUintParam(w, r, "id")
	if !ok {
		return
	}

	// Resolve PK → ERP produccion_id for the worker trigger file.
	production, err := h.Productions.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
		return
	}
	if production == nil {
		Error(w, http.StatusNotFound, "produccion not found")
		return
	}
	produccionID := production.ProduccionID

	// Reject if global worker is actively running (not just sleeping).
	global, productions := daemon.ReadAllStatuses()
	if global != nil && global.Phase == daemon.PhaseProcessing {
		Error(w, http.StatusConflict, "worker global run is active — wait for it to finish")
		return
	}
	// Reject if any production run is active (serial execution: one at a time).
	if len(productions) > 0 {
		for _, p := range productions {
			if p.CurrentProduccionID == produccionID {
				Error(w, http.StatusConflict, "esta producción ya está siendo procesada")
				return
			}
		}
		Error(w, http.StatusConflict, "otra producción está siendo procesada — espera a que termine")
		return
	}

	if err := daemon.WriteProdTrigger(produccionID); err != nil {
		Error(w, http.StatusInternalServerError, "writing trigger: "+err.Error())
		return
	}

	JSON(w, http.StatusAccepted, map[string]any{
		"status":        "queued",
		"produccion_id": produccionID,
		"message":       "worker will pick up the request within 30 seconds",
	})
}

// TriggerWorkerAll handles POST /api/v1/worker/run.
// Requests an immediate full run of the --auto worker (all pending productions).
// Returns 409 if a global run is already in progress.
func TriggerWorkerAll(w http.ResponseWriter, r *http.Request) {
	global, productions := daemon.ReadAllStatuses()
	if global != nil && global.Phase == daemon.PhaseProcessing {
		Error(w, http.StatusConflict, "worker global run is already active")
		return
	}
	if len(productions) > 0 {
		Error(w, http.StatusConflict, "una producción está siendo procesada — espera a que termine antes de ejecutar todo")
		return
	}
	if err := daemon.WriteGlobalTrigger(); err != nil {
		Error(w, http.StatusInternalServerError, "writing global trigger: "+err.Error())
		return
	}
	JSON(w, http.StatusAccepted, map[string]any{
		"status":  "queued",
		"message": "global worker run will start within 30 seconds",
	})
}

// WorkerStatus handles GET /api/v1/worker/status.
// Returns the current status of the global (--auto) worker, all active
// per-production workers, and whether a graceful stop signal is pending.
func WorkerStatus(w http.ResponseWriter, r *http.Request) {
	global, productions := daemon.ReadAllStatuses()
	JSON(w, http.StatusOK, map[string]any{
		"global":       global,
		"productions":  productions,
		"stop_pending": daemon.StopTriggerExists(),
	})
}

// WorkerEvents handles GET /api/v1/worker/events.
// Streams worker status as Server-Sent Events, pushing a new event only when
// the status changes. The browser reconnects automatically on disconnect.
// Auth: token via cookie agro_token (EventSource doesn't support headers).
func WorkerEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Streaming not supported — fall back to a single status response.
		global, productions := daemon.ReadAllStatuses()
		writeSSEEvent(w, global, productions)
		return
	}

	type statusSnapshot struct {
		Global      *daemon.WorkerStatus
		Productions []*daemon.WorkerStatus
	}

	encode := func(g *daemon.WorkerStatus, ps []*daemon.WorkerStatus) string {
		b, _ := json.Marshal(map[string]any{
			"global":       g,
			"productions":  ps,
			"stop_pending": daemon.StopTriggerExists(),
		})
		return string(b)
	}

	var lastJSON string
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Send initial state immediately.
	g, ps := daemon.ReadAllStatuses()
	j := encode(g, ps)
	fmt.Fprintf(w, "data: %s\n\n", j)
	flusher.Flush()
	lastJSON = j

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			g, ps := daemon.ReadAllStatuses()
			j := encode(g, ps)
			if j == lastJSON {
				// No change — send a keep-alive comment so the connection stays open.
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", j)
			flusher.Flush()
			lastJSON = j
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, g *daemon.WorkerStatus, ps []*daemon.WorkerStatus) {
	b, _ := json.Marshal(map[string]any{
		"global":       g,
		"productions":  ps,
		"stop_pending": daemon.StopTriggerExists(),
	})
	fmt.Fprintf(w, "data: %s\n\n", b)
}

// CancelWorker handles POST /api/v1/worker/cancel.
// Uses a file-based stop signal so the worker finishes the current scene
// cleanly before stopping — no work is left half-done.
//
// Body (optional JSON):
//   - {} or omitted → write stop trigger; if already pending respond 200 "already_stopping"
//   - {"cancel_stop": true} → remove the stop trigger (undo a pending stop)
//
// The legacy {"produccion_id": N} field is accepted but ignored — stop is global.
func CancelWorker(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CancelStop bool `json:"cancel_stop"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.CancelStop {
		daemon.RemoveStopTrigger()
		JSON(w, http.StatusOK, map[string]any{"status": "stop_cancelled"})
		return
	}

	written, err := daemon.WriteStopTrigger()
	if err != nil {
		Error(w, http.StatusInternalServerError, "writing stop trigger: "+err.Error())
		return
	}
	if !written {
		JSON(w, http.StatusOK, map[string]any{"status": "already_stopping"})
		return
	}
	JSON(w, http.StatusOK, map[string]any{"status": "stop_requested"})
}

// UnlockWorker handles POST /api/v1/worker/unlock.
// Force-removes a stale lock and its state file.
// Body (optional JSON): {"produccion_id": 123} — omit for global (--auto).
func UnlockWorker(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProduccionID *int64 `json:"produccion_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var lockPath string
	if req.ProduccionID != nil {
		lockPath = daemon.ProductionLockPathFor(*req.ProduccionID)
	} else {
		lockPath = daemon.GlobalLockPath()
	}

	daemon.ForceUnlock(lockPath)
	JSON(w, http.StatusOK, map[string]string{"unlocked": lockPath})
}

// parseInt64Param parses the named path value as an int64, writing a 400
// error response and returning ok=false if it is missing or invalid.
func parseInt64Param(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	if raw == "" {
		Error(w, http.StatusBadRequest, name+" is required")
		return 0, false
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}

	return v, true
}

// tipoFromKey infers the canonical frontend tipo from the stored DB tipo and
// the S3 key filename. Source of truth: real DB values from production data:
//
//	DB tipo      filename                → frontend tipo
//	"truth_tif"  multiband.tif          → "multiband"
//	"image"      natural.png            → "rgb"
//	"image"      false_color.png        → "false_color"
//	"image"      red_edge.png           → "red_edge"
//	"image"      swir.png               → "swir"
//	"image"      ndvi/evi/savi/…png     → same name
//	"ia"         multiband.ia.json      → "ia_result"
//	"ia_req"     multiband.ia_req.json  → "ia_req"   (unchanged)
//	"params"     multiband.params.json  → "params"   (unchanged)
func tipoFromKey(dbTipo, s3Key string) string {
	switch dbTipo {
	case "truth_tif":
		return "multiband"
	case "ia":
		return "ia_result"
	case "image":
		// stem = filename without extension
		base := s3Key
		if idx := strings.LastIndexByte(base, '/'); idx >= 0 {
			base = base[idx+1:]
		}
		if idx := strings.LastIndexByte(base, '.'); idx >= 0 {
			base = base[:idx]
		}
		if base == "natural" {
			return "rgb"
		}
		return base
	default:
		return dbTipo
	}
}

// imageFileTypes is the complete set of frontend tipos that map to DB tipo="image".
var imageFileTypes = map[string]bool{
	"rgb": true, "false_color": true, "red_edge": true, "swir": true,
	"ndvi": true, "ndre": true, "evi": true, "gndvi": true,
	"nbr": true, "ndmi": true, "savi": true,
}

func isImageBandType(tipo string) bool {
	return imageFileTypes[tipo] || tipo == "multiband" || tipo == "ia_result"
}

// parseUintParam parses the named path value as a uint, writing a 400
// error response and returning ok=false if it is missing or invalid.
func parseUintParam(w http.ResponseWriter, r *http.Request, name string) (uint, bool) {
	raw := r.PathValue(name)
	if raw == "" {
		Error(w, http.StatusBadRequest, name+" is required")
		return 0, false
	}

	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		Error(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}

	return uint(v), true
}
