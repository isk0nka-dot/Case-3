// =============================================================================
// Argus SDK — Vision Engine (MediaPipe Face Detection)
// =============================================================================
//
// Face detection, head pose estimation, gaze tracking, and blink detection.
// Extracted from useVisionEngine — Vue refs replaced with class state.
//
// MediaPipe FaceLandmarker runs inference in the main thread with
// requestAnimationFrame pacing. GPU delegate is preferred when available.
// =============================================================================

import type { VisionFrame } from '../types';
import { EventEmitter } from '../core/event-emitter';

interface VisionEvents {
  frame: VisionFrame;
  faceDetected: { detected: boolean; count: number };
  error: { code: string; message: string };
}

export interface VisionEngineOptions {
  /** MediaPipe WASM/model base path. Default: jsdelivr CDN. */
  mediapipeBasePath?: string;
  /** Inference interval in ms. Default: 100 (10 FPS). */
  inferenceIntervalMs?: number;
}

/**
 * Vision engine — MediaPipe FaceLandmarker face detection.
 *
 * Processes video frames from a HTMLVideoElement and emits VisionFrame
 * data with face detection, head pose, gaze direction, and blink info.
 */
export class VisionEngine extends EventEmitter<VisionEvents> {
  private _faceLandmarker: unknown = null; // FaceLandmarker instance (dynamic import)
  private _videoElement: HTMLVideoElement | null = null;
  private _rafId: number | null = null;
  private _running = false;
  private _lastInferenceTime = 0;
  private _inferenceIntervalMs: number;
  private _mediapipeBasePath: string;

  // Stats.
  private _frameCount = 0;
  private _currentFrame: VisionFrame | null = null;

  // Rolling stats for anti-spoofing.
  private _blinkCount = 0;
  private _blinkTimestamps: number[] = [];

  constructor(options?: VisionEngineOptions) {
    super();
    this._mediapipeBasePath = options?.mediapipeBasePath
      ?? 'https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@latest/wasm';
    this._inferenceIntervalMs = options?.inferenceIntervalMs ?? 100;
  }

  // ---------------------------------------------------------------------------
  // Public
  // ---------------------------------------------------------------------------

  get isRunning(): boolean { return this._running; }
  get currentFrame(): VisionFrame | null { return this._currentFrame; }
  get frameCount(): number { return this._frameCount; }
  get blinkRate(): number {
    const now = Date.now();
    const recent = this._blinkTimestamps.filter(t => now - t < 60_000);
    return recent.length; // Blinks per minute.
  }

  /** Set the inference interval (for tier-based throttling). */
  setInferenceInterval(ms: number): void {
    this._inferenceIntervalMs = ms;
  }

  /** Initialize MediaPipe and start processing frames. */
  async start(videoElement: HTMLVideoElement): Promise<void> {
    this._videoElement = videoElement;

    try {
      await this._initMediaPipe();
      this._running = true;
      this._processLoop();
    } catch (err) {
      this.emit('error', {
        code: 'VISION_INIT_FAILED',
        message: err instanceof Error ? err.message : 'Failed to initialize vision engine',
      });
      throw err;
    }
  }

  /** Stop processing. */
  stop(): void {
    this._running = false;
    if (this._rafId !== null) {
      cancelAnimationFrame(this._rafId);
      this._rafId = null;
    }
  }

  /** Clean up all resources. */
  destroy(): void {
    this.stop();
    if (this._faceLandmarker && typeof (this._faceLandmarker as { close(): void }).close === 'function') {
      (this._faceLandmarker as { close(): void }).close();
    }
    this._faceLandmarker = null;
    this._videoElement = null;
    this.removeAllListeners();
  }

  // ---------------------------------------------------------------------------
  // Private — MediaPipe initialization
  // ---------------------------------------------------------------------------

