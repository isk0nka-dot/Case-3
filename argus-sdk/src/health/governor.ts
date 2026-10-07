// =============================================================================
// Argus SDK — Health Governor
// =============================================================================
//
// Real-time health scoring based on FPS, RTT, and packet loss.
// Extracted from useHealthGovernor — Vue refs replaced with class state.
//
// Health Score = FPS (35%) + RTT (35%) + PacketLoss (30%)
// Tier A ≥ 70, Tier B ≥ 40, Tier C < 40
// Hysteresis: 3 checks to downgrade, 5 to upgrade (no A↔C jumps)
// =============================================================================

import type { ResilienceTier, HealthMetrics, TierConfig } from '../types';
import { TIER_CONFIGS } from '../types';
import { EventEmitter } from '../core/event-emitter';

interface GovernorEvents {
  healthUpdate: HealthMetrics;
  tierChange: { from: ResilienceTier; to: ResilienceTier; config: TierConfig };
}

/** Health governor options. */
export interface GovernorOptions {
  /** Sampling interval in ms. Default: 5000. */
  sampleIntervalMs?: number;
  /** Server base URL for RTT measurement. */
  serverUrl?: string;
}

/**
 * Health governor — scores device/network health and assigns resilience tiers.
 *
 * The governor runs a periodic sampling loop that measures:
 * - FPS (via requestAnimationFrame)
 * - RTT (via /healthz ping)
 * - Packet loss (from transport metrics)
 *
 * Based on the composite score, it assigns a tier (A/B/C) with hysteresis
 * to prevent rapid oscillation.
 */
export class HealthGovernor extends EventEmitter<GovernorEvents> {
  // Tier state.
  private _currentTier: ResilienceTier = 'A';
  private _score = 100;
  private _fps = 60;
  private _rttMs = 0;
  private _packetLossRate = 0;

  // Hysteresis counters.
  private _downgradeCount = 0;
  private _upgradeCount = 0;

  // FPS measurement.
  private _fpsFrameCount = 0;
  private _fpsLastTime = 0;
  private _rafId: number | null = null;

  // Sampling.
  private _sampleTimer: ReturnType<typeof setInterval> | null = null;
  private readonly _sampleIntervalMs: number;
  private readonly _serverUrl: string;

  // Transport metrics provider.
  private _getTransportMetrics?: () => { totalRequests: number; totalFailures: number };

  constructor(options?: GovernorOptions) {
    super();
    this._sampleIntervalMs = options?.sampleIntervalMs ?? 5000;
    this._serverUrl = options?.serverUrl ?? '';
  }

  // ---------------------------------------------------------------------------
  // Public
  // ---------------------------------------------------------------------------

  get currentTier(): ResilienceTier { return this._currentTier; }
  get currentConfig(): TierConfig { return TIER_CONFIGS[this._currentTier]; }
  get score(): number { return this._score; }

  getMetrics(): HealthMetrics {
    return {
      fps: this._fps,
      rttMs: this._rttMs,
      packetLossRate: this._packetLossRate,
      score: this._score,
      tier: this._currentTier,
      cpuPressure: this._deriveCpuPressure(),
    };
  }

  /** Set a transport metrics provider for packet loss calculation. */
  setTransportMetrics(fn: () => { totalRequests: number; totalFailures: number }): void {
    this._getTransportMetrics = fn;
  }

  /** Start health monitoring. */
  start(): void {
    this._startFpsMonitor();
    this._sampleTimer = setInterval(() => this._sample(), this._sampleIntervalMs);
    this._sample(); // Immediate first sample.
  }

  /** Stop health monitoring. */
  stop(): void {
    if (this._rafId !== null) {
      cancelAnimationFrame(this._rafId);
      this._rafId = null;
    }
    if (this._sampleTimer) {
      clearInterval(this._sampleTimer);
      this._sampleTimer = null;
    }
  }

  /** Destroy the governor. */
  destroy(): void {
    this.stop();
    this.removeAllListeners();
  }

  // ---------------------------------------------------------------------------
  // Health scoring (pure math)
  // ---------------------------------------------------------------------------

  /** Normalize FPS to 0-100 score. */
  static normalizeFps(fps: number): number {
    if (fps >= 30) return 100;
    if (fps >= 20) return 70 + ((fps - 20) / 10) * 30;
    if (fps >= 10) return 30 + ((fps - 10) / 10) * 40;
    return Math.max(0, fps * 3);
  }

  /** Normalize RTT to 0-100 score (lower RTT = higher score). */
  static normalizeRtt(rttMs: number): number {
    if (rttMs <= 50) return 100;
    if (rttMs <= 150) return 80 + ((150 - rttMs) / 100) * 20;
    if (rttMs <= 500) return 40 + ((500 - rttMs) / 350) * 40;
    if (rttMs <= 2000) return 10 + ((2000 - rttMs) / 1500) * 30;
    return 0;
  }

