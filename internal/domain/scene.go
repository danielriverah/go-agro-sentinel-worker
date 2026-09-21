package domain

import "time"

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
	FechaCreacion          time.Time  // fecha_creacion
	FechaActualizacion     *time.Time // fecha_actualizacion

	// In-memory only — not columns of s3_monitoring_escenas.
	RetryCount   int
	ErrorType    string
	ErrorMessage string
	ProcessedAt  *time.Time
}

func (s *Scene) NeedsProcessing() bool {
	return s.Status == StatusPending || s.Status == ""
}

func (s *Scene) CanRetry(maxRetries int) bool {
	return s.Status == StatusFailed && s.RetryCount < maxRetries
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
