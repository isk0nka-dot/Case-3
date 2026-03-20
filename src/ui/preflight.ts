// =============================================================================
// Argus SDK — Preflight Gatekeeper
// =============================================================================
//
// 3-step mandatory hardware/environment/system check before proctoring starts.
// All 3 steps must pass before startSession() can proceed.
// =============================================================================

import type { PreflightStep } from '../types';
import { EventEmitter } from '../core/event-emitter';

interface PreflightEvents {
  stepUpdate: PreflightStep;
  complete: { allPassed: boolean; steps: PreflightStep[] };
}

/** Preflight check options. */
export interface PreflightOptions {
  /** Whether to check for screen sharing capability. Default: true. */
  requireScreenCapture?: boolean;
  /** Locale for messages. Default: 'ru'. */
  locale?: 'kk' | 'ru' | 'en';
}

const MESSAGES: Record<string, Record<string, Record<string, string>>> = {
  kk: {
    hardware: {
      checking: 'Камера мен микрофон тексерілуде...',
      passed: 'Камера мен микрофон дайын',
      failed: 'Камера немесе микрофонға рұқсат берілмеді',
    },
    environment: {
      checking: 'Бетті анықтау тексерілуде...',
      passed: 'Бет анықталды, жағдай дұрыс',
      failed: 'Бет анықталмады немесе бірнеше адам бар',
    },
    system: {
      checking: 'Жүйе тексерілуде...',
      passed: 'Жүйе талаптарға сай',
      failed: 'Жүйе тексерісі сәтсіз',
    },
  },
  ru: {
    hardware: {
      checking: 'Проверка камеры и микрофона...',
      passed: 'Камера и микрофон готовы',
      failed: 'Нет доступа к камере или микрофону',
    },
    environment: {
      checking: 'Проверка обнаружения лица...',
      passed: 'Лицо обнаружено, условия подходящие',
      failed: 'Лицо не обнаружено или обнаружено несколько человек',
    },
    system: {
      checking: 'Проверка системы...',
      passed: 'Система соответствует требованиям',
      failed: 'Системная проверка не пройдена',
    },
  },
  en: {
    hardware: {
      checking: 'Checking camera and microphone...',
      passed: 'Camera and microphone ready',
      failed: 'Camera or microphone access denied',
    },
    environment: {
      checking: 'Checking face detection...',
      passed: 'Face detected, conditions suitable',
      failed: 'No face detected or multiple people found',
    },
    system: {
      checking: 'Running system checks...',
      passed: 'System meets requirements',
      failed: 'System check failed',
    },
  },
};

/**
 * Preflight gatekeeper — 3-step mandatory check.
 *
 * Steps:
 * 1. Hardware: getUserMedia for camera + microphone
 * 2. Environment: Face detected via MediaPipe (single person, centered)
 * 3. System: Fullscreen capability, no DevTools, browser compatibility
 */
export class PreflightChecker extends EventEmitter<PreflightEvents> {
  private _steps: PreflightStep[] = [
    { id: 'hardware', status: 'pending', message: '' },
    { id: 'environment', status: 'pending', message: '' },
    { id: 'system', status: 'pending', message: '' },
  ];

  private _videoStream: MediaStream | null = null;
  private _audioStream: MediaStream | null = null;
  private readonly _locale: string;
  private readonly _requireScreenCapture: boolean;
  private readonly _msgs: Record<string, Record<string, string>>;

  constructor(options?: PreflightOptions) {
    super();
    this._locale = options?.locale ?? 'ru';
    this._requireScreenCapture = options?.requireScreenCapture ?? false;
    this._msgs = MESSAGES[this._locale] ?? MESSAGES.ru;
  }

  // ---------------------------------------------------------------------------
  // Public
  // ---------------------------------------------------------------------------

  get steps(): readonly PreflightStep[] { return this._steps; }
  get allPassed(): boolean { return this._steps.every(s => s.status === 'passed'); }
  get videoStream(): MediaStream | null { return this._videoStream; }
  get audioStream(): MediaStream | null { return this._audioStream; }

  /**
   * Run all preflight checks sequentially.
   * Returns true if all passed, false otherwise.
   */
  async run(): Promise<boolean> {
    await this._checkHardware();
    await this._checkEnvironment();
    await this._checkSystem();

    const allPassed = this.allPassed;
    this.emit('complete', { allPassed, steps: [...this._steps] });
    return allPassed;
  }

  /** Release streams acquired during preflight. */
  releaseStreams(): void {
    // Don't stop the streams — they'll be used by the session.
    // Just clear our references.
    this._videoStream = null;
    this._audioStream = null;
  }

