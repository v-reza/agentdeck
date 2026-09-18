package migrate

import (
	"strings"
	"testing"
)

// baselineScript exercises the constructs the real 0001.up.sql uses. A naive
// split on ";" mis-splits every one of them.
const baselineScript = `-- comment with a ; semicolon inside
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- role block: semicolons appear inside the dollar-quoted body
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'agentdeck_app') THEN
        CREATE ROLE agentdeck_app LOGIN;
    END IF;
END
$$;

CREATE TABLE t (
    note text NOT NULL DEFAULT 'a; b'  -- literal semicolon in a default
);

/* block comment; with semicolon */
CREATE TABLE u (
    quote text NOT NULL DEFAULT 'it''s ; ok'
);

CREATE TABLE v (
    body text NOT NULL DEFAULT $tag$ body ; here $tag$
);
`

func TestStatementsHandlesQuotesAndComments(t *testing.T) {
	got := statements(baselineScript)
	if len(got) != 5 {
		t.Fatalf("expected 5 statements, got %d: %#v", len(got), got)
	}

	for _, statement := range got {
		// A statement keeps its trailing semicolon only when the script put
		// one there; the terminator itself is never part of the output.
		if strings.HasSuffix(statement, ";") {
			t.Fatalf("statement kept its terminating semicolon: %#v", statement)
		}
	}

	for needle, index := range map[string]int{
		"CREATE EXTENSION": 0,
		"DO $$":            1,
		"CREATE TABLE t":   2,
		"CREATE TABLE u":   3,
		"CREATE TABLE v":   4,
	} {
		// A leading line comment stays attached to the statement it describes.
		if !strings.Contains(got[index], needle) {
			t.Fatalf("statement %d = %#v, want %q", index, got[index], needle)
		}
	}

	if !strings.Contains(got[1], "END\n$$") {
		t.Fatalf("dollar-quoted body truncated: %#v", got[1])
	}
	if !strings.Contains(got[2], "'a; b'") {
		t.Fatalf("literal semicolon in default lost: %#v", got[2])
	}
	if !strings.Contains(got[3], "it''s ; ok") {
		t.Fatalf("escaped single quote lost: %#v", got[3])
	}
	if !strings.Contains(got[4], "$tag$ body ; here $tag$") {
		t.Fatalf("tagged dollar quote lost: %#v", got[4])
	}
}

func TestStatementsIgnoresSemicolonInComments(t *testing.T) {
	got := statements("-- a ; b\nSELECT 1;")
	if len(got) != 1 || got[0] != "-- a ; b\nSELECT 1" {
		t.Fatalf("expected comment preserved and one statement, got %#v", got)
	}
}

func TestStatementsKeepsTrailingStatementWithoutSemicolon(t *testing.T) {
	got := statements("SELECT 1;\nSELECT 2")
	if len(got) != 2 || got[1] != "SELECT 2" {
		t.Fatalf("trailing statement lost: %#v", got)
	}
}

func TestStatementsSkipsBlankStatements(t *testing.T) {
	got := statements(";;\n\n;\nSELECT 1;")
	if len(got) != 1 || got[0] != "SELECT 1" {
		t.Fatalf("expected only the real statement, got %#v", got)
	}
}

func TestLoadFindsUpMigrationsInOrder(t *testing.T) {
	pending, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 {
		t.Fatal("load found no embedded migrations")
	}
	for index := 1; index < len(pending); index++ {
		if pending[index-1].version >= pending[index].version {
			t.Fatalf("migrations not ascending: %#v", pending)
		}
	}
	if pending[0].version != 1 {
		t.Fatalf("first migration version = %d, want 1", pending[0].version)
	}
}
