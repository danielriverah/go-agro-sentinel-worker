import type { Geometry } from 'geojson'

// Tipos que reflejan exactamente los schemas del backend (docs.go / OpenAPI 1.1.0)

export type FileType =
  | 'multiband' | 'rgb' | 'false_color' | 'red_edge' | 'swir'
  | 'ndvi' | 'evi' | 'savi' | 'ndre' | 'gndvi' | 'nbr' | 'ndmi'
  | 'params' | 'ia_req' | 'ia_result'

export interface ProductionStats {
  scenes_total: number
  scenes_usables: number
  scenes_completadas: number
  scenes_pendientes: number
  scenes_tif_ok: number
  scenes_ia_ok: number
  last_tif_fecha: string | null
  last_ia_fecha: string | null
}

// Formato real en DB: {"pbox":[min_lon,min_lat,max_lon,max_lat], "min_lat":..., "max_lat":..., "min_lon":..., "max_lon":...}
export interface PBoxJSON {
  pbox: [number, number, number, number] // [min_lon, min_lat, max_lon, max_lat]
  min_lat: number
  max_lat: number
  min_lon: number
  max_lon: number
}

export interface Production {
  ID: number
  ProduccionID: number
  Folio: string
  Rancho: string
  Cosecha: string
  Prefix: string
  Monitoring: boolean
  Bloqueado: boolean
  IAuto: boolean
  PosibleCosecha: boolean
  TileCenterLat: number | null
  TileCenterLon: number | null
  TileEdgeMeters: number
  PBoxJSON: PBoxJSON | null        // bbox del polígono del cultivo — extent real de los PNGs
  PoligonoJSON: Geometry | null
  TileBBoxJSON: PBoxJSON | null    // tile de descarga (2000m), más grande que pbox
  FechaPlantacion: string | null
  FechaFin: string | null
  TifCompleteAt: string | null
  IaCompleteAt: string | null
  UltimaSincronizacion: string | null
  stats: ProductionStats | null
}

export interface ProductionDetail extends Production {
  escenas: Scene[]
}

export type SceneStatus = 'PENDING' | 'PROCESSING' | 'COMPLETED' | 'FAILED' | 'SKIPPED'
export type RiesgoNivel = 'bajo' | 'medio' | 'alto'

export interface Scene {
  ID: number
  MonitoringProduccionID: number
  SceneName: string
  Fecha: string | null
  CloudCover: number | null
  ProductionCloud: number | null
  Status: SceneStatus
  Usable: boolean
  TruthTifExists: boolean
  ParamsExists: boolean
  IaExists: boolean
  Analysis: boolean
  LatestIaRiesgoNivel: RiesgoNivel | ''
  LatestIaFechaAnalisis: string | null
  MultibandRefEscenaID: number | null
}

export interface ArchivoItem {
  tipo: FileType
  s3_key: string
  extension: string
  size_bytes: number
  url: string | null
}

export interface ArchivoResponse {
  file_name: string
  file_type: string
  url: string
  expires_in_seconds: number
}

export type EstadoClave = 'optimo' | 'bueno' | 'alerta' | 'critico' | 'normal' | 'anomalia' | 'sin_datos'

export interface IAHallazgo {
  tipo: string
  zona: string
  severidad: 'baja' | 'media' | 'alta'
  descripcion: string
}

export interface IAResultDetail {
  estado_clave: EstadoClave
  estado_general: string
  resumen: string
  hallazgos: IAHallazgo[]
  recomendaciones: string[]
  riesgo: { nivel: RiesgoNivel; motivo: string }
  posible_cosecha: boolean
}

export interface IAResult {
  id: number
  escena_id: number
  estado_clave: EstadoClave
  estado_general: string
  riesgo_nivel: RiesgoNivel
  riesgo_motivo: string
  fecha_analisis: string | null
  json_original: string | null
}

export type WorkerPhase = 'processing' | 'sleeping'
export type WorkerMode = 'auto' | 'manual-all' | 'production' | 'scene' | 'regen' | 'trigger-production'

export interface WorkerStatus {
  mode: WorkerMode
  pid: number
  phase: WorkerPhase
  started_at: string
  current_scene: string | null
  current_produccion_id: number | null
  scenes_done: number
  scenes_failed: number
  scenes_total: number
  next_schedule_at: string | null
  last_completed_at: string | null
}

export interface WorkerStatusResponse {
  global: WorkerStatus | null
  productions: WorkerStatus[] | null
  stop_pending: boolean
}

export type SkipReason =
  | 'sin_poligono'
  | 'sin_fecha_plantacion'
  | 'no_existe_en_erp'
  | 'bloqueada'
  | 'fin_monitoreo'

export interface SkippedProd {
  produccion_id: number
  folio?: string
  reason: SkipReason
}

// Resultado del último ciclo de sync — permite saber si MySQL quedó al día con
// DynamoDB, no solo cuándo corrió el ciclo.
export interface SyncReport {
  started_at: string
  finished_at: string
  prods_in_dynamo: number
  prods_upserted: number
  prods_skipped?: SkippedProd[]
  prods_monitoring: number
  escenas_inserted: number
  errors?: string[]
}

export interface SyncStatus {
  running: boolean
  schedule: string
  timezone: string
  last_run: string | null
  next_run: string | null
  report?: SyncReport | null
}

export interface LoginResponse {
  token: string
  expires_at: string
  username: string
}
