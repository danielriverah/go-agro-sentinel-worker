<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { alertas as alertasApi, apiErrorMessage } from '@/api/client'
import { useProductionsStore } from '@/stores/productions'
import type { Alerta, AlertaSeveridad } from '@/api/types'

const { t, locale } = useI18n()
const router = useRouter()
const prodStore = useProductionsStore()

const items = ref<Alerta[]>([])
const loading = ref(true)
const error = ref('')
const noDisponible = ref(false)

// Por omisión se ocultan las resueltas: la vista es una bandeja de pendientes.
const verResueltas = ref(false)
const severidadFiltro = ref<AlertaSeveridad | ''>('')

const colapsados = ref<Set<string>>(new Set())

async function cargar() {
  loading.value = true
  error.value = ''
  try {
    items.value = await alertasApi.list({
      estado: verResueltas.value ? undefined : 'nueva,vista',
      severidad: severidadFiltro.value || undefined,
      limite: 500,
    })
  } catch (e) {
    const err = e as { response?: { status?: number } }
    if (err.response?.status === 503) noDisponible.value = true
    error.value = apiErrorMessage(e)
  } finally {
    loading.value = false
  }
}

// ── Agrupación: rancho → cultivo ─────────────────────────────────────────────

interface GrupoCultivo {
  cultivo: string
  alertas: Alerta[]
  sinLeer: number
}
interface GrupoRancho {
  rancho: string
  cultivos: GrupoCultivo[]
  total: number
  sinLeer: number
  peor: AlertaSeveridad
}

const ordenSeveridad: Record<AlertaSeveridad, number> = { baja: 1, media: 2, alta: 3 }

const grupos = computed<GrupoRancho[]>(() => {
  const porRancho = new Map<string, Map<string, Alerta[]>>()

  for (const a of items.value) {
    const rancho = a.rancho || '—'
    const cultivo = a.cultivo || '—'
    if (!porRancho.has(rancho)) porRancho.set(rancho, new Map())
    const cultivos = porRancho.get(rancho)!
    if (!cultivos.has(cultivo)) cultivos.set(cultivo, [])
    cultivos.get(cultivo)!.push(a)
  }

  const salida: GrupoRancho[] = []
  for (const [rancho, cultivos] of porRancho) {
    const gruposCultivo: GrupoCultivo[] = []
    let total = 0
    let sinLeer = 0
    let peor: AlertaSeveridad = 'baja'

    for (const [cultivo, lista] of cultivos) {
      const nuevas = lista.filter((a) => a.estado === 'nueva').length
      gruposCultivo.push({ cultivo, alertas: lista, sinLeer: nuevas })
      total += lista.length
      sinLeer += nuevas
      for (const a of lista) {
        if (ordenSeveridad[a.severity] > ordenSeveridad[peor]) peor = a.severity
      }
    }

    // Dentro del rancho, primero los cultivos con más pendientes.
    gruposCultivo.sort((a, b) => b.sinLeer - a.sinLeer || a.cultivo.localeCompare(b.cultivo))
    salida.push({ rancho, cultivos: gruposCultivo, total, sinLeer, peor })
  }

  // Los ranchos con avisos sin leer, y los más graves, arriba.
  return salida.sort(
    (a, b) => b.sinLeer - a.sinLeer || ordenSeveridad[b.peor] - ordenSeveridad[a.peor] || a.rancho.localeCompare(b.rancho),
  )
})

const totalSinLeer = computed(() => items.value.filter((a) => a.estado === 'nueva').length)

function toggle(clave: string) {
  const next = new Set(colapsados.value)
  next.has(clave) ? next.delete(clave) : next.add(clave)
  colapsados.value = next
}

// ── Acciones ──────────────────────────────────────────────────────────────────

