package cryptoutil

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestRandomTokenIsOpaqueAndUnique(t *testing.T) {
	first, err := RandomToken()
	if err != nil {
		t.Fatalf("RandomToken() error = %v", err)
	}
	second, err := RandomToken()
	if err != nil {
		t.Fatalf("RandomToken() error = %v", err)
	}
	if first == second {
		t.Fatal("two random tokens were equal")
	}
	raw, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("token is not unpadded base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("decoded token length = %d, want 32", len(raw))
	}
}

func TestDigesterIsKeyedAndDomainSeparated(t *testing.T) {
	first, err := NewDigester(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDigester(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	firstDigest := first.Digest("opaque-token")
	repeated := first.Digest("opaque-token")
	secondDigest := second.Digest("opaque-token")
	plainDigest := Digest("opaque-token")
	if !EqualDigest(firstDigest[:], repeated[:]) {
		t.Fatal("same key and value did not produce the same digest")
	}
	if EqualDigest(firstDigest[:], secondDigest[:]) {
		t.Fatal("different keys produced the same digest")
	}
	if EqualDigest(firstDigest[:], plainDigest[:]) {
		t.Fatal("session digest matched an unkeyed SHA-256 digest")
	}
}

func TestCipherAuthenticatesCiphertextAndAdditionalData(t *testing.T) {
	cipher, err := NewCipher("test-v1", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	additionalData := []byte("transaction-id")
	sealed, err := cipher.Seal([]byte("pkce-verifier"), additionalData)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := cipher.Open(sealed, additionalData)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if string(opened) != "pkce-verifier" {
		t.Fatalf("Open() = %q", opened)
	}
	if _, err := cipher.Open(sealed, []byte("other-transaction")); err != ErrInvalidCiphertext {
		t.Fatalf("Open() with wrong additional data error = %v, want %v", err, ErrInvalidCiphertext)
	}
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 1
	if _, err := cipher.Open(tampered, additionalData); err != ErrInvalidCiphertext {
		t.Fatalf("Open() with tampered ciphertext error = %v, want %v", err, ErrInvalidCiphertext)
	}
}

func TestCSRFTokenIsBoundToSession(t *testing.T) {
	csrf, err := NewCSRF(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	token := csrf.Token("session-a")
	if !csrf.Valid("session-a", token) {
		t.Fatal("valid CSRF token was rejected")
	}
	if csrf.Valid("session-b", token) {
		t.Fatal("CSRF token was accepted for another session")
	}
	if csrf.Valid("session-a", token+"x") {
		t.Fatal("tampered CSRF token was accepted")
	}
}

func TestKeyConstructorsRejectWrongLength(t *testing.T) {
	if _, err := NewDigester(make([]byte, 31)); err == nil {
		t.Fatal("NewDigester accepted a 31-byte key")
	}
	if _, err := NewCipher("v1", make([]byte, 33)); err == nil {
		t.Fatal("NewCipher accepted a 33-byte key")
	}
	if _, err := NewCSRF(nil); err == nil {
		t.Fatal("NewCSRF accepted an empty key")
	}
}
