<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { admin, apiErrorMessage } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { usePermissionsStore } from '@/stores/permissions'
import RolesPermisosView from './RolesPermisosView.vue'
import type { UsuarioAdmin } from '@/api/types'

const { t } = useI18n()
const authStore = useAuthStore()
const permStore = usePermissionsStore()

const tab = ref<'usuarios' | 'roles'>('usuarios')

const usuarios = ref<UsuarioAdmin[]>([])
const loading = ref(true)
const error = ref('')
const okMsg = ref('')
// Cierto cuando el backend responde 503: faltan los procedimientos MySQL.
const noDisponible = ref(false)

// Diálogo de restablecer contraseña.
const resetUser = ref<UsuarioAdmin | null>(null)
const nuevaPassword = ref('')
const resetting = ref(false)

async function cargar() {
  loading.value = true
  error.value = ''
  try {
    usuarios.value = await admin.listUsuarios()
  } catch (e) {
    const err = e as { response?: { status?: number } }
    if (err.response?.status === 503) noDisponible.value = true
    error.value = apiErrorMessage(e)
  } finally {
    loading.value = false
  }
}

function esYoMismo(u: UsuarioAdmin): boolean {
  return authStore.username === u.username
}

async function alternarActivo(u: UsuarioAdmin) {
  error.value = ''
  okMsg.value = ''
  const nuevoValor = u.activo !== 1
  try {
    await admin.setActivo(u.user_id, nuevoValor)
    u.activo = nuevoValor ? 1 : 0
    okMsg.value = nuevoValor ? t('config.activado', { u: u.username }) : t('config.desactivado', { u: u.username })
  } catch (e) {
    error.value = apiErrorMessage(e)
  }
}

function abrirReset(u: UsuarioAdmin) {
  resetUser.value = u
  nuevaPassword.value = ''
  error.value = ''
  okMsg.value = ''
}

async function confirmarReset() {
  if (!resetUser.value || nuevaPassword.value.length < 8) return
  resetting.value = true
  error.value = ''
  try {
    await admin.resetPassword(resetUser.value.user_id, nuevaPassword.value)
    okMsg.value = t('config.passwordCambiada', { u: resetUser.value.username })
    resetUser.value = null
    nuevaPassword.value = ''
  } catch (e) {
    error.value = apiErrorMessage(e)
  } finally {
    resetting.value = false
  }
}

onMounted(cargar)
</script>

<template>
  <div class="mx-auto max-w-4xl px-4 py-6">
    <h1 class="mb-1 text-xl font-semibold text-gray-900">{{ t('config.titulo') }}</h1>
    <p class="mb-5 text-sm text-gray-500">{{ t('config.subtitulo') }}</p>

    <!-- Tabs -->
    <div class="mb-4 flex gap-1 border-b border-gray-200">
      <button
        class="px-4 py-2 text-sm font-medium transition-colors"
        :class="tab === 'usuarios' ? 'border-b-2 border-green-600 text-green-700' : 'text-gray-500 hover:text-gray-700'"
        @click="tab = 'usuarios'"
      >{{ t('config.usuarios') }}</button>
      <button
        v-if="permStore.puede('roles.administrar')"
        class="px-4 py-2 text-sm font-medium transition-colors"
        :class="tab === 'roles' ? 'border-b-2 border-green-600 text-green-700' : 'text-gray-500 hover:text-gray-700'"
        @click="tab = 'roles'"
      >{{ t('permisos.titulo') }}</button>
    </div>

    <!-- Roles y Permisos tab -->
    <RolesPermisosView v-if="tab === 'roles'" :usuarios="usuarios" />

    <!-- Usuarios tab -->
    <div v-if="tab === 'usuarios'" class="rounded-2xl border border-gray-200 bg-white shadow-sm">
      <div class="border-b border-gray-100 px-5 py-3">
        <h2 class="font-semibold text-gray-800">{{ t('config.usuarios') }}</h2>
      </div>

      <div v-if="noDisponible" class="m-4 rounded bg-yellow-50 px-3 py-2 text-xs text-yellow-800">
        {{ t('config.sinProcedimientos') }}
      </div>

      <div v-else-if="loading" class="py-10 text-center text-sm text-gray-500">
        {{ t('common.loading') }}
      </div>

      <table v-else-if="usuarios.length > 0" class="w-full text-sm">
        <thead>
          <tr class="bg-gray-50 text-xs uppercase tracking-wide text-gray-400">
            <th class="px-4 py-2.5 text-left font-medium">{{ t('config.usuario') }}</th>
            <th class="px-3 py-2.5 text-left font-medium">{{ t('config.estado') }}</th>
            <th class="px-3 py-2.5 text-left font-medium">{{ t('config.acciones') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100">
          <tr v-for="u in usuarios" :key="u.user_id">
            <td class="px-4 py-2.5">
              <span class="font-medium text-gray-800">{{ u.username }}</span>
              <span v-if="esYoMismo(u)" class="ml-2 text-xs text-gray-400">{{ t('config.tu') }}</span>
            </td>
            <td class="px-3 py-2.5">
              <span
                class="rounded-full px-2 py-0.5 text-xs"
                :class="u.activo === 1 ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'"
              >{{ u.activo === 1 ? t('config.activo') : t('config.inactivo') }}</span>
            </td>
            <td class="px-3 py-2.5">
              <div class="flex flex-wrap gap-2">
                <!-- Desactivarse a uno mismo cierra la propia sesión, así que
                     el botón se deshabilita en la propia fila. -->
                <button
                  class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50 disabled:opacity-40"
                  :disabled="esYoMismo(u) && u.activo === 1"
                  :title="esYoMismo(u) && u.activo === 1 ? t('config.noTeDesactives') : ''"
                  @click="alternarActivo(u)"
                >
                  {{ u.activo === 1 ? t('config.desactivar') : t('config.activar') }}
                </button>
                <button
                  class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50"
                  @click="abrirReset(u)"
                >{{ t('config.cambiarPassword') }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <p v-else class="py-10 text-center text-sm text-gray-400">{{ t('config.sinUsuarios') }}</p>

      <p v-if="error" class="m-4 rounded bg-red-50 px-3 py-2 text-xs text-red-700">{{ error }}</p>
      <p v-if="okMsg" class="m-4 rounded bg-green-50 px-3 py-2 text-xs text-green-700">✓ {{ okMsg }}</p>
    </div>

    <!-- Diálogo de restablecer contraseña -->
    <div
      v-if="tab === 'usuarios' && resetUser"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      @click.self="resetUser = null"
    >
      <div class="w-full max-w-sm rounded-2xl bg-white p-5 shadow-2xl">
        <h3 class="mb-1 font-semibold text-gray-900">{{ t('config.cambiarPassword') }}</h3>
        <p class="mb-3 text-sm text-gray-500">{{ resetUser.username }}</p>

        <input
          v-model="nuevaPassword"
          type="password"
          autocomplete="new-password"
          :placeholder="t('config.nuevaPassword')"
          class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
        />
        <p class="mt-1 text-xs text-gray-400">{{ t('config.minimo8') }}</p>

        <div class="mt-4 flex justify-end gap-2">
          <button class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
                  @click="resetUser = null">
            {{ t('common.cancel') }}
          </button>
          <button
            class="rounded bg-green-600 px-3 py-1.5 text-sm text-white hover:bg-green-700 disabled:opacity-40"
            :disabled="nuevaPassword.length < 8 || resetting"
            @click="confirmarReset"
          >{{ resetting ? t('common.loading') : t('common.save') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>
