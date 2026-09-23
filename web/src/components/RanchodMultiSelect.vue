<script setup lang="ts">
import { ref, computed } from 'vue'
import type { CentroCostoItem } from '@/api/types'

interface Props {
  centros: CentroCostoItem[]
  selectedIds: number[]
  title?: string
}

interface Emits {
  (e: 'update:selectedIds', ids: number[]): void
}

defineProps<Props>()
defineEmits<Emits>()

const search = ref('')
const selectAll = ref(false)

const filteredCentros = computed(() => {
  if (!search.value.trim()) return props.centros
  const q = search.value.toLowerCase()
  return props.centros.filter(
    c => c.nombre.toLowerCase().includes(q) ||
         (c.ubicacion && c.ubicacion.toLowerCase().includes(q))
  )
})

function toggleRancho(id: number) {
  const newSelected = [...props.selectedIds]
  const idx = newSelected.indexOf(id)
  if (idx >= 0) {
    newSelected.splice(idx, 1)
    selectAll.value = false
  } else {
    newSelected.push(id)
  }
  emit('update:selectedIds', newSelected)
}

function toggleSelectAll() {
  const emit = defineEmits<Emits>()[1]
  if (selectAll.value) {
    emit('update:selectedIds', filteredCentros.value.map(c => c.centro_costo_id))
  } else {
    emit('update:selectedIds', [])
  }
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const isAllSelected = computed(
  () => filteredCentros.value.length > 0 &&
        filteredCentros.value.every(c => props.selectedIds.includes(c.centro_costo_id))
)
</script>

<template>
  <div class="space-y-3">
    <!-- Buscador -->
    <input
      v-model="search"
      type="text"
      placeholder="Buscar por nombre o ubicación..."
      class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-green-500 focus:outline-none focus:ring-2 focus:ring-green-100"
    />

    <!-- Tabla de ranchos -->
    <div class="border border-gray-200 rounded-lg overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="bg-gray-50 border-b border-gray-200">
            <th class="px-3 py-2 text-left w-8">
              <input
                type="checkbox"
                :checked="isAllSelected"
                @change="toggleSelectAll"
                class="rounded cursor-pointer"
              />
            </th>
            <th class="px-3 py-2 text-left font-medium text-gray-700">Rancho</th>
            <th class="px-3 py-2 text-left font-medium text-gray-700">Ubicación</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 max-h-60 overflow-y-auto block">
          <tr
            v-for="rancho in filteredCentros"
            :key="rancho.centro_costo_id"
            class="hover:bg-gray-50 table w-full table-fixed"
          >
            <td class="px-3 py-2.5 w-8">
              <input
                type="checkbox"
                :checked="selectedIds.includes(rancho.centro_costo_id)"
                @change="toggleRancho(rancho.centro_costo_id)"
                class="rounded cursor-pointer"
              />
            </td>
            <td class="px-3 py-2.5 text-gray-800 font-medium">
              {{ rancho.nombre }}
            </td>
            <td class="px-3 py-2.5 text-gray-600 text-xs">
              {{ rancho.ubicacion || '—' }}
            </td>
          </tr>
          <tr v-if="filteredCentros.length === 0" class="table w-full table-fixed">
            <td colspan="3" class="px-3 py-4 text-center text-gray-400 text-xs">
              Sin resultados
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Chips de selección -->
    <div v-if="selectedIds.length > 0" class="flex flex-wrap gap-2">
      <span
        v-for="id in selectedIds"
        :key="id"
        class="inline-flex items-center gap-1 bg-green-100 text-green-700 px-2.5 py-1 rounded-full text-xs font-medium"
      >
        {{ centros.find(c => c.centro_costo_id === id)?.nombre }}
        <button
          @click="toggleRancho(id)"
          class="text-green-600 hover:text-green-800 font-bold"
        >
          ×
        </button>
      </span>
    </div>

    <!-- Contador -->
    <p class="text-xs text-gray-500">
      {{ selectedIds.length }} de {{ centros.length }} ranchos seleccionados
    </p>
  </div>
</template>
