// =============================================================================
// Argus AI — gRPC Transport Interceptors
// =============================================================================
//
// Request/response interceptors for the gRPC-Web transport.
// These run in a pipeline: each interceptor can modify the request before
// it's sent, transform the response after receipt, or handle errors.
//
// Pipeline execution order:
//   beforeRequest:  AuthInterceptor → LoggingInterceptor → MetricsInterceptor
//   afterResponse:  MetricsInterceptor → LoggingInterceptor → AuthInterceptor
//   onError:        All interceptors notified in order
//
// =============================================================================

import type { TransportInterceptor, TransportRequest, TransportResponse, TransportError } from './transport'

// ---------------------------------------------------------------------------
// Auth Interceptor — JWT Token Injection
// ---------------------------------------------------------------------------

/**
 * Token provider function type.
 * Returns the current JWT token or null if not authenticated.
 */
export type TokenProvider = () => string | null

/**
 * Creates an auth interceptor that injects the JWT Bearer token into every
 * gRPC-Web request.
 *
 * The token is retrieved lazily from a provider function (typically bound to
 * the Pinia auth store). This ensures the interceptor always uses the latest
 * token, even after token refresh.
 *
 * Headers injected:
 *   - Authorization: Bearer <jwt>
 *   - X-Argus-Session-Id: <session_id>  (for server-side session correlation)
 *
 * @param getToken - Function that returns the current JWT token or null.
 * @param getSessionId - Optional function that returns the current session ID.
 */
export function createAuthInterceptor(
  getToken: TokenProvider,
  getSessionId?: () => string | null
): TransportInterceptor {
  return {
    beforeRequest(request: TransportRequest): TransportRequest {
      const token = getToken()
      if (token) {
        request.headers['Authorization'] = `Bearer ${token}`
      }

      const sessionId = getSessionId?.()
      if (sessionId) {
        request.headers['X-Argus-Session-Id'] = sessionId
      }

      return request
    },

    onError(error: TransportError): void {
      // 401 Unauthorized — token expired or invalid.
      if (error.httpStatus === 401) {
        console.warn('[argus:auth] JWT token rejected (401). Token may be expired.')
        // In production, trigger token refresh or redirect to login.
        // The Pinia auth store should handle this via an event bus.
      }

      // gRPC UNAUTHENTICATED (16) — server rejected the token.
      if (error.grpcStatus === 16) {
        console.warn('[argus:auth] gRPC UNAUTHENTICATED. Token verification failed.')
      }
    }
  }
}

// ---------------------------------------------------------------------------
// Logging Interceptor — Request/Response Logging
// ---------------------------------------------------------------------------

/** Log level for the logging interceptor. */
export type LogLevel = 'debug' | 'info' | 'warn' | 'error' | 'none'

/**
 * Creates a logging interceptor for development debugging.
 *
 * Logs:
 *   - Request method, URL, and body size
 *   - Response status, gRPC status, and latency
 *   - Errors with classification
 *
 * In production, set level to 'error' or 'none' to reduce console noise.
 */
export function createLoggingInterceptor(level: LogLevel = 'debug'): TransportInterceptor {
  if (level === 'none') {
    return {} // No-op interceptor.
  }

  const shouldLog = (msgLevel: LogLevel): boolean => {
    const levels: LogLevel[] = ['debug', 'info', 'warn', 'error']
    return levels.indexOf(msgLevel) >= levels.indexOf(level)
  }

  // Track request start times for latency calculation.
  const requestTimers = new Map<string, number>()

  return {
    beforeRequest(request: TransportRequest): TransportRequest {
      if (shouldLog('debug')) {
        // Extract the gRPC method name from the URL path.
        const methodName = request.url.split('/').pop() ?? request.url
        const bodySize = new Blob([request.body]).size

        requestTimers.set(request.url, performance.now())

        console.debug(
          `[argus:grpc] → ${methodName}`,
          `(${formatBytes(bodySize)})`,
          request.headers['Authorization'] ? '[auth]' : '[no-auth]'
        )
      }

      return request
    },

    afterResponse(response: TransportResponse): TransportResponse {
      if (shouldLog('debug')) {
        const latency = requestTimers.get('')
          ? `${(performance.now() - (requestTimers.get('') ?? 0)).toFixed(1)}ms`
          : ''

        console.debug(
          `[argus:grpc] ← ${response.status}`,
          response.grpcStatus !== undefined ? `grpc=${response.grpcStatus}` : '',
          latency
        )
      }

      return response
    },

    onError(error: TransportError): void {
      if (shouldLog('warn')) {
        console.warn(
          `[argus:grpc] ✗ ${error.code}`,
          error.message,
          error.retryable ? '[retryable]' : '[fatal]',
          error.httpStatus ? `http=${error.httpStatus}` : '',
          error.grpcStatus ? `grpc=${error.grpcStatus}` : ''
        )
      }
    }
  }
}

