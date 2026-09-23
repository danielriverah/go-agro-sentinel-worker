<script setup lang="ts">
import { computed, ref, toRef, watch } from 'vue'
import { useWindowSize } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useTimeline, parseLocalDate } from '@/composables/useTimeline'
import type { TimelinePunto } from '@/api/types'

const props = defineProps<{ produccionId: number }>()
const emit = defineEmits<{ (e: 'select', escenaId: number): void }>()

const { t, locale } = useI18n()

const {
  data, loading, error, points, allPoints, bounds,
  viewStart, viewEnd, spanDays, preset, load,
  canShiftBack, canShiftForward, selectPreset, selectCustom, shift,
} = useTimeline(toRef(props, 'produccionId'))

// ── Series ────────────────────────────────────────────────────────────────────

const SERIE_COLORS: Record<string, string> = {
  ndvi: '#16a34a', ndmi: '#2563eb', evi: '#65a30d', savi: '#ca8a04',
  ndre: '#0d9488', gndvi: '#7c3aed', nbr: '#0891b2',
}

const active = ref<Set<string>>(new Set(['ndvi', 'ndmi']))

function toggleSerie(clave: string) {
  const next = new Set(active.value)
  if (next.has(clave)) {
    if (next.size === 1) return // nunca dejar la gráfica sin ninguna serie
    next.delete(clave)
  } else {
    next.add(clave)
  }
  active.value = next
}

const activeSeries = computed(() =>
  (data.value?.series ?? []).filter((s) => active.value.has(s.clave)),
)

// ── Geometría ─────────────────────────────────────────────────────────────────

const W = 800
const H = 300
const PAD = { top: 12, right: 14, bottom: 26, left: 34 }
const plotW = W - PAD.left - PAD.right
const plotH = H - PAD.top - PAD.bottom

// En móvil el viewBox de 800 se escala a ~340 px reales, así que un texto de
// 9 acaba en 4 px ilegibles. El tamaño se compensa en unidades de viewBox.
const { width: winWidth } = useWindowSize()
const isNarrow = computed(() => winWidth.value < 640)
const tickFont = computed(() => (isNarrow.value ? 17 : 9))

// Dominio fijo en lugar de ajustado a los datos: así dos producciones
// distintas se leen a la misma escala y una mala no "se ve igual" que una buena.
const Y_MIN = -0.2
const Y_MAX = 1.0

function yScale(v: number): number {
  const clamped = Math.min(Math.max(v, Y_MIN), Y_MAX)
  return PAD.top + (1 - (clamped - Y_MIN) / (Y_MAX - Y_MIN)) * plotH
}

const xDomain = computed(() => {
  const s = viewStart.value?.getTime() ?? 0
  const e = viewEnd.value?.getTime() ?? 0
  return { s, e, span: e - s }
})

function xScale(fecha: string): number {
  const { s, span } = xDomain.value
  if (span <= 0) return PAD.left + plotW / 2
  return PAD.left + ((parseLocalDate(fecha).getTime() - s) / span) * plotW
}

// ── Bandas de estado ──────────────────────────────────────────────────────────

// Los umbrales difieren entre vigor y humedad, así que las bandas sólo se
// dibujan cuando hay una sola serie activa; con varias serían ambiguas.
const bands = computed(() => {
  const only = activeSeries.value.length === 1 ? activeSeries.value[0] : undefined
  if (!only) return []
  const clave = only.clave
  if (clave === 'ndmi' || clave === 'nbr') {
    return [
      { from: Y_MIN, to: 0.10, fill: '#fef3c7', label: t('timeline.estado.seco') },
      { from: 0.10, to: 0.40, fill: '#ecfdf5', label: t('timeline.estado.normal') },
      { from: 0.40, to: Y_MAX, fill: '#dbeafe', label: t('timeline.estado.humedo') },
    ]
  }
  return [
    { from: Y_MIN, to: 0.30, fill: '#fee2e2', label: t('timeline.estado.bajo') },
    { from: 0.30, to: 0.55, fill: '#fef3c7', label: t('timeline.estado.moderado') },
    { from: 0.55, to: Y_MAX, fill: '#dcfce7', label: t('timeline.estado.vigoroso') },
  ]
})

// ── Trazado con cortes ────────────────────────────────────────────────────────

interface Dot { x: number; y: number; escenaId: number }
interface Seg { d: string; first: Dot; last: Dot }

/**
 * Parte la serie en tramos continuos de puntos confiables.
 *
 * Un día nublado no se interpola: unir la línea por encima de él dibujaría
 * una caída que el productor leería como estrés real del cultivo.
 */
