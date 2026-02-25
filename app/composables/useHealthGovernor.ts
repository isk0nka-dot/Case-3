// =============================================================================
// Argus AI — useHealthGovernor Composable
// =============================================================================
//
// Background service that monitors client device health every 5 seconds.
// Produces a composite health score (0-100) that drives tier selection
// in the adaptive resilience layer.
//
// Metrics (lean — zero synthetic load):
//   - FPS (35%):          requestAnimationFrame loop counter
//   - RTT (35%):          heartbeat response time + /healthz ping
//   - Packet loss (30%):  failed request ratio from transport metrics
//
// CPU pressure is derived passively from FPS drop (no microbenchmark).
// Bandwidth is used only as a hard floor override, not in the composite score.
//
// Hysteresis:
//   - Downgrade: 3 consecutive checks below threshold
//   - Upgrade:   5 consecutive checks above threshold
//   - Prevents tier flapping on transient network spikes
//
// Usage:
//   const governor = useHealthGovernor()
//   governor.start()
//   watch(governor.currentTier, (tier) => { ... })
//   governor.stop()
//
// =============================================================================

import { ref, computed, type Ref, type ComputedRef } from 'vue'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Raw health metrics sampled every 5 seconds. */
export interface HealthMetrics {
  /** CPU pressure estimate (0-1). Derived from FPS drop — zero overhead. */
  cpuPressure: number
  /** Measured frames per second (from rAF loop). */
  fps: number
  /** Round-trip time in milliseconds (from /healthz ping). */
  rttMs: number
  /** Packet loss ratio (0-1). 0 = no loss, 1 = 100% loss. */
  packetLoss: number
  /** Estimated upload bandwidth in kbps. Used for floor override only. */
  bandwidthKbps: number
  /** Memory usage in MB (reserved, always 0 — Chrome-only API not worth the cost). */
  memoryUsageMB: number
  /** Timestamp of this sample. */
  timestamp: number
}

/** Resilience tier determined by health score. */
export type ResilienceTier = 'A' | 'B' | 'C'

/** Tier threshold configuration. */
export interface TierThresholds {
  /** Minimum score for Tier A. Default: 70. */
  tierAMin: number
  /** Minimum score for Tier B. Default: 40. */
  tierBMin: number
  /** Consecutive degraded samples before downgrade. Default: 3. */
  downgradeConsecutive: number
  /** Consecutive healthy samples before upgrade. Default: 5. */
  upgradeConsecutive: number
}

/** Health Governor configuration. */
export interface HealthGovernorConfig {
  /** Sampling interval in ms. Default: 5000 (5 seconds). */
  sampleIntervalMs: number
  /** Tier thresholds. */
  thresholds: TierThresholds
  /** Base URL for the health endpoint ping. Default: ''. */
  healthEndpoint: string
}

