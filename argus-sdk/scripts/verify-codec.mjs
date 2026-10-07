import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import ts from 'typescript';

const codecPath = resolve(process.cwd(), 'src/core/codec.ts');
const source = readFileSync(codecPath, 'utf8');

const transpiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ES2022,
    target: ts.ScriptTarget.ES2022,
    strict: true,
  },
});

const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled.outputText).toString('base64')}`;
const codec = await import(moduleUrl);

const event = {
  eventId: 'evt-test-1',
  sessionId: 'session-1',
  studentId: 'student-1',
  examId: 'exam-1',
  orgId: 'org-1',
  eventType: 3,
  severity: 2,
  source: 1,
  clientTimestamp: '2026-06-01T10:00:00.000Z',
  label: 'Face not detected',
  confidence: 0.91,
  payload: {
    type: 'audioAnalysis',
    data: {
      rmsDb: -32.5,
      vadConfidence: 0.82,
      classificationConfidence: 0.77,
    },
  },
};

const batch = codec.encodeIngestBatchRequest({
  events: [event],
  batchId: 'batch-1',
});

assert.equal(batch.batch_id, 'batch-1');
assert.equal(batch.events.length, 1);
assert.equal(batch.events[0].event_id, 'evt-test-1');
assert.equal(batch.events[0].session_id, 'session-1');
assert.equal(batch.events[0].client_timestamp, '2026-06-01T10:00:00.000Z');
assert.deepEqual(batch.events[0].audio_analysis, {
  rms_db: -32.5,
  vad_confidence: 0.82,
  classification_confidence: 0.77,
});

const heartbeat = codec.encodeHeartbeatRequest({
  sessionId: 'session-1',
  studentId: 'student-1',
  examId: 'exam-1',
  clientTimestamp: '2026-06-01T10:00:00.000Z',
  currentFocusScore: 88,
  violationCount: 2,
});

assert.deepEqual(heartbeat, {
  session_id: 'session-1',
  student_id: 'student-1',
  exam_id: 'exam-1',
  client_timestamp: '2026-06-01T10:00:00.000Z',
  current_focus_score: 88,
  violation_count: 2,
});

assert.deepEqual(codec.decodeResponse({
  accepted_count: 2,
  rejected_count: 1,
  rejected_event_ids: ['evt-bad'],
  batch_sequence: 42,
}), {
  acceptedCount: 2,
  rejectedCount: 1,
  rejectedEventIds: ['evt-bad'],
  batchSequence: 42,
});

assert.deepEqual(codec.decodeResponse({
  session_active: true,
  server_timestamp: '2026-06-01T10:00:01.000Z',
  directive: {
    telemetry_mode: 2,
    terminate: false,
    terminate_reason: '',
  },
}), {
  sessionActive: true,
  serverTimestamp: '2026-06-01T10:00:01.000Z',
  directive: {
    telemetryMode: 2,
    terminate: false,
    terminateReason: '',
  },
});

console.log('Verified SDK proto JSON codec encoding.');
