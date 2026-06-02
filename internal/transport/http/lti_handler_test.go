package http

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
)

func TestLTIAudienceContainsStringAndArray(t *testing.T) {
	rawString := json.RawMessage(`"client-123"`)
	if !ltiAudienceContains(rawString, "client-123") {
		t.Fatal("string audience was not matched")
	}

	rawArray := json.RawMessage(`["other","client-123"]`)
	if !ltiAudienceContains(rawArray, "client-123") {
		t.Fatal("array audience was not matched")
	}

	if ltiAudienceContains(rawArray, "missing") {
		t.Fatal("unexpected audience match")
	}
}

func TestLTIJWKToRSAPublicKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	exponentBytes := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
	jwk := ltiJWK{
		Kty: "RSA",
		Kid: "lms-key-1",
		N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(exponentBytes),
	}

	publicKey, err := ltiJWKToRSAPublicKey(jwk)
	if err != nil {
		t.Fatalf("convert jwk: %v", err)
	}

	if publicKey.E != privateKey.PublicKey.E {
		t.Fatalf("exponent = %d, want %d", publicKey.E, privateKey.PublicKey.E)
	}
	if publicKey.N.Cmp(privateKey.PublicKey.N) != 0 {
		t.Fatal("modulus mismatch")
	}
}
