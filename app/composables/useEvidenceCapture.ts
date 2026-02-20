// =============================================================================
// Argus AI — useEvidenceCapture Composable
// =============================================================================
//
// One-click evidence capture: grabs current video frame as JPEG, computes
// SHA-256 integrity hash, attaches AI metadata, and stores in the inspector
// store as an EvidencePacket.
//
// =============================================================================

import { sha256 } from '~/lib/storage/idb'
import { useInspectorStore, type EvidencePacket } from '~/stores/useInspectorStore'
import { useTelemetryStore } from '~/stores/useTelemetryStore'

export function useEvidenceCapture() {
  const inspectorStore = useInspectorStore()
  const telemetryStore = useTelemetryStore()

  /**
   * Capture a JPEG snapshot from a video element, compute SHA-256,
   * attach AI metadata, and store as evidence.
   */
  async function captureEvidence(
    videoElement: HTMLVideoElement | null,
    sessionId: string
  ): Promise<EvidencePacket | null> {
    if (!videoElement) return null
    if (videoElement.readyState < 2) return null
    if (videoElement.videoWidth === 0) return null

    try {
      // Step 1: Canvas capture at native resolution (capped at 640px wide)
      const scale = Math.min(1, 640 / videoElement.videoWidth)
      const w = Math.round(videoElement.videoWidth * scale)
      const h = Math.round(videoElement.videoHeight * scale)

      const canvas = document.createElement('canvas')
      canvas.width = w
      canvas.height = h
      const ctx = canvas.getContext('2d')
      if (!ctx) return null

      ctx.drawImage(videoElement, 0, 0, w, h)

      // Step 2: Export as JPEG data URL
      const frameDataUrl = canvas.toDataURL('image/jpeg', 0.85)

      // Step 3: Compute SHA-256 for tamper evidence
      const blob = await new Promise<Blob | null>((resolve) => {
        canvas.toBlob(resolve, 'image/jpeg', 0.85)
      })
      if (!blob) return null

      const arrayBuffer = await blob.arrayBuffer()
      const frameSha256 = await sha256(arrayBuffer)

      // Step 4: Gather AI metadata from telemetry store
      const telemetry = telemetryStore.getSessionTelemetry(sessionId)
      const riskScore = inspectorStore.getRiskScore(sessionId)

      // Derive gaze direction from current gaze coordinates
      let gazeDirection: string | null = null
      if (telemetry?.currentGaze) {
        const { x, y } = telemetry.currentGaze
        if (x < 0.3) gazeDirection = 'left'
        else if (x > 0.7) gazeDirection = 'right'
        else if (y < 0.3) gazeDirection = 'up'
        else if (y > 0.7) gazeDirection = 'down'
        else gazeDirection = 'center'
      }

      // Step 5: Build evidence packet
      const packet: EvidencePacket = {
        id: `ev-${sessionId.substring(0, 8)}-${Date.now()}`,
        sessionId,
        timestamp: Date.now(),
        frameDataUrl,
        frameSha256,
        aiMetadata: {
          headPose: null, // Populated when backend AI sends head pose events
          audioClass: null,
          gazeDirection,
          livenessScore: null,
          faceCount: 0,
          riskScore: riskScore.composite
        }
      }

      // Step 6: Store in inspector store
      inspectorStore.addEvidence(packet)

      // Release canvas memory
      canvas.width = 0
      canvas.height = 0

      return packet
    } catch (err) {
      console.error('[argus:evidence] Capture failed:', err)
      return null
    }
  }

  return {
    captureEvidence
  }
}
