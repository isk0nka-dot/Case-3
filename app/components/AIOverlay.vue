<!-- ==========================================================================
  Argus AI — AIOverlay Component
  ==========================================================================

  Real-time AI visualization overlay for the proctoring video feed. Renders:
    1. Face bounding box with confidence badge
    2. Head pose Euler angles (Yaw/Pitch/Roll) with deviation indicator
    3. Gaze direction vector
    4. Eye Aspect Ratio (blink detection) indicators
    5. Liveness score gauge
    6. Audio level meter + VAD + classification badge
    7. Multi-face detection warning
    8. FPS / inference latency HUD

  Architecture:
    This component is a pure presentation layer. It receives reactive state
    from useVisionEngine and useAudioEngine composables and renders SVG/CSS
    overlays on top of the video element.

    No inference happens here. All AI computation is in the composables.

  Usage:
    <AIOverlay
      :vision-frame="visionEngine.currentFrame.value"
      :audio-frame="audioEngine.currentFrame.value"
      :vision-stats="visionEngine.stats.value"
      :is-vision-active="visionEngine.isActive.value"
      :is-audio-active="audioEngine.isActive.value"
      :thresholds="visionEngine.thresholds"
    />
  ========================================================================== -->

<script setup lang="ts">
import { computed } from 'vue'
import type { VisionFrame, VisionStats } from '~/composables/useVisionEngine'
import type { AudioFrame } from '~/composables/useAudioEngine'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

const props = withDefaults(defineProps<{
  visionFrame: VisionFrame | null
  audioFrame: AudioFrame | null
  visionStats?: VisionStats | null
  isVisionActive: boolean
  isAudioActive: boolean
  thresholds?: {
    headPose: { yaw: number, pitch: number, roll: number }
    gaze: number
    blinkEAR: number
  }
  showDebug?: boolean
}>(), {
  visionStats: null,
  thresholds: () => ({ headPose: { yaw: 25, pitch: 20, roll: 15 }, gaze: 15, blinkEAR: 0.21 }),
  showDebug: false
})

// ---------------------------------------------------------------------------
// Computed — Face Bounding Box
// ---------------------------------------------------------------------------

const faceBBox = computed(() => {
  if (!props.visionFrame?.faceBBox) return null
  const { x, y, w, h } = props.visionFrame.faceBBox
  return {
    left: `${x * 100}%`,
    top: `${y * 100}%`,
    width: `${w * 100}%`,
    height: `${h * 100}%`
  }
})

const faceDetected = computed(() => (props.visionFrame?.faceCount ?? 0) > 0)
const multiFace = computed(() => (props.visionFrame?.faceCount ?? 0) > 1)

// ---------------------------------------------------------------------------
// Computed — Head Pose Status
// ---------------------------------------------------------------------------

const headPoseStatus = computed(() => {
  if (!props.visionFrame) return 'none'
  const { yaw, pitch, roll } = props.visionFrame.headPose
  const t = props.thresholds.headPose
  if (Math.abs(yaw) > t.yaw || Math.abs(pitch) > t.pitch || Math.abs(roll) > t.roll) {
    return 'anomaly'
  }
  if (Math.abs(yaw) > t.yaw * 0.7 || Math.abs(pitch) > t.pitch * 0.7) {
    return 'warning'
  }
  return 'normal'
})

const headPoseColor = computed(() => {
  switch (headPoseStatus.value) {
    case 'anomaly': return 'var(--argus-error)'
    case 'warning': return 'var(--argus-warning)'
    default: return 'var(--argus-success)'
  }
})

// ---------------------------------------------------------------------------
// Computed — Gaze Vector
// ---------------------------------------------------------------------------

const gazeStatus = computed(() => {
  if (!props.visionFrame) return 'center'
  return props.visionFrame.gaze.direction
})

const gazeDeviation = computed(() => {
  if (!props.visionFrame) return false
  return props.visionFrame.gaze.direction !== 'center'
})

// Gaze indicator position (normalized 0-1 → percentage)
const gazeIndicator = computed(() => {
  if (!props.visionFrame) return { x: '50%', y: '50%' }
  return {
    x: `${props.visionFrame.gaze.x * 100}%`,
    y: `${props.visionFrame.gaze.y * 100}%`
  }
})

// ---------------------------------------------------------------------------
// Computed — Liveness
// ---------------------------------------------------------------------------

const livenessScore = computed(() => props.visionFrame?.livenessScore ?? 0)

