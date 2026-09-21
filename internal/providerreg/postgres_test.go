package providerreg

// Postgres-backed coverage for the two things the in-memory doubles cannot
// prove.
//
// 1. The name collision. `nameTakenError` is a pure function the handler tests
//    pin by construction — the fake repo returns ErrNameTaken itself — so a
//    mutation that removes the translation from pgxRepo.Create leaves every one
//    of those tests green. Only the real pgx path proves the wiring, and the
//    production symptom is a 23505 surfacing as a 500.
//
// 2. The default move. providers_org_default_key is a non-deferrable partial
//    unique index, which is the whole reason Create and Update clear before they
//    set: a single `UPDATE ... SET is_default = (id = $x)` can still raise 23505
//    because Postgres checks the index per row. That ordering is invisible to a
//    fake repo, so a mutation removing ClearDefault passes every unit test and
//    fails only here.
//
// Gated on AGENTDECK_TEST_DATABASE_URL exactly like internal/board and
// internal/auth, so a checkout without Postgres still runs every other suite.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/migrate"
	"agentdeck/internal/ulid"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("AGENTDECK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AGENTDECK_TEST_DATABASE_URL not set; Postgres-backed provider tests skipped")
	}
	return url
}

// pgSuitePool is shared across the suite for the same reason as in auth and
// board: the migrations take an advisory lock, and one pool means they are never
// contested.
var pgSuitePool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("AGENTDECK_TEST_DATABASE_URL")
	if url == "" {
		os.Exit(m.Run())
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		os.Exit(1)
	}
	pgSuitePool = pool
	os.Exit(m.Run())
}

// pgRepo applies the schema and returns the production repository over the
// suite's pool — the same constructor main.go uses, so the code under test is
// the code that runs.
func pgRepo(t *testing.T) Repo {
	t.Helper()
	testDatabaseURL(t)
	if pgSuitePool == nil {
		t.Fatal("pgRepo called without AGENTDECK_TEST_DATABASE_URL")
	}
	if err := migrate.Apply(context.Background(), pgSuitePool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return NewPgxRepository(pgSuitePool)
}

// newOrg inserts a bare org row. The registry only needs the tenant id to exist
// for the foreign key; registration is auth's business, not this suite's.
func newOrg(t *testing.T, ctx context.Context) string {
	t.Helper()
	id := ulid.Must()
	slug := strings.ToLower("org-" + id[len(id)-12:])
	if _, err := pgSuitePool.Exec(ctx,
		"INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", id, slug, "Provider Suite Org"); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	return id
}

// create is the shared setup: one provider in orgID with a name unique to this
// test run, so subtests sharing an org cannot collide with each other.
func create(t *testing.T, repo Repo, ctx context.Context, orgID, name, baseURL string, isDefault *bool) Provider {
	t.Helper()
	p, err := repo.Create(ctx, orgID, ulid.Must(), CreateInput{
		Name:      name,
		Protocol:  string(ProtocolOpenAICompatible),
		BaseURL:   baseURL,
		IsDefault: isDefault,
	})
	if err != nil {
		t.Fatalf("create provider %s: %v", name, err)
	}
	return p
}

// TestPgStaleProvidersSpansWorkspacesAndOrdersNeverFetchedFirst pins the three
// properties of AC7's automatic query that nothing else can.
//
//  1. It crosses orgs. This is the one provider query without an org_id, so if the
//     WHERE clause ever gained one the refresher would silently stop seeing every
//     workspace but one — and no fake repo would notice, because the fake filters
//     in Go.
//  2. NULL models_fetched_at is stale, not fresh. `NULL < $1` is NULL in SQL, so a
//     naive comparison would exclude exactly the rows that need fetching first.
//  3. The LIMIT is real, so a large backlog degrades into several ticks instead of
//     one burst of upstream calls.
func TestPgStaleProvidersSpansWorkspacesAndOrdersNeverFetchedFirst(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgA := newOrg(t, ctx)
	orgB := newOrg(t, ctx)

	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	// Never fetched, in a second workspace: must be found, and first.
	never := create(t, repo, ctx, orgA, "never-fetched", "https://never.example.com/v1", nil)
	// Stale, in the other workspace: must be found.
	aged := create(t, repo, ctx, orgB, "aged-out", "https://aged.example.com/v1", nil)
	// Fresh: must not be found.
	fresh := create(t, repo, ctx, orgB, "just-fetched", "https://fresh.example.com/v1", nil)

	if err := repo.SetModels(ctx, never.OrgID, never.ID, []string{"m"}, old); err != nil {
		t.Fatalf("set models (aged): %v", err)
	}
	if err := repo.SetModels(ctx, aged.OrgID, aged.ID, []string{"m"}, old); err != nil {
		t.Fatalf("set models (aged): %v", err)
	}
	if err := repo.SetModels(ctx, fresh.OrgID, fresh.ID, []string{"m"}, recent); err != nil {
		t.Fatalf("set models (fresh): %v", err)
	}
	// Put the never-fetched one back to NULL: SetModels always stamps a time, and
	// NULL is the state under test.
	if _, err := pgSuitePool.Exec(ctx,
		"UPDATE providers SET models_fetched_at = NULL WHERE id = $1", never.ID); err != nil {
		t.Fatalf("reset models_fetched_at: %v", err)
	}

	// The dev database already holds providers with NULL models_fetched_at, and
	// NULLS FIRST means they sort ahead of everything this test creates. So the
	// assertions below filter to this test's own orgs, and the limit is set high
	// enough to reach them — the LIMIT behaviour itself is asserted separately at
	// the end, against the whole table.
	stale, err := repo.StaleProviders(ctx, now.Add(-ModelsStaleAfter), 10000)
	if err != nil {
		t.Fatalf("StaleProviders: %v", err)
	}

	found := map[string]bool{}
	for _, p := range stale {
		if p.OrgID != orgA && p.OrgID != orgB {
			continue
		}
		found[p.ID] = true
	}
	if !found[never.ID] {
		t.Error("a provider with models_fetched_at NULL was not reported stale")
	}
	if !found[aged.ID] {
		t.Error("a stale provider in another workspace was not found — the query is not crossing orgs")
	}
	if found[fresh.ID] {
		t.Error("a fresh provider was reported stale")
	}
	// NULLS FIRST: the never-fetched row must precede the aged one, within this
	// test's own rows.
	var neverIdx, agedIdx = -1, -1
	for i, p := range stale {
		if p.OrgID != orgA && p.OrgID != orgB {
			continue
		}
		if p.ID == never.ID {
			neverIdx = i
		}
		if p.ID == aged.ID {
			agedIdx = i
		}
	}
	if neverIdx == -1 || agedIdx == -1 || neverIdx > agedIdx {
		t.Errorf("never-fetched row (idx %d) must come before the aged row (idx %d)", neverIdx, agedIdx)
	}

	// The limit is honoured: a smaller cap must return fewer rows than the large
	// one, not the same set.
	limited, err := repo.StaleProviders(ctx, now.Add(-ModelsStaleAfter), 1)
	if err != nil {
		t.Fatalf("StaleProviders (limit 1): %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limit 1 returned %d rows", len(limited))
	}
}

// TestPgCreateProviderDuplicateNameIsANameConflict is the wiring proof for
// providers_org_name_key: the second insert collides and the repository must
// translate the 23505 into ErrNameTaken, which writeProviderError answers as
// 409. Without the translation the handler's default branch returns the raw
// SQLSTATE as a 500 — a user mistake reported as a server fault.
func TestPgCreateProviderDuplicateNameIsANameConflict(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgID := newOrg(t, ctx)

	name := "dup-" + ulid.Must()[20:]
	create(t, repo, ctx, orgID, name, "https://api.example.com/v1", nil)

	_, err := repo.Create(ctx, orgID, ulid.Must(), CreateInput{
		Name: name, Protocol: string(ProtocolOpenAICompatible), BaseURL: "https://other.example.com/v1",
	})
	if !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate name error = %v, want ErrNameTaken (AC1 requires 409)", err)
	}

	// And the same name in another workspace is fine: the index is per org.
	other := newOrg(t, ctx)
	if _, err := repo.Create(ctx, other, ulid.Must(), CreateInput{
		Name: name, Protocol: string(ProtocolOpenAICompatible), BaseURL: "https://other.example.com/v1",
	}); err != nil {
		t.Fatalf("same name in another workspace must be allowed: %v", err)
	}
}