/** Weights for the composite health score. Must sum to 1.0. */
interface MetricWeights {
  fps: number
  rtt: number
  packetLoss: number
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_CONFIG: HealthGovernorConfig = {
  sampleIntervalMs: 5_000,
  thresholds: {
    tierAMin: 70,
    tierBMin: 40,
    downgradeConsecutive: 3,
    upgradeConsecutive: 5
  },
  healthEndpoint: ''
}

const WEIGHTS: MetricWeights = {
  fps: 0.35,
  rtt: 0.35,
  packetLoss: 0.30
}

// ---------------------------------------------------------------------------
// Score Normalization Functions
// ---------------------------------------------------------------------------

/**
 * Normalize FPS to a 0-100 score.
 * Target: 30fps = 100. Below 15fps = 0. Linear interpolation.
 */
function normalizeFps(fps: number): number {
  if (fps >= 30) return 100
  if (fps <= 15) return 0
  return Math.round(((fps - 15) / 15) * 100)
}

/**
 * Normalize RTT to a 0-100 score.
 * < 100ms = 100. > 2000ms = 0. Logarithmic curve for natural perception.
 */
function normalizeRtt(rttMs: number): number {
  if (rttMs <= 100) return 100
  if (rttMs >= 2000) return 0
  // Logarithmic: fast drop from 100-200ms, then gradual
  const logRange = Math.log(2000) - Math.log(100)
  const logValue = Math.log(rttMs) - Math.log(100)
  return Math.round(100 * (1 - logValue / logRange))
}

/**
 * Normalize packet loss to a 0-100 score.
 * 0% = 100. > 10% = 0. Linear.
 */
function normalizePacketLoss(loss: number): number {
  if (loss <= 0) return 100
  if (loss >= 0.10) return 0
  return Math.round(100 * (1 - loss / 0.10))
}

/**
 * Derive CPU pressure passively from FPS.
 * When FPS drops below target (30), CPU is under pressure.
 * Zero overhead — no microbenchmark needed.
 */
function deriveCpuPressureFromFps(fps: number): number {
  if (fps >= 30) return 0
  if (fps <= 5) return 1
  return Math.round(((30 - fps) / 25) * 100) / 100
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useHealthGovernor(config?: Partial<HealthGovernorConfig>) {
  const cfg: HealthGovernorConfig = {
    ...DEFAULT_CONFIG,
    ...config,
    thresholds: {
      ...DEFAULT_CONFIG.thresholds,
      ...config?.thresholds
    }
  }

  // -------------------------------------------------------------------------
  // Reactive State
  // -------------------------------------------------------------------------

  const currentTier: Ref<ResilienceTier> = ref('A')
  const healthScore: Ref<number> = ref(100)
  const metrics: Ref<HealthMetrics> = ref({
    cpuPressure: 0,
    fps: 60,
    rttMs: 0,
    packetLoss: 0,
    bandwidthKbps: 1000,
    memoryUsageMB: 0,
    timestamp: Date.now()
  })
  const isMonitoring: Ref<boolean> = ref(false)

  // Hysteresis counters
  const consecutiveDowngradeSamples: Ref<number> = ref(0)
  const consecutiveUpgradeSamples: Ref<number> = ref(0)

  // Internal state
  let sampleTimer: ReturnType<typeof setInterval> | null = null
  let rAFHandle: number | null = null

  // FPS measurement
  let frameCount = 0
  let lastFpsCalcTime = 0
  let currentFps = 60

  // Bandwidth estimation
  let lastBytesSent = 0
  let lastBandwidthCalcTime = 0
  let currentBandwidthKbps = 1000

  // RTT tracking
  let lastRttMs = 0

  // Inference latency tracking (reported by VisionEngine)
  const inferenceLatencyMs: Ref<number> = ref(0)

  // -------------------------------------------------------------------------
  // FPS Measurement via requestAnimationFrame
  // -------------------------------------------------------------------------

  function fpsLoop(): void {
    frameCount++
    const now = performance.now()

    if (now - lastFpsCalcTime >= 1000) {
      currentFps = frameCount
      frameCount = 0
      lastFpsCalcTime = now
    }

    if (isMonitoring.value) {
      rAFHandle = requestAnimationFrame(fpsLoop)
    }
  }

  // -------------------------------------------------------------------------
  // RTT Measurement via /healthz ping
  // -------------------------------------------------------------------------

  async function measureRtt(): Promise<number> {
    try {
      let pingUrl = '/healthz'
      // If a specific endpoint is configured, use it
      if (cfg.healthEndpoint) {
        pingUrl = cfg.healthEndpoint + '/healthz'
      } else {
        try {
          const { $grpc } = useNuxtApp()
          if ($grpc) {
            const transport = $grpc.transport as unknown as Record<string, unknown>
            const transportConfig = transport?.config as Record<string, string> | undefined
            pingUrl = transportConfig?.baseUrl ? transportConfig.baseUrl + '/healthz' : '/healthz'
          }
        } catch {
          // Use default
        }
      }

      const startTime = performance.now()
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), 5000)

      try {
        await fetch(pingUrl, {
          method: 'GET',
          signal: controller.signal,
          cache: 'no-store'
        })
        clearTimeout(timeoutId)
        const rtt = performance.now() - startTime
        lastRttMs = rtt
        return rtt
      } catch {
        clearTimeout(timeoutId)
        // If fetch fails, RTT is "infinite" - return a high value
        lastRttMs = 5000
        return 5000
      }
    } catch {
      return lastRttMs || 5000
    }
  }

  // -------------------------------------------------------------------------
  // Bandwidth Estimation (used for floor override only — not in score)
  // -------------------------------------------------------------------------