function segmentsFor(clave: string): { solid: Seg[]; gaps: string[]; dots: Dot[] } {
  const solid: Seg[] = []
  const dots: Dot[] = []
  let cur: Dot[] = []

  const flush = () => {
    const first = cur[0]
    const last = cur[cur.length - 1]
    if (!first || !last) return
    const d = cur
      .map((pt, i) => `${i === 0 ? 'M' : 'L'}${pt.x.toFixed(1)},${pt.y.toFixed(1)}`)
      .join(' ')
    solid.push({ d, first, last })
    cur = []
  }

  for (const p of points.value) {
    const v = p.confiable ? p.valores?.[clave] : undefined
    if (v === undefined) {
      flush()
      continue
    }
    const dot: Dot = { x: xScale(p.fecha), y: yScale(v), escenaId: p.escena_id }
    cur.push(dot)
    dots.push(dot)
  }
  flush()

  const gaps: string[] = []
  for (let i = 1; i < solid.length; i++) {
    const a = solid[i - 1]
    const b = solid[i]
    if (!a || !b) continue
    gaps.push(
      `M${a.last.x.toFixed(1)},${a.last.y.toFixed(1)} L${b.first.x.toFixed(1)},${b.first.y.toFixed(1)}`,
    )
  }

  return { solid, gaps, dots }
}

const paths = computed(() =>
  activeSeries.value.map((s) => ({
    clave: s.clave,
    color: SERIE_COLORS[s.clave] ?? '#64748b',
    ...segmentsFor(s.clave),
  })),
)

const unreliable = computed(() => points.value.filter((p) => !p.confiable))

// ── Fases del cultivo ─────────────────────────────────────────────────────────

/**
 * Convierte cada fase (definida en días desde la plantación) a una franja
 * vertical de la gráfica, cuyo eje X está en fechas.
 *
 * Sin fecha de plantación no hay día 0 al que anclarlas, así que no se pintan.
 */
const faseBands = computed(() => {
  const plantacion = data.value?.produccion.fecha_plantacion
  const fases = data.value?.fases ?? []
  if (!plantacion || fases.length === 0) return []

  const day0 = parseLocalDate(plantacion).getTime()
  const { s, e, span } = xDomain.value
  if (span <= 0) return []

  const out: { x: number; w: number; color: string; nombre: string }[] = []
  for (const f of fases) {
    const desde = day0 + f.dia_inicio * 86400000
    const hasta = day0 + f.dia_fin * 86400000
    // Recortar a la ventana visible; descartar las que quedan fuera.
    const ini = Math.max(desde, s)
    const fin = Math.min(hasta, e)
    if (fin <= ini) continue

    const x = PAD.left + ((ini - s) / span) * plotW
    const w = ((fin - ini) / span) * plotW
    out.push({ x, w, color: f.color, nombre: f.nombre })
  }
  return out
})

// ── Eje X ─────────────────────────────────────────────────────────────────────

const dateFmt = computed(
  () => new Intl.DateTimeFormat(locale.value === 'en' ? 'en-US' : 'es-MX', { day: 'numeric', month: 'short' }),
)

function fmtDate(d: Date | string): string {
  return dateFmt.value.format(typeof d === 'string' ? parseLocalDate(d) : d)
}

/** Densidad de etiquetas según el rango, para no saturar el eje. */
const xTicks = computed(() => {
  const { s, e, span } = xDomain.value
  if (span <= 0) return []
  const days = spanDays.value
  const base = days <= 7 ? 1 : days <= 30 ? 7 : days <= 90 ? 14 : 30
  // Con el texto más grande en móvil, la mitad de etiquetas se solaparían.
  const step = isNarrow.value ? base * 2 : base

  const ticks: { x: number; label: string }[] = []
  for (let t = s; t <= e; t += step * 86400000) {
    const d = new Date(t)
    ticks.push({ x: PAD.left + ((t - s) / span) * plotW, label: fmtDate(d) })
  }
  return ticks
})

const rangeLabel = computed(() => {
  if (!viewStart.value || !viewEnd.value) return ''
  return `${fmtDate(viewStart.value)} – ${fmtDate(viewEnd.value)}`
})

// ── Cursor ────────────────────────────────────────────────────────────────────

const hovered = ref<TimelinePunto | null>(null)
const svgEl = ref<SVGSVGElement | null>(null)

// Al cambiar el rango, el punto bajo el cursor puede quedar fuera de la vista
// y el tooltip seguiría mostrando una fecha que ya no está en la gráfica.
watch([viewStart, viewEnd], () => { hovered.value = null })

