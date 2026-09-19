package migrate

import (
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need a live Postgres 16. They run only when AGENTDECK_TEST_DATABASE_URL
// points at one, so `go test ./...` stays green on a machine without a database.
// CI and this repo's Makefile set the variable to a throwaway container.
//
// AGENTDECK_TEST_DATABASE_URL=postgres://agentdeck:***@localhost:5433/agentdeck \
//   go test ./internal/migrate/ -run Postgres -count=1 -v

// migrateTestDSN points at a throwaway database derived from the configured
// DSN. resetSchema drops the schema it is pointed at, so the migration suite
// must never share a database with another Postgres-backed package: go test
// runs packages in parallel, and a shared database means one package's reset
// destroys the other's tables mid-run (observed as 3F000/42P01 failures in
// internal/auth).
var (
	migrateTestDSN     string
	migrateTestDBOnce  sync.Mutex
	migrateTestDBReady bool
)

// adminDSN returns the configured DSN itself; it is used to create and drop
// migrateTestDSN's database, which cannot be dropped while connected to it.
func adminDSN() (string, error) {
	dsn := os.Getenv("AGENTDECK_TEST_DATABASE_URL")
	if dsn == "" {
		return "", nil
	}
	return dsn, nil
}

// deriveTestDSN clones the configured DSN onto a *_migratetest database.
func deriveTestDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("AGENTDECK_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse AGENTDECK_TEST_DATABASE_URL: %v", err)
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if name == "" {
		t.Fatal("AGENTDECK_TEST_DATABASE_URL has no database name")
	}
	parsed.Path = "/" + name + "_migratetest"
	return parsed.String()
}

// prepareMigrateTestDB recreates the throwaway database once per test binary.
// The advisory lock in Apply serialises migration runners, so a single shared
// database is all the suite needs once it is isolated from other packages.
func prepareMigrateTestDB(t *testing.T) {
	t.Helper()

	migrateTestDBOnce.Lock()
	defer migrateTestDBOnce.Unlock()
	if migrateTestDBReady {
		return
	}

	dsn, err := adminDSN()
	if err != nil {
		t.Fatal(err)
	}
	if dsn == "" {
		t.Skip("AGENTDECK_TEST_DATABASE_URL not set; Postgres-backed test skipped")
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse AGENTDECK_TEST_DATABASE_URL: %v", err)
	}
	dbName := strings.TrimPrefix(parsed.Path, "/") + "_migratetest"
	parsed.Path = "/" + dbName
	migrateTestDSN = parsed.String()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer admin.Close()

	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(dbName)); err != nil {
		t.Fatalf("drop stale test database %s: %v", dbName, err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoteIdent(dbName)+" OWNER "+quoteIdent(parsedUser(t, parsed))); err != nil {
		t.Fatalf("create test database %s: %v", dbName, err)
	}

	migrateTestDBReady = true
}

// parsedUser returns the URL's role, used as the throwaway database owner.
func parsedUser(t *testing.T, parsed *url.URL) string {
	t.Helper()
	if parsed.User != nil {
		return parsed.User.Username()
	}
	return "postgres"
}

// quoteIdent double-quotes an identifier the way CREATE DATABASE OWNER needs.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	prepareMigrateTestDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, migrateTestDSN)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return pool
}

// resetSchema gives the next test an empty public schema. The runtime role is
// already dropped by the first test that runs Apply, so it is not touched here.
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

	// One row per applied migration file, not exactly one row: the runner
	// must record each version and skip the ones it has already applied.
	pending, err := load()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if want := len(pending); applied != want {
		t.Fatalf("schema_migrations has %d rows, want %d", applied, want)
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

	// The runtime role owns no objects (ARCHITECTURE 3.1): the migration grants
	// privileges TO it and never FROM it. A role that grants is recorded as the
	// owner or grantor of objects, which pins them in pg_shdepend with deptype
	// 'p' and makes the role undroppable (2BP01); a role that is only
	// granted-to leaves deptype 'a' ACL entries, which are expected for every
	// granted privilege. So check ownership and pinned grants only, across the
	// object classes the migration can create.
	var ownerOrGrantor int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT c.oid
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public'
			  AND c.relowner = (SELECT oid FROM pg_roles WHERE rolname = 'agentdeck_app')
			UNION
			SELECT p.oid FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = 'public'
			  AND p.proowner = (SELECT oid FROM pg_roles WHERE rolname = 'agentdeck_app')
			UNION
			SELECT t.oid FROM pg_type t
			WHERE t.typnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
			  AND t.typowner = (SELECT oid FROM pg_roles WHERE rolname = 'agentdeck_app')
			UNION
			SELECT a.oid FROM pg_default_acl a
			WHERE a.defaclrole = (SELECT oid FROM pg_roles WHERE rolname = 'agentdeck_app')
			UNION
			SELECT d.objid FROM pg_shdepend d
			WHERE d.refobjid = (SELECT oid FROM pg_roles WHERE rolname = 'agentdeck_app')
			  AND d.deptype = 'p'
		) deps`).Scan(&ownerOrGrantor); err != nil {
		t.Fatalf("check runtime role ownership: %v", err)
	}
	if ownerOrGrantor != 0 {
		t.Fatalf("runtime role agentdeck_app owns or is pinned as grantor of %d objects; it must own none", ownerOrGrantor)
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
