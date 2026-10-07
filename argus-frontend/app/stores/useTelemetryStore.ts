// =============================================================================
// Argus AI — Telemetry State Store
// =============================================================================
//
// Optimized Pinia store for high-frequency telemetry data (gaze, mouse,
// keyboard tracking). Designed to handle 30-60Hz sensor data without
// causing browser lag or excessive memory consumption.
//
// The Problem:
//   A typical proctoring session generates:
//     - Gaze tracking: 30 points/sec (webcam eye-tracking)
//     - Mouse movement: 60 points/sec (mousemove events)
//     - Keyboard timing: ~5 events/sec (keydown/keyup)
//     = ~95 events/sec per session × 10,000 sessions = 950,000 events/sec
//
//   Storing every raw data point in Vue reactive state would:
//     1. Trigger 95 re-renders/sec per session (browser can handle ~60fps)
//     2. Consume ~50MB/hour per session (95 events × 500B × 3600s)
//     3. Cause GC pauses every ~100ms due to array allocations
//
// The Solution:
//
//   1. Typed Arrays (Float32Array) instead of Object arrays
//      - 4x less memory (4 bytes vs 16+ bytes per number)
//      - No GC pressure (pre-allocated, reused)
//      - CPU cache-friendly (contiguous memory layout)
//
//   2. Downsampled Visualization
//      - Raw data at 30-60Hz for backend ingestion
//      - Visualization data at 2Hz for chart rendering
//      - Heatmap data at 0.1Hz (10-second snapshots)
//
//   3. Double-Buffered Updates
//      - Write buffer: receives raw sensor data (non-reactive)
//      - Read buffer: copied to reactive state at 2Hz for rendering
//      - Swap is O(1) — just pointer exchange
//
//   4. Per-Session Isolation
//      - Each session has its own telemetry buffer
//      - Sessions can be independently cleared or inspected
//      - Total memory is bounded: 50 sessions × 2KB per buffer = 100KB
//
// =============================================================================

import { defineStore } from 'pinia'
import { ref, shallowRef, triggerRef, onUnmounted } from 'vue'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** A single gaze data point for visualization. */
export interface GazePoint {
  readonly x: number // Normalized 0-1
  readonly y: number // Normalized 0-1
  readonly timestamp: number // ms since session start
}

/** A gaze heatmap cell. */
export interface HeatmapCell {
  readonly gridX: number // 0-based grid column
  readonly gridY: number // 0-based grid row
  readonly count: number // Number of gaze points in this cell
  readonly intensity: number // Normalized 0-1
}

