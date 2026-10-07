// =============================================================================
//  Argus AI — k6 Load Testing Script
//  Simulates 5,000 concurrent students sending heartbeat & violation events
//  to the Go backend via gRPC-Web JSON (HTTP POST to port 8080).
//
//  Usage:
//    k6 run scripts/k6/load-test.js
//    k6 run scripts/k6/load-test.js --env BASE_URL=http://prod:8080
//    k6 run scripts/k6/load-test.js --env VUS=1000  # override VU count
// =============================================================================

import http from 'k6/http'
import { check, sleep, group } from 'k6'
import { Counter, Rate, Trend } from 'k6/metrics'

// ---------------------------------------------------------------------------
// Custom Metrics
// ---------------------------------------------------------------------------

const batchLatency = new Trend('batch_latency', true)
const heartbeatLatency = new Trend('heartbeat_latency', true)
const batchAcceptedRate = new Rate('batch_accepted_rate')
const idempotencyDuplicateRate = new Rate('idempotency_duplicate_rate')
const eventsIngested = new Counter('events_ingested')
const batchesSent = new Counter('batches_sent')
const heartbeatsSent = new Counter('heartbeats_sent')

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080'
const EVENT_COLLECTOR_SERVICE = '/argus.eventcollector.v1.EventCollectorService'
const MAX_VUS = parseInt(__ENV.VUS) || 5000

export const options = {
  scenarios: {
    students: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: Math.floor(MAX_VUS * 0.2) },   // warm-up → 20%
        { duration: '2m', target: MAX_VUS },                      // ramp to peak
        { duration: '5m', target: MAX_VUS },                      // sustained peak
        { duration: '2m', target: 0 },                            // cool-down
      ],
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'batch_latency':              ['p(95)<50', 'p(99)<100'],
    'heartbeat_latency':          ['p(95)<30', 'p(99)<50'],
    'http_req_failed':            ['rate<0.01'],
    'batch_accepted_rate':        ['rate>0.99'],
    'idempotency_duplicate_rate': ['rate>0.90'],
  },
}

// ---------------------------------------------------------------------------
// UUIDv7 Generator (k6 doesn't have crypto.randomUUID)
// ---------------------------------------------------------------------------

function uuidv7() {
  const timestamp = Date.now()
  const hex = timestamp.toString(16).padStart(12, '0')

  // Random bytes for the rest
  const rand = () => Math.floor(Math.random() * 16).toString(16)
  const r = () => rand() + rand()

  // Format: tttttttt-tttt-7rrr-rrrr-rrrrrrrrrrrr
  return (
    hex.slice(0, 8) + '-' +
    hex.slice(8, 12) + '-' +
    '7' + r() + rand() + '-' +
    (8 + Math.floor(Math.random() * 4)).toString(16) + r() + rand() + '-' +
    r() + r() + r() + r() + r() + r()
  )
}

// ---------------------------------------------------------------------------
// Event Type Distributions
// ---------------------------------------------------------------------------

const HONEST_EVENTS = [
  { type: 'GAZE_DEVIATION', severity: 'INFO', confidence: [0.2, 0.4], weight: 60 },
  { type: 'HEAD_POSE_ANOMALY', severity: 'INFO', confidence: [0.15, 0.3], weight: 30 },
  { type: 'VOICE_ACTIVITY', severity: 'INFO', confidence: [0.1, 0.25], weight: 10 },
]

const SUSPICIOUS_EVENTS = [
  { type: 'GAZE_DEVIATION', severity: 'WARNING', confidence: [0.5, 0.75], weight: 30 },
  { type: 'HEAD_POSE_ANOMALY', severity: 'WARNING', confidence: [0.55, 0.8], weight: 25 },
  { type: 'VOICE_ACTIVITY', severity: 'WARNING', confidence: [0.6, 0.85], weight: 20 },
  { type: 'FACE_NOT_DETECTED', severity: 'WARNING', confidence: [0.7, 0.9], weight: 15 },
  { type: 'FACE_OCCLUDED', severity: 'WARNING', confidence: [0.5, 0.7], weight: 10 },
]

