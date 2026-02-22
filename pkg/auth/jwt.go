// Package auth provides stateless JWT authentication for the Event Collector.
//
// Architecture-level design decisions (ADR-006 supersedes ADR-005):
//
//  1. Stateless verification — no database calls per request.
//     The Eduser (Java) authentication system issues JWTs signed with an RSA-256
//     private key. The Event Collector verifies the signature using only the
//     public key, without contacting Eduser. This eliminates a network round-trip
//     on every RPC — critical for 10K+ streams.
//
//  2. RSA-256 (asymmetric) as default, HMAC-SHA256 supported for migration.
//     RSA-256 provides key separation: the API server holds the private key to
//     sign tokens; workers and downstream services hold only the public key to
//     verify. A compromised worker CANNOT forge admin tokens. HMAC-SHA256 remains
//     supported as a legacy fallback during migration (set Algorithm="HS256").
//
//  3. Claims extraction is type-safe.
//     The ProctoringClaims struct enforces required fields (session_id, student_id,
//     org_id, exam_id) at parse time. If any field is missing, authentication fails.
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
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
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

	// ErrInvalidAlgorithm is returned when the JWT header specifies an
	// algorithm that does not match the verifier's configuration.
	ErrInvalidAlgorithm = errors.New("auth: algorithm mismatch")
)

// ---------------------------------------------------------------------------
// Algorithm constants
// ---------------------------------------------------------------------------

