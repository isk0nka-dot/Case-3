// =============================================================================
// Argus SDK — Audio Engine (Web Audio API VAD)
// =============================================================================
//
// Voice activity detection, speaker counting, and audio classification.
// Extracted from useAudioEngine — Vue refs replaced with class state.
// All DSP functions are pure math with no framework dependencies.
// =============================================================================

import type { AudioFrame } from '../types';
import { EventEmitter } from '../core/event-emitter';

interface AudioEvents {
  frame: AudioFrame;
  voiceActivity: { detected: boolean; speakerCount: number };
  error: { code: string; message: string };
}

export interface AudioEngineOptions {
  /** FFT size for the analyser node. Default: 2048. */
  fftSize?: number;
  /** Analysis interval in ms. Default: 100. */
  analysisIntervalMs?: number;
  /** Noise floor calibration duration in ms. Default: 2000. */
  calibrationDurationMs?: number;
}

/**
 * Audio engine — Web Audio API voice activity detection.
 *
 * Processes audio from the user's microphone and emits AudioFrame data
 * with RMS/peak levels, voice activity, speaker count, and classification.
 */
export class AudioEngine extends EventEmitter<AudioEvents> {
  private _audioCtx: AudioContext | null = null;
  private _analyser: AnalyserNode | null = null;
  private _source: MediaStreamAudioSourceNode | null = null;
  private _stream: MediaStream | null = null;
  private _analysisTimer: ReturnType<typeof setInterval> | null = null;
  private _running = false;

  // A-weighting lookup table.
  private _aWeightLUT: Float32Array | null = null;

  // Noise floor (calibrated).
  private _noiseFloorDb = -60;
  private _calibrated = false;
  private _calibrationSamples: number[] = [];

  // Config.
  private readonly _fftSize: number;
  private readonly _analysisIntervalMs: number;
  private readonly _calibrationDurationMs: number;

  // Stats.
  private _currentFrame: AudioFrame | null = null;
  private _frameCount = 0;

  constructor(options?: AudioEngineOptions) {
    super();
    this._fftSize = options?.fftSize ?? 2048;
    this._analysisIntervalMs = options?.analysisIntervalMs ?? 100;
    this._calibrationDurationMs = options?.calibrationDurationMs ?? 2000;
  }

  // ---------------------------------------------------------------------------
  // Public
  // ---------------------------------------------------------------------------

  get isRunning(): boolean { return this._running; }
  get currentFrame(): AudioFrame | null { return this._currentFrame; }
  get frameCount(): number { return this._frameCount; }

  /** Start audio processing from a media stream. */
  async start(stream?: MediaStream): Promise<void> {
    try {
      this._stream = stream ?? await navigator.mediaDevices.getUserMedia({ audio: true });
      this._audioCtx = new AudioContext();
      this._analyser = this._audioCtx.createAnalyser();
      this._analyser.fftSize = this._fftSize;
      this._analyser.smoothingTimeConstant = 0.3;

      this._source = this._audioCtx.createMediaStreamSource(this._stream);
      this._source.connect(this._analyser);

      // Build A-weighting LUT.
      this._aWeightLUT = this._buildAWeightLUT(this._analyser.frequencyBinCount,
        this._audioCtx.sampleRate);

      this._running = true;

      // Calibrate noise floor.
      this._calibrateNoiseFloor();

      // Start analysis loop.
      this._analysisTimer = setInterval(() => this._analyze(), this._analysisIntervalMs);
    } catch (err) {
      this.emit('error', {
        code: 'AUDIO_INIT_FAILED',
        message: err instanceof Error ? err.message : 'Failed to initialize audio engine',
      });
      throw err;
    }
  }

  /** Stop audio processing. */
  stop(): void {
    this._running = false;
    if (this._analysisTimer) {
      clearInterval(this._analysisTimer);
      this._analysisTimer = null;
    }
    if (this._source) {
      this._source.disconnect();
      this._source = null;
    }
    if (this._audioCtx && this._audioCtx.state !== 'closed') {
      this._audioCtx.close().catch(() => {});
    }
  }

  /** Destroy all resources. */
  destroy(): void {
    this.stop();
    if (this._stream) {
      this._stream.getTracks().forEach(t => t.stop());
      this._stream = null;
    }
    this.removeAllListeners();
  }

  // ---------------------------------------------------------------------------
  // Pure DSP functions
  // ---------------------------------------------------------------------------

  /** A-weighting factor for a given frequency. */
  static aWeightFactor(f: number): number {
    if (f <= 0) return 0;
    const f2 = f * f;
    const num = 12194 ** 2 * f2 * f2;
    const den = (f2 + 20.6 ** 2) * Math.sqrt((f2 + 107.7 ** 2) * (f2 + 737.9 ** 2)) * (f2 + 12194 ** 2);
    return den > 0 ? num / den : 0;
  }

