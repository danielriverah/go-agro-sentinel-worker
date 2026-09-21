<script setup lang="ts">
// Estado del pipeline como dos eslabones de una cadena:
//   DynamoDB ──①SYNC──► MySQL ──②PROCESAMIENTO──► Archivos + IA
// Regla de lectura: el estado general nunca puede ser mejor que el eslabón más
// débil. Si el sync dejó producciones fuera, el verde del procesamiento sólo
// significa "terminé todo lo que hay en la base", no "todo está procesado".
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useSyncStore } from '@/stores/sync'
import { useWorkerStore } from '@/stores/worker'
import { useProductionsStore } from '@/stores/productions'
import type { SkipReason } from '@/api/types'

const props = withDefaults(defineProps<{ compact?: boolean }>(), { compact: false })

const router = useRouter()
const syncStore = useSyncStore()
const workerStore = useWorkerStore()
const prodStore = useProductionsStore()

const expanded = ref(false)

type LinkState = 'running' | 'error' | 'warn' | 'ok' | 'unknown'

const REASON_LABEL: Record<SkipReason, string> = {
  sin_poligono:         'Sin polígono asignado',
  sin_fecha_plantacion: 'Sin fecha de plantación',
  no_existe_en_erp:     'No existe en el ERP',
  bloqueada:            'Bloqueada',
  fin_monitoreo:        'Fin de monitoreo',
}

const REASON_FIX: Record<SkipReason, string> = {
  sin_poligono:         'Asignar el polígono en asignaciones_zonas_producciones',
  sin_fecha_plantacion: 'Capturar la fecha de plantación en DynamoDB',
  no_existe_en_erp:     'Revisar que la producción exista en la tabla producciones',
  bloqueada:            'Desbloquear desde el detalle de la producción',
  fin_monitoreo:        'Ninguna — el periodo de monitoreo terminó',
}

// ── Eslabón ①: SYNC ─────────────────────────────────────────────────────────
const report = computed(() => syncStore.status?.report ?? null)
const skipped = computed(() => report.value?.prods_skipped ?? [])
const syncErrors = computed(() => report.value?.errors ?? [])

// "Verificación vieja": el doble del intervalo programado sin correr.
const staleThresholdMs = computed(() => {
  const next = syncStore.status?.next_run
  const last = syncStore.status?.last_run
  if (!next || !last) return null
  const interval = new Date(next).getTime() - new Date(last).getTime()
  return interval > 0 ? interval * 2 : null
})

const syncStale = computed(() => {
  const last = syncStore.status?.last_run
  const threshold = staleThresholdMs.value
  if (!last || !threshold) return false
  return Date.now() - new Date(last).getTime() > threshold
})

const syncState = computed<LinkState>(() => {
  if (syncStore.status?.running) return 'running'
  if (!report.value) return 'unknown'
  if (syncErrors.value.length > 0) return 'error'
  if (skipped.value.length > 0 || syncStale.value) return 'warn'
  return 'ok'
})

const syncDetail = computed(() => {
  if (syncStore.status?.running) return 'Sincronizando...'
  if (!report.value) return 'Sin ciclos registrados'
  if (syncErrors.value.length > 0) {
    return `${syncErrors.value.length} error${syncErrors.value.length !== 1 ? 'es' : ''}`
  }
  if (skipped.value.length > 0) {
    return `${skipped.value.length} producción${skipped.value.length !== 1 ? 'es' : ''} omitida${skipped.value.length !== 1 ? 's' : ''}`
  }
  if (syncStale.value) return 'Verificación vieja'
  return 'Al día'
})

// ── Eslabón ②: PROCESAMIENTO ───────────────────────────────────────────────
// Las FAILED cuentan igual que las PENDING: ambas son trabajo por hacer.
const pendientes = computed(() =>
  prodStore.productions.reduce((n, p) => n + (p.stats?.scenes_pendientes ?? 0), 0)
)

const procState = computed<LinkState>(() => {
  if (workerStore.isGlobalProcessing || workerStore.anyProductionProcessing) return 'running'
  return pendientes.value > 0 ? 'warn' : 'ok'
})

const procDetail = computed(() => {
  if (workerStore.isGlobalProcessing) return 'Procesando todo'
  if (workerStore.anyProductionProcessing) return 'Procesando producción'
  if (pendientes.value > 0) {
    return `${pendientes.value} escena${pendientes.value !== 1 ? 's' : ''} pendiente${pendientes.value !== 1 ? 's' : ''}`
  }
  return 'Al día'
})

