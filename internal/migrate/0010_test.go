package migrate

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need a live Postgres 16; testPool skips when
// AGENTDECK_TEST_DATABASE_URL is unset (see postgres_test.go), so the suite
// stays green on a machine without a database.
//
// 0010 introduces the provider registry and the backfill that moves existing
// agents' address + credential onto it. The tests below pin the properties the
// backfill can get wrong while still "succeeding":
//
//   - grouping: two agents that share an address and a credential must end up on
//     ONE provider row, and agents that differ must not be merged — merging is
//     silent data corruption, it repoints one agent at the other's endpoint;
//   - what does NOT move: an agent with no address keeps provider_id NULL,
//     because there is no address to put on a provider and inventing a vendor
//     base URL would contradict DECISIONS 6A.J;
//   - convergence: re-running the file (the state a partially applied database
//     or a removed version row produces) must not insert a second provider per
//     group, which is why the id is derived from the agents rather than random;
//   - phase 1 stays phase 1: `agents.base_url` and `agents_base_url_chk` survive,
//     because phase 6 removes them once the agent form stops reading them.

const (
	orgA  = "01JZ00000000000000000000A0"
	orgB  = "01JZ00000000000000000000B0"
	orgC  = "01JZ00000000000000000000C0"
	projA = "01JZ00000000000000000000D0"

	agentA1 = "01JZ00000000000000000000A1"
	agentA2 = "01JZ00000000000000000000A2"
	agentA3 = "01JZ00000000000000000000A3"
	agentA4 = "01JZ00000000000000000000A4"
	agentB1 = "01JZ00000000000000000000B1"
	agentC1 = "01JZ00000000000000000000C1"
	agentC2 = "01JZ00000000000000000000C2"

	agentsBaseURLChk = "agents_base_url_chk"
)

