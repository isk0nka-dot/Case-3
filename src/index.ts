// =============================================================================
// Argus AI SDK — Main Entry Point
// =============================================================================
//
// Plug-and-play exam proctoring SDK for third-party integrations.
//
// Usage:
//   const proctoring = new ArgusSDK({
//     sessionToken: 'eyJ...',
//     serverUrl: 'https://argusai.kz',
//   });
//   await proctoring.startPreflight();
//   await proctoring.startSession();
//   await proctoring.endSession();
//   proctoring.destroy();
// =============================================================================

import type {
  ArgusSDKConfig,
  SessionStatus,
  PreflightCheckResult,
  PreflightResult,
  PreflightStep,
  ViolationEvent,
  SDKError,
  HealthMetrics,
  ResilienceTier,
  TierConfig,
  VisionFrame,
  AudioFrame,
  EventPayload,
  DeliveryState,
  LiveKitPublishingState,
} from './types';

import { GrpcWebTransport } from './core/transport';
import { SessionManager } from './core/session';
import { EventEmitter } from './core/event-emitter';
import { HealthGovernor } from './health/governor';
import { VisionEngine } from './media/vision';
import { AudioEngine } from './media/audio';
import { LiveKitPublisher } from './media/livekit';
import { BrowserIntegrityMonitor } from './security/browser';
import { CameraWidget } from './ui/widget';
import { PreflightChecker } from './ui/preflight';
import { EventSource, EventType, Severity } from './types';

// Re-export public types and classes.
export {
  // Types
  type ArgusSDKConfig,
  type SessionStatus,
  type PreflightCheckResult,
  type PreflightResult,
  type PreflightStep,
  type ViolationEvent,
  type SDKError,
  type HealthMetrics,
  type ResilienceTier,
  type TierConfig,
  type VisionFrame,
  type AudioFrame,
  type EventPayload,
  type DeliveryState,
  type LiveKitPublishingState,
} from './types';

export { EventType, Severity, EventSource } from './types';
export { GrpcWebTransport } from './core/transport';
export { SessionManager } from './core/session';
export { EVENT_COLLECTOR_SERVICE_PATHS } from './core/service-paths';
export { HealthGovernor } from './health/governor';
export { VisionEngine } from './media/vision';
export { AudioEngine } from './media/audio';
export { LiveKitPublisher } from './media/livekit';
export { BrowserIntegrityMonitor } from './security/browser';
export { CameraWidget } from './ui/widget';
export { PreflightChecker } from './ui/preflight';

// ---------------------------------------------------------------------------
// SDK Events
// ---------------------------------------------------------------------------

interface SDKEvents {
  ready: void;
  statusChange: SessionStatus;
  violation: ViolationEvent;
  error: SDKError;
  preflightStep: PreflightStep;
  preflightCheck: PreflightCheckResult;
  preflightComplete: PreflightResult;
  deliveryUpdate: DeliveryState;
  liveKitStateChange: LiveKitPublishingState;
  healthUpdate: HealthMetrics;
  tierChange: { from: ResilienceTier; to: ResilienceTier; config: TierConfig };
}

// ---------------------------------------------------------------------------
// Main SDK Class
// ---------------------------------------------------------------------------

/**
 * ArgusSDK — plug-and-play exam proctoring.
 *
 * Orchestrates the full proctoring lifecycle:
 * 1. Preflight checks (camera, face detection, system)
 * 2. Session start (event streaming, heartbeat)
 * 3. AI monitoring (face detection, audio VAD, browser integrity)
 * 4. Health monitoring (FPS, RTT, adaptive tier switching)
 * 5. Session end (verdict, cleanup)
 *
 * @example
 * ```html
 * <script src="https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js"></script>
 * <script>
 *   const proctoring = new ArgusSDK({
 *     sessionToken: 'eyJ...',
 *     serverUrl: 'https://argusai.kz',
 *     onViolation: (e) => console.warn('Violation:', e),
 *   });
 *   await proctoring.startPreflight();
 *   await proctoring.startSession();
 * </script>
 * ```
 */
export class ArgusSDK extends EventEmitter<SDKEvents> {
  private readonly _config: ArgusSDKConfig;

