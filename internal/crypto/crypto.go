// Package crypto seals LLM provider credentials (US-AD86) with AES-256-GCM.
//
// Wire format stored in agents.provider_api_key_enc (BYTEA), per
// docs/ARCHITECTURE.md §16 and internal/migrate/0004.up.sql:
//
//	nonce (12B) || ciphertext || GCM tag (16B)
//
// The master key comes from the environment (AGENTDECK_MASTER_KEY), never from
// the database. No HTTP, no DB access: callers own persistence.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// KeySize is the AES-256 key length in bytes.
	KeySize = 32
	// nonceSize is the GCM standard nonce length, prefixed to the ciphertext.
	nonceSize = 12
	// tagSize is the GCM authentication tag appended by Seal.
	tagSize = 16
)

var (
	// ErrKeySize reports a master key that is not exactly 32 bytes. The key is
	// never padded, truncated, or repeated to fit.
	ErrKeySize = errors.New("crypto: key must be 32 bytes (AES-256)")
	// ErrTooShort reports a sealed value shorter than nonce + tag.
	ErrTooShort = errors.New("crypto: sealed value too short")
	// ErrOpen reports a failed GCM authentication: wrong key, tampered
	// ciphertext, or tampered nonce. No plaintext is ever returned with it.
	ErrOpen = errors.New("crypto: authentication failed")
)

// Seal encrypts plaintext with AES-256-GCM under key.
// Output layout: nonce (12 byte) || ciphertext || tag (16 byte).
// An empty plaintext is sealed successfully and Open returns ""; callers decide
// whether an empty credential is meaningful (the DDL uses NULL for "unset").
func Seal(key []byte, plaintext string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	// A fresh random nonce per call. Reusing a nonce with the same key breaks
	// GCM completely (it leaks the keystream and forges tags), so this must
	// never become a counter or a constant.
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	// Seal on a copy we own, so the plaintext bytes can be wiped before return.
	// ponytail: the caller's string itself is immutable and cannot be wiped
	// without unsafe; this limits exposure, it does not remove it.
	pt := []byte(plaintext)
	defer zero(pt)
	// Appending to a 12-byte slice with cap 12 allocates a new array, so the
	// result is nonce || ciphertext || tag.
	return gcm.Seal(nonce, nonce, pt, nil), nil
}

// Open decrypts a value produced by Seal. It returns ErrTooShort for truncated
// input and ErrOpen when the GCM tag does not authenticate; it never returns
// unauthenticated plaintext.
func Open(key []byte, sealed []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(sealed) < nonceSize+tagSize {
		return "", ErrTooShort
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	pt, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrOpen, err)
	}
	// Wipe the decrypted bytes once they have been copied into the result
	// string; the credential should not sit in the heap any longer than needed.
	defer zero(pt)
	return string(pt), nil
}

// LoadKey decodes the master key from configuration. Accepted forms:
//
//	64 hex characters                  -> 32 bytes
//	base64 (std, padded or unpadded)   -> 32 bytes
//	exactly 32 raw bytes               -> used as-is
//
// Anything else is rejected. A key of the wrong length is an error, never
// silently padded or truncated: a shortened key is a different (weaker) key.
func LoadKey(raw string) ([]byte, error) {
	if len(raw) == 2*KeySize {
		if key, err := hex.DecodeString(raw); err == nil {
			return key, nil
		}
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		if key, err := enc.DecodeString(raw); err == nil && len(key) == KeySize {
			return key, nil
		}
	}
	if len(raw) == KeySize {
		return []byte(raw), nil
	}
	// Error text carries only the length: never the key material itself.
	return nil, fmt.Errorf("%w (got %d bytes; use 64 hex chars, base64, or 32 raw bytes)", ErrKeySize, len(raw))
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("%w (got %d)", ErrKeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// zero overwrites b in place. The compiler is not guaranteed to elide this, and
// the plaintext/key bytes we allocate here are the only ones we can wipe.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
