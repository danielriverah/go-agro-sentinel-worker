<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { produccionesExtra, worker as workerApi } from '@/api/client'
import PipelineStatus from '@/components/PipelineStatus.vue'
import { useNavContext, scrollToElement } from '@/composables/useNavContext'
import { useSyncStore } from '@/stores/sync'
import { useWorkerStore } from '@/stores/worker'
import { useProductionsStore } from '@/stores/productions'
import { usePermissionsStore } from '@/stores/permissions'
import type { Production } from '@/api/types'

const { t, locale } = useI18n()
const router = useRouter()
const syncStore = useSyncStore()
const workerStore = useWorkerStore()
const prodStore = useProductionsStore()
const permStore = usePermissionsStore()

const productions = computed(() => prodStore.productions)
const loading = computed(() => prodStore.loading)

// Per-card loading sets — IDs en vuelo, para deshabilitar botones individualmente.
const togglingIA     = ref(new Set<number>())
const triggeringProd = ref(new Set<number>())
const triggerErrors  = ref(new Map<number, string>())

const runningAll     = ref(false)
const runAllMsg      = ref('')
const cancellingAll  = ref(false)
const cancelAllMsg   = ref('')
const cancellingCard = ref(false)
const canRunSync = computed(() => permStore.puede('sync.ejecutar'))
const canControlWorker = computed(() => permStore.puede('worker.controlar'))

onMounted(() => {
  workerStore.refresh()
  syncStore.refresh()
  nowTimer = setInterval(() => {
    now.value = Date.now()
    checkCountdownFire()
  }, 1000)
})
onUnmounted(() => { if (nowTimer) clearInterval(nowTimer) })

// ── Agrupación y orden ───────────────────────────────────────────────────────
const SIN_RANCHO  = '— Sin rancho —'
const SIN_ARTICULO = '— Sin artículo —'

// Menor = más urgente. Define el orden de atención acordado.
function attentionRank(p: Production): number {
  if (p.PosibleCosecha) return 0
  if (p.Bloqueado) return 1
  if ((p.stats?.scenes_pendientes ?? 0) > 0) return 2
  if (!p.Monitoring) return 3
  return 4
}

interface AttentionMeta { icon: string; label: string; cls: string }

const AL_DIA: AttentionMeta = { icon: '✓', label: 'Al día', cls: 'bg-green-100 text-green-700' }

const ATTENTION_META: AttentionMeta[] = [
  { icon: '🌾', label: 'Posible cosecha', cls: 'bg-yellow-100 text-yellow-700' },
  { icon: '⛔', label: 'Bloqueada',       cls: 'bg-red-100 text-red-700' },
  { icon: '⏳', label: 'Pendientes',      cls: 'bg-orange-100 text-orange-700' },
  { icon: '⚠',  label: 'Sin monitoreo',   cls: 'bg-gray-100 text-gray-500' },
  AL_DIA,
]

function meta(rank: number): AttentionMeta {
  return ATTENTION_META[rank] ?? AL_DIA
}

function plantTime(p: Production): number {
  return p.FechaPlantacion ? new Date(p.FechaPlantacion).getTime() : -Infinity
}

// Atención primero; dentro del mismo nivel, de la más nueva a la más antigua.
function byAttentionThenDate(a: Production, b: Production): number {
  const ra = attentionRank(a)
  const rb = attentionRank(b)
  if (ra !== rb) return ra - rb
  return plantTime(b) - plantTime(a)
}

const search = ref('')

// ── Colapso en ambos niveles ─────────────────────────────────────────────────
// Un artículo se identifica con "rancho|articulo" porque el mismo artículo
// puede aparecer bajo varios ranchos.
const collapsedRanchos  = ref(new Set<string>())
const collapsedArticulos = ref(new Set<string>())

function articuloId(rancho: string, articulo: string) {
  return `${rancho}|${articulo}`
}

function toggleRancho(rancho: string) {
  const next = new Set(collapsedRanchos.value)
  if (next.has(rancho)) next.delete(rancho)
  else next.add(rancho)
  collapsedRanchos.value = next
}

function toggleArticulo(rancho: string, articulo: string) {
  const id = articuloId(rancho, articulo)
  const next = new Set(collapsedArticulos.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsedArticulos.value = next
}

const allCollapsed = computed(() =>
  grouped.value.length > 0 && grouped.value.every(g => collapsedRanchos.value.has(g.rancho))
)

function toggleAll() {
  if (allCollapsed.value) {
    collapsedRanchos.value = new Set()
    collapsedArticulos.value = new Set()
  } else {
    collapsedRanchos.value = new Set(grouped.value.map(g => g.rancho))
  }
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return productions.value
  return productions.value.filter(p =>
    (p.Folio ?? '').toLowerCase().includes(q) ||
    (p.Rancho ?? '').toLowerCase().includes(q) ||
    (p.Cosecha ?? '').toLowerCase().includes(q)
  )
})

interface Tally {
  count: number
  pendientes: number
  completadas: number
  totalEscenas: number
}
interface ArticuloGroup extends Tally {
  articulo: string
  prods: Production[]
  rank: number
  newest: number
}
interface RanchoGroup extends Tally {
  rancho: string
  articulos: ArticuloGroup[]
  rank: number
  newest: number
}

