package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return key
}

func TestSealOpenRoundTrip(t *testing.T) {
	key := testKey(t)
	plaintexts := map[string]string{
		"typical openai key": "sk-proj-abc123DEF456ghi789",
		"empty":              "",
		"single byte":        "x",
		"unicode":            "kunci-rahasia-ünïcödé-🔐-日本語",
		"control chars":      "a\x00b\n\r\t\x1bc",
		"quote and slash":    `{"k":"v\"}\\//`,
		"long":               strings.Repeat("abcdefghij", 20000), // 200 KB
	}
	for name, plaintext := range plaintexts {
		t.Run(name, func(t *testing.T) {
			sealed, err := Seal(key, plaintext)
			if err != nil {
				t.Fatalf("Seal: %v", err)
			}
			if want := nonceSize + len(plaintext) + tagSize; len(sealed) != want {
				t.Fatalf("sealed length = %d, want %d (nonce+ct+tag)", len(sealed), want)
			}
			got, err := Open(key, sealed)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if got != plaintext {
				t.Fatalf("round trip mismatch: got %q", got)
			}
		})
	}
}

// Nonce reuse under a fixed key destroys GCM. Two seals of the same plaintext
// must differ, and differ in the nonce prefix specifically.
func TestSealUsesFreshNonce(t *testing.T) {
	key := testKey(t)
	const plaintext = "sk-same-plaintext-both-times"
	seen := make(map[string]int)
	const rounds = 200
	for i := 0; i < rounds; i++ {
		sealed, err := Seal(key, plaintext)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		nonce := string(sealed[:nonceSize])
		if prev, dup := seen[nonce]; dup {
			t.Fatalf("nonce reused at iteration %d (first seen at %d): %x", i, prev, nonce)
		}
		seen[nonce] = i
	}
	if len(seen) != rounds {
		t.Fatalf("distinct nonces = %d, want %d", len(seen), rounds)
	}

	a, err := Seal(key, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	b, err := Seal(key, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same plaintext produced identical ciphertext")
	}
	if bytes.Equal(a[:nonceSize], b[:nonceSize]) {
		t.Fatal("two seals reused the same nonce")
	}
	// The ciphertext body must also differ (same plaintext, different keystream).
	if bytes.Equal(a[nonceSize:], b[nonceSize:]) {
		t.Fatal("ciphertext body identical across seals; nonce is not feeding GCM")
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	key := testKey(t)
	sealed, err := Seal(key, "sk-secret-value")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	for _, idx := range []int{nonceSize, nonceSize + 1, len(sealed) - 1} {
		tampered := bytes.Clone(sealed)
		tampered[idx] ^= 0x01
		got, err := Open(key, tampered)
		if err == nil {
			t.Fatalf("Open accepted tampered byte at %d, returned %q", idx, got)
		}
		if !errors.Is(err, ErrOpen) {
			t.Fatalf("byte %d: err = %v, want ErrOpen", idx, err)
		}
		if got != "" {
			t.Fatalf("byte %d: Open returned plaintext %q alongside error", idx, got)
		}
	}
}

func TestOpenRejectsTamperedNonce(t *testing.T) {
	key := testKey(t)
	sealed, err := Seal(key, "sk-secret-value")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	for _, idx := range []int{0, nonceSize - 1} {
		tampered := bytes.Clone(sealed)
		tampered[idx] ^= 0x80
		if got, err := Open(key, tampered); err == nil {
			t.Fatalf("Open accepted tampered nonce byte at %d, returned %q", idx, got)
		}
	}
}

// A wrong-but-valid-length key must fail authentication, not panic.
func TestOpenWithDifferentKey(t *testing.T) {
	sealed, err := Seal(testKey(t), "sk-secret-value")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	for i := 0; i < 8; i++ {
		other := testKey(t)
		got, err := Open(other, sealed)
		if err == nil {
			t.Fatalf("Open with a different key succeeded, returned %q", got)
		}
		if !errors.Is(err, ErrOpen) {
			t.Fatalf("err = %v, want ErrOpen", err)
		}
	}
}

func TestOpenRejectsShortInput(t *testing.T) {
	key := testKey(t)
	cases := map[string][]byte{
		"nil":                  nil,
		"empty":                {},
		"one byte":             {0x01},
		"nonce only":           make([]byte, nonceSize),
		"nonce+tag-1":          make([]byte, nonceSize+tagSize-1),
		"junk of wrong length": []byte("too short"),
	}
	for name, sealed := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Open(key, sealed)
			if err == nil {
				t.Fatalf("Open accepted short input, returned %q", got)
			}
			if !errors.Is(err, ErrTooShort) {
				t.Fatalf("err = %v, want ErrTooShort", err)
			}
		})
	}
}

