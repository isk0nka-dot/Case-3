// =============================================================================
// Argus SDK — HTTP Transport Layer
// =============================================================================
//
// gRPC-Web compatible JSON transport with retry, backoff, and interceptors.
// Extracted from argus-frontend/app/lib/grpc/transport.ts — framework-agnostic.
// =============================================================================

import type {
  TransportConfig,
  TransportInterceptor,
  TransportRequest,
  TransportResponse,
} from '../types';

/** Transport error with retry context. */
export class TransportError extends Error {
  constructor(
    message: string,
    public readonly status: number = 0,
    public readonly retryable: boolean = false,
    public readonly grpcCode?: number,
  ) {
    super(message);
    this.name = 'TransportError';
  }
}

/** Connection health snapshot. */
export interface ConnectionHealth {
  totalRequests: number;
  totalFailures: number;
  totalBytesSent: number;
  avgRttMs: number;
  isOnline: boolean;
  lastSuccessAt: number;
}

/**
 * HTTP transport for gRPC-Web JSON encoding.
 *
 * Features:
 * - Exponential backoff with jitter
 * - Request/response interceptor pipeline
 * - Online/offline detection
 * - Connection health metrics
 */
export class GrpcWebTransport {
  private readonly baseUrl: string;
  private readonly timeout: number;
  private readonly maxRetries: number;
  private readonly retryBaseDelay: number;
  private readonly retryMaxDelay: number;
  private readonly interceptors: TransportInterceptor[] = [];

  // Metrics
  private _totalRequests = 0;
  private _totalFailures = 0;
  private _totalBytesSent = 0;
  private _rttSamples: number[] = [];
  private _isOnline = true;
  private _lastSuccessAt = 0;

  // Online/offline listeners
  private readonly _onOnline: () => void;
  private readonly _onOffline: () => void;

  constructor(config: TransportConfig) {
    this.baseUrl = config.baseUrl.replace(/\/$/, '');
    this.timeout = config.timeout ?? 10_000;
    this.maxRetries = config.maxRetries ?? 3;
    this.retryBaseDelay = config.retryBaseDelay ?? 100;
    this.retryMaxDelay = config.retryMaxDelay ?? 5_000;

    this._onOnline = () => { this._isOnline = true; };
    this._onOffline = () => { this._isOnline = false; };

    if (typeof window !== 'undefined') {
      window.addEventListener('online', this._onOnline);
      window.addEventListener('offline', this._onOffline);
      this._isOnline = navigator.onLine;
    }
  }

  /** Add a transport interceptor. */
  addInterceptor(interceptor: TransportInterceptor): void {
    this.interceptors.push(interceptor);
  }

  /** Get connection health metrics. */
  getHealth(): ConnectionHealth {
    const samples = this._rttSamples;
    const avgRtt = samples.length > 0
      ? samples.reduce((a, b) => a + b, 0) / samples.length
      : 0;

    return {
      totalRequests: this._totalRequests,
      totalFailures: this._totalFailures,
      totalBytesSent: this._totalBytesSent,
      avgRttMs: Math.round(avgRtt),
      isOnline: this._isOnline,
      lastSuccessAt: this._lastSuccessAt,
    };
  }

  /**
   * Send a unary RPC call (POST with JSON body).
   *
   * @param path  - gRPC service path (e.g., '/argus.v1.EventCollector/IngestBatch')
   * @param body  - JSON-encoded request body
   * @param headers - Additional headers (e.g., Authorization)
   * @param signal - AbortSignal for cancellation
   */
  async call(
    path: string,
    body: string,
    headers: Record<string, string> = {},
    signal?: AbortSignal,
  ): Promise<TransportResponse> {
    let request: TransportRequest = {
      url: `${this.baseUrl}${path}`,
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...headers,
      },
      body,
      signal,
    };

    // Run before-request interceptors.
    for (const interceptor of this.interceptors) {
      if (interceptor.beforeRequest) {
        request = await interceptor.beforeRequest(request);
      }
    }

    let lastError: Error | null = null;

    for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
      if (attempt > 0) {
        const delay = this._backoffDelay(attempt);
        await this._sleep(delay);
      }

      try {
        const response = await this._doFetch(request);
        this._recordSuccess(response);

        // Run after-response interceptors.
        let result = response;
        for (const interceptor of this.interceptors) {
          if (interceptor.afterResponse) {
            result = await interceptor.afterResponse(result);
          }
        }

        return result;
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));

        this._totalFailures++;

        // Notify interceptors.
        for (const interceptor of this.interceptors) {
          if (interceptor.onError) {
            interceptor.onError(lastError);
          }
        }

        // Non-retryable errors.
        if (err instanceof TransportError && !err.retryable) {
          throw err;
        }

        // AbortError is never retryable.
        if (lastError.name === 'AbortError') {
          throw lastError;
        }
      }
    }

    throw lastError ?? new TransportError('Max retries exceeded', 0, false);
  }

  /** Clean up event listeners. */
  destroy(): void {
    if (typeof window !== 'undefined') {
      window.removeEventListener('online', this._onOnline);
      window.removeEventListener('offline', this._onOffline);
    }
  }

  // ---------------------------------------------------------------------------
  // Private
  // ---------------------------------------------------------------------------

  private async _doFetch(request: TransportRequest): Promise<TransportResponse> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    // Combine external signal with our timeout signal.
    const signal = request.signal
      ? this._combineSignals(request.signal, controller.signal)
      : controller.signal;

    this._totalRequests++;
    this._totalBytesSent += request.body.length;

    const start = performance.now();

    try {
      const res = await fetch(request.url, {
        method: request.method,
        headers: request.headers,
        body: request.body,
        signal,
      });

      const rtt = performance.now() - start;
      this._rttSamples.push(rtt);
      if (this._rttSamples.length > 100) this._rttSamples.shift();

      // Parse response headers.
      const responseHeaders: Record<string, string> = {};
      res.headers.forEach((v, k) => { responseHeaders[k] = v; });

      // Read body.
      const text = await res.text();
      let data: unknown;
      try {
        data = JSON.parse(text);
      } catch {
        data = text;
      }

      // Non-2xx status.
      if (!res.ok) {
        const retryable = res.status >= 500 || res.status === 429;
        throw new TransportError(
          `HTTP ${res.status}: ${res.statusText}`,
          res.status,
          retryable,
        );
      }

      return { status: res.status, headers: responseHeaders, data };
    } finally {
      clearTimeout(timeoutId);
    }
  }

  private _backoffDelay(attempt: number): number {
    const base = this.retryBaseDelay * Math.pow(2, attempt - 1);
    const jitter = Math.random() * base * 0.3;
    return Math.min(base + jitter, this.retryMaxDelay);
  }

  private _sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  private _recordSuccess(response: TransportResponse): void {
    this._lastSuccessAt = Date.now();
    // Reset failure count on success (sliding window).
    void response;
  }

  private _combineSignals(a: AbortSignal, b: AbortSignal): AbortSignal {
    const controller = new AbortController();
    const onAbort = () => controller.abort();
    a.addEventListener('abort', onAbort, { once: true });
    b.addEventListener('abort', onAbort, { once: true });
    return controller.signal;
  }
}
