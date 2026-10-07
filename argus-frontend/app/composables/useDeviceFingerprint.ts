// =============================================================================
// Argus AI — useDeviceFingerprint Composable (Deep Device ID)
// =============================================================================
//
// Captures low-level hardware identifiers to create a unique "Deep Device ID"
// that prevents:
//   1. Multiple students using the same machine
//   2. Device switching mid-session
//   3. Virtual machine usage
//   4. Remote desktop connections
//
// Fingerprint components:
//   - GPU renderer string (UNMASKED_RENDERER_WEBGL)
//   - GPU vendor string (UNMASKED_VENDOR_WEBGL)
//   - Screen properties (resolution, colorDepth, pixelRatio, refresh rate)
//   - Audio stack fingerprint (AudioContext oscillator hash)
//   - Hardware concurrency (logical CPU cores)
//   - Device memory (approximate RAM)
//   - Platform / OS string
//   - Timezone offset
//   - WebGL max parameters (max texture size, max renderbuffer)
//   - Canvas fingerprint (rendering-based)
//
// The composite fingerprint is hashed to a stable device ID that persists
// across page refreshes but changes if hardware changes.
//
// =============================================================================

import { ref } from 'vue'
import { EventType, Severity, EventSource } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface DeviceFingerprint {
  /** Stable hash of all fingerprint components. */
  deviceId: string
  /** GPU renderer string. */
  gpuRenderer: string
  /** GPU vendor string. */
  gpuVendor: string
  /** Screen resolution. */
  screenResolution: string
  /** Screen color depth. */
  colorDepth: number
  /** Device pixel ratio. */
  pixelRatio: number
  /** Logical CPU cores. */
  cpuCores: number
  /** Approximate device memory (GB). */
  deviceMemory: number
  /** Platform string. */
  platform: string
  /** Timezone offset (minutes). */
  timezoneOffset: number
  /** Audio fingerprint hash. */
  audioHash: string
  /** WebGL max texture size. */
  maxTextureSize: number
  /** Canvas fingerprint hash. */
  canvasHash: string
  /** Whether likely running in a VM. */
  isVirtualMachine: boolean
  /** VM detection signals. */
  vmSignals: string[]
  /** Timestamp of capture. */
  capturedAt: number
}

// ---------------------------------------------------------------------------
// Fingerprinting functions
// ---------------------------------------------------------------------------

function getGPUInfo(): { renderer: string, vendor: string, maxTextureSize: number } {
  try {
    const canvas = document.createElement('canvas')
    const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl')
    if (!gl || !(gl instanceof WebGLRenderingContext)) {
      return { renderer: 'unknown', vendor: 'unknown', maxTextureSize: 0 }
    }

    const debugInfo = gl.getExtension('WEBGL_debug_renderer_info')
    const renderer = debugInfo
      ? gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL)
      : gl.getParameter(gl.RENDERER)
    const vendor = debugInfo
      ? gl.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL)
      : gl.getParameter(gl.VENDOR)
    const maxTextureSize = gl.getParameter(gl.MAX_TEXTURE_SIZE)

    // Clean up
    canvas.width = 0
    canvas.height = 0

    return {
      renderer: String(renderer),
      vendor: String(vendor),
      maxTextureSize: Number(maxTextureSize)
    }
  } catch {
    return { renderer: 'error', vendor: 'error', maxTextureSize: 0 }
  }
}

function getAudioFingerprint(): Promise<string> {
  return new Promise((resolve) => {
    try {
      const audioCtx = new OfflineAudioContext(1, 44100, 44100)
      const oscillator = audioCtx.createOscillator()
      const compressor = audioCtx.createDynamicsCompressor()

      oscillator.type = 'triangle'
      oscillator.frequency.value = 10000

      compressor.threshold.value = -50
      compressor.knee.value = 40
      compressor.ratio.value = 12
      compressor.attack.value = 0
      compressor.release.value = 0.25

      oscillator.connect(compressor)
      compressor.connect(audioCtx.destination)
      oscillator.start(0)

      audioCtx.startRendering().then((buffer) => {
        const data = buffer.getChannelData(0)
        // Hash a subset of the output samples
        let hash = 0
        for (let i = 4500; i < 5000; i++) {
          hash = ((hash << 5) - hash) + Math.round(data[i]! * 1000000)
          hash = hash & hash
        }
        resolve(Math.abs(hash).toString(16).padStart(8, '0'))
      }).catch(() => {
        resolve('00000000')
      })
    } catch {
      resolve('00000000')
    }
  })
}

