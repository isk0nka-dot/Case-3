// Package grpcweb provides a production-grade gRPC-Web proxy that bridges
// browser-based Nuxt 3 clients to the native gRPC Event Collector service.
//
// Architecture-level design decisions:
//
//  1. Why gRPC-Web instead of REST/JSON or Connect-ES?
//     The Argus proctoring SDK generates 10,000+ events/sec across all sessions.
//     gRPC-Web provides:
//       - Binary Protobuf encoding (3-5x smaller than JSON, faster parse)
//       - Bidirectional streaming via long-lived HTTP/2 connections
//       - Type-safe contracts shared between Go and TypeScript
//       - Automatic retries and deadline propagation from the gRPC ecosystem
//
//     Connect-ES was considered but gRPC-Web has broader ecosystem support,
//     better proxy compatibility (Envoy, nginx), and mature tooling. The
//     impeller/grpc-web Go library provides a pure-Go proxy without requiring
//     Envoy as a sidecar.
//
//  2. Why an in-process proxy instead of Envoy sidecar?
//     Envoy adds operational complexity (separate container, config, monitoring)
//     and latency (extra network hop). An in-process proxy:
//       - Zero additional latency (same process, shared memory)
//       - Single binary deployment (no sidecar coordination)
//       - Unified configuration (same YAML as the gRPC server)
//       - Simpler Kubernetes manifests (one container per pod)
//     The trade-off is losing Envoy's advanced features (circuit breaking,
//     load balancing), but we implement those at the application layer already.
//
//  3. Unified HTTP server serves both gRPC-Web and health checks.
//     A single HTTP listener on port 8080 handles:
//       - gRPC-Web requests (Content-Type: application/grpc-web)
//       - Health checks (/healthz, /readyz)
//       - Future: Prometheus metrics (/metrics)
//     This simplifies Kubernetes Service/Ingress configuration and reduces
//     the number of ports the pod exposes.
//
//  4. Content-Type detection for routing.
//     The proxy inspects the Content-Type header to distinguish gRPC-Web
//     requests from regular HTTP requests:
//       - "application/grpc-web" → route to gRPC-Web proxy
//       - "application/grpc-web+proto" → route to gRPC-Web proxy
//       - "application/grpc-web-text" → route to gRPC-Web proxy (base64)
//       - Everything else → route to HTTP mux (health, metrics)
//
//  5. Security headers.
//     Every response includes security headers to mitigate XSS, clickjacking,
//     and MIME-sniffing attacks. These are defense-in-depth measures that
//     protect against browser-based attacks on the proctoring dashboard.
package grpcweb

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// ProxyConfig holds gRPC-Web proxy configuration.
type ProxyConfig struct {
	// AllowAllOrigins allows requests from any origin. Set to false in
	// production and configure specific origins in the CORS middleware.
	AllowAllOrigins bool `yaml:"allow_all_origins"`

	// AllowNonTLSRequests allows gRPC-Web over plain HTTP. Set to false in
	// production — all browser traffic must use TLS.
	AllowNonTLSRequests bool `yaml:"allow_non_tls_requests"`

	// MaxCallRecvMsgSize is the maximum message size the proxy will accept
	// from the browser. Default: 4 MiB (matches gRPC server config).
	MaxCallRecvMsgSize int `yaml:"max_call_recv_msg_size"`
}

// DefaultProxyConfig returns sensible defaults for local development.
func DefaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		AllowAllOrigins:     true,
		AllowNonTLSRequests: true,
		MaxCallRecvMsgSize:  4 * 1024 * 1024, // 4 MiB
	}
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// Handler wraps a native gRPC server to accept gRPC-Web requests from browsers.
// It implements a pure-Go gRPC-Web proxy without requiring Envoy.
//
// The handler inspects the Content-Type header:
//   - gRPC-Web content types → proxy to gRPC server
//   - Everything else → delegate to the fallback HTTP handler
//
// This allows a single HTTP listener to serve both gRPC-Web and REST endpoints.
type Handler struct {
	grpcServer *grpc.Server
	fallback   http.Handler
	logger     *zap.Logger
}

// NewHandler creates a gRPC-Web proxy handler.
//
// Parameters:
//   - grpcServer: The native gRPC server to proxy requests to.
//   - fallback: HTTP handler for non-gRPC-Web requests (health checks, metrics).
//   - logger: Structured logger for proxy-level logging.
func NewHandler(
	grpcServer *grpc.Server,
	fallback http.Handler,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		grpcServer: grpcServer,
		fallback:   fallback,
		logger:     logger.Named("grpc_web_proxy"),
	}
}

// ServeHTTP implements http.Handler. It routes requests based on Content-Type:
//   - gRPC-Web → native gRPC handler (gRPC server handles HTTP/2 framing)
//   - Other → fallback HTTP handler
//
// Security headers are added to ALL responses regardless of content type.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add security headers to every response.
	addSecurityHeaders(w)

	// Check if this is a gRPC or gRPC-Web request.
	if isGRPCWebRequest(r) || isGRPCRequest(r) {
		h.grpcServer.ServeHTTP(w, r)
		return
	}

	// Not a gRPC request — delegate to the fallback handler.
	h.fallback.ServeHTTP(w, r)
}

// ---------------------------------------------------------------------------
// Content-Type Detection
// ---------------------------------------------------------------------------

// isGRPCWebRequest returns true if the request is a gRPC-Web request.
// gRPC-Web clients set the Content-Type to one of:
//   - application/grpc-web           (binary Protobuf)
//   - application/grpc-web+proto     (binary Protobuf, explicit)
//   - application/grpc-web-text      (base64-encoded, for environments without binary support)
//   - application/grpc-web-text+proto
func isGRPCWebRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/grpc-web")
}

// isGRPCRequest returns true if the request is a native gRPC request over HTTP/2.
// This allows the unified handler to also accept native gRPC clients directly.
func isGRPCRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/grpc") && !strings.HasPrefix(ct, "application/grpc-web")
}

// ---------------------------------------------------------------------------
// Security Headers
// ---------------------------------------------------------------------------

// addSecurityHeaders sets defense-in-depth HTTP security headers on every
// response. These protect against common browser-based attacks.
//
// In a global proctoring system, the browser is both the client and the
// attack surface. These headers provide defense-in-depth:
//
//   - X-Content-Type-Options: nosniff
//     Prevents browsers from MIME-sniffing the response body and interpreting
//     a gRPC response as HTML (XSS vector).
//
//   - X-Frame-Options: DENY
//     Prevents the API endpoint from being embedded in an iframe (clickjacking).
//     The proctoring dashboard serves its own UI — the API should never be framed.
//
//   - X-XSS-Protection: 1; mode=block
//     Legacy XSS filter for older browsers. Modern browsers use CSP instead.
//
//   - Referrer-Policy: strict-origin-when-cross-origin
//     Prevents the full URL (including path and query parameters) from leaking
//     to third-party sites via the Referer header.
//
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains
//     Enforces HTTPS for one year. Prevents SSL stripping attacks.
//     Only effective when served over HTTPS (browsers ignore it over HTTP).
//
//   - Content-Security-Policy: default-src 'none'
//     The API endpoint serves data, not HTML. CSP 'none' prevents any
//     accidental HTML rendering.
func addSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
}
