import { computed, ref, watch, type Ref } from 'vue'
import { producciones } from '@/api/client'
import { apiErrorMessage } from '@/api/client'
import type { TimelinePunto, TimelineResponse } from '@/api/types'

export type RangePreset = 'semana' | 'mes' | 'todo' | 'custom'

const DAY_MS = 24 * 60 * 60 * 1000

/**
 * Parsea "2024-06-15" como fecha local.
 *
 * `new Date("2024-06-15")` la interpreta como UTC, lo que en México (UTC-6)
 * la corre al día anterior y desalinea la gráfica un día completo.
 */
export function parseLocalDate(s: string): Date {
  return new Date(`${s}T00:00:00`)
}

export function useTimeline(produccionId: Ref<number>) {
  const data = ref<TimelineResponse | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  const preset = ref<RangePreset>('todo')
  const viewStart = ref<Date | null>(null)
  const viewEnd = ref<Date | null>(null)

  const allPoints = computed<TimelinePunto[]>(() => data.value?.puntos ?? [])

  /** Rango completo disponible, derivado de los puntos recibidos. */
  const bounds = computed(() => {
    const pts = allPoints.value
    const first = pts[0]
    const last = pts[pts.length - 1]
    if (!first || !last) return null
    return { start: parseLocalDate(first.fecha), end: parseLocalDate(last.fecha) }
  })

  const points = computed<TimelinePunto[]>(() => {
    const start = viewStart.value
    const end = viewEnd.value
    if (!start || !end) return allPoints.value
    return allPoints.value.filter((p) => {
      const d = parseLocalDate(p.fecha)
      return d >= start && d <= end
    })
  })

  const reliablePoints = computed(() => points.value.filter((p) => p.confiable))

  /** Días que abarca la vista actual; decide la densidad de etiquetas del eje X. */
  const spanDays = computed(() => {
    if (!viewStart.value || !viewEnd.value) return 0
    return Math.round((viewEnd.value.getTime() - viewStart.value.getTime()) / DAY_MS)
  })

  function resetToAll() {
    preset.value = 'todo'
    viewStart.value = bounds.value?.start ?? null
    viewEnd.value = bounds.value?.end ?? null
  }

  function selectPreset(p: Exclude<RangePreset, 'custom'>) {
    const b = bounds.value
    if (!b) return
    preset.value = p

    if (p === 'todo') {
      viewStart.value = b.start
      viewEnd.value = b.end
      return
    }

    // La ventana se ancla al final de los datos, no a hoy: si la última
    // captura es de hace tres semanas, "última semana" quedaría vacía.
    const days = p === 'semana' ? 7 : 30
    const end = b.end
    const start = new Date(end.getTime() - days * DAY_MS)
    viewStart.value = start < b.start ? b.start : start
    viewEnd.value = end
  }

  function selectCustom(start: Date, end: Date) {
    preset.value = 'custom'
    viewStart.value = start
    viewEnd.value = end
  }

  /** Desplaza la ventana una duración completa hacia atrás o adelante. */
  function shift(direction: -1 | 1) {
    const b = bounds.value
    if (!b || !viewStart.value || !viewEnd.value) return

    const span = viewEnd.value.getTime() - viewStart.value.getTime()
    if (span <= 0) return

    let start = new Date(viewStart.value.getTime() + direction * span)
    let end = new Date(viewEnd.value.getTime() + direction * span)

    if (start < b.start) {
      start = b.start
      end = new Date(Math.min(b.start.getTime() + span, b.end.getTime()))
    }
    if (end > b.end) {
      end = b.end
      start = new Date(Math.max(b.end.getTime() - span, b.start.getTime()))
    }

    preset.value = 'custom'
    viewStart.value = start
    viewEnd.value = end
  }

  const canShiftBack = computed(
    () => !!bounds.value && !!viewStart.value && viewStart.value > bounds.value.start,
  )
  const canShiftForward = computed(
    () => !!bounds.value && !!viewEnd.value && viewEnd.value < bounds.value.end,
  )

  async function load() {
    if (!produccionId.value) return
    loading.value = true
    error.value = null
    try {
      data.value = await producciones.timeline(produccionId.value)
      resetToAll()
    } catch (e) {
      error.value = apiErrorMessage(e)
      data.value = null
    } finally {
      loading.value = false
    }
  }

  watch(produccionId, load, { immediate: true })

  return {
    data,
    loading,
    error,
    points,
    allPoints,
    reliablePoints,
    bounds,
    viewStart,
    viewEnd,
    spanDays,
    preset,
    canShiftBack,
    canShiftForward,
    load,
    selectPreset,
    selectCustom,
    shift,
  }
}
