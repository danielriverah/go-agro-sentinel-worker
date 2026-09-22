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
  CentroCostoID: number
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
  // Extensión real del raster. null = usa el tile_bbox de su producción;
  // con valor = heredada de la escena origen del multiband reutilizado.
  // Evita tener que resolver la cadena escena -> origen -> producción.
  ImageBBox: unknown | null
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

// ── Timeline de índices ───────────────────────────────────────────────────────

export interface TimelineSerie {
  clave: string
  principal: boolean
  grupo: 'vegetacion' | 'humedad'
}

export interface TimelineExtremo {
  fecha: string
  valor: number
}

export interface TimelineResumen {
  puntos_totales: number
  puntos_confiables: number
  mejor_dia: TimelineExtremo | null
  peor_dia: TimelineExtremo | null
  tendencia: 'mejorando' | 'declinando' | 'estable'
}

export interface TimelineProduccion {
  id: number
  folio: string
  rancho: string
  cosecha: string
  variedades: string
  fecha_plantacion: string | null
  fecha_fin: string | null
}

export interface TimelinePunto {
  escena_id: number
  scene_name: string
  fecha: string
  dia_cultivo: number | null
  dias_a_cosecha: number | null
  nubosidad: number | null
  confiable: boolean
  // Presente sólo cuando confiable=false: sin_dato_sensor, sin_params,
  // nubosidad_alta o escena_no_usable.
  motivo_no_confiable?: string
  // Ausentes en puntos no confiables: el backend los omite a propósito para
  // que la gráfica no dibuje una caída que en realidad es ruido de nubes.
  valores?: Record<string, number>
  delta?: Record<string, number>
  estado?: Record<string, string>
  ia?: { estado: string; riesgo: string; motivo?: string }
}

export interface TimelineFase {
  nombre: string
  dia_inicio: number
  dia_fin: number
  color: string
}

export interface TimelineResponse {
  produccion: TimelineProduccion
  resumen: TimelineResumen
  series: TimelineSerie[]
  puntos: TimelinePunto[]
  fases: TimelineFase[]
}

// Fase del ciclo de cultivo. Cuelga del produccion_id del ERP, no del
// monitoreo, para sobrevivir al borrado de éste.
export interface FaseCultivo {
  id: number
  produccion_id: number
  nombre: string
  dia_inicio: number
  dia_fin: number
  orden: number
}

export interface FaseInput {
  nombre: string
  dia_inicio: number
  dia_fin: number
}

// Usuario en la pantalla de administración. El backend nunca expone hash ni salt.
export interface UsuarioAdmin {
  user_id: number
  username: string
  activo: number
  fecha_creacion: string
}

// Resultado del borrado de monitoreo, desglosado por sistema.
export interface BorradoMonitoreo {
  produccion_id: number
  folio: string
  s3_objetos_borrados: number
  dynamodb_escenas_borradas: number
  mysql: {
    escenas_dependientes_desvinculadas: number
    archivos: number
    analisis_ia: number
    escenas: number
    filas_monitoreo: number
    monitoring_erp_apagado: boolean
  }
  advertencia?: string
}

// ── Alertas (notificaciones de la IA) ─────────────────────────────────────────

export type AlertaEstado = 'nueva' | 'vista' | 'resuelta'
export type AlertaSeveridad = 'baja' | 'media' | 'alta'

export interface Alerta {
  monitoring_alerta_id: number
  produccion_id: number
  scene_name?: string
  scene_date?: string
  alert_type: string
  severity: AlertaSeveridad
  estado: AlertaEstado
  title: string
  message: string
  action_suggested?: string
  source: string
  notify_email: boolean
  seen_at?: string
  seen_by?: string
  resolved_at?: string
  resolved_by?: string
  created_at: string
  updated_at?: string
  // Enriquecidos por JOIN con el ERP, para agrupar en la vista.
  folio?: string
  rancho?: string
  cultivo?: string
}

// Producción que ya tiene fases y puede servir de modelo para copiar.
// Trae las fases incluidas para poder revisarlas antes de aplicarlas.
export interface PlantillaFases {
  produccion_id: number
  folio: string
  cultivo: string
  rancho: string
  fases: FaseCultivo[]
}

// ── Permisos y Roles ──────────────────────────────────────────────────────────

export interface PermisosEfectivos {
  global: string[]
  por_rancho: Record<string, string[]>
  ranchos_todos: boolean
  degraded: boolean
}

export interface PermisoCatalogo {
  permiso_id: number
  clave: string
  modulo: string
  descripcion: string
  scope: 'global' | 'rancho'
}

export interface Rol {
  rol_id: number
  nombre: string
  descripcion: string
  es_sistema: boolean
  permiso_ids: number[]
}

export interface AsignacionRol {
  rol_id: number
  centro_costo_id: number | null
}

export interface AsignacionRolDetalle {
  usuario_rol_id: number
  rol_id: number
  rol_nombre: string
  centro_costo_id: number | null
  rancho_nombre: string
}

export interface AsignacionPermisoDetalle {
  usuario_permiso_id: number
  permiso_id: number
  permiso_clave: string
  centro_costo_id: number | null
  rancho_nombre: string
}

export interface UsuarioAsignaciones {
  roles: AsignacionRolDetalle[]
  directos: AsignacionPermisoDetalle[]
}

export interface CentroCostoItem {
  centro_costo_id: number
  nombre: string
}