const livenessColor = computed(() => {
  const s = livenessScore.value
  if (s >= 0.7) return 'var(--argus-success)'
  if (s >= 0.4) return 'var(--argus-warning)'
  return 'var(--argus-error)'
})

// ---------------------------------------------------------------------------
// Computed — Blink State
// ---------------------------------------------------------------------------

const blinkActive = computed(() => props.visionFrame?.blink.isBlinking ?? false)

// ---------------------------------------------------------------------------
// Computed — Audio Analysis
// ---------------------------------------------------------------------------

const audioLevel = computed(() => {
  if (!props.audioFrame) return 0
  // Normalize dB to 0-100 visual range (-60dB=0%, 0dB=100%)
  return Math.max(0, Math.min(100, ((props.audioFrame.rmsDb + 60) / 60) * 100))
})

const audioColor = computed(() => {
  if (!props.audioFrame) return 'var(--argus-text-dimmed)'
  switch (props.audioFrame.classification) {
    case 'speech': return 'var(--argus-success)'
    case 'whisper': return 'var(--argus-warning)'
    case 'music': return 'var(--argus-accent)'
    case 'keyboard': return 'var(--argus-text-dimmed)'
    case 'ambient': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
})

const vadActive = computed(() => props.audioFrame?.vadActive ?? false)

// ---------------------------------------------------------------------------
// Computed — Waveform SVG path
// ---------------------------------------------------------------------------

const waveformPath = computed(() => {
  if (!props.audioFrame?.waveform) return ''
  const w = props.audioFrame.waveform
  const width = 120
  const height = 32
  const mid = height / 2
  const step = width / w.length
  let d = `M 0 ${mid}`
  for (let i = 0; i < w.length; i++) {
    const x = i * step
    const y = mid + (w[i] ?? 0) * mid * 3 // amplify for visibility
    d += ` L ${x.toFixed(1)} ${Math.max(0, Math.min(height, y)).toFixed(1)}`
  }
  return d
})

// ---------------------------------------------------------------------------
// Computed — Face bbox border color based on state
// ---------------------------------------------------------------------------

const bboxBorderColor = computed(() => {
  if (multiFace.value) return 'var(--argus-error)'
  if (headPoseStatus.value === 'anomaly') return 'var(--argus-error)'
  if (gazeDeviation.value) return 'var(--argus-warning)'
  return 'var(--argus-success)'
})

// ---------------------------------------------------------------------------
// Computed — Inference FPS
// ---------------------------------------------------------------------------

const inferenceMs = computed(() => props.visionFrame?.inferenceMs?.toFixed(1) ?? '—')
</script>

<template>
  <div class="ai-overlay">
    <!-- =================================================================
      FACE BOUNDING BOX
    ================================================================== -->
    <div
      v-if="faceBBox && faceDetected"
      class="ai-face-bbox"
      :style="{
        left: faceBBox.left,
        top: faceBBox.top,
        width: faceBBox.width,
        height: faceBBox.height,
        borderColor: bboxBorderColor
      }"
    >
      <!-- Corner markers (tactical HUD style) -->
      <span
        class="ai-corner ai-corner-tl"
        :style="{ borderColor: bboxBorderColor }"
      />
      <span
        class="ai-corner ai-corner-tr"
        :style="{ borderColor: bboxBorderColor }"
      />
      <span
        class="ai-corner ai-corner-bl"
        :style="{ borderColor: bboxBorderColor }"
      />
      <span
        class="ai-corner ai-corner-br"
        :style="{ borderColor: bboxBorderColor }"
      />

      <!-- Face ID Badge (top-left) -->
      <div class="ai-badge ai-badge-tl">
        <span
          class="ai-badge-dot"
          :style="{ background: bboxBorderColor }"
        />
        <span class="ai-badge-text">
          {{ multiFace ? `${visionFrame?.faceCount} ЛИЦА` : 'ЛИЦО' }}
        </span>
      </div>

      <!-- Liveness Badge (top-right) -->
      <div
        class="ai-badge ai-badge-tr"
        :style="{ color: livenessColor }"
      >
        {{ (livenessScore * 100).toFixed(0) }}%
      </div>
    </div>

    <!-- No face warning -->
    <div
      v-if="isVisionActive && !faceDetected && visionFrame"
      class="ai-no-face"
    >
      <div class="ai-no-face-icon">
        <span class="i-lucide-user-x" />
      </div>
      <span>ЛИЦО НЕ ОБНАРУЖЕНО</span>
    </div>

    <!-- Multi-face warning -->
    <div
      v-if="multiFace"
      class="ai-multi-face-warning"
    >
      <span class="i-lucide-users" />
      <span>{{ visionFrame?.faceCount }} ЛИЦА ОБНАРУЖЕНЫ</span>
    </div>

    <!-- =================================================================
      GAZE INDICATOR (small crosshair at gaze point)
    ================================================================== -->
    <div
      v-if="faceDetected && gazeStatus !== 'center'"
      class="ai-gaze-indicator"
      :class="{ 'ai-gaze-deviated': gazeDeviation }"
      :style="{ left: gazeIndicator.x, top: gazeIndicator.y }"
    >
      <span class="ai-gaze-ring" />
      <span class="ai-gaze-dot" />
    </div>

    <!-- =================================================================
      HEAD-UP DISPLAY (bottom-left)
    ================================================================== -->
    <div
      v-if="isVisionActive && visionFrame"
      class="ai-hud ai-hud-bl"
    >
      <!-- Head Pose -->
      <div
        class="ai-hud-row"
        :style="{ color: headPoseColor }"
      >
        <span class="i-lucide-rotate-3d ai-hud-icon" />
        <span class="ai-hud-label">HPE</span>
        <span class="ai-hud-value">
          Y:{{ visionFrame.headPose.yaw.toFixed(0) }}°
          P:{{ visionFrame.headPose.pitch.toFixed(0) }}°
          R:{{ visionFrame.headPose.roll.toFixed(0) }}°
        </span>
      </div>

      <!-- Gaze Direction -->
      <div
        class="ai-hud-row"
        :style="{ color: gazeDeviation ? 'var(--argus-warning)' : 'var(--argus-success)' }"
      >
        <span class="i-lucide-eye ai-hud-icon" />
        <span class="ai-hud-label">GAZE</span>
        <span class="ai-hud-value">
          {{ gazeStatus.toUpperCase() }}
          <span
            v-if="visionFrame.gaze.angleDegrees > 0"
            class="ai-hud-dim"
          >
            {{ visionFrame.gaze.angleDegrees.toFixed(0) }}°
          </span>
        </span>
      </div>

      <!-- Blink Detection -->
      <div class="ai-hud-row">
        <span class="i-lucide-scan-face ai-hud-icon" />
        <span class="ai-hud-label">EAR</span>
        <span
          class="ai-hud-value"
          :class="{ 'ai-blink-flash': blinkActive }"
        >
          {{ ((visionFrame.blink.leftEAR + visionFrame.blink.rightEAR) / 2).toFixed(2) }}
          <span class="ai-hud-dim">
            {{ visionFrame.blink.blinkRatePerMin.toFixed(0) }}/мин
          </span>
        </span>
      </div>

      <!-- Liveness -->
      <div
        class="ai-hud-row"
        :style="{ color: livenessColor }"
      >
        <span class="i-lucide-shield-check ai-hud-icon" />
        <span class="ai-hud-label">LIVE</span>
        <div class="ai-liveness-bar">
          <div
            class="ai-liveness-fill"
            :style="{ width: `${livenessScore * 100}%`, background: livenessColor }"
          />
        </div>
        <span class="ai-hud-value">{{ (livenessScore * 100).toFixed(0) }}%</span>
      </div>
    </div>

    <!-- =================================================================
      AUDIO HUD (bottom-right)
    ================================================================== -->
    <div
      v-if="isAudioActive && audioFrame"
      class="ai-hud ai-hud-br"
    >
      <!-- Audio Level Meter -->
      <div class="ai-hud-row">
        <span
          class="i-lucide-volume-2 ai-hud-icon"
          :style="{ color: audioColor }"
        />
        <span class="ai-hud-label">RMS</span>
        <div class="ai-audio-meter">
          <div
            class="ai-audio-meter-fill"
            :style="{ width: `${audioLevel}%`, background: audioColor }"
          />
        </div>
        <span
          class="ai-hud-value"
          :style="{ color: audioColor }"
        >
          {{ audioFrame.rmsDb.toFixed(0) }}dB
        </span>
      </div>

      <!-- VAD Status -->
      <div class="ai-hud-row">
        <span
          class="ai-hud-icon"
          :class="vadActive ? 'i-lucide-mic' : 'i-lucide-mic-off'"
          :style="{ color: vadActive ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
        />
        <span class="ai-hud-label">VAD</span>
        <span
          class="ai-hud-value"
          :style="{ color: vadActive ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
        >
          {{ vadActive ? 'АКТИВНО' : 'ТИХО' }}
        </span>
      </div>

      <!-- Classification -->
      <div class="ai-hud-row">
        <span
          class="i-lucide-audio-waveform ai-hud-icon"
          :style="{ color: audioColor }"
        />
        <span class="ai-hud-label">CLS</span>
        <span
          class="ai-hud-value ai-classification-badge"
          :style="{ color: audioColor }"
        >
          {{ audioFrame.classification.toUpperCase() }}
        </span>
        <span
          v-if="audioFrame.speakerCount > 1"
          class="ai-speaker-count"
        >
          ×{{ audioFrame.speakerCount }}
        </span>
      </div>

      <!-- Waveform -->
      <div class="ai-waveform-container">
        <svg
          viewBox="0 0 120 32"
          preserveAspectRatio="none"
          class="ai-waveform-svg"
        >
          <path
            :d="waveformPath"
            fill="none"
            :stroke="audioColor"
            stroke-width="1.2"
            stroke-linecap="round"
          />
        </svg>
      </div>
    </div>

    <!-- =================================================================
      DEBUG HUD (top-right, optional)
    ================================================================== -->
    <div
      v-if="showDebug && visionFrame"
      class="ai-hud ai-hud-debug"
    >
      <div class="ai-hud-row">
        <span class="ai-hud-label">INF</span>
        <span class="ai-hud-value">{{ inferenceMs }}ms</span>
      </div>
      <div class="ai-hud-row">
        <span class="ai-hud-label">FRM</span>
        <span class="ai-hud-value">{{ visionStats?.totalFrames ?? 0 }}</span>
      </div>
      <div class="ai-hud-row">
        <span class="ai-hud-label">QTY</span>
        <span class="ai-hud-value">{{ (visionFrame.frameQuality * 100).toFixed(0) }}%</span>
      </div>
    </div>

    <!-- =================================================================
      STATUS INDICATORS (top bar)
    ================================================================== -->
    <div class="ai-status-bar">
      <div
        class="ai-status-indicator"
        :class="isVisionActive ? 'ai-status-active' : 'ai-status-inactive'"
      >
        <span class="i-lucide-camera" />
        <span>AI Vision</span>
      </div>
      <div
        class="ai-status-indicator"
        :class="isAudioActive ? 'ai-status-active' : 'ai-status-inactive'"
      >
        <span class="i-lucide-mic" />
        <span>AI Audio</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ===================================================================
   AI Overlay — Root Container
   =================================================================== */