async function marcarVista(a: Alerta) {
  if (a.estado !== 'nueva') return
  try {
    await alertasApi.marcarVista(a.monitoring_alerta_id)
    a.estado = 'vista'
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

async function resolver(a: Alerta) {
  try {
    await alertasApi.marcarResuelta(a.monitoring_alerta_id)
    a.estado = 'resuelta'
    if (!verResueltas.value) {
      items.value = items.value.filter((x) => x.monitoring_alerta_id !== a.monitoring_alerta_id)
    }
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

// El id que viaja en la alerta es el del ERP; la ruta usa el del monitoreo.
function irAProduccion(a: Alerta) {
  const prod = prodStore.productions.find((p) => p.ProduccionID === a.produccion_id)
  if (prod) router.push({ name: 'produccion', params: { id: prod.ID } })
}

// ── Formato ───────────────────────────────────────────────────────────────────

const expandidas = ref<Set<number>>(new Set())
function toggleDetalle(id: number) {
  const next = new Set(expandidas.value)
  next.has(id) ? next.delete(id) : next.add(id)
  expandidas.value = next
}

function fmtFecha(iso: string): string {
  return new Date(iso).toLocaleDateString(locale.value === 'es' ? 'es-MX' : 'en-US', {
    day: 'numeric', month: 'short', year: 'numeric',
  })
}

const colorSeveridad: Record<AlertaSeveridad, string> = {
  alta: 'bg-red-100 text-red-700',
  media: 'bg-yellow-100 text-yellow-800',
  baja: 'bg-gray-100 text-gray-600',
}

onMounted(() => {
  cargar()
  if (prodStore.productions.length === 0) prodStore.loadAll().catch(() => {})
})
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-6">
    <div class="mb-4 flex flex-wrap items-baseline justify-between gap-2">
      <div>
        <h1 class="text-xl font-semibold text-gray-900">{{ t('alertas.titulo') }}</h1>
        <p class="text-sm text-gray-500">
          {{ t('alertas.subtitulo', { n: totalSinLeer }) }}
        </p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <select
          v-model="severidadFiltro"
          class="rounded border border-gray-300 px-2 py-1 text-xs"
          @change="cargar"
        >
          <option value="">{{ t('alertas.todasSeveridades') }}</option>
          <option value="alta">{{ t('alertas.sev.alta') }}</option>
          <option value="media">{{ t('alertas.sev.media') }}</option>
          <option value="baja">{{ t('alertas.sev.baja') }}</option>
        </select>
        <label class="flex items-center gap-1 text-xs text-gray-600">
          <input v-model="verResueltas" type="checkbox" @change="cargar" />
          {{ t('alertas.verResueltas') }}
        </label>
      </div>
    </div>

    <div v-if="noDisponible" class="rounded bg-yellow-50 px-3 py-2 text-xs text-yellow-800">
      {{ t('alertas.sinTabla') }}
    </div>

    <div v-else-if="loading" class="py-12 text-center text-sm text-gray-500">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="grupos.length === 0" class="rounded-2xl border border-gray-200 bg-white py-12 text-center">
      <p class="text-sm font-medium text-gray-700">{{ t('alertas.vacio') }}</p>
    </div>

    <div v-else class="space-y-3">
      <div
        v-for="g in grupos" :key="g.rancho"
        class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm"
      >
        <!-- Rancho -->
        <button
          class="flex w-full items-center gap-2 px-4 py-3 text-left hover:bg-gray-50"
          @click="toggle(g.rancho)"
        >
          <span class="text-gray-400">{{ colapsados.has(g.rancho) ? '▸' : '▾' }}</span>
          <span class="font-semibold text-gray-800">🏡 {{ g.rancho }}</span>
          <span
            v-if="g.sinLeer > 0"
            class="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700"
          >{{ g.sinLeer }} {{ t('alertas.sinLeer') }}</span>
          <span class="ml-auto text-xs text-gray-400">{{ g.total }}</span>
        </button>

        <div v-if="!colapsados.has(g.rancho)" class="border-t border-gray-100">
          <!-- Cultivo -->
          <div v-for="c in g.cultivos" :key="c.cultivo" class="border-b border-gray-50 last:border-0">
            <button
              class="flex w-full items-center gap-2 bg-gray-50/60 px-5 py-2 text-left hover:bg-gray-100/60"
              @click="toggle(g.rancho + '/' + c.cultivo)"
            >
              <span class="text-gray-400">{{ colapsados.has(g.rancho + '/' + c.cultivo) ? '▸' : '▾' }}</span>
              <span class="text-sm text-gray-700">🌱 {{ c.cultivo }}</span>
              <span class="ml-auto text-xs text-gray-400">{{ c.alertas.length }}</span>
            </button>

            <ul v-if="!colapsados.has(g.rancho + '/' + c.cultivo)" class="divide-y divide-gray-100">
              <li
                v-for="a in c.alertas" :key="a.monitoring_alerta_id"
                class="px-5 py-3"
                :class="a.estado === 'nueva' ? 'bg-blue-50/40' : ''"
              >
                <div class="flex flex-wrap items-start gap-2">
                  <span class="rounded-full px-2 py-0.5 text-xs" :class="colorSeveridad[a.severity]">
                    {{ t(`alertas.sev.${a.severity}`) }}
                  </span>
                  <span v-if="a.estado === 'nueva'" class="rounded-full bg-blue-100 px-2 py-0.5 text-xs text-blue-700">
                    {{ t('alertas.estado.nueva') }}
                  </span>
                  <span v-else-if="a.estado === 'resuelta'" class="rounded-full bg-green-100 px-2 py-0.5 text-xs text-green-700">
                    {{ t('alertas.estado.resuelta') }}
                  </span>

                  <button
                    class="flex-1 text-left text-sm font-medium text-gray-800 hover:underline"
                    @click="toggleDetalle(a.monitoring_alerta_id); marcarVista(a)"
                  >{{ a.title }}</button>

                  <span class="text-xs text-gray-400">{{ fmtFecha(a.created_at) }}</span>
                </div>

                <p class="mt-1 text-xs text-gray-500">
                  {{ a.folio }}<span v-if="a.scene_name"> · {{ a.scene_name }}</span>
                </p>

                <!-- Detalle -->
                <div v-if="expandidas.has(a.monitoring_alerta_id)" class="mt-2 space-y-2">
                  <p class="whitespace-pre-line rounded bg-gray-50 px-3 py-2 text-xs text-gray-700">
                    {{ a.message }}
                  </p>
                  <div v-if="a.action_suggested" class="rounded bg-green-50 px-3 py-2 text-xs text-green-900">
                    <span class="font-medium">{{ t('alertas.accion') }}:</span> {{ a.action_suggested }}
                  </div>
                  <p v-if="a.seen_by" class="text-[11px] text-gray-400">
                    {{ t('alertas.vistaPor', { u: a.seen_by }) }}
                  </p>
                </div>

                <div class="mt-2 flex flex-wrap gap-2">
                  <button
                    class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50"
                    @click="toggleDetalle(a.monitoring_alerta_id); marcarVista(a)"
                  >
                    {{ expandidas.has(a.monitoring_alerta_id) ? t('alertas.ocultar') : t('alertas.verDetalle') }}
                  </button>
                  <button
                    v-if="a.estado !== 'resuelta'"
                    class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50"
                    @click="resolver(a)"
                  >{{ t('alertas.resolver') }}</button>
                  <button
                    class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50"
                    @click="irAProduccion(a)"
                  >{{ t('alertas.verProduccion') }}</button>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </div>

    <p v-if="error" class="mt-3 rounded bg-red-50 px-3 py-2 text-xs text-red-700">{{ error }}</p>
  </div>
</template>
