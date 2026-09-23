<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { producciones as prodApi, worker as workerApi, apiErrorMessage } from '@/api/client'
import { useNavContext, scrollToElement } from '@/composables/useNavContext'
import { useWorkerStore } from '@/stores/worker'
import { useProductionsStore } from '@/stores/productions'
import { usePermissionsStore } from '@/stores/permissions'
import TimelineChart from '@/components/TimelineChart.vue'
import FasesEditor from '@/components/FasesEditor.vue'
import type { Scene, SceneStatus } from '@/api/types'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const workerStore = useWorkerStore()
const prodStore = useProductionsStore()
const permStore = usePermissionsStore()

const produccionId = Number(route.params.id)
const error = ref('')

// ── Navegación prev/next entre producciones ───────────────────────────────────
const { productionIds, sceneIds: navSceneIds, cameFromEscena } = useNavContext()

// Si se entra directo por URL, el store ya tiene las producciones (App.vue las cargó).
// Si aún no están, esperamos — prodStore.productions es reactivo.
onMounted(async () => {
  if (productionIds.value.length === 0 && prodStore.productions.length > 0) {
    productionIds.value = prodStore.productions.map(p => p.ID)
  }
  // Cargar detalle si no está en caché
  try {
    await prodStore.ensureDetail(produccionId)
  } catch (err) {
    error.value = apiErrorMessage(err)
  }
})

// Si las producciones llegan después (primera carga de App.vue), poblar el nav
watch(() => prodStore.productions, (list) => {
  if (productionIds.value.length === 0 && list.length > 0) {
    productionIds.value = list.map(p => p.ID)
  }
}, { once: true })

const production = computed(() => prodStore.details[produccionId] ?? null)
const loading = computed(() => prodStore.isLoadingDetail(produccionId).value && !production.value)

const currentIdx = computed(() => productionIds.value.indexOf(produccionId))
const prevProductionId = computed(() => currentIdx.value > 0 ? productionIds.value[currentIdx.value - 1] : null)
const nextProductionId = computed(() => currentIdx.value !== -1 && currentIdx.value < productionIds.value.length - 1 ? productionIds.value[currentIdx.value + 1] : null)

function goToProduccion(id: number) {
  router.push({ name: 'produccion', params: { id } })
}

// ── Volver a la escena que estaba viendo ─────────────────────────────────────
// Al regresar del detalle de una escena, el listado se posiciona en ella y la
// resalta, en lugar de dejar la vista al principio.
const highlightedEscenaId = ref<number | null>(null)
let escenaRevealed = false

async function revealLastVisitedEscena() {
  const id = cameFromEscena.value
  if (escenaRevealed || !id) return
  if (!production.value?.escenas?.some(s => s.ID === id)) return // sin datos aún
  escenaRevealed = true

  highlightedEscenaId.value = id
  await nextTick()
  await scrollToElement(`escena-${id}`)
  setTimeout(() => {
    if (highlightedEscenaId.value === id) highlightedEscenaId.value = null
  }, 3000)
}

// El detalle llega del store y puede no estar listo al montar.
watch(production, revealLastVisitedEscena, { immediate: true })

const triggerMsg = ref('')
const triggerError = ref('')
const triggeringWorker = ref(false)
const unlocking = ref(false)
const cancelling = ref(false)
const cancelMsg = ref('')

async function loadProduction() {
  try {
    await prodStore.refreshDetail(produccionId)
  } catch (err) {
    error.value = apiErrorMessage(err)
  }
}

async function triggerWorker() {
  if (!canRunProductionWorker.value) return
  triggeringWorker.value = true
  triggerMsg.value = ''
  triggerError.value = ''
  try {
    await workerApi.runProduction(produccionId)
    triggerMsg.value = t('production.processing_queued')
    if (erpProduccionId.value) workerStore.markTriggered(erpProduccionId.value)
  } catch (err) {
    const status = (err as any)?.response?.status
    const msg: string = (err as any)?.response?.data?.error ?? ''
    if (status === 409) {
      triggerError.value = msg.includes('otra') ? 'Otra producción en proceso — espera a que termine' : t('production.already_processing')
    } else {
      triggerError.value = apiErrorMessage(err)
    }
  } finally {
    triggeringWorker.value = false
    setTimeout(() => { triggerMsg.value = ''; triggerError.value = '' }, 5000)
  }
}

