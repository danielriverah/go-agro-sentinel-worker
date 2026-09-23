<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import RanchodMultiSelect from './RanchodMultiSelect.vue'
import type { Rol, CentroCostoItem, AsignacionRol } from '@/api/types'

interface Props {
  open: boolean
  roles: Rol[]
  centros: CentroCostoItem[]
  loading?: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'assign', selectedRolId: number, selectedCentroIds: number[]): void
}

defineProps<Props>()
defineEmits<Emits>()

const { t } = useI18n()
const selectedRolId = ref<number | null>(null)
const selectedCentroIds = ref<number[]>([])

const selectedRol = computed(() =>
  props.roles.find(r => r.rol_id === selectedRolId.value)
)

function handleClose() {
  resetForm()
  defineEmits<Emits>()[0]('close')
}

function handleAssign() {
  if (!selectedRolId.value || selectedCentroIds.value.length === 0) return
  defineEmits<Emits>()[1]('assign', selectedRolId.value, selectedCentroIds.value)
  resetForm()
}

function resetForm() {
  selectedRolId.value = null
  selectedCentroIds.value = []
}
</script>

<template>
  <div v-if="open" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
    <div class="bg-white rounded-xl shadow-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto">
      <!-- Header -->
      <div class="sticky top-0 border-b border-gray-200 bg-white px-6 py-4 flex items-center justify-between">
        <h2 class="text-lg font-semibold text-gray-800">
          {{ t('permisos.asignarRolRanchos') || 'Asignar Rol a Múltiples Ranchos' }}
        </h2>
        <button
          @click="handleClose"
          class="text-gray-400 hover:text-gray-600 transition-colors"
        >
          <svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <!-- Content -->
      <div class="p-6 space-y-6">
        <!-- Step 1: Seleccionar Rol -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-3">
            {{ t('permisos.paso1SeleccionarRol') || 'Paso 1: Selecciona el rol' }}
          </label>
          <select
            v-model.number="selectedRolId"
            class="w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-green-500 focus:outline-none focus:ring-2 focus:ring-green-100"
          >
            <option :value="null">-- {{ t('common.select') || 'Seleccionar' }} --</option>
            <option v-for="rol in roles" :key="rol.rol_id" :value="rol.rol_id">
              {{ rol.nombre }}
              <span v-if="rol.es_sistema" class="ml-2 text-xs text-gray-400">🔒</span>
            </option>
          </select>
        </div>

        <!-- Step 2: Seleccionar Ranchos -->
        <div v-if="selectedRol" class="space-y-3">
          <label class="block text-sm font-medium text-gray-700">
            {{ t('permisos.paso2SeleccionarRanchos') || 'Paso 2: Selecciona los ranchos' }}
          </label>
          <p class="text-xs text-gray-500">
            Asignando <span class="font-semibold text-gray-700">{{ selectedRol.nombre }}</span> a:
          </p>
          <RanchodMultiSelect
            :centros="centros"
            :selected-ids="selectedCentroIds"
            @update:selected-ids="selectedCentroIds = $event"
          />
        </div>

        <!-- Resumen -->
        <div v-if="selectedRol && selectedCentroIds.length > 0" class="bg-blue-50 border border-blue-200 rounded-lg p-4">
          <p class="text-sm text-blue-900">
            <span class="font-semibold">Resumen:</span> Se asignará el rol
            <span class="font-semibold text-blue-700">{{ selectedRol.nombre }}</span>
            a <span class="font-semibold text-blue-700">{{ selectedCentroIds.length }}</span>
            rancho{{ selectedCentroIds.length !== 1 ? 's' : '' }}.
          </p>
        </div>
      </div>

      <!-- Footer -->
      <div class="sticky bottom-0 border-t border-gray-200 bg-white px-6 py-4 flex justify-end gap-2">
        <button
          @click="handleClose"
          class="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors"
        >
          {{ t('common.cancel') || 'Cancelar' }}
        </button>
        <button
          @click="handleAssign"
          :disabled="!selectedRolId || selectedCentroIds.length === 0 || loading"
          class="rounded-lg bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {{ loading ? t('common.loading') : t('permisos.asignar') || 'Asignar' }}
        </button>
      </div>
    </div>
  </div>
</template>