  /** Compute RMS level in dB from time-domain data. */
  static computeRmsDb(data: Float32Array): number {
    let sum = 0;
    for (let i = 0; i < data.length; i++) {
      sum += data[i] * data[i];
    }
    const rms = Math.sqrt(sum / data.length);
    return rms > 0 ? 20 * Math.log10(rms) : -100;
  }

  /** Compute peak level in dB. */
  static computePeakDb(data: Float32Array): number {
    let peak = 0;
    for (let i = 0; i < data.length; i++) {
      const abs = Math.abs(data[i]);
      if (abs > peak) peak = abs;
    }
    return peak > 0 ? 20 * Math.log10(peak) : -100;
  }

  /** Compute zero-crossing rate. */
  static computeZCR(data: Float32Array): number {
    let crossings = 0;
    for (let i = 1; i < data.length; i++) {
      if ((data[i] >= 0) !== (data[i - 1] >= 0)) crossings++;
    }
    return crossings / data.length;
  }

  /** Compute spectral centroid from frequency-domain data. */
  static computeSpectralCentroid(freqData: Uint8Array, sampleRate: number, fftSize: number): number {
    let weightedSum = 0;
    let totalMagnitude = 0;
    const binWidth = sampleRate / fftSize;

    for (let i = 0; i < freqData.length; i++) {
      const magnitude = freqData[i];
      const frequency = i * binWidth;
      weightedSum += frequency * magnitude;
      totalMagnitude += magnitude;
    }

    return totalMagnitude > 0 ? weightedSum / totalMagnitude : 0;
  }

  /** Classify audio signal. */
  static classifyAudio(rmsDb: number, zcr: number, centroid: number, noiseFloor: number): AudioFrame['classification'] {
    if (rmsDb < noiseFloor + 6) return 'silence';
    if (centroid > 4000 && zcr > 0.3) return 'noise';
    if (centroid > 2000 && centroid < 5000 && zcr > 0.05 && zcr < 0.25) return 'speech';
    if (centroid > 500 && centroid < 3000 && zcr < 0.1) return 'music';
    return rmsDb > noiseFloor + 12 ? 'speech' : 'silence';
  }

  // ---------------------------------------------------------------------------
  // Private
  // ---------------------------------------------------------------------------

  private _analyze(): void {
    if (!this._analyser || !this._running) return;

    const bufferLength = this._analyser.fftSize;
    const timeData = new Float32Array(bufferLength);
    const freqData = new Uint8Array(this._analyser.frequencyBinCount);

    this._analyser.getFloatTimeDomainData(timeData);
    this._analyser.getByteFrequencyData(freqData);

    const rmsDb = AudioEngine.computeRmsDb(timeData);
    const peakDb = AudioEngine.computePeakDb(timeData);
    const zcr = AudioEngine.computeZCR(timeData);
    const centroid = AudioEngine.computeSpectralCentroid(
      freqData, this._audioCtx!.sampleRate, this._fftSize,
    );

    const classification = AudioEngine.classifyAudio(rmsDb, zcr, centroid, this._noiseFloorDb);
    const voiceDetected = classification === 'speech';
    const speakerCount = voiceDetected ? this._estimateSpeakerCount(rmsDb, zcr) : 0;

    const frame: AudioFrame = {
      rmsDb,
      peakDb,
      zcr,
      spectralCentroid: centroid,
      voiceActivityDetected: voiceDetected,
      speakerCount,
      classification,
      timestamp: performance.now(),
    };

    this._currentFrame = frame;
    this._frameCount++;

    this.emit('frame', frame);

    if (voiceDetected) {
      this.emit('voiceActivity', { detected: true, speakerCount });
    }
  }

  private _estimateSpeakerCount(rmsDb: number, zcr: number): number {
    // Heuristic: higher ZCR + higher volume suggests multiple speakers.
    if (rmsDb > this._noiseFloorDb + 25 && zcr > 0.2) return 2;
    return 1;
  }

  private _calibrateNoiseFloor(): void {
    const startTime = Date.now();
    const calibrationInterval = setInterval(() => {
      if (!this._analyser) { clearInterval(calibrationInterval); return; }

      const timeData = new Float32Array(this._analyser.fftSize);
      this._analyser.getFloatTimeDomainData(timeData);
      this._calibrationSamples.push(AudioEngine.computeRmsDb(timeData));

      if (Date.now() - startTime >= this._calibrationDurationMs) {
        clearInterval(calibrationInterval);
        if (this._calibrationSamples.length > 0) {
          // Use median as noise floor.
          const sorted = [...this._calibrationSamples].sort((a, b) => a - b);
          this._noiseFloorDb = sorted[Math.floor(sorted.length / 2)];
          this._calibrated = true;
        }
      }
    }, 50);
  }

  private _buildAWeightLUT(binCount: number, sampleRate: number): Float32Array {
    const lut = new Float32Array(binCount);
    const binWidth = sampleRate / (binCount * 2);
    for (let i = 0; i < binCount; i++) {
      lut[i] = AudioEngine.aWeightFactor(i * binWidth);
    }
    return lut;
  }
}
