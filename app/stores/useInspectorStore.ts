// =============================================================================
// Argus AI — Inspector Store (Live Inspector Dashboard Brain)
// =============================================================================
//
// Central state management for the Live Inspector Dashboard. Computes derived
// state on top of existing stores (useEventFeedStore, useTelemetryStore).
//
// Responsibilities:
//   1. Dynamic risk scoring with weighted composite algorithm
//   2. Grid ordering by risk score (recomputed every 5 seconds)
//   3. Evidence packet storage (one-click capture results)
//   4. UI state: grid density, focus mode, silence mode, sidebar filter
//   5. SharedWorker delegation for risk computation offloading
//
// =============================================================================

import { defineStore } from 'pinia'
import { ref, computed, shallowRef, triggerRef } from 'vue'
import { useEventFeedStore, type FeedEvent } from '~/stores/useEventFeedStore'
import type { ActiveSession } from '~/composables/useAdminAPI'
import { Severity } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type GridDensity = '2x2' | '3x3' | '4x4' | '5x5'

export interface RiskScore {
  sessionId: string
  composite: number // 0-100 final score
  violationWeight: number // 0-30
  severityWeight: number // 0-25
  recencyWeight: number // 0-25
  confidenceWeight: number // 0-20
  trend: number[] // last 12 values (5s intervals = 60s history)
}

export interface EvidencePacket {
  id: string
  sessionId: string
  timestamp: number
  frameDataUrl: string // JPEG data URL from canvas capture
  frameSha256: string // integrity hash
  aiMetadata: {
    headPose: { yaw: number, pitch: number, roll: number } | null
    audioClass: string | null
    gazeDirection: string | null
    livenessScore: number | null
    faceCount: number
    riskScore: number
  }
}

// ---------------------------------------------------------------------------
// Risk Scoring Algorithm
// ---------------------------------------------------------------------------