.ai-overlay {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 20;
  overflow: hidden;
  font-family: 'JetBrains Mono', 'SF Mono', 'Fira Code', monospace;
}

/* ===================================================================
   Face Bounding Box
   =================================================================== */

.ai-face-bbox {
  position: absolute;
  border: 2px solid var(--argus-success);
  border-radius: 4px;
  transition: all 0.15s ease-out;
}

/* Corner markers — tactical HUD aesthetic */
.ai-corner {
  position: absolute;
  width: 12px;
  height: 12px;
  border-style: solid;
  border-color: inherit;
}
.ai-corner-tl { top: -2px; left: -2px; border-width: 3px 0 0 3px; border-radius: 2px 0 0 0; }
.ai-corner-tr { top: -2px; right: -2px; border-width: 3px 3px 0 0; border-radius: 0 2px 0 0; }
.ai-corner-bl { bottom: -2px; left: -2px; border-width: 0 0 3px 3px; border-radius: 0 0 0 2px; }
.ai-corner-br { bottom: -2px; right: -2px; border-width: 0 3px 3px 0; border-radius: 0 0 2px 0; }

/* Badges on face bbox */
.ai-badge {
  position: absolute;
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.05em;
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 4px;
}
.ai-badge-tl { top: -20px; left: 0; }
.ai-badge-tr { top: -20px; right: 0; }
.ai-badge-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  animation: ai-pulse 2s ease-in-out infinite;
}
.ai-badge-text { color: rgba(255, 255, 255, 0.85); }

