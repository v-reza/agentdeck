package migrate

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// runtimeRoleTables are the domain tables ARCHITECTURE 3.1 requires the runtime
// role to read and write after a migration. schema_migrations is deliberately
// not here: the migration runner owns it, the API never touches it.
var runtimeRoleTables = []string{"orgs", "users", "memberships", "sessions", "org_kinds"}

// runtimeRole is the role the API connects as; the migration DSN belongs to the
// schema owner (ARCHITECTURE 3.1 / P6).
const runtimeRole = "agentdeck_app"

// TestPostgresRuntimeRoleCanUseDomainTables is ARCHITECTURE 3.1 / P6 from the
// positive side: the role the API connects as must be able to select, insert,
// update and delete on every domain table once an empty database has been
// migrated.
//
// The negative half of the same contract (the role owns no object and is
// nobody's grantor) lives in TestPostgresMigrationIsIdempotent. Both halves are
// needed: GRANT ... ON ALL TABLES IN SCHEMA silently matches zero tables when it
// sits above the CREATE TABLE statements, which leaves every other suite green
// while the API dies with "permission denied for table orgs".
func TestPostgresRuntimeRoleCanUseDomainTables(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertRuntimeRolePrivileges(t, ctx, pool)

	conn := asRuntimeRole(t, ctx, pool)
	assertRuntimeRoleCannotDDL(t, ctx, conn)
	assertRuntimeRoleCanWriteDomainTables(t, ctx, conn)
}

// assertRuntimeRolePrivileges reads the catalog. The privilege check is always
// called with the table OID: has_table_privilege's by-name overload parses its
// second argument as an OID and aborts on the first relation that is not a
// table.
func assertRuntimeRolePrivileges(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var missing []string
	for _, table := range runtimeRoleTables {
		var granted bool
		err := pool.QueryRow(ctx, `
			SELECT has_table_privilege($1, c.oid, 'SELECT,INSERT,UPDATE,DELETE')
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $2`, runtimeRole, table).Scan(&granted)
		if err != nil {
			t.Fatalf("read privileges for %s: %v", table, err)
		}
		if !granted {
			missing = append(missing, table)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("%s lacks SELECT/INSERT/UPDATE/DELETE on %d of %d domain tables: %v (ARCHITECTURE 3.1)",
			runtimeRole, len(missing), len(runtimeRoleTables), missing)
	}
}

// asRuntimeRole returns a pooled connection whose statements are checked against
// the runtime role's privileges. SET ROLE is used instead of a second login
// because the throwaway database's pg_hba rules authenticate TCP connections
// with scram-sha-256, and agentdeck_app deliberately has no password.
func asRuntimeRole(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *pgxpool.Conn {
	t.Helper()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(context.Background(), "RESET ROLE"); err != nil {
			t.Errorf("reset role: %v", err)
		}
		conn.Release()
	})

	if _, err := conn.Exec(ctx, "SET ROLE "+runtimeRole); err != nil {
		t.Fatalf("set role %s: %v", runtimeRole, err)
	}
	return conn
}

// assertRuntimeRoleCannotDDL is the negative control for this file: it proves
// the statements really are checked against the runtime role, so a passing
// privilege assertion cannot be a false positive.
func assertRuntimeRoleCannotDDL(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()

	if _, err := conn.Exec(ctx, "CREATE TABLE runtime_role_should_not_ddl (id integer)"); err == nil {
		t.Fatal("runtime role created a table; ARCHITECTURE 3.1 forbids the runtime role any DDL")
	}
}

// assertRuntimeRoleCanWriteDomainTables exercises the grants the way the API
// does. Catalog reads alone cannot catch a grant that exists but is unreachable
// at runtime.
func assertRuntimeRoleCanWriteDomainTables(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()

	const (
		orgID     = "01JZ00000000000000000000R1"
		userID    = "01JZ00000000000000000000R2"
		sessionID = "01JZ00000000000000000000R3"
	)

	inserts := []struct {
		table string
		sql   string
		args  []any
	}{
		{"orgs", "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)",
			[]any{orgID, "runtime-role", "Runtime Role"}},
		{"users", "INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, $4)",
			[]any{userID, "runtime-role@example.test", "Runtime", "$argon2id$v=19$m=65536,t=1,p=4$00$00"}},
		{"org_kinds", "INSERT INTO org_kinds (org_id, kind) VALUES ($1, $2)",
			[]any{orgID, "manual"}},
		{"memberships", "INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, $3)",
			[]any{orgID, userID, "owner"}},
		{"sessions", "INSERT INTO sessions (id, user_id, token_hash, expires_at) VALUES ($1, $2, $3, now() + interval '1 hour')",
			[]any{sessionID, userID, "0000000000000000000000000000000000000000000000000000000000000000"}},
	}
	for _, insert := range inserts {
		if _, err := conn.Exec(ctx, insert.sql, insert.args...); err != nil {
			t.Fatalf("runtime role INSERT into %s: %v", insert.table, err)
		}
	}

	if _, err := conn.Exec(ctx, "UPDATE orgs SET name = $2 WHERE id = $1", orgID, "Renamed"); err != nil {
		t.Fatalf("runtime role UPDATE orgs: %v", err)
	}

	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM memberships WHERE org_id = $1", orgID).Scan(&count); err != nil {
		t.Fatalf("runtime role SELECT memberships: %v", err)
	}
	if count != 1 {
		t.Fatalf("runtime role scoped SELECT returned %d rows, want 1", count)
	}

	if _, err := conn.Exec(ctx, "DELETE FROM sessions WHERE id = $1", sessionID); err != nil {
		t.Fatalf("runtime role DELETE sessions: %v", err)
	}

	var remaining int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE id = $1", sessionID).Scan(&remaining); err != nil {
		t.Fatalf("runtime role re-SELECT sessions: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("runtime role DELETE left %d session rows", remaining)
	}
}