function tally(prods: Production[]): Tally {
  const t: Tally = { count: prods.length, pendientes: 0, completadas: 0, totalEscenas: 0 }
  for (const p of prods) {
    t.pendientes   += p.stats?.scenes_pendientes ?? 0
    t.completadas  += p.stats?.scenes_completadas ?? 0
    t.totalEscenas += p.stats?.scenes_total ?? 0
  }
  return t
}

const grouped = computed<RanchoGroup[]>(() => {
  const tree = new Map<string, Map<string, Production[]>>()
  for (const p of filtered.value) {
    const rancho = p.Rancho?.trim() || SIN_RANCHO
    const articulo = p.Cosecha?.trim() || SIN_ARTICULO
    let branch = tree.get(rancho)
    if (!branch) { branch = new Map(); tree.set(rancho, branch) }
    const list = branch.get(articulo)
    if (list) list.push(p)
    else branch.set(articulo, [p])
  }

  const groups: RanchoGroup[] = []
  for (const [rancho, branch] of tree) {
    const articulos: ArticuloGroup[] = []
    const total: Tally = { count: 0, pendientes: 0, completadas: 0, totalEscenas: 0 }

    for (const [articulo, prods] of branch) {
      prods.sort(byAttentionThenDate)
      const t = tally(prods)
      articulos.push({
        articulo,
        prods,
        rank: Math.min(...prods.map(attentionRank)),
        newest: Math.max(...prods.map(plantTime)),
        ...t,
      })
      total.count        += t.count
      total.pendientes   += t.pendientes
      total.completadas  += t.completadas
      total.totalEscenas += t.totalEscenas
    }

    articulos.sort((a, b) => a.rank - b.rank || b.newest - a.newest)
    groups.push({
      rancho,
      articulos,
      rank: Math.min(...articulos.map(a => a.rank)),
      newest: Math.max(...articulos.map(a => a.newest)),
      ...total,
    })
  }

  groups.sort((a, b) => a.rank - b.rank || b.newest - a.newest)
  return groups
})

// Orden aplanado tal como se ve en pantalla — alimenta la navegación prev/next.
const flatOrder = computed(() =>
  grouped.value.flatMap(g => g.articulos.flatMap(a => a.prods.map(p => p.ID)))
)

function progressPct(t: Tally): number | null {
  if (t.totalEscenas === 0) return null
  return Math.round((t.completadas / t.totalEscenas) * 100)
}

// ── Acciones ─────────────────────────────────────────────────────────────────
const { productionIds: navProductionIds, cameFromProduccion } = useNavContext()

function openProduccion(prod: Production) {
  navProductionIds.value = flatOrder.value
  router.push({ name: 'produccion', params: { id: prod.ID } })
}

// ── Volver al punto donde estaba ─────────────────────────────────────────────
// Al regresar desde cualquier parte dentro de una producción, la vista se
// posiciona en ella y la resalta, en lugar de saltar al inicio del listado.
const highlightedId = ref<number | null>(null)
let revealed = false

async function revealLastVisited() {
  const id = cameFromProduccion.value
  if (revealed || !id) return

  const prod = productions.value.find(p => p.ID === id)
  if (!prod) return // aún no llegan los datos
  revealed = true

  // Su grupo puede estar colapsado — abrirlo o el elemento no existe en el DOM.
  const rancho = prod.Rancho?.trim() || SIN_RANCHO
  const articulo = prod.Cosecha?.trim() || SIN_ARTICULO
  if (collapsedRanchos.value.has(rancho)) {
    const next = new Set(collapsedRanchos.value)
    next.delete(rancho)
    collapsedRanchos.value = next
  }
  const artId = articuloId(rancho, articulo)
  if (collapsedArticulos.value.has(artId)) {
    const next = new Set(collapsedArticulos.value)
    next.delete(artId)
    collapsedArticulos.value = next
  }

  highlightedId.value = id
  await nextTick()
  await scrollToElement(`prod-${id}`)
  setTimeout(() => {
    if (highlightedId.value === id) highlightedId.value = null
  }, 3000)
}

// Las producciones vienen del store y pueden llegar después del montaje.
watch(productions, revealLastVisited, { immediate: true })

async function ponerAlCorriente(prod: Production, e: MouseEvent) {
  e.stopPropagation()
  if (!permStore.puede('monitoreo.worker', prod.CentroCostoID)) return
  if (triggeringProd.value.has(prod.ID)) return
  triggeringProd.value.add(prod.ID)
  triggerErrors.value.delete(prod.ID)
  try {
    await workerApi.runProduction(prod.ID)
    workerStore.markTriggered(prod.ProduccionID)
  } catch (err) {
    const status = (err as any)?.response?.status
    const msg: string = (err as any)?.response?.data?.error ?? ''
    if (status === 409) {
      triggerErrors.value.set(prod.ID, msg.includes('otra') ? 'Otra producción en proceso' : t('production.already_processing'))
    } else {
      triggerErrors.value.set(prod.ID, t('common.error'))
    }
    setTimeout(() => triggerErrors.value.delete(prod.ID), 4000)
  } finally {
    triggeringProd.value.delete(prod.ID)
  }
}

