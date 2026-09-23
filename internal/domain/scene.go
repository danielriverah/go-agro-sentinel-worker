package domain

import (
	"encoding/json"
	"time"
)

// JobStatus is the processing status of a scene.
type JobStatus = string

const (
	StatusPending    JobStatus = "PENDING"
	StatusProcessing JobStatus = "PROCESSING"
	StatusCompleted  JobStatus = "COMPLETED"
	StatusFailed     JobStatus = "FAILED"
)

// Scene maps to s3_monitoring_escenas.
// RetryCount, ErrorType, ErrorMessage and ProcessedAt are not columns of the
// real table — they are kept in memory to support worker logic only.
type Scene struct {
	ID                     uint64     // s3_monitoring_escena_id
	MonitoringProduccionID uint       // s3_monitoring_produccion_id (FK → s3_monitoring_producciones)
	SceneName              string     // scene_name
	Fecha                  *time.Time // fecha
	SceneJsonKey           string     // scene_json_key
	SceneJsonUri           string     // scene_json_uri
	CloudCover             *float64   // cloud_cover
	Status                 string     // status
	TruthTifExists         bool       // truth_tif_exists
	RenderTifExists        bool       // render_tif_exists
	ParamsExists           bool       // params_exists
	IaExists               bool       // ia_exists
	Fase2CompletaAt        *time.Time // fase2_completa_at
	LatestIaRiesgoNivel    string     // latest_ia_riesgo_nivel
	LatestIaFechaAnalisis  *time.Time // latest_ia_fecha_analisis
	UltimaSincronizacion   *time.Time // ultima_sincronizacion
	ProductionCloud        *float64   // production_cloud
	Usable                 bool       // usable
	Analysis               bool       // analysis
	UrlsBandas             string     // urls_bandas
	BaseBands              string     // base_bands — URL base de las bandas (sin nombre de archivo)
	MultibandRefEscenaID   *uint64    // multiband_ref_escena_id — escena origen del multiband reutilizado (NULL = propio)
	// ImageBBox es la extensión geográfica real del raster de esta escena.
	// Vacío = la escena usa el tile_bbox de su propia producción; con valor =
	// tile_bbox de la escena origen cuando se reutilizó su multiband. Guardarlo
	// aquí hace la escena autosuficiente: sin él, la georreferencia depende de
	// que la producción origen siga existiendo.
	ImageBBox          json.RawMessage // image_bbox (JSON)
	FechaCreacion      time.Time       // fecha_creacion
	FechaActualizacion *time.Time      // fecha_actualizacion

	// In-memory only — not columns of s3_monitoring_escenas.
	RetryCount   int
	ErrorType    string
	ErrorMessage string
	ProcessedAt  *time.Time
}

func (s *Scene) NeedsProcessing() bool {
	return s.Status == StatusPending || s.Status == ""
}

// EffectiveBBox devuelve la extensión geográfica real de las imágenes de esta
// escena: la propia cuando reutilizó el multiband de otra producción, y el
// tile_bbox de prod en caso contrario.
//
// Es el único punto donde debe resolverse esa extensión: seguir la cadena
// multiband_ref_escena_id a mano deja de funcionar en cuanto se borra la
// producción origen.
func (s *Scene) EffectiveBBox(prod *Production) *BBox {
	// image_bbox se copia tal cual desde tile_bbox, así que comparte formato
	// y puede reutilizar el mismo parser.
	if len(s.ImageBBox) > 0 {
		if b := (&Production{TileBBoxJSON: s.ImageBBox}).ParseTileBBox(); b != nil {
			return b
		}
	}
	if prod == nil {
		return nil
	}
	return prod.ParseTileBBox()
}

func (s *Scene) CanRetry(maxRetries int) bool {
	return s.Status == StatusFailed && s.RetryCount < maxRetries
}

// TimelineRow is one scene's raw input for the indices timeline: the scene
// fields the chart needs plus the params.json content, which carries the
// per-index statistics. ParamsJSON is empty when the scene has no params file
// indexed yet.
type TimelineRow struct {
	EscenaID        uint64
	SceneName       string
	Fecha           *time.Time
	CloudCover      *float64
	ProductionCloud *float64
	Usable          bool
	Status          string
	ParamsJSON      string
}

// MultibandSource is a candidate multiband.tif from another production that
// might cover the current production's polygon.
type MultibandSource struct {
	EscenaID     uint64 // s3_monitoring_escena_id of the origin scene
	TileBBox     BBox   // tile_bbox of the origin production
	MultibandKey string // S3 key of multiband.tif
}

// SceneFile maps to s3_monitoring_escena_archivos.
type SceneFile struct {
	ID            uint64     // s3_monitoring_escena_archivo_id
	EscenaID      uint64     // s3_monitoring_escena_id
	Tipo          string     // tipo (e.g. multiband, rgb, params, ia)
	S3Key         string     // s3_key
	S3KeyHash     string     // s3_key_hash (MD5 of s3_key, NOT NULL)
	S3Uri         string     // s3_uri  (s3://bucket/key)
	Extension     string     // extension
	SizeBytes     int64      // size_bytes
	LastModified  *time.Time // last_modified
	Existe        bool       // existe
	JsonContent   string     // json_content
	FechaCreacion time.Time  // fecha_creacion
}
