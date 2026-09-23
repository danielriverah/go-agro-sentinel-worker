<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { producciones as prodApi, fases as fasesApi, apiErrorMessage } from '@/api/client'
import type { FaseInput, PlantillaFases } from '@/api/types'

const props = defineProps<{ produccionId: number }>()
const emit = defineEmits<{ (e: 'saved'): void }>()

const { t } = useI18n()

const fases = ref<FaseInput[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const okMsg = ref('')
// Se marca cuando el servidor responde 503: la tabla aún no existe.
const noDisponible = ref(false)

// Plantillas: sólo producciones que YA tienen fases. El servidor las filtra,
// así que el selector nunca ofrece una opción que copiaría nada.
const plantillas = ref<PlantillaFases[]>([])
const copiarDe = ref<number | null>(null)

// Fases de la plantilla elegida, para revisarlas antes de aplicarlas y no
// copiar a ciegas.
const vistaPrevia = computed<PlantillaFases | null>(
  () => plantillas.value.find((p) => p.produccion_id === copiarDe.value) ?? null,
)

async function cargar() {
  loading.value = true
  error.value = ''
  try {
    const data = await prodApi.getFases(props.produccionId)
    fases.value = data.map((f) => ({
      nombre: f.nombre,
      dia_inicio: f.dia_inicio,
      dia_fin: f.dia_fin,
    }))
  } catch (e) {
    error.value = apiErrorMessage(e)
  } finally {
    loading.value = false
  }
}

function agregar() {
  // La nueva fase arranca donde terminó la anterior: encadenar es lo habitual
  // y evita solapamientos, que el servidor rechaza.
  const ultima = fases.value[fases.value.length - 1]
  const inicio = ultima ? ultima.dia_fin : 0
  fases.value.push({ nombre: '', dia_inicio: inicio, dia_fin: inicio + 30 })
}

function quitar(i: number) {
  fases.value.splice(i, 1)
}

async function guardar() {
  saving.value = true
  error.value = ''
  okMsg.value = ''
  try {
    await prodApi.putFases(props.produccionId, fases.value)
    okMsg.value = t('fases.guardado')
    emit('saved')
  } catch (e) {
    const err = e as { response?: { status?: number } }
    if (err.response?.status === 503) {
      noDisponible.value = true
    }
    error.value = apiErrorMessage(e)
  } finally {
    saving.value = false
  }
}

// Copiar sólo traslada lo que ya se está viendo en la vista previa; el
// guardado sigue siendo un paso aparte para poder ajustar antes.
function copiar() {
  const origen = vistaPrevia.value
  if (!origen || origen.fases.length === 0) return

  fases.value = origen.fases.map((f) => ({
    nombre: f.nombre,
    dia_inicio: f.dia_inicio,
    dia_fin: f.dia_fin,
  }))
  okMsg.value = t('fases.copiadas', { n: origen.fases.length })
}

async function cargarPlantillas() {
  try {
    const todas = await fasesApi.plantillas()
    // Excluir la propia producción: copiarse a sí misma no aporta nada.
    plantillas.value = todas.filter((p) => p.fases.length > 0)
  } catch {
    // Sin plantillas el editor sigue siendo usable a mano.
    plantillas.value = []
  }
}

onMounted(() => {
  cargar()
  cargarPlantillas()
})
</script>

<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <div class="mb-3 flex items-center justify-between">
      <h3 class="text-sm font-semibold text-gray-800">{{ t('fases.titulo') }}</h3>
      <span class="text-xs text-gray-400">{{ t('fases.ayuda') }}</span>
    </div>

    <div v-if="noDisponible" class="rounded bg-yellow-50 px-3 py-2 text-xs text-yellow-800">
      {{ t('fases.sinTabla') }}
    </div>

    <div v-else-if="loading" class="py-6 text-center text-sm text-gray-500">
      {{ t('common.loading') }}
    </div>

    <template v-else>
      <!-- Copiar de otra producción: sólo se listan las que ya tienen fases -->
      <div class="mb-3 rounded bg-gray-50 px-2 py-2">
        <div v-if="plantillas.length === 0" class="text-xs text-gray-500">
          {{ t('fases.sinPlantillas') }}
        </div>

        <template v-else>
          <div class="flex flex-wrap items-center gap-2">
            <label class="text-xs text-gray-600">{{ t('fases.copiarDe') }}</label>
            <select v-model="copiarDe" class="max-w-64 rounded border border-gray-300 px-1.5 py-1 text-xs">
              <option :value="null">—</option>
              <option v-for="p in plantillas" :key="p.produccion_id" :value="p.produccion_id">
                {{ p.cultivo || '—' }} · {{ p.folio }} ({{ p.fases.length }})
              </option>
            </select>
            <button
              class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-white disabled:opacity-40"
              :disabled="!vistaPrevia"
              @click="copiar"
            >{{ t('fases.copiar') }}</button>
          </div>

          <!-- Vista previa: qué fases se van a copiar exactamente -->
          <div v-if="vistaPrevia" class="mt-2 rounded border border-gray-200 bg-white px-2 py-1.5">
            <p class="mb-1 text-[11px] text-gray-500">
              {{ t('fases.vistaPrevia') }} · {{ vistaPrevia.rancho }}
            </p>
            <ul class="space-y-0.5">
              <li
                v-for="f in vistaPrevia.fases" :key="f.id"
                class="flex justify-between text-xs text-gray-700"
              >
                <span>{{ f.nombre }}</span>
                <span class="tabular-nums text-gray-500">{{ f.dia_inicio }} – {{ f.dia_fin }}</span>
              </li>
            </ul>
          </div>
        </template>
      </div>

      <!-- Tabla editable -->
      <div v-if="fases.length === 0" class="py-4 text-center text-xs text-gray-500">
        {{ t('fases.vacio') }}
      </div>

      <table v-else class="w-full text-xs">
        <thead>
          <tr class="text-gray-400">
            <th class="pb-1 text-left font-medium">{{ t('fases.nombre') }}</th>
            <th class="pb-1 text-left font-medium w-20">{{ t('fases.diaInicio') }}</th>
            <th class="pb-1 text-left font-medium w-20">{{ t('fases.diaFin') }}</th>
            <th class="w-8"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(f, i) in fases" :key="i">
            <td class="py-1 pr-2">
              <input v-model="f.nombre" type="text" :placeholder="t('fases.nombre')"
                     class="w-full rounded border border-gray-300 px-1.5 py-1" />
            </td>
            <td class="py-1 pr-2">
              <input v-model.number="f.dia_inicio" type="number" min="0"
                     class="w-full rounded border border-gray-300 px-1.5 py-1" />
            </td>
            <td class="py-1 pr-2">
              <input v-model.number="f.dia_fin" type="number" min="1"
                     class="w-full rounded border border-gray-300 px-1.5 py-1" />
            </td>
            <td class="py-1">
              <button class="text-gray-400 hover:text-red-600" :title="t('fases.quitar')"
                      @click="quitar(i)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div class="mt-3 flex flex-wrap items-center gap-2">
        <button class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50"
                @click="agregar">
          + {{ t('fases.agregar') }}
        </button>
        <button
          class="rounded bg-green-600 px-3 py-1 text-xs text-white hover:bg-green-700 disabled:opacity-40"
          :disabled="saving"
          @click="guardar"
        >{{ saving ? t('common.loading') : t('common.save') }}</button>

        <span v-if="okMsg" class="text-xs text-green-700">✓ {{ okMsg }}</span>
      </div>
    </template>

    <p v-if="error" class="mt-2 rounded bg-red-50 px-2 py-1 text-xs text-red-700">{{ error }}</p>
  </div>
</template>