  function estimateBandwidth(): number {
    try {
      const { $grpc } = useNuxtApp()
      if (!$grpc) return currentBandwidthKbps

      const currentMetrics = $grpc.getMetrics()
      const now = Date.now()
      const timeDelta = (now - lastBandwidthCalcTime) / 1000 // seconds

      if (timeDelta > 0 && lastBandwidthCalcTime > 0) {
        const bytesDelta = currentMetrics.totalBytesSent - lastBytesSent
        const bytesPerSec = bytesDelta / timeDelta
        currentBandwidthKbps = (bytesPerSec * 8) / 1024 // Convert to kbps
      }

      lastBytesSent = currentMetrics.totalBytesSent
      lastBandwidthCalcTime = now

      return Math.max(0, currentBandwidthKbps)
    } catch {
      return currentBandwidthKbps
    }
  }

  // -------------------------------------------------------------------------
  // Packet Loss Calculation
  // -------------------------------------------------------------------------

  function calculatePacketLoss(): number {
    try {
      const { $grpc } = useNuxtApp()
      if (!$grpc) return 0

      const m = $grpc.getMetrics()
      if (m.totalRequests === 0) return 0
      return m.totalFailures / m.totalRequests
    } catch {
      return 0
    }
  }

  // -------------------------------------------------------------------------
  // Composite Health Score (3 metrics — no CPU microbenchmark)
  // -------------------------------------------------------------------------

  function computeHealthScore(m: HealthMetrics): number {
    const fpsScore = normalizeFps(m.fps) * WEIGHTS.fps
    const rttScore = normalizeRtt(m.rttMs) * WEIGHTS.rtt
    const lossScore = normalizePacketLoss(m.packetLoss) * WEIGHTS.packetLoss

    // CPU spike penalty: AI inference latency >50ms indicates CPU pressure
    // from vision processing. This supplements the FPS-based detection
    // which may lag behind actual CPU spikes.
    //   50ms → 0 penalty   (normal)
    //   75ms → 5 penalty   (moderate)
    //  100ms → 10 penalty  (high)
    //  150ms → 20 penalty  (severe — capped)
    let cpuPenalty = 0
    if (inferenceLatencyMs.value > 50) {
      cpuPenalty = Math.min(20, (inferenceLatencyMs.value - 50) / 5)
    }

    return Math.max(0, Math.round(fpsScore + rttScore + lossScore - cpuPenalty))
  }

  /**
   * Report the latest AI inference latency from the VisionEngine.
   * This feeds the CPU spike penalty in the health score computation.
   * Called by VisionEngine after each frame inference completes.
   */
  function reportInferenceLatency(ms: number): void {
    inferenceLatencyMs.value = ms
  }

  // -------------------------------------------------------------------------
  // Tier Determination with Hysteresis
  // -------------------------------------------------------------------------

  function determineTierFromScore(score: number): ResilienceTier {
    if (score >= cfg.thresholds.tierAMin) return 'A'
    if (score >= cfg.thresholds.tierBMin) return 'B'
    return 'C'
  }

  function applyHysteresis(proposedTier: ResilienceTier): void {
    const tierRank: Record<ResilienceTier, number> = { A: 3, B: 2, C: 1 }
    const currentRank = tierRank[currentTier.value]
    const proposedRank = tierRank[proposedTier]

    if (proposedRank < currentRank) {
      // Proposed downgrade
      consecutiveUpgradeSamples.value = 0
      consecutiveDowngradeSamples.value++

      if (consecutiveDowngradeSamples.value >= cfg.thresholds.downgradeConsecutive) {
        // No direct A→C: step through B first
        if (currentTier.value === 'A' && proposedTier === 'C') {
          currentTier.value = 'B'
        } else {
          currentTier.value = proposedTier
        }
        consecutiveDowngradeSamples.value = 0
      }
    } else if (proposedRank > currentRank) {
      // Proposed upgrade
      consecutiveDowngradeSamples.value = 0
      consecutiveUpgradeSamples.value++

      if (consecutiveUpgradeSamples.value >= cfg.thresholds.upgradeConsecutive) {
        // No direct C→A: step through B first
        if (currentTier.value === 'C' && proposedTier === 'A') {
          currentTier.value = 'B'
        } else {
          currentTier.value = proposedTier
        }
        consecutiveUpgradeSamples.value = 0
      }
    } else {
      // Same tier — reset both counters
      consecutiveDowngradeSamples.value = 0
      consecutiveUpgradeSamples.value = 0
    }
  }

