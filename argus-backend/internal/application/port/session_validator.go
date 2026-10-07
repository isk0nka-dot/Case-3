// Package port defines the interfaces (ports) that the application layer
// requires from infrastructure.
package port

import (
	"context"
)

// SessionValidator validates that a proctoring session is active and authorized.
//
// This port abstracts the session validation logic so the transport layer
// can verify sessions without coupling to the Eduser system's implementation.
//
// Implementations:
//   - CachedSessionValidator: In-memory TTL cache backed by Eduser gRPC/HTTP API.
//   - NoopSessionValidator: Always returns valid (for local development/testing).
//
// The session validation flow:
//
//	Browser → [JWT with session_id] → Event Collector
//	  → Auth Interceptor verifies JWT signature + claims
//	  → SessionValidator.ValidateSession(session_id, session_secret)
//	    → Cache hit: return cached result (0ms)
//	    → Cache miss: call Eduser API to verify session (10-50ms)
//	  → If valid: proceed with event ingestion
//	  → If invalid: reject with codes.Unauthenticated
type SessionValidator interface {
	// ValidateSession checks whether the given session is active and the
	// session secret matches. Returns nil if the session is valid, or an
	// error describing why it is invalid.
	//
	// Parameters:
	//   - ctx: Request context with deadline. Implementations should respect
	//     context cancellation for graceful shutdown.
	//   - sessionID: The proctoring session identifier from the JWT claims.
	//   - sessionSecret: The per-session HMAC from the JWT claims (optional,
	//     depends on VerifierConfig.RequireSessionSecret).
	ValidateSession(ctx context.Context, sessionID string, sessionSecret string) error

	// InvalidateSession removes a session from the cache, forcing re-validation
	// on the next request. Called when the Eduser system notifies the Event
	// Collector that a session has been terminated.
	InvalidateSession(ctx context.Context, sessionID string) error
}
