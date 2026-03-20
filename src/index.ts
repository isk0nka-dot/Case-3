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
  PreflightStep,
  ViolationEvent,
  SDKError,
  HealthMetrics,
  ResilienceTier,
  TierConfig,
  VisionFrame,
  AudioFrame,
} from './types';

import { GrpcWebTransport } from './core/transport';
import { SessionManager } from './core/session';
import { EventEmitter } from './core/event-emitter';
import { HealthGovernor } from './health/governor';
import { VisionEngine } from './media/vision';
import { AudioEngine } from './media/audio';
import { BrowserIntegrityMonitor } from './security/browser';
import { CameraWidget } from './ui/widget';
import { PreflightChecker } from './ui/preflight';

// Re-export public types and classes.
export {
  // Types
  type ArgusSDKConfig,
  type SessionStatus,
  type PreflightStep,
  type ViolationEvent,
  type SDKError,
  type HealthMetrics,
  type ResilienceTier,
  type TierConfig,
  type VisionFrame,
  type AudioFrame,
} from './types';

export { EventType, Severity, EventSource } from './types';
export { GrpcWebTransport } from './core/transport';
export { SessionManager } from './core/session';
export { HealthGovernor } from './health/governor';
export { VisionEngine } from './media/vision';
export { AudioEngine } from './media/audio';
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
 * <script src="https://argusai.kz/sdk/argus-sdk.umd.js"></script>
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
  private _security: BrowserIntegrityMonitor | null = null;
  private _widget: CameraWidget | null = null;
  private _preflight: PreflightChecker | null = null;

  // State.
  private _status: SessionStatus = 'idle';
  private _videoStream: MediaStream | null = null;

  constructor(config: ArgusSDKConfig) {
    super();
    this._config = config;

    // Wire external callbacks.
    if (config.onReady) this.on('ready', config.onReady);
    if (config.onError) this.on('error', config.onError);
    if (config.onViolation) this.on('violation', config.onViolation);
    if (config.onStatusChange) this.on('statusChange', config.onStatusChange);
    if (config.onPreflightStep) this.on('preflightStep', config.onPreflightStep);
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

    this._preflight = new PreflightChecker({
      locale: this._config.locale,
    });

    this._preflight.on('stepUpdate', (step) => {
      this.emit('preflightStep', step);
    });

    const passed = await this._preflight.run();

    if (passed) {
      this._videoStream = this._preflight.videoStream;
      this._setStatus('ready');
    } else {
      this._setStatus('error');
      this.emit('error', {
        code: 'PREFLIGHT_FAILED',
        message: 'One or more preflight checks failed',
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
      // Initialize transport.
      this._transport = new GrpcWebTransport({
        baseUrl: this._config.serverUrl,
        timeout: 10_000,
        maxRetries: 3,
      });

      // Initialize session manager.
      this._session = new SessionManager(this._transport, this._config.sessionToken);

      this._session.on('violation', (v) => this.emit('violation', v));
      this._session.on('error', (e) => this.emit('error', e));
      this._session.on('statusChange', (s) => this._setStatus(s));

      // Initialize camera widget.
      this._widget = new CameraWidget({
        containerId: this._config.containerId,
        locale: this._config.locale,
      });

      if (this._videoStream) {
        this._widget.setStream(this._videoStream);
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
      this._session.start();
      this._governor.start();
      this._security.start();

      // Start vision (needs video element from widget).
      await this._vision.start(this._widget.videoElement);

      // Start audio.
      if (this._preflight?.audioStream) {
        await this._audio.start(this._preflight.audioStream);
      }

      this._widget.setStatus('active');
      this._setStatus('active');
      this.emit('ready', undefined as unknown as void);
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
    this._vision?.stop();
    this._audio?.stop();
    this._security?.stop();
    this._governor?.stop();

    // Stop session (flushes events).
    if (this._session) {
      await this._session.stop();
    }

    this._widget?.setStatus('completed');
    this._setStatus('completed');
  }

  /**
   * Destroy the SDK instance — releases all resources.
   *
   * Must be called when the partner page navigates away or unmounts.
   */
  destroy(): void {
    // Stop all modules.
    this._vision?.destroy();
    this._audio?.destroy();
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
}

// UMD global export.
if (typeof window !== 'undefined') {
  (window as unknown as Record<string, unknown>).ArgusSDK = ArgusSDK;
}

// Default export for ESM.
export default ArgusSDK;