async function toggleIAAuto(prod: Production, e: MouseEvent) {
  e.stopPropagation()
  if (!permStore.puede('producciones.editar', prod.CentroCostoID)) return
  if (togglingIA.value.has(prod.ID)) return
  togglingIA.value.add(prod.ID)
  try {
    const updated = await produccionesExtra.patchIAAuto(prod.ID, !prod.IAuto)
    prodStore.patchProduction(prod.ID, updated)
  } catch { /* silencioso */ } finally {
    togglingIA.value.delete(prod.ID)
  }
}

async function triggerRunAll() {
  if (!canControlWorker.value || runningAll.value || workerStore.blockIndividualTrigger) return
  runningAll.value = true
  runAllMsg.value = ''
  try {
    await workerApi.runAll()
    runAllMsg.value = 'ok'
    workerStore.markGlobalTriggered()
  } catch (err: any) {
    runAllMsg.value = err?.response?.status === 409 ? 'already_running' : 'error'
  } finally {
    runningAll.value = false
    setTimeout(() => { runAllMsg.value = '' }, 4000)
  }
}

async function cancelRunAll() {
  if (!canControlWorker.value) return
  if (cancellingAll.value) return
  if (workerStore.stopPending) {
    try { await workerApi.cancel(undefined, true) } catch { /* silencioso */ }
    return
  }
  cancellingAll.value = true
  cancelAllMsg.value = ''
  try {
    await workerApi.cancel()
    cancelAllMsg.value = 'stop_requested'
  } catch {
    cancelAllMsg.value = 'error'
    cancellingAll.value = false
    setTimeout(() => { cancelAllMsg.value = '' }, 5000)
  }
}

async function cancelFromCard(e: MouseEvent) {
  e.stopPropagation()
  if (!canControlWorker.value) return
  if (cancellingCard.value) return
  if (workerStore.stopPending) {
    try { await workerApi.cancel(undefined, true) } catch { /* silencioso */ }
    return
  }
  cancellingCard.value = true
  try { await workerApi.cancel() } catch { /* silencioso */ } finally {
    cancellingCard.value = false
  }
}

watch(
  () => workerStore.status.global?.phase,
  (phase, prev) => {
    if (prev === 'processing' && phase !== 'processing' && cancellingAll.value) {
      cancellingAll.value = false
      cancelAllMsg.value = 'stopped'
      setTimeout(() => { cancelAllMsg.value = '' }, 3000)
    }
  }
)

// ── Cuenta regresiva ─────────────────────────────────────────────────────────
const now = ref(Date.now())
let nowTimer: ReturnType<typeof setInterval> | null = null
let workerOptimisticFired = false

// Al llegar a 0 activamos el estado optimista local, igual que si se hubiera
// pulsado el botón. El SSE confirma el lock real en ≤2s.
function checkCountdownFire() {
  const workerNext = workerStore.status.global?.next_schedule_at
  if (workerNext) {
    const s = rawSecsUntil(workerNext)
    if (s !== null && s <= 0 && !workerOptimisticFired && !workerStore.isGlobalProcessing) {
      workerOptimisticFired = true
      workerStore.markGlobalTriggered()
    } else if (s !== null && s > 5) {
      workerOptimisticFired = false
    }
  }
}

function rawSecsUntil(iso: string): number | null {
  const secs = Math.round((new Date(iso).getTime() - now.value) / 1000)
  return secs <= 60 ? secs : null
}

function secsUntil(iso: string | null | undefined): number | null {
  if (!iso) return null
  let d = new Date(iso)
  const DAY = 24 * 60 * 60 * 1000
  while (d.getTime() <= now.value) d = new Date(d.getTime() + DAY)
  const secs = Math.round((d.getTime() - now.value) / 1000)
  return secs <= 60 ? secs : null
}

function countdown(iso: string | null | undefined): string {
  const s = secsUntil(iso)
  if (s === null) return ''
  return s <= 0 ? (locale.value === 'es' ? 'ahora' : 'now') : (locale.value === 'es' ? `en ${s}s` : `in ${s}s`)
}

function formatIn(iso: string | null | undefined): string {
  if (!iso) return '—'
  let d = new Date(iso)
  const DAY = 24 * 60 * 60 * 1000
  while (d.getTime() <= Date.now()) d = new Date(d.getTime() + DAY)
  const timeStr = d.toLocaleTimeString(locale.value === 'es' ? 'es-MX' : 'en-US', { hour: '2-digit', minute: '2-digit' })
  const diffM = Math.floor((d.getTime() - Date.now()) / 60_000)
  if (diffM < 60) return locale.value === 'es' ? `en ${diffM}m` : `in ${diffM}m`
  const diffH = Math.floor(diffM / 60)
  if (diffH < 24) return locale.value === 'es' ? `en ${diffH}h` : `in ${diffH}h`
  const diffD = Math.floor(diffH / 24)
  if (diffD === 1) return locale.value === 'es' ? `mañana ${timeStr}` : `tomorrow ${timeStr}`
  return `${d.toLocaleDateString(locale.value === 'es' ? 'es-MX' : 'en-US', { month: 'short', day: 'numeric' })} ${timeStr}`
}

