// =============================================================================
// Argus AI — gRPC-Web Nuxt Plugin
// =============================================================================
//
// Initializes the gRPC-Web transport and EventCollector client as a Nuxt plugin.
// The `.client` suffix ensures this plugin runs only in the browser (not SSR).
//
// What this plugin does:
//   1. Creates a GrpcWebTransport with the configured backend URL
//   2. Adds an Auth interceptor that reads JWT from the Pinia auth store
//   3. Adds a Logging interceptor (debug in dev, errors-only in production)
//   4. Adds a Metrics interceptor for transport performance tracking
//   5. Creates an EventCollectorClient with smart batching
//   6. Provides both transport and client via `useNuxtApp().$grpc`
//
// Usage in components/composables:
//
//   const { $grpc } = useNuxtApp()
//   await $grpc.client.ingestEvent({ event: myEvent })
//   $grpc.client.startBatching()
//   await $grpc.client.queueEvent(telemetryEvent)
//
// =============================================================================

import { useAuthStore as createAuthStore } from '~/stores/useAuthStore'
import { GrpcWebTransport } from '~/lib/grpc/transport'
import { EventCollectorClient } from '~/lib/grpc/client'
import {
  createAuthInterceptor,
  createIdempotencyInterceptor,
  createLoggingInterceptor,
  createMetricsInterceptor
} from '~/lib/grpc/interceptors'
import type { TransportMetrics } from '~/lib/grpc/interceptors'

// ---------------------------------------------------------------------------
// Plugin Type Declaration
// ---------------------------------------------------------------------------

export interface GrpcPlugin {
  /** The raw transport for advanced use cases. */
  transport: GrpcWebTransport

  /** The typed EventCollector client. */
  client: EventCollectorClient

  /** Get current transport metrics. */
  getMetrics: () => TransportMetrics

  /** Reset transport metrics. */
  resetMetrics: () => void

  /** Check if the transport is healthy (< 3 consecutive failures). */
  isHealthy: () => boolean
}

// ---------------------------------------------------------------------------
// Plugin Registration
// ---------------------------------------------------------------------------

export default defineNuxtPlugin((_nuxtApp) => {
  // -------------------------------------------------------------------------
  // Step 1: Create Transport
  // -------------------------------------------------------------------------
  const runtimeConfig = useRuntimeConfig()
  const grpcUrl = (runtimeConfig.public?.grpcUrl as string) || 'http://localhost:8080'
  const isDev = import.meta.dev

  const transport = new GrpcWebTransport({
    baseUrl: grpcUrl,
    timeout: 10_000,
    maxRetries: 3,
    retryBaseDelay: 100,
    retryMaxDelay: 5_000
  })

  // -------------------------------------------------------------------------
  // Step 2: Add Auth Interceptor
  // -------------------------------------------------------------------------
  //
  // The auth interceptor lazily reads the JWT token from the Pinia auth store.
  // This ensures the interceptor always has the latest token, even after
  // token refresh.
  //
  // The token provider is a closure that captures the Pinia store reference.
  // Since Nuxt plugins run after Pinia is initialized, the store is guaranteed
  // to be available.
  //
  let authStoreRef: ReturnType<typeof createAuthStore> | null = null

  function getAuthStore(): ReturnType<typeof createAuthStore> {
    if (!authStoreRef) {
      authStoreRef = createAuthStore()
    }
    return authStoreRef
  }

  const authInterceptor = createAuthInterceptor(
    // Token provider — returns the JWT token from the Pinia auth store.
    () => {
      try {
        const store = getAuthStore()
        return store.jwtToken ?? null
      } catch {
        return null
      }
    },
    // Session ID provider — returns the current session from auth store.
    () => {
      try {
        const store = getAuthStore()
        return store.sessionId ?? null
      } catch {
        return null
      }
    }
  )
  transport.addInterceptor(authInterceptor)

  // -------------------------------------------------------------------------
  // Step 2b: Add Idempotency Interceptor
  // -------------------------------------------------------------------------
  //
  // Attaches a unique X-Idempotency-Key header to every request.
  // The backend's dedup interceptor uses this to prevent duplicate
  // event ingestion during transport retries.
  //
  const idempotencyInterceptor = createIdempotencyInterceptor()
  transport.addInterceptor(idempotencyInterceptor)

  // -------------------------------------------------------------------------
  // Step 3: Add Logging Interceptor
  // -------------------------------------------------------------------------
  const loggingInterceptor = createLoggingInterceptor(isDev ? 'debug' : 'error')
  transport.addInterceptor(loggingInterceptor)

  // -------------------------------------------------------------------------
  // Step 4: Add Metrics Interceptor
  // -------------------------------------------------------------------------
  const { interceptor: metricsInterceptor, getMetrics, reset: resetMetrics } = createMetricsInterceptor()
  transport.addInterceptor(metricsInterceptor)

  // -------------------------------------------------------------------------
  // Step 5: Create EventCollector Client
  // -------------------------------------------------------------------------
  const client = new EventCollectorClient(transport, {
    maxBatchSize: 100,
    flushIntervalMs: 500,
    separateTelemetry: true
  })

  // -------------------------------------------------------------------------
  // Step 6: Provide via Nuxt Plugin
  // -------------------------------------------------------------------------
  const grpc: GrpcPlugin = {
    transport,
    client,
    getMetrics,
    resetMetrics,
    isHealthy: () => transport.isHealthy
  }

  return {
    provide: {
      grpc
    }
  }
})

// ---------------------------------------------------------------------------
// TypeScript Module Augmentation
// ---------------------------------------------------------------------------

declare module '#app' {
  interface NuxtApp {
    $grpc: GrpcPlugin
  }
}

declare module 'vue' {
  interface ComponentCustomProperties {
    $grpc: GrpcPlugin
  }
}