const CHEATER_EVENTS = [
  { type: 'PHONE_DETECTED', severity: 'CRITICAL', confidence: [0.85, 0.98], weight: 25 },
  { type: 'MULTIPLE_PERSONS', severity: 'CRITICAL', confidence: [0.8, 0.95], weight: 20 },
  { type: 'FACE_MISMATCH', severity: 'CRITICAL', confidence: [0.75, 0.95], weight: 20 },
  { type: 'BOOK_DETECTED', severity: 'WARNING', confidence: [0.7, 0.9], weight: 15 },
  { type: 'EARBUDS_DETECTED', severity: 'WARNING', confidence: [0.65, 0.88], weight: 10 },
  { type: 'FACE_SPOOF_DETECTED', severity: 'CRITICAL', confidence: [0.9, 0.99], weight: 10 },
]

// Select a random event from a weighted distribution
function pickWeightedEvent(events) {
  const totalWeight = events.reduce((sum, e) => sum + e.weight, 0)
  let rand = Math.random() * totalWeight
  for (const event of events) {
    rand -= event.weight
    if (rand <= 0) return event
  }
  return events[events.length - 1]
}

// ---------------------------------------------------------------------------
// Student Profile Assignment
// ---------------------------------------------------------------------------

function getStudentProfile(vuId) {
  // Deterministic: 80% honest, 15% suspicious, 5% cheater
  const bucket = vuId % 100
  if (bucket < 80) return 'honest'
  if (bucket < 95) return 'suspicious'
  return 'cheater'
}

function getEventPool(profile) {
  switch (profile) {
    case 'honest': return HONEST_EVENTS
    case 'suspicious': return SUSPICIOUS_EVENTS
    case 'cheater': return CHEATER_EVENTS
    default: return HONEST_EVENTS
  }
}

// ---------------------------------------------------------------------------
// Event Builder
// ---------------------------------------------------------------------------

function buildEvent(vuId, profile) {
  const pool = getEventPool(profile)
  const eventDef = pickWeightedEvent(pool)
  const confRange = eventDef.confidence
  const confidence = confRange[0] + Math.random() * (confRange[1] - confRange[0])

  return {
    event_id: uuidv7(),
    session_id: `loadtest-session-${vuId}`,
    student_id: `loadtest-student-${vuId}`,
    exam_id: 'loadtest-exam-001',
    org_id: 'org-loadtest',
    event_type: eventDef.type,
    severity: eventDef.severity,
    source: 'CAMERA',
    client_timestamp: new Date().toISOString(),
    label: `[k6] ${eventDef.type} from VU ${vuId}`,
    confidence: Math.round(confidence * 100) / 100,
  }
}

// ---------------------------------------------------------------------------
// Setup — Authenticate Once Per VU
// ---------------------------------------------------------------------------

export function setup() {
  const loginRes = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({
      phone: '+77077469966',
      password: 'Astana01+',
    }),
    { headers: { 'Content-Type': 'application/json' } }
  )

  const loginOk = check(loginRes, {
    'login: status 200': (r) => r.status === 200,
    'login: has token': (r) => {
      try { return !!r.json('token') } catch { return false }
    },
  })

  if (!loginOk) {
    console.error(`Login failed: ${loginRes.status} — ${loginRes.body}`)
    return { jwt: '' }
  }

  const jwt = loginRes.json('token')
  console.log(`[setup] Authenticated. Token length: ${jwt.length}`)
  return { jwt }
}

// ---------------------------------------------------------------------------
// Main VU Loop
// ---------------------------------------------------------------------------