// ── Helpers de presentación ──────────────────────────────────────────────────
function formatAgo(iso: string | null | undefined): string {
  if (!iso) return t('common.never')
  const diffM = Math.floor((Date.now() - new Date(iso).getTime()) / 60_000)
  if (diffM < 1)  return locale.value === 'es' ? 'ahora mismo' : 'just now'
  if (diffM < 60) return locale.value === 'es' ? `hace ${diffM}m` : `${diffM}m ago`
  const diffH = Math.floor(diffM / 60)
  if (diffH < 24) return locale.value === 'es' ? `hace ${diffH}h` : `${diffH}h ago`
  const diffD = Math.floor(diffH / 24)
  if (diffD < 7)  return locale.value === 'es' ? `hace ${diffD}d` : `${diffD}d ago`
  return new Date(iso).toLocaleDateString(locale.value === 'es' ? 'es-MX' : 'en-US', { month: 'short', day: 'numeric' })
}

function formatPlant(iso: string | null): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString(locale.value === 'es' ? 'es-MX' : 'en-US', {
    year: 'numeric', month: 'short', day: 'numeric',
  })
}

// Toda escena completada genera TIF, nublada o no, así que se compara contra
// el total. Antes se comparaba con las usables —un subconjunto— y por eso
// salía ✓ aunque faltaran escenas por procesar.
function tifStatus(prod: Production): 'ok' | 'pending' | 'never' {
  const s = prod.stats
  if (!s || s.scenes_total === 0) return 'never'
  if (s.last_tif_fecha === null) return 'never'
  return s.scenes_tif_ok >= s.scenes_total ? 'ok' : 'pending'
}

// La IA solo corre sobre escenas usables. Comparándola contra los TIF nunca
// llegaba a ✓ mientras hubiera escenas nubladas.
function iaStatus(prod: Production): 'ok' | 'pending' | 'never' | 'disabled' {
  if (!prod.IAuto) return 'disabled'
  const s = prod.stats
  if (!s || s.scenes_ia_ok === 0) return 'never'
  return s.scenes_ia_ok >= s.scenes_usables ? 'ok' : 'pending'
}

const statusIcon = { ok: '✓', pending: '⚠', never: '–', disabled: '○' }
const statusColor = {
  ok:       'text-green-600',
  pending:  'text-yellow-600',
  never:    'text-gray-400',
  disabled: 'text-gray-300',
}

const anyProductionProcessing = computed(() => workerStore.anyProductionProcessing)
const blockIndividualTrigger = computed(() => workerStore.blockIndividualTrigger)

function workerStatusFor(produccionId: number) {
  const s = workerStore.status
  if (s.global?.phase === 'processing' && s.global.current_produccion_id === produccionId) {
    return s.global
  }
  return (s.productions ?? []).find(
    p => p.phase === 'processing' && p.current_produccion_id === produccionId
  ) ?? null
}

function workerProgressPct(ws: { scenes_done: number; scenes_total: number } | null) {
  if (!ws || ws.scenes_total === 0) return null
  return Math.round((ws.scenes_done / ws.scenes_total) * 100)
}
</script>

