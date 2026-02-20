// =============================================================================
// Argus AI — useTierEngine Composable
// =============================================================================
//
// State machine that consumes Health Governor metrics and emits tier-specific
// configuration objects controlling video, AI, snapshots, upload, and telemetry.
//
// Tier Definitions:
//   A (Optimal):  Full video + real-time AI + realtime upload
//   B (Strained): 240p video + screen snapshots + batched upload
//   C (Critical): No video + JPEG snapshots + store-and-forward
//
// State Machine (no direct A↔C transitions):
//   A ──[score<70 × 3]──► B ──[score<40 × 3]──► C
//   C ──[score≥40 × 5]──► B ──[score≥70 × 5]──► A
//
// Usage:
//   const governor = useHealthGovernor()
//   const engine = useTierEngine(governor)
//   watch(engine.activeTierConfig, (config) => { ... })
//
// =============================================================================

import { computed, watch, type ComputedRef } from 'vue'
import type { ResilienceTier } from './useHealthGovernor'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Video capture configuration per tier. */
export interface TierVideoConfig {
  enabled: boolean
  resolution: '720p' | '480p' | '240p' | 'none'
  fps: number
  width: number
  height: number

  // Burst mode (Tier C only) — periodic low-fps capture when continuous
  // video is disabled but exam policy requires visual evidence.
  // When burstEnabled=true, the system captures at burstFps for
  // burstDurationSec every burstIntervalSec.
  burstEnabled?: boolean
  burstFps?: number          // FPS during burst window (default: 1)
  burstIntervalSec?: number  // seconds between burst starts (default: 30)
  burstDurationSec?: number  // seconds each burst lasts (default: 5)
}

/** AI inference configuration per tier. */
export interface TierAIConfig {
  enabled: boolean
  frequencyHz: number
  modelComplexity: 'full' | 'lite' | 'minimal'
}

/** Snapshot capture configuration per tier. */
export interface TierSnapshotConfig {
  enabled: boolean
  intervalMs: number
  quality: number
  resolution: '720p' | '480p' | '240p'
  width: number
  height: number
}

/** Audio capture configuration per tier. */
export interface TierAudioConfig {
  enabled: boolean
  sampleRate: number
}

/** Upload strategy configuration per tier. */
export interface TierUploadConfig {
  strategy: 'realtime' | 'batched' | 'store-and-forward'
  batchIntervalMs: number
  maxBatchSize: number
  priorityOrder: readonly ('critical' | 'snapshot' | 'audio' | 'video')[]
}

/** Telemetry configuration per tier. */
export interface TierTelemetryConfig {
  sampleIntervalMs: number
}

/** Complete tier configuration object. */
export interface TierConfig {
  tier: ResilienceTier
  label: string
  video: TierVideoConfig
  ai: TierAIConfig
  snapshot: TierSnapshotConfig
  audio: TierAudioConfig
  upload: TierUploadConfig
  telemetry: TierTelemetryConfig
}

/** Tier change event callback. */
export type TierChangeCallback = (from: ResilienceTier, to: ResilienceTier, config: TierConfig) => void

// ---------------------------------------------------------------------------
// Tier Configuration Definitions
// ---------------------------------------------------------------------------

