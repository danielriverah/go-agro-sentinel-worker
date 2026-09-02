package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// PresignExpiry is how long presigned S3 URLs served by the API remain valid.
const PresignExpiry = 15 * time.Minute

// ProductionRepository is the subset of database.ProductionRepo the API needs.
type ProductionRepository interface {
	ListActive(ctx context.Context) ([]*domain.Production, error)
	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
	Desbloquear(ctx context.Context, produccionID int64, usuario string) error
}

// SceneRepository is the subset of database.SceneRepo the API needs.
type SceneRepository interface {
	ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.Scene, error)
	GetByID(ctx context.Context, id int64) (*domain.Scene, error)
}

// FileRepository is the subset of database.FileRepo the API needs.
type FileRepository interface {
	ListByEscena(ctx context.Context, escenaID int64) ([]*domain.SceneFile, error)
	GetByType(ctx context.Context, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error)
}

// Presigner is the subset of aws.S3Client the API needs.
type Presigner interface {
	PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// Syncer triggers a sync cycle. Implemented by *sync.Service.
type Syncer interface {
	RunOnce(ctx context.Context) error
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
	Files       FileRepository
	S3          Presigner
	S3Bucket    string
	Sync        Syncer
}

// desbloquearRequest is the optional body for POST .../desbloquear.
type desbloquearRequest struct {
	Usuario string `json:"usuario"`
}

// ListProducciones handles GET /api/v1/producciones.
func (h *Handlers) ListProducciones(w http.ResponseWriter, r *http.Request) {
	productions, err := h.Productions.ListActive(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "listing producciones: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, productions)
}

// GetProduccion handles GET /api/v1/producciones/{id}.
func (h *Handlers) GetProduccion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}

	production, err := h.Productions.GetByProduccionID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
		return
	}
	if production == nil {
		Error(w, http.StatusNotFound, "produccion not found")
		return
	}

	scenes, err := h.Scenes.ListByProduccion(r.Context(), id)
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
func (h *Handlers) DesbloquearProduccion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64Param(w, r, "id")
	if !ok {
		return
	}

	var req desbloquearRequest
	if r.Body != nil {
		// Body is optional; ignore decode errors for an empty body.
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	if err := h.Productions.Desbloquear(r.Context(), id, req.Usuario); err != nil {
		Error(w, http.StatusInternalServerError, "desbloqueando produccion: "+err.Error())
		return
	}

	production, err := h.Productions.GetByProduccionID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting produccion: "+err.Error())
		return
	}
	if production == nil {
		Error(w, http.StatusNotFound, "produccion not found")
		return
	}

	JSON(w, http.StatusOK, production)
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

	file, err := h.Files.GetByType(r.Context(), id, domain.FileType(tipo))
	if err != nil {
		Error(w, http.StatusInternalServerError, "getting archivo: "+err.Error())
		return
	}
	if file == nil {
		Error(w, http.StatusNotFound, "archivo not found")
		return
	}

	bucket := file.S3Bucket
	if bucket == "" {
		bucket = h.S3Bucket
	}

	url, err := h.S3.PresignGetObject(r.Context(), bucket, file.S3Key, PresignExpiry)
	if err != nil {
		Error(w, http.StatusInternalServerError, "presigning url: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, archivoResponse{
		FileName:  file.FileName,
		FileType:  string(file.FileType),
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

// TriggerSync handles POST /api/v1/sync/trigger. It runs the sync cycle in
// the background and immediately returns 202 Accepted.
func (h *Handlers) TriggerSync(w http.ResponseWriter, r *http.Request) {
	if h.Sync == nil {
		Error(w, http.StatusServiceUnavailable, "sync service not configured")
		return
	}

	go func() {
		_ = h.Sync.RunOnce(context.Background())
	}()

	JSON(w, http.StatusAccepted, map[string]string{"status": "triggered"})
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
