// =============================================================================
// Argus AI — Risk Computation SharedWorker
// =============================================================================
//
// Offloads risk scoring computation from the main thread.
// Receives session data + events, computes composite risk scores, returns
// sorted session IDs and score maps.
//
// Message protocol:
//   IN:  { type: 'compute', sessions: [...], events: [...] }
//   OUT: { type: 'result', scores: Map<sessionId, RiskScore>, sorted: string[] }
//
// =============================================================================

/**
 * Compute risk score for a single session.
 * @param {Object} session - { sessionId, criticalCount, warningCount }
 * @param {Array} sessionEvents - [{ receivedAt, confidence, severity }]
 * @returns {Object} risk score components
 */
function computeRiskScore(session, sessionEvents) {
  const now = Date.now()

  // Weight 1: Violation count (0-30 points)
  const violationWeight = Math.min(30, session.criticalCount * 6 + session.warningCount * 2)

  // Weight 2: Severity max (0-25 points)
  const severityWeight = session.criticalCount > 0 ? 25
    : session.warningCount > 0 ? 12
    : 0

  // Weight 3: Recency — events in last 60 seconds (0-25 points)
  const recentCount = sessionEvents.filter(e => now - e.receivedAt < 60000).length
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

// SharedWorker connection handler
self.onconnect = function(e) {
  const port = e.ports[0]

  port.onmessage = function(msg) {
    const { type, sessions, events, prevScores } = msg.data

    if (type !== 'compute') return

    const scores = {}
    const eventsBySession = {}

    // Group events by session ID
    for (const event of events) {
      if (!eventsBySession[event.sessionId]) {
        eventsBySession[event.sessionId] = []
      }
      eventsBySession[event.sessionId].push(event)
    }

    // Compute scores for each session
    for (const session of sessions) {
      const sessionEvents = eventsBySession[session.sessionId] || []
      const score = computeRiskScore(session, sessionEvents)

      // Preserve trend from previous scores
      const prevTrend = prevScores?.[session.sessionId]?.trend || []
      score.trend = [...prevTrend, score.composite].slice(-12)

      scores[session.sessionId] = score
    }

    // Sort by composite descending
    const sorted = Object.entries(scores)
      .sort((a, b) => b[1].composite - a[1].composite)
      .map(([id]) => id)

    port.postMessage({ type: 'result', scores, sorted })
  }

  port.start()
}