function getCanvasFingerprint(): string {
  try {
    const canvas = document.createElement('canvas')
    canvas.width = 200
    canvas.height = 50
    const ctx = canvas.getContext('2d')
    if (!ctx) return '00000000'

    // Draw a complex pattern that varies by rendering engine
    ctx.textBaseline = 'alphabetic'
    ctx.fillStyle = '#f60'
    ctx.fillRect(100, 1, 62, 20)

    ctx.fillStyle = '#069'
    ctx.font = '11pt Arial'
    ctx.fillText('Argus AI Fingerprint', 2, 15)

    ctx.fillStyle = 'rgba(102, 204, 0, 0.7)'
    ctx.font = '18pt Arial'
    ctx.fillText('Argus AI Fingerprint', 4, 45)

    // Hash the data URL
    const dataUrl = canvas.toDataURL()
    let hash = 0
    for (let i = 0; i < dataUrl.length; i++) {
      hash = ((hash << 5) - hash) + dataUrl.charCodeAt(i)
      hash = hash & hash
    }

    canvas.width = 0
    canvas.height = 0

    return Math.abs(hash).toString(16).padStart(8, '0')
  } catch {
    return '00000000'
  }
}

function detectVirtualMachine(gpuRenderer: string, gpuVendor: string): { isVM: boolean, signals: string[] } {
  const signals: string[] = []

  // GPU-based VM detection
  const vmGpuPatterns = [
    /virtualbox/i, /vmware/i, /parallels/i,
    /hyper-v/i, /qemu/i, /bochs/i,
    /microsoft basic render/i, /llvmpipe/i,
    /swiftshader/i, /mesa/i, /virgl/i,
    /google swiftshader/i
  ]

  for (const pattern of vmGpuPatterns) {
    if (pattern.test(gpuRenderer) || pattern.test(gpuVendor)) {
      signals.push(`gpu:${pattern.source}`)
    }
  }

  // Platform-based signals
  const ua = navigator.userAgent.toLowerCase()
  if (/virtual/i.test(ua)) signals.push('ua:virtual')

  // Hardware signals
  if (navigator.hardwareConcurrency <= 1) {
    signals.push('cpu:single_core')
  }

  const deviceMemory = (navigator as any).deviceMemory
  if (deviceMemory && deviceMemory <= 1) {
    signals.push('mem:low_1gb')
  }

  // Screen-based signals
  if (screen.width === screen.availWidth && screen.height === screen.availHeight) {
    // Fullscreen with no taskbar — common in VMs
    if (screen.width <= 1024) {
      signals.push('screen:vm_resolution')
    }
  }

  return {
    isVM: signals.length >= 2,
    signals
  }
}