  // -------------------------------------------------------------------------
  // Main Sampling Loop
  // -------------------------------------------------------------------------

  async function sample(): Promise<void> {
    // Collect metrics — all passive reads, no synthetic workload
    const rttMs = await measureRtt()
    const bandwidthKbps = estimateBandwidth()
    const packetLoss = calculatePacketLoss()

    // Derive CPU pressure from FPS (free — no microbenchmark)
    const cpuPressure = deriveCpuPressureFromFps(currentFps)

    const newMetrics: HealthMetrics = {
      cpuPressure,
      fps: currentFps,
      rttMs,
      packetLoss,
      bandwidthKbps,
      memoryUsageMB: 0,
      timestamp: Date.now()
    }

    metrics.value = newMetrics

    // Compute composite score
    const score = computeHealthScore(newMetrics)
    healthScore.value = score

    // Determine tier with hysteresis
    let proposedTier = determineTierFromScore(score)

    // Bandwidth floor override: force downgrade when bandwidth is critically low,
    // regardless of composite score. This prevents staying in Tier A when bandwidth
    // is insufficient for video streaming (e.g., 100kbps + perfect FPS/RTT = 100 score).
    if (newMetrics.bandwidthKbps < 128 && proposedTier === 'A') {
      proposedTier = 'C' // Below 128kbps = store-and-forward only
    } else if (newMetrics.bandwidthKbps < 256 && proposedTier === 'A') {
      proposedTier = 'B' // Below 256kbps = degraded mode
    }

    applyHysteresis(proposedTier)
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  /**
   * Start health monitoring.
   * Begins FPS measurement loop and periodic health sampling.
   */
  function start(): void {
    if (isMonitoring.value) return

    isMonitoring.value = true

    // Start FPS measurement
    frameCount = 0
    lastFpsCalcTime = performance.now()
    rAFHandle = requestAnimationFrame(fpsLoop)

    // Initialize bandwidth baseline
    lastBandwidthCalcTime = Date.now()
    try {
      const { $grpc } = useNuxtApp()
      if ($grpc) {
        lastBytesSent = $grpc.getMetrics().totalBytesSent
      }
    } catch {
      // Plugin not available yet
    }

    // Start periodic sampling
    sampleTimer = setInterval(() => {
      void sample()
    }, cfg.sampleIntervalMs)

    // Initial sample after a short delay (let FPS stabilize)
    setTimeout(() => {
      void sample()
    }, 1000)

    console.debug('[argus:health] Health governor started', {
      intervalMs: cfg.sampleIntervalMs,
      thresholds: cfg.thresholds
    })
  }

  /**
   * Stop health monitoring.
   * Releases all timers and animation frames.
   */
  function stop(): void {
    isMonitoring.value = false

    if (sampleTimer) {
      clearInterval(sampleTimer)
      sampleTimer = null
    }

    if (rAFHandle !== null) {
      cancelAnimationFrame(rAFHandle)
      rAFHandle = null
    }

    console.debug('[argus:health] Health governor stopped')
  }

  /**
   * Force an immediate health sample (useful for testing).
   */
  async function forceSample(): Promise<void> {
    await sample()
  }

  // -------------------------------------------------------------------------
  // Computed
  // -------------------------------------------------------------------------

  const tierLabel: ComputedRef<string> = computed(() => {
    switch (currentTier.value) {
      case 'A': return 'Оптимальный'
      case 'B': return 'Облегчённый'
      case 'C': return 'Автономный'
    }
  })

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // State
    currentTier,
    healthScore,
    metrics,
    isMonitoring,
    tierLabel,
    consecutiveDowngradeSamples,
    consecutiveUpgradeSamples,
    inferenceLatencyMs: computed(() => inferenceLatencyMs.value),

    // Lifecycle
    start,
    stop,
    forceSample,

    // Vision Engine integration
    reportInferenceLatency
  }
}