function onMove(ev: MouseEvent) {
  const el = svgEl.value
  if (!el || points.value.length === 0) return

  // El SVG se escala con el contenedor, así que hay que llevar la coordenada
  // de pantalla al espacio del viewBox antes de comparar.
  const rect = el.getBoundingClientRect()
  const x = ((ev.clientX - rect.left) / rect.width) * W

  let best: TimelinePunto | null = null
  let bestDist = Infinity
  for (const p of points.value) {
    const d = Math.abs(xScale(p.fecha) - x)
    if (d < bestDist) {
      bestDist = d
      best = p
    }
  }
  hovered.value = best
}

const hoveredX = computed(() => (hovered.value ? xScale(hovered.value.fecha) : 0))

/** Voltea el tooltip al otro lado cuando el punto está cerca del borde derecho. */
const tooltipFlips = computed(() => hoveredX.value > W * 0.62)

function motivoText(p: TimelinePunto): string {
  return t(`timeline.motivo.${p.motivo_no_confiable ?? 'sin_params'}`)
}

function deltaText(p: TimelinePunto, clave: string): string | null {
  const d = p.delta?.[clave]
  if (d === undefined) return null
  return `${d > 0 ? '+' : ''}${d.toFixed(2)}`
}

// ── Rango personalizado ───────────────────────────────────────────────────────

const showCustom = ref(false)
const customFrom = ref('')
const customTo = ref('')

function applyCustom() {
  if (!customFrom.value || !customTo.value) return
  const from = parseLocalDate(customFrom.value)
  const to = parseLocalDate(customTo.value)
  if (from > to) return
  selectCustom(from, to)
  showCustom.value = false
}

const isoBounds = computed(() => ({
  min: bounds.value ? bounds.value.start.toISOString().slice(0, 10) : '',
  max: bounds.value ? bounds.value.end.toISOString().slice(0, 10) : '',
}))
</script>

