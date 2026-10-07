// =============================================================================
// Argus AI — useLivenessChallenge Composable (Active Liveness Verification)
// =============================================================================
//
// Issues random challenges to the student at unpredictable intervals:
//   - "Поверните голову влево" (turn head left)
//   - "Моргните дважды" (blink twice)
//   - "Покажите 3 пальца" (show 3 fingers — placeholder, needs hand model)
//   - "Посмотрите вверх" (look up)
//   - "Улыбнитесь" (smile — placeholder)
//
// Verification uses VisionFrame data from useVisionEngine:
//   - Head pose yaw/pitch for direction challenges
//   - BlinkState.blinkCount for blink challenges
//   - Liveness score aggregation for anti-photo detection
//
// Challenge scheduling:
//   - Random interval: 45-120 seconds between challenges
//   - Max 8 challenges per 30-minute session
//   - Response window: 10 seconds
//   - Failed challenges: escalate severity progressively
//
// =============================================================================

import { ref, computed, type Ref } from 'vue'
import type { VisionFrame } from './useVisionEngine'
import { EventType, Severity, EventSource } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type ChallengeType
  = | 'turn_head_left'
    | 'turn_head_right'
    | 'look_up'
    | 'look_down'
    | 'blink_twice'
    | 'nod_yes'

export type ChallengeStatus = 'idle' | 'active' | 'verifying' | 'passed' | 'failed' | 'timeout'

export interface LivenessChallenge {
  id: string
  type: ChallengeType
  instruction: string
  status: ChallengeStatus
  issuedAt: number
  deadline: number // issuedAt + responseWindowMs
  completedAt: number | null
  attempts: number
}

export interface ChallengeConfig {
  /** Min interval between challenges (ms). Default: 45000. */
  minIntervalMs?: number
  /** Max interval between challenges (ms). Default: 120000. */
  maxIntervalMs?: number
  /** Response window for each challenge (ms). Default: 10000. */
  responseWindowMs?: number
  /** Max challenges per session. Default: 8. */
  maxChallenges?: number
  /** Head pose threshold for direction challenges (degrees). Default: 20. */
  headPoseThreshold?: number
  /** Blink count required for blink challenges. Default: 2. */
  blinkCountRequired?: number
}

// ---------------------------------------------------------------------------
// Challenge definitions
// ---------------------------------------------------------------------------

