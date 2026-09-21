<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { producciones as prodApi, worker as workerApi, apiErrorMessage } from '@/api/client'
import { useNavContext, scrollToElement } from '@/composables/useNavContext'
import { useWorkerStore } from '@/stores/worker'
import { useProductionsStore } from '@/stores/productions'
import type { Scene, SceneStatus } from '@/api/types'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const workerStore = useWorkerStore()
const prodStore = useProductionsStore()

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

async function desbloquear() {
  unlocking.value = true
  try {
    await prodApi.desbloquear(produccionId)
    await loadProduction()
  } catch { /* ignore */ } finally {
    unlocking.value = false
  }
}

function goToEscena(scene: Scene) {
  navSceneIds.value = (production.value?.escenas?.map(s => s.ID) ?? []).reverse()
  router.push({ name: 'escena', params: { produccionId, escenaId: scene.ID } })
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
        class="text-xs px-2.5 py-1 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 hover:border-green-300 hover:text-green-700 transition-colors"
        title="Ajustar el polígono de monitoreo sobre el mapa"
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
              :disabled="unlocking"
              class="text-sm px-3 py-2 rounded-lg border border-red-200 text-red-600 hover:bg-red-50 disabled:opacity-60 transition-colors"
            >
              {{ unlocking ? '...' : t('production.unblock') }}
            </button>
            <!-- Botón Detener / Cancelar detención — visible cuando ESTA producción procesa -->
            <button
              v-if="isProcessing"
              @click="cancelWorker"
              :disabled="cancelling"
              class="text-sm px-4 py-2 rounded-lg border transition-colors flex items-center gap-1.5 disabled:opacity-60"
              :class="workerStore.stopPending
                ? 'border-orange-200 text-orange-600 hover:bg-orange-50'
                : 'border-red-200 text-red-600 hover:bg-red-50'"
              :title="workerStore.stopPending
                ? 'Quitar señal de detención — el worker continuará'
                : 'El worker terminará la escena actual y se detendrá'"
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
              :disabled="triggeringWorker || workerStore.blockIndividualTrigger"
              class="text-sm px-4 py-2 rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-60 transition-colors"
              :title="workerStore.blockIndividualTrigger && !isProcessing ? 'Otra producción en proceso — espera a que termine' : undefined"
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

      <!-- Scenes table -->
      <div class="bg-white border border-gray-200 rounded-2xl shadow-sm overflow-hidden">
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
    </template>
  </div>
</template>
