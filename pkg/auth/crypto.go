package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// HMAC-SHA256 — JWT Signature
// ---------------------------------------------------------------------------

// hmacSHA256 computes the HMAC-SHA256 digest of message using the given key.
// This is the core cryptographic operation for JWT verification.
//
// HMAC-SHA256 is chosen over RSA/ECDSA for service-to-service token
// verification because:
//   - 50-100x faster verification (~1µs vs ~50-100µs for RSA-2048)
//   - No public/private key management complexity
//   - Eduser and Event Collector share a single secret via Kubernetes secrets
//
// For public-facing APIs where the signing key cannot be shared, use
// RSA-2048 or ECDSA-P256 and distribute the public key via JWKS.
func hmacSHA256(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

// ---------------------------------------------------------------------------
// Base64URL encoding/decoding
// ---------------------------------------------------------------------------

// base64URLDecode decodes a Base64URL-encoded string (RFC 4648 §5).
// JWT uses Base64URL (no padding) for header, payload, and signature segments.
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if necessary. Base64URL omits trailing '=' characters.
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// base64URLEncode encodes bytes to Base64URL without padding.
func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

// ---------------------------------------------------------------------------
// Claims Parsing
// ---------------------------------------------------------------------------

// jwtClaims is the raw JSON structure of the JWT payload. It uses json.Number
// for numeric fields to avoid float64 precision loss on Unix timestamps.
type jwtClaims struct {
	Sub           string      `json:"sub"`
	Iss           string      `json:"iss"`
	Aud           jsonAud     `json:"aud"`
	Exp           json.Number `json:"exp"`
	Iat           json.Number `json:"iat"`
	JTI           string      `json:"jti,omitempty"`
	SessionID     string      `json:"session_id"`
	StudentID     string      `json:"student_id"`
	OrgID         string      `json:"org_id"`
	ExamID        string      `json:"exam_id"`
	Roles         []string    `json:"roles,omitempty"`
	SessionSecret string      `json:"session_secret,omitempty"`
}

// jsonAud handles the JWT "aud" claim which can be either a single string
// or an array of strings per RFC 7519 §4.1.3.
type jsonAud []string

func (a *jsonAud) UnmarshalJSON(data []byte) error {
	// Try array first.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*a = arr
		return nil
	}
	// Fall back to single string.
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("aud must be a string or array of strings")
	}
	*a = []string{s}
	return nil
}

// parseClaims decodes a raw JWT payload into ProctoringClaims.
func parseClaims(payload []byte) (*ProctoringClaims, error) {
	var raw jwtClaims
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	claims := &ProctoringClaims{
		Subject:       raw.Sub,
		Issuer:        raw.Iss,
		Audience:      raw.Aud,
		JTI:           raw.JTI,
		SessionID:     raw.SessionID,
		StudentID:     raw.StudentID,
		OrgID:         raw.OrgID,
		ExamID:        raw.ExamID,
		Roles:         raw.Roles,
		SessionSecret: raw.SessionSecret,
	}

	// Parse exp.
	if raw.Exp.String() != "" {
		exp, err := raw.Exp.Int64()
		if err != nil {
			return nil, fmt.Errorf("invalid exp claim: %w", err)
		}
		claims.ExpiresAt = time.Unix(exp, 0)
	}

	// Parse iat.
	if raw.Iat.String() != "" {
		iat, err := raw.Iat.Int64()
		if err != nil {
			return nil, fmt.Errorf("invalid iat claim: %w", err)
		}
		claims.IssuedAt = time.Unix(iat, 0)
	}

	return claims, nil
}

// ---------------------------------------------------------------------------
// Token Generation (for testing / internal use)
// ---------------------------------------------------------------------------

// GenerateToken creates a signed JWT token from the given claims.
// This is primarily used for testing and for the session validator to issue
// internal tokens. Production tokens are issued by the Eduser system.
func GenerateToken(claims *ProctoringClaims, signingKey []byte) (string, error) {
	// Header.
	header := base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

	// Payload.
	payload := map[string]interface{}{
		"sub":        claims.Subject,
		"iss":        claims.Issuer,
		"aud":        claims.Audience,
		"exp":        claims.ExpiresAt.Unix(),
		"iat":        claims.IssuedAt.Unix(),
		"session_id": claims.SessionID,
		"student_id": claims.StudentID,
		"org_id":     claims.OrgID,
		"exam_id":    claims.ExamID,
	}
	if claims.JTI != "" {
		payload["jti"] = claims.JTI
	}
	if len(claims.Roles) > 0 {
		payload["roles"] = claims.Roles
	}
	if claims.SessionSecret != "" {
		payload["session_secret"] = claims.SessionSecret
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}
	payloadB64 := base64URLEncode(payloadBytes)

	// Signature.
	signingInput := header + "." + payloadB64
	signature := base64URLEncode(hmacSHA256([]byte(signingInput), signingKey))

	return signingInput + "." + signature, nil
}