// ── Lectura conjunta ────────────────────────────────────────────────────────
const STATE_STYLE: Record<LinkState, { dot: string; text: string; icon: string }> = {
  running: { dot: 'bg-blue-500 animate-pulse', text: 'text-blue-600',   icon: '🔄' },
  error:   { dot: 'bg-red-500',                text: 'text-red-600',    icon: '⛔' },
  warn:    { dot: 'bg-amber-500',              text: 'text-amber-600',  icon: '⚠' },
  ok:      { dot: 'bg-green-500',              text: 'text-green-600',  icon: '✓' },
  unknown: { dot: 'bg-gray-300',               text: 'text-gray-400',   icon: '?' },
}

// El estado general es el eslabón más débil.
const RANK: Record<LinkState, number> = { error: 0, warn: 1, unknown: 2, running: 3, ok: 4 }
const overallState = computed<LinkState>(() =>
  RANK[syncState.value] <= RANK[procState.value] ? syncState.value : procState.value
)

// El mensaje que explica la combinación en lenguaje llano.
const message = computed(() => {
  const syncBad = syncState.value === 'warn' || syncState.value === 'error'
  const procPending = pendientes.value > 0

  if (syncState.value === 'unknown') {
    return 'El sync no ha corrido todavía, así que no se sabe si la base está completa.'
  }
  if (syncErrors.value.length > 0 && procPending) {
    return `El último sync falló y quedan ${pendientes.value} escenas por procesar.`
  }
  if (syncErrors.value.length > 0) {
    return 'El procesamiento terminó todo lo que hay en la base, pero el último sync tuvo errores — puede haber datos sin bajar.'
  }
  if (syncBad && !procPending) {
    const n = skipped.value.length
    if (n > 0) {
      return `El procesamiento terminó todo lo que hay en la base, pero el sync dejó ${n} producción${n !== 1 ? 'es' : ''} fuera — puede haber datos sin bajar.`
    }
    return 'El procesamiento está al día, pero hace rato que el sync no verifica DynamoDB.'
  }
  if (syncBad && procPending) {
    return `Sync incompleto (${skipped.value.length} omitidas) y ${pendientes.value} escenas pendientes.`
  }
  if (procPending) {
    return `La base está al día; faltan ${pendientes.value} escenas por procesar.`
  }
  return 'Todo al día — la base coincide con DynamoDB y no hay escenas pendientes.'
})

function openProduccion(id: number) {
  const prod = prodStore.productions.find(p => p.ProduccionID === id)
  if (prod) router.push({ name: 'produccion', params: { id: prod.ID } })
}

function goToProducciones() {
  if (props.compact) router.push({ name: 'producciones' })
}
</script>