func TestPostgres0010ProviderRegistryMatchesContract(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 10); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Every column DECISIONS 6 and ARCHITECTURE 3 declare, with nullability. A
	// column that exists but is NOT NULL where the contract says nullable (or
	// vice versa) is a different table wearing the right names.
	want := map[string]string{
		"id":                "NO",
		"org_id":            "NO",
		"name":              "NO",
		"protocol":          "NO",
		"base_url":          "NO",
		"api_key_enc":       "YES",
		"models_json":       "NO",
		"models_fetched_at": "YES",
		"last_verified_at":  "YES",
		"is_default":        "NO",
		"created_at":        "NO",
	}
	rows, err := pool.Query(ctx, `
		SELECT column_name, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'providers'`)
	if err != nil {
		t.Fatalf("read providers columns: %v", err)
	}
	got := map[string]string{}
	for rows.Next() {
		var name, nullable string
		if err := rows.Scan(&name, &nullable); err != nil {
			t.Fatal(err)
		}
		got[name] = nullable
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for name, nullable := range want {
		have, ok := got[name]
		if !ok {
			t.Errorf("providers.%s is missing after 0010", name)
			continue
		}
		if have != nullable {
			t.Errorf("providers.%s is_nullable = %q, want %q", name, have, nullable)
		}
	}
	if len(got) != len(want) {
		t.Errorf("providers has %d columns, want %d: %v", len(got), len(want), got)
	}

	if _, err := pool.Exec(ctx,
		"INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", orgA, "enum-org", "Enum"); err != nil {
		t.Fatal(err)
	}

	// The protocol enum is the one thing that has to exist at the schema level
	// rather than in Go: DECISIONS 6A.J adds Anthropic and Google later as CODE,
	// not as a data migration, and that promise only holds if the column already
	// accepts all three values.
	for i, protocol := range []string{"openai_compatible", "anthropic", "google"} {
		// 26 characters, like every other id in this suite: providers_id_ulid_chk
		// is enforced, so a short literal fails as a ULID error rather than as the
		// protocol CHECK this loop is about.
		id := []string{
			"01JZ0000000000000000000P10",
			"01JZ0000000000000000000P20",
			"01JZ0000000000000000000P30",
		}[i]
		if _, err := pool.Exec(ctx, `
			INSERT INTO providers (id, org_id, name, protocol, base_url)
			VALUES ($1, $2, $3, $4, 'https://enum.test/v1')`,
			id, orgA, "enum-"+protocol, protocol); err != nil {
			t.Errorf("providers rejected protocol %q: %v", protocol, err)
		}
	}

	// And a protocol outside the enum is refused, so the CHECK is a constraint
	// rather than a comment.
	if _, err := pool.Exec(ctx, `
		INSERT INTO providers (id, org_id, name, protocol, base_url)
		VALUES ($1, $2, $3, 'ollama', 'https://enum.test/v1')`,
		"01JZ0000000000000000000P90", orgA, "enum-bad"); err == nil {
		t.Error("providers accepted protocol 'ollama'; the three-value enum is not enforced")
	}

	// agents.provider_id exists and is nullable. It carries no FK: the contract's
	// DDL declares none, and an FK cannot express the part that matters (that the
	// provider belongs to the same org as the agent), so AC5 queries the users.
	var nullable string
	if err := pool.QueryRow(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'agents' AND column_name = 'provider_id'`).
		Scan(&nullable); err != nil {
		t.Fatalf("agents.provider_id is missing after 0010: %v", err)
	}
	if nullable != "YES" {
		t.Errorf("agents.provider_id is_nullable = %q, want YES", nullable)
	}
}

// seedBackfillScenario creates the shapes the backfill has to tell apart, then
// drops the 0010 version row so the next Apply re-runs the file against them.
// A first Apply has already happened, so every table this seeds into exists.
func seedBackfillScenario(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}

	for _, org := range []struct{ id, slug string }{{orgA, "bf-a"}, {orgB, "bf-b"}, {orgC, "bf-c"}} {
		exec("INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", org.id, org.slug, org.slug)
	}
	exec("INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)",
		projA, orgA, "bf-proj", "BF")

	key := []byte("nonce||ciphertext||tag")

	// Org A: two agents sharing one address AND one credential, a third on a
	// different address, and a fourth that carries no address at all.
	exec(`INSERT INTO agents (id, org_id, project_id, name, provider, model, base_url, provider_api_key_enc)
	      VALUES ($1, $2, $3, $4, 'openai_compatible', 'shared-model-1', 'https://shared.test/v1', $5)`,
		agentA1, orgA, projA, "bf-a1", key)
	exec(`INSERT INTO agents (id, org_id, project_id, name, provider, model, base_url, provider_api_key_enc)
	      VALUES ($1, $2, $3, $4, 'openai_compatible', 'shared-model-2', 'https://shared.test/v1', $5)`,
		agentA2, orgA, projA, "bf-a2", key)
	exec(`INSERT INTO agents (id, org_id, project_id, name, provider, model, base_url)
	      VALUES ($1, $2, $3, $4, 'openai_compatible', 'other-model', 'https://other.test/v1')`,
		agentA3, orgA, projA, "bf-a3")
	// No address, so nothing to move. `openai` is required here: agents_base_url_chk
	// ties base_url IS NULL to a provider that is not openai_compatible.
	exec(`INSERT INTO agents (id, org_id, project_id, name, provider, model)
	      VALUES ($1, $2, $3, $4, 'openai', 'gpt-4o')`,
		agentA4, orgA, projA, "bf-a4")

	// Org B: exactly one provider, so it has to become that workspace's default.
	exec(`INSERT INTO agents (id, org_id, project_id, name, provider, model, base_url)
	      VALUES ($1, $2, $3, $4, 'openai_compatible', 'solo-model', 'https://solo.test/v1')`,
		agentB1, orgB, projA, "bf-b1")

	// Org C: two addresses on the same host, which must still get distinct names
	// (providers_org_name_key) or the whole migration aborts.
	exec(`INSERT INTO agents (id, org_id, project_id, name, provider, model, base_url)
	      VALUES ($1, $2, $3, $4, 'openai_compatible', 'dup-model-1', 'https://dup.test/v1')`,
		agentC1, orgC, projA, "bf-c1")
	exec(`INSERT INTO agents (id, org_id, project_id, name, provider, model, base_url)
	      VALUES ($1, $2, $3, $4, 'openai_compatible', 'dup-model-2', 'https://dup.test/v2')`,
		agentC2, orgC, projA, "bf-c2")

	exec("DELETE FROM schema_migrations WHERE version = 10")
}

// providerOf reads an agent's link, returning nil when it is NULL.
func providerOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, agentID string) *string {
	t.Helper()
	var id *string
	if err := pool.QueryRow(ctx, "SELECT provider_id FROM agents WHERE id = $1", agentID).Scan(&id); err != nil {
		t.Fatalf("read provider_id for %s: %v", agentID, err)
	}
	return id
}

func TestPostgres0010BackfillMovesAddressAndCredentialOntoProviders(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 10); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	seedBackfillScenario(t, pool, ctx)
	if err := apply(ctx, pool, 10); err != nil {
		t.Fatalf("re-apply 0010 over seeded agents: %v", err)
	}

	// Org A contributes two providers (shared + other), org B one, org C two.
	var total int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM providers").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("backfill created %d providers, want 5 (A:2 B:1 C:2)", total)
	}

	// The two agents sharing an address and a credential must share a provider:
	// that shared row is the entire point of the registry (AC6).
	a1, a2 := providerOf(t, ctx, pool, agentA1), providerOf(t, ctx, pool, agentA2)
	if a1 == nil || a2 == nil {
		t.Fatalf("shared-address agents were not linked: a1=%v a2=%v", a1, a2)
	}
	if *a1 != *a2 {
		t.Errorf("agents sharing an address and credential got different providers: %s vs %s", *a1, *a2)
	}

	// A different address must NOT be merged into that row.
	a3 := providerOf(t, ctx, pool, agentA3)
	if a3 == nil {
		t.Fatal("the differently-addressed agent was not linked")
	}
	if *a3 == *a1 {
		t.Error("agents on different addresses were merged into one provider")
	}

	// No address means nothing to move. This is the deliberate ceiling of phase 1.
	if a4 := providerOf(t, ctx, pool, agentA4); a4 != nil {
		t.Errorf("an agent with no base_url was linked to provider %s; it has nothing to move", *a4)
	}

	// The credential and the address moved with the link, and the model allowlist
	// holds every model the grouped agents were using, so a migrated agent is not
	// born violating the provider-model gate (DECISIONS 6A.J).
	var protocol, baseURL string
	var keyMatches, hasModel1, hasModel2 bool
	if err := pool.QueryRow(ctx, `
		SELECT protocol,
		       base_url,
		       api_key_enc = $2,
		       models_json @> '["shared-model-1"]'::jsonb,
		       models_json @> '["shared-model-2"]'::jsonb
		FROM providers WHERE id = $1`, *a1, []byte("nonce||ciphertext||tag")).
		Scan(&protocol, &baseURL, &keyMatches, &hasModel1, &hasModel2); err != nil {
		t.Fatalf("read backfilled provider: %v", err)
	}
	if protocol != "openai_compatible" {
		t.Errorf("backfilled protocol = %q, want openai_compatible", protocol)
	}
	if baseURL != "https://shared.test/v1" {
		t.Errorf("backfilled base_url = %q, want https://shared.test/v1", baseURL)
	}
	if !keyMatches {
		t.Error("the credential did not move onto the provider")
	}
	if !hasModel1 || !hasModel2 {
		t.Errorf("models_json is missing a model the grouped agents use: m1=%v m2=%v", hasModel1, hasModel2)
	}

	// This is a backfill, not a fetch and not a verification: both timestamps must
	// stay NULL so phase 3 reads "never fetched" and refreshes. A non-NULL
	// models_fetched_at here would make the 24h rule skip a provider that was
	// never actually queried.
	var verified, fetched int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE last_verified_at IS NOT NULL),
		       count(*) FILTER (WHERE models_fetched_at IS NOT NULL)
		FROM providers`).Scan(&verified, &fetched); err != nil {
		t.Fatal(err)
	}
	if verified != 0 || fetched != 0 {
		t.Errorf("backfill set %d last_verified_at and %d models_fetched_at; both must stay NULL", verified, fetched)
	}

	// A workspace whose only provider is this one gets it as the default (AC9);
	// org A has two, so it gets none and the form must ask.
	var bDefault, aDefaults int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM providers WHERE org_id = $1 AND is_default", orgB).Scan(&bDefault); err != nil {
		t.Fatal(err)
	}
	if bDefault != 1 {
		t.Errorf("org B has %d default providers, want 1 (its only provider)", bDefault)
	}
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM providers WHERE org_id = $1 AND is_default", orgA).Scan(&aDefaults); err != nil {
		t.Fatal(err)
	}
	if aDefaults != 0 {
		t.Errorf("org A has %d default providers, want 0 (two providers, no obvious default)", aDefaults)
	}

	// Two addresses on one host still need distinct names, or the insert aborts
	// on providers_org_name_key and the whole migration fails.
	var names []string
	rows, err := pool.Query(ctx, "SELECT name FROM providers WHERE org_id = $1 ORDER BY name", orgC)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] == names[1] {
		t.Errorf("two addresses on one host produced names %v, want two distinct names", names)
	}
}

