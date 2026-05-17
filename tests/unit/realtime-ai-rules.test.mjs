import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import ts from 'typescript'

function loadRulesModule() {
  const source = readFileSync(new URL('../../app/lib/ai/realtimeEventRules.ts', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022
    }
  }).outputText

  const module = { exports: {} }
  const fn = new Function('module', 'exports', compiled)
  fn(module, module.exports)
  return module.exports
}

test('vision rules rate-limit no-face and liveness events', () => {
  const {
    createDefaultRealtimeAIRuleState,
    evaluateVisionFrame
  } = loadRulesModule()

  const state = createDefaultRealtimeAIRuleState()
  const thresholds = {
    noFaceFrames: 2,
    eventCooldownMs: 5_000,
    gazeDeviationMinMs: 2_000,
    livenessThreshold: 0.4,
    headPoseYawDeg: 25,
    headPosePitchDeg: 20,
    headPoseRollDeg: 15
  }

  const first = evaluateVisionFrame({
    timestamp: 1_000,
    faceCount: 0,
    gaze: { x: 0.5, y: 0.5, direction: 'center', angleDegrees: 0 },
    headPose: { yaw: 0, pitch: 0, roll: 0 },
    livenessScore: 1
  }, thresholds, state, 1_000)
  assert.deepEqual(first.map(d => d.kind), ['gaze_telemetry'])

  const second = evaluateVisionFrame({
    timestamp: 1_100,
    faceCount: 0,
    gaze: { x: 0.5, y: 0.5, direction: 'center', angleDegrees: 0 },
    headPose: { yaw: 0, pitch: 0, roll: 0 },
    livenessScore: 1
  }, thresholds, state, 1_100)
  assert.deepEqual(second.map(d => d.kind), ['gaze_telemetry', 'face_not_detected'])

  const cooledDown = evaluateVisionFrame({
    timestamp: 1_200,
    faceCount: 0,
    gaze: { x: 0.5, y: 0.5, direction: 'center', angleDegrees: 0 },
    headPose: { yaw: 0, pitch: 0, roll: 0 },
    livenessScore: 1
  }, thresholds, state, 1_200)
  assert.deepEqual(cooledDown.map(d => d.kind), ['gaze_telemetry'])
})

test('vision rules require sustained gaze deviation before violation', () => {
  const {
    createDefaultRealtimeAIRuleState,
    evaluateVisionFrame
  } = loadRulesModule()

  const state = createDefaultRealtimeAIRuleState()
  const thresholds = {
    noFaceFrames: 2,
    eventCooldownMs: 5_000,
    gazeDeviationMinMs: 2_000,
    livenessThreshold: 0.4,
    headPoseYawDeg: 25,
    headPosePitchDeg: 20,
    headPoseRollDeg: 15
  }

  evaluateVisionFrame({
    timestamp: 1_000,
    faceCount: 1,
    gaze: { x: 0.1, y: 0.5, direction: 'left', angleDegrees: 22 },
    headPose: { yaw: 0, pitch: 0, roll: 0 },
    livenessScore: 1
  }, thresholds, state, 1_000)

  const decisions = evaluateVisionFrame({
    timestamp: 3_200,
    faceCount: 1,
    gaze: { x: 0.1, y: 0.5, direction: 'left', angleDegrees: 22 },
    headPose: { yaw: 0, pitch: 0, roll: 0 },
    livenessScore: 1
  }, thresholds, state, 3_200)

  assert.equal(decisions.some(d => d.kind === 'gaze_deviation' && d.durationMs === 2_200), true)
})

test('audio rules aggregate voice and noise anomalies with cooldowns', () => {
  const {
    createDefaultRealtimeAIRuleState,
    createDefaultRealtimeAudioThresholds,
    evaluateAudioFrame
  } = loadRulesModule()

  const state = createDefaultRealtimeAIRuleState()
  const thresholds = {
    ...createDefaultRealtimeAudioThresholds(),
    audioEventCooldownMs: 5_000,
    vadConfidenceThreshold: 0.65,
    noiseRmsDbThreshold: -25,
    audioClassificationConfidenceThreshold: 0.7
  }

  const first = evaluateAudioFrame({
    timestamp: 10_000,
    rmsDb: -18,
    peakDb: -12,
    vadActive: true,
    vadConfidence: 0.82,
    speechRatio: 0.55,
    speakerCount: 2,
    classification: 'whisper',
    classificationConfidence: 0.91
  }, thresholds, state, 10_000)

  assert.deepEqual(first.map(d => d.kind), [
    'audio_level_telemetry',
    'voice_activity',
    'audio_anomaly',
    'whisper_detected',
    'second_speaker_detected'
  ])

  const second = evaluateAudioFrame({
    timestamp: 10_100,
    rmsDb: -17,
    peakDb: -11,
    vadActive: true,
    vadConfidence: 0.9,
    speechRatio: 0.6,
    speakerCount: 2,
    classification: 'whisper',
    classificationConfidence: 0.95
  }, thresholds, state, 10_100)

  assert.deepEqual(second.map(d => d.kind), ['audio_level_telemetry'])
})
