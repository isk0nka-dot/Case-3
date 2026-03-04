// =============================================================================
// Argus AI — gRPC-Web Transport Layer
// =============================================================================
//
// Production-grade gRPC-Web transport for Nuxt 3 browser clients.
//
// Architecture:
//
//   This transport layer speaks the gRPC-Web protocol directly to the Go
//   Event Collector service. The Go server's in-process gRPC-Web proxy
//   (grpcweb.Handler) routes requests based on Content-Type:
//
//     Content-Type: application/grpc-web+proto → binary Protobuf
//     Content-Type: application/grpc-web-text  → base64 Protobuf
//     Content-Type: application/grpc+proto     → native gRPC (HTTP/2)
//
//   Since browsers cannot speak native HTTP/2 gRPC, we use gRPC-Web.
//   However, encoding raw Protobuf in the browser requires either:
//     1. A protobuf-ts/protobuf-es runtime (~30KB gzipped)
//     2. Manual binary encoding (fragile, error-prone)
//
//   Design decision: REST-over-gRPC via JSON
//
//     Instead of binary Protobuf, we use **JSON encoding** sent as standard
//     HTTP POST requests. The Go server uses jsonpb for deserialization.
//     This approach is used by Connect protocol and grpc-gateway.
//
//     For the Argus proctoring system, the JSON overhead (~3-5x larger than
//     Protobuf binary) is acceptable because:
//       1. Events are small (200-500 bytes JSON, 60-150 bytes Protobuf)
//       2. Batch mode amortizes HTTP overhead (100 events per request)
//       3. Browser gzip compression reduces JSON to near-Protobuf sizes
//       4. Developer experience: JSON is debuggable in browser DevTools
//
//     For the StreamEvents bidirectional streaming RPC, we use Server-Sent
//     Events (SSE) + HTTP POST fallback, because WebSocket support requires
//     additional infrastructure (Envoy/nginx WebSocket upgrade).
//
//   Production path: When the system scales beyond 50K concurrent sessions,
//   switch to Connect-ES (@connectrpc/connect-web) for binary Protobuf
//   encoding. The transport layer interface remains identical.
//
// =============================================================================

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Transport configuration. */
export interface TransportConfig {
  /** Base URL of the gRPC-Web server (e.g., "http://localhost:8080"). */
  baseUrl: string

  /** Request timeout in milliseconds. Default: 10_000 (10 seconds). */
  timeout?: number

  /** Maximum number of retry attempts for transient failures. Default: 3. */
  maxRetries?: number

  /** Base delay for exponential backoff in milliseconds. Default: 100. */
  retryBaseDelay?: number

  /** Maximum backoff delay in milliseconds. Default: 5_000. */
  retryMaxDelay?: number
}

/** Interceptor for modifying requests before they are sent. */
export interface TransportInterceptor {
  /** Called before each request. Can modify headers or body. */
  beforeRequest?(request: TransportRequest): TransportRequest | Promise<TransportRequest>

  /** Called after each response. Can transform the response. */
  afterResponse?(response: TransportResponse): TransportResponse | Promise<TransportResponse>

  /** Called on transport errors. Can decide to retry or throw. */
  onError?(error: TransportError): void
}

/** Internal request representation. */
export interface TransportRequest {
  /** Full URL (baseUrl + path). */
  url: string
  /** HTTP method. */
  method: 'POST'
  /** HTTP headers. */
  headers: Record<string, string>
  /** JSON-encoded body. */
  body: string
  /** AbortSignal for cancellation. */
  signal?: AbortSignal
}

/** Internal response representation. */
export interface TransportResponse {
  /** HTTP status code. */
  status: number
  /** Response headers. */
  headers: Record<string, string>
  /** Parsed JSON body. */
  data: unknown
  /** gRPC status code (from grpc-status header or trailer). */
  grpcStatus?: number
  /** gRPC status message. */
  grpcMessage?: string
}

/** Transport-level error with retry information. */
export class TransportError extends Error {
  public readonly originalCause?: Error

  constructor(
    message: string,
    public readonly code: TransportErrorCode,
    public readonly httpStatus?: number,
    public readonly grpcStatus?: number,
    public readonly retryable: boolean = false,
    originalCause?: Error
  ) {
    super(message)
    this.name = 'TransportError'
    this.originalCause = originalCause
  }
}

/** Error classification for retry logic. */
export enum TransportErrorCode {
  /** Network error (DNS, connection refused, timeout). */
  NETWORK = 'NETWORK',
  /** Server returned 5xx. */
  SERVER = 'SERVER',
  /** Server returned 4xx. */
  CLIENT = 'CLIENT',
  /** Request was cancelled (AbortController). */
  CANCELLED = 'CANCELLED',
  /** Response could not be parsed. */
  PARSE = 'PARSE',
  /** gRPC-level error (non-OK grpc-status). */
  GRPC = 'GRPC',
  /** Request timed out. */
  TIMEOUT = 'TIMEOUT',
  /** Rate limited (429). */
  RATE_LIMITED = 'RATE_LIMITED'
}

