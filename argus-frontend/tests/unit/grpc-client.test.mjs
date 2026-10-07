import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import ts from 'typescript'

function transpileModule(relativePath, requireMap = {}) {
  const source = readFileSync(new URL(relativePath, import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022
    }
  }).outputText

  const module = { exports: {} }
  const require = (id) => {
    if (id in requireMap) return requireMap[id]
    throw new Error(`Unexpected import in test loader: ${id}`)
  }
  const fn = new Function('module', 'exports', 'require', compiled)
  fn(module, module.exports, require)
  return module.exports
}

function loadTypesModule() {
  return transpileModule('../../app/lib/proto/types.ts')
}

function loadClientModule() {
  return transpileModule('../../app/lib/grpc/client.ts', {
    '../proto/types': loadTypesModule(),
    '../proto/codec': {
      encodeIngestEventRequest: req => req,
      encodeIngestBatchRequest: req => req,
      encodeHeartbeatRequest: req => req,
      decodeResponse: response => response
    }
  })
}

function buildEvent({ eventType, severity, source }) {
  return {
    eventId: `evt-${eventType}`,
    sessionId: 'session-1',
    studentId: 'student-1',
    examId: 'exam-1',
    orgId: 'org-1',
    eventType,
    severity,
    source,
    clientTimestamp: new Date(0).toISOString(),
    label: '',
    confidence: 1
  }
}

test('client batches AI audio telemetry in the telemetry buffer', async () => {
  const { EventCollectorClient } = loadClientModule()
  const { EventType, Severity, EventSource } = loadTypesModule()
  const transport = {
    unary: async () => ({
      acceptedCount: 1,
      rejectedCount: 0,
      rejectedEventIds: [],
      batchSequence: 1
    })
  }

  const client = new EventCollectorClient(transport, {
    maxBatchSize: 100,
    flushIntervalMs: 60_000,
    separateTelemetry: true
  })

  await client.queueEvent(buildEvent({
    eventType: EventType.AUDIO_LEVEL_TELEMETRY,
    severity: Severity.INFO,
    source: EventSource.BROWSER
  }))

  const stats = client.getBufferStats()
  assert.equal(stats.telemetry, 1)
  assert.equal(stats.violations, 0)
})
