package migrate

import (
	"context"
	"testing"
	"time"
)

// 0011 drops the per-agent endpoint. US-AD109 moved the address onto the
// provider; the column outlived the change only so the running agent form kept
// working, and phase 6 removed the form's last read of it.
//
// The assertions below are the three ways the drop can go wrong: the column
// survives (the migration silently no-ops), the CHECK constraint survives it
// (a constraint on a column that is gone blocks every write to the table), and
// the drop is not idempotent (a re-run aborts the whole migration transaction).

func TestPostgres0011DropsAgentBaseURL(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 11); err != nil {
		t.Fatalf("apply migrations up to 0011: %v", err)
	}

	var column int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'agents' AND column_name = 'base_url'`).
		Scan(&column); err != nil {
		t.Fatal(err)
	}
	if column != 0 {
		t.Error("agents.base_url survives 0011; the provider is the only place an endpoint may live")
	}

	var constraint int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint
		WHERE conname = 'agents_base_url_chk'`).Scan(&constraint); err != nil {
		t.Fatal(err)
	}
	if constraint != 0 {
		t.Error("agents_base_url_chk survives 0011; a CHECK on a dropped column blocks every insert")
	}

	// The reference is what replaced the address, so it must still be there —
	// dropping base_url without provider_id would leave an agent unreachable.
	var providerID int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'agents' AND column_name = 'provider_id'`).
		Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	if providerID != 1 {
		t.Error("agents.provider_id is gone; it is what replaced the address")
	}
}

// TestPostgres0011IsRerunnable replays the file against a schema that already
// has the drop applied. `DROP COLUMN IF EXISTS` is what makes this pass; a bare
// DROP would fail with 42703 and, because Apply runs every pending file in one
// transaction, take the rest of the migration with it.
func TestPostgres0011IsRerunnable(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 11); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version = 11"); err != nil {
		t.Fatal(err)
	}
	if err := apply(ctx, pool, 11); err != nil {
		t.Fatalf("replay of 0011: %v", err)
	}

	// The table has to still accept writes: that is what an orphaned constraint
	// would have broken.
	const orgID = "01JZ00000000000000000000E1"
	const projectID = "01JZ00000000000000000000E2"
	if _, err := pool.Exec(ctx, "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", orgID, "p6-org", "P6"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)",
		projectID, orgID, "p6-project", "P6"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, org_id, project_id, name, provider, model)
		VALUES ($1, $2, $3, $4, 'openai', 'gpt-4o')`,
		"01JZ00000000000000000000E3", orgID, projectID, "p6-agent"); err != nil {
		t.Fatalf("agents rejects a plain insert after 0011: %v", err)
	}
}