// ---------------------------------------------------------------------------
// gRPC-Web Transport Implementation
// ---------------------------------------------------------------------------

/**
 * GrpcWebTransport provides a type-safe HTTP transport for communicating
 * with the Go gRPC-Web server.
 *
 * It handles:
 *   - JSON encoding of Proto3 messages
 *   - gRPC status code extraction from trailers
 *   - Exponential backoff with jitter for retries
 *   - Request/response interceptor pipeline
 *   - AbortController-based cancellation
 *   - Connection health monitoring
 */
export class GrpcWebTransport {
  private readonly config: Required<TransportConfig>
  private readonly interceptors: TransportInterceptor[] = []

  // Connection health tracking
  private consecutiveFailures = 0
  private lastSuccessAt = 0

  // Online/offline detection
  private _isOnline = true
  private readonly connectionChangeListeners = new Set<(online: boolean) => void>()

  constructor(config: TransportConfig) {
    this.config = {
      baseUrl: config.baseUrl.replace(/\/+$/, ''), // Strip trailing slashes
      timeout: config.timeout ?? 10_000,
      maxRetries: config.maxRetries ?? 3,
      retryBaseDelay: config.retryBaseDelay ?? 100,
      retryMaxDelay: config.retryMaxDelay ?? 5_000
    }

    // Track browser online/offline events
    if (typeof window !== 'undefined') {
      this._isOnline = navigator.onLine
      window.addEventListener('online', () => this._setOnline(true))
      window.addEventListener('offline', () => this._setOnline(false))
    }
  }

  /** Add an interceptor to the pipeline. */
  addInterceptor(interceptor: TransportInterceptor): void {
    this.interceptors.push(interceptor)
  }

  /** Get connection health status. */
  get isHealthy(): boolean {
    return this.consecutiveFailures < 3
  }

  /** Get the number of consecutive failures. */
  get failureCount(): number {
    return this.consecutiveFailures
  }

  /**
   * Perform a unary RPC call.
   *
   * @param path - gRPC method path (e.g., "/argus.eventcollector.v1.EventCollectorService/IngestEvent")
   * @param body - JSON-encoded request body
   * @returns Parsed JSON response
   */
  async unary<TResponse>(
    path: string,
    body: Record<string, unknown>
  ): Promise<TResponse> {
    const url = `${this.config.baseUrl}${path}`

    let request: TransportRequest = {
      url,
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        // Request compressed responses — gzip/br reduces JSON payloads by 80-87%.
        // The browser automatically decompresses Content-Encoding: gzip/br responses.
        'Accept-Encoding': 'gzip, deflate, br'
      },
      body: JSON.stringify(body)
    }

    // Run beforeRequest interceptors.
    for (const interceptor of this.interceptors) {
      if (interceptor.beforeRequest) {
        request = await interceptor.beforeRequest(request)
      }
    }

    // Execute with retry logic.
    let lastError: TransportError | undefined
    for (let attempt = 0; attempt <= this.config.maxRetries; attempt++) {
      try {
        const response = await this.executeRequest(request)

        // Run afterResponse interceptors.
        let processedResponse = response
        for (const interceptor of this.interceptors) {
          if (interceptor.afterResponse) {
            processedResponse = await interceptor.afterResponse(processedResponse)
          }
        }

        // Track success.
        this.consecutiveFailures = 0
        this.lastSuccessAt = Date.now()

        return processedResponse.data as TResponse
      } catch (err) {
        const transportError = err instanceof TransportError
          ? err
          : new TransportError(
              `Unexpected error: ${(err as Error).message}`,
              TransportErrorCode.NETWORK,
              undefined,
              undefined,
              true,
              err as Error
            )

        lastError = transportError

        // Notify error interceptors.
        for (const interceptor of this.interceptors) {
          if (interceptor.onError) {
            interceptor.onError(transportError)
          }
        }

        // Don't retry non-retryable errors.
        if (!transportError.retryable || attempt === this.config.maxRetries) {
          this.consecutiveFailures++
          throw transportError
        }

        // Exponential backoff with jitter.
        const delay = Math.min(
          this.config.retryBaseDelay * Math.pow(2, attempt) * (0.5 + Math.random() * 0.5),
          this.config.retryMaxDelay
        )
        await sleep(delay)
      }
    }

