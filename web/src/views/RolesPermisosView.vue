<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { roles as rolesApi, apiErrorMessage } from '@/api/client'
import type { Rol, PermisoCatalogo, UsuarioAdmin, CentroCostoItem, UsuarioAsignaciones, AsignacionRol } from '@/api/types'

const props = defineProps<{ usuarios: UsuarioAdmin[] }>()
const { t } = useI18n()

const catalogo = ref<PermisoCatalogo[]>([])
const rolesList = ref<Rol[]>([])
const centros = ref<CentroCostoItem[]>([])
const loading = ref(true)
const error = ref('')
const okMsg = ref('')

const expandedRol = ref<number | null>(null)
const editingRol = ref<Partial<Rol> | null>(null)
const editPermisoIds = ref<number[]>([])
const saving = ref(false)

const selectedUser = ref<UsuarioAdmin | null>(null)
const userAsignaciones = ref<UsuarioAsignaciones | null>(null)
const loadingUser = ref(false)

const addRolDialog = ref(false)
const addRolId = ref<number | null>(null)
const addRolCcId = ref<number | null>(null)

const addPermDialog = ref(false)
const addPermId = ref<number | null>(null)
const addPermCcId = ref<number | null>(null)

const modulos = computed(() => {
  const mods: Record<string, PermisoCatalogo[]> = {}
  for (const p of catalogo.value) {
    const arr = mods[p.modulo] ?? (mods[p.modulo] = [])
    arr.push(p)
  }
  return mods
})

async function cargar() {
  loading.value = true
  error.value = ''
  try {
    const [r, p, c] = await Promise.all([
      rolesApi.list(),
      rolesApi.permisosCatalogo(),
      rolesApi.centrosCostos(),
    ])
    rolesList.value = r
    catalogo.value = p
    centros.value = c
  } catch (e) {
    error.value = apiErrorMessage(e)
  } finally {
    loading.value = false
  }
}

function toggleRol(rol: Rol) {
  if (expandedRol.value === rol.rol_id) {
    expandedRol.value = null
    editingRol.value = null
    return
  }
  expandedRol.value = rol.rol_id
  editingRol.value = { ...rol }
  editPermisoIds.value = [...rol.permiso_ids]
}

function togglePermiso(id: number) {
  const idx = editPermisoIds.value.indexOf(id)
  if (idx >= 0) editPermisoIds.value.splice(idx, 1)
  else editPermisoIds.value.push(id)
}

async function guardarRol() {
  if (!editingRol.value) return
  saving.value = true
  error.value = ''
  try {
    if (editingRol.value.rol_id) {
      await rolesApi.update(editingRol.value.rol_id, editingRol.value.nombre ?? '', editingRol.value.descripcion ?? '', editPermisoIds.value)
    } else {
      await rolesApi.create(editingRol.value.nombre ?? '', editingRol.value.descripcion ?? '', editPermisoIds.value)
    }
    okMsg.value = editingRol.value.rol_id ? t('permisos.editarRol') : t('permisos.crearRol')
    expandedRol.value = null
    editingRol.value = null
    await cargar()
  } catch (e) {
    error.value = apiErrorMessage(e)
  } finally {
    saving.value = false
  }
}

