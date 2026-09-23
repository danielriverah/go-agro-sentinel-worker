import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { permisos as permisosApi } from '@/api/client'
import type { PermisosEfectivos } from '@/api/types'

export const usePermissionsStore = defineStore('permissions', () => {
  const data = ref<PermisosEfectivos | null>(null)
  const loaded = ref(false)

  async function load() {
    try {
      data.value = await permisosApi.mis()
    } catch {
      data.value = { global: [], por_rancho: {}, ranchos_todos: false, degraded: true }
    }
    loaded.value = true
  }

  async function refrescar() {
    try {
      data.value = await permisosApi.refrescar()
    } catch {
      data.value = { global: [], por_rancho: {}, ranchos_todos: false, degraded: true }
    }
    loaded.value = true
  }

  function reset() {
    data.value = null
    loaded.value = false
  }

  const degraded = computed(() => data.value?.degraded ?? true)

  function puede(clave: string, centroCostoId?: number): boolean {
    if (!data.value || data.value.degraded) return true
    if (data.value.global.includes(clave)) return true
    if (centroCostoId != null) {
      const permsRancho = data.value.por_rancho[String(centroCostoId)]
      if (permsRancho?.includes(clave)) return true
    }
    return false
  }

  function ranchosCon(clave: string): number[] | null {
    if (!data.value || data.value.degraded) return null
    if (data.value.global.includes(clave)) return null
    const ids: number[] = []
    for (const [ccId, perms] of Object.entries(data.value.por_rancho)) {
      if (perms.includes(clave)) ids.push(Number(ccId))
    }
    return ids
  }

  function tieneAlgunPermiso(modulo: string): boolean {
    if (!data.value || data.value.degraded) return true
    const prefix = modulo + '.'
    if (data.value.global.some((p) => p.startsWith(prefix))) return true
    for (const perms of Object.values(data.value.por_rancho)) {
      if (perms.some((p) => p.startsWith(prefix))) return true
    }
    return false
  }

  return { data, loaded, degraded, load, refrescar, reset, puede, ranchosCon, tieneAlgunPermiso }
})
