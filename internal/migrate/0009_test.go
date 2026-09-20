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
// 0009 adds agents.has_provider_key as a GENERATED column. The tests below pin
// the three properties the feature depends on, and none of them is visible from
// the column's existence alone:
//
//   - it is generated ALWAYS and STORED, not a plain BOOLEAN a caller must
//     remember to keep in sync;
//   - it reports the truth for both the empty and the populated case, and
//     follows a rotation and a revocation without anyone writing it;
//   - the database refuses a direct write, so no code path can set the flag
//     independently of the ciphertext it is supposed to describe.

func TestPostgres0009HasProviderKeyIsGenerated(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// information_schema reports the generation expression; pg_attribute carries
	// attgenerated ('s' for STORED, 'v' for virtual). Assert both, because a
	// virtual column would be recomputed per read and the point of this column is
	// that it cannot drift.
	var isGenerated, generationExpr, attGenerated string
	if err := pool.QueryRow(ctx, `
		SELECT c.is_generated, coalesce(c.generation_expression, ''), a.attgenerated::text
		FROM information_schema.columns c
		JOIN pg_attribute a ON a.attname = c.column_name
		JOIN pg_class k ON k.oid = a.attrelid AND k.relname = c.table_name
		WHERE c.table_name = 'agents' AND c.column_name = 'has_provider_key'`).
		Scan(&isGenerated, &generationExpr, &attGenerated); err != nil {
		t.Fatalf("agents.has_provider_key is missing after 0009: %v", err)
	}
	if isGenerated != "ALWAYS" {
		t.Errorf("has_provider_key is_generated = %q, want ALWAYS", isGenerated)
	}
	// attgenerated is `char`; cast to text in the query so it scans as a string.
	if attGenerated != "s" {
		t.Errorf("has_provider_key attgenerated = %q, want \"s\" (STORED)", attGenerated)
	}
	if generationExpr == "" {
		t.Error("has_provider_key has no generation expression")
	}
}

func TestPostgres0009HasProviderKeyFollowsTheCiphertext(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const orgID = "01JZ00000000000000000000E1"
	const projectID = "01JZ00000000000000000000E2"
	const agentID = "01JZ00000000000000000000E3"
	if _, err := pool.Exec(ctx, "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", orgID, "key-org", "Key"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)",
		projectID, orgID, "key-project", "Key"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, org_id, project_id, name, provider, model)
		VALUES ($1, $2, $3, $4, 'anthropic', 'claude-opus-4-6')`,
		agentID, orgID, projectID, "agent-key"); err != nil {
		t.Fatal(err)
	}

	read := func() bool {
		t.Helper()
		var has bool
		if err := pool.QueryRow(ctx, "SELECT has_provider_key FROM agents WHERE id = $1", agentID).Scan(&has); err != nil {
			t.Fatalf("read has_provider_key: %v", err)
		}
		return has
	}

	// A fresh agent has no credential, and NULL must read as false rather than
	// as a NULL that a Go scan would turn into a zero value by accident.
	if read() {
		t.Error("a new agent reports has_provider_key = true")
	}

	// Storing a credential flips it without anyone writing the flag.
	if _, err := pool.Exec(ctx, "UPDATE agents SET provider_api_key_enc = $2 WHERE id = $1",
		agentID, []byte("nonce||ciphertext||tag")); err != nil {
		t.Fatal(err)
	}
	if !read() {
		t.Error("after storing a credential has_provider_key is still false")
	}

	// Rotation keeps it true (the column is NOT NULL, not a change counter).
	if _, err := pool.Exec(ctx, "UPDATE agents SET provider_api_key_enc = $2 WHERE id = $1",
		agentID, []byte("another-nonce||ciphertext||tag")); err != nil {
		t.Fatal(err)
	}
	if !read() {
		t.Error("after rotating a credential has_provider_key became false")
	}

	// Revocation is the case a hand-maintained BOOLEAN would get wrong: DELETE
	// clears the ciphertext and the flag has to follow it back down.
	if _, err := pool.Exec(ctx, "UPDATE agents SET provider_api_key_enc = NULL WHERE id = $1", agentID); err != nil {
		t.Fatal(err)
	}
	if read() {
		t.Error("after revoking the credential has_provider_key is still true")
	}
}

// TestPostgres0009HasProviderKeyRefusesDirectWrites is the reason the column is
// generated rather than stored: if a caller could write it, the flag could
// disagree with the ciphertext it describes, and an agent would be labelled
// ready while holding no credential at all.
func TestPostgres0009HasProviderKeyRefusesDirectWrites(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const orgID = "01JZ00000000000000000000F1"
	const projectID = "01JZ00000000000000000000F2"
	if _, err := pool.Exec(ctx, "INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", orgID, "gen-org", "Gen"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)",
		projectID, orgID, "gen-project", "Gen"); err != nil {
		t.Fatal(err)
	}

	// An INSERT that tries to set the flag must be refused outright.
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, org_id, project_id, name, provider, model, has_provider_key)
		VALUES ($1, $2, $3, $4, 'anthropic', 'claude-opus-4-6', true)`,
		"01JZ00000000000000000000F3", orgID, projectID, "agent-gen-insert"); err == nil {
		t.Error("agents accepted a direct INSERT into the generated column has_provider_key")
	}

	const agentID = "01JZ00000000000000000000F4"
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, org_id, project_id, name, provider, model)
		VALUES ($1, $2, $3, $4, 'anthropic', 'claude-opus-4-6')`,
		agentID, orgID, projectID, "agent-gen-update"); err != nil {
		t.Fatal(err)
	}

	// So must an UPDATE, which is the shape a "keep the flag in sync" helper
	// would have taken.
	if _, err := pool.Exec(ctx, "UPDATE agents SET has_provider_key = true WHERE id = $1", agentID); err == nil {
		t.Error("agents accepted a direct UPDATE of the generated column has_provider_key")
	}

	// And the column is still reporting the truth after both refusals.
	var has bool
	if err := pool.QueryRow(ctx, "SELECT has_provider_key FROM agents WHERE id = $1", agentID).Scan(&has); err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("has_provider_key is true after only refused writes")
	}
}