function computeRiskScore(
  session: ActiveSession,
  sessionEvents: FeedEvent[]
): Omit<RiskScore, 'trend'> {
  const now = Date.now()

  // Weight 1: Violation count (0-30 points)
  const violationWeight = Math.min(30, session.criticalCount * 6 + session.warningCount * 2)

  // Weight 2: Severity max (0-25 points)
  const severityWeight = session.criticalCount > 0
    ? 25
    : session.warningCount > 0
      ? 12
      : 0

  // Weight 3: Recency — events in last 60 seconds (0-25 points)
  const recentCount = sessionEvents.filter(e => now - e.receivedAt < 60_000).length
  const recencyWeight = Math.min(25, recentCount * 5)

  // Weight 4: AI confidence average of 10 most recent events (0-20 points)
  const recentSlice = sessionEvents.slice(0, 10)
  const avgConfidence = recentSlice.length > 0
    ? recentSlice.reduce((sum, e) => sum + e.confidence, 0) / recentSlice.length
    : 0
  const confidenceWeight = Math.round(avgConfidence * 20)

  return {
    sessionId: session.sessionId,
    composite: Math.min(100, violationWeight + severityWeight + recencyWeight + confidenceWeight),
    violationWeight,
    severityWeight,
    recencyWeight,
    confidenceWeight
  }
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

export const useInspectorStore = defineStore('inspector', () => {
  const feedStore = useEventFeedStore()

  // =========================================================================
  // Grid Display State
  // =========================================================================

  const gridDensity = ref<GridDensity>('3x3')
  const compactMode = computed(() => gridDensity.value === '5x5' || gridDensity.value === '4x4')

  function setGridDensity(density: GridDensity) {
    gridDensity.value = density
  }

  function gridCols(): number {
    switch (gridDensity.value) {
      case '2x2': return 2
      case '3x3': return 3
      case '4x4': return 4
      case '5x5': return 5
      default: return 3
    }
  }

  // =========================================================================
  // Focus Mode
  // =========================================================================

  const focusedSessionId = ref<string | null>(null)

  function setFocus(sessionId: string | null) {
    focusedSessionId.value = sessionId
    if (sessionId) {
      sidebarFilterSessionId.value = sessionId
    }
  }

  // =========================================================================
  // Silence Mode
  // =========================================================================

  const silenceMode = ref(false)
  const silenceUntil = ref(0)

  function toggleSilence(durationMinutes: number = 5) {
    if (silenceMode.value) {
      silenceMode.value = false
      silenceUntil.value = 0
    } else {
      silenceMode.value = true
      silenceUntil.value = Date.now() + durationMinutes * 60_000
    }
  }

  function checkSilenceExpiry() {
    if (silenceMode.value && silenceUntil.value > 0 && Date.now() > silenceUntil.value) {
      silenceMode.value = false
      silenceUntil.value = 0
    }
  }

  // =========================================================================
  // Sidebar Filter
  // =========================================================================

  const sidebarFilterSessionId = ref<string | null>(null)

  function setSidebarFilter(sessionId: string | null) {
    sidebarFilterSessionId.value = sessionId
  }

  // =========================================================================
  // Selected Cell (keyboard navigation)
  // =========================================================================

  const selectedCellIndex = ref(0)

  // =========================================================================
  // Risk Scoring
  // =========================================================================

  const sessionRiskScores = shallowRef<Map<string, RiskScore>>(new Map())
  const sortedSessionIds = shallowRef<string[]>([])

  const top3SessionIds = computed(() => sortedSessionIds.value.slice(0, 3))

  let riskTimer: ReturnType<typeof setInterval> | null = null
  let sessionsRef: ActiveSession[] = []

  /** Update the session reference for risk computation. */
  function updateSessions(sessions: ActiveSession[]) {
    sessionsRef = sessions
  }

  /** Compute risk scores for all sessions and sort. */
  function recomputeRisk() {
    checkSilenceExpiry()

    const events = feedStore.visibleEvents
    const newScores = new Map<string, RiskScore>()
    const oldScores = sessionRiskScores.value

    for (const session of sessionsRef) {
      const sessionEvents = events.filter(e => e.sessionId === session.sessionId)
      const score = computeRiskScore(session, sessionEvents)

      // Preserve trend from previous score, append new composite
      const prevScore = oldScores.get(session.sessionId)
      const prevTrend = prevScore?.trend ?? []
      const trend = [...prevTrend, score.composite].slice(-12) // keep last 12

      newScores.set(session.sessionId, { ...score, trend })
    }

    // Sort by composite descending. Apply hysteresis: only reorder if a session
    // moves 2+ positions from its current slot to prevent visual jitter.
    const newSorted = [...newScores.entries()]
      .sort((a, b) => b[1].composite - a[1].composite)
      .map(([id]) => id)

    const prevSorted = sortedSessionIds.value

    // Only apply new order if there's a meaningful position change
    let shouldReorder = false
    if (prevSorted.length !== newSorted.length) {
      shouldReorder = true
    } else {
      for (let i = 0; i < newSorted.length; i++) {
        const prevIdx = prevSorted.indexOf(newSorted[i]!)
        if (prevIdx === -1 || Math.abs(prevIdx - i) >= 2) {
          shouldReorder = true
          break
        }
      }
    }

    sessionRiskScores.value = newScores
    triggerRef(sessionRiskScores)

    if (shouldReorder) {
      sortedSessionIds.value = newSorted
      triggerRef(sortedSessionIds)
    }
  }

  function startRiskUpdates() {
    if (riskTimer) return
    recomputeRisk() // initial computation
    riskTimer = setInterval(recomputeRisk, 5_000)
  }

  function stopRiskUpdates() {
    if (riskTimer) {
      clearInterval(riskTimer)
      riskTimer = null
    }
  }

  /** Get risk score for a specific session. */
  function getRiskScore(sessionId: string): RiskScore {
    return sessionRiskScores.value.get(sessionId) ?? {
      sessionId,
      composite: 0,
      violationWeight: 0,
      severityWeight: 0,
      recencyWeight: 0,
      confidenceWeight: 0,
      trend: []
    }
  }

  // =========================================================================
  // Evidence Capture
  // =========================================================================

  const evidencePackets = ref<Map<string, EvidencePacket[]>>(new Map())

  function addEvidence(packet: EvidencePacket) {
    const existing = evidencePackets.value.get(packet.sessionId) ?? []
    existing.push(packet)
    evidencePackets.value.set(packet.sessionId, existing)
  }

  function getEvidenceCount(sessionId: string): number {
    return evidencePackets.value.get(sessionId)?.length ?? 0
  }

  function getEvidence(sessionId: string): EvidencePacket[] {
    return evidencePackets.value.get(sessionId) ?? []
  }

  // =========================================================================
  // Sorted & Filtered Sessions Helper
  // =========================================================================

  /** Get sessions sorted by risk. Falls back to input order if no risk data. */
  function sortSessionsByRisk(sessions: ActiveSession[]): ActiveSession[] {
    if (sortedSessionIds.value.length === 0) return sessions

    const sessionMap = new Map(sessions.map(s => [s.sessionId, s]))
    const result: ActiveSession[] = []

    // First add sessions that appear in sortedSessionIds (in sorted order)
    for (const id of sortedSessionIds.value) {
      const session = sessionMap.get(id)
      if (session) {
        result.push(session)
        sessionMap.delete(id)
      }
    }

    // Then add any remaining sessions not yet in the risk ranking
    for (const session of sessionMap.values()) {
      result.push(session)
    }

    return result
  }

  // =========================================================================
  // Public API
  // =========================================================================

  return {
    // Grid display
    gridDensity,
    compactMode,
    setGridDensity,
    gridCols,

    // Focus mode
    focusedSessionId,
    setFocus,

    // Silence mode
    silenceMode,
    silenceUntil,
    toggleSilence,

    // Sidebar filter
    sidebarFilterSessionId,
    setSidebarFilter,

    // Selected cell
    selectedCellIndex,

    // Risk scoring
    sessionRiskScores,
    sortedSessionIds,
    top3SessionIds,
    updateSessions,
    recomputeRisk,
    startRiskUpdates,
    stopRiskUpdates,
    getRiskScore,
    sortSessionsByRisk,

    // Evidence
    evidencePackets,
    addEvidence,
    getEvidenceCount,
    getEvidence
  }
})