const CHALLENGE_POOL: { type: ChallengeType, instruction: string }[] = [
  { type: 'turn_head_left', instruction: 'Поверните голову влево' },
  { type: 'turn_head_right', instruction: 'Поверните голову вправо' },
  { type: 'look_up', instruction: 'Посмотрите вверх' },
  { type: 'look_down', instruction: 'Посмотрите вниз' },
  { type: 'blink_twice', instruction: 'Моргните дважды' },
  { type: 'nod_yes', instruction: 'Кивните головой' }
]

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useLivenessChallenge(config: ChallengeConfig = {}) {
  const {
    minIntervalMs = 45_000,
    maxIntervalMs = 120_000,
    responseWindowMs = 10_000,
    maxChallenges = 8,
    headPoseThreshold = 20,
    blinkCountRequired = 2
  } = config

  // State
  const currentChallenge = ref<LivenessChallenge | null>(null)
  const challengeHistory = ref<LivenessChallenge[]>([])
  const isRunning = ref(false)
  const consecutiveFailures = ref(0)

  // Stats
  const totalIssued = computed(() => challengeHistory.value.length)
  const totalPassed = computed(() => challengeHistory.value.filter(c => c.status === 'passed').length)
  const totalFailed = computed(() => challengeHistory.value.filter(c => c.status === 'failed' || c.status === 'timeout').length)
  const passRate = computed(() => totalIssued.value > 0 ? totalPassed.value / totalIssued.value : 1)

  // Internals
  let scheduleTimer: ReturnType<typeof setTimeout> | null = null
  let deadlineTimer: ReturnType<typeof setTimeout> | null = null
  // Blink baseline: -1 means "not yet captured for this challenge".
  // Will be set to frame.blink.blinkCount on the FIRST verifyFrame() call
  // after a blink_twice challenge is issued. This ensures we measure only
  // blinks that occur DURING the challenge, not global accumulated count.
  let blinkBaselineCount = -1
  let consecutivePasses = 0

  // Callback for sending events
  let sendEventFn: ((
    eventType: EventType,
    severity: Severity,
    payload?: any,
    label?: string,
    confidence?: number,
    source?: EventSource
  ) => void) | null = null

  /** Register event callback for integration with useProctoringSession. */
  function onEvent(fn: typeof sendEventFn) {
    sendEventFn = fn
  }

  /** Generate a random challenge, avoiding the last used type. */
  function pickChallenge(): { type: ChallengeType, instruction: string } {
    const lastType = challengeHistory.value.length > 0
      ? challengeHistory.value[challengeHistory.value.length - 1]!.type
      : null

    const available = CHALLENGE_POOL.filter(c => c.type !== lastType)
    return available[Math.floor(Math.random() * available.length)]!
  }

  /** Issue a new challenge. */
  function issueChallenge() {
    if (!isRunning.value) return
    if (totalIssued.value >= maxChallenges) return

    const template = pickChallenge()
    const now = Date.now()

    const challenge: LivenessChallenge = {
      id: `lc-${now.toString(36)}-${Math.random().toString(36).substring(2, 6)}`,
      type: template.type,
      instruction: template.instruction,
      status: 'active',
      issuedAt: now,
      deadline: now + responseWindowMs,
      completedAt: null,
      attempts: 0
    }

    // Reset blink baseline — will be captured on first verifyFrame() call
    blinkBaselineCount = -1

    currentChallenge.value = challenge

    // Set deadline timer
    deadlineTimer = setTimeout(() => {
      if (currentChallenge.value?.id === challenge.id && currentChallenge.value.status === 'active') {
        failChallenge('timeout')
      }
    }, responseWindowMs)

    // Report event
    sendEventFn?.(
      EventType.LIVENESS_CHECK_FAILED, // Will update status on result
      Severity.INFO,
      { type: 'liveness', data: { livenessScore: 0, blinkDetected: false, blinkRatePerMin: 0, textureScore: 0, depthScore: 0, spoofVector: '', frameQuality: 0 } },
      `Проверка живости: ${template.instruction}`,
      1.0,
      EventSource.WEBCAM
    )

    scheduleNext()
  }

  /** Schedule the next challenge at a random interval. */
  function scheduleNext() {
    if (scheduleTimer) clearTimeout(scheduleTimer)
    if (!isRunning.value) return
    if (totalIssued.value >= maxChallenges) return

    const delay = minIntervalMs + Math.random() * (maxIntervalMs - minIntervalMs)
    scheduleTimer = setTimeout(issueChallenge, delay)
  }

  /** Verify the current vision frame against the active challenge. */
  function verifyFrame(frame: VisionFrame): boolean {
    const challenge = currentChallenge.value
    if (!challenge || challenge.status !== 'active') return false

    challenge.attempts++
    let passed = false

    switch (challenge.type) {
      case 'turn_head_left':
        passed = frame.headPose.yaw < -headPoseThreshold
        break
      case 'turn_head_right':
        passed = frame.headPose.yaw > headPoseThreshold
        break
      case 'look_up':
        passed = frame.headPose.pitch > headPoseThreshold
        break
      case 'look_down':
        passed = frame.headPose.pitch < -headPoseThreshold
        break
      case 'blink_twice': {
        // Capture baseline on FIRST verifyFrame() call for this challenge.
        // This ensures we count only blinks that happen AFTER the challenge
        // is issued, not the global accumulated count from session start.
        if (blinkBaselineCount < 0) {
          blinkBaselineCount = frame.blink.blinkCount
        }
        let blinksSinceChallenge = frame.blink.blinkCount - blinkBaselineCount
        // Handle counter wrap-around (e.g., 8-bit counter rolling over)
        if (blinksSinceChallenge < 0) blinksSinceChallenge += 256
        passed = blinksSinceChallenge >= blinkCountRequired
        break
      }
      case 'nod_yes':
        // Detect significant pitch change (nod = pitch goes down then up)
        passed = Math.abs(frame.headPose.pitch) > headPoseThreshold * 0.8
        break
    }

    if (passed) {
      passChallenge()
      return true
    }

    return false
  }

  function passChallenge() {
    const challenge = currentChallenge.value
    if (!challenge) return

    challenge.status = 'passed'
    challenge.completedAt = Date.now()
    challengeHistory.value.push({ ...challenge })

    // Track consecutive passes to recover from failure escalation.
    // After 2 consecutive passes, reset the failure counter so that
    // severity doesn't stay stuck at CRITICAL forever.
    consecutivePasses++
    if (consecutivePasses >= 2) {
      consecutiveFailures.value = 0
      consecutivePasses = 0
    }

    if (deadlineTimer) {
      clearTimeout(deadlineTimer)
      deadlineTimer = null
    }

    currentChallenge.value = null
  }

  function failChallenge(reason: 'timeout' | 'failed') {
    const challenge = currentChallenge.value
    if (!challenge) return

    challenge.status = reason === 'timeout' ? 'timeout' : 'failed'
    challenge.completedAt = Date.now()
    challengeHistory.value.push({ ...challenge })
    consecutiveFailures.value++
    consecutivePasses = 0 // Reset pass streak on failure

    if (deadlineTimer) {
      clearTimeout(deadlineTimer)
      deadlineTimer = null
    }

    // Escalate severity based on consecutive failures
    const severity = consecutiveFailures.value >= 3
      ? Severity.CRITICAL
      : consecutiveFailures.value >= 2
        ? Severity.WARNING
        : Severity.INFO

    sendEventFn?.(
      EventType.LIVENESS_CHECK_FAILED,
      severity,
      { type: 'liveness', data: { livenessScore: 0, blinkDetected: false, blinkRatePerMin: 0, textureScore: 0, depthScore: 0, spoofVector: reason, frameQuality: 0 } },
      `Проверка живости не пройдена: ${challenge.instruction} (${reason})`,
      1.0,
      EventSource.WEBCAM
    )

    currentChallenge.value = null
  }

  /** Start the liveness challenge engine. */
  function start() {
    if (isRunning.value) return
    isRunning.value = true
    // First challenge after a random initial delay (15-45s)
    const initialDelay = 15_000 + Math.random() * 30_000
    scheduleTimer = setTimeout(issueChallenge, initialDelay)
  }

  /** Stop the liveness challenge engine. */
  function stop() {
    isRunning.value = false
    if (scheduleTimer) { clearTimeout(scheduleTimer); scheduleTimer = null }
    if (deadlineTimer) { clearTimeout(deadlineTimer); deadlineTimer = null }
    currentChallenge.value = null
  }

  return {
    // State
    currentChallenge,
    challengeHistory,
    isRunning,
    consecutiveFailures,

    // Stats
    totalIssued,
    totalPassed,
    totalFailed,
    passRate,

    // Actions
    start,
    stop,
    verifyFrame,
    onEvent
  }
}