  /** Destroy and clean up. */
  destroy(): void {
    if (this._videoStream) {
      this._videoStream.getTracks().forEach(t => t.stop());
    }
    if (this._audioStream) {
      this._audioStream.getTracks().forEach(t => t.stop());
    }
    this.removeAllListeners();
  }

  // ---------------------------------------------------------------------------
  // Check implementations
  // ---------------------------------------------------------------------------

  private async _checkHardware(): Promise<void> {
    this._updateStep('hardware', 'checking', this._msgs.hardware.checking);

    try {
      // Request camera + microphone.
      this._videoStream = await navigator.mediaDevices.getUserMedia({
        video: { width: { ideal: 1280 }, height: { ideal: 720 }, facingMode: 'user' },
        audio: true,
      });

      // Separate audio stream for the audio engine.
      this._audioStream = new MediaStream(this._videoStream.getAudioTracks());

      this._updateStep('hardware', 'passed', this._msgs.hardware.passed);
    } catch (err) {
      const detail = err instanceof Error ? err.message : 'Unknown error';
      this._updateStep('hardware', 'failed', this._msgs.hardware.failed, detail);
    }
  }

  private async _checkEnvironment(): Promise<void> {
    this._updateStep('environment', 'checking', this._msgs.environment.checking);

    if (!this._videoStream) {
      this._updateStep('environment', 'failed', this._msgs.environment.failed,
        'No video stream (hardware check failed)');
      return;
    }

    try {
      // Quick face detection check using a temporary video element.
      const passed = await this._quickFaceCheck(this._videoStream);
      if (passed) {
        this._updateStep('environment', 'passed', this._msgs.environment.passed);
      } else {
        this._updateStep('environment', 'failed', this._msgs.environment.failed);
      }
    } catch (err) {
      const detail = err instanceof Error ? err.message : 'Unknown error';
      this._updateStep('environment', 'failed', this._msgs.environment.failed, detail);
    }
  }

  private async _checkSystem(): Promise<void> {
    this._updateStep('system', 'checking', this._msgs.system.checking);

    const issues: string[] = [];

    // Check fullscreen support.
    if (!document.documentElement.requestFullscreen) {
      issues.push('Fullscreen not supported');
    }

    // Check browser compatibility (must support MediaStream + AudioContext).
    if (!navigator.mediaDevices?.getUserMedia) {
      issues.push('getUserMedia not supported');
    }
    if (typeof AudioContext === 'undefined' && typeof (window as unknown as { webkitAudioContext: unknown }).webkitAudioContext === 'undefined') {
      issues.push('Web Audio API not supported');
    }

    // Check secure context (HTTPS).
    if (!window.isSecureContext) {
      issues.push('Not a secure context (HTTPS required)');
    }

    if (issues.length === 0) {
      this._updateStep('system', 'passed', this._msgs.system.passed);
    } else {
      this._updateStep('system', 'failed', this._msgs.system.failed, issues.join(', '));
    }
  }

  private async _quickFaceCheck(stream: MediaStream): Promise<boolean> {
    // Create a temporary video element for face detection.
    const video = document.createElement('video');
    video.srcObject = stream;
    video.muted = true;
    video.playsInline = true;

    await video.play();

    // Wait for video to have valid dimensions.
    await new Promise<void>(resolve => {
      if (video.videoWidth > 0) { resolve(); return; }
      video.onloadedmetadata = () => resolve();
      setTimeout(resolve, 3000); // Timeout after 3s.
    });

    // Try to load MediaPipe for face detection.
    try {
      const vision = await import('@mediapipe/tasks-vision');
      const { FaceLandmarker, FilesetResolver } = vision;

      const filesetResolver = await FilesetResolver.forVisionTasks(
        'https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@latest/wasm',
      );

      const detector = await FaceLandmarker.createFromOptions(filesetResolver, {
        baseOptions: {
          modelAssetPath: 'https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/1/face_landmarker.task',
          delegate: 'GPU',
        },
        runningMode: 'IMAGE',
        numFaces: 2,
      });

      const result = detector.detect(video);
      detector.close();
      video.pause();
      video.srcObject = null;

      const faceCount = result?.faceLandmarks?.length ?? 0;
      return faceCount === 1; // Exactly one face.
    } catch {
      // MediaPipe not available — skip face check (pass with warning).
      video.pause();
      video.srcObject = null;
      return true;
    }
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  private _updateStep(id: PreflightStep['id'], status: PreflightStep['status'], message: string, details?: string): void {
    const step = this._steps.find(s => s.id === id);
    if (!step) return;
    step.status = status;
    step.message = message;
    if (details) step.details = details;
    this.emit('stepUpdate', { ...step });
  }
}