func TestPostgres0010BackfillConvergesWhenReRun(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 10); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	seedBackfillScenario(t, pool, ctx)
	if err := apply(ctx, pool, 10); err != nil {
		t.Fatalf("re-apply 0010: %v", err)
	}

	snapshot := func() (int, map[string]string) {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM providers").Scan(&n); err != nil {
			t.Fatal(err)
		}
		links := map[string]string{}
		rows, err := pool.Query(ctx, "SELECT id, coalesce(provider_id, '') FROM agents ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, provider string
			if err := rows.Scan(&id, &provider); err != nil {
				t.Fatal(err)
			}
			links[id] = provider
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return n, links
	}

	countBefore, linksBefore := snapshot()

	// A second run of the same file. A random provider id would insert a second
	// row per group here; the id derived from the agents makes the insert a no-op
	// instead, which is what ON CONFLICT DO NOTHING relies on.
	if _, err := pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version = 10"); err != nil {
		t.Fatal(err)
	}
	if err := apply(ctx, pool, 10); err != nil {
		t.Fatalf("third apply of 0010: %v", err)
	}

	countAfter, linksAfter := snapshot()
	if countAfter != countBefore {
		t.Errorf("re-running the backfill changed the provider count: %d -> %d", countBefore, countAfter)
	}
	if len(linksAfter) != len(linksBefore) {
		t.Fatalf("re-running the backfill changed the agent count: %d -> %d", len(linksBefore), len(linksAfter))
	}
	for id, provider := range linksBefore {
		if linksAfter[id] != provider {
			t.Errorf("agent %s was relinked on re-run: %q -> %q", id, provider, linksAfter[id])
		}
	}
}

