// Package cors provides CORS middleware for the HTTP/gRPC-Web transport.
//
// Architecture-level design decisions:
//
//  1. Why CORS is critical for a proctoring SPA.
//     The Nuxt 3 frontend runs on a different origin (e.g., https://app.argus.ai)
//     than the Event Collector gRPC-Web endpoint (e.g., https://api.argus.ai:443).
//     Browsers enforce the Same-Origin Policy, blocking cross-origin requests
//     unless the server explicitly allows them via CORS headers. Without CORS,
//     the proctoring SDK cannot send events.
//
//  2. Preflight caching (Access-Control-Max-Age: 86400).
//     Browsers send an OPTIONS preflight before every non-simple cross-origin
//     request. At 10K+ concurrent sessions, each with 10-30 events/sec, that's
//     potentially 100K+ extra OPTIONS requests per second. A 24-hour max-age
//     tells browsers to cache the preflight result and skip it for subsequent
//     requests from the same origin.
//
//  3. Strict origin allowlist — no wildcard in production.
//     Access-Control-Allow-Origin: * is convenient for development but
//     disastrous in production — it allows any website to send events to the
//     Event Collector, enabling CSRF-like attacks. This middleware requires an
//     explicit list of allowed origins and rejects everything else.
//
//  4. Exposed headers for gRPC-Web.
//     gRPC-Web uses custom response headers (grpc-status, grpc-message,
//     grpc-status-details-bin) that browsers hide by default. The middleware
//     exposes these so the client SDK can read gRPC status codes.
//
//  5. Credentials support (Access-Control-Allow-Credentials: true).
//     Required for sending the Authorization header (JWT token) in cross-origin
//     requests. When credentials are enabled, the origin cannot be wildcard —
//     the middleware echoes the exact origin from the request.
package cors

import (
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// Config holds CORS middleware configuration.
type Config struct {
	// AllowedOrigins is the list of origins permitted to make cross-origin
	// requests. Each entry must be an exact match (protocol + host + port).
	// Example: ["https://app.argus.ai", "https://staging.argus.ai"]
	//
	// Special value "*" allows all origins (development only — NEVER in production).
	AllowedOrigins []string `yaml:"allowed_origins"`

	// AllowedMethods is the list of HTTP methods permitted in cross-origin
	// requests. Default: ["GET", "POST", "OPTIONS"].
	// gRPC-Web uses POST exclusively, but GET is needed for health checks.
	AllowedMethods []string `yaml:"allowed_methods"`

	// AllowedHeaders is the list of HTTP headers the client is permitted to
	// send in cross-origin requests.
	// gRPC-Web requires: Content-Type, X-Grpc-Web, Authorization, X-User-Agent.
	AllowedHeaders []string `yaml:"allowed_headers"`

	// ExposedHeaders is the list of headers the browser is allowed to access
	// from the response. gRPC-Web requires: Grpc-Status, Grpc-Message,
	// Grpc-Status-Details-Bin.
	ExposedHeaders []string `yaml:"exposed_headers"`

	// MaxAge is the maximum time (in seconds) that the browser caches the
	// preflight response. Default: 86400 (24 hours).
	MaxAge string `yaml:"max_age"`

	// AllowCredentials indicates whether the browser should send credentials
	// (cookies, Authorization header) with cross-origin requests.
	// Must be true for JWT-based authentication. Default: true.
	AllowCredentials bool `yaml:"allow_credentials"`
}

// DefaultConfig returns a CORS configuration suitable for the Argus proctoring
// system. It allows the standard gRPC-Web headers and enables credentials
// for JWT authentication.
//
// IMPORTANT: Set AllowedOrigins to your actual frontend domains before
// deploying to production.
func DefaultConfig() Config {
	return Config{
		AllowedOrigins: []string{"*"}, // Override in production!
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept",
			"Accept-Language",
			"Content-Type",
			"Content-Language",
			"Authorization",
			"X-Auth-Token",
			"X-Grpc-Web",
			"X-User-Agent",
			"X-Requested-With",
			"Grpc-Timeout",
			"X-Argus-Session-Id",
		},
		ExposedHeaders: []string{
			"Grpc-Status",
			"Grpc-Message",
			"Grpc-Status-Details-Bin",
			"X-Argus-Request-Id",
		},
		MaxAge:           "86400",
		AllowCredentials: true,
	}
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// Middleware returns an http.Handler that wraps the given handler with CORS
// headers. It handles OPTIONS preflight requests and adds CORS headers to
// all responses.
//
// Usage with gRPC-Web:
//
//	grpcWebHandler := grpcweb.WrapServer(grpcServer)
//	corsHandler := cors.Middleware(corsConfig)(grpcWebHandler)
//	http.ListenAndServe(":8080", corsHandler)
func Middleware(cfg Config) func(http.Handler) http.Handler {
	// Pre-compute the allowed origins set for O(1) lookup.
	originsSet := make(map[string]bool, len(cfg.AllowedOrigins))
	allowAll := false
	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		originsSet[origin] = true
	}

	// Pre-compute header values (computed once, used on every request).
	methodsStr := strings.Join(cfg.AllowedMethods, ", ")
	headersStr := strings.Join(cfg.AllowedHeaders, ", ")
	exposedStr := strings.Join(cfg.ExposedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Determine if this origin is allowed.
			if origin != "" {
				allowed := allowAll || originsSet[origin]
				if !allowed {
					// Origin not allowed — respond without CORS headers.
					// The browser will block the response on the client side.
					next.ServeHTTP(w, r)
					return
				}

				// Set the allowed origin. When credentials are enabled,
				// we MUST echo the specific origin (not "*").
				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}

				// Vary tells caches that the response depends on the Origin header.
				w.Header().Add("Vary", "Origin")

				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				if exposedStr != "" {
					w.Header().Set("Access-Control-Expose-Headers", exposedStr)
				}
			}

			// Handle preflight OPTIONS request.
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", methodsStr)
				w.Header().Set("Access-Control-Allow-Headers", headersStr)
				if cfg.MaxAge != "" {
					w.Header().Set("Access-Control-Max-Age", cfg.MaxAge)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
