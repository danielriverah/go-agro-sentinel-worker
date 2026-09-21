import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { producciones as prodApi } from '@/api/client'
import type { Production, ProductionDetail } from '@/api/types'

export const useProductionsStore = defineStore('productions', () => {
  const productions = ref<Production[]>([])
  const details     = ref<Record<number, ProductionDetail>>({})
  const loading     = ref(false)
  const loadingDetail = ref<Set<number>>(new Set())

  // Carga la lista completa (dashboard)
  async function loadAll() {
    loading.value = true
    try {
      productions.value = await prodApi.list()
    } finally {
      loading.value = false
    }
  }

  // Carga el detalle de una producción si no está en caché
  async function ensureDetail(id: number): Promise<ProductionDetail | null> {
    if (details.value[id]) return details.value[id]
    return refreshDetail(id)
  }

  // Recarga el detalle forzando un fetch (para SSE updates)
  async function refreshDetail(id: number): Promise<ProductionDetail | null> {
    if (loadingDetail.value.has(id)) return details.value[id] ?? null
    const next = new Set(loadingDetail.value)
    next.add(id)
    loadingDetail.value = next
    try {
      const d = await prodApi.get(id)
      details.value = { ...details.value, [id]: d }
      // Actualizar también el registro en la lista principal
      const idx = productions.value.findIndex(p => p.ID === id)
      const prev = productions.value[idx]
      if (idx !== -1 && prev) {
        const updated = [...productions.value]
        // preserve stats from the list entry — the detail endpoint doesn't include them
        updated[idx] = { ...d, stats: prev.stats ?? null }
        productions.value = updated
      }
      return d
    } catch {
      return details.value[id] ?? null
    } finally {
      const next2 = new Set(loadingDetail.value)
      next2.delete(id)
      loadingDetail.value = next2
    }
  }

  // Actualiza un campo de la lista sin fetch (e.g. tras patch IAuto)
  function patchProduction(id: number, patch: Partial<Production>) {
    const idx = productions.value.findIndex(p => p.ID === id)
    const cur = productions.value[idx]
    if (idx !== -1 && cur) {
      const updated = [...productions.value]
      updated[idx] = { ...cur, ...patch }
      productions.value = updated
    }
    if (details.value[id]) {
      details.value = { ...details.value, [id]: { ...details.value[id], ...patch } }
    }
  }

  function isLoadingDetail(id: number) {
    return computed(() => loadingDetail.value.has(id))
  }

  // Limpia todo al cerrar sesión, para que el siguiente usuario no vea en
  // caché producciones a las que quizá no tenga acceso.
  function reset() {
    productions.value = []
    details.value = {}
    loadingDetail.value = new Set()
    loading.value = false
  }

  return { productions, details, loading, loadAll, ensureDetail, refreshDetail, patchProduction, isLoadingDetail, reset }
})
