package board

// Unit coverage for the two driver-error mappings US-AD09 needs beyond the slug
// one. Both were live defects found by probing the running API:
//
//  1. AC2 names the *name* ("Nama board duplikat dalam satu project mengembalikan
//     409"), but the DDL made only the slug unique, so a second board called
//     "Sprint 24" with a different slug returned 201. The name index arrives in
//     migration 0007; this pins the mapping from its violation to a 409.
//  2. A project id that does not exist in the caller's org reached Postgres as a
//     foreign-key violation and came back as `500 ERROR: insert or update on
//     table "boards" violates foreign key constraint`. A caller naming a missing
//     parent is a 404, and leaking the raw SQLSTATE is an information leak on top
//     of the wrong status.
//
// These run without a database: the mapping functions are pure, so the failure
// mode they guard (a user mistake reported as a server fault) is testable in
// isolation. The live path is covered by postgres_test.go.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestBoardNameTakenErrorIsTheDomainSentinel is AC2 at the repository boundary:
// a unique violation on boards_project_name_key must arrive at the HTTP layer as
// ErrNameTaken, which writeBoardError answers as 409.
func TestBoardNameTakenErrorIsTheDomainSentinel(t *testing.T) {
	mapped := boardNameTakenError(&pgconn.PgError{Code: "23505", ConstraintName: "boards_project_name_key"})
	if !errors.Is(mapped, ErrNameTaken) {
		t.Fatalf("duplicate board name mapped to %v, want ErrNameTaken", mapped)
	}

	// The slug index must not be reported as a name conflict: they are different
	// facts and the operator fixes them differently.
	slug := boardNameTakenError(&pgconn.PgError{Code: "23505", ConstraintName: "boards_project_slug_key"})
	if errors.Is(slug, ErrNameTaken) {
		t.Fatal("a slug collision was reported as a name collision")
	}

	other := errors.New("connection reset")
	if got := boardNameTakenError(other); !errors.Is(got, other) {
		t.Fatalf("unrelated error was rewritten to %v, want the original error", got)
	}
	if got := boardNameTakenError(nil); got != nil {
		t.Fatalf("boardNameTakenError(nil) = %v, want nil", got)
	}
}

// TestMissingParentProjectIsNotFound is US-AD09 AC4's failure path: creating a
// board under a project that does not exist in the caller's org is a 404, not a
// 500 carrying the raw foreign-key text.
func TestMissingParentProjectIsNotFound(t *testing.T) {
	fk := &pgconn.PgError{Code: "23503", ConstraintName: "boards_project_fk"}
	if got := missingParentError(fk); !errors.Is(got, ErrNotFound) {
		t.Fatalf("foreign-key violation on boards_project_fk mapped to %v, want ErrNotFound", got)
	}

	// A different foreign key is a different bug; it must not be silently
	// reported to the caller as "not found".
	other := &pgconn.PgError{Code: "23503", ConstraintName: "tasks_board_fk"}
	if got := missingParentError(other); errors.Is(got, ErrNotFound) {
		t.Fatal("an unrelated foreign key was reported as a missing project")
	}

	plain := errors.New("context deadline exceeded")
	if got := missingParentError(plain); !errors.Is(got, plain) {
		t.Fatalf("unrelated error was rewritten to %v, want the original error", got)
	}
	if got := missingParentError(nil); got != nil {
		t.Fatalf("missingParentError(nil) = %v, want nil", got)
	}
}

// TestNoRowsBecomesNotFound pins the read path. `SELECT ... WHERE id = $1 AND
// org_id = $2` returns no rows both for a genuinely absent id and for an id that
// belongs to another tenant — the two must be indistinguishable to the caller
// (US-AD07), and both are 404. pgx reports that as pgx.ErrNoRows, which the
// handler's default branch would otherwise answer as 500.
func TestNoRowsBecomesNotFound(t *testing.T) {
	if got := noRowsError(pgxErrNoRows()); !errors.Is(got, ErrNotFound) {
		t.Fatalf("no rows mapped to %v, want ErrNotFound", got)
	}

	other := errors.New("connection refused")
	if got := noRowsError(other); !errors.Is(got, other) {
		t.Fatalf("unrelated error was rewritten to %v, want the original error", got)
	}
	if got := noRowsError(nil); got != nil {
		t.Fatalf("noRowsError(nil) = %v, want nil", got)
	}
}

// TestWrappedNoRowsIsStillNotFound covers the shape pgx actually returns from a
// query helper: the sentinel is wrapped, so a bare == comparison would miss it.
func TestWrappedNoRowsIsStillNotFound(t *testing.T) {
	wrapped := wrappedNoRows()
	if got := noRowsError(wrapped); !errors.Is(got, ErrNotFound) {
		t.Fatalf("wrapped no-rows error mapped to %v, want ErrNotFound", got)
	}
}
