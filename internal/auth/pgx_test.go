package auth

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgErr builds a synthetic pgconn.PgError so the mapping helpers can be
// exercised without a live database. The production path only ever sees these
// shapes come out of pgx, so a synthesized one is the same input the helper
// gets at runtime.
func pgErr(code string) *pgconn.PgError {
	return &pgconn.PgError{Code: code, Message: "synthetic", ConstraintName: "synthetic_key"}
}

// TestMapPgErrorContext covers the F3 fix: the sentinel a read returns is the
// one that matches what was queried. A missing row is not always a missing
// user — a missing org is a 404 against the workspace, and a missing session
// is a 401, never a user-shaped 404.
func TestMapPgErrorContext(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		got     error
		mapping func(error) error
		want    error
	}{
		{"missing row is a user when the user was read", pgx.ErrNoRows, mapPgError, ErrUserNotFound},
		{"missing row is a workspace when the org was read", pgx.ErrNoRows, func(e error) error {
			return mapNotFound(e, ErrWorkspaceNotFound)
		}, ErrWorkspaceNotFound},
		{"missing row is a dead session when the token was read", pgx.ErrNoRows, func(e error) error {
			return mapNotFound(e, ErrSessionNotFound)
		}, ErrSessionNotFound},
		{"email unique violation is a 409 email conflict", pgErr("23505"), mapPgError, ErrEmailExists},
		{"org slug unique violation is a 409 slug conflict", pgErr("23505"), mapSlugConflict, ErrSlugTaken},
		{"membership foreign key is a missing member", pgErr("23503"), mapPgError, ErrMemberNotFound},
		{"an unrelated failure passes through untouched", boom, func(e error) error { return mapPgError(e) }, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mapping(tt.got)
			if !errors.Is(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMapPgErrorPassesThroughUnknownCodes guards the F3 change from swallowing
// a new failure class into a not-found: anything pgx raises that is not a
// known no-rows or constraint case must reach the handler as itself so it
// surfaces as a 500 and not a silent 404.
func TestMapPgErrorPassesThroughUnknownCodes(t *testing.T) {
	unknown := pgErr("42P01") // undefined_table
	if got := mapPgError(unknown); !errors.Is(got, unknown) {
		t.Fatalf("undefined_table swallowed into %v", got)
	}
	if got := mapNotFound(unknown, ErrWorkspaceNotFound); !errors.Is(got, unknown) {
		t.Fatalf("undefined_table swallowed into %v", got)
	}
	// A slug insert hitting a hard failure must not become ErrSlugTaken either.
	if got := mapSlugConflict(unknown); !errors.Is(got, unknown) {
		t.Fatalf("undefined_table swallowed into %v", got)
	}
}

// TestNilErrorsMapToNil keeps the nil contract explicit: every helper is safe
// to call on a successful read, so callers do not need to nil-check first.
func TestNilErrorsMapToNil(t *testing.T) {
	if got := mapPgError(nil); got != nil {
		t.Fatalf("mapPgError(nil) = %v", got)
	}
	if got := mapNotFound(nil, ErrWorkspaceNotFound); got != nil {
		t.Fatalf("mapNotFound(nil) = %v", got)
	}
	if got := mapSlugConflict(nil); got != nil {
		t.Fatalf("mapSlugConflict(nil) = %v", got)
	}
	if got := memberNotFound(nil); got != nil {
		t.Fatalf("memberNotFound(nil) = %v", got)
	}
	if got := domainNotFound(nil, ErrUserNotFound); got != nil {
		t.Fatalf("domainNotFound(nil) = %v", got)
	}
}

// TestShadowSentinelNeverPanics is the F2 gate at the mapping layer: the
// shadow row's sentinel hash has an empty salt, and hex.DecodeString turns
// that into a zero-length slice — argon2.IDKey panics on one instead of
// returning false. verifyPassword must reject it before the compare runs, so
// a pending invitation can never take the process down.
func TestShadowSentinelNeverPanics(t *testing.T) {
	if verifyPassword(shadowPasswordHash, "password1") {
		t.Fatal("shadow sentinel verified against a password")
	}
	if verifyPassword(shadowPasswordHash, "") {
		t.Fatal("shadow sentinel verified against the empty password")
	}
	if verifyPassword("$argon2id$v=19$m=65536,t=1,p=4$$", "password1") {
		t.Fatal("empty salt verified against a password")
	}
	if verifyPassword("$argon2id$v=19$m=65536,t=1,p=4$deadbeef$", "") {
		t.Fatal("hash with no expected key verified against a password")
	}
}