async function cancelWorker() {
  if (!canControlWorker.value) return
  if (cancelling.value) return

  // Si ya hay señal de detención activa → cancelarla (undo)
  if (workerStore.stopPending) {
    try {
      await workerApi.cancel(undefined, true)  // cancel_stop: true
      cancelMsg.value = 'Detención cancelada'
      setTimeout(() => { cancelMsg.value = '' }, 3000)
    } catch (err) {
      cancelMsg.value = apiErrorMessage(err)
      setTimeout(() => { cancelMsg.value = '' }, 5000)
    }
    return
  }

  if (!erpProduccionId.value) return
  // Si el global está procesando esta producción, la señal es global
  // (el stop trigger es único — aplica a cualquier worker corriendo)
  cancelling.value = true
  cancelMsg.value = ''
  try {
    await workerApi.cancel()  // stop trigger global — el worker lo consume entre escenas
    cancelMsg.value = 'Deteniendo — terminará después de la escena actual...'
    // cancelling permanece true hasta que isProcessing sea false (el lock desapareció)
  } catch (err) {
    cancelMsg.value = apiErrorMessage(err)
    cancelling.value = false
    setTimeout(() => { cancelMsg.value = '' }, 5000)
  }
}

// Ambas acciones escriben posible_cosecha: un trigger de la tabla deriva
// bloqueado a partir de ese campo, así que no se toca bloqueado directamente.
async function desbloquear() {
  if (!production.value || !permStore.puede('producciones.bloquear', production.value.CentroCostoID)) return
  unlocking.value = true
  try {
    await prodApi.desbloquear(produccionId)
    await loadProduction()
  } catch { /* ignore */ } finally {
    unlocking.value = false
  }
}

// Marcar el lote como listo para cosecha; el trigger lo bloquea.
async function marcarPosibleCosecha() {
  if (!production.value || !permStore.puede('producciones.bloquear', production.value.CentroCostoID)) return
  unlocking.value = true
  try {
    await prodApi.bloquear(produccionId)
    await loadProduction()
  } catch { /* ignore */ } finally {
    unlocking.value = false
  }
}

const activeTab = ref<'escenas' | 'tendencias'>('escenas')
const showFases = ref(false)

// ── Eliminar monitoreo (destructivo) ──────────────────────────────────────────
const showDelete = ref(false)
const folioConfirm = ref('')
const deleting = ref(false)
const deleteError = ref('')

// El botón sólo se habilita cuando el folio tecleado coincide exactamente.
// Un "¿estás seguro?" no basta para algo irreversible en tres sistemas.
const folioCoincide = computed(
  () => folioConfirm.value.trim() === (production.value?.Folio ?? '').trim() && folioConfirm.value !== '',
)

async function eliminarMonitoreo() {
  if (!folioCoincide.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    await prodApi.eliminarMonitoreo(produccionId, folioConfirm.value.trim())
    // La producción ya no existe como monitoreo: no hay a dónde volver.
    router.push({ name: 'producciones' })
  } catch (e) {
    deleteError.value = apiErrorMessage(e)
  } finally {
    deleting.value = false
  }
}
// Cambiar esta clave remonta la gráfica tras guardar fases, para que las
// franjas nuevas se reflejen sin recargar la página.
const fasesVersion = ref(0)

function goToEscenaById(escenaId: number) {
  navSceneIds.value = (production.value?.escenas?.map(s => s.ID) ?? []).reverse()
  router.push({ name: 'escena', params: { produccionId, escenaId } })
}

function goToEscena(scene: Scene) {
  goToEscenaById(scene.ID)
}

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleString(locale.value === 'es' ? 'es-MX' : 'en-US', {
    year: 'numeric', month: 'short', day: 'numeric',
  })
}

const statusColors: Record<SceneStatus, string> = {
  PENDING:    'bg-gray-100 text-gray-600',
  PROCESSING: 'bg-blue-100 text-blue-700',
  COMPLETED:  'bg-green-100 text-green-700',
  FAILED:     'bg-red-100 text-red-700',
  SKIPPED:    'bg-yellow-100 text-yellow-700',
}

