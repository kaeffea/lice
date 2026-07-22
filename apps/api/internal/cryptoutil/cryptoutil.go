package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
)

const tokenBytes = 32

var ErrInvalidCiphertext = errors.New("invalid ciphertext")

func RandomToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func Digest(value string) [sha256.Size]byte {
	return sha256.Sum256([]byte(value))
}

type Digester struct {
	key []byte
}

func NewDigester(key []byte) (*Digester, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("session hash key must contain 32 bytes, got %d", len(key))
	}
	return &Digester{key: append([]byte(nil), key...)}, nil
}

func (d *Digester) Digest(value string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, d.key)
	_, _ = mac.Write([]byte("lice-session-v1\x00"))
	_, _ = mac.Write([]byte(value))
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func EqualDigest(left, right []byte) bool {
	if len(left) != sha256.Size || len(right) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(left, right) == 1
}

type Cipher struct {
	keyID string
	aead  cipher.AEAD
}

func NewCipher(keyID string, key []byte) (*Cipher, error) {
	if keyID == "" {
		return nil, errors.New("crypto key id is required")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto key must contain 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &Cipher{keyID: keyID, aead: aead}, nil
}

func (c *Cipher) KeyID() string { return c.keyID }

func (c *Cipher) Seal(plaintext, additionalData []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, additionalData), nil
}

func (c *Cipher) Open(ciphertext, additionalData []byte) ([]byte, error) {
	if len(ciphertext) < c.aead.NonceSize()+c.aead.Overhead() {
		return nil, ErrInvalidCiphertext
	}
	nonce := ciphertext[:c.aead.NonceSize()]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext[c.aead.NonceSize():], additionalData)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

type CSRF struct {
	key []byte
}

func NewCSRF(key []byte) (*CSRF, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("csrf key must contain 32 bytes, got %d", len(key))
	}
	return &CSRF{key: append([]byte(nil), key...)}, nil
}

func (c *CSRF) Token(sessionID string) string {
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte("lice-csrf-v1\x00"))
	_, _ = mac.Write([]byte(sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (c *CSRF) Valid(sessionID, presented string) bool {
	expected := c.Token(sessionID)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}