async function eliminarRol(rol: Rol) {
  if (!confirm(t('permisos.confirmarEliminar', { nombre: rol.nombre }))) return
  error.value = ''
  try {
    await rolesApi.delete(rol.rol_id)
    await cargar()
    okMsg.value = t('permisos.eliminarRol')
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

function crearNuevo() {
  editingRol.value = { nombre: '', descripcion: '', es_sistema: false, permiso_ids: [] }
  editPermisoIds.value = []
  expandedRol.value = -1
}

async function seleccionarUsuario(u: UsuarioAdmin) {
  selectedUser.value = u
  loadingUser.value = true
  error.value = ''
  try {
    userAsignaciones.value = await rolesApi.getUsuarioPermisos(u.user_id)
  } catch (e) {
    error.value = apiErrorMessage(e)
  } finally {
    loadingUser.value = false
  }
}

function nombreCentro(ccId: number | null): string {
  if (ccId == null) return t('permisos.todosRanchos')
  return centros.value.find(c => c.centro_costo_id === ccId)?.nombre ?? `#${ccId}`
}

async function agregarRolUsuario() {
  if (!selectedUser.value || addRolId.value == null) return
  const existentes: AsignacionRol[] = (userAsignaciones.value?.roles ?? []).map(r => ({
    rol_id: r.rol_id,
    centro_costo_id: r.centro_costo_id,
  }))
  existentes.push({ rol_id: addRolId.value, centro_costo_id: addRolCcId.value })
  error.value = ''
  try {
    await rolesApi.setUsuarioRoles(selectedUser.value.user_id, existentes)
    addRolDialog.value = false
    addRolId.value = null
    addRolCcId.value = null
    await seleccionarUsuario(selectedUser.value)
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

async function quitarRol(index: number) {
  if (!selectedUser.value || !userAsignaciones.value) return
  const existentes: AsignacionRol[] = userAsignaciones.value.roles
    .filter((_, i) => i !== index)
    .map(r => ({ rol_id: r.rol_id, centro_costo_id: r.centro_costo_id }))
  error.value = ''
  try {
    await rolesApi.setUsuarioRoles(selectedUser.value.user_id, existentes)
    await seleccionarUsuario(selectedUser.value)
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

async function agregarPermisoDirecto() {
  if (!selectedUser.value || addPermId.value == null) return
  const existentes = (userAsignaciones.value?.directos ?? []).map(d => ({
    permiso_id: d.permiso_id,
    centro_costo_id: d.centro_costo_id,
  }))
  existentes.push({ permiso_id: addPermId.value, centro_costo_id: addPermCcId.value })
  error.value = ''
  try {
    await rolesApi.setUsuarioPermisos(selectedUser.value.user_id, existentes)
    addPermDialog.value = false
    addPermId.value = null
    addPermCcId.value = null
    await seleccionarUsuario(selectedUser.value)
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

async function quitarPermiso(index: number) {
  if (!selectedUser.value || !userAsignaciones.value) return
  const existentes = userAsignaciones.value.directos
    .filter((_, i) => i !== index)
    .map(d => ({ permiso_id: d.permiso_id, centro_costo_id: d.centro_costo_id }))
  error.value = ''
  try {
    await rolesApi.setUsuarioPermisos(selectedUser.value.user_id, existentes)
    await seleccionarUsuario(selectedUser.value)
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

onMounted(cargar)
</script>

<template>
  <div v-if="loading" class="py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>

  <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-4">
    <!-- Left: Roles -->
    <div class="rounded-xl border border-gray-200 bg-white">
      <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3">
        <h3 class="font-semibold text-gray-800">{{ t('permisos.roles') }}</h3>
        <button class="rounded bg-green-600 px-3 py-1 text-xs text-white hover:bg-green-700" @click="crearNuevo">
          + {{ t('permisos.crearRol') }}
        </button>
      </div>

      <div class="divide-y divide-gray-100">
        <!-- Nuevo rol -->
        <div v-if="expandedRol === -1" class="p-4 bg-green-50/50">
          <input v-model="editingRol!.nombre" :placeholder="t('permisos.nombre')"
            class="mb-2 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm" />
          <input v-model="editingRol!.descripcion" :placeholder="t('permisos.descripcion')"
            class="mb-3 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm" />
          <div v-for="(perms, mod) in modulos" :key="mod" class="mb-2">
            <p class="text-xs font-medium text-gray-500 uppercase mb-1">{{ t(`permisos.modulos.${mod}`) }}</p>
            <label v-for="p in perms" :key="p.permiso_id" class="flex items-center gap-2 text-sm py-0.5">
              <input type="checkbox" :checked="editPermisoIds.includes(p.permiso_id)" @change="togglePermiso(p.permiso_id)" />
              <span>{{ p.clave }}</span>
              <span class="text-xs text-gray-400">{{ p.descripcion }}</span>
            </label>
          </div>
          <div class="flex gap-2 mt-3">
            <button class="rounded bg-green-600 px-3 py-1.5 text-xs text-white hover:bg-green-700 disabled:opacity-40"
              :disabled="saving || !editingRol?.nombre?.trim()" @click="guardarRol">
              {{ t('permisos.guardar') }}
            </button>
            <button class="rounded border border-gray-300 px-3 py-1.5 text-xs hover:bg-gray-50"
              @click="expandedRol = null; editingRol = null">
              {{ t('common.cancel') }}
            </button>
          </div>
        </div>

        <div v-for="rol in rolesList" :key="rol.rol_id">
          <div class="flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-gray-50" @click="toggleRol(rol)">
            <div>
              <span class="font-medium text-gray-800">{{ rol.nombre }}</span>
              <span v-if="rol.es_sistema" class="ml-2 text-xs text-gray-400" :title="t('permisos.rolSistema')">🔒</span>
              <span class="ml-2 rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500">{{ rol.permiso_ids.length }}</span>
            </div>
            <div class="flex gap-1">
              <button v-if="!rol.es_sistema" class="rounded border border-red-200 px-2 py-0.5 text-xs text-red-600 hover:bg-red-50"
                @click.stop="eliminarRol(rol)">
                {{ t('permisos.eliminarRol') }}
              </button>
              <svg class="w-4 h-4 text-gray-400 transition-transform" :class="{'rotate-180': expandedRol === rol.rol_id}"
                fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
              </svg>
            </div>
          </div>

          <div v-if="expandedRol === rol.rol_id" class="px-4 pb-4 bg-gray-50/50">
            <input v-model="editingRol!.nombre" :placeholder="t('permisos.nombre')" :disabled="rol.es_sistema"
              class="mb-2 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm disabled:bg-gray-100" />
            <input v-model="editingRol!.descripcion" :placeholder="t('permisos.descripcion')" :disabled="rol.es_sistema"
              class="mb-3 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm disabled:bg-gray-100" />
            <div v-for="(perms, mod) in modulos" :key="mod" class="mb-2">
              <p class="text-xs font-medium text-gray-500 uppercase mb-1">{{ t(`permisos.modulos.${mod}`) }}</p>
              <label v-for="p in perms" :key="p.permiso_id" class="flex items-center gap-2 text-sm py-0.5">
                <input type="checkbox" :checked="editPermisoIds.includes(p.permiso_id)" :disabled="rol.es_sistema"
                  @change="togglePermiso(p.permiso_id)" />
                <span>{{ p.clave }}</span>
                <span class="text-xs text-gray-400">{{ p.descripcion }}</span>
              </label>
            </div>
            <button v-if="!rol.es_sistema"
              class="mt-2 rounded bg-green-600 px-3 py-1.5 text-xs text-white hover:bg-green-700 disabled:opacity-40"
              :disabled="saving" @click="guardarRol">
              {{ t('permisos.guardar') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Right: User Permissions -->
    <div class="rounded-xl border border-gray-200 bg-white">
      <div class="border-b border-gray-100 px-4 py-3">
        <h3 class="font-semibold text-gray-800">{{ t('permisos.usuarios') }}</h3>
      </div>

      <div class="flex flex-col sm:flex-row">
        <!-- User list -->
        <div class="sm:w-48 border-b sm:border-b-0 sm:border-r border-gray-100">
          <button v-for="u in props.usuarios" :key="u.user_id"
            class="w-full text-left px-3 py-2 text-sm hover:bg-gray-50 transition-colors"
            :class="selectedUser?.user_id === u.user_id ? 'bg-green-50 text-green-700 font-medium' : 'text-gray-700'"
            @click="seleccionarUsuario(u)">
            {{ u.username }}
          </button>
        </div>

        <!-- User detail -->
        <div class="flex-1 p-4">
          <div v-if="!selectedUser" class="py-10 text-center text-sm text-gray-400">
            {{ t('permisos.usuarios') }}
          </div>
          <div v-else-if="loadingUser" class="py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
          <div v-else-if="userAsignaciones">
            <!-- Roles asignados -->
            <div class="mb-4">
              <div class="flex items-center justify-between mb-2">
                <h4 class="text-sm font-medium text-gray-700">{{ t('permisos.roles') }}</h4>
                <button class="rounded bg-green-600 px-2 py-0.5 text-xs text-white hover:bg-green-700"
                  @click="addRolDialog = true; addRolId = null; addRolCcId = null">
                  + {{ t('permisos.agregarRol') }}
                </button>
              </div>
              <div class="flex flex-wrap gap-1.5">
                <span v-for="(r, i) in userAsignaciones.roles" :key="r.usuario_rol_id"
                  class="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2.5 py-1 text-xs text-blue-700">
                  {{ r.rol_nombre }}
                  <span class="text-blue-400">@ {{ nombreCentro(r.centro_costo_id) }}</span>
                  <button class="ml-0.5 text-blue-400 hover:text-red-500" @click="quitarRol(i)">&times;</button>
                </span>
                <span v-if="userAsignaciones.roles.length === 0" class="text-xs text-gray-400">—</span>
              </div>
            </div>

            <!-- Permisos directos -->
            <div>
              <div class="flex items-center justify-between mb-2">
                <h4 class="text-sm font-medium text-gray-700">{{ t('permisos.permisoDirecto') }}</h4>
                <button class="rounded bg-green-600 px-2 py-0.5 text-xs text-white hover:bg-green-700"
                  @click="addPermDialog = true; addPermId = null; addPermCcId = null">
                  +
                </button>
              </div>
              <div class="flex flex-wrap gap-1.5">
                <span v-for="(d, i) in userAsignaciones.directos" :key="d.usuario_permiso_id"
                  class="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2.5 py-1 text-xs text-amber-700">
                  {{ d.permiso_clave }}
                  <span class="text-amber-400">@ {{ nombreCentro(d.centro_costo_id) }}</span>
                  <button class="ml-0.5 text-amber-400 hover:text-red-500" @click="quitarPermiso(i)">&times;</button>
                </span>
                <span v-if="userAsignaciones.directos.length === 0" class="text-xs text-gray-400">—</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- Add role dialog -->
  <div v-if="addRolDialog" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="addRolDialog = false">
    <div class="w-full max-w-sm rounded-2xl bg-white p-5 shadow-2xl">
      <h3 class="mb-3 font-semibold text-gray-900">{{ t('permisos.agregarRol') }}</h3>
      <label class="block mb-2 text-sm text-gray-600">
        {{ t('permisos.roles') }}
        <select v-model="addRolId" class="mt-1 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm">
          <option :value="null" disabled>—</option>
          <option v-for="r in rolesList" :key="r.rol_id" :value="r.rol_id">{{ r.nombre }}</option>
        </select>
      </label>
      <label class="block mb-3 text-sm text-gray-600">
        {{ t('permisos.seleccionarRancho') }}
        <select v-model="addRolCcId" class="mt-1 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm">
          <option :value="null">{{ t('permisos.todosRanchos') }}</option>
          <option v-for="c in centros" :key="c.centro_costo_id" :value="c.centro_costo_id">{{ c.nombre }}</option>
        </select>
      </label>
      <div class="flex justify-end gap-2">
        <button class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50" @click="addRolDialog = false">{{ t('common.cancel') }}</button>
        <button class="rounded bg-green-600 px-3 py-1.5 text-sm text-white hover:bg-green-700 disabled:opacity-40"
          :disabled="addRolId == null" @click="agregarRolUsuario">{{ t('permisos.guardar') }}</button>
      </div>
    </div>
  </div>

  <!-- Add direct permission dialog -->
  <div v-if="addPermDialog" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="addPermDialog = false">
    <div class="w-full max-w-sm rounded-2xl bg-white p-5 shadow-2xl">
      <h3 class="mb-3 font-semibold text-gray-900">{{ t('permisos.permisoDirecto') }}</h3>
      <label class="block mb-2 text-sm text-gray-600">
        {{ t('permisos.permisos') }}
        <select v-model="addPermId" class="mt-1 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm">
          <option :value="null" disabled>—</option>
          <option v-for="p in catalogo" :key="p.permiso_id" :value="p.permiso_id">{{ p.clave }} — {{ p.descripcion }}</option>
        </select>
      </label>
      <label class="block mb-3 text-sm text-gray-600">
        {{ t('permisos.seleccionarRancho') }}
        <select v-model="addPermCcId" class="mt-1 w-full rounded border border-gray-300 px-2.5 py-1.5 text-sm">
          <option :value="null">{{ t('permisos.todosRanchos') }}</option>
          <option v-for="c in centros" :key="c.centro_costo_id" :value="c.centro_costo_id">{{ c.nombre }}</option>
        </select>
      </label>
      <div class="flex justify-end gap-2">
        <button class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50" @click="addPermDialog = false">{{ t('common.cancel') }}</button>
        <button class="rounded bg-green-600 px-3 py-1.5 text-sm text-white hover:bg-green-700 disabled:opacity-40"
          :disabled="addPermId == null" @click="agregarPermisoDirecto">{{ t('permisos.guardar') }}</button>
      </div>
    </div>
  </div>

  <p v-if="error" class="mt-3 rounded bg-red-50 px-3 py-2 text-xs text-red-700">{{ error }}</p>
  <p v-if="okMsg" class="mt-3 rounded bg-green-50 px-3 py-2 text-xs text-green-700">{{ okMsg }}</p>
</template>
