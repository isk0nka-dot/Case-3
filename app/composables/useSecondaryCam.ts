// =============================================================================
// Argus AI — Secondary Camera (Mobile) Composable
// =============================================================================
//
// Manages the complete lifecycle of mobile secondary camera pairing:
//   1. QR code generation & pairing initiation
//   2. Spatial calibration with Golden Angle verification
//   3. Device telemetry polling (battery, thermal, displacement)
//   4. Stream health heartbeat (5s interval)
//   5. Session gating for mandatory/optional modes
//
// State is reactive and auto-syncs with backend via REST polling.
// =============================================================================

import type {
  SidecamPolicy,
  SidecamPairingState,
  SidecamPairingSession,
  SidecamCalibrationStatus,
  SidecamCalibrationFrame,
  SidecamStreamHealth,
  SidecamDeviceDirective,
  SidecamMobileDevice,
  SidecamQRPayload,
} from '~/composables/useAdminAPI'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

interface SecondaryCamState {
  // Pairing
  pairingSession: SidecamPairingSession | null
  qrData: string
  qrPayload: SidecamQRPayload | null

  // UI state
  isLoading: boolean
  error: string
  phase: 'idle' | 'pairing' | 'calibrating' | 'ready' | 'disconnected' | 'failed'

  // Telemetry
  lastDirective: SidecamDeviceDirective | null
  streamHealth: SidecamStreamHealth | null

  // Fix 12: Device token for post-pairing auth
  deviceToken: string
}

