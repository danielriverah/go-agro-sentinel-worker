<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import PipelineStatus from '@/components/PipelineStatus.vue'
import { useNavContext } from '@/composables/useNavContext'
import { useProductionsStore } from '@/stores/productions'
import type { Production } from '@/api/types'

const { t, locale } = useI18n()
const router = useRouter()
const prodStore = useProductionsStore()

const productions = computed(() => prodStore.productions)
const loading = computed(() => prodStore.loading)

// ── KPIs ─────────────────────────────────────────────────────────────────────
interface Totals {
  monitoreadas: number
  total: number
  escenas: number
  completadas: number
  pendientes: number
  usables: number
  tifOk: number
  iaOk: number
  cosecha: number
  bloqueadas: number
  sinMonitoreo: number
}

const totals = computed<Totals>(() => {
  const acc: Totals = {
    monitoreadas: 0, total: 0, escenas: 0, completadas: 0, pendientes: 0,
    usables: 0, tifOk: 0, iaOk: 0, cosecha: 0, bloqueadas: 0, sinMonitoreo: 0,
  }
  for (const p of productions.value) {
    acc.total++
    if (p.Monitoring) acc.monitoreadas++; else acc.sinMonitoreo++
    if (p.PosibleCosecha) acc.cosecha++
    if (p.Bloqueado) acc.bloqueadas++
    const s = p.stats
    if (!s) continue
    acc.escenas     += s.scenes_total
    acc.completadas += s.scenes_completadas
    acc.pendientes  += s.scenes_pendientes
    acc.usables     += s.scenes_usables
    acc.tifOk       += s.scenes_tif_ok
    acc.iaOk        += s.scenes_ia_ok
  }
  return acc
})

const avancePct = computed(() => {
  const { escenas, completadas } = totals.value
  return escenas === 0 ? 0 : Math.round((completadas / escenas) * 100)
})

function pct(part: number, whole: number): number {
  return whole === 0 ? 0 : Math.round((part / whole) * 100)
}

// ── Top ranchos por escenas pendientes ───────────────────────────────────────
const topRanchos = computed(() => {
  const map = new Map<string, { rancho: string; pendientes: number; prods: number }>()
  for (const p of productions.value) {
    const rancho = p.Rancho?.trim() || '— Sin rancho —'
    const pend = p.stats?.scenes_pendientes ?? 0
    const cur = map.get(rancho)
    if (cur) { cur.pendientes += pend; cur.prods++ }
    else map.set(rancho, { rancho, pendientes: pend, prods: 1 })
  }
  return [...map.values()]
    .filter(r => r.pendientes > 0)
    .sort((a, b) => b.pendientes - a.pendientes)
    .slice(0, 5)
})

const maxPendientes = computed(() => Math.max(1, ...topRanchos.value.map(r => r.pendientes)))

// ── Listas de atención ───────────────────────────────────────────────────────
const conCosecha  = computed(() => productions.value.filter(p => p.PosibleCosecha))
const bloqueadas  = computed(() => productions.value.filter(p => p.Bloqueado))

const actividadReciente = computed(() => {
  const withDate = productions.value
    .map(p => {
      const tif = p.stats?.last_tif_fecha ? new Date(p.stats.last_tif_fecha).getTime() : 0
      const ia  = p.stats?.last_ia_fecha  ? new Date(p.stats.last_ia_fecha).getTime()  : 0
      const last = Math.max(tif, ia)
      return { prod: p, last, kind: ia >= tif ? 'IA' : 'TIF' }
    })
    .filter(x => x.last > 0)
  withDate.sort((a, b) => b.last - a.last)
  return withDate.slice(0, 6)
})

// ── Navegación ───────────────────────────────────────────────────────────────
const { productionIds: navProductionIds } = useNavContext()

function openProduccion(prod: Production) {
  navProductionIds.value = productions.value.map(p => p.ID)
  router.push({ name: 'produccion', params: { id: prod.ID } })
}