/** Per-session telemetry state. */
export interface SessionTelemetry {
  /** Recent gaze points for the gaze trail visualization (last 2 seconds). */
  gazeTrail: GazePoint[]
  /** Gaze heatmap grid (10×10 = 100 cells). */
  heatmap: HeatmapCell[]
  /** Current gaze position. */
  currentGaze: { x: number, y: number } | null
  /** Current mouse position. */
  currentMouse: { x: number, y: number } | null
  /** Focus score history (last 60 data points = 2 minutes at 2Hz). */
  focusHistory: number[]
  /** Typing speed history (last 60 data points). */
  typingSpeedHistory: number[]
  /** Last update timestamp. */
  lastUpdate: number
  /** Total gaze samples received. */
  totalGazeSamples: number
  /** Total mouse samples received. */
  totalMouseSamples: number
  /** Total keyboard events received. */
  totalKeyboardEvents: number
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

export const useTelemetryStore = defineStore('telemetry', () => {
  // -------------------------------------------------------------------------
  // Worker Initialization
  // -------------------------------------------------------------------------

  let worker: Worker | null = null

  function initWorker() {
    if (worker) return
    // Assuming the worker is served correctly by Nuxt/Vite
    worker = new Worker(new URL('../workers/telemetry.worker.ts', import.meta.url), { type: 'module' })

    worker.onmessage = (e) => {
      const { type, payload } = e.data
      if (type === 'SYNC_STATE') {
        const now = Date.now()
        const newSessions = new Map<string, SessionTelemetry>()

        // Convert plain object back to Map and freeze states
        for (const sessionId of Object.keys(payload.sessions)) {
          const s = payload.sessions[sessionId]
          newSessions.set(sessionId, Object.freeze({
            ...s,
            lastUpdate: now
          }) as SessionTelemetry)
        }

        sessions.value = newSessions
        triggerRef(sessions)

        globalStats.value = payload.globalStats
      }
    }
  }

  // -------------------------------------------------------------------------
  // Reactive State (updated at 2Hz from Worker)
  // -------------------------------------------------------------------------

  /**
   * Per-session telemetry state.
   * Uses shallowRef to avoid deep reactivity on the Map contents.
   */
  const sessions = shallowRef<Map<string, SessionTelemetry>>(new Map())

  /** Active session IDs being tracked. */
  const activeSessionIds = ref<string[]>([])

  /** Global telemetry stats. */
  const globalStats = ref({
    totalSessions: 0,
    totalGazeSamples: 0,
    totalMouseSamples: 0,
    totalKeyboardEvents: 0
  })

  // Update timer (asks worker for state).
  let updateTimer: ReturnType<typeof setInterval> | null = null

  // -------------------------------------------------------------------------
  // Session Management
  // -------------------------------------------------------------------------

  function startSession(sessionId: string): void {
    initWorker()
    if (activeSessionIds.value.includes(sessionId)) return

    worker?.postMessage({ type: 'START_SESSION', payload: { sessionId } })
    activeSessionIds.value = [...activeSessionIds.value, sessionId]
  }

  function stopSession(sessionId: string): void {
    worker?.postMessage({ type: 'STOP_SESSION', payload: { sessionId } })
    activeSessionIds.value = activeSessionIds.value.filter(id => id !== sessionId)
  }

  // -------------------------------------------------------------------------
  // Data Ingestion (offloaded to Web Worker via postMessage)
  // -------------------------------------------------------------------------

  function recordGaze(sessionId: string, x: number, y: number): void {
    worker?.postMessage({ type: 'RECORD_GAZE', payload: { sessionId, x, y } })
  }

  function recordMouse(sessionId: string, x: number, y: number): void {
    worker?.postMessage({ type: 'RECORD_MOUSE', payload: { sessionId, x, y } })
  }

  function recordKeyboard(sessionId: string, wpm: number): void {
    worker?.postMessage({ type: 'RECORD_KEYBOARD', payload: { sessionId, wpm } })
  }

  function recordFocusScore(sessionId: string, score: number): void {
    worker?.postMessage({ type: 'RECORD_FOCUS', payload: { sessionId, score } })
  }

  // -------------------------------------------------------------------------
  // Reactive Update (2Hz)
  // -------------------------------------------------------------------------

  /**
   * Start periodic reactive state updates.
   * Call when the telemetry dashboard is mounted.
   */
  function startUpdates(): void {
    if (updateTimer) return

    updateTimer = setInterval(() => {
      syncReactiveState()
    }, 500) // 2Hz
  }

  /**
   * Stop periodic reactive state updates.
   */
  function stopUpdates(): void {
    if (updateTimer) {
      clearInterval(updateTimer)
      updateTimer = null
    }
  }

  /**
   * Sync non-reactive internal state → reactive Vue state.
   * This simply asks the background Worker to construct the state and postMessage it back.
   */
  function syncReactiveState(): void {
    if (!worker) return
    worker.postMessage({ type: 'GET_STATE' })
  }

  // -------------------------------------------------------------------------
  // Queries
  // -------------------------------------------------------------------------

  /**
   * Get telemetry for a specific session.
   */
  function getSessionTelemetry(sessionId: string): SessionTelemetry | undefined {
    return sessions.value.get(sessionId)
  }

  /**
   * Get all active sessions with their telemetry.
   */
  function getAllSessions(): Map<string, SessionTelemetry> {
    return sessions.value
  }

  /**
   * Clear all telemetry data.
   */
  function clearAll(): void {
    for (const sessionId of [...activeSessionIds.value]) {
      stopSession(sessionId)
    }
    sessions.value = new Map()
    triggerRef(sessions)
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // Reactive state
    sessions,
    activeSessionIds,
    globalStats,

    // Session management
    startSession,
    stopSession,

    // Data ingestion (high-frequency)
    recordGaze,
    recordMouse,
    recordKeyboard,
    recordFocusScore,

    // Update lifecycle
    startUpdates,
    stopUpdates,

    // Queries
    getSessionTelemetry,
    getAllSessions,
    clearAll
  }
})