export function useSecondaryCam() {
  const api = useAdminAPI()

  const state = reactive<SecondaryCamState>({
    pairingSession: null,
    qrData: '',
    qrPayload: null,
    isLoading: false,
    error: '',
    phase: 'idle',
    lastDirective: null,
    streamHealth: null,
    deviceToken: '',
  })

  // Polling intervals
  let healthPollId: ReturnType<typeof setInterval> | null = null
  let statusPollId: ReturnType<typeof setInterval> | null = null

  // ---------------------------------------------------------------------------
  // Computed
  // ---------------------------------------------------------------------------

  const isConnected = computed(() =>
    state.pairingSession?.state === 'connected' ||
    state.pairingSession?.state === 'calibrating' ||
    state.pairingSession?.state === 'ready'
  )

  const isCalibrated = computed(() =>
    state.pairingSession?.calibration?.passed === true
  )

  const isReady = computed(() =>
    state.pairingSession?.state === 'ready'
  )

  const calibration = computed(() =>
    state.pairingSession?.calibration ?? null
  )

  const deviceInfo = computed(() =>
    state.pairingSession?.deviceInfo ?? null
  )

  const batteryLevel = computed(() =>
    state.pairingSession?.deviceInfo?.batteryLevel ?? 0
  )

  const batteryWarning = computed(() =>
    batteryLevel.value > 0 && batteryLevel.value < 0.20
  )

  const batteryCritical = computed(() =>
    batteryLevel.value > 0 && batteryLevel.value < 0.10
  )

  const thermalState = computed(() =>
    state.pairingSession?.deviceInfo?.thermalState ?? 'nominal'
  )

  const streamQuality = computed(() =>
    state.streamHealth?.quality ?? state.pairingSession?.health?.quality ?? 'critical'
  )

  const handsOnDesk = computed(() =>
    state.pairingSession?.handsOnDesk ?? true
  )

  const displacementAlert = computed(() =>
    state.pairingSession?.displacementAlert ?? false
  )

  const alerts = computed(() =>
    state.lastDirective?.alerts ?? []
  )

  const pairingState = computed((): SidecamPairingState =>
    (state.pairingSession?.state as SidecamPairingState) ?? 'pending'
  )

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  async function initiatePairing(params: {
    sessionId: string
    studentId: string
    examId: string
    orgId: string
    policy: SidecamPolicy
  }) {
    state.isLoading = true
    state.error = ''

    try {
      const result = await api.initiateSidecamPairing(params)
      state.pairingSession = result.session
      state.qrData = result.qrData
      state.qrPayload = result.payload
      state.phase = 'pairing'

      // Start polling for pairing completion
      startStatusPolling(params.sessionId)
    } catch (err: any) {
      state.error = err.message || 'Не удалось инициировать сопряжение'
      state.phase = 'failed'
    } finally {
      state.isLoading = false
    }
  }

  async function validateStart(sessionId: string, policy: SidecamPolicy): Promise<{
    allowed: boolean
    reason?: string
  }> {
    try {
      return await api.validateSidecamStart(sessionId, policy)
    } catch (err: any) {
      return { allowed: false, reason: err.message }
    }
  }

  async function submitCalibrationFrame(sessionId: string, frame: SidecamCalibrationFrame) {
    try {
      const status = await api.submitSidecamCalibration(sessionId, frame, state.deviceToken || undefined)
      if (state.pairingSession) {
        state.pairingSession.calibration = status
        if (status.passed) {
          state.phase = 'ready'
          state.pairingSession.state = 'ready'
          // Start health heartbeat once calibrated
          startHealthPolling(sessionId)
        } else {
          state.phase = 'calibrating'
        }
      }
      return status
    } catch (err: any) {
      state.error = err.message || 'Ошибка калибровки'
      return null
    }
  }

  async function sendTelemetry(sessionId: string, device: SidecamMobileDevice) {
    try {
      const directive = await api.sendSidecamTelemetry(sessionId, device, state.deviceToken || undefined)
      state.lastDirective = directive
      if (state.pairingSession) {
        state.pairingSession.deviceInfo = device
      }
      return directive
    } catch {
      return null
    }
  }

  async function refreshSession(sessionId: string) {
    try {
      const session = await api.getSidecamSession(sessionId)
      state.pairingSession = session
      updatePhase(session.state as SidecamPairingState)
    } catch {
      // Session may not exist yet
    }
  }

  async function cleanup(sessionId: string) {
    stopPolling()
    try {
      await api.cleanupSidecamSession(sessionId)
    } catch {
      // Ignore cleanup errors
    }
    resetState()
  }

  // ---------------------------------------------------------------------------
  // Polling
  // ---------------------------------------------------------------------------

  function startStatusPolling(sessionId: string) {
    stopStatusPolling()
    statusPollId = setInterval(async () => {
      try {
        const session = await api.getSidecamSession(sessionId)
        state.pairingSession = session
        updatePhase(session.state as SidecamPairingState)

        // Fix 12: Capture device token from session (issued at pairing completion)
        if (session.deviceToken && !state.deviceToken) {
          state.deviceToken = session.deviceToken
        }

        // Once connected, switch from status polling to health polling
        if (session.state === 'connected' || session.state === 'calibrating') {
          state.phase = 'calibrating'
        }
        if (session.state === 'ready') {
          stopStatusPolling()
          startHealthPolling(sessionId)
        }
        if (session.state === 'failed') {
          stopStatusPolling()
          state.phase = 'failed'
        }
      } catch {
        // Polling error — session may not exist yet
      }
    }, 3000)
  }

  function stopStatusPolling() {
    if (statusPollId) {
      clearInterval(statusPollId)
      statusPollId = null
    }
  }

  function startHealthPolling(sessionId: string) {
    stopHealthPolling()
    healthPollId = setInterval(async () => {
      try {
        const health = await api.sendSidecamHeartbeat(sessionId, 0, 0, state.deviceToken || undefined)
        state.streamHealth = health

        if (state.pairingSession) {
          state.pairingSession.health = health
        }

        if (!health.connected) {
          state.phase = 'disconnected'
        } else if (state.phase === 'disconnected') {
          state.phase = 'ready'
        }
      } catch {
        // Heartbeat failed — will retry next interval
      }
    }, 5000)
  }

  function stopHealthPolling() {
    if (healthPollId) {
      clearInterval(healthPollId)
      healthPollId = null
    }
  }

  function stopPolling() {
    stopStatusPolling()
    stopHealthPolling()
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  function updatePhase(pState: SidecamPairingState) {
    switch (pState) {
      case 'pending':
        state.phase = 'pairing'
        break
      case 'connected':
      case 'calibrating':
        state.phase = 'calibrating'
        break
      case 'ready':
        state.phase = 'ready'
        break
      case 'disconnected':
        state.phase = 'disconnected'
        break
      case 'failed':
        state.phase = 'failed'
        break
    }
  }

  function resetState() {
    state.pairingSession = null
    state.qrData = ''
    state.qrPayload = null
    state.isLoading = false
    state.error = ''
    state.phase = 'idle'
    state.lastDirective = null
    state.streamHealth = null
    state.deviceToken = ''
  }

  // ---------------------------------------------------------------------------
  // Fix 11: React to backend directives
  // ---------------------------------------------------------------------------

  watch(
    () => state.lastDirective,
    (directive) => {
      if (!directive) return

      // Pause session if backend instructs (e.g., battery critical in mandatory mode)
      if (directive.pauseSession && state.phase !== 'disconnected') {
        state.phase = 'disconnected'
      }

      // Propagate displacement alerts to session state for UI display
      if (state.pairingSession) {
        const hasDisplacement = directive.alerts?.some(a => a.type === 'device_displaced')
        if (hasDisplacement !== undefined) {
          state.pairingSession.displacementAlert = !!hasDisplacement
        }
      }
    },
    { deep: true },
  )

  // Cleanup on unmount
  onUnmounted(() => {
    stopPolling()
  })

  return {
    // State
    state: readonly(state),
    phase: computed(() => state.phase),
    error: computed(() => state.error),
    isLoading: computed(() => state.isLoading),
    qrData: computed(() => state.qrData),
    qrPayload: computed(() => state.qrPayload),

    // Computed
    isConnected,
    isCalibrated,
    isReady,
    calibration,
    deviceInfo,
    batteryLevel,
    batteryWarning,
    batteryCritical,
    thermalState,
    streamQuality,
    handsOnDesk,
    displacementAlert,
    alerts,
    pairingState,
    pairingSession: computed(() => state.pairingSession),

    // Actions
    initiatePairing,
    validateStart,
    submitCalibrationFrame,
    sendTelemetry,
    refreshSession,
    cleanup,
    stopPolling,
  }
}