const TIER_CONFIGS: Record<ResilienceTier, TierConfig> = {
  A: {
    tier: 'A',
    label: 'Оптимальный',
    video: {
      enabled: true,
      resolution: '720p',
      fps: 15,
      width: 1280,
      height: 720
    },
    ai: {
      enabled: true,
      frequencyHz: 10,
      modelComplexity: 'full'
    },
    snapshot: {
      enabled: false,
      intervalMs: 0,
      quality: 0.8,
      resolution: '720p',
      width: 1280,
      height: 720
    },
    audio: {
      enabled: true,
      sampleRate: 16000
    },
    upload: {
      strategy: 'realtime',
      batchIntervalMs: 500,
      maxBatchSize: 100,
      priorityOrder: ['critical', 'video', 'audio', 'snapshot'] as const
    },
    telemetry: {
      sampleIntervalMs: 100 // 10Hz
    }
  },

  B: {
    tier: 'B',
    label: 'Облегчённый',
    video: {
      enabled: true,
      resolution: '240p',
      fps: 10,
      width: 426,
      height: 240
    },
    ai: {
      enabled: true,
      frequencyHz: 5,
      modelComplexity: 'lite'
    },
    snapshot: {
      enabled: true,
      intervalMs: 5000,
      quality: 0.6,
      resolution: '480p',
      width: 854,
      height: 480
    },
    audio: {
      enabled: true,
      sampleRate: 8000
    },
    upload: {
      strategy: 'batched',
      batchIntervalMs: 2000,
      maxBatchSize: 200,
      priorityOrder: ['critical', 'snapshot', 'audio', 'video'] as const
    },
    telemetry: {
      sampleIntervalMs: 300 // ~3Hz
    }
  },

  C: {
    tier: 'C',
    label: 'Автономный',
    video: {
      enabled: false,
      resolution: 'none',
      fps: 0,
      width: 0,
      height: 0,
      // Burst mode: periodic 1fps capture for 5s every 30s.
      // Activated when exam policy requires visual evidence even in Tier C.
      burstEnabled: true,
      burstFps: 1,
      burstIntervalSec: 30,
      burstDurationSec: 5
    },
    ai: {
      enabled: true,
      frequencyHz: 2,
      modelComplexity: 'minimal'
    },
    snapshot: {
      enabled: true,
      intervalMs: 10000,
      quality: 0.4,
      resolution: '240p',
      width: 426,
      height: 240
    },
    audio: {
      enabled: true,
      sampleRate: 8000
    },
    upload: {
      strategy: 'store-and-forward',
      batchIntervalMs: 5000,
      maxBatchSize: 500,
      priorityOrder: ['critical', 'snapshot', 'audio', 'video'] as const
    },
    telemetry: {
      sampleIntervalMs: 1000 // 1Hz
    }
  }
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useTierEngine(
  governor: { currentTier: { value: ResilienceTier } }
) {
  // -------------------------------------------------------------------------
  // Tier Change Callbacks
  // -------------------------------------------------------------------------

  const changeCallbacks: TierChangeCallback[] = []
  let previousTier: ResilienceTier = governor.currentTier.value

  // -------------------------------------------------------------------------
  // Reactive Configuration
  // -------------------------------------------------------------------------

  /** The active tier configuration, reactively updated when tier changes. */
  const activeTierConfig: ComputedRef<TierConfig> = computed(() => {
    return TIER_CONFIGS[governor.currentTier.value]
  })

  /** Current tier label (Russian). */
  const tierLabel: ComputedRef<string> = computed(() => {
    return activeTierConfig.value.label
  })

  /** Whether the system is degraded (not in Tier A). */
  const isDegraded: ComputedRef<boolean> = computed(() => {
    return governor.currentTier.value !== 'A'
  })

  /** Whether the system is in critical mode (Tier C). */
  const isCritical: ComputedRef<boolean> = computed(() => {
    return governor.currentTier.value === 'C'
  })

  /** Whether video is enabled in the current tier. */
  const isVideoEnabled: ComputedRef<boolean> = computed(() => {
    return activeTierConfig.value.video.enabled
  })

  /** Whether snapshots are enabled in the current tier. */
  const isSnapshotEnabled: ComputedRef<boolean> = computed(() => {
    return activeTierConfig.value.snapshot.enabled
  })

  // -------------------------------------------------------------------------
  // Tier Change Watcher
  // -------------------------------------------------------------------------

  watch(
    () => governor.currentTier.value,
    (newTier) => {
      if (newTier !== previousTier) {
        const config = TIER_CONFIGS[newTier]
        console.info(
          `[argus:tier] Tier transition: ${previousTier} → ${newTier} (${config.label})`,
          {
            video: config.video.enabled ? config.video.resolution : 'disabled',
            snapshot: config.snapshot.enabled ? `${config.snapshot.intervalMs}ms` : 'disabled',
            upload: config.upload.strategy,
            telemetry: `${config.telemetry.sampleIntervalMs}ms`
          }
        )

        // Notify all callbacks
        for (const cb of changeCallbacks) {
          try {
            cb(previousTier, newTier, config)
          } catch (err) {
            console.error('[argus:tier] Tier change callback error:', err)
          }
        }

        previousTier = newTier
      }
    }
  )

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  /**
   * Register a callback for tier change events.
   * Returns an unsubscribe function.
   */
  function onTierChange(callback: TierChangeCallback): () => void {
    changeCallbacks.push(callback)
    return () => {
      const idx = changeCallbacks.indexOf(callback)
      if (idx !== -1) changeCallbacks.splice(idx, 1)
    }
  }

  /**
   * Get the configuration for a specific tier (for preview/comparison).
   */
  function getTierConfig(tier: ResilienceTier): TierConfig {
    return TIER_CONFIGS[tier]
  }

  return {
    // Reactive state
    activeTierConfig,
    tierLabel,
    isDegraded,
    isCritical,
    isVideoEnabled,
    isSnapshotEnabled,

    // Event handlers
    onTierChange,

    // Utilities
    getTierConfig,

    // Raw configs (for direct access)
    TIER_CONFIGS
  }
}