  private async _initMediaPipe(): Promise<void> {
    // Dynamic import to keep MediaPipe as optional peer dependency.
    const vision = await import('@mediapipe/tasks-vision');
    const { FaceLandmarker, FilesetResolver } = vision;

    const filesetResolver = await FilesetResolver.forVisionTasks(this._mediapipeBasePath);

    this._faceLandmarker = await FaceLandmarker.createFromOptions(filesetResolver, {
      baseOptions: {
        modelAssetPath: `https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/1/face_landmarker.task`,
        delegate: 'GPU',
      },
      runningMode: 'VIDEO',
      numFaces: 2,
      minFaceDetectionConfidence: 0.5,
      minFacePresenceConfidence: 0.5,
      minTrackingConfidence: 0.5,
      outputFaceBlendshapes: true,
      outputFacialTransformationMatrixes: false,
    });
  }

  // ---------------------------------------------------------------------------
  // Private — Frame processing loop
  // ---------------------------------------------------------------------------

  private _processLoop(): void {
    if (!this._running) return;

    const now = performance.now();
    if (now - this._lastInferenceTime >= this._inferenceIntervalMs) {
      this._processFrame(now);
      this._lastInferenceTime = now;
    }

    this._rafId = requestAnimationFrame(() => this._processLoop());
  }

  private _processFrame(timestamp: number): void {
    if (!this._faceLandmarker || !this._videoElement) return;
    if (this._videoElement.readyState < 2) return; // Not enough data.

    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const results = (this._faceLandmarker as any).detectForVideo(this._videoElement, timestamp);
      const frame = this._buildFrame(results, timestamp);

      this._currentFrame = frame;
      this._frameCount++;

      this.emit('frame', frame);

      // Track face detection changes.
      this.emit('faceDetected', {
        detected: frame.faceDetected,
        count: frame.faceCount,
      });
    } catch {
      // Inference errors are non-fatal — skip frame.
    }
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private _buildFrame(results: any, timestamp: number): VisionFrame {
    const landmarks = results?.faceLandmarks;
    const blendshapes = results?.faceBlendshapes;
    const faceCount = landmarks?.length ?? 0;
    const faceDetected = faceCount > 0;

    if (!faceDetected || !landmarks[0]) {
      return {
        faceDetected: false,
        faceCount: 0,
        blinkDetected: false,
        quality: 0,
        timestamp,
      };
    }

    const lm = landmarks[0]; // Primary face landmarks.
    const bs = blendshapes?.[0]?.categories;

    // Head pose estimation from landmark ratios.
    const headPose = this._estimateHeadPose(lm);

    // Gaze estimation.
    const gaze = this._estimateGaze(lm);

    // Eye aspect ratio for blink detection.
    const ear = this._computeEAR(lm);
    const blinkDetected = ear.left < 0.2 && ear.right < 0.2;

    if (blinkDetected) {
      this._blinkCount++;
      this._blinkTimestamps.push(Date.now());
      // Keep only last 2 minutes of blink timestamps.
      const cutoff = Date.now() - 120_000;
      this._blinkTimestamps = this._blinkTimestamps.filter(t => t > cutoff);
    }

    // Face bounding box.
    const bbox = this._computeFaceBbox(lm);

    // Quality score (0-1) based on face size and centering.
    const quality = this._assessQuality(bbox, bs);

    return {
      faceDetected: true,
      faceCount,
      faceBbox: bbox,
      headPose,
      gazeDirection: gaze,
      eyeAspectRatio: ear,
      blinkDetected,
      quality,
      timestamp,
    };
  }

