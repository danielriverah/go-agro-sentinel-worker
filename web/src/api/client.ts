import axios, { type AxiosError } from 'axios'
import type {
  ArchivoItem,
  ArchivoResponse,
  IAResult,
  LoginResponse,
  Production,
  ProductionDetail,
  Scene,
  SyncStatus,
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

  desbloquear: (id: number) =>
    http.post<{ data: Production }>(`/producciones/${id}/desbloquear`).then(unwrap),
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

// Helpers para mensajes de error del backend
export function apiErrorMessage(err: unknown): string {
  const e = err as AxiosError<{ error?: string }>
  return e.response?.data?.error ?? (err instanceof Error ? err.message : 'Error desconocido')
}
