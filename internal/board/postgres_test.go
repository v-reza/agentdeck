package board

// Postgres-backed coverage for the two M1 acceptance criteria the in-memory
// helpers cannot prove: US-AD08 AC2 (duplicate slug inside one org is a 409,
// not a 500) and the tenant scope of that uniqueness (the same slug in a
// *different* org is a 201, because projects_org_slug_key is per org).
//
// The unit tests in pgx_test.go pin the mapping as a pure function. They cannot
// catch the repository forgetting to *call* it — a mutation that removes the
// slugTakenError call from CreateProject leaves every unit test green. Only the
// real pgx path proves the wiring, which is why this file exists.
//
// Gated on AGENTDECK_TEST_DATABASE_URL exactly like internal/auth and
// internal/migrate, so a checkout without Postgres still runs every other suite.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/migrate"
	"agentdeck/internal/ulid"
)

// lower makes a ULID-derived slug satisfy the DDL CHECK
// (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'): ULIDs are Crockford base32 and
// carry uppercase letters, which the constraint rejects. The same normalisation
// the service applies to a caller's slug.
func lower(s string) string { return strings.ToLower(s) }

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("AGENTDECK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AGENTDECK_TEST_DATABASE_URL not set; Postgres-backed board tests skipped")
	}
	return url
}

// pgSuitePool is shared across the suite for the same reason as in auth: the
// migrations take an advisory lock, and one pool means they are never contested.
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
func pgRepo(t *testing.T) Repository {
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

// newOrg inserts a bare org row. The board domain only needs the tenant id to
// exist for the foreign key; registration is auth's business, not this suite's.
func newOrg(t *testing.T, ctx context.Context) string {
	t.Helper()
	id := ulid.Must()
	slug := lower("org-" + id[len(id)-12:])
	if _, err := pgSuitePool.Exec(ctx,
		"INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", id, slug, "Board Suite Org"); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	return id
}

// TestPgCreateProjectDuplicateSlugIsASlugConflict is US-AD08 AC2 against the
// live database: the second insert collides on projects_org_slug_key and the
// repository must translate that into ErrSlugTaken, which writeBoardError
// answers as 409. Without the translation the handler's default branch returns
// the raw SQLSTATE as a 500 — a user mistake reported as a server fault.
func TestPgCreateProjectDuplicateSlugIsASlugConflict(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgID := newOrg(t, ctx)
	slug := lower("dup-" + ulid.Must()[20:])

	first, err := repo.CreateProject(ctx, Project{ID: ulid.Must(), OrgID: orgID, Slug: slug, Name: "First"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.Slug != slug {
		t.Fatalf("first create slug = %q, want %q", first.Slug, slug)
	}

	_, err = repo.CreateProject(ctx, Project{ID: ulid.Must(), OrgID: orgID, Slug: slug, Name: "Second"})
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate slug error = %v, want ErrSlugTaken (AC2 requires 409)", err)
	}
}

// TestPgCreateProjectSlugIsUniquePerOrgNotGlobally is the other half of AC1:
// uniqueness is scoped to the org, so two tenants may each own the same slug.
// A global unique index would make one tenant's project name block another's,
// which is also a tenant-isolation defect.
func TestPgCreateProjectSlugIsUniquePerOrgNotGlobally(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgA := newOrg(t, ctx)
	orgB := newOrg(t, ctx)
	slug := lower("shared-" + ulid.Must()[20:])

	if _, err := repo.CreateProject(ctx, Project{ID: ulid.Must(), OrgID: orgA, Slug: slug, Name: "In A"}); err != nil {
		t.Fatalf("create in org A: %v", err)
	}
	if _, err := repo.CreateProject(ctx, Project{ID: ulid.Must(), OrgID: orgB, Slug: slug, Name: "In B"}); err != nil {
		t.Fatalf("same slug in another org must be allowed: %v", err)
	}
}

// TestPgCreateBoardDuplicateSlugIsASlugConflict covers US-AD09 AC2, which has
// the identical 500 waiting one story later on boards_project_slug_key.
func TestPgCreateBoardDuplicateSlugIsASlugConflict(t *testing.T) {
	repo := pgRepo(t)
	ctx := context.Background()
	orgID := newOrg(t, ctx)

	project, err := repo.CreateProject(ctx, Project{ID: ulid.Must(), OrgID: orgID, Slug: "board-owner", Name: "Board Owner"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	slug := "board-dup"

	if _, err := repo.CreateBoard(ctx, Board{
		ID: ulid.Must(), OrgID: orgID, ProjectID: project.ID, Slug: slug, Name: "First", Columns: DefaultColumns,
	}); err != nil {
		t.Fatalf("first board: %v", err)
	}

	_, err = repo.CreateBoard(ctx, Board{
		ID: ulid.Must(), OrgID: orgID, ProjectID: project.ID, Slug: slug, Name: "Second", Columns: DefaultColumns,
	})
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate board slug error = %v, want ErrSlugTaken (AC2 requires 409)", err)
	}
}