// ---------------------------------------------------------------------------
// Metrics Interceptor — Performance Tracking
// ---------------------------------------------------------------------------

/** Metrics snapshot for the transport layer. */
export interface TransportMetrics {
  /** Total requests sent. */
  totalRequests: number
  /** Total successful responses. */
  totalSuccesses: number
  /** Total failed requests. */
  totalFailures: number
  /** Total retries attempted. */
  totalRetries: number
  /** Average response latency in ms. */
  avgLatencyMs: number
  /** P95 response latency in ms. */
  p95LatencyMs: number
  /** Total bytes sent. */
  totalBytesSent: number
  /** Total bytes received. */
  totalBytesReceived: number
  /** Events per second (rolling 10-second window). */
  eventsPerSecond: number
}

/**
 * Creates a metrics interceptor that tracks transport performance.
 *
 * Provides real-time metrics for the dashboard's infrastructure health panel.
 * Metrics are stored in memory with a rolling window for rate calculations.
 */
export function createMetricsInterceptor(): {
  interceptor: TransportInterceptor
  getMetrics: () => TransportMetrics
  reset: () => void
} {
  let totalRequests = 0
  let totalSuccesses = 0
  let totalFailures = 0
  let totalRetries = 0
  let totalBytesSent = 0
  let totalBytesReceived = 0

  // Latency tracking (circular buffer for P95 calculation).
  const latencies: number[] = []
  const MAX_LATENCY_SAMPLES = 1000
  let latencyIndex = 0

  // Rate tracking (timestamps of recent requests).
  const recentRequestTimes: number[] = []
  const RATE_WINDOW_MS = 10_000 // 10-second window

  // Per-request timing.
  const requestStartTimes = new WeakMap<TransportRequest, number>()

  const interceptor: TransportInterceptor = {
    beforeRequest(request: TransportRequest): TransportRequest {
      totalRequests++
      totalBytesSent += new Blob([request.body]).size

      requestStartTimes.set(request, performance.now())
      recentRequestTimes.push(Date.now())

      // Prune old rate entries.
      const cutoff = Date.now() - RATE_WINDOW_MS
      while (recentRequestTimes.length > 0 && recentRequestTimes[0]! < cutoff) {
        recentRequestTimes.shift()
      }

      return request
    },

    afterResponse(response: TransportResponse): TransportResponse {
      totalSuccesses++

      // Estimate response size.
      const responseSize = JSON.stringify(response.data).length
      totalBytesReceived += responseSize

      return response
    },

    onError(_error: TransportError): void {
      totalFailures++
      if (_error.retryable) {
        totalRetries++
      }
    }
  }

  function getMetrics(): TransportMetrics {
    // Calculate average latency.
    const validLatencies = latencies.filter(l => l > 0)
    const avgLatencyMs = validLatencies.length > 0
      ? validLatencies.reduce((a, b) => a + b, 0) / validLatencies.length
      : 0

    // Calculate P95 latency.
    const sorted = [...validLatencies].sort((a, b) => a - b)
    const p95Index = Math.floor(sorted.length * 0.95)
    const p95LatencyMs = sorted[p95Index] ?? 0

    // Calculate events per second.
    const now = Date.now()
    const cutoff = now - RATE_WINDOW_MS
    const recentCount = recentRequestTimes.filter(t => t >= cutoff).length
    const eventsPerSecond = recentCount / (RATE_WINDOW_MS / 1000)

    return {
      totalRequests,
      totalSuccesses,
      totalFailures,
      totalRetries,
      avgLatencyMs: Math.round(avgLatencyMs * 10) / 10,
      p95LatencyMs: Math.round(p95LatencyMs * 10) / 10,
      totalBytesSent,
      totalBytesReceived,
      eventsPerSecond: Math.round(eventsPerSecond * 10) / 10
    }
  }

  function reset(): void {
    totalRequests = 0
    totalSuccesses = 0
    totalFailures = 0
    totalRetries = 0
    totalBytesSent = 0
    totalBytesReceived = 0
    latencies.length = 0
    latencyIndex = 0
    recentRequestTimes.length = 0
  }

  return { interceptor, getMetrics, reset }
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

/** Format bytes into human-readable string. */
function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`
}