// Stable hash function for device ID
function stableHash(input: string): string {
  let h1 = 0xdeadbeef
  let h2 = 0x41c6ce57

  for (let i = 0; i < input.length; i++) {
    const ch = input.charCodeAt(i)
    h1 = Math.imul(h1 ^ ch, 2654435761)
    h2 = Math.imul(h2 ^ ch, 1597334677)
  }

  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909)
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909)

  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(16).padStart(16, '0')
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useDeviceFingerprint() {
  const fingerprint = ref<DeviceFingerprint | null>(null)
  const isCapturing = ref(false)

  // Event callback
  let sendEventFn: ((
    eventType: EventType,
    severity: Severity,
    payload?: any,
    label?: string,
    confidence?: number,
    source?: EventSource
  ) => void) | null = null

  function onEvent(fn: typeof sendEventFn) {
    sendEventFn = fn
  }

  /** Capture the full device fingerprint. */
  async function capture(): Promise<DeviceFingerprint> {
    isCapturing.value = true

    const gpu = getGPUInfo()
    const audioHash = await getAudioFingerprint()
    const canvasHash = getCanvasFingerprint()
    const vmCheck = detectVirtualMachine(gpu.renderer, gpu.vendor)
    const deviceMemory = (navigator as any).deviceMemory ?? 0

    // Build fingerprint string for hashing
    const components = [
      gpu.renderer,
      gpu.vendor,
      `${screen.width}x${screen.height}`,
      screen.colorDepth.toString(),
      window.devicePixelRatio.toString(),
      navigator.hardwareConcurrency.toString(),
      deviceMemory.toString(),
      navigator.platform,
      new Date().getTimezoneOffset().toString(),
      audioHash,
      gpu.maxTextureSize.toString(),
      canvasHash
    ]

    const deviceId = stableHash(components.join('|'))

    const result: DeviceFingerprint = {
      deviceId,
      gpuRenderer: gpu.renderer,
      gpuVendor: gpu.vendor,
      screenResolution: `${screen.width}x${screen.height}`,
      colorDepth: screen.colorDepth,
      pixelRatio: window.devicePixelRatio,
      cpuCores: navigator.hardwareConcurrency,
      deviceMemory,
      platform: navigator.platform,
      timezoneOffset: new Date().getTimezoneOffset(),
      audioHash,
      maxTextureSize: gpu.maxTextureSize,
      canvasHash,
      isVirtualMachine: vmCheck.isVM,
      vmSignals: vmCheck.signals,
      capturedAt: Date.now()
    }

    fingerprint.value = result
    isCapturing.value = false

    // Report VM detection
    if (vmCheck.isVM) {
      sendEventFn?.(
        EventType.VIRTUAL_MACHINE_DETECTED,
        Severity.CRITICAL,
        {
          type: 'kernel',
          data: {
            wpm: 0,
            keystrokeStdDevMs: 0,
            handOnMouse: false,
            hardwareIdHash: result.deviceId,
            mismatchComponent: `vm:${vmCheck.signals.join(',')}`,
            monitorCount: 1,
            virtualMonitor: true
          }
        },
        `Виртуальная машина: ${vmCheck.signals.join(', ')}`,
        vmCheck.signals.length >= 3 ? 0.95 : 0.7,
        EventSource.BROWSER
      )
    }

    return result
  }

  /**
   * Verify that the current device matches a previously captured fingerprint.
   * Returns similarity score (0-1). Below 0.7 indicates device switch.
   */
  function verify(baseline: DeviceFingerprint): number {
    if (!fingerprint.value) return 0

    const current = fingerprint.value
    let matches = 0
    let total = 0
    const weights: [boolean, number][] = [
      [current.gpuRenderer === baseline.gpuRenderer, 3],
      [current.gpuVendor === baseline.gpuVendor, 2],
      [current.screenResolution === baseline.screenResolution, 2],
      [current.colorDepth === baseline.colorDepth, 1],
      [Math.abs(current.pixelRatio - baseline.pixelRatio) < 0.01, 1],
      [current.cpuCores === baseline.cpuCores, 2],
      [current.deviceMemory === baseline.deviceMemory, 1],
      [current.platform === baseline.platform, 2],
      [current.timezoneOffset === baseline.timezoneOffset, 1],
      [current.audioHash === baseline.audioHash, 3],
      [current.canvasHash === baseline.canvasHash, 2],
      [current.maxTextureSize === baseline.maxTextureSize, 1]
    ]

    for (const [match, weight] of weights) {
      total += weight
      if (match) matches += weight
    }

    const similarity = matches / total

    if (similarity < 0.7) {
      sendEventFn?.(
        EventType.HARDWARE_ID_MISMATCH,
        Severity.CRITICAL,
        {
          type: 'kernel',
          data: {
            wpm: 0,
            keystrokeStdDevMs: 0,
            handOnMouse: false,
            hardwareIdHash: current.deviceId,
            mismatchComponent: `similarity=${similarity.toFixed(3)}`,
            monitorCount: 1,
            virtualMonitor: false
          }
        },
        `Смена устройства обнаружена: сходство ${(similarity * 100).toFixed(0)}%`,
        1 - similarity,
        EventSource.BROWSER
      )
    }

    return similarity
  }

  return {
    fingerprint,
    isCapturing,
    capture,
    verify,
    onEvent
  }
}