/* ===================================================================
   No Face / Multi-Face Warnings
   =================================================================== */

.ai-no-face {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--argus-error);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.1em;
  animation: ai-pulse 1.5s ease-in-out infinite;
}
.ai-no-face-icon {
  font-size: 32px;
  opacity: 0.8;
}

.ai-multi-face-warning {
  position: absolute;
  top: 8px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(239, 68, 68, 0.85);
  color: white;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.08em;
  padding: 4px 10px;
  border-radius: 4px;
  animation: ai-pulse 1s ease-in-out infinite;
}

/* ===================================================================
   Gaze Indicator
   =================================================================== */

.ai-gaze-indicator {
  position: absolute;
  width: 16px;
  height: 16px;
  transform: translate(-50%, -50%);
  transition: left 0.1s ease-out, top 0.1s ease-out;
}
.ai-gaze-ring {
  position: absolute;
  inset: 0;
  border: 1.5px solid var(--argus-accent);
  border-radius: 50%;
  opacity: 0.6;
}
.ai-gaze-dot {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 4px;
  height: 4px;
  background: var(--argus-accent);
  border-radius: 50%;
  transform: translate(-50%, -50%);
}
.ai-gaze-deviated .ai-gaze-ring {
  border-color: var(--argus-warning);
  animation: ai-gaze-warn 0.6s ease-in-out infinite;
}
.ai-gaze-deviated .ai-gaze-dot {
  background: var(--argus-warning);
}

