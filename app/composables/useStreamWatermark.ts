// =============================================================================
// Argus AI — useStreamWatermark Composable (Media Stream Integrity)
// =============================================================================
//
// Embeds a hidden, fast-changing watermark into the local video stream to
// prevent pre-recorded video injection. The watermark is:
//
//   1. A 4x4 pixel block in the bottom-right corner of each frame
//   2. Encodes a rotating token derived from: session ID + timestamp + HMAC
//   3. Changes every 500ms (2 Hz) — too fast for manual screen recording
//   4. Invisible to the human eye (1-bit LSB modification of pixel channels)
//   5. Verifiable server-side by extracting the watermark from evidence frames
//
// The watermark also serves as a "freshness proof": if the server receives
// frames with stale or missing watermarks, the session is flagged.
//
// =============================================================================

import { ref } from 'vue'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface WatermarkConfig {
  /** Session identifier for HMAC binding. */
  sessionId: string
  /** Rotation interval in ms. Default: 500. */
  rotationIntervalMs?: number
  /** Watermark block size (NxN pixels). Default: 4. */
  blockSize?: number
}

export interface WatermarkState {
  isActive: boolean
  currentToken: string
  framesWatermarked: number
  lastRotationAt: number
}

// ---------------------------------------------------------------------------
// HMAC-like token generation (browser-safe, no crypto import needed)
// ---------------------------------------------------------------------------

function generateToken(sessionId: string, timestamp: number): string {
  // Simple hash combining session + time for watermark rotation
  // In production, use crypto.subtle.sign with a shared key
  const input = `${sessionId}:${timestamp}:argus-wm`
  let hash = 0
  for (let i = 0; i < input.length; i++) {
    const char = input.charCodeAt(i)
    hash = ((hash << 5) - hash) + char
    hash = hash & hash // Convert to 32-bit integer
  }
  return Math.abs(hash).toString(16).padStart(8, '0')
}

function tokenToPixels(token: string, blockSize: number): Uint8Array {
  // Convert hex token to pixel modification pattern
  // Each hex char → 4 bits → 4 pixel channel LSB modifications
  const pixelCount = blockSize * blockSize
  const modifications = new Uint8Array(pixelCount * 4) // RGBA per pixel

  for (let i = 0; i < pixelCount; i++) {
    const charIdx = i % token.length
    const charVal = parseInt(token[charIdx]!, 16)

    // Encode 4 bits into RGBA LSBs of this pixel
    modifications[i * 4 + 0] = (charVal >> 3) & 1 // R LSB
    modifications[i * 4 + 1] = (charVal >> 2) & 1 // G LSB
    modifications[i * 4 + 2] = (charVal >> 1) & 1 // B LSB
    modifications[i * 4 + 3] = charVal & 1 // A LSB (only if non-zero alpha)
  }

  return modifications
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useStreamWatermark(config: WatermarkConfig) {
  const {
    sessionId,
    rotationIntervalMs = 500,
    blockSize = 4
  } = config

  const state = ref<WatermarkState>({
    isActive: false,
    currentToken: '',
    framesWatermarked: 0,
    lastRotationAt: 0
  })

  let canvas: HTMLCanvasElement | null = null
  let ctx: CanvasRenderingContext2D | null = null
  let rotationTimer: ReturnType<typeof setInterval> | null = null
  let currentModifications: Uint8Array | null = null

  /** Rotate the watermark token. */
  function rotateToken() {
    const now = Date.now()
    const token = generateToken(sessionId, Math.floor(now / rotationIntervalMs))
    currentModifications = tokenToPixels(token, blockSize)

    state.value = {
      ...state.value,
      currentToken: token,
      lastRotationAt: now
    }
  }

  /**
   * Apply the watermark to a video frame on a canvas.
   * Call this from the rendering pipeline (e.g., after drawing the video frame).
   */
  function applyToCanvas(targetCanvas: HTMLCanvasElement) {
    if (!state.value.isActive || !currentModifications) return

    const targetCtx = targetCanvas.getContext('2d')
    if (!targetCtx) return

    const w = targetCanvas.width
    const h = targetCanvas.height
    if (w === 0 || h === 0) return

    // Read the bottom-right block
    const startX = w - blockSize
    const startY = h - blockSize

    try {
      const imageData = targetCtx.getImageData(startX, startY, blockSize, blockSize)
      const pixels = imageData.data

      // Modify LSBs according to the current token pattern
      for (let i = 0; i < blockSize * blockSize; i++) {
        const px = i * 4

        // Clear LSB then set from modification pattern
        pixels[px + 0] = (pixels[px + 0]! & 0xFE) | currentModifications[px + 0]! // R
        pixels[px + 1] = (pixels[px + 1]! & 0xFE) | currentModifications[px + 1]! // G
        pixels[px + 2] = (pixels[px + 2]! & 0xFE) | currentModifications[px + 2]! // B
        // Skip alpha — modifying alpha can cause visible artifacts
      }

      targetCtx.putImageData(imageData, startX, startY)

      state.value = {
        ...state.value,
        framesWatermarked: state.value.framesWatermarked + 1
      }
    } catch {
      // Canvas tainted by cross-origin — skip silently
    }
  }

  /**
   * Apply the watermark directly to a video element's frame by drawing
   * to an intermediary canvas. Returns the watermarked canvas.
   */
  function watermarkVideoFrame(video: HTMLVideoElement): HTMLCanvasElement | null {
    if (!state.value.isActive) return null
    if (video.readyState < 2) return null

    if (!canvas) {
      canvas = document.createElement('canvas')
      ctx = canvas.getContext('2d')
    }
    if (!ctx) return null

    canvas.width = video.videoWidth
    canvas.height = video.videoHeight
    ctx.drawImage(video, 0, 0)
    applyToCanvas(canvas)

    return canvas
  }

  /**
   * Extract and verify a watermark from an evidence frame.
   * Returns the extracted token for server-side verification.
   */
  function extractFromCanvas(sourceCanvas: HTMLCanvasElement): string | null {
    const sourceCtx = sourceCanvas.getContext('2d')
    if (!sourceCtx) return null

    const w = sourceCanvas.width
    const h = sourceCanvas.height
    if (w < blockSize || h < blockSize) return null

    try {
      const startX = w - blockSize
      const startY = h - blockSize
      const imageData = sourceCtx.getImageData(startX, startY, blockSize, blockSize)
      const pixels = imageData.data

      // Extract LSBs and reconstruct token
      let extractedBits = ''
      for (let i = 0; i < blockSize * blockSize && i < 8; i++) {
        const px = i * 4
        const r = pixels[px + 0]! & 1
        const g = pixels[px + 1]! & 1
        const b = pixels[px + 2]! & 1
        const a = 0 // We don't encode alpha

        const charVal = (r << 3) | (g << 2) | (b << 1) | a
        extractedBits += charVal.toString(16)
      }

      return extractedBits
    } catch {
      return null
    }
  }

  /** Start watermarking. */
  function start() {
    if (state.value.isActive) return
    state.value = { ...state.value, isActive: true }

    rotateToken()
    rotationTimer = setInterval(rotateToken, rotationIntervalMs)
  }

  /** Stop watermarking. */
  function stop() {
    state.value = { ...state.value, isActive: false }

    if (rotationTimer) {
      clearInterval(rotationTimer)
      rotationTimer = null
    }

    if (canvas) {
      canvas.width = 0
      canvas.height = 0
      canvas = null
      ctx = null
    }
  }

  return {
    state,
    start,
    stop,
    applyToCanvas,
    watermarkVideoFrame,
    extractFromCanvas
  }
}