// Exactly nonce+tag with no plaintext is a valid sealed empty string.
func TestOpenAcceptsEmptySealedValue(t *testing.T) {
	key := testKey(t)
	sealed, err := Seal(key, "")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if len(sealed) != nonceSize+tagSize {
		t.Fatalf("sealed empty length = %d, want %d", len(sealed), nonceSize+tagSize)
	}
	got, err := Open(key, sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestSealAndOpenRejectBadKeyLength(t *testing.T) {
	for _, n := range []int{0, 1, 16, 24, 31, 33, 64} {
		key := make([]byte, n)
		if _, err := Seal(key, "x"); !errors.Is(err, ErrKeySize) {
			t.Fatalf("Seal with %d-byte key: err = %v, want ErrKeySize", n, err)
		}
		if _, err := Open(key, make([]byte, nonceSize+tagSize)); !errors.Is(err, ErrKeySize) {
			t.Fatalf("Open with %d-byte key: err = %v, want ErrKeySize", n, err)
		}
	}
}

func TestLoadKeyAcceptedFormats(t *testing.T) {
	raw := make([]byte, KeySize)
	for i := range raw {
		raw[i] = byte(i)
	}
	cases := map[string]string{
		"hex lowercase":      hex.EncodeToString(raw),
		"hex uppercase":      strings.ToUpper(hex.EncodeToString(raw)),
		"base64 std padded":  base64.StdEncoding.EncodeToString(raw),
		"base64 std raw":     base64.RawStdEncoding.EncodeToString(raw),
		"32 raw ascii bytes": strings.Repeat("k", KeySize),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			key, err := LoadKey(encoded)
			if err != nil {
				t.Fatalf("LoadKey(%q): %v", encoded, err)
			}
			if len(key) != KeySize {
				t.Fatalf("key length = %d, want %d", len(key), KeySize)
			}
			sealed, err := Seal(key, "round-trip")
			if err != nil {
				t.Fatalf("Seal: %v", err)
			}
			if got, err := Open(key, sealed); err != nil || got != "round-trip" {
				t.Fatalf("Open = (%q, %v)", got, err)
			}
		})
	}

	// hex and base64 of the same bytes must decode to the same key.
	hexKey, err := LoadKey(hex.EncodeToString(raw))
	if err != nil {
		t.Fatalf("LoadKey hex: %v", err)
	}
	b64Key, err := LoadKey(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("LoadKey base64: %v", err)
	}
	if !bytes.Equal(hexKey, b64Key) {
		t.Fatal("hex and base64 forms of the same key decoded differently")
	}
}

func TestLoadKeyRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"empty":              "",
		"too short raw":      "short",
		"31 bytes":           strings.Repeat("k", 31),
		"33 bytes":           strings.Repeat("k", 33),
		"63 hex chars":       strings.Repeat("a", 63),
		"65 hex chars":       strings.Repeat("a", 65),
		"64 non-hex chars":   strings.Repeat("z", 64),
		"base64 of 16 bytes": base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"base64 of 31 bytes": base64.StdEncoding.EncodeToString(make([]byte, 31)),
		"64 chars with pad":  strings.Repeat("a", 62) + "==",
		"whitespace padded":  " " + strings.Repeat("k", KeySize),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			key, err := LoadKey(raw)
			if err == nil {
				t.Fatalf("LoadKey(%q) accepted input, returned %d bytes", raw, len(key))
			}
			if !errors.Is(err, ErrKeySize) {
				t.Fatalf("err = %v, want ErrKeySize", err)
			}
		})
	}
}

// Errors must never carry the key or the plaintext.
func TestErrorsDoNotLeakSecrets(t *testing.T) {
	const secret = "sk-live-DO-NOT-LEAK-0123456789"
	key := testKey(t)
	sealed, err := Seal(key, secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	wrong := testKey(t)
	_, openErr := Open(wrong, sealed)
	if openErr == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(openErr.Error(), secret) {
		t.Fatalf("error leaked plaintext: %v", openErr)
	}
	if strings.Contains(openErr.Error(), string(wrong)) {
		t.Fatalf("error leaked key: %v", openErr)
	}

	_, shortErr := Open(wrong, []byte("abc"))
	if shortErr == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(shortErr.Error(), "abc") {
		t.Fatalf("error leaked input: %v", shortErr)
	}

	// 64 chars that are neither valid hex nor 32 bytes of base64.
	rawKey := strings.Repeat("z", 64)
	_, keyErr := LoadKey(rawKey)
	if keyErr == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(keyErr.Error(), rawKey) {
		t.Fatalf("error leaked raw key: %v", keyErr)
	}
}

// Seal must not mutate or retain the caller's key slice.
func TestSealDoesNotMutateKey(t *testing.T) {
	key := testKey(t)
	before := bytes.Clone(key)
	if _, err := Seal(key, "sk-secret-value"); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if !bytes.Equal(key, before) {
		t.Fatal("Seal mutated the key")
	}
}

// Ciphertext is never the plaintext (obvious, but it pins "encryption happened").
func TestCiphertextIsNotPlaintext(t *testing.T) {
	key := testKey(t)
	const plaintext = "sk-visible-in-clear-if-broken"
	sealed, err := Seal(key, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, []byte(plaintext)) {
		t.Fatal("plaintext appears verbatim inside the sealed value")
	}
}
