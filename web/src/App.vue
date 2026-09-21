<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useWorkerStore } from '@/stores/worker'
import { useSyncStore } from '@/stores/sync'
import { useProductionsStore } from '@/stores/productions'
import { worker as workerApi } from '@/api/client'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const workerStore = useWorkerStore()
const syncStore = useSyncStore()
const prodStore = useProductionsStore()

const showWorkerPanel = ref(false)
const userMenuOpen = ref(false)

const isPublic = computed(() => !!route.meta?.public)
const globalStatus = computed(() => workerStore.status.global)
const isProcessing = computed(() => globalStatus.value?.phase === 'processing')

// Arrancar y detener con la sesión, no en el montaje: App.vue se monta una sola
// vez, así que al entrar por /login sin sesión el onMounted no encontraba nada
// que arrancar y la vista quedaba sin datos ni actualizaciones hasta recargar.
// immediate cubre también la carga con sesión ya activa.
watch(
  () => auth.isAuthenticated,
  (authed) => {
    if (authed) {
      workerStore.startPolling()
      syncStore.startEvents()
      prodStore.loadAll()
    } else {
      workerStore.stopPolling()
      syncStore.stopEvents()
      prodStore.reset() // no dejar datos de la sesión anterior en memoria
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  workerStore.stopPolling()
  syncStore.stopEvents()
})

// SSE: cuando el worker completa una escena, refresca esa producción en el store.
// current_produccion_id es el ERP ProduccionID; hay que mapearlo al monitoring ID.
watch(
  () => workerStore.status.global?.scenes_done,
  (done, prev) => {
    if (done !== undefined && prev !== undefined && done > prev) {
      const erpId = workerStore.status.global?.current_produccion_id
      if (erpId) {
        const prod = prodStore.productions.find(p => p.ProduccionID === erpId)
        if (prod) prodStore.refreshDetail(prod.ID)
      }
    }
  }
)
watch(
  () => workerStore.status.productions,
  (prods) => {
    for (const p of prods ?? []) {
      if (p.phase === 'processing' && p.current_produccion_id) {
        // Monitoreado: refrescar cuando cambia scenes_done
      }
    }
  }
)
// Cuando el worker termina (fase → sleeping), refresca la producción que acabó
watch(
  () => workerStore.status.global?.phase,
  (phase, prev) => {
    if (prev === 'processing' && phase !== 'processing') {
      prodStore.loadAll()
    }
  }
)
// Cuando el sync termina, recargar lista completa
watch(
  () => syncStore.status?.running,
  (running, prev) => {
    if (prev === true && running === false) prodStore.loadAll()
  }
)

function toggleLocale() {
  locale.value = locale.value === 'es' ? 'en' : 'es'
  localStorage.setItem('agro_locale', locale.value)
}

async function logout() {
  auth.logout()
  workerStore.stopPolling()
  router.push('/login')
}

function formatDate(iso: string | null | undefined): string {
  if (!iso) return t('common.never')
  return new Date(iso).toLocaleString(locale.value === 'es' ? 'es-MX' : 'en-US', {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

async function cancelWorker() {
  try { await workerApi.cancel() } catch { /* ignore */ }
  await workerStore.refresh()
}
async function unlockWorker() {
  try { await workerApi.unlock() } catch { /* ignore */ }
  await workerStore.refresh()
}
</script>

<template>
  <!-- Layout autenticado -->
  <template v-if="!isPublic && auth.isAuthenticated">
    <!-- Navbar -->
    <nav class="fixed top-0 inset-x-0 z-30 bg-white border-b border-gray-200 h-14 flex items-center px-4 gap-4">
      <!-- Logo -->
      <RouterLink to="/" class="flex items-center gap-2 font-bold text-green-700 shrink-0">
        <span class="text-xl">🌱</span>
        <span class="hidden sm:block text-sm">Agro Sentinel</span>
      </RouterLink>

      <!-- Navegación principal -->
      <div class="flex items-center gap-1 shrink-0 ml-2">
        <RouterLink
          to="/"
          class="text-xs px-2.5 py-1 rounded-lg transition-colors"
          :class="route.name === 'dashboard'
            ? 'bg-green-50 text-green-700 font-medium'
            : 'text-gray-500 hover:text-gray-800 hover:bg-gray-50'"
        >Dashboard</RouterLink>
        <RouterLink
          to="/producciones"
          class="text-xs px-2.5 py-1 rounded-lg transition-colors"
          :class="route.name === 'producciones' || route.name === 'produccion' || route.name === 'escena'
            ? 'bg-green-50 text-green-700 font-medium'
            : 'text-gray-500 hover:text-gray-800 hover:bg-gray-50'"
        >Producciones</RouterLink>
      </div>

      <div class="flex-1" />

      <!-- Worker status badge -->
      <button
        @click="showWorkerPanel = !showWorkerPanel"
        class="flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full border transition-colors"
        :class="isProcessing
          ? 'border-green-300 bg-green-50 text-green-700'
          : 'border-gray-200 bg-gray-50 text-gray-500'"
      >
        <span
          class="w-2 h-2 rounded-full"
          :class="isProcessing ? 'bg-green-500 animate-pulse' : 'bg-gray-300'"
        />
        <span>{{ t('worker.title') }}</span>
        <span v-if="isProcessing" class="font-medium">
          {{ globalStatus?.scenes_done }}/{{ globalStatus?.scenes_total }}
        </span>
      </button>

      <!-- Lang toggle -->
      <button @click="toggleLocale" class="text-xs text-gray-400 hover:text-gray-600 shrink-0">
        {{ locale === 'es' ? 'EN' : 'ES' }}
      </button>

      <!-- User menu -->
      <div class="relative shrink-0">
        <button
          @click="userMenuOpen = !userMenuOpen"
          class="flex items-center gap-1.5 text-sm text-gray-700 hover:text-gray-900"
        >
          <span class="w-7 h-7 rounded-full bg-green-100 text-green-700 flex items-center justify-center font-semibold text-xs uppercase">
            {{ auth.username?.charAt(0) }}
          </span>
          <span class="hidden sm:block max-w-24 truncate">{{ auth.username }}</span>
          <svg class="w-3 h-3 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        <div
          v-if="userMenuOpen"
          class="absolute right-0 top-full mt-1 w-36 bg-white border border-gray-200 rounded-lg shadow-lg py-1 z-50"
          @click.stop
        >
          <button
            @click="logout(); userMenuOpen = false"
            class="w-full text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
          >
            {{ t('nav.logout') }}
          </button>
        </div>
      </div>
    </nav>

    <!-- Click outside to close user menu -->
    <div v-if="userMenuOpen" class="fixed inset-0 z-20" @click="userMenuOpen = false" />

    <!-- Worker panel (slide-down from navbar) -->
    <transition name="slide-down">
      <div
        v-if="showWorkerPanel"
        class="fixed top-14 right-4 z-40 w-72 bg-white border border-gray-200 rounded-xl shadow-lg p-4 space-y-3"
      >
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-semibold text-gray-800">{{ t('worker.title') }}</h3>
          <button @click="showWorkerPanel = false" class="text-gray-400 hover:text-gray-600 text-lg leading-none">&times;</button>
        </div>

        <div v-if="!globalStatus" class="text-xs text-gray-400">{{ t('common.loading') }}</div>

        <template v-else>
          <!-- Phase + mode -->
          <div class="flex items-center gap-2">
            <span
              class="w-2 h-2 rounded-full shrink-0"
              :class="isProcessing ? 'bg-green-500 animate-pulse' : 'bg-gray-300'"
            />
            <span class="text-xs font-medium" :class="isProcessing ? 'text-green-700' : 'text-gray-500'">
              {{ isProcessing ? t('worker.phase_processing') : t('worker.phase_sleeping') }}
            </span>
            <span class="text-xs text-gray-400 ml-auto">
              {{ t(`worker.mode_${globalStatus.mode}`, globalStatus.mode) }}
            </span>
          </div>

          <!-- Progress bar -->
          <div v-if="isProcessing && globalStatus.scenes_total > 0">
            <div class="flex justify-between text-xs text-gray-500 mb-1">
              <span>{{ t('worker.progress', { done: globalStatus.scenes_done, total: globalStatus.scenes_total }) }}</span>
              <span>{{ Math.round(globalStatus.scenes_done / globalStatus.scenes_total * 100) }}%</span>
            </div>
            <div class="h-1.5 bg-gray-100 rounded-full overflow-hidden">
              <div
                class="h-full bg-green-500 rounded-full transition-all"
                :style="{ width: `${Math.round(globalStatus.scenes_done / globalStatus.scenes_total * 100)}%` }"
              />
            </div>
            <p v-if="globalStatus.current_scene" class="text-xs text-gray-400 mt-1 truncate">
              {{ globalStatus.current_scene }}
            </p>
          </div>

          <!-- Dates -->
          <div class="space-y-1 text-xs text-gray-500">
            <div class="flex justify-between">
              <span>{{ t('worker.last_run') }}</span>
              <span class="text-gray-700">{{ formatDate(globalStatus.last_completed_at) }}</span>
            </div>
            <div class="flex justify-between">
              <span>{{ t('worker.next_run') }}</span>
              <span class="text-gray-700">{{ formatDate(globalStatus.next_schedule_at) }}</span>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex gap-2 pt-1">
            <button
              v-if="isProcessing"
              @click="cancelWorker"
              class="flex-1 text-xs px-2 py-1.5 rounded-lg border border-red-200 text-red-600 hover:bg-red-50 transition-colors"
            >
              {{ t('worker.cancel') }}
            </button>
            <button
              @click="unlockWorker"
              class="flex-1 text-xs px-2 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 transition-colors"
            >
              {{ t('worker.unlock') }}
            </button>
            <button
              @click="workerStore.refresh()"
              class="text-xs px-2 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 transition-colors"
              title="Refresh"
            >↻</button>
          </div>
        </template>
      </div>
    </transition>

    <!-- Main content -->
    <main class="pt-14 min-h-screen bg-gray-50">
      <RouterView :key="route.fullPath" />
    </main>
  </template>

  <!-- Layout público (login) -->
  <RouterView v-else />
</template>

<style scoped>
.slide-down-enter-active,
.slide-down-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.slide-down-enter-from,
.slide-down-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