const (
	// AlgorithmRS256 indicates RSA-SHA256 asymmetric signing (recommended).
	AlgorithmRS256 = "RS256"

	// AlgorithmHS256 indicates HMAC-SHA256 symmetric signing (legacy).
	AlgorithmHS256 = "HS256"
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
	// Algorithm selects the JWT signing algorithm.
	// "RS256" (default, recommended): RSA-SHA256 asymmetric verification.
	// "HS256" (legacy): HMAC-SHA256 symmetric verification.
	Algorithm string `yaml:"algorithm"`

	// ── RSA-256 fields (recommended) ─────────────────────────────────

	// PublicKeyPEM is the PEM-encoded RSA public key for RS256 verification.
	// The worker only needs the public key — it CANNOT forge tokens.
	PublicKeyPEM []byte `yaml:"-"`

	// PublicKeyPath is the filesystem path to the PEM-encoded RSA public key.
	// Used to load PublicKeyPEM at startup.
	PublicKeyPath string `yaml:"public_key_path"`

	// PrivateKeyPEM is the PEM-encoded RSA private key for RS256 signing.
	// Only the API server needs this — workers MUST NOT have access.
	PrivateKeyPEM []byte `yaml:"-"`

	// PrivateKeyPath is the filesystem path to the PEM-encoded RSA private key.
	// Used to load PrivateKeyPEM at startup.
	PrivateKeyPath string `yaml:"private_key_path"`

	// ── HMAC-SHA256 fields (legacy, deprecated) ──────────────────────

	// SigningKey is the shared HMAC-SHA256 secret for HS256 token verification.
	// DEPRECATED: Use RSA-256 (PublicKeyPEM/PrivateKeyPEM) instead.
	// A shared secret means a compromised worker can forge admin tokens.
	SigningKey []byte `yaml:"-"`

	// SigningKeyBase64 is the base64-encoded HMAC signing key from config/env.
	// DEPRECATED: Use RSA-256 key files instead.
	SigningKeyBase64 string `yaml:"signing_key"`

	// ── Common fields ────────────────────────────────────────────────

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

// Verifier performs stateless JWT verification using either RSA-256 (asymmetric)
// or HMAC-SHA256 (symmetric legacy). It validates:
//   - Algorithm header matches configuration (prevents algorithm confusion attacks)
//   - Signature integrity (token has not been tampered with)
//   - Expiration (exp claim, with configurable clock skew tolerance)
//   - Issuer (iss claim matches expected value)
//   - Audience (aud claim contains expected value)
//   - Required claims (session_id, student_id, org_id, exam_id)
type Verifier struct {
	cfg       VerifierConfig
	algorithm string
	rsaPubKey *rsa.PublicKey  // RS256 verification key (nil for HS256)
	rsaPriKey *rsa.PrivateKey // RS256 signing key (nil for verify-only / HS256)
	hmacKey   []byte          // HS256 signing/verification key (nil for RS256)
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
	if cfg.Algorithm == "" {
		cfg.Algorithm = AlgorithmRS256
	}

	v := &Verifier{
		cfg:       cfg,
		algorithm: cfg.Algorithm,
	}

	switch cfg.Algorithm {
	case AlgorithmRS256:
		if err := v.initRS256(cfg); err != nil {
			return nil, err
		}
	case AlgorithmHS256:
		if err := v.initHS256(cfg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("auth: unsupported algorithm %q (use RS256 or HS256)", cfg.Algorithm)
	}

	return v, nil
}

// initRS256 loads RSA keys for asymmetric verification/signing.
func (v *Verifier) initRS256(cfg VerifierConfig) error {
	// Load public key from file if path is provided.
	if len(cfg.PublicKeyPEM) == 0 && cfg.PublicKeyPath != "" {
		data, err := os.ReadFile(cfg.PublicKeyPath)
		if err != nil {
			return fmt.Errorf("auth: failed to read RSA public key from %q: %w", cfg.PublicKeyPath, err)
		}
		cfg.PublicKeyPEM = data
	}

	// Load private key from file if path is provided.
	if len(cfg.PrivateKeyPEM) == 0 && cfg.PrivateKeyPath != "" {
		data, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return fmt.Errorf("auth: failed to read RSA private key from %q: %w", cfg.PrivateKeyPath, err)
		}
		cfg.PrivateKeyPEM = data
	}

	// Parse public key (required for verification).
	if len(cfg.PublicKeyPEM) > 0 {
		pubKey, err := parseRSAPublicKey(cfg.PublicKeyPEM)
		if err != nil {
			return fmt.Errorf("auth: invalid RSA public key: %w", err)
		}
		v.rsaPubKey = pubKey
	}

	// Parse private key (optional — only needed for signing).
	if len(cfg.PrivateKeyPEM) > 0 {
		priKey, err := parseRSAPrivateKey(cfg.PrivateKeyPEM)
		if err != nil {
			return fmt.Errorf("auth: invalid RSA private key: %w", err)
		}
		v.rsaPriKey = priKey

		// Derive public key from private key if public key not explicitly set.
		if v.rsaPubKey == nil {
			v.rsaPubKey = &priKey.PublicKey
		}
	}

	if v.rsaPubKey == nil {
		return fmt.Errorf("auth: RS256 requires at least a public key (set public_key_path or private_key_path)")
	}

	// Enforce minimum RSA key size (2048 bits).
	if v.rsaPubKey.N.BitLen() < 2048 {
		return fmt.Errorf("auth: RSA key must be at least 2048 bits, got %d", v.rsaPubKey.N.BitLen())
	}

	return nil
}

// initHS256 loads the HMAC key for symmetric verification (legacy).
func (v *Verifier) initHS256(cfg VerifierConfig) error {
	// Decode base64 key if raw key is not set.
	if len(cfg.SigningKey) == 0 && cfg.SigningKeyBase64 != "" {
		cfg.SigningKey = []byte(cfg.SigningKeyBase64)
	}

	if len(cfg.SigningKey) == 0 {
		return fmt.Errorf("auth: HS256 requires a signing key")
	}

	if len(cfg.SigningKey) < 32 {
		return fmt.Errorf("auth: signing key must be at least 32 bytes for HMAC-SHA256 security, got %d", len(cfg.SigningKey))
	}

	v.hmacKey = cfg.SigningKey
	return nil
}

// VerifyToken parses and validates a JWT token string, returning the verified
// proctoring claims. This is the primary entry point for the auth interceptor.
//
// The token is expected in the format: "Bearer <token>" or just "<token>".
//
// Verification steps (in order):
//  1. Strip "Bearer " prefix if present.
//  2. Split into header.payload.signature (3 parts).
//  3. Verify algorithm header matches configuration (prevents confusion attacks).
//  4. Verify signature (RS256 or HS256).
//  5. Decode and parse claims from the payload.
//  6. Check expiration with clock skew tolerance.
//  7. Validate issuer and audience.
//  8. Validate required proctoring claims.
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

	// Verify algorithm from header (prevents algorithm confusion attacks).
	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	headerAlg, err := extractAlgorithm(headerBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if headerAlg != v.algorithm {
		return nil, fmt.Errorf("%w: token uses %q, verifier expects %q",
			ErrInvalidAlgorithm, headerAlg, v.algorithm)
	}

	// Verify signature based on algorithm.
	signingInput := parts[0] + "." + parts[1]
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	switch v.algorithm {
	case AlgorithmRS256:
		if err := v.verifyRS256([]byte(signingInput), actualSig); err != nil {
			return nil, ErrInvalidToken
		}
	case AlgorithmHS256:
		if err := v.verifyHS256([]byte(signingInput), actualSig); err != nil {
			return nil, ErrInvalidToken
		}
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

// verifyRS256 verifies an RSA-SHA256 signature against the public key.
func (v *Verifier) verifyRS256(signingInput, signature []byte) error {
	hash := sha256.Sum256(signingInput)
	return rsa.VerifyPKCS1v15(v.rsaPubKey, crypto.SHA256, hash[:], signature)
}

// verifyHS256 verifies an HMAC-SHA256 signature against the shared key.
func (v *Verifier) verifyHS256(signingInput, signature []byte) error {
	expectedSig := hmacSHA256(signingInput, v.hmacKey)
	if subtle.ConstantTimeCompare(expectedSig, signature) != 1 {
		return fmt.Errorf("hmac mismatch")
	}
	return nil
}

// CanSign returns true if the verifier has a private key (RS256) or
// shared secret (HS256) available for token generation.
func (v *Verifier) CanSign() bool {
	switch v.algorithm {
	case AlgorithmRS256:
		return v.rsaPriKey != nil
	case AlgorithmHS256:
		return len(v.hmacKey) > 0
	}
	return false
}

// Algorithm returns the configured JWT algorithm ("RS256" or "HS256").
func (v *Verifier) Algorithm() string {
	return v.algorithm
}

// ---------------------------------------------------------------------------
// Context helpers — claims propagation through gRPC context
// ---------------------------------------------------------------------------

// Context metadata keys for propagating verified claims to downstream handlers.
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
// metadata. This is the reverse of InjectClaimsMetadata.
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

// ---------------------------------------------------------------------------
// RSA Key Parsing
// ---------------------------------------------------------------------------

// parseRSAPublicKey parses a PEM-encoded RSA public key. It accepts both
// PKCS#1 (RSA PUBLIC KEY) and PKIX (PUBLIC KEY) formats.
func parseRSAPublicKey(pemData []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	// Try PKIX format first (most common for RSA public keys).
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA public key")
		}
		return rsaPub, nil
	}

	// Fall back to PKCS#1 format.
	rsaPub, err2 := x509.ParsePKCS1PublicKey(block.Bytes)
	if err2 != nil {
		return nil, fmt.Errorf("failed to parse RSA public key (tried PKIX and PKCS1): PKIX: %v, PKCS1: %v", err, err2)
	}
	return rsaPub, nil
}

// parseRSAPrivateKey parses a PEM-encoded RSA private key. It accepts both
// PKCS#1 (RSA PRIVATE KEY) and PKCS#8 (PRIVATE KEY) formats.
func parseRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	// Try PKCS#1 format first.
	priKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return priKey, nil
	}

	// Fall back to PKCS#8 format.
	key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err2 != nil {
		return nil, fmt.Errorf("failed to parse RSA private key (tried PKCS1 and PKCS8): PKCS1: %v, PKCS8: %v", err, err2)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}
	return rsaKey, nil
}
