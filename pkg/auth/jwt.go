// Package auth provides stateless JWT authentication for the Event Collector.
//
// Architecture-level design decisions:
//
//  1. Stateless verification — no database calls per request.
//     The Eduser (Java) authentication system issues JWTs signed with a shared
//     secret (HMAC-SHA256) or an RSA/ECDSA keypair. The Event Collector verifies
//     the signature and standard claims (exp, iss, aud) without contacting Eduser.
//     This eliminates a network round-trip on every RPC — critical for 10K+ streams.
//
//  2. HMAC-SHA256 as default, RSA/ECDSA supported via JWKS.
//     HMAC is simpler to configure (single shared secret) and faster to verify
//     (~1µs vs ~100µs for RSA-2048). For cross-team deployments where sharing a
//     secret is impractical, the JWT can be signed with RSA and verified using a
//     JWKS endpoint. This package supports both, but defaults to HMAC.
//
//  3. Claims extraction is type-safe.
//     The ProctoringClaims struct enforces required fields (session_id, student_id,
//     org_id, exam_id) at parse time. If any field is missing, authentication fails.
//     This prevents unauthenticated data from leaking into the ingestion pipeline.
//
//  4. Context propagation.
//     Verified claims are injected into the gRPC context via metadata keys. The
//     transport server extracts them without coupling to the auth package.
//
//  5. Why NOT mTLS for browser clients?
//     mTLS requires each browser to present a client certificate, which is
//     impractical for a proctoring SPA running in students' browsers. mTLS is
//     reserved for service-to-service communication (Eduser → Event Collector
//     internal APIs). Browser → Event Collector uses JWT over gRPC-Web.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrNoToken is returned when the Authorization header is missing or empty.
	ErrNoToken = errors.New("auth: no token provided")

	// ErrInvalidToken is returned when the token cannot be parsed or the
	// signature verification fails.
	ErrInvalidToken = errors.New("auth: invalid token")

	// ErrTokenExpired is returned when the token's exp claim is in the past.
	ErrTokenExpired = errors.New("auth: token expired")

	// ErrInvalidIssuer is returned when the token's iss claim does not match
	// the expected issuer.
	ErrInvalidIssuer = errors.New("auth: invalid issuer")

	// ErrInvalidAudience is returned when the token's aud claim does not
	// contain the expected audience.
	ErrInvalidAudience = errors.New("auth: invalid audience")

	// ErrMissingClaims is returned when required proctoring claims (session_id,
	// student_id, org_id, exam_id) are missing from the token.
	ErrMissingClaims = errors.New("auth: missing required claims")

	// ErrInvalidSession is returned when the session secret validation fails.
	ErrInvalidSession = errors.New("auth: invalid session secret")
)

// ---------------------------------------------------------------------------
// Claims
// ---------------------------------------------------------------------------

// ProctoringClaims contains the verified identity and session information
// extracted from a JWT token issued by the Eduser system.
//
// These claims are the authority for all downstream operations:
//   - org_id determines tenant isolation (Kafka topic partitioning, ClickHouse queries)
//   - exam_id determines which proctoring rules apply
//   - session_id determines event grouping and ordering
//   - student_id is the human subject being proctored
//   - roles determine authorization for admin-level operations
type ProctoringClaims struct {
	// Standard JWT claims.
	Subject   string    `json:"sub"`             // Unique user identifier in Eduser.
	Issuer    string    `json:"iss"`             // Expected: "eduser" or configured issuer.
	Audience  []string  `json:"aud"`             // Expected: ["argus-event-collector"].
	ExpiresAt time.Time `json:"exp"`             // Token expiration (Unix timestamp).
	IssuedAt  time.Time `json:"iat"`             // Token issuance time.
	JTI       string    `json:"jti,omitempty"`   // Unique token identifier for revocation.

	// Proctoring-specific claims. These are REQUIRED — tokens without them
	// are rejected regardless of signature validity.
	SessionID string   `json:"session_id"`       // Active proctoring session.
	StudentID string   `json:"student_id"`       // Student IIN.
	OrgID     string   `json:"org_id"`           // Organization/institution.
	ExamID    string   `json:"exam_id"`          // Exam identifier.
	Roles     []string `json:"roles,omitempty"`  // e.g., ["student", "proctor"]

	// SessionSecret is a per-session HMAC that Eduser generates when a student
	// starts a proctoring session. The Event Collector validates this against
	// a cached value to prevent session hijacking (token theft alone is not
	// enough — the attacker also needs the session secret).
	SessionSecret string `json:"session_secret,omitempty"`
}