// TestPostgres0010LeavesTheOldColumnsInPlace is the boundary of this phase.
// DECISIONS 6A.J keeps `agents.base_url` and `agents_base_url_chk` until the
// agent form stops reading them (phase 5/6). Dropping them here would break the
// running form, and the backfill above depends on the CHECK to prove a grouped
// agent really is openai_compatible.
func TestPostgres0010LeavesTheOldColumnsInPlace(t *testing.T) {
	pool := testPool(t)
	resetSchema(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apply(ctx, pool, 10); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var baseURLCol int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'agents' AND column_name = 'base_url'`).
		Scan(&baseURLCol); err != nil {
		t.Fatal(err)
	}
	if baseURLCol != 1 {
		t.Error("agents.base_url is gone after 0010; phase 6 removes it, not phase 1")
	}

	var constraint int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint
		WHERE conname = $1 AND conrelid = 'agents'::regclass`, agentsBaseURLChk).Scan(&constraint); err != nil {
		t.Fatal(err)
	}
	if constraint != 1 {
		t.Error("agents_base_url_chk is gone after 0010; phase 6 removes it, not phase 1")
	}

	// The CHECK is still doing its job, which is what lets the backfill treat
	// `base_url IS NOT NULL` as proof of the protocol. An openai_compatible agent
	// without an address must still be refused.
	if _, err := pool.Exec(ctx,
		"INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)",
		"01JZ00000000000000000000E0", "keep-org", "Keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		"INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)",
		"01JZ00000000000000000000E1", "01JZ00000000000000000000E0", "keep-proj", "Keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, org_id, project_id, name, provider, model)
		VALUES ($1, $2, $3, $4, 'openai_compatible', 'no-address')`,
		"01JZ00000000000000000000E2", "01JZ00000000000000000000E0",
		"01JZ00000000000000000000E1", "keep-agent"); err == nil {
		t.Error("an openai_compatible agent with no base_url was accepted; agents_base_url_chk is not enforced")
	}
}
