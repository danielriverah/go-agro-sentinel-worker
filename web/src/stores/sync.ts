import { defineStore } from 'pinia'
import { ref } from 'vue'
import { sync as syncApi } from '@/api/client'
import type { SyncStatus } from '@/api/types'

const POLL_INTERVAL = 15_000 // ms

export const useSyncStore = defineStore('sync', () => {
  const status = ref<SyncStatus | null>(null)
  const triggering = ref(false)
  const triggerMsg = ref('')
  let pollTimer: ReturnType<typeof setInterval> | null = null

  async function refresh() {
    try { status.value = await syncApi.status() } catch { /* silencioso */ }
  }

  function startEvents() {
    if (pollTimer) return
    refresh()
    pollTimer = setInterval(refresh, POLL_INTERVAL)
  }

  function stopEvents() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  }

  async function trigger() {
    if (triggering.value) return
    if (status.value?.running) {
      triggerMsg.value = 'already_running'
      setTimeout(() => (triggerMsg.value = ''), 3000)
      return
    }
    triggering.value = true
    triggerMsg.value = ''
    try {
      await syncApi.trigger()
      triggerMsg.value = 'ok'
      // Poll inmediato para que el usuario vea el estado updated.
      setTimeout(refresh, 1500)
    } catch (err: any) {
      triggerMsg.value = err?.response?.status === 409 ? 'already_running' : 'error'
    } finally {
      triggering.value = false
      setTimeout(() => (triggerMsg.value = ''), 4000)
    }
  }

  return { status, triggering, triggerMsg, refresh, startEvents, stopEvents, trigger }
})
