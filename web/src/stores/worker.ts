import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { worker as workerApi } from '@/api/client'
import type { WorkerStatusResponse } from '@/api/types'

export const useWorkerStore = defineStore('worker', () => {
  const status = ref<WorkerStatusResponse>({ global: null, productions: null, stop_pending: false })
  let es: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectDelay = 1000 // ms, doubles on each failure up to 30s

  // ERP produccion_ids disparados pero que el worker aún no ha levantado el lock.
  // Permite mostrar "procesando" de inmediato, sin esperar el SSE.
  const pendingTriggers = ref(new Set<number>())
  const pendingTimers = new Map<number, ReturnType<typeof setTimeout>>()

  // Global trigger disparado pero el worker aún no levantó el lock global.
  const pendingGlobalTrigger = ref(false)
  let pendingGlobalTimer: ReturnType<typeof setTimeout> | null = null
  let pendingGlobalPollStop = false

  const isGlobalProcessing = computed(
    () => pendingGlobalTrigger.value || status.value.global?.phase === 'processing',
  )

  // true cuando cualquier producción individual está siendo procesada (SSE o pending)
  const anyProductionProcessing = computed(() =>
    pendingTriggers.value.size > 0 ||
    (status.value.productions ?? []).some(p => p.phase === 'processing'),
  )

  // true cuando no se puede lanzar ninguna producción (global corriendo O alguna individual)
  const blockIndividualTrigger = computed(
    () => isGlobalProcessing.value || anyProductionProcessing.value,
  )

  // true cuando hay una señal de detención activa esperando ser consumida por el worker
  const stopPending = computed(() => status.value.stop_pending ?? false)

  function isProcessingProduction(produccionId: number) {
    // Considera tanto el estado real del SSE como el estado optimista local
    if (pendingTriggers.value.has(produccionId)) return true
    return (
      (status.value.global?.phase === 'processing' &&
        status.value.global.current_produccion_id === produccionId) ||
      (status.value.productions ?? []).some(
        (p) => p.phase === 'processing' && p.current_produccion_id === produccionId,
      )
    )
  }

  // Llámalo justo después de disparar "Procesar todo" para mostrar estado inmediatamente.
  // Se limpia cuando el SSE confirma el lock global real, o en 35s máximo.
  function markGlobalTriggered() {
    pendingGlobalTrigger.value = true
    pendingGlobalPollStop = false
    if (pendingGlobalTimer) clearTimeout(pendingGlobalTimer)
    pendingGlobalTimer = setTimeout(() => {
      pendingGlobalTrigger.value = false
      pendingGlobalTimer = null
    }, 35_000)

    let attempts = 0
    const poll = async () => {
      if (pendingGlobalPollStop || !pendingGlobalTrigger.value) return
      if (++attempts > 12) return
      await refresh()
      if (!pendingGlobalTrigger.value) return  // cleared by refresh
      setTimeout(poll, 3_000)
    }
    setTimeout(poll, 3_000)
  }

  function clearGlobalPending() {
    pendingGlobalPollStop = true
    pendingGlobalTrigger.value = false
    if (pendingGlobalTimer) { clearTimeout(pendingGlobalTimer); pendingGlobalTimer = null }
  }

  // Llámalo justo después de un trigger exitoso para mostrar el estado inmediatamente.
  // Se limpia automáticamente cuando el SSE confirma el lock real, o en 35s máximo.
  function markTriggered(erpProduccionId: number) {
    pendingTriggers.value = new Set(pendingTriggers.value).add(erpProduccionId)

    // Auto-limpiar después de 35s (timeout máximo del worker ticker)
    const t = setTimeout(() => clearPending(erpProduccionId), 35_000)
    const prev = pendingTimers.get(erpProduccionId)
    if (prev) clearTimeout(prev)
    pendingTimers.set(erpProduccionId, t)

    // Polling agresivo: refresca cada 3s hasta que el SSE confirme, máx 12 intentos
    let attempts = 0
    const poll = async () => {
      if (!pendingTriggers.value.has(erpProduccionId)) return
      if (++attempts > 12) return
      await refresh()
      if (isProcessingProduction(erpProduccionId) && !pendingTriggers.value.has(erpProduccionId)) return
      setTimeout(poll, 3_000)
    }
    setTimeout(poll, 3_000)
  }

  function clearPending(erpProduccionId: number) {
    const next = new Set(pendingTriggers.value)
    next.delete(erpProduccionId)
    pendingTriggers.value = next
    const t = pendingTimers.get(erpProduccionId)
    if (t) { clearTimeout(t); pendingTimers.delete(erpProduccionId) }
  }

  // One-shot refresh via REST — used for initial load or manual refresh.
  async function refresh() {
    try {
      const data = await workerApi.status()
      status.value = data
      // Limpiar pendingGlobalTrigger cuando el lock global apareció
      if (pendingGlobalTrigger.value && data.global?.phase === 'processing') {
        clearGlobalPending()
      }
      // Limpiar pendingTriggers cuyo lock ya apareció en el SSE
      for (const id of pendingTriggers.value) {
        const confirmed = (
          (data.global?.phase === 'processing' && data.global.current_produccion_id === id) ||
          (data.productions ?? []).some(p => p.phase === 'processing' && p.current_produccion_id === id)
        )
        if (confirmed) clearPending(id)
      }
    } catch { /* silencioso */ }
  }

  function startPolling() {
    if (es) return // ya conectado
    connect()
  }

  function connect() {
    if (es) { es.close(); es = null }

    const source = new EventSource('/api/v1/worker/events', { withCredentials: true })
    es = source

    source.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as WorkerStatusResponse
        status.value = data
        reconnectDelay = 1000 // reset backoff on success
        // Limpiar pendingGlobalTrigger cuando el lock global apareció en el SSE
        if (pendingGlobalTrigger.value && data.global?.phase === 'processing') {
          clearGlobalPending()
        }
        // Limpiar pendingTriggers cuyo lock ya apareció en el SSE
        for (const id of pendingTriggers.value) {
          const confirmed = (
            (data.global?.phase === 'processing' && data.global.current_produccion_id === id) ||
            (data.productions ?? []).some(p => p.phase === 'processing' && p.current_produccion_id === id)
          )
          if (confirmed) clearPending(id)
        }
      } catch { /* ignore parse errors */ }
    }

    source.onerror = () => {
      // EventSource reconnects automatically for transient errors, but if the
      // connection is closed (readyState CLOSED) we apply our own backoff.
      if (source.readyState === EventSource.CLOSED) {
        source.close()
        es = null
        scheduleReconnect()
      }
    }
  }

  function scheduleReconnect() {
    if (reconnectTimer) return
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      reconnectDelay = Math.min(reconnectDelay * 2, 30_000)
      connect()
    }, reconnectDelay)
  }

  function stopPolling() {
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null }
    if (es) { es.close(); es = null }
  }

  return { status, isGlobalProcessing, anyProductionProcessing, blockIndividualTrigger, stopPending, refresh, startPolling, stopPolling, isProcessingProduction, markTriggered, markGlobalTriggered }
})