function formatAgo(iso: string | null | undefined): string {
  if (!iso) return t('sync.never')
  const diffM = Math.floor((Date.now() - new Date(iso).getTime()) / 60_000)
  if (diffM < 1)  return locale.value === 'es' ? 'ahora mismo' : 'just now'
  if (diffM < 60) return locale.value === 'es' ? `hace ${diffM}m` : `${diffM}m ago`
  const diffH = Math.floor(diffM / 60)
  if (diffH < 24) return locale.value === 'es' ? `hace ${diffH}h` : `${diffH}h ago`
  const diffD = Math.floor(diffH / 24)
  if (diffD < 7)  return locale.value === 'es' ? `hace ${diffD}d` : `${diffD}d ago`
  return new Date(iso).toLocaleDateString(locale.value === 'es' ? 'es-MX' : 'en-US', { month: 'short', day: 'numeric' })
}
</script>

<template>
  <div class="max-w-7xl mx-auto px-4 py-6 space-y-5">

    <!-- Header -->
    <div class="flex items-center justify-between gap-4 flex-wrap">
      <div>
        <h1 class="text-xl font-bold text-gray-900">Dashboard</h1>
        <p class="text-xs text-gray-400 mt-0.5">Resumen operativo de monitoreo satelital</p>
      </div>
      <RouterLink
        to="/producciones"
        class="text-xs px-3 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 hover:border-green-300 hover:text-green-700 transition-colors"
      >
        Ver producciones →
      </RouterLink>
    </div>

    <!-- Skeleton -->
    <div v-if="loading && productions.length === 0" class="grid grid-cols-2 lg:grid-cols-5 gap-3">
      <div v-for="i in 5" :key="i" class="bg-white border border-gray-200 rounded-2xl p-4 h-24 animate-pulse">
        <div class="h-3 bg-gray-100 rounded w-2/3 mb-3"/>
        <div class="h-6 bg-gray-100 rounded w-1/2"/>
      </div>
    </div>

    <template v-else>
      <!-- Estado del pipeline — resumen clickeable hacia producciones -->
      <PipelineStatus compact />

      <!-- ── Fila 1: KPIs ─────────────────────────────────────────────────── -->
      <div class="grid grid-cols-2 lg:grid-cols-5 gap-3">
        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <p class="text-xs text-gray-400 font-medium uppercase tracking-wide">Monitoreadas</p>
          <p class="text-2xl font-bold text-gray-900 tabular-nums mt-1">{{ totals.monitoreadas }}</p>
          <p class="text-xs text-gray-400 mt-0.5">de {{ totals.total }} producciones</p>
        </div>

        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <p class="text-xs text-gray-400 font-medium uppercase tracking-wide">Avance global</p>
          <p class="text-2xl font-bold text-green-600 tabular-nums mt-1">{{ avancePct }}%</p>
          <div class="w-full bg-gray-100 rounded-full h-1.5 overflow-hidden mt-2">
            <div class="bg-green-500 h-1.5 rounded-full transition-all duration-500" :style="{ width: avancePct + '%' }" />
          </div>
          <p class="text-xs text-gray-400 mt-1 tabular-nums">{{ totals.completadas }} / {{ totals.escenas }} escenas</p>
        </div>

        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <p class="text-xs text-gray-400 font-medium uppercase tracking-wide">Pendientes</p>
          <p class="text-2xl font-bold tabular-nums mt-1" :class="totals.pendientes > 0 ? 'text-orange-600' : 'text-gray-900'">
            {{ totals.pendientes }}
          </p>
          <p class="text-xs text-gray-400 mt-0.5">escenas por procesar</p>
        </div>

        <div class="bg-white border rounded-2xl p-4" :class="totals.cosecha > 0 ? 'border-yellow-300 bg-yellow-50/40' : 'border-gray-200'">
          <p class="text-xs text-gray-400 font-medium uppercase tracking-wide">🌾 Posible cosecha</p>
          <p class="text-2xl font-bold tabular-nums mt-1" :class="totals.cosecha > 0 ? 'text-yellow-700' : 'text-gray-900'">
            {{ totals.cosecha }}
          </p>
          <p class="text-xs text-gray-400 mt-0.5">requieren revisión</p>
        </div>

        <div class="bg-white border rounded-2xl p-4" :class="totals.bloqueadas > 0 ? 'border-red-300 bg-red-50/40' : 'border-gray-200'">
          <p class="text-xs text-gray-400 font-medium uppercase tracking-wide">⛔ Bloqueadas</p>
          <p class="text-2xl font-bold tabular-nums mt-1" :class="totals.bloqueadas > 0 ? 'text-red-600' : 'text-gray-900'">
            {{ totals.bloqueadas }}
          </p>
          <p class="text-xs text-gray-400 mt-0.5">requieren desbloqueo</p>
        </div>
      </div>

      <!-- ── Fila 2: Visualizaciones ──────────────────────────────────────── -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-3">

        <!-- Distribución de escenas -->
        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <h2 class="text-sm font-semibold text-gray-800 mb-3">Escenas por estado</h2>
          <div v-if="totals.escenas === 0" class="text-xs text-gray-400 py-6 text-center">Sin escenas registradas</div>
          <template v-else>
            <div class="flex w-full h-3 rounded-full overflow-hidden bg-gray-100">
              <div class="bg-green-500 transition-all duration-500" :style="{ width: pct(totals.completadas, totals.escenas) + '%' }" title="Completadas" />
              <div class="bg-orange-400 transition-all duration-500" :style="{ width: pct(totals.pendientes, totals.escenas) + '%' }" title="Pendientes" />
            </div>
            <div class="mt-3 space-y-1.5 text-xs">
              <div class="flex items-center gap-2">
                <span class="w-2.5 h-2.5 rounded-sm bg-green-500 shrink-0" />
                <span class="text-gray-600">Completadas</span>
                <span class="ml-auto tabular-nums text-gray-800 font-medium">{{ totals.completadas }}</span>
                <span class="tabular-nums text-gray-400 w-9 text-right">{{ pct(totals.completadas, totals.escenas) }}%</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="w-2.5 h-2.5 rounded-sm bg-orange-400 shrink-0" />
                <span class="text-gray-600">Pendientes</span>
                <span class="ml-auto tabular-nums text-gray-800 font-medium">{{ totals.pendientes }}</span>
                <span class="tabular-nums text-gray-400 w-9 text-right">{{ pct(totals.pendientes, totals.escenas) }}%</span>
              </div>
              <div class="flex items-center gap-2 pt-1.5 border-t border-gray-100">
                <span class="w-2.5 h-2.5 rounded-sm bg-gray-200 shrink-0" />
                <span class="text-gray-600">Usables</span>
                <span class="ml-auto tabular-nums text-gray-800 font-medium">{{ totals.usables }}</span>
                <span class="tabular-nums text-gray-400 w-9 text-right">{{ pct(totals.usables, totals.escenas) }}%</span>
              </div>
            </div>
          </template>
        </div>

        <!-- Cobertura TIF / IA -->
        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <h2 class="text-sm font-semibold text-gray-800 mb-3">Cobertura de procesamiento</h2>
          <div v-if="totals.escenas === 0" class="text-xs text-gray-400 py-6 text-center">Sin escenas registradas</div>
          <div v-else class="space-y-3">
            <!-- Toda escena completada tiene TIF, nublada o no: el denominador
                 es el total de escenas, no las usables. -->
            <div>
              <div class="flex items-center justify-between text-xs mb-1">
                <span class="text-gray-600">TIF generados</span>
                <span class="tabular-nums text-gray-800 font-medium">{{ totals.tifOk }} / {{ totals.escenas }}</span>
              </div>
              <div class="w-full bg-gray-100 rounded-full h-2 overflow-hidden">
                <div class="bg-blue-500 h-2 rounded-full transition-all duration-500" :style="{ width: pct(totals.tifOk, totals.escenas) + '%' }" />
              </div>
              <p class="text-xs text-gray-400 mt-1 tabular-nums">{{ pct(totals.tifOk, totals.escenas) }}% de las escenas</p>
            </div>
            <!-- La IA solo corre sobre escenas usables, así que ese es su universo. -->
            <div>
              <div class="flex items-center justify-between text-xs mb-1">
                <span class="text-gray-600">Análisis IA</span>
                <span class="tabular-nums text-gray-800 font-medium">{{ totals.iaOk }} / {{ totals.usables }}</span>
              </div>
              <div class="w-full bg-gray-100 rounded-full h-2 overflow-hidden">
                <div class="bg-purple-500 h-2 rounded-full transition-all duration-500" :style="{ width: pct(totals.iaOk, totals.usables) + '%' }" />
              </div>
              <p class="text-xs text-gray-400 mt-1 tabular-nums">{{ pct(totals.iaOk, totals.usables) }}% de las escenas usables</p>
            </div>
          </div>
        </div>

        <!-- Top ranchos por pendientes -->
        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <h2 class="text-sm font-semibold text-gray-800 mb-3">Ranchos con más pendientes</h2>
          <div v-if="topRanchos.length === 0" class="text-xs text-gray-400 py-6 text-center">
            ✓ Sin escenas pendientes
          </div>
          <div v-else class="space-y-2">
            <div v-for="r in topRanchos" :key="r.rancho" class="text-xs">
              <div class="flex items-center justify-between gap-2 mb-1">
                <span class="text-gray-600 truncate">{{ r.rancho }}</span>
                <span class="tabular-nums text-orange-600 font-medium shrink-0">{{ r.pendientes }}</span>
              </div>
              <div class="w-full bg-gray-100 rounded-full h-1.5 overflow-hidden">
                <div class="bg-orange-400 h-1.5 rounded-full transition-all duration-500" :style="{ width: pct(r.pendientes, maxPendientes) + '%' }" />
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- ── Fila 3: Listas de atención ───────────────────────────────────── -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-3">

        <!-- Posible cosecha -->
        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <div class="flex items-center gap-2 mb-3">
            <h2 class="text-sm font-semibold text-gray-800">🌾 Posible cosecha</h2>
            <span v-if="conCosecha.length" class="text-xs px-1.5 py-0.5 rounded-full bg-yellow-100 text-yellow-700 tabular-nums">{{ conCosecha.length }}</span>
          </div>
          <div v-if="conCosecha.length === 0" class="text-xs text-gray-400 py-4 text-center">Ninguna detectada</div>
          <div v-else class="space-y-1">
            <button
              v-for="p in conCosecha.slice(0, 6)"
              :key="p.ID"
              @click="openProduccion(p)"
              class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-yellow-50 transition-colors text-left group"
            >
              <span class="text-xs font-medium text-gray-800 group-hover:text-yellow-700 truncate">{{ p.Folio || '—' }}</span>
              <span class="text-xs text-gray-400 truncate">{{ p.Rancho }}</span>
              <span class="ml-auto text-gray-300 group-hover:text-yellow-600 shrink-0">→</span>
            </button>
          </div>
        </div>

        <!-- Bloqueadas -->
        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <div class="flex items-center gap-2 mb-3">
            <h2 class="text-sm font-semibold text-gray-800">⛔ Bloqueadas</h2>
            <span v-if="bloqueadas.length" class="text-xs px-1.5 py-0.5 rounded-full bg-red-100 text-red-700 tabular-nums">{{ bloqueadas.length }}</span>
          </div>
          <div v-if="bloqueadas.length === 0" class="text-xs text-gray-400 py-4 text-center">Ninguna bloqueada</div>
          <div v-else class="space-y-1">
            <button
              v-for="p in bloqueadas.slice(0, 6)"
              :key="p.ID"
              @click="openProduccion(p)"
              class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-red-50 transition-colors text-left group"
            >
              <span class="text-xs font-medium text-gray-800 group-hover:text-red-700 truncate">{{ p.Folio || '—' }}</span>
              <span class="text-xs text-gray-400 truncate">{{ p.Rancho }}</span>
              <span class="ml-auto text-gray-300 group-hover:text-red-600 shrink-0">→</span>
            </button>
          </div>
        </div>

        <!-- Actividad reciente -->
        <div class="bg-white border border-gray-200 rounded-2xl p-4">
          <h2 class="text-sm font-semibold text-gray-800 mb-3">Actividad reciente</h2>
          <div v-if="actividadReciente.length === 0" class="text-xs text-gray-400 py-4 text-center">Sin actividad registrada</div>
          <div v-else class="space-y-1">
            <button
              v-for="a in actividadReciente"
              :key="a.prod.ID"
              @click="openProduccion(a.prod)"
              class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-gray-50 transition-colors text-left group"
            >
              <span
                class="text-[10px] font-semibold px-1 py-0.5 rounded shrink-0"
                :class="a.kind === 'IA' ? 'bg-purple-100 text-purple-600' : 'bg-blue-100 text-blue-600'"
              >{{ a.kind }}</span>
              <span class="text-xs font-medium text-gray-800 group-hover:text-green-700 truncate">{{ a.prod.Folio || '—' }}</span>
              <span class="ml-auto text-xs text-gray-400 shrink-0">{{ formatAgo(new Date(a.last).toISOString()) }}</span>
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
