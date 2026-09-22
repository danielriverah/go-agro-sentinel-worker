import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { auth as authApi, apiErrorMessage } from '@/api/client'
import { usePermissionsStore } from './permissions'

const COOKIE_NAME = 'agro_token'

function setCookie(value: string, ttlHours = 8) {
  const expires = new Date(Date.now() + ttlHours * 3600 * 1000).toUTCString()
  document.cookie = `${COOKIE_NAME}=${value}; expires=${expires}; path=/; SameSite=Strict`
}

function clearCookie() {
  document.cookie = `${COOKIE_NAME}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/`
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('agro_token'))
  const username = ref<string | null>(localStorage.getItem('agro_user'))

  const isAuthenticated = computed(() => !!token.value)

  async function login(u: string, password: string) {
    const res = await authApi.login(u, password)
    token.value = res.token
    username.value = res.username
    localStorage.setItem('agro_token', res.token)
    localStorage.setItem('agro_user', res.username)
    // Cookie needed for EventSource (SSE) which doesn't support custom headers.
    setCookie(res.token)
    usePermissionsStore().load()
  }

  function logout() {
    token.value = null
    username.value = null
    localStorage.removeItem('agro_token')
    localStorage.removeItem('agro_user')
    clearCookie()
    usePermissionsStore().reset()
  }

  return { token, username, isAuthenticated, login, logout }
})

export { apiErrorMessage }