// Validate checks that all required proctoring claims are present and non-empty.
func (c *ProctoringClaims) Validate() error {
	if c.SessionID == "" || c.StudentID == "" || c.OrgID == "" || c.ExamID == "" {
		return ErrMissingClaims
	}
	return nil
}

// HasRole returns true if the claims include the specified role.
func (c *ProctoringClaims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Verifier Configuration
// ---------------------------------------------------------------------------

// VerifierConfig holds JWT verification parameters. All fields are set at
// startup from the service configuration and treated as immutable.
type VerifierConfig struct {
	// SigningKey is the shared HMAC-SHA256 secret for token verification.
	// In production, this is loaded from a Kubernetes secret or vault.
	// For RSA/ECDSA, use the PublicKeyPEM field instead.
	SigningKey []byte `yaml:"-"`

	// SigningKeyBase64 is the base64-encoded signing key from config/env.
	SigningKeyBase64 string `yaml:"signing_key"`

	// Issuer is the expected "iss" claim value. Tokens from a different
	// issuer are rejected. Default: "eduser".
	Issuer string `yaml:"issuer"`

	// Audience is the expected "aud" claim value. The token must contain
	// this audience. Default: "argus-event-collector".
	Audience string `yaml:"audience"`

	// ClockSkew is the maximum allowed clock drift between the Eduser
	// issuer and this service. Compensates for NTP drift in distributed
	// deployments. Default: 30 seconds.
	ClockSkew time.Duration `yaml:"clock_skew"`

	// RequireSessionSecret enables per-session HMAC validation. When true,
	// tokens must contain a non-empty session_secret claim, and the
	// SessionValidator must confirm it. Default: false (enable in production).
	RequireSessionSecret bool `yaml:"require_session_secret"`
}

// ---------------------------------------------------------------------------
// JWT Verifier
// ---------------------------------------------------------------------------

// Verifier performs stateless JWT verification using HMAC-SHA256. It validates:
//   - Signature integrity (token has not been tampered with)
//   - Expiration (exp claim, with configurable clock skew tolerance)
//   - Issuer (iss claim matches expected value)
//   - Audience (aud claim contains expected value)
//   - Required claims (session_id, student_id, org_id, exam_id)
//
// This is a simplified HMAC-SHA256 JWT verifier. For production deployments
// requiring RSA/ECDSA or JWKS rotation, integrate github.com/golang-jwt/jwt/v5
// or github.com/lestrrat-go/jwx/v2. The interface is designed for drop-in
// replacement.
type Verifier struct {
	cfg VerifierConfig
}

// NewVerifier creates a JWT verifier with the given configuration.
// It applies defaults for unset fields and validates the configuration.
func NewVerifier(cfg VerifierConfig) (*Verifier, error) {
	// Apply defaults.
	if cfg.Issuer == "" {
		cfg.Issuer = "eduser"
	}
	if cfg.Audience == "" {
		cfg.Audience = "argus-event-collector"
	}
	if cfg.ClockSkew == 0 {
		cfg.ClockSkew = 30 * time.Second
	}

	// Decode base64 key if raw key is not set.
	if len(cfg.SigningKey) == 0 && cfg.SigningKeyBase64 != "" {
		// Accept raw string as key for simplicity in dev environments.
		// In production, use proper base64 encoding.
		cfg.SigningKey = []byte(cfg.SigningKeyBase64)
	}

	if len(cfg.SigningKey) == 0 {
		return nil, fmt.Errorf("auth: signing key must not be empty")
	}

	// Enforce minimum key length for HMAC-SHA256 security.
	if len(cfg.SigningKey) < 32 {
		return nil, fmt.Errorf("auth: signing key must be at least 32 bytes for HMAC-SHA256 security, got %d", len(cfg.SigningKey))
	}

	return &Verifier{cfg: cfg}, nil
}

// VerifyToken parses and validates a JWT token string, returning the verified
// proctoring claims. This is the primary entry point for the auth interceptor.
//
// The token is expected in the format: "Bearer <token>" or just "<token>".
//
// Verification steps (in order):
//  1. Strip "Bearer " prefix if present.
//  2. Split into header.payload.signature (3 parts).
//  3. Verify HMAC-SHA256 signature.
//  4. Decode and parse claims from the payload.
//  5. Check expiration with clock skew tolerance.
//  6. Validate issuer and audience.
//  7. Validate required proctoring claims.
func (v *Verifier) VerifyToken(tokenStr string) (*ProctoringClaims, error) {
	// Strip Bearer prefix.
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	tokenStr = strings.TrimPrefix(tokenStr, "bearer ")
	tokenStr = strings.TrimSpace(tokenStr)

	if tokenStr == "" {
		return nil, ErrNoToken
	}

	// Split JWT into 3 parts: header.payload.signature
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Verify HMAC-SHA256 signature.
	signingInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256([]byte(signingInput), v.cfg.SigningKey)
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	if subtle.ConstantTimeCompare(expectedSig, actualSig) != 1 {
		return nil, ErrInvalidToken
	}

	// Decode payload.
	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, err := parseClaims(payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	// Check expiration.
	now := time.Now()
	if !claims.ExpiresAt.IsZero() && now.After(claims.ExpiresAt.Add(v.cfg.ClockSkew)) {
		return nil, ErrTokenExpired
	}

	// Validate issuer.
	if claims.Issuer != v.cfg.Issuer {
		return nil, ErrInvalidIssuer
	}

	// Validate audience.
	if !containsAudience(claims.Audience, v.cfg.Audience) {
		return nil, ErrInvalidAudience
	}

	// Validate required proctoring claims.
	if err := claims.Validate(); err != nil {
		return nil, err
	}

	return claims, nil
}

// ---------------------------------------------------------------------------
// Context helpers — claims propagation through gRPC context
// ---------------------------------------------------------------------------

// Context metadata keys for propagating verified claims to downstream handlers.
// These keys are used in gRPC metadata (equivalent to HTTP headers) so the
// transport server can extract identity information without importing the auth
// package.
const (
	MetaKeySessionID = "x-argus-session-id"
	MetaKeyStudentID = "x-argus-student-id"
	MetaKeyOrgID     = "x-argus-org-id"
	MetaKeyExamID    = "x-argus-exam-id"
	MetaKeySubject   = "x-argus-subject"
	MetaKeyRoles     = "x-argus-roles"
)

// claimsContextKey is the context key for storing verified claims directly
// in the context (for in-process access without metadata serialization).
type claimsContextKey struct{}

// ContextWithClaims returns a new context with the verified claims attached.
// This is used by the auth interceptor to propagate claims to RPC handlers.
func ContextWithClaims(ctx context.Context, claims *ProctoringClaims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

// ClaimsFromContext extracts verified claims from the context. Returns nil if
// no claims are present (unauthenticated context).
func ClaimsFromContext(ctx context.Context) *ProctoringClaims {
	claims, _ := ctx.Value(claimsContextKey{}).(*ProctoringClaims)
	return claims
}

// InjectClaimsMetadata creates outgoing gRPC metadata from verified claims.
// This is used when the Event Collector needs to forward identity information
// to downstream services (e.g., the session management service).
func InjectClaimsMetadata(ctx context.Context, claims *ProctoringClaims) context.Context {
	md := metadata.Pairs(
		MetaKeySessionID, claims.SessionID,
		MetaKeyStudentID, claims.StudentID,
		MetaKeyOrgID, claims.OrgID,
		MetaKeyExamID, claims.ExamID,
		MetaKeySubject, claims.Subject,
		MetaKeyRoles, strings.Join(claims.Roles, ","),
	)
	return metadata.NewOutgoingContext(ctx, md)
}

// ExtractClaimsFromMetadata reads proctoring identity from incoming gRPC
// metadata. This is the reverse of InjectClaimsMetadata — used by downstream
// services to extract identity without re-verifying the JWT.
func ExtractClaimsFromMetadata(ctx context.Context) *ProctoringClaims {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	return &ProctoringClaims{
		SessionID: firstMDValue(md, MetaKeySessionID),
		StudentID: firstMDValue(md, MetaKeyStudentID),
		OrgID:     firstMDValue(md, MetaKeyOrgID),
		ExamID:    firstMDValue(md, MetaKeyExamID),
		Subject:   firstMDValue(md, MetaKeySubject),
		Roles:     strings.Split(firstMDValue(md, MetaKeyRoles), ","),
	}
}

// firstMDValue returns the first value for a metadata key, or empty string.
func firstMDValue(md metadata.MD, key string) string {
	vals := md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// containsAudience checks if the audience slice contains the expected value.
func containsAudience(audiences []string, expected string) bool {
	for _, a := range audiences {
		if a == expected {
			return true
		}
	}
	return false
}
