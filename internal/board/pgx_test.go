package board

// Unit coverage for the driver-error mapping in pgx.go. US-AD08 AC2 requires a
// duplicate slug inside one org to answer 409, but the constraint is enforced by
// Postgres (projects_org_slug_key) and pgx surfaces it as a raw *pgconn.PgError.
// Without a mapping the handler's default branch turns a *user* mistake into a
// 500. These tests pin the mapping as a pure function, so they run without a
// database; the live path is exercised by the API-level check in
// cmd/api/boards_rbac_test.go and by the Postgres-backed suite.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestUniqueViolationMatchesOnlyTheNamedConstraint(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		constraint string
		want       bool
	}{
		{
			name:       "unique violation on the queried constraint",
			err:        &pgconn.PgError{Code: "23505", ConstraintName: "projects_org_slug_key"},
			constraint: "projects_org_slug_key",
			want:       true,
		},
		{
			// A slug collision and a board-name collision are different facts:
			// mapping the wrong one would report "slug is already taken" for a
			// constraint the caller never touched.
			name:       "unique violation on another constraint",
			err:        &pgconn.PgError{Code: "23505", ConstraintName: "boards_project_slug_key"},
			constraint: "projects_org_slug_key",
			want:       false,
		},
		{
			name:       "foreign key violation is not a unique violation",
			err:        &pgconn.PgError{Code: "23503", ConstraintName: "projects_org_slug_key"},
			constraint: "projects_org_slug_key",
			want:       false,
		},
		{
			name:       "wrapped pg error is still detected",
			err:        fmt.Errorf("create project: %w", &pgconn.PgError{Code: "23505", ConstraintName: "projects_org_slug_key"}),
			constraint: "projects_org_slug_key",
			want:       true,
		},
		{
			name:       "a plain error is not a unique violation",
			err:        errors.New("connection reset"),
			constraint: "projects_org_slug_key",
			want:       false,
		},
		{name: "nil error", err: nil, constraint: "projects_org_slug_key", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uniqueViolation(tc.err, tc.constraint); got != tc.want {
				t.Fatalf("uniqueViolation(%v, %q) = %v, want %v", tc.err, tc.constraint, got, tc.want)
			}
		})
	}
}

// TestProjectSlugTakenErrorIsTheDomainSentinel is the AC2 contract at the
// repository boundary: a unique violation on projects_org_slug_key must arrive
// at the HTTP layer as ErrSlugTaken, which writeBoardError maps to 409.
func TestProjectSlugTakenErrorIsTheDomainSentinel(t *testing.T) {
	mapped := slugTakenError(&pgconn.PgError{Code: "23505", ConstraintName: "projects_org_slug_key"})
	if !errors.Is(mapped, ErrSlugTaken) {
		t.Fatalf("duplicate project slug mapped to %v, want ErrSlugTaken", mapped)
	}

	other := errors.New("deadlock detected")
	if got := slugTakenError(other); !errors.Is(got, other) {
		t.Fatalf("unrelated error was rewritten to %v, want the original error", got)
	}
	if got := slugTakenError(nil); got != nil {
		t.Fatalf("slugTakenError(nil) = %v, want nil", got)
	}
}

// TestBoardSlugTakenErrorIsTheDomainSentinel covers the same constraint class on
// boards (US-AD09 AC2): the identical 500 would otherwise appear one story later.
func TestBoardSlugTakenErrorIsTheDomainSentinel(t *testing.T) {
	mapped := boardSlugTakenError(&pgconn.PgError{Code: "23505", ConstraintName: "boards_project_slug_key"})
	if !errors.Is(mapped, ErrSlugTaken) {
		t.Fatalf("duplicate board slug mapped to %v, want ErrSlugTaken", mapped)
	}

	other := errors.New("connection reset")
	if got := boardSlugTakenError(other); !errors.Is(got, other) {
		t.Fatalf("unrelated error was rewritten to %v, want the original error", got)
	}
}