const riesgoColors = {
  bajo:   'text-green-600',
  medio:  'text-yellow-600',
  alto:   'text-red-600',
  '':     'text-gray-400',
}

// ERP produccion_id — used for worker status (different from MySQL PK in route)
const erpProduccionId = computed(() => production.value?.ProduccionID ?? null)

const isProcessing = computed(() =>
  erpProduccionId.value !== null && workerStore.isProcessingProduction(erpProduccionId.value)
)

// Worker status for THIS production specifically (global or per-production lock)
const workerStatus = computed(() => {
  const id = erpProduccionId.value
  if (id === null) return null
  const s = workerStore.status
  if (s.global?.phase === 'processing' && s.global.current_produccion_id === id) return s.global
  return (s.productions ?? []).find(p => p.phase === 'processing' && p.current_produccion_id === id) ?? null
})

const workerPct = computed(() => {
  const ws = workerStatus.value
  if (!ws || ws.scenes_total === 0) return null
  return Math.round((ws.scenes_done / ws.scenes_total) * 100)
})

const canRunProductionWorker = computed(() =>
  production.value ? permStore.puede('monitoreo.worker', production.value.CentroCostoID) : false
)
const canControlWorker = computed(() => permStore.puede('worker.controlar'))

// Reload scenes on each scene completion and when worker finishes
watch(
  () => workerStatus.value?.scenes_done,
  (done, prev) => {
    if (done !== undefined && prev !== undefined && done > prev) {
      loadProduction()
    }
  }
)
// Observa el lock real del SSE (no el pending optimista) para limpiar el estado de cancelación
const realIsProcessing = computed(() => {
  const id = erpProduccionId.value
  if (id === null) return false
  const s = workerStore.status
  return (
    (s.global?.phase === 'processing' && s.global.current_produccion_id === id) ||
    (s.productions ?? []).some(p => p.phase === 'processing' && p.current_produccion_id === id)
  )
})

watch(realIsProcessing, (active, prev) => {
  if (prev === false && active === true) {
    // Worker acaba de empezar (automático o manual) — recargar para mostrar escena en PROCESSING
    loadProduction()
  }
  if (prev === true && active === false) {
    if (cancelling.value) {
      cancelling.value = false
      cancelMsg.value = 'Detenido'
      setTimeout(() => { cancelMsg.value = '' }, 3000)
    }
    loadProduction()
  }
})

const usableScenes = computed(() =>
  production.value?.escenas?.filter(s => s.Usable).length ?? 0
)
const totalScenes = computed(() => production.value?.escenas?.length ?? 0)
</script>