    // Should never reach here, but TypeScript needs it.
    throw lastError ?? new TransportError('Max retries exceeded', TransportErrorCode.NETWORK)
  }

  // -------------------------------------------------------------------------
  // Online / Offline Detection
  // -------------------------------------------------------------------------

  /** Whether the browser reports being online. */
  get isOnline(): boolean {
    return this._isOnline
  }

  /**
   * Whether the transport is effectively offline.
   * True when: browser reports offline OR 3+ consecutive failures.
   */
  get effectivelyOffline(): boolean {
    return !this._isOnline || this.consecutiveFailures >= 3
  }

  /**
   * Register a callback for online/offline transitions.
   * Returns an unsubscribe function.
   */
  onConnectionChange(callback: (online: boolean) => void): () => void {
    this.connectionChangeListeners.add(callback)
    return () => {
      this.connectionChangeListeners.delete(callback)
    }
  }

  /** Internal: update online status and notify listeners. */
  private _setOnline(online: boolean): void {
    const changed = this._isOnline !== online
    this._isOnline = online

    if (changed) {
      // When coming back online, also check if failures should be reset
      if (online) {
        // Don't reset consecutiveFailures yet — let the next successful
        // request do that. But do notify listeners.
      }

      for (const listener of this.connectionChangeListeners) {
        try {
          listener(online)
        } catch {
          // Swallow listener errors
        }
      }
    }
  }

  /**
   * Execute a single HTTP request with timeout.
   */
  private async executeRequest(request: TransportRequest): Promise<TransportResponse> {
    const controller = new AbortController()
    const timeoutId = setTimeout(() => controller.abort(), this.config.timeout)

    try {
      const fetchResponse = await fetch(request.url, {
        method: request.method,
        headers: request.headers,
        body: request.body,
        signal: request.signal ?? controller.signal,
        credentials: 'include' // Send cookies + Authorization header
      })

      // Extract response headers.
      const responseHeaders: Record<string, string> = {}
      fetchResponse.headers.forEach((value, key) => {
        responseHeaders[key.toLowerCase()] = value
      })

      // Check for gRPC status in trailers/headers.
      const grpcStatus = parseInt(responseHeaders['grpc-status'] ?? '0', 10)
      const grpcMessage = responseHeaders['grpc-message'] ?? ''

      // Handle HTTP errors.
      if (!fetchResponse.ok) {
        const errorCode = this.classifyHttpError(fetchResponse.status)
        throw new TransportError(
          `HTTP ${fetchResponse.status}: ${fetchResponse.statusText}`,
          errorCode,
          fetchResponse.status,
          grpcStatus || undefined,
          this.isRetryableStatus(fetchResponse.status)
        )
      }

      // Handle gRPC errors (HTTP 200 but grpc-status != 0).
      if (grpcStatus !== 0) {
        throw new TransportError(
          `gRPC error ${grpcStatus}: ${decodeURIComponent(grpcMessage)}`,
          TransportErrorCode.GRPC,
          200,
          grpcStatus,
          this.isRetryableGrpcStatus(grpcStatus)
        )
      }

      // Parse response body.
      let data: unknown
      const contentType = responseHeaders['content-type'] ?? ''
      if (contentType.includes('application/json')) {
        data = await fetchResponse.json()
      } else {
        // Try JSON parsing anyway (some servers don't set Content-Type).
        const text = await fetchResponse.text()
        try {
          data = JSON.parse(text)
        } catch {
          data = { raw: text }
        }
      }

      return {
        status: fetchResponse.status,
        headers: responseHeaders,
        data,
        grpcStatus,
        grpcMessage: grpcMessage || undefined
      }
    } catch (err) {
      if (err instanceof TransportError) throw err

      // AbortController timeout.
      if ((err as Error).name === 'AbortError') {
        throw new TransportError(
          'Request timed out',
          TransportErrorCode.TIMEOUT,
          undefined,
          undefined,
          true
        )
      }

      // Network errors (DNS, connection refused, etc.).
      throw new TransportError(
        `Network error: ${(err as Error).message}`,
        TransportErrorCode.NETWORK,
        undefined,
        undefined,
        true,
        err as Error
      )
    } finally {
      clearTimeout(timeoutId)
    }
  }

  /**
   * Classify HTTP status codes into error categories.
   */
  private classifyHttpError(status: number): TransportErrorCode {
    if (status === 429) return TransportErrorCode.RATE_LIMITED
    if (status >= 500) return TransportErrorCode.SERVER
    if (status >= 400) return TransportErrorCode.CLIENT
    return TransportErrorCode.NETWORK
  }

  /**
   * Determine if an HTTP status code is retryable.
   * 429 (rate limited), 502, 503, 504 are retryable.
   */
  private isRetryableStatus(status: number): boolean {
    return status === 429 || status === 502 || status === 503 || status === 504
  }

  /**
   * Determine if a gRPC status code is retryable.
   * UNAVAILABLE (14), DEADLINE_EXCEEDED (4), RESOURCE_EXHAUSTED (8) are retryable.
   */
  private isRetryableGrpcStatus(status: number): boolean {
    return status === 14 || status === 4 || status === 8
  }
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}