  // Core modules.
  private _transport: GrpcWebTransport | null = null;
  private _session: SessionManager | null = null;
  private _governor: HealthGovernor | null = null;
  private _vision: VisionEngine | null = null;
  private _audio: AudioEngine | null = null;
  private _liveKit: LiveKitPublisher | null = null;
  private _security: BrowserIntegrityMonitor | null = null;
  private _widget: CameraWidget | null = null;
  private _preflight: PreflightChecker | null = null;
  private _aiSnapshotTimer: ReturnType<typeof setInterval> | null = null;
  private _aiSnapshotInFlight = false;
  private _sessionStartedAtMs = 0;

  // State.
  private _status: SessionStatus = 'idle';
  private _videoStream: MediaStream | null = null;
  private _lastPreflightResult: PreflightResult | null = null;

  constructor(config: ArgusSDKConfig) {
    super();
    this._config = config;

    // Wire external callbacks.
    if (config.onReady) this.on('ready', config.onReady);
    if (config.onError) this.on('error', config.onError);
    if (config.onViolation) this.on('violation', config.onViolation);
    if (config.onStatusChange) this.on('statusChange', config.onStatusChange);
    if (config.onPreflightStep) this.on('preflightStep', config.onPreflightStep);
    if (config.onPreflightCheck) this.on('preflightCheck', config.onPreflightCheck);
    if (config.onPreflightComplete) this.on('preflightComplete', config.onPreflightComplete);
    if (config.onDeliveryUpdate) this.on('deliveryUpdate', config.onDeliveryUpdate);
    if (config.onLiveKitStateChange) this.on('liveKitStateChange', config.onLiveKitStateChange);
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /** Current session status. */
  get status(): SessionStatus { return this._status; }

  /** Health metrics (null before session start). */
  get health(): HealthMetrics | null { return this._governor?.getMetrics() ?? null; }

  /** Current vision frame (null before session start). */
  get visionFrame(): VisionFrame | null { return this._vision?.currentFrame ?? null; }

  /** Current audio frame (null before session start). */
  get audioFrame(): AudioFrame | null { return this._audio?.currentFrame ?? null; }

  /** Violation count (0 before session start). */
  get violationCount(): number { return this._session?.violationCount ?? 0; }

  /** Buffered event queue depth. */
  get eventQueueDepth(): number { return this._session?.queueDepth ?? 0; }

  /** Number of events dropped due to bounded queue pressure. */
  get droppedEventCount(): number { return this._session?.droppedEventCount ?? 0; }

  /** Current LiveKit publishing state. */
  get liveKitState(): LiveKitPublishingState | null { return this._liveKit?.currentState ?? null; }

  /** Last structured preflight result (null before startPreflight()). */
  get lastPreflightResult(): PreflightResult | null { return this._lastPreflightResult; }

  /**
   * Run preflight checks.
   *
   * Must be called before startSession(). Checks:
   * 1. Camera + microphone access
   * 2. Face detection (single person, centered)
   * 3. System compatibility
   *
   * @returns true if all checks passed.
   */
  async startPreflight(): Promise<boolean> {
    this._setStatus('preflight');

    this._ensureTransportAndSession();
    const consentAccepted = this._config.preflight?.consentAccepted ?? false;
    if (consentAccepted) {
      void this._sendPreflightLifecycleEvent('PREFLIGHT_STARTED', 'Preflight started');
    }

    this._preflight = new PreflightChecker({
      locale: this._config.locale,
      consentAccepted,
      serverUrl: this._config.serverUrl,
      networkCheckUrl: this._config.preflight?.networkCheckUrl,
      networkTimeoutMs: this._config.preflight?.networkTimeoutMs,
      requireScreenCapture: this._config.preflight?.requireScreenCapture,
      mediapipeBasePath: this._config.preflight?.mediapipeBasePath ?? this._config.mediapipeBasePath,
      mediapipeModelAssetPath: this._config.preflight?.mediapipeModelAssetPath,
      allowFaceCheckFallback: this._config.preflight?.allowFaceCheckFallback,
      requireLivenessChallenge: this._config.preflight?.requireLivenessChallenge,
      requireDesktopAgent: this._config.preflight?.requireDesktopAgent,
      desktopAgentPort: this._config.preflight?.desktopAgentPort,
    });

    this._preflight.on('stepUpdate', (step) => {
      this.emit('preflightStep', step);
    });
    this._preflight.on('checkUpdate', (check) => {
      this.emit('preflightCheck', check);
    });
    this._preflight.on('complete', (result) => {
      this._lastPreflightResult = result;
      this.emit('preflightComplete', result);
    });

    const result = await this._preflight.runDetailed();
    const passed = result.allPassed;
    this._lastPreflightResult = result;
    if (consentAccepted) {
      await this._sendPreflightLifecycleEvent(
        passed ? 'PREFLIGHT_PASSED' : 'PREFLIGHT_FAILED',
        passed ? 'Preflight passed' : 'Preflight failed',
        result,
      );
    }

    if (passed) {
      this._videoStream = this._preflight.videoStream;
      this._setStatus('ready');
    } else {
      this._setStatus('error');
      this.emit('error', {
        code: 'PREFLIGHT_FAILED',
        message: this._formatPreflightFailure(result),
        recoverable: true,
      });
    }

    return passed;
  }

  /**
   * Start the proctoring session.
   *
   * Begins event streaming, AI monitoring, and health tracking.
   * Preflight must pass before calling this.
   */
  async startSession(): Promise<void> {
    if (this._status !== 'ready') {
      throw new Error('Cannot start session: preflight not completed. Call startPreflight() first.');
    }

    this._setStatus('initializing');

    try {
      this._ensureTransportAndSession();

      // Initialize camera widget.
      this._widget = new CameraWidget({
        containerId: this._config.containerId,
        locale: this._config.locale,
      });

      if (this._videoStream) {
        this._widget.setStream(this._videoStream);
      }

      if (this._config.liveKit?.enabled) {
        await this._startLiveKitPublishing();
      }

      // Initialize health governor.
      this._governor = new HealthGovernor({
        serverUrl: this._config.serverUrl,
        sampleIntervalMs: 5000,
      });

      this._governor.setTransportMetrics(() => {
        const h = this._transport!.getHealth();
        return { totalRequests: h.totalRequests, totalFailures: h.totalFailures };
      });

      this._governor.on('tierChange', (change) => {
        this.emit('tierChange', change);
        // Adjust vision engine inference rate based on tier.
        if (this._vision) {
          this._vision.setInferenceInterval(change.config.ai.intervalMs || 500);
        }
      });

      this._governor.on('healthUpdate', (metrics) => {
        this.emit('healthUpdate', metrics);
        if (this._session) {
          this._session.updateFocusScore(metrics.score);
        }
      });

      // Initialize vision engine.
      this._vision = new VisionEngine({
        mediapipeBasePath: this._config.mediapipeBasePath,
        inferenceIntervalMs: 100,
      });

      this._vision.on('frame', (frame) => {
        if (!frame.faceDetected && this._session) {
          this._session.sendEvent(
            3, // FACE_NOT_DETECTED
            2, // WARNING
            1, // WEBCAM
            'Face not detected',
            0.9,
          );
        }
      });

      // Initialize audio engine.
      this._audio = new AudioEngine({ analysisIntervalMs: 100 });

      this._audio.on('voiceActivity', (va) => {
        if (va.detected && this._session) {
          this._session.sendEvent(
            20, // VOICE_ACTIVITY
            1,  // INFO
            1,  // WEBCAM
            'Voice activity detected',
            0.8,
            { type: 'audio', data: { speakerCount: va.speakerCount } },
          );
        }
      });

      // Initialize browser security.
      this._security = new BrowserIntegrityMonitor();
      this._security.on('violation', (v) => {
        if (this._session) {
          this._session.sendEvent(v.eventType, v.severity, v.source, v.label, v.confidence);
        }
      });

      // Start everything.
      this._session!.start();
      this._sessionStartedAtMs = Date.now();
      this._governor.start();
      this._security.start();

      // Start vision (needs video element from widget).
      await this._vision.start(this._widget.videoElement);

      // Start audio.
      if (this._preflight?.audioStream) {
        await this._audio.start(this._preflight.audioStream);
      }

      this._startAISnapshotSampling();

      this._widget.setStatus('active');
      this._setStatus('active');
      this.emit('ready', undefined as unknown as void);

      // Notify browser extension (if installed) to start monitoring this tab.
      this._notifyExtensionStart();
    } catch (err) {
      this._setStatus('error');
      this.emit('error', {
        code: 'SESSION_START_FAILED',
        message: err instanceof Error ? err.message : 'Failed to start session',
        recoverable: false,
      });
      throw err;
    }
  }

  /**
   * End the proctoring session.
   *
   * Flushes remaining events, stops monitoring, and cleans up resources.
   */
  async endSession(): Promise<void> {
    if (this._status !== 'active' && this._status !== 'paused') return;

    // Stop monitoring.
    this._stopAISnapshotSampling();
    this._vision?.stop();
    this._audio?.stop();
    this._liveKit?.stop();
    this._security?.stop();
    this._governor?.stop();

    // Stop session (flushes events).
    if (this._session) {
      await this._session.stop();
    }

    this._widget?.setStatus('completed');
    this._setStatus('completed');
    this._notifyExtensionStop();
  }

  /**
   * Destroy the SDK instance — releases all resources.
   *
   * Must be called when the partner page navigates away or unmounts.
   */
  destroy(): void {
    // Stop all modules.
    this._vision?.destroy();
    this._stopAISnapshotSampling();
    this._audio?.destroy();
    this._liveKit?.destroy();
    this._security?.destroy();
    this._governor?.destroy();
    this._session?.destroy();
    this._widget?.destroy();
    this._transport?.destroy();
    this._preflight?.destroy();

    // Stop video stream.
    if (this._videoStream) {
      this._videoStream.getTracks().forEach(t => t.stop());
      this._videoStream = null;
    }

    this.removeAllListeners();
    this._setStatus('idle');
  }

  // ---------------------------------------------------------------------------
  // Private
  // ---------------------------------------------------------------------------

  private _setStatus(status: SessionStatus): void {
    if (this._status === status) return;
    this._status = status;
    this.emit('statusChange', status);
    this._widget?.setStatus(status);
  }

  private async _startLiveKitPublishing(): Promise<void> {
    if (!this._videoStream || !this._session) return;

    try {
      this._liveKit = new LiveKitPublisher({
        serverUrl: this._config.serverUrl,
        sessionId: this._session.sessionId,
        sessionToken: this._config.sessionToken,
        config: this._config.liveKit,
      });
      const state = await this._liveKit.start(this._videoStream);
      this.emit('liveKitStateChange', state);
    } catch (err) {
      const error: SDKError = {
        code: 'LIVEKIT_PUBLISH_FAILED',
        message: err instanceof Error ? err.message : 'Failed to publish LiveKit media',
        recoverable: !this._config.liveKit?.required,
      };
      this.emit('error', error);
      if (this._config.liveKit?.required) {
        throw err;
      }
    }
  }

  private _startAISnapshotSampling(): void {
    const cfg = this._config.aiSnapshots;
    if (cfg?.enabled === false || !this._session || !this._widget?.videoElement) return;

    const intervalMs = Math.max(5000, cfg?.intervalMs ?? 15000);
    this._captureAndSendAIFrame().catch(() => {/* best-effort */});
    this._aiSnapshotTimer = setInterval(() => {
      this._captureAndSendAIFrame().catch(() => {/* best-effort */});
    }, intervalMs);
  }

  private _stopAISnapshotSampling(): void {
    if (this._aiSnapshotTimer) {
      clearInterval(this._aiSnapshotTimer);
      this._aiSnapshotTimer = null;
    }
    this._aiSnapshotInFlight = false;
  }

  private async _captureAndSendAIFrame(): Promise<void> {
    if (this._aiSnapshotInFlight || !this._session || !this._widget?.videoElement) return;
    const video = this._widget.videoElement;
    if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA || video.videoWidth <= 0 || video.videoHeight <= 0) {
      return;
    }

    this._aiSnapshotInFlight = true;
    try {
      const cfg = this._config.aiSnapshots;
      const maxWidth = Math.max(160, cfg?.maxWidth ?? 640);
      const scale = Math.min(1, maxWidth / video.videoWidth);
      const width = Math.max(1, Math.round(video.videoWidth * scale));
      const height = Math.max(1, Math.round(video.videoHeight * scale));

      const canvas = document.createElement('canvas');
      canvas.width = width;
      canvas.height = height;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.drawImage(video, 0, 0, width, height);

      const quality = Math.max(0.4, Math.min(0.92, cfg?.quality ?? 0.72));
      const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', quality));
      if (!blob) return;

      const form = new FormData();
      form.append('frame', blob, `argus-${this._session.sessionId}-${Date.now()}.jpg`);
      form.append('contentType', 'image/jpeg');
      const ts = this._sessionStartedAtMs > 0 ? (Date.now() - this._sessionStartedAtMs) / 1000 : 0;
      form.append('videoTimestampSec', ts.toFixed(3));

      const base = this._config.serverUrl.replace(/\/+$/, '');
      const response = await fetch(`${base}/api/v1/external/sessions/${encodeURIComponent(this._session.sessionId)}/ai-frame`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${this._config.sessionToken}`,
        },
        body: form,
        keepalive: false,
      });

      if (!response.ok && response.status >= 500) {
        this.emit('error', {
          code: 'AI_FRAME_UPLOAD_FAILED',
          message: `Backend AI frame upload failed: HTTP ${response.status}`,
          recoverable: true,
        });
      }
    } catch (err) {
      this.emit('error', {
        code: 'AI_FRAME_UPLOAD_FAILED',
        message: err instanceof Error ? err.message : 'Backend AI frame upload failed',
        recoverable: true,
      });
    } finally {
      this._aiSnapshotInFlight = false;
    }
  }

  private _ensureTransportAndSession(): void {
    if (!this._transport) {
      this._transport = new GrpcWebTransport({
        baseUrl: this._config.serverUrl,
        timeout: 10_000,
        maxRetries: 3,
      });
    }

    if (!this._session) {
      this._session = new SessionManager(this._transport, this._config.sessionToken);
      this._session.on('violation', (v) => this.emit('violation', v));
      this._session.on('error', (e) => this.emit('error', e));
      this._session.on('statusChange', (s) => this._setStatus(s));
      this._session.on('delivery', (state) => this.emit('deliveryUpdate', state));
    }
  }

  private async _sendPreflightLifecycleEvent(
    code: 'PREFLIGHT_STARTED' | 'PREFLIGHT_PASSED' | 'PREFLIGHT_FAILED',
    label: string,
    result?: PreflightResult,
  ): Promise<void> {
    if (!this._session) return;

    const payload: EventPayload = {
      type: 'system',
      data: {
        category: code.toLowerCase(),
        terminated: false,
      },
    };

    this._session.sendLifecycleEvent(
      EventType.FOCUS_SCORE_UPDATE,
      code === 'PREFLIGHT_FAILED' ? Severity.WARNING : Severity.INFO,
      EventSource.SYSTEM,
      label,
      result?.allPassed === false ? 0.99 : 1,
      payload,
    );

    if (result) {
      this._session.sendLifecycleEvent(
        EventType.FOCUS_SCORE_UPDATE,
        Severity.INFO,
        EventSource.SYSTEM,
        `Preflight summary: ${result.summary.passed} passed, ${result.summary.requiredFailed} blocking failures`,
        1,
        {
          type: 'system',
          data: {
            category: 'preflight_summary',
            terminated: false,
          },
        },
      );
    }

    await this._session.flushNow();
  }

  /** Notify Argus browser extension (if installed) that a session has started. */
  private _notifyExtensionStart(): void {
    try {
      if (typeof window !== 'undefined') {
        window.postMessage({
          argus: {
            type: 'SESSION_START',
            payload: {
              sessionToken: this._config.sessionToken,
              serverUrl: this._config.serverUrl,
            },
          },
        }, '*');
      }
    } catch { /* Extension may not be installed — non-fatal */ }
  }

  /** Notify Argus browser extension that the session has ended. */
  private _notifyExtensionStop(): void {
    try {
      if (typeof window !== 'undefined') {
        window.postMessage({ argus: { type: 'SESSION_END' } }, '*');
      }
    } catch { /* non-fatal */ }
  }

  private _formatPreflightFailure(result: PreflightResult): string {
    const failures = result.checks
      .filter(check => check.required && check.status !== 'passed')
      .map(check => check.message);
    if (failures.length === 0) return 'One or more preflight checks failed';
    return failures.join('; ');
  }
}

// UMD global export.
if (typeof window !== 'undefined') {
  (window as unknown as Record<string, unknown>).ArgusSDK = ArgusSDK;
}

// Default export for ESM.
export default ArgusSDK;