  // ---------------------------------------------------------------------------
  // Pure geometry functions
  // ---------------------------------------------------------------------------

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private _estimateHeadPose(lm: any[]): { yaw: number; pitch: number; roll: number } {
    // Use nose tip (1), chin (152), left eye (33), right eye (263).
    const nose = lm[1];
    const chin = lm[152];
    const leftEye = lm[33];
    const rightEye = lm[263];

    if (!nose || !chin || !leftEye || !rightEye) {
      return { yaw: 0, pitch: 0, roll: 0 };
    }

    const dx = rightEye.x - leftEye.x;
    const dy = rightEye.y - leftEye.y;
    const roll = Math.atan2(dy, dx) * (180 / Math.PI);

    const midX = (leftEye.x + rightEye.x) / 2;
    const yaw = (nose.x - midX) * 180;

    const midY = (leftEye.y + rightEye.y) / 2;
    const pitch = (nose.y - midY) * 180;

    return { yaw, pitch, roll };
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private _estimateGaze(lm: any[]): { x: number; y: number } {
    // Approximate gaze from iris position relative to eye corners.
    const leftIris = lm[468]; // Left iris center.
    const rightIris = lm[473]; // Right iris center.
    const leftInner = lm[133];
    const leftOuter = lm[33];
    const rightInner = lm[362];
    const rightOuter = lm[263];

    if (!leftIris || !rightIris || !leftInner || !leftOuter || !rightInner || !rightOuter) {
      return { x: 0, y: 0 };
    }

    const leftRatio = (leftIris.x - leftOuter.x) / (leftInner.x - leftOuter.x);
    const rightRatio = (rightIris.x - rightInner.x) / (rightOuter.x - rightInner.x);
    const x = ((leftRatio + rightRatio) / 2 - 0.5) * 2;

    const leftYRatio = (leftIris.y - leftOuter.y) / (leftInner.y - leftOuter.y);
    const y = (leftYRatio - 0.5) * 2;

    return { x: Math.max(-1, Math.min(1, x)), y: Math.max(-1, Math.min(1, y)) };
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private _computeEAR(lm: any[]): { left: number; right: number } {
    const leftEAR = this._earForEye(lm, [33, 160, 158, 133, 153, 144]);
    const rightEAR = this._earForEye(lm, [362, 385, 387, 263, 380, 373]);
    return { left: leftEAR, right: rightEAR };
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private _earForEye(lm: any[], indices: number[]): number {
    const [p1, p2, p3, p4, p5, p6] = indices.map(i => lm[i]);
    if (!p1 || !p2 || !p3 || !p4 || !p5 || !p6) return 0.3;

    const v1 = Math.sqrt((p2.x - p6.x) ** 2 + (p2.y - p6.y) ** 2);
    const v2 = Math.sqrt((p3.x - p5.x) ** 2 + (p3.y - p5.y) ** 2);
    const h = Math.sqrt((p1.x - p4.x) ** 2 + (p1.y - p4.y) ** 2);

    return h > 0 ? (v1 + v2) / (2 * h) : 0.3;
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private _computeFaceBbox(lm: any[]): { x: number; y: number; w: number; h: number } {
    let minX = 1, maxX = 0, minY = 1, maxY = 0;
    for (const pt of lm) {
      if (pt.x < minX) minX = pt.x;
      if (pt.x > maxX) maxX = pt.x;
      if (pt.y < minY) minY = pt.y;
      if (pt.y > maxY) maxY = pt.y;
    }
    return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private _assessQuality(bbox: { x: number; y: number; w: number; h: number }, _bs: any): number {
    // Quality based on face size (should be 15-60% of frame).
    const faceArea = bbox.w * bbox.h;
    let sizeScore = 1;
    if (faceArea < 0.02) sizeScore = 0.3;
    else if (faceArea < 0.05) sizeScore = 0.6;
    else if (faceArea > 0.6) sizeScore = 0.7;

    // Quality based on centering (face center should be near frame center).
    const cx = bbox.x + bbox.w / 2;
    const cy = bbox.y + bbox.h / 2;
    const offCenter = Math.sqrt((cx - 0.5) ** 2 + (cy - 0.5) ** 2);
    const centerScore = Math.max(0, 1 - offCenter * 2);

    return Math.round((sizeScore * 0.6 + centerScore * 0.4) * 100) / 100;
  }
}