<template>
  <!-- ── Modo compacto: KPI para el dashboard ──────────────────────────────── -->
  <div
    v-if="compact"
    @click="goToProducciones"
    class="bg-white border rounded-2xl p-4 cursor-pointer hover:shadow-md transition-all group"
    :class="overallState === 'error' ? 'border-red-300 bg-red-50/40'
      : overallState === 'warn' ? 'border-amber-300 bg-amber-50/40'
      : 'border-gray-200'"
  >
    <p class="text-xs text-gray-400 font-medium uppercase tracking-wide">Estado del pipeline</p>
    <div class="flex items-center gap-2 mt-2">
      <span class="flex items-center gap-1 text-xs" :class="STATE_STYLE[syncState].text">
        <span class="w-2 h-2 rounded-full shrink-0" :class="STATE_STYLE[syncState].dot" />
        Sync
      </span>
      <span class="text-gray-300">→</span>
      <span class="flex items-center gap-1 text-xs" :class="STATE_STYLE[procState].text">
        <span class="w-2 h-2 rounded-full shrink-0" :class="STATE_STYLE[procState].dot" />
        Proceso
      </span>
      <span class="ml-auto text-gray-300 group-hover:text-green-500 transition-colors">→</span>
    </div>
    <p class="text-xs mt-2 leading-snug" :class="overallState === 'ok' ? 'text-gray-400' : 'text-gray-600'">
      {{ message }}
    </p>
  </div>

  <!-- ── Modo completo ─────────────────────────────────────────────────────── -->
  <div
    v-else
    class="bg-white border rounded-2xl p-4"
    :class="overallState === 'error' ? 'border-red-300'
      : overallState === 'warn' ? 'border-amber-300'
      : 'border-gray-200'"
  >
    <h2 class="text-sm font-semibold text-gray-800 mb-3">Estado del pipeline</h2>

    <!-- La cadena -->
    <div class="flex items-stretch gap-2 overflow-x-auto">
      <!-- Origen -->
      <div class="flex flex-col justify-center shrink-0">
        <span class="text-xs text-gray-400 whitespace-nowrap">DynamoDB</span>
      </div>

      <div class="flex items-center text-gray-300 shrink-0">──►</div>

      <!-- Eslabón 1 -->
      <div
        class="rounded-lg border px-3 py-2 shrink-0 min-w-32"
        :class="syncState === 'error' ? 'border-red-200 bg-red-50/50'
          : syncState === 'warn' ? 'border-amber-200 bg-amber-50/50'
          : syncState === 'running' ? 'border-blue-200 bg-blue-50/50'
          : 'border-gray-200 bg-gray-50/50'"
      >
        <div class="flex items-center gap-1.5">
          <span class="w-2 h-2 rounded-full shrink-0" :class="STATE_STYLE[syncState].dot" />
          <span class="text-xs font-semibold text-gray-700">① SYNC</span>
        </div>
        <p class="text-xs mt-1 font-medium" :class="STATE_STYLE[syncState].text">{{ syncDetail }}</p>
      </div>

      <div class="flex items-center text-gray-300 shrink-0">──►</div>

      <!-- Intermedio -->
      <div class="flex flex-col justify-center shrink-0">
        <span class="text-xs text-gray-400 whitespace-nowrap">MySQL</span>
      </div>

      <div class="flex items-center text-gray-300 shrink-0">──►</div>

      <!-- Eslabón 2 -->
      <div
        class="rounded-lg border px-3 py-2 shrink-0 min-w-32"
        :class="procState === 'warn' ? 'border-amber-200 bg-amber-50/50'
          : procState === 'running' ? 'border-blue-200 bg-blue-50/50'
          : 'border-gray-200 bg-gray-50/50'"
      >
        <div class="flex items-center gap-1.5">
          <span class="w-2 h-2 rounded-full shrink-0" :class="STATE_STYLE[procState].dot" />
          <span class="text-xs font-semibold text-gray-700">② PROCESO</span>
          <!-- El verde del proceso lleva advertencia si el sync dejó pendientes -->
          <span
            v-if="procState === 'ok' && (syncState === 'warn' || syncState === 'error')"
            class="text-amber-500 text-xs"
            title="Al día sólo con lo que hay en la base"
          >⚠</span>
        </div>
        <p class="text-xs mt-1 font-medium" :class="STATE_STYLE[procState].text">{{ procDetail }}</p>
      </div>

      <div class="flex items-center text-gray-300 shrink-0">──►</div>

      <!-- Destino -->
      <div class="flex flex-col justify-center shrink-0">
        <span class="text-xs text-gray-400 whitespace-nowrap">Archivos + IA</span>
      </div>
    </div>

    <!-- Mensaje de lectura conjunta -->
    <div
      class="mt-3 flex items-start gap-2 text-xs rounded-lg px-3 py-2"
      :class="overallState === 'error' ? 'bg-red-50 text-red-700'
        : overallState === 'warn' ? 'bg-amber-50 text-amber-800'
        : overallState === 'running' ? 'bg-blue-50 text-blue-700'
        : 'bg-green-50 text-green-700'"
    >
      <span class="shrink-0">{{ STATE_STYLE[overallState].icon }}</span>
      <p class="flex-1 leading-snug">{{ message }}</p>
      <button
        v-if="skipped.length > 0 || syncErrors.length > 0"
        @click="expanded = !expanded"
        class="shrink-0 underline hover:no-underline whitespace-nowrap"
      >
        {{ expanded ? 'Ocultar' : 'Ver detalle' }}
      </button>
    </div>

    <!-- Detalle de omitidas -->
    <div v-if="expanded" class="mt-2 border-t border-gray-100 pt-2 space-y-1">
      <button
        v-for="s in skipped"
        :key="s.produccion_id"
        @click="openProduccion(s.produccion_id)"
        class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-gray-50 transition-colors text-left text-xs group"
      >
        <span class="font-medium text-gray-800 group-hover:text-green-700 shrink-0">
          {{ s.folio || `#${s.produccion_id}` }}
        </span>
        <span class="px-1.5 py-0.5 rounded bg-amber-100 text-amber-700 shrink-0">
          {{ REASON_LABEL[s.reason] ?? s.reason }}
        </span>
        <span class="text-gray-400 truncate">{{ REASON_FIX[s.reason] ?? '' }}</span>
      </button>

      <p v-for="(e, i) in syncErrors" :key="`err-${i}`" class="px-2 py-1.5 text-xs text-red-600 font-mono break-all">
        {{ e }}
      </p>
    </div>

    <!-- Pie con los conteos del último ciclo -->
    <p v-if="report" class="mt-2 text-xs text-gray-400 tabular-nums">
      Último ciclo: {{ report.prods_in_dynamo }} en DynamoDB ·
      {{ report.prods_upserted }} sincronizadas ·
      {{ report.prods_monitoring }} en monitoreo ·
      {{ report.escenas_inserted }} escenas nuevas
    </p>
  </div>
</template>
