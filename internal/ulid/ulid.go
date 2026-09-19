// Package ulid produces Crockford-base32 ULID identifiers.
//
// AgentDeck uses ULID TEXT(26) for every domain entity because ULIDs sort
// lexicographically by time (ARCHITECTURE 3.3 / DECISIONS "Aturan ID"), which
// is what cursor pagination relies on. The schema enforces the length with a
// CHECK constraint, so an ID that is not exactly 26 chars fails at the DB.
package ulid

import (
	"crypto/rand"
	"errors"
	"time"
)

const (
	// Encoding is the Crockford base32 alphabet used by the ULID spec.
	encoding = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	// Len is the encoded length of a ULID: 10 timestamp chars + 16 randomness.
	Len = 26

	tsLen   = 10
	randLen = 16
)

var errOverflow = errors.New("ulid: timestamp overflow")

// Must returns id, panicking only if the system RNG fails. The RNG failure is a
// process-fatal condition: no new user, session, or org can be created without
// fresh randomness, so there is nothing to recover into.
func Must() string {
	id, err := New()
	if err != nil {
		panic(err)
	}
	return id
}

// New returns a fresh ULID encoded as uppercase Crockford base32. It is safe for
// concurrent use; the timestamp is per-call and the randomness comes from
// crypto/rand, so two calls in the same millisecond still differ.
func New() (string, error) {
	now := time.Now().UTC()
	ts := uint64(now.UnixMilli())
	if ts > 0x7FFFFFFFFFF {
		return "", errOverflow
	}

	var b [Len]byte

	// Timestamp: 48 bits packed into the first 10 characters, most significant
	// first, so string sorting matches chronological sorting. UnixMilli fits
	// inside 48 bits until the year 10889, well past the ULID spec horizon.
	for i := 0; i < tsLen; i++ {
		b[i] = encoding[(ts>>uint(45-5*i))&0x1F]
	}

randomness:
	var rnd [randLen]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	for i := 0; i < randLen; i++ {
		b[tsLen+i] = encoding[rnd[i]&0x1F]
	}

	// A zero-randomness ULID would collide for same-millisecond IDs; retry.
	if allZero(rnd[:]) {
		goto randomness
	}

	return string(b[:]), nil
}

// allZero reports whether the entropy segment is empty. crypto/rand basically
// never returns this, but the retry keeps same-millisecond IDs distinct by
// construction instead of by probability.
func allZero(p []byte) bool {
	for _, v := range p {
		if v != 0 {
			return false
		}
	}
	return true
}