<template>
  <div class="max-w-7xl mx-auto px-4 py-6 space-y-5">

    <!-- Header -->
    <div class="flex items-center justify-between gap-4 flex-wrap">
      <div>
        <h1 class="text-xl font-bold text-gray-900">Producciones</h1>
        <p class="text-xs text-gray-400 mt-0.5">
          {{ filtered.length }} de {{ productions.length }} · agrupadas por rancho y artículo
        </p>
      </div>
      <div class="flex items-center gap-2 w-full sm:w-auto">
        <button
          v-if="grouped.length > 0"
          @click="toggleAll"
          class="text-xs px-2.5 py-2 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 transition-colors whitespace-nowrap shrink-0"
        >
          {{ allCollapsed ? '▸ Expandir todo' : '▾ Colapsar todo' }}
        </button>
        <input
          v-model="search"
          type="search"
          placeholder="Buscar folio, rancho o artículo..."
          class="text-sm px-3 py-2 rounded-lg border border-gray-200 bg-white flex-1 sm:w-72 focus:outline-none focus:ring-2 focus:ring-green-200 focus:border-green-300"
        />
      </div>
    </div>

    <!-- Leyenda de prioridad -->
    <div class="flex items-center gap-2 flex-wrap text-xs text-gray-400">
      <span>Orden:</span>
      <span v-for="(m, i) in ATTENTION_META" :key="m.label" class="flex items-center gap-1">
        <span class="px-1.5 py-0.5 rounded" :class="m.cls">{{ m.icon }} {{ m.label }}</span>
        <span v-if="i < ATTENTION_META.length - 1" class="text-gray-300">›</span>
      </span>
      <span class="text-gray-300">· luego más reciente primero</span>
    </div>

    <!-- Estado del pipeline: los dos eslabones y su lectura conjunta -->
    <PipelineStatus />

    <!-- ── Operación: sync + worker ─────────────────────────────────────────── -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">

      <!-- Sync -->
      <div class="bg-white border border-gray-200 rounded-2xl p-4 space-y-3">
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded bg-blue-100 text-blue-600 leading-none">SYNC</span>
          <h2 class="text-sm font-semibold text-gray-800">Sincronización</h2>
          <div v-if="syncStore.status?.running" class="ml-auto flex items-center gap-1.5 text-xs text-blue-600 font-medium">
            <svg class="animate-spin h-3.5 w-3.5 shrink-0" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
            </svg>
            Sincronizando
          </div>
          <span v-else class="ml-auto w-2 h-2 rounded-full bg-gray-300" />
        </div>

        <div class="grid grid-cols-2 gap-3 text-xs">
          <div>
            <p class="text-gray-400">Último</p>
            <p class="text-gray-800 font-medium mt-0.5">{{ formatAgo(syncStore.status?.last_run) }}</p>
          </div>
          <div>
            <p class="text-gray-400">Próximo</p>
            <p class="mt-0.5">
              <span v-if="secsUntil(syncStore.status?.next_run) !== null"
                    class="font-semibold tabular-nums animate-pulse text-amber-600">
                {{ countdown(syncStore.status?.next_run) }}
              </span>
              <span v-else class="text-gray-800 font-medium">{{ formatIn(syncStore.status?.next_run) }}</span>
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            @click="syncStore.trigger()"
            :disabled="!canRunSync || syncStore.triggering || syncStore.status?.running"
            :title="!canRunSync ? t('permisos.sinPermiso') : undefined"
            class="text-xs px-3 py-1.5 rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-60 disabled:cursor-not-allowed transition-colors flex items-center gap-1.5"
          >
            <svg v-if="syncStore.triggering" class="animate-spin h-3 w-3" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
            </svg>
            {{ syncStore.triggering ? '...' : t('sync.trigger') }}
          </button>
          <span v-if="syncStore.triggerMsg === 'ok'" class="text-xs text-green-600 font-medium">✓ {{ t('sync.triggered') }}</span>
          <span v-else-if="syncStore.triggerMsg === 'already_running'" class="text-xs text-blue-500">Ya corriendo</span>
          <span v-else-if="syncStore.triggerMsg === 'error'" class="text-xs text-red-500">Error</span>
        </div>
      </div>

      <!-- Worker -->
      <div class="bg-white border rounded-2xl p-4 space-y-3" :class="workerStore.isGlobalProcessing ? 'border-green-300 bg-green-50/30' : 'border-gray-200'">
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded bg-green-100 text-green-700 leading-none">WORKER</span>
          <h2 class="text-sm font-semibold text-gray-800">Procesamiento</h2>
          <div v-if="workerStore.isGlobalProcessing" class="ml-auto flex items-center gap-1.5 text-xs text-green-600 font-medium">
            <span class="w-2 h-2 rounded-full bg-green-500 animate-pulse shrink-0" />
            Procesando
          </div>
          <div v-else-if="anyProductionProcessing" class="ml-auto flex items-center gap-1.5 text-xs text-green-600 font-medium">
            <span class="w-2 h-2 rounded-full bg-green-500 animate-pulse shrink-0" />
            Producción individual
          </div>
          <span v-else class="ml-auto w-2 h-2 rounded-full bg-gray-300" />
        </div>

        <!-- Progreso en vivo -->
        <div v-if="workerStore.isGlobalProcessing && workerStore.status.global" class="space-y-1.5">
          <div class="flex items-center justify-between text-xs text-green-700">
            <span class="tabular-nums">
              {{ workerStore.status.global.scenes_done }}<template v-if="workerStore.status.global.scenes_total"> / {{ workerStore.status.global.scenes_total }}</template>
              <span class="text-green-600/70 ml-0.5">escenas</span>
            </span>
            <span v-if="workerStore.status.global.scenes_total" class="tabular-nums font-medium">
              {{ Math.round((workerStore.status.global.scenes_done / workerStore.status.global.scenes_total) * 100) }}%
            </span>
          </div>
          <div class="w-full bg-green-100 rounded-full h-2 overflow-hidden">
            <div
              v-if="workerStore.status.global.scenes_total"
              class="bg-green-500 h-2 rounded-full transition-all duration-500"
              :style="{ width: Math.round((workerStore.status.global.scenes_done / workerStore.status.global.scenes_total) * 100) + '%' }"
            />
            <div v-else class="bg-green-400 h-2 rounded-full animate-pulse w-1/2" />
          </div>
          <p v-if="workerStore.status.global.current_scene" class="text-xs text-green-600 font-mono truncate">
            {{ workerStore.status.global.current_scene }}
          </p>
          <p v-if="workerStore.status.global.scenes_failed > 0" class="text-xs text-red-500 tabular-nums">
            {{ workerStore.status.global.scenes_failed }} fallida{{ workerStore.status.global.scenes_failed !== 1 ? 's' : '' }}
          </p>
        </div>

        <!-- Horarios -->
        <div v-else class="grid grid-cols-2 gap-3 text-xs">
          <div>
            <p class="text-gray-400">Último ciclo</p>
            <p class="mt-0.5" :class="workerStore.status.global?.last_completed_at ? 'text-gray-800 font-medium' : 'text-gray-400 italic'">
              {{ workerStore.status.global?.last_completed_at ? formatAgo(workerStore.status.global.last_completed_at) : 'Sin registros' }}
            </p>
          </div>
          <div>
            <p class="text-gray-400">Programado</p>
            <p class="mt-0.5">
              <template v-if="workerStore.status.global?.next_schedule_at">
                <span v-if="secsUntil(workerStore.status.global.next_schedule_at) !== null"
                      class="font-semibold tabular-nums animate-pulse text-amber-600">
                  {{ countdown(workerStore.status.global.next_schedule_at) }}
                </span>
                <span v-else class="text-gray-800 font-medium">{{ formatIn(workerStore.status.global.next_schedule_at) }}</span>
              </template>
              <span v-else class="text-gray-400 italic">—</span>
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            v-if="workerStore.isGlobalProcessing"
            @click="cancelRunAll"
            :disabled="!canControlWorker || cancellingAll"
            :title="!canControlWorker ? t('permisos.sinPermiso') : undefined"
            class="text-xs px-3 py-1.5 rounded-lg border transition-colors flex items-center gap-1.5 disabled:opacity-60"
            :class="workerStore.stopPending
              ? 'border-orange-200 text-orange-600 hover:bg-orange-50'
              : 'border-red-200 text-red-600 hover:bg-red-50'"
          >
            {{ cancellingAll ? 'Deteniendo...' : workerStore.stopPending ? '↩ Cancelar detención' : '■ Detener' }}
          </button>
          <button
            v-else
            @click="triggerRunAll"
            :disabled="!canControlWorker || runningAll || blockIndividualTrigger"
            :title="!canControlWorker ? t('permisos.sinPermiso') : anyProductionProcessing ? 'Una producción está en proceso' : undefined"
            class="text-xs px-3 py-1.5 rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-60 disabled:cursor-not-allowed transition-colors flex items-center gap-1.5"
          >
            <svg v-if="runningAll" class="animate-spin h-3 w-3" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
            </svg>
            {{ runningAll ? '...' : 'Procesar todo' }}
          </button>
          <span v-if="cancelAllMsg === 'stop_requested'" class="text-xs text-orange-500">Detención solicitada</span>
          <span v-else-if="cancelAllMsg === 'stopped'" class="text-xs text-gray-500">✓ Detenido</span>
          <span v-else-if="runAllMsg === 'ok'" class="text-xs text-green-600 font-medium">✓ En cola</span>
          <span v-else-if="runAllMsg === 'already_running'" class="text-xs text-blue-500">Ya corriendo</span>
          <span v-else-if="runAllMsg === 'error'" class="text-xs text-red-500">Error</span>
        </div>
      </div>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
      <div v-for="i in 6" :key="i" class="bg-white border border-gray-200 rounded-2xl p-4 h-40 animate-pulse">
        <div class="h-4 bg-gray-100 rounded w-1/2 mb-3"/>
        <div class="h-3 bg-gray-100 rounded w-3/4 mb-2"/>
        <div class="h-3 bg-gray-100 rounded w-1/2"/>
      </div>
    </div>

    <!-- Vacío -->
    <div v-else-if="grouped.length === 0" class="text-center py-16 text-gray-400">
      <p class="text-4xl mb-3">🌱</p>
      <p class="text-sm">{{ search ? 'Sin resultados para la búsqueda' : 'No hay producciones registradas' }}</p>
    </div>

    <!-- Árbol: Rancho → Artículo → tarjetas -->
    <div v-else class="space-y-4">
      <section
        v-for="g in grouped"
        :key="g.rancho"
        class="bg-white border border-gray-200 rounded-2xl overflow-hidden"
      >
        <!-- Encabezado de rancho -->
        <button
          @click="toggleRancho(g.rancho)"
          class="w-full flex items-center gap-3 px-4 py-3 hover:bg-gray-50 transition-colors text-left"
        >
          <span class="text-gray-300 text-sm shrink-0 transition-transform" :class="collapsedRanchos.has(g.rancho) ? '' : 'rotate-90'">▶</span>
          <span class="text-base shrink-0">🏡</span>
          <span class="font-semibold text-gray-900 truncate">{{ g.rancho }}</span>
          <span
            class="text-xs px-1.5 py-0.5 rounded shrink-0"
            :class="meta(g.rank).cls"
            :title="meta(g.rank).label"
          >{{ meta(g.rank).icon }}</span>

          <div class="ml-auto flex items-center gap-3 shrink-0 text-xs">
            <span class="text-gray-400">{{ g.count }} prod.</span>
            <span v-if="g.pendientes > 0" class="text-orange-600 tabular-nums">{{ g.pendientes }} pend.</span>
            <template v-if="progressPct(g) !== null">
              <div class="w-20 bg-gray-100 rounded-full h-1.5 overflow-hidden">
                <div class="bg-green-500 h-1.5 rounded-full transition-all" :style="{ width: progressPct(g) + '%' }" />
              </div>
              <span class="text-gray-400 tabular-nums w-9 text-right">{{ progressPct(g) }}%</span>
            </template>
          </div>
        </button>

        <!-- Contenido del rancho -->
        <div v-if="!collapsedRanchos.has(g.rancho)" class="border-t border-gray-100">
          <div v-for="a in g.articulos" :key="a.articulo" class="px-4 py-3">

            <!-- Encabezado de artículo (colapsable) -->
            <button
              @click="toggleArticulo(g.rancho, a.articulo)"
              class="w-full flex items-center gap-2 mb-2.5 text-left hover:opacity-70 transition-opacity"
            >
              <span
                class="text-gray-300 text-xs shrink-0 transition-transform"
                :class="collapsedArticulos.has(articuloId(g.rancho, a.articulo)) ? '' : 'rotate-90'"
              >▶</span>
              <span class="text-sm shrink-0">🌱</span>
              <h3 class="text-sm font-medium text-gray-700 truncate">{{ a.articulo }}</h3>
              <span class="text-xs text-gray-400 shrink-0">({{ a.count }})</span>
              <span
                class="text-xs px-1.5 py-0.5 rounded shrink-0"
                :class="meta(a.rank).cls"
                :title="meta(a.rank).label"
              >{{ meta(a.rank).icon }}</span>
              <span v-if="a.pendientes > 0" class="text-xs text-orange-600 tabular-nums shrink-0">{{ a.pendientes }} pend.</span>
              <div class="flex-1 border-t border-gray-100 ml-1" />
              <template v-if="progressPct(a) !== null">
                <div class="w-16 bg-gray-100 rounded-full h-1 overflow-hidden shrink-0">
                  <div class="bg-green-500 h-1 rounded-full transition-all" :style="{ width: progressPct(a) + '%' }" />
                </div>
                <span class="text-xs text-gray-400 tabular-nums w-8 text-right shrink-0">{{ progressPct(a) }}%</span>
              </template>
            </button>

            <!-- Tarjetas -->
            <div v-if="!collapsedArticulos.has(articuloId(g.rancho, a.articulo))" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              <div
                v-for="prod in a.prods"
                :key="prod.ID"
                :data-nav-id="`prod-${prod.ID}`"
                @click="openProduccion(prod)"
                class="bg-white rounded-xl p-3 hover:shadow-md cursor-pointer transition-all group select-none border"
                :class="highlightedId === prod.ID
                  ? 'border-green-500 ring-2 ring-green-300 shadow-md'
                  : 'border-gray-200 hover:border-green-300'"
              >
                <p v-if="highlightedId === prod.ID" class="text-xs text-green-600 font-medium mb-1.5">
                  ← Vienes de aquí
                </p>
                <!-- Folio + badges -->
                <div class="flex items-start justify-between gap-2 mb-2">
                  <div class="min-w-0">
                    <p class="font-bold text-gray-900 group-hover:text-green-700 transition-colors truncate text-sm">{{ prod.Folio || '—' }}</p>
                    <p class="text-xs text-gray-400 mt-0.5">🗓 {{ formatPlant(prod.FechaPlantacion) }}</p>
                  </div>
                  <div class="flex flex-wrap gap-1 justify-end shrink-0">
                    <span v-if="prod.PosibleCosecha" class="text-xs px-1.5 py-0.5 rounded-full bg-yellow-100 text-yellow-700" title="Posible cosecha">🌾</span>
                    <span v-if="prod.Bloqueado" class="text-xs px-1.5 py-0.5 rounded-full bg-red-100 text-red-700">{{ t('production.blocked') }}</span>
                    <span v-else-if="prod.Monitoring" class="text-xs px-1.5 py-0.5 rounded-full bg-green-100 text-green-700">● Live</span>
                    <span v-else class="text-xs px-1.5 py-0.5 rounded-full bg-gray-100 text-gray-500" title="Sin monitoreo">⚠</span>
                  </div>
                </div>

                <!-- Worker progress -->
                <div v-if="workerStore.isProcessingProduction(prod.ProduccionID)" class="mb-2 rounded-lg bg-green-50 border border-green-200 px-2 py-1.5 space-y-1">
                  <div class="flex items-center justify-between text-xs">
                    <div class="flex items-center gap-1.5 text-green-700 font-medium">
                      <span class="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse shrink-0"/>
                      <span v-if="workerStore.stopPending" class="text-orange-600">Deteniendo...</span>
                      <span v-else>{{ t('worker.phase_processing') }}</span>
                    </div>
                    <div class="flex items-center gap-1.5">
                      <span v-if="workerStatusFor(prod.ProduccionID)?.scenes_total" class="text-green-600 tabular-nums">
                        {{ workerStatusFor(prod.ProduccionID)?.scenes_done }}/{{ workerStatusFor(prod.ProduccionID)?.scenes_total }}
                      </span>
                      <button
                        @click.stop="cancelFromCard"
                        :disabled="!canControlWorker || cancellingCard"
                        :title="!canControlWorker ? t('permisos.sinPermiso') : undefined"
                        class="text-xs px-1.5 py-0.5 rounded border transition-colors disabled:opacity-60"
                        :class="workerStore.stopPending
                          ? 'border-orange-200 text-orange-500 hover:bg-orange-50'
                          : 'border-red-200 text-red-500 hover:bg-red-50'"
                      >{{ workerStore.stopPending ? '↩' : '■' }}</button>
                    </div>
                  </div>
                  <div v-if="workerProgressPct(workerStatusFor(prod.ProduccionID)) !== null" class="w-full bg-green-100 rounded-full h-1 overflow-hidden">
                    <div class="bg-green-500 h-1 rounded-full transition-all duration-500" :style="{ width: workerProgressPct(workerStatusFor(prod.ProduccionID)) + '%' }" />
                  </div>
                  <p v-if="workerStatusFor(prod.ProduccionID)?.current_scene" class="text-xs text-green-600 truncate font-mono leading-none">
                    {{ workerStatusFor(prod.ProduccionID)?.current_scene }}
                  </p>
                </div>

                <!-- Conteo de escenas -->
                <div v-if="prod.stats" class="border-t border-gray-100 pt-2 mb-2 flex items-center gap-2 text-xs flex-wrap">
                  <span class="text-gray-400">{{ prod.stats.scenes_total }} esc.</span>
                  <span class="text-green-600">{{ prod.stats.scenes_usables }} usables</span>
                  <span class="text-blue-600">{{ prod.stats.scenes_completadas }} ✓</span>
                  <span v-if="prod.stats.scenes_pendientes > 0" class="text-yellow-600">{{ prod.stats.scenes_pendientes }} pend.</span>
                </div>

                <!-- TIF / IA -->
                <div class="border-t border-gray-100 pt-2 grid grid-cols-2 gap-2 text-xs">
                  <div class="space-y-0.5">
                    <div class="flex items-center justify-between gap-1">
                      <div class="flex items-center gap-1">
                        <span class="text-gray-400 font-medium">TIF</span>
                        <span class="font-bold" :class="statusColor[tifStatus(prod)]">{{ statusIcon[tifStatus(prod)] }}</span>
                        <span class="text-gray-500 tabular-nums" v-if="prod.stats">
                          {{ prod.stats.scenes_tif_ok }}/{{ prod.stats.scenes_total }}
                        </span>
                      </div>
                      <button
                        v-if="tifStatus(prod) !== 'ok'"
                        @click.stop="ponerAlCorriente(prod, $event)"
                        :disabled="!permStore.puede('monitoreo.worker', prod.CentroCostoID) || triggeringProd.has(prod.ID) || workerStore.isProcessingProduction(prod.ProduccionID) || blockIndividualTrigger"
                        class="flex items-center gap-1 px-1.5 py-0.5 rounded border text-xs transition-colors"
                        :class="!permStore.puede('monitoreo.worker', prod.CentroCostoID) || triggeringProd.has(prod.ID) || workerStore.isProcessingProduction(prod.ProduccionID) || blockIndividualTrigger
                          ? 'border-gray-200 text-gray-300 cursor-not-allowed'
                          : 'border-green-200 text-green-600 hover:bg-green-50'"
                        :title="!permStore.puede('monitoreo.worker', prod.CentroCostoID) ? t('permisos.sinPermiso') : workerStore.isGlobalProcessing ? 'Worker global en ejecución' : anyProductionProcessing ? 'Otra producción en proceso' : t('production.process')"
                      >
                        <svg v-if="triggeringProd.has(prod.ID)" class="animate-spin h-2.5 w-2.5" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
                        </svg>
                        <span v-else-if="workerStore.isProcessingProduction(prod.ProduccionID)">⟳</span>
                        <span v-else>▶</span>
                      </button>
                    </div>
                    <p class="text-gray-400 leading-none">
                      {{ prod.stats?.last_tif_fecha ? formatAgo(prod.stats.last_tif_fecha) : t('common.never') }}
                    </p>
                    <p v-if="triggerErrors.has(prod.ID)" class="text-red-500 leading-none">{{ triggerErrors.get(prod.ID) }}</p>
                  </div>

                  <div class="space-y-0.5">
                    <div class="flex items-center gap-1">
                      <span class="text-gray-400 font-medium">IA</span>
                      <span class="font-bold" :class="statusColor[iaStatus(prod)]">{{ statusIcon[iaStatus(prod)] }}</span>
                      <span v-if="prod.IAuto && prod.stats" class="text-gray-500 tabular-nums">
                        {{ prod.stats.scenes_ia_ok }}/{{ prod.stats.scenes_usables }}
                      </span>
                      <button
                        @click.stop="toggleIAAuto(prod, $event)"
                        :disabled="!permStore.puede('producciones.editar', prod.CentroCostoID) || togglingIA.has(prod.ID)"
                        class="ml-auto flex items-center gap-0.5 text-xs rounded px-1 py-0.5 transition-colors border"
                        :class="!permStore.puede('producciones.editar', prod.CentroCostoID)
                          ? 'border-gray-200 bg-gray-50 text-gray-300 cursor-not-allowed'
                          : prod.IAuto
                          ? 'border-blue-200 bg-blue-50 text-blue-600 hover:bg-blue-100'
                          : 'border-gray-200 bg-gray-50 text-gray-400 hover:bg-gray-100'"
                        :title="!permStore.puede('producciones.editar', prod.CentroCostoID) ? t('permisos.sinPermiso') : prod.IAuto ? t('production.ia_auto_on') : t('production.ia_auto_off')"
                      >
                        <svg v-if="togglingIA.has(prod.ID)" class="animate-spin h-2.5 w-2.5" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
                        </svg>
                        <span v-else>{{ prod.IAuto ? 'auto' : 'off' }}</span>
                      </button>
                    </div>
                    <p class="text-gray-400 leading-none">
                      <template v-if="!prod.IAuto">—</template>
                      <template v-else>{{ prod.stats?.last_ia_fecha ? formatAgo(prod.stats.last_ia_fecha) : t('common.never') }}</template>
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