<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <!-- Carga / error / vacíos -->
    <div v-if="loading" class="py-12 text-center text-sm text-gray-500">
      {{ t('timeline.cargando') }}
    </div>

    <div v-else-if="error" class="py-8 text-center">
      <p class="text-sm text-red-600">{{ error }}</p>
      <button class="mt-2 text-sm text-blue-600 hover:underline" @click="load()">
        {{ t('timeline.reintentar') }}
      </button>
    </div>

    <div v-else-if="allPoints.length === 0" class="py-12 text-center">
      <p class="text-sm font-medium text-gray-700">{{ t('timeline.vacio.titulo') }}</p>
      <p class="mt-1 text-xs text-gray-500">{{ t('timeline.vacio.detalle') }}</p>
    </div>

    <template v-else>
      <!-- Resumen -->
      <div class="mb-3 flex flex-wrap items-center gap-x-6 gap-y-2 text-xs">
        <span class="font-medium text-gray-900">{{ rangeLabel }}</span>
        <span class="text-gray-600">
          {{ t('timeline.resumen.confiables', {
            ok: data!.resumen.puntos_confiables,
            total: data!.resumen.puntos_totales,
          }) }}
        </span>
        <span class="text-gray-600">
          {{ t('timeline.resumen.tendencia') }}:
          <strong :class="{
            'text-green-700': data!.resumen.tendencia === 'mejorando',
            'text-red-700': data!.resumen.tendencia === 'declinando',
            'text-gray-700': data!.resumen.tendencia === 'estable',
          }">{{ t(`timeline.tendencia.${data!.resumen.tendencia}`) }}</strong>
        </span>
      </div>

      <!-- Controles de rango -->
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <button
          class="rounded border border-gray-300 px-2 py-1 text-xs disabled:opacity-40"
          :disabled="!canShiftBack" :title="t('timeline.rango.anterior')"
          @click="shift(-1)"
        >◀</button>

        <button
          v-for="p in (['semana', 'mes', 'todo'] as const)" :key="p"
          class="rounded px-2 py-1 text-xs border"
          :class="preset === p
            ? 'border-green-600 bg-green-50 font-medium text-green-800'
            : 'border-gray-300 text-gray-700 hover:bg-gray-50'"
          @click="selectPreset(p)"
        >{{ t(`timeline.rango.${p}`) }}</button>

        <button
          class="rounded px-2 py-1 text-xs border"
          :class="preset === 'custom'
            ? 'border-green-600 bg-green-50 font-medium text-green-800'
            : 'border-gray-300 text-gray-700 hover:bg-gray-50'"
          @click="showCustom = !showCustom"
        >{{ t('timeline.rango.personalizado') }}</button>

        <button
          class="rounded border border-gray-300 px-2 py-1 text-xs disabled:opacity-40"
          :disabled="!canShiftForward" :title="t('timeline.rango.siguiente')"
          @click="shift(1)"
        >▶</button>
      </div>

      <div v-if="showCustom" class="mb-3 flex flex-wrap items-end gap-2 rounded bg-gray-50 p-2">
        <label class="text-xs text-gray-600">
          {{ t('timeline.rango.desde') }}
          <input v-model="customFrom" type="date" :min="isoBounds.min" :max="isoBounds.max"
                 class="ml-1 rounded border border-gray-300 px-1 py-0.5 text-xs" />
        </label>
        <label class="text-xs text-gray-600">
          {{ t('timeline.rango.hasta') }}
          <input v-model="customTo" type="date" :min="isoBounds.min" :max="isoBounds.max"
                 class="ml-1 rounded border border-gray-300 px-1 py-0.5 text-xs" />
        </label>
        <button class="rounded bg-green-600 px-2 py-1 text-xs text-white hover:bg-green-700"
                @click="applyCustom">
          {{ t('timeline.rango.aplicar') }}
        </button>
      </div>

      <!-- Selector de series -->
      <div class="mb-2 flex flex-wrap gap-1.5">
        <button
          v-for="s in data!.series" :key="s.clave"
          class="rounded-full border px-2 py-0.5 text-xs"
          :class="active.has(s.clave)
            ? 'border-transparent text-white'
            : 'border-gray-300 bg-white text-gray-500 hover:bg-gray-50'"
          :style="active.has(s.clave) ? { backgroundColor: SERIE_COLORS[s.clave] } : {}"
          @click="toggleSerie(s.clave)"
        >{{ t(`timeline.indice.${s.clave}`) }}</button>
      </div>

      <!-- Gráfica -->
      <div class="relative">
        <svg
          ref="svgEl" :viewBox="`0 0 ${W} ${H}`" class="w-full select-none"
          style="height: auto" @mousemove="onMove" @mouseleave="hovered = null"
        >
          <!-- Bandas de estado (sólo con una serie activa) -->
          <rect
            v-for="b in bands" :key="b.label"
            :x="PAD.left" :y="yScale(b.to)"
            :width="plotW" :height="Math.max(0, yScale(b.from) - yScale(b.to))"
            :fill="b.fill"
          />
          <rect
            v-if="bands.length === 0"
            :x="PAD.left" :y="PAD.top" :width="plotW" :height="plotH" fill="#fafafa"
          />

          <!-- Fases del cultivo: franjas verticales bajo la rejilla, con su
               nombre arriba cuando hay sitio para leerlo -->
          <g v-if="faseBands.length > 0">
            <rect
              v-for="f in faseBands" :key="`fase-${f.nombre}-${f.x}`"
              :x="f.x" :y="PAD.top" :width="f.w" :height="plotH"
              :fill="f.color" opacity="0.55"
            />
            <text
              v-for="f in faseBands.filter((b) => b.w > 48)"
              :key="`faselbl-${f.nombre}-${f.x}`"
              :x="f.x + f.w / 2" :y="PAD.top + 11"
              text-anchor="middle" :font-size="tickFont" fill="#6b7280"
            >{{ f.nombre }}</text>
          </g>

          <!-- Rejilla y eje Y -->
          <g>
            <!-- Incluye -0.20 porque la humedad del suelo es negativa en suelo
                 desnudo: sin esa marca los primeros puntos flotan sin referencia -->
            <template v-for="v in [-0.2, 0, 0.25, 0.5, 0.75, 1.0]" :key="v">
              <line
                :x1="PAD.left" :y1="yScale(v)" :x2="W - PAD.right" :y2="yScale(v)"
                stroke="#e5e7eb" stroke-width="1"
              />
              <text :x="PAD.left - 5" :y="yScale(v) + 3" text-anchor="end"
                    :font-size="tickFont" fill="#9ca3af">{{ v.toFixed(2) }}</text>
            </template>
          </g>

          <!-- Eje X -->
          <g>
            <text
              v-for="(tk, i) in xTicks" :key="i"
              :x="tk.x" :y="H - 8" text-anchor="middle" :font-size="tickFont" fill="#9ca3af"
            >{{ tk.label }}</text>
          </g>

          <!-- Series -->
          <g v-for="p in paths" :key="p.clave" fill="none" :stroke="p.color">
            <path v-for="(g, i) in p.gaps" :key="`g${i}`" :d="g"
                  stroke-width="1.5" stroke-dasharray="3 3" opacity="0.4" />
            <path v-for="(s, i) in p.solid" :key="`s${i}`" :d="s.d"
                  stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
          </g>

          <!-- Puntos confiables -->
          <g v-for="p in paths" :key="`pts-${p.clave}`">
            <circle
              v-for="dot in p.dots" :key="`${p.clave}-${dot.escenaId}`"
              :cx="dot.x" :cy="dot.y" r="3"
              :fill="p.color" stroke="#fff" stroke-width="1"
              class="cursor-pointer" @click="emit('select', dot.escenaId)"
            />
          </g>

          <!-- Días sin dato confiable: hueco y a media altura, para que se note
               que hubo paso del satélite pero no medición utilizable -->
          <circle
            v-for="pt in unreliable" :key="`u-${pt.escena_id}`"
            :cx="xScale(pt.fecha)" :cy="yScale(0.4)" r="3.5"
            fill="#fff" stroke="#9ca3af" stroke-width="1.5" stroke-dasharray="2 1.5"
            class="cursor-pointer" @click="emit('select', pt.escena_id)"
          />

          <!-- Cursor -->
          <line
            v-if="hovered" :x1="hoveredX" :y1="PAD.top" :x2="hoveredX" :y2="H - PAD.bottom"
            stroke="#6b7280" stroke-width="1" stroke-dasharray="3 3"
          />
        </svg>

        <!-- Tooltip -->
        <div
          v-if="hovered"
          class="pointer-events-none absolute top-2 z-10 w-56 rounded-lg border border-gray-200 bg-white p-2 text-xs shadow-lg"
          :style="tooltipFlips ? { left: '2%' } : { right: '2%' }"
        >
          <p class="font-medium text-gray-900">{{ fmtDate(hovered.fecha) }}</p>
          <p class="mb-1 truncate font-mono text-[10px] text-gray-400">{{ hovered.scene_name }}</p>

          <template v-if="hovered.confiable">
            <div v-for="s in activeSeries" :key="s.clave" class="flex items-baseline justify-between gap-2">
              <span class="flex items-center gap-1 text-gray-600">
                <span class="inline-block h-2 w-2 rounded-full"
                      :style="{ backgroundColor: SERIE_COLORS[s.clave] }" />
                {{ t(`timeline.indice.${s.clave}`) }}
              </span>
              <span class="tabular-nums text-gray-900">
                {{ hovered.valores?.[s.clave]?.toFixed(2) ?? '—' }}
                <span v-if="deltaText(hovered, s.clave)"
                      :class="(hovered.delta?.[s.clave] ?? 0) >= 0 ? 'text-green-600' : 'text-red-600'"
                >({{ deltaText(hovered, s.clave) }})</span>
              </span>
            </div>
            <p v-if="hovered.estado?.ndvi && active.has('ndvi')" class="mt-1 text-gray-600">
              {{ t('timeline.indice.ndvi') }}:
              <strong>{{ t(`timeline.estado.${hovered.estado.ndvi}`) }}</strong>
            </p>
          </template>

          <p v-else class="rounded bg-gray-50 px-1.5 py-1 text-gray-600">
            {{ motivoText(hovered) }}
          </p>

          <div class="mt-1.5 space-y-0.5 border-t border-gray-100 pt-1.5 text-[11px] text-gray-500">
            <p v-if="hovered.nubosidad !== null">
              {{ t('timeline.nubosidad') }}: {{ hovered.nubosidad?.toFixed(0) }}%
            </p>
            <p v-if="hovered.dia_cultivo !== null">
              {{ t('timeline.diaCultivo') }}: {{ hovered.dia_cultivo }}
            </p>
            <p v-if="hovered.dias_a_cosecha !== null && hovered.dias_a_cosecha >= 0">
              {{ t('timeline.diasACosecha') }}: {{ hovered.dias_a_cosecha }}
            </p>
            <p v-if="hovered.ia">
              {{ t('timeline.ia') }}: {{ hovered.ia.estado }} · {{ hovered.ia.riesgo }}
            </p>
          </div>
        </div>
      </div>

      <!-- Leyenda de puntos huecos -->
      <p v-if="unreliable.length > 0" class="mt-2 flex items-center gap-1.5 text-[11px] text-gray-500">
        <svg width="10" height="10"><circle cx="5" cy="5" r="3.5" fill="#fff" stroke="#9ca3af"
          stroke-width="1.5" stroke-dasharray="2 1.5" /></svg>
        {{ t('timeline.leyendaNoConfiable', { n: unreliable.length }) }}
      </p>

      <p v-if="points.length === 1" class="mt-2 text-[11px] text-gray-500">
        {{ t('timeline.unSoloPunto') }}
      </p>
    </template>
  </div>
</template>
