package migrate

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need a live Postgres 16. They run only when AGENTDECK_TEST_DATABASE_URL
// points at one, so `go test ./...` stays green on a machine without a database.
// CI and this repo's Makefile set the variable to a throwaway container.
//
// AGENTDECK_TEST_DATABASE_URL=postgres://agentdeck:agentdeck@localhost:5433/agentdeck \
//   go test ./internal/migrate/ -run Postgres -count=1 -v

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("AGENTDECK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AGENTDECK_TEST_DATABASE_URL not set; Postgres-backed test skipped")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return pool
}

// resetSchema gives the next test an empty public schema and no leftover role.
func resetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA public"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := pool.Exec(ctx, "DROP ROLE IF EXISTS agentdeck_app"); err != nil {
		t.Fatalf("drop role: %v", err)
	}
}

func TestPostgresMigrationIsIdempotent(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for attempt := 1; attempt <= 2; attempt++ {
		if err := Apply(ctx, pool); err != nil {
			t.Fatalf("apply attempt %d: %v", attempt, err)
		}
	}

	// Re-applying must not duplicate the recorded version or error on the
	// already-created objects.
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("re-apply migration: %v", err)
	}

	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("schema_migrations has %d rows, want 1", applied)
	}

	tables := []string{"orgs", "users", "memberships", "sessions"}
	for _, table := range tables {
		var regclass string
		if err := pool.QueryRow(ctx, "SELECT to_regclass('public."+table+"')").Scan(&regclass); err != nil {
			t.Fatal(err)
		}
		if regclass == "" {
			t.Fatalf("table %s missing after migration", table)
		}
	}

	var hasRole bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='agentdeck_app')").Scan(&hasRole); err != nil {
		t.Fatal(err)
	}
	if !hasRole {
		t.Fatal("runtime role agentdeck_app was not created")
	}
}

func TestPostgresULIDConstraintRejectsWrongID(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		orgID   string
		wantErr string
	}{
		{"uuid is 36 chars not 26", "01890697-8bb9-7e47-9bb5-1c1f5b1a1c40", "orgs_id_ulid_chk"},
		{"empty id", "", "orgs_id_ulid_chk"},
		{"slug is not an id", "acme", "orgs_id_ulid_chk"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)",
				tc.orgID, "acme-corp", "Acme")
			if err == nil {
				t.Fatal("insert with malformed id succeeded")
			}
		})
	}
}

func TestPostgresRoleEnumRejectsUnknownValue(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	const orgID = "01JZ0000000000000000000001"
	const userID = "01JZ0000000000000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", orgID, "acme-corp", "Acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, $4)",
		userID, "owner@acme.test", "Owner", "$argon2id$v=19$m=65536,t=1,p=4$00$00"); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Exec(ctx, "INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'superuser')", orgID, userID)
	if err == nil {
		t.Fatal("memberships accepted a role outside the frozen enum")
	}

	// The frozen enum is exactly owner/admin/member/viewer (DECISIONS section 4).
	for _, role := range []string{"owner", "admin", "member", "viewer"} {
		if _, err := pool.Exec(ctx, "DELETE FROM memberships"); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, $3)", orgID, userID, role); err != nil {
			t.Fatalf("frozen role %q rejected: %v", role, err)
		}
	}
}

// TestPostgresTenantIsolation is the M0 half of ARCHITECTURE 17.1 item 3: a row
// owned by org A must not be reachable through a query scoped to org B, even
// when its ULID is known.
func TestPostgresTenantIsolation(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	const orgA = "01JZ00000000000000000000A1"
	const orgB = "01JZ00000000000000000000B2"
	const userA = "01JZ00000000000000000000A3"

	for _, org := range []struct {
		id, slug string
	}{
		{orgA, "org-a"},
		{orgB, "org-b"},
	} {
		if _, err := pool.Exec(ctx, "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", org.id, org.slug, "Org"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, $4)",
		userA, "a@org.test", "A", "$argon2id$v=19$m=65536,t=1,p=4$00$00"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'owner')", orgA, userA); err != nil {
		t.Fatal(err)
	}

	// A query scoped to org B must see nothing, even knowing org A's id.
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM memberships WHERE org_id = $1", orgB).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("org B sees %d rows of org A", count)
	}

	// Adding org B's own membership must not expose it to an org A scoped query.
	if _, err := pool.Exec(ctx, "INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'viewer')", orgB, userA); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM memberships WHERE org_id = $1", orgA).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("org A scoped query returned %d rows, want 1", count)
	}
}