// TestPgClearDefaultThenSetKeepsExactlyOneDefault is the ordering proof AC9
// rests on, driven through the *service* rather than the repository.
//
// That indirection is the point. "The first provider of a workspace becomes its
// default" and "clear before you set" both live in the service, so a test that
// calls repo.Create directly would hand it a nil IsDefault and assert against a
// rule the repository does not implement. It also has to reach pgxRepo for the
// ordering to be under test at all: a mutation that makes ClearDefaultProvider a
// no-op leaves every in-memory test green, because the fake repo does not carry
// providers_org_default_key. Here it surfaces as the 23505 the index would raise.
func TestPgClearDefaultThenSetKeepsExactlyOneDefault(t *testing.T) {
	repo := pgRepo(t)
	svc := NewService(repo)
	ctx := context.Background()
	orgID := newOrg(t, ctx)

	first, err := svc.Create(ctx, orgID, CreateInput{
		Name: "first", Protocol: string(ProtocolOpenAICompatible), BaseURL: "https://first.example.com/v1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if !first.IsDefault {
		t.Fatal("the first provider of a workspace is not its default")
	}

	second, err := svc.Create(ctx, orgID, CreateInput{
		Name: "second", Protocol: string(ProtocolOpenAICompatible), BaseURL: "https://second.example.com/v1",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.IsDefault {
		t.Fatal("the second provider became the default on its own")
	}

	// The production move, through the production path.
	yes := true
	if _, err := svc.Update(ctx, orgID, second.ID, UpdateInput{IsDefault: &yes}); err != nil {
		t.Fatalf("promote the second provider: %v", err)
	}

	list, err := repo.List(ctx, orgID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defaults := 0
	for _, p := range list {
		if p.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("workspace has %d defaults after the move, want exactly 1", defaults)
	}
	if !list[0].IsDefault || list[0].ID != second.ID {
		t.Fatalf("the default is not the promoted provider: %+v", list[0])
	}

	// Setting the same default again is a no-op, not a collision: the service
	// skips the clear when the row already holds the flag, so the index never
	// sees two rows claiming it.
	if _, err := svc.Update(ctx, orgID, second.ID, UpdateInput{IsDefault: &yes}); err != nil {
		t.Fatalf("re-setting the current default: %v", err)
	}

	// And an explicit false clears it, leaving the workspace with none — which
	// AC9 calls legal rather than an error.
	no := false
	if _, err := svc.Update(ctx, orgID, second.ID, UpdateInput{IsDefault: &no}); err != nil {
		t.Fatalf("clear the default explicitly: %v", err)
	}
	list, err = repo.List(ctx, orgID)
	if err != nil {
		t.Fatalf("list after clearing: %v", err)
	}
	for _, p := range list {
		if p.IsDefault {
			t.Fatalf("provider %q is still the default after an explicit false", p.Name)
		}
	}
}

// TestPgUpdateProviderIsPartialAgainstRealColumns is the mutation the handler
// test cannot make: it proves the UPDATE statement itself leaves the phase-3
// columns alone. models_json is written here directly, the way phase 3 will,
// then a rename must not blank it.
func TestPgUpdateProviderIsPartialAgainstRealColumns(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgID := newOrg(t, ctx)

	p := create(t, repo, ctx, orgID, "partial", "https://api.example.com/v1", nil)

	if _, err := pgSuitePool.Exec(ctx,
		`UPDATE providers SET models_json = '["gpt-4o","gpt-4o-mini"]'::jsonb,
		                      models_fetched_at = now(),
		                      last_verified_at = now()
		 WHERE id = $1`, p.ID); err != nil {
		t.Fatalf("seed phase-3 columns: %v", err)
	}

	renamed := "partial-renamed"
	got, err := repo.Update(ctx, orgID, p.ID, UpdateInput{Name: &renamed})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got.Name != renamed {
		t.Fatalf("name = %q, want %q", got.Name, renamed)
	}
	if len(got.Models) != 2 {
		t.Fatalf("models = %v, want the two stored entries — the UPDATE blanked them", got.Models)
	}
	if got.ModelsFetchedAt == nil {
		t.Fatal("models_fetched_at was blanked by a rename")
	}
	if got.LastVerifiedAt == nil {
		t.Fatal("last_verified_at was blanked by a rename")
	}
	if got.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("base_url = %q, want it unchanged", got.BaseURL)
	}
}

// TestPgAgentsUsingProviderIsOrgScoped pins the AC5 query's tenant scope: an
// agent in another workspace pointing at the same provider id must not appear,
// or the 409 would name agents the caller cannot see.
func TestPgAgentsUsingProviderIsOrgScoped(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgA := newOrg(t, ctx)
	orgB := newOrg(t, ctx)

	p := create(t, repo, ctx, orgA, "scoped", "https://api.example.com/v1", nil)

	// orgA needs a project to hang an agent off, then one agent pointing here.
	projectID := ulid.Must()
	if _, err := pgSuitePool.Exec(ctx,
		`INSERT INTO projects (id, org_id, slug, name) VALUES ($1, $2, $3, $4)`,
		projectID, orgA, strings.ToLower("p-"+projectID[len(projectID)-10:]), "P"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	agentID := ulid.Must()
	if _, err := pgSuitePool.Exec(ctx,
		`INSERT INTO agents (id, org_id, project_id, name, provider, model, provider_id, base_url)
		 VALUES ($1, $2, $3, $4, 'openai_compatible', 'gpt-4o', $5, $6)`,
		agentID, orgA, projectID, "Reviewer", p.ID, p.BaseURL); err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	refs, err := repo.AgentsUsing(ctx, orgA, p.ID)
	if err != nil {
		t.Fatalf("agents using: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != agentID {
		t.Fatalf("AgentsUsing in the owning org = %+v, want just %s", refs, agentID)
	}

	// Another tenant asking about the same provider id sees nothing.
	foreign, err := repo.AgentsUsing(ctx, orgB, p.ID)
	if err != nil {
		t.Fatalf("agents using from another org: %v", err)
	}
	if len(foreign) != 0 {
		t.Fatalf("a foreign workspace saw %d agents, want 0", len(foreign))
	}

	// And the AC6 sync reaches that agent — the column the agent screens render.
	const newURL = "https://moved.example.com/v1"
	if err := repo.SyncAgentBaseURL(ctx, orgA, p.ID, newURL); err != nil {
		t.Fatalf("sync base url: %v", err)
	}
	var got string
	if err := pgSuitePool.QueryRow(ctx, `SELECT base_url FROM agents WHERE id = $1`, agentID).Scan(&got); err != nil {
		t.Fatalf("read agent base_url: %v", err)
	}
	if got != newURL {
		t.Fatalf("agent base_url = %q, want %q — the sync did not reach it", got, newURL)
	}
}
