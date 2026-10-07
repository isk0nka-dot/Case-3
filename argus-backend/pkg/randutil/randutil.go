// Package randutil provides centralized cryptographically-secure random ID
// and token generation. All random identifiers in the codebase should use
// this package to avoid duplicating crypto/rand + hex patterns.
package randutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// PrefixedID generates a random hex ID with the given prefix.
// byteLen controls the entropy — the resulting hex string is 2*byteLen characters.
//
// Examples:
//
//	PrefixedID("cns-", 16) → "cns-a1b2c3d4e5f6..."  (32 hex chars after prefix)
//	PrefixedID("apl-", 16) → "apl-f7e8d9c0b1a2..."
func PrefixedID(prefix string, byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("randutil: crypto/rand failed: %v", err))
	}
	return prefix + hex.EncodeToString(b)
}

// HexToken generates a cryptographically random hex string.
// byteLen controls the entropy — the resulting string is 2*byteLen characters.
//
// Examples:
//
//	HexToken(16) → "a1b2c3d4e5f67890..."  (32 hex chars)
//	HexToken(32) → "..."                   (64 hex chars)
func HexToken(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("randutil: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}