export default function main(data) {
  if (!data.jwt) {
    console.error('[VU] No JWT token — skipping iteration')
    sleep(5)
    return
  }

  const vuId = __VU
  const profile = getStudentProfile(vuId)
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${data.jwt}`,
  }

  // Track previous batch IDs for idempotency testing
  const prevBatchIds = []

  // --- Batch Ingest (every iteration ≈ 5s) ---
  group('batch_ingest', () => {
    const batchSize = 3 + Math.floor(Math.random() * 6) // 3-8 events
    const events = []
    for (let i = 0; i < batchSize; i++) {
      events.push(buildEvent(vuId, profile))
    }

    const batchId = `batch-${vuId}-${__ITER}-${Date.now()}`
    const payload = JSON.stringify({
      events: events,
      batch_id: batchId,
    })

    const res = http.post(
      `${BASE_URL}${EVENT_COLLECTOR_SERVICE}/IngestBatch`,
      payload,
      {
        headers: headers,
        tags: { endpoint: 'ingest' },
      }
    )

    batchLatency.add(res.timings.duration)
    batchesSent.add(1)

    const ok = check(res, {
      'ingest: status 200': (r) => r.status === 200,
      'ingest: accepted > 0': (r) => {
        try {
          const body = r.json()
          return body.accepted_count > 0 || body.acceptedCount > 0
        } catch {
          return false
        }
      },
    })

    batchAcceptedRate.add(ok ? 1 : 0)

    if (ok) {
      try {
        const body = res.json()
        const accepted = body.accepted_count || body.acceptedCount || 0
        eventsIngested.add(accepted)
      } catch { /* ignore */ }
    }

    // Store batch ID for idempotency replay
    prevBatchIds.push(batchId)
    if (prevBatchIds.length > 5) prevBatchIds.shift()

    // --- Idempotency Test: 10% chance to replay a previous batch ---
    if (Math.random() < 0.10 && prevBatchIds.length > 1) {
      const replayBatchId = prevBatchIds[Math.floor(Math.random() * (prevBatchIds.length - 1))]
      const replayPayload = JSON.stringify({
        events: events, // Same events
        batch_id: replayBatchId, // Duplicate batch ID
      })

      const replayRes = http.post(
        `${BASE_URL}${EVENT_COLLECTOR_SERVICE}/IngestBatch`,
        replayPayload,
        {
          headers: headers,
          tags: { endpoint: 'ingest_replay' },
        }
      )

      if (replayRes.status === 200) {
        try {
          const body = replayRes.json()
          const accepted = body.accepted_count || body.acceptedCount || 0
          // Idempotent: duplicate batch should be rejected (accepted_count = 0)
          idempotencyDuplicateRate.add(accepted === 0 ? 1 : 0)
        } catch {
          idempotencyDuplicateRate.add(0)
        }
      }
    }
  })

  // --- Heartbeat (every 6th iteration ≈ 30s at 5s/iter) ---
  if (__ITER % 6 === 0) {
    group('heartbeat', () => {
      const payload = JSON.stringify({
        session_id: `loadtest-session-${vuId}`,
        student_id: `loadtest-student-${vuId}`,
        exam_id: 'loadtest-exam-001',
        client_timestamp: new Date().toISOString(),
        current_focus_score: 0.5 + Math.random() * 0.5,
        violation_count: Math.floor(Math.random() * 5),
      })

      const res = http.post(
        `${BASE_URL}${EVENT_COLLECTOR_SERVICE}/Heartbeat`,
        payload,
        {
          headers: headers,
          tags: { endpoint: 'heartbeat' },
        }
      )

      heartbeatLatency.add(res.timings.duration)
      heartbeatsSent.add(1)

      check(res, {
        'heartbeat: status 200': (r) => r.status === 200,
        'heartbeat: session active': (r) => {
          try {
            const body = r.json()
            return body.session_active !== false && body.sessionActive !== false
          } catch {
            return true // Assume active if parse fails
          }
        },
      })
    })
  }

  // Pace: ~5 seconds between iterations (with ±1s jitter)
  sleep(4 + Math.random() * 2)
}

// ---------------------------------------------------------------------------
// Teardown — Print Summary
// ---------------------------------------------------------------------------

export function teardown(data) {
  console.log('═══════════════════════════════════════════════════')
  console.log('  ARGUS AI — LOAD TEST COMPLETE')
  console.log('═══════════════════════════════════════════════════')
  console.log(`  Max VUs:    ${MAX_VUS}`)
  console.log(`  Backend:    ${BASE_URL}`)
  console.log(`  Auth:       ${data.jwt ? 'OK' : 'FAILED'}`)
  console.log('═══════════════════════════════════════════════════')
}
