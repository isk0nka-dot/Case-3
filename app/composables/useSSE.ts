// =============================================================================
// Argus AI — SSE Composable for Real-Time Session Monitoring
// =============================================================================
//
// Provides a reactive composable for connecting to the backend SSE stream
// endpoint. Replaces polling with push-based real-time updates.
//
// Features:
//   - Live violation events as they arrive
//   - Continuous integrity score updates (every 3s from backend)
//   - Auto-terminate detection
//   - Exponential backoff reconnection
//   - Automatic cleanup on unmount
//
// Usage:
//   const { events, score, verdict, terminated, connected } = useSessionStream(sessionId)
// =============================================================================

import { useAdminAPI } from '~/composables/useAdminAPI'

// ---------------------------------------------------------------------------
// SSE Event Types (mirrors backend SSE payloads)
// ---------------------------------------------------------------------------

export interface SSEViolation {
  eventType: string
  severity: string
  label: string
  confidence: number
  source: string
  timestamp: string
}

export interface SSEScoreUpdate {
  score: number
  verdict: 'clean' | 'warning' | 'fraud'
  verdictLabel: string
  totalEvents: number
}

export interface SSETerminate {
  reason: string
  score: number
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useSessionStream(sessionId: Ref<string> | string) {
  const api = useAdminAPI()

  // Reactive state
  const events = ref<SSEViolation[]>([])
  const score = ref<number>(100)
  const verdict = ref<'clean' | 'warning' | 'fraud'>('clean')
  const verdictLabel = ref<string>('Чисто')
  const totalEvents = ref<number>(0)
  const terminated = ref(false)
  const terminateReason = ref('')
  const connected = ref(false)
  const error = ref<string | null>(null)

  // Internal
  let eventSource: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectAttempts = 0
  const MAX_RECONNECT_ATTEMPTS = 10
  const BASE_RECONNECT_DELAY_MS = 1000

  // Resolve sessionId to string
  const resolvedId = computed(() => typeof sessionId === 'string' ? sessionId : sessionId.value)

  function connect() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }

    const id = resolvedId.value
    if (!id) return

    const url = api.getSessionStreamUrl(id)
    error.value = null

    try {
      eventSource = new EventSource(url)
    } catch {
      error.value = 'Failed to create EventSource'
      scheduleReconnect()
      return
    }

    eventSource.onopen = () => {
      connected.value = true
      reconnectAttempts = 0
      error.value = null
    }

    eventSource.onerror = () => {
      connected.value = false
      eventSource?.close()
      eventSource = null

      if (!terminated.value) {
        scheduleReconnect()
      }
    }

    // --- Event: violation ---
    eventSource.addEventListener('violation', (e: MessageEvent) => {
      try {
        const violation: SSEViolation = JSON.parse(e.data)
        events.value.unshift(violation) // newest first

        // Keep max 200 events in memory
        if (events.value.length > 200) {
          events.value = events.value.slice(0, 200)
        }
      } catch {
        // Ignore malformed JSON
      }
    })

    // --- Event: score_update ---
    eventSource.addEventListener('score_update', (e: MessageEvent) => {
      try {
        const update: SSEScoreUpdate = JSON.parse(e.data)
        score.value = update.score
        verdict.value = update.verdict
        verdictLabel.value = update.verdictLabel
        totalEvents.value = update.totalEvents
      } catch {
        // Ignore malformed JSON
      }
    })

    // --- Event: terminate ---
    eventSource.addEventListener('terminate', (e: MessageEvent) => {
      try {
        const term: SSETerminate = JSON.parse(e.data)
        terminated.value = true
        terminateReason.value = term.reason
        score.value = term.score
        connected.value = false
        eventSource?.close()
        eventSource = null
      } catch {
        // Ignore malformed JSON
      }
    })
  }

  function disconnect() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    connected.value = false
  }

  function reset() {
    disconnect()
    events.value = []
    score.value = 100
    verdict.value = 'clean'
    verdictLabel.value = 'Чисто'
    totalEvents.value = 0
    terminated.value = false
    terminateReason.value = ''
    error.value = null
    reconnectAttempts = 0
  }

  function scheduleReconnect() {
    if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      error.value = 'Превышено количество попыток переподключения'
      return
    }

    const delay = Math.min(
      BASE_RECONNECT_DELAY_MS * Math.pow(2, reconnectAttempts),
      30_000 // max 30s
    )
    reconnectAttempts++

    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, delay)
  }

  // Watch for sessionId changes — reconnect
  watch(resolvedId, (newId, oldId) => {
    if (newId !== oldId) {
      reset()
      if (newId) {
        connect()
      }
    }
  })

  // Auto-connect on mount if sessionId is available
  onMounted(() => {
    if (resolvedId.value) {
      connect()
    }
  })

  // Cleanup on unmount
  onUnmounted(() => {
    disconnect()
  })

  return {
    /** Live violation events (newest first, max 200) */
    events: readonly(events),
    /** Current integrity score (0-100) */
    score: readonly(score),
    /** Current verdict: 'clean' | 'warning' | 'fraud' */
    verdict: readonly(verdict),
    /** Localized verdict label */
    verdictLabel: readonly(verdictLabel),
    /** Total events counted by scorer */
    totalEvents: readonly(totalEvents),
    /** Whether the session was auto-terminated */
    terminated: readonly(terminated),
    /** Termination reason (if terminated) */
    terminateReason: readonly(terminateReason),
    /** Whether the SSE connection is currently open */
    connected: readonly(connected),
    /** Error message (if any) */
    error: readonly(error),
    /** Manually reconnect */
    connect,
    /** Manually disconnect */
    disconnect,
    /** Reset all state and disconnect */
    reset
  }
}