/* ===================================================================
   Head-Up Display (HUD) Panels
   =================================================================== */

.ai-hud {
  position: absolute;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  border-radius: 6px;
  padding: 6px 8px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 160px;
}
.ai-hud-bl { bottom: 8px; left: 8px; }
.ai-hud-br { bottom: 8px; right: 8px; }
.ai-hud-debug { top: 36px; right: 8px; min-width: 80px; opacity: 0.7; }

.ai-hud-row {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  color: rgba(255, 255, 255, 0.8);
  line-height: 1.4;
}
.ai-hud-icon {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
  opacity: 0.7;
}
.ai-hud-label {
  font-weight: 700;
  font-size: 9px;
  letter-spacing: 0.08em;
  opacity: 0.5;
  min-width: 30px;
}
.ai-hud-value {
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}
.ai-hud-dim {
  opacity: 0.5;
  font-size: 9px;
  margin-left: 2px;
}

/* ===================================================================
   Liveness Bar
   =================================================================== */

.ai-liveness-bar {
  width: 40px;
  height: 4px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
  overflow: hidden;
  flex-shrink: 0;
}
.ai-liveness-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease-out, background 0.3s ease-out;
}

/* ===================================================================
   Audio Meter
   =================================================================== */

.ai-audio-meter {
  width: 40px;
  height: 4px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
  overflow: hidden;
  flex-shrink: 0;
}
.ai-audio-meter-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.1s linear;
}

.ai-classification-badge {
  font-weight: 600;
  font-size: 9px;
  letter-spacing: 0.05em;
}
.ai-speaker-count {
  font-size: 9px;
  color: var(--argus-error);
  font-weight: 700;
}

/* ===================================================================
   Waveform
   =================================================================== */

.ai-waveform-container {
  width: 100%;
  height: 24px;
  margin-top: 2px;
}
.ai-waveform-svg {
  width: 100%;
  height: 100%;
}

/* ===================================================================
   Blink Flash
   =================================================================== */

.ai-blink-flash {
  animation: ai-blink 0.15s ease-out;
}

/* ===================================================================
   Status Bar (top)
   =================================================================== */

.ai-status-bar {
  position: absolute;
  top: 6px;
  left: 6px;
  display: flex;
  gap: 6px;
}
.ai-status-indicator {
  display: flex;
  align-items: center;
  gap: 3px;
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.06em;
  padding: 2px 6px;
  border-radius: 3px;
  background: rgba(0, 0, 0, 0.5);
}
.ai-status-active {
  color: var(--argus-success);
}
.ai-status-active::before {
  content: '';
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--argus-success);
  animation: ai-pulse 2s ease-in-out infinite;
}
.ai-status-inactive {
  color: var(--argus-text-dimmed);
  opacity: 0.5;
}

/* ===================================================================
   Animations
   =================================================================== */

@keyframes ai-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

@keyframes ai-blink {
  0% { color: white; }
  100% { color: inherit; }
}

@keyframes ai-gaze-warn {
  0%, 100% { transform: scale(1); opacity: 0.6; }
  50% { transform: scale(1.3); opacity: 1; }
}
</style>