  /** Normalize packet loss to 0-100 score. */
  static normalizePacketLoss(rate: number): number {
    if (rate <= 0.01) return 100;
    if (rate <= 0.05) return 70 + ((0.05 - rate) / 0.04) * 30;
    if (rate <= 0.15) return 30 + ((0.15 - rate) / 0.10) * 40;
    return Math.max(0, 30 * (1 - rate));
  }

  /** Compute composite health score (0-100). */
  static computeScore(fps: number, rttMs: number, packetLoss: number): number {
    const fpsScore = HealthGovernor.normalizeFps(fps);
    const rttScore = HealthGovernor.normalizeRtt(rttMs);
    const plScore = HealthGovernor.normalizePacketLoss(packetLoss);
    return Math.round(fpsScore * 0.35 + rttScore * 0.35 + plScore * 0.30);
  }

  /** Determine tier from score (without hysteresis). */
  static tierFromScore(score: number): ResilienceTier {
    if (score >= 70) return 'A';
    if (score >= 40) return 'B';
    return 'C';
  }

  // ---------------------------------------------------------------------------
  // Private
  // ---------------------------------------------------------------------------

  private _sample(): void {
    // Measure RTT.
    this._measureRtt().then(rtt => {
      this._rttMs = rtt;
    }).catch(() => {
      this._rttMs = 9999; // Assume worst-case on failure.
    });

    // Calculate packet loss from transport metrics.
    if (this._getTransportMetrics) {
      const metrics = this._getTransportMetrics();
      this._packetLossRate = metrics.totalRequests > 0
        ? metrics.totalFailures / metrics.totalRequests
        : 0;
    }

    // Compute score and apply hysteresis.
    this._score = HealthGovernor.computeScore(this._fps, this._rttMs, this._packetLossRate);
    const rawTier = HealthGovernor.tierFromScore(this._score);

    this._applyHysteresis(rawTier);

    this.emit('healthUpdate', this.getMetrics());
  }

  private _applyHysteresis(targetTier: ResilienceTier): void {
    const tierOrder: ResilienceTier[] = ['A', 'B', 'C'];
    const currentIdx = tierOrder.indexOf(this._currentTier);
    const targetIdx = tierOrder.indexOf(targetTier);

    if (targetIdx > currentIdx) {
      // Downgrade requested.
      this._upgradeCount = 0;
      this._downgradeCount++;
      if (this._downgradeCount >= 3) {
        // Only allow one-step downgrades (no A→C jumps).
        const newTier = tierOrder[Math.min(currentIdx + 1, 2)];
        this._changeTier(newTier);
        this._downgradeCount = 0;
      }
    } else if (targetIdx < currentIdx) {
      // Upgrade requested.
      this._downgradeCount = 0;
      this._upgradeCount++;
      if (this._upgradeCount >= 5) {
        // Only allow one-step upgrades.
        const newTier = tierOrder[Math.max(currentIdx - 1, 0)];
        this._changeTier(newTier);
        this._upgradeCount = 0;
      }
    } else {
      // Same tier — reset both counters.
      this._downgradeCount = 0;
      this._upgradeCount = 0;
    }
  }

  private _changeTier(newTier: ResilienceTier): void {
    if (newTier === this._currentTier) return;
    const from = this._currentTier;
    this._currentTier = newTier;
    this.emit('tierChange', { from, to: newTier, config: TIER_CONFIGS[newTier] });
  }

  private async _measureRtt(): Promise<number> {
    if (!this._serverUrl) return 0;
    const start = performance.now();
    try {
      await fetch(`${this._serverUrl}/healthz`, {
        method: 'GET',
        cache: 'no-store',
        signal: AbortSignal.timeout(5000),
      });
      return performance.now() - start;
    } catch {
      return 9999;
    }
  }

  private _startFpsMonitor(): void {
    this._fpsLastTime = performance.now();
    this._fpsFrameCount = 0;

    const loop = (): void => {
      this._fpsFrameCount++;
      const now = performance.now();
      const elapsed = now - this._fpsLastTime;

      if (elapsed >= 1000) {
        this._fps = Math.round((this._fpsFrameCount / elapsed) * 1000);
        this._fpsFrameCount = 0;
        this._fpsLastTime = now;
      }

      this._rafId = requestAnimationFrame(loop);
    };

    this._rafId = requestAnimationFrame(loop);
  }

  private _deriveCpuPressure(): 'nominal' | 'fair' | 'serious' | 'critical' {
    if (this._fps >= 25) return 'nominal';
    if (this._fps >= 15) return 'fair';
    if (this._fps >= 8) return 'serious';
    return 'critical';
  }
}