<template>
  <div class="max-w-5xl mx-auto px-4 py-6 space-y-6">

    <!-- Barra de navegación: atrás + prev/next producción -->
    <div class="flex items-center justify-between gap-2">
      <button
        @click="router.push({ name: 'producciones' })"
        class="flex items-center gap-1.5 text-sm text-gray-500 hover:text-gray-800 transition-colors"
      >
        ← Producciones
      </button>
      <button
        @click="router.push({ name: 'poligono', params: { id: produccionId } })"
        :disabled="!!production && !permStore.puede('producciones.editar', production.CentroCostoID)"
        class="text-xs px-2.5 py-1 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 hover:border-green-300 hover:text-green-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        :title="production && !permStore.puede('producciones.editar', production.CentroCostoID) ? t('permisos.sinPermiso') : 'Ajustar el polígono de monitoreo sobre el mapa'"
      >
        ✎ Editar polígono
      </button>
      <div v-if="productionIds.length > 1" class="flex items-center gap-1">
        <button
          @click="goToProduccion(prevProductionId!)"
          :disabled="prevProductionId === null"
          class="px-2.5 py-1 text-sm rounded-lg border border-gray-200 text-gray-500 hover:bg-gray-50 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          title="Producción anterior"
        >‹</button>
        <span class="text-xs text-gray-400 px-1 tabular-nums">
          {{ currentIdx + 1 }} / {{ productionIds.length }}
        </span>
        <button
          @click="goToProduccion(nextProductionId!)"
          :disabled="nextProductionId === null"
          class="px-2.5 py-1 text-sm rounded-lg border border-gray-200 text-gray-500 hover:bg-gray-50 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          title="Producción siguiente"
        >›</button>
      </div>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="text-center py-16 text-gray-400">{{ t('common.loading') }}</div>

    <!-- Error -->
    <div v-else-if="error" class="text-center py-16">
      <p class="text-red-600 text-sm mb-3">{{ error }}</p>
      <button @click="loadProduction" class="text-xs px-3 py-1.5 rounded-lg border border-gray-200 hover:bg-gray-50">
        {{ t('common.retry') }}
      </button>
    </div>

    <template v-else-if="production">
      <!-- Header card -->
      <div class="bg-white border border-gray-200 rounded-2xl p-5 shadow-sm">
        <div class="flex items-start justify-between gap-4 flex-wrap">
          <div class="space-y-1">
            <div class="flex items-center gap-2 flex-wrap">
              <h1 class="text-xl font-bold text-gray-900">{{ production.Folio }}</h1>
              <span v-if="production.Monitoring" class="text-xs px-2 py-0.5 rounded-full bg-green-100 text-green-700 font-medium">● Live</span>
              <span v-if="production.Bloqueado" class="text-xs px-2 py-0.5 rounded-full bg-red-100 text-red-700">{{ t('production.blocked') }}</span>
              <span v-if="production.IAuto" class="text-xs px-2 py-0.5 rounded-full bg-blue-100 text-blue-700">IA auto</span>
            </div>
            <p class="text-gray-600">{{ production.Rancho }} · {{ production.Cosecha }}</p>
            <div class="flex gap-4 text-xs text-gray-500 mt-1">
              <span>{{ t('production.scenes') }}: <strong class="text-gray-700">{{ usableScenes }}/{{ totalScenes }}</strong> usables</span>
              <span v-if="production.FechaPlantacion">🌱 {{ formatDate(production.FechaPlantacion) }}</span>
              <span v-if="production.FechaFin">🏁 {{ formatDate(production.FechaFin) }}</span>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex gap-2 items-start flex-wrap">
            <button
              v-if="production.Bloqueado"
              @click="desbloquear"
              :disabled="unlocking || !permStore.puede('producciones.bloquear', production.CentroCostoID)"
              :title="!permStore.puede('producciones.bloquear', production.CentroCostoID) ? t('permisos.sinPermiso') : undefined"
              class="text-sm px-3 py-2 rounded-lg border border-red-200 text-red-600 hover:bg-red-50 disabled:opacity-60 transition-colors"
            >
              {{ unlocking ? '...' : t('production.unblock') }}
            </button>
            <button
              v-else
              @click="marcarPosibleCosecha"
              :disabled="unlocking || !permStore.puede('producciones.bloquear', production.CentroCostoID)"
              :title="!permStore.puede('producciones.bloquear', production.CentroCostoID) ? t('permisos.sinPermiso') : t('production.markHarvestHint')"
              class="text-sm px-3 py-2 rounded-lg border border-yellow-300 text-yellow-700 hover:bg-yellow-50 disabled:opacity-60 transition-colors"
            >
              {{ unlocking ? '...' : t('production.markHarvest') }}
            </button>
            <!-- Botón Detener / Cancelar detención — visible cuando ESTA producción procesa -->
            <button
              v-if="isProcessing"
              @click="cancelWorker"
              :disabled="!canControlWorker || cancelling"
              class="text-sm px-4 py-2 rounded-lg border transition-colors flex items-center gap-1.5 disabled:opacity-60"
              :class="workerStore.stopPending
                ? 'border-orange-200 text-orange-600 hover:bg-orange-50'
                : 'border-red-200 text-red-600 hover:bg-red-50'"
              :title="workerStore.stopPending
                ? (!canControlWorker ? t('permisos.sinPermiso') : 'Quitar señal de detención — el worker continuará')
                : (!canControlWorker ? t('permisos.sinPermiso') : 'El worker terminará la escena actual y se detendrá')"
            >
              <svg v-if="cancelling" class="animate-spin h-3.5 w-3.5" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
              </svg>
              <span v-if="cancelling">Deteniendo...</span>
              <span v-else-if="workerStore.stopPending">↩ Cancelar detención</span>
              <span v-else>■ Detener</span>
            </button>
            <!-- Botón Procesar — deshabilitado si cualquier producción está corriendo -->
            <button
              v-else
              @click="triggerWorker"
              :disabled="!canRunProductionWorker || triggeringWorker || workerStore.blockIndividualTrigger"
              class="text-sm px-4 py-2 rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-60 transition-colors"
              :title="!canRunProductionWorker ? t('permisos.sinPermiso') : workerStore.blockIndividualTrigger && !isProcessing ? 'Otra producción en proceso — espera a que termine' : undefined"
            >
              {{ triggeringWorker ? '...' : t('production.process') }}
            </button>
          </div>
        </div>

        <!-- Worker progress for this production -->
        <div v-if="isProcessing && workerStatus" class="mt-4 pt-4 border-t border-gray-100 space-y-1.5">
          <div class="flex justify-between text-xs">
            <span class="text-green-600 font-medium flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse shrink-0" />
              {{ t('worker.phase_processing') }}
            </span>
            <span class="text-gray-500">
              {{ workerStatus.scenes_done }}/{{ workerStatus.scenes_total }} escenas
              <template v-if="workerPct !== null"> · {{ workerPct }}%</template>
            </span>
          </div>
          <div v-if="workerPct !== null" class="h-2 bg-gray-100 rounded-full overflow-hidden">
            <div
              class="h-full bg-green-500 rounded-full transition-all duration-500"
              :style="{ width: workerPct + '%' }"
            />
          </div>
          <p v-if="workerStatus.current_scene" class="text-xs text-gray-400 truncate font-mono">
            {{ workerStatus.current_scene }}
          </p>
          <p v-if="(workerStatus.scenes_failed ?? 0) > 0" class="text-xs text-red-500">
            {{ workerStatus.scenes_failed }} escenas fallidas
          </p>
          <p class="text-xs text-gray-400 italic">La lista de escenas se actualizará al terminar.</p>
        </div>

        <!-- Feedback msgs -->
        <p v-if="triggerMsg" class="mt-3 text-xs text-green-600 bg-green-50 rounded-lg px-3 py-2">✓ {{ triggerMsg }}</p>
        <p v-if="triggerError" class="mt-3 text-xs text-red-600 bg-red-50 rounded-lg px-3 py-2">{{ triggerError }}</p>
        <p v-if="cancelMsg" class="mt-3 text-xs text-orange-600 bg-orange-50 rounded-lg px-3 py-2">{{ cancelMsg }}</p>
      </div>

      <!-- Tabs: listado de escenas vs evolución histórica -->
      <div class="flex gap-1 border-b border-gray-200">
        <button
          v-for="tab in (['escenas', 'tendencias'] as const)"
          :key="tab"
          class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors"
          :class="activeTab === tab
            ? 'border-green-600 text-green-700'
            : 'border-transparent text-gray-500 hover:text-gray-700'"
          @click="activeTab = tab"
        >
          {{ tab === 'escenas' ? t('production.scenes') : t('timeline.tab') }}
        </button>
      </div>

      <!-- Scenes table -->
      <div v-show="activeTab === 'escenas'" class="bg-white border border-gray-200 rounded-2xl shadow-sm overflow-hidden">
        <div class="px-5 py-3 border-b border-gray-100 flex items-center justify-between">
          <h2 class="font-semibold text-gray-800">{{ t('production.scenes') }}</h2>
          <span class="text-xs text-gray-400">{{ totalScenes }} escenas</span>
        </div>

        <!-- Mobile: cards -->
        <div class="sm:hidden divide-y divide-gray-100">
          <div
            v-for="scene in production.escenas"
            :key="scene.ID"
            :data-nav-id="`escena-${scene.ID}`"
            @click="goToEscena(scene)"
            class="px-4 py-3 flex items-center gap-3 cursor-pointer transition-colors"
            :class="highlightedEscenaId === scene.ID
              ? 'bg-green-50 border-l-4 border-green-500'
              : 'hover:bg-gray-50'"
          >
            <div class="flex-1 min-w-0">
              <p class="text-xs font-mono text-gray-600 truncate">{{ scene.SceneName }}</p>
              <p v-if="highlightedEscenaId === scene.ID" class="text-xs text-green-600 font-medium">
                ← Vienes de aquí
              </p>
              <p class="text-xs text-gray-400">{{ formatDate(scene.Fecha) }} · ☁ {{ scene.CloudCover?.toFixed(0) ?? '-' }}%</p>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <span class="text-xs px-2 py-0.5 rounded-full" :class="statusColors[scene.Status]">
                {{ t(`scene.status_${scene.Status}`) }}
              </span>
              <span v-if="scene.LatestIaRiesgoNivel" class="text-xs font-medium" :class="riesgoColors[scene.LatestIaRiesgoNivel]">
                ▲ {{ scene.LatestIaRiesgoNivel }}
              </span>
              <span class="text-gray-300 text-sm">→</span>
            </div>
          </div>
        </div>

        <!-- Desktop: table -->
        <div class="hidden sm:block overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-xs text-gray-400 uppercase tracking-wide bg-gray-50">
                <th class="text-left px-4 py-2.5 font-medium">Escena</th>
                <th class="text-left px-3 py-2.5 font-medium">{{ t('scene.date') }}</th>
                <th class="text-center px-3 py-2.5 font-medium">☁</th>
                <th class="text-center px-3 py-2.5 font-medium">{{ t('scene.status') }}</th>
                <th class="text-center px-3 py-2.5 font-medium">{{ t('scene.usable') }}</th>
                <th class="text-center px-3 py-2.5 font-medium">TIF</th>
                <th class="text-center px-3 py-2.5 font-medium">IA</th>
                <th class="text-center px-3 py-2.5 font-medium">Riesgo</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100">
              <tr
                v-for="scene in production.escenas"
                :key="scene.ID"
                :data-nav-id="`escena-${scene.ID}`"
                @click="goToEscena(scene)"
                class="cursor-pointer transition-colors"
                :class="highlightedEscenaId === scene.ID ? 'bg-green-50' : 'hover:bg-gray-50'"
              >
                <td class="px-4 py-3" :class="highlightedEscenaId === scene.ID ? 'border-l-4 border-green-500' : ''">
                  <p class="font-mono text-xs text-gray-700 truncate max-w-48">{{ scene.SceneName }}</p>
                  <p v-if="highlightedEscenaId === scene.ID" class="text-xs text-green-600 font-medium">
                    ← Vienes de aquí
                  </p>
                  <p v-if="scene.MultibandRefEscenaID" class="text-xs text-gray-400">↗ {{ t('scene.reused_multiband') }}</p>
                </td>
                <td class="px-3 py-3 text-xs text-gray-600 whitespace-nowrap">{{ formatDate(scene.Fecha) }}</td>
                <td class="px-3 py-3 text-center text-xs text-gray-600">
                  {{ scene.CloudCover != null ? scene.CloudCover.toFixed(0) + '%' : '-' }}
                </td>
                <td class="px-3 py-3 text-center">
                  <span class="text-xs px-2 py-0.5 rounded-full" :class="statusColors[scene.Status]">
                    {{ t(`scene.status_${scene.Status}`) }}
                  </span>
                </td>
                <td class="px-3 py-3 text-center text-xs">
                  <span :class="scene.Usable ? 'text-green-600' : 'text-gray-300'">
                    {{ scene.Usable ? '✓' : '–' }}
                  </span>
                </td>
                <td class="px-3 py-3 text-center text-xs">
                  <span :class="scene.TruthTifExists ? 'text-green-600' : 'text-gray-300'">
                    {{ scene.TruthTifExists ? '✓' : '–' }}
                  </span>
                </td>
                <td class="px-3 py-3 text-center text-xs">
                  <span :class="scene.IaExists ? 'text-blue-600' : 'text-gray-300'">
                    {{ scene.IaExists ? '✓' : '–' }}
                  </span>
                </td>
                <td class="px-3 py-3 text-center text-xs font-medium" :class="riesgoColors[scene.LatestIaRiesgoNivel ?? '']">
                  {{ scene.LatestIaRiesgoNivel || '–' }}
                </td>
              </tr>

              <tr v-if="!production.escenas?.length">
                <td colspan="8" class="px-4 py-10 text-center text-gray-400 text-sm">
                  Sin escenas registradas
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Evolución histórica de los índices -->
      <template v-if="activeTab === 'tendencias'">
        <TimelineChart
          :key="`tl-${fasesVersion}`"
          :produccion-id="produccionId"
          @select="goToEscenaById"
        />

        <div class="mt-3">
          <button
            class="text-xs text-gray-500 hover:text-gray-800"
            @click="showFases = !showFases"
          >
            {{ showFases ? '▾' : '▸' }} {{ t('fases.titulo') }}
          </button>

          <!-- Al guardar se remonta la gráfica para que repinte las franjas -->
          <FasesEditor
            v-if="showFases"
            class="mt-2"
            :produccion-id="produccionId"
            @saved="fasesVersion++"
          />
        </div>
      </template>

      <!-- Zona de acciones peligrosas, separada del resto a propósito -->
      <div class="mt-8 rounded-2xl border border-red-200 bg-red-50/40 p-4">
        <h3 class="text-sm font-semibold text-red-800">{{ t('borrado.zona') }}</h3>
        <p class="mt-1 text-xs text-red-700">{{ t('borrado.descripcion') }}</p>
        <!-- Sólo se borra el monitoreo de un lote ya cerrado: bloqueado lo deriva
             un trigger de posible_cosecha, así que equivale a estar cosechado. -->
        <button
          class="mt-3 rounded border border-red-300 bg-white px-3 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40"
          :disabled="!production.Bloqueado || !permStore.puede('monitoreo.eliminar', production.CentroCostoID)"
          :title="!permStore.puede('monitoreo.eliminar', production.CentroCostoID) ? t('permisos.sinPermiso') : undefined"
          @click="showDelete = true; folioConfirm = ''; deleteError = ''"
        >
          {{ t('borrado.boton') }}
        </button>
        <p v-if="!production.Bloqueado" class="mt-1.5 text-xs text-red-700/80">
          {{ t('borrado.requiereBloqueo') }}
        </p>
      </div>
    </template>

    <!-- Confirmación del borrado -->
    <div
      v-if="showDelete && production"
      class="fixed inset-0 z-[2000] flex items-center justify-center bg-black/50 p-4"
      @click.self="showDelete = false"
    >
      <div class="w-full max-w-md rounded-2xl bg-white p-5 shadow-2xl">
        <h3 class="font-semibold text-red-800">{{ t('borrado.tituloDialogo') }}</h3>
        <p class="mt-1 text-sm text-gray-600">{{ production.Folio }} · {{ production.Rancho }}</p>

        <div class="mt-3 rounded-lg bg-gray-50 p-3 text-xs text-gray-700">
          <p class="mb-1 font-medium">{{ t('borrado.seBorra') }}</p>
          <ul class="list-inside list-disc space-y-0.5 text-gray-600">
            <li>{{ t('borrado.itemEscenas', { n: totalScenes }) }}</li>
            <li>{{ t('borrado.itemS3') }}</li>
            <li>{{ t('borrado.itemDynamo') }}</li>
          </ul>
          <p class="mt-2 mb-1 font-medium">{{ t('borrado.seConserva') }}</p>
          <ul class="list-inside list-disc space-y-0.5 text-gray-600">
            <li>{{ t('borrado.itemProduccion') }}</li>
            <li>{{ t('borrado.itemFases') }}</li>
          </ul>
        </div>

        <p class="mt-3 rounded bg-yellow-50 px-2 py-1.5 text-xs text-yellow-800">
          {{ t('borrado.avisoSync') }}
        </p>

        <label class="mt-3 block text-xs text-gray-600">
          {{ t('borrado.escribeFolio', { folio: production.Folio }) }}
          <input
            v-model="folioConfirm"
            type="text"
            autocomplete="off"
            class="mt-1 w-full rounded border border-gray-300 px-2 py-1.5 text-sm font-mono"
          />
        </label>

        <p v-if="deleteError" class="mt-2 rounded bg-red-50 px-2 py-1 text-xs text-red-700">
          {{ deleteError }}
        </p>

        <div class="mt-4 flex justify-end gap-2">
          <button class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
                  @click="showDelete = false">
            {{ t('common.cancel') }}
          </button>
          <button
            class="rounded bg-red-600 px-3 py-1.5 text-sm text-white hover:bg-red-700 disabled:opacity-40"
            :disabled="!folioCoincide || deleting"
            @click="eliminarMonitoreo"
          >{{ deleting ? t('common.loading') : t('borrado.confirmar') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>
