package migrate

import (
	"context"
	"testing"
	"time"
)

// These tests need a live Postgres 16; testPool skips when
// AGENTDECK_TEST_DATABASE_URL is unset (see postgres_test.go), so the suite
// stays green on a machine without a database.
//
// 0008 is the only migration whose tables were documented in ARCHITECTURE
// before they existed, and ledger_entries cannot be created before runs
// (its run_id FK). So the assertions below are about the surface other
// packages will compile against: the exact column set, the FK direction, and
// the runtime role's privileges -- without the GRANT, every new query fails
// with "permission denied" while the migration itself reports success.

func TestPostgres0008TablesAndColumns(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 8); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	cases := []struct {
		table string
		want  []string
	}{
		{"runs", []string{
			"id", "org_id", "task_id", "agent_id", "attempt", "status", "outcome",
			"failure_kind", "claim_lock", "claim_expires", "worker_pid",
			"last_heartbeat_at", "max_runtime_seconds", "cost_micros", "tokens_in",
			"tokens_out", "summary", "error", "metadata_json", "started_at", "ended_at",
		}},
		{"ledger_entries", []string{
			"id", "org_id", "run_id", "task_id", "provider", "model", "kind",
			"tokens_in", "tokens_out", "cache_read_tokens", "cache_write_tokens",
			"reasoning_tokens", "cost_micros", "price_version", "price_source",
			"pricing_model", "created_at",
		}},
		{"agent_skills", []string{
			"id", "org_id", "slug", "name", "body_md", "version", "is_system",
			"created_by", "created_at", "updated_at",
		}},
	}

	for _, tc := range cases {
		rows, err := pool.Query(ctx, `
			SELECT a.attname
			FROM pg_attribute a
			JOIN pg_class c ON c.oid = a.attrelid
			WHERE c.relname = $1 AND a.attnum > 0 AND NOT a.attisdropped
			ORDER BY a.attname`, tc.table)
		if err != nil {
			t.Fatalf("read columns of %s: %v", tc.table, err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			got[name] = true
		}
		rows.Close()
		if len(got) == 0 {
			t.Fatalf("table %s does not exist after migration", tc.table)
		}
		for _, column := range tc.want {
			if !got[column] {
				t.Errorf("table %s is missing column %s", tc.table, column)
			}
		}
		if len(got) != len(tc.want) {
			t.Errorf("table %s has %d columns, want %d", tc.table, len(got), len(tc.want))
		}
	}

	// The FK must point runs -> ledger_entries, never the reverse: the whole
	// reason 0008 exists in one file is that ledger_entries cannot precede runs.
	var refTable string
	if err := pool.QueryRow(ctx, `
		SELECT c2.relname
		FROM pg_constraint con
		JOIN pg_class c1 ON c1.oid = con.conrelid
		JOIN pg_class c2 ON c2.oid = con.confrelid
		WHERE con.conname = 'ledger_entries_run_fk'`).Scan(&refTable); err != nil {
		t.Fatalf("ledger_entries_run_fk missing: %v", err)
	}
	if refTable != "runs" {
		t.Fatalf("ledger_entries_run_fk references %s, want runs", refTable)
	}

	// Without the GRANT at the end of 0008 the API role cannot touch the new
	// tables even though the migration succeeded.
	for _, table := range []string{"runs", "ledger_entries", "agent_skills"} {
		var canSelect, canInsert bool
		if err := pool.QueryRow(ctx, `
			SELECT has_table_privilege('agentdeck_app', $1, 'SELECT'),
			       has_table_privilege('agentdeck_app', $1, 'INSERT')`,
			table).Scan(&canSelect, &canInsert); err != nil {
			t.Fatalf("check privileges on %s: %v", table, err)
		}
		if !canSelect || !canInsert {
			t.Errorf("agentdeck_app lacks SELECT/INSERT on %s (select=%v insert=%v)", table, canSelect, canInsert)
		}
	}
}

// TestPostgres0008AgentsBaseURLConstraint is DECISIONS 6A.F: base_url is a
// property of the openai_compatible provider only. A base_url on a built-in
// provider would silently reroute traffic to a host the user chose, so the
// database -- not application code -- has to refuse it.
func TestPostgres0008AgentsBaseURLConstraint(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 8); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const orgID = "01JZ00000000000000000000C1"
	const projectID = "01JZ00000000000000000000C2"
	if _, err := pool.Exec(ctx, "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", orgID, "byo-org", "BYO"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)",
		projectID, orgID, "byo-project", "BYO"); err != nil {
		t.Fatal(err)
	}

	insertAgent := func(id, provider string, baseURL any) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO agents (id, org_id, project_id, name, provider, model, base_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, orgID, projectID, "agent-"+id[len(id)-4:], provider, "some-model", baseURL)
		return err
	}

	// A built-in provider with a base_url is the misconfiguration to reject.
	if err := insertAgent("01JZ00000000000000000000D1", "anthropic", "https://evil.test/v1"); err == nil {
		t.Error("agents accepted base_url on provider 'anthropic'")
	}
	// A BYO provider without one cannot be reached at all.
	if err := insertAgent("01JZ00000000000000000000D2", "openai_compatible", nil); err == nil {
		t.Error("agents accepted provider 'openai_compatible' without base_url")
	}
	// The two valid shapes.
	if err := insertAgent("01JZ00000000000000000000D3", "anthropic", nil); err != nil {
		t.Errorf("built-in provider without base_url rejected: %v", err)
	}
	if err := insertAgent("01JZ00000000000000000000D4", "openai_compatible", "https://byo.test/v1"); err != nil {
		t.Errorf("openai_compatible with base_url rejected: %v", err)
	}

	// archived_at is nullable and defaults to NULL, i.e. not archived (US-AD73).
	var archived *time.Time
	if err := pool.QueryRow(ctx, "SELECT archived_at FROM agents WHERE id = $1",
		"01JZ00000000000000000000D3").Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != nil {
		t.Fatalf("archived_at defaults to %v, want NULL", archived)
	}
}
