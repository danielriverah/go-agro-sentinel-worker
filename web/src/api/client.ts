import axios, { type AxiosError } from 'axios'
import type {
  ArchivoItem,
  ArchivoResponse,
  AsignacionRol,
  CentroCostoItem,
  FaseCultivo,
  FaseInput,
  IAResult,
  LoginResponse,
  PermisoCatalogo,
  PermisosEfectivos,
  Production,
  ProductionDetail,
  Rol,
  Scene,
  SyncStatus,
  TimelineResponse,
  Alerta,
  BorradoMonitoreo,
  PlantillaFases,
  UsuarioAdmin,
  UsuarioAsignaciones,
  WorkerStatus,
  WorkerStatusResponse,
} from './types'

// El token se guarda en localStorage y se adjunta automáticamente a cada request.
const http = axios.create({ baseURL: '/api/v1' })

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('agro_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

http.interceptors.response.use(
  (r) => r,
  (err: AxiosError) => {
    if (err.response?.status === 401) {
      // Token expirado o inválido — limpiar sesión y redirigir al login.
      localStorage.removeItem('agro_token')
      localStorage.removeItem('agro_user')
      window.location.href = '/login'
    }
    return Promise.reject(err)
  },
)

// Extrae el campo `data` de la respuesta envelope { data: ... }
function unwrap<T>(res: { data: { data: T } }): T {
  return res.data.data
}

// ── Auth ─────────────────────────────────────────────────────────────────────

export const auth = {
  login: (username: string, password: string) =>
    http.post<{ data: LoginResponse }>('/auth/login', { username, password }).then(unwrap),

  changePassword: (passwordActual: string, passwordNueva: string) =>
    http.post('/auth/change-password', { password_actual: passwordActual, password_nueva: passwordNueva }),
}

// ── Producciones ──────────────────────────────────────────────────────────────

export const producciones = {
  list: () =>
    http.get<{ data: Production[] }>('/producciones').then(unwrap),

  get: (id: number) =>
    http.get<{ data: ProductionDetail }>(`/producciones/${id}`).then(unwrap),

  // Bloquear y desbloquear escriben posible_cosecha; un trigger de la tabla
  // deriva el campo bloqueado a partir de él.
  desbloquear: (id: number) =>
    http.post<{ data: Production }>(`/producciones/${id}/desbloquear`).then(unwrap),

  bloquear: (id: number) =>
    http.post<{ data: Production }>(`/producciones/${id}/bloquear`).then(unwrap),

  timeline: (id: number) =>
    http.get<{ data: TimelineResponse }>(`/producciones/${id}/timeline`).then(unwrap),

  getFases: (id: number) =>
    http.get<{ data: FaseCultivo[] }>(`/producciones/${id}/fases`).then(unwrap),

  // Reemplaza el conjunto completo de fases; el servidor rechaza solapamientos.
  putFases: (id: number, fases: FaseInput[]) =>
    http.put<{ data: FaseCultivo[] }>(`/producciones/${id}/fases`, { fases }).then(unwrap),

  // Destructivo e irreversible: borra el monitoreo en MySQL, S3 y DynamoDB.
  // El folio va en el cuerpo como confirmación; el servidor lo compara.
  eliminarMonitoreo: (id: number, folio: string) =>
    http.delete<{ data: BorradoMonitoreo }>(`/producciones/${id}/monitoreo`, {
      data: { folio },
    }).then(unwrap),
}

// ── Escenas ───────────────────────────────────────────────────────────────────

export const escenas = {
  get: (id: number) =>
    http.get<{ data: Scene }>(`/escenas/${id}`).then(unwrap),

  listArchivos: (id: number) =>
    http.get<{ data: ArchivoItem[] }>(`/escenas/${id}/archivos`).then(unwrap),

  getArchivo: (id: number, tipo: string) =>
    http.get<{ data: ArchivoResponse }>(`/escenas/${id}/archivos/${tipo}`).then(unwrap),

  getAnalisis: (id: number) =>
    http.get<{ data: IAResult }>(`/escenas/${id}/analisis`).then(unwrap),

  analizar: (id: number, dryRun = false) =>
    http.post(`/escenas/${id}/analizar${dryRun ? '?dry_run=true' : ''}`),
}

// ── Sync ──────────────────────────────────────────────────────────────────────

export const sync = {
  status: () =>
    http.get<{ data: SyncStatus }>('/sync/status').then(unwrap),

  trigger: () =>
    http.post('/sync/trigger'),
}

// ── Producciones extra ────────────────────────────────────────────────────────

export const produccionesExtra = {
  patchIAAuto: (id: number, iaAuto: boolean) =>
    http.patch<{ data: Production }>(`/producciones/${id}/ia-auto`, { ia_auto: iaAuto }).then(unwrap),

  // poligono es [[lon,lat],...]; el servidor lo valida y lo normaliza a antihorario.
  putPoligono: (id: number, poligono: [number, number][]) =>
    http.put<{ data: { produccion: ProductionDetail; vertices: number; area_hectareas: number } }>(
      `/producciones/${id}/poligono`, { poligono },
    ).then(unwrap),
}

// ── Worker ────────────────────────────────────────────────────────────────────

export const worker = {
  status: () =>
    http.get<{ data: WorkerStatusResponse }>('/worker/status').then(unwrap),

  runAll: () =>
    http.post('/worker/run'),

  runProduction: (produccionId: number) =>
    http.post(`/worker/run-production/${produccionId}`),

  cancel: (produccionId?: number, cancelStop = false) =>
    http.post('/worker/cancel', {
      ...(produccionId != null ? { produccion_id: produccionId } : {}),
      ...(cancelStop ? { cancel_stop: true } : {}),
    }),

  unlock: (produccionId?: number) =>
    http.post('/worker/unlock', produccionId != null ? { produccion_id: produccionId } : {}),
}

// ── Fases: plantillas para copiar ─────────────────────────────────────────────

export const fases = {
  // Sólo producciones que YA tienen fases, con las fases incluidas para poder
  // revisarlas antes de copiarlas.
  plantillas: () =>
    http.get<{ data: PlantillaFases[] }>('/fases/plantillas').then(unwrap),
}

// ── Alertas ───────────────────────────────────────────────────────────────────

export const alertas = {
  list: (params: { estado?: string; severidad?: string; produccion_id?: number; limite?: number } = {}) =>
    http.get<{ data: Alerta[] }>('/alertas', { params }).then(unwrap),

  marcarVista: (id: number) =>
    http.post(`/alertas/${id}/vista`),

  marcarResuelta: (id: number) =>
    http.post(`/alertas/${id}/resuelta`),
}

// ── Administración ────────────────────────────────────────────────────────────

export const admin = {
  listUsuarios: () =>
    http.get<{ data: UsuarioAdmin[] }>('/admin/usuarios').then(unwrap),

  setActivo: (userId: number, activo: boolean) =>
    http.put(`/admin/usuarios/${userId}/activo`, { activo }),

  // El administrador no necesita la contraseña actual.
  resetPassword: (userId: number, passwordNueva: string) =>
    http.put(`/admin/usuarios/${userId}/password`, { password_nueva: passwordNueva }),
}

// ── Permisos ─────────────────────────────────────────────────────────────────

export const permisos = {
  mis: () =>
    http.get<{ data: PermisosEfectivos }>('/auth/permisos').then(unwrap),

  refrescar: () =>
    http.post<{ data: PermisosEfectivos }>('/auth/refrescar-permisos').then(unwrap),
}

// ── Roles y asignaciones (admin) ─────────────────────────────────────────────

export const roles = {
  list: () =>
    http.get<{ data: Rol[] }>('/admin/roles').then(unwrap),

  create: (nombre: string, descripcion: string, permiso_ids: number[]) =>
    http.post<{ data: Rol }>('/admin/roles', { nombre, descripcion, permiso_ids }).then(unwrap),

  update: (id: number, nombre: string, descripcion: string, permiso_ids: number[]) =>
    http.put(`/admin/roles/${id}`, { nombre, descripcion, permiso_ids }),

  delete: (id: number) =>
    http.delete(`/admin/roles/${id}`),

  permisosCatalogo: () =>
    http.get<{ data: PermisoCatalogo[] }>('/admin/permisos').then(unwrap),

  getUsuarioPermisos: (userId: number) =>
    http.get<{ data: UsuarioAsignaciones }>(`/admin/usuarios/${userId}/permisos`).then(unwrap),

  setUsuarioRoles: (userId: number, asignaciones: AsignacionRol[]) =>
    http.put(`/admin/usuarios/${userId}/roles`, { asignaciones }),

  setUsuarioPermisos: (userId: number, asignaciones: { permiso_id: number; centro_costo_id: number | null }[]) =>
    http.put(`/admin/usuarios/${userId}/permisos-directos`, { asignaciones }),

  centrosCostos: () =>
    http.get<{ data: CentroCostoItem[] }>('/admin/centros-costos').then(unwrap),
}

// Helpers para mensajes de error del backend
export function apiErrorMessage(err: unknown): string {
  const e = err as AxiosError<{ error?: string }>
  return e.response?.data?.error ?? (err instanceof Error ? err.message : 'Error desconocido')
}
