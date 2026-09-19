package migrate

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepairMigrationGrantsPrivilegesOnLegacySchema is the forward-fix
// guard. A database that applied the broken 0001.up.sql (HEAD b9c88be..8a18b37)
// records versions 1 and 2 but grants agentdeck_app nothing, so every API query
// dies with "permission denied for table orgs". Apply never re-runs a recorded
// version, which means editing 0001 heals only a database migrated from empty --
// an already-migrated one stays broken forever. Only a later migration can
// repair it.
//
// The test rebuilds that legacy state directly: migrate, strip the table
// privileges, rewind schema_migrations to the two versions the broken revision
// recorded. It then asserts Apply restores the privileges. It fails while no
// repair migration exists.
func TestPostgresRepairMigrationGrantsPrivilegesOnLegacySchema(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Reproduce the broken revision's end state: schema present, both versions
	// recorded, runtime role granted nothing.
	if _, err := pool.Exec(ctx, "REVOKE ALL ON ALL TABLES IN SCHEMA public FROM "+runtimeRole); err != nil {
		t.Fatalf("revoke table privileges: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version > 2"); err != nil {
		t.Fatalf("rewind schema_migrations: %v", err)
	}

	// Precondition: this must really be the broken state, or a green run here
	// would prove nothing.
	assertRuntimeRoleLacksPrivileges(t, ctx, pool)

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply repair migration: %v", err)
	}

	assertRuntimeRolePrivileges(t, ctx, pool)
	assertRuntimeRoleCanQuery(t, ctx, pool)
}

// assertRuntimeRoleLacksPrivileges fails when the legacy state the test builds
// already has the grants it is meant to be missing.
func assertRuntimeRoleLacksPrivileges(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var granted int
	err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relname = ANY($2)
		  AND has_table_privilege($1, c.oid, 'SELECT,INSERT,UPDATE,DELETE')`,
		runtimeRole, runtimeRoleTables).Scan(&granted)
	if err != nil {
		t.Fatalf("count granted tables: %v", err)
	}
	if granted != 0 {
		t.Fatalf("legacy state still has %d granted tables; the repair assertion would be vacuous", granted)
	}
}

// assertRuntimeRoleCanQuery proves the restored grants are usable, not merely
// present in the catalog.
func assertRuntimeRoleCanQuery(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	conn := asRuntimeRole(t, ctx, pool)

	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM orgs").Scan(&count); err != nil {
		t.Fatalf("runtime role SELECT orgs after repair: %v", err)
	}
}
