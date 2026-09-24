package modelprice

// Postgres-backed coverage for the four properties no fake repo can prove.
//
// A fake filters in Go and hands back whatever it was told to store. The real
// path is where these live:
//
//  1. The upsert. `ON CONFLICT (org_id, model)` is what makes a second PUT an
//     update rather than a 23505 — and the unique index is what makes it fire.
//     Re-pricing a model must not create a second row, or `Resolve` would pick
//     one of two prices by row order.
//  2. The absent sentinel round trip. -1 goes in, nil comes out, and the row is
//     NOT NULL throughout. A driver-level NULL or a CHECK that rejected -1 would
//     both break the "cached follows input" fallback in production only.
//  3. Tenant isolation. Every query carries org_id; a wrong org must see nothing
//     rather than another workspace's prices, because a leaked price is a leaked
//     cost structure.
//  4. Delete reports whether it removed anything, which is what makes a DELETE of
//     a model another workspace owns a 404 instead of a silent 204.
//
// Gated on AGENTDECK_TEST_DATABASE_URL exactly like internal/board, internal/auth
// and internal/providerreg, so a checkout without Postgres still runs the unit
// suite above.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/migrate"
	"agentdeck/internal/pricing"
	"agentdeck/internal/ulid"
)

const testDBEnv = "AGENTDECK_TEST_DATABASE_URL"

var pgSuitePool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv(testDBEnv)
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

func pgService(t *testing.T) *Service {
	t.Helper()
	if os.Getenv(testDBEnv) == "" {
		t.Skip(testDBEnv + " not set; Postgres-backed model price tests skipped")
	}
	if err := migrate.Apply(context.Background(), pgSuitePool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return NewService(NewPgxRepository(pgSuitePool))
}

// newOrg inserts a bare org row. The foreign key only needs the tenant to exist;
// registration is auth's business, not this suite's.
func newOrg(t *testing.T, ctx context.Context) string {
	t.Helper()
	id := ulid.Must()
	slug := strings.ToLower("org-" + id[len(id)-12:])
	if _, err := pgSuitePool.Exec(ctx,
		"INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)", id, slug, "Model Price Suite Org"); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	return id
}

func TestPgUpsertUpdatesInsteadOfDuplicating(t *testing.T) {
	svc := pgService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	org := newOrg(t, ctx)

	const model = "pg-upsert-model"
	first, err := svc.Set(ctx, org, "user-1", SetInput{
		Model: model, InputMicrosPer1M: i64(1_000_000), OutputMicrosPer1M: i64(2_000_000),
	})
	if err != nil {
		t.Fatalf("first set: %v", err)
	}
	second, err := svc.Set(ctx, org, "user-1", SetInput{
		Model: model, InputMicrosPer1M: i64(3_000_000), OutputMicrosPer1M: i64(4_000_000),
	})
	if err != nil {
		t.Fatalf("second set: %v", err)
	}

	if second.InputMicrosPer1M != 3_000_000 {
		t.Errorf("input after re-price = %d, want 3000000", second.InputMicrosPer1M)
	}
	if second.CreatedAt.Before(first.CreatedAt) {
		t.Errorf("created_at moved backwards: %v -> %v", first.CreatedAt, second.CreatedAt)
	}

	var rows int
	if err := pgSuitePool.QueryRow(ctx,
		"SELECT count(*) FROM agent_model_prices WHERE org_id = $1 AND model = $2",
		org, model).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	// Two rows for one (org, model) would make the resolved price depend on the
	// order Postgres happened to return them in.
	if rows != 1 {
		t.Errorf("rows = %d, want 1 (the upsert must update, not insert)", rows)
	}
}

// The sentinel round trip: nil -> -1 in the column, -1 -> nil on the way out,
// and the fallback still lands on the right neighbour.
func TestPgAbsentRatesRoundTripAndFallBack(t *testing.T) {
	svc := pgService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	org := newOrg(t, ctx)

	const model = "pg-roundtrip-model"
	if _, err := svc.Set(ctx, org, "user-1", SetInput{
		Model: model,
		// Only the two required rates: the other three stay absent.
		InputMicrosPer1M: i64(1_000_000), OutputMicrosPer1M: i64(2_000_000),
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	var stored int64
	if err := pgSuitePool.QueryRow(ctx,
		"SELECT cached_micros_per_1m FROM agent_model_prices WHERE org_id = $1 AND model = $2",
		org, model).Scan(&stored); err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if stored != MarkAbsent {
		t.Errorf("stored cached = %d, want the absent sentinel %d", stored, MarkAbsent)
	}

	got, err := svc.Get(ctx, org, model)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CachedMicrosPer1M != nil {
		t.Errorf("cached read back as %d, want nil (absent)", *got.CachedMicrosPer1M)
	}

	res := pricing.Resolve(model, got.AsPricingOverridePtr())
	if res.Source != pricing.SourceManual {
		t.Fatalf("source = %q, want manual", res.Source)
	}
	if res.Price.CachedMicrosPer1M != 1_000_000 {
		t.Errorf("cached = %d, want the input fallback 1000000", res.Price.CachedMicrosPer1M)
	}
	if res.Price.ReasoningMicrosPer1M != 2_000_000 {
		t.Errorf("reasoning = %d, want the output fallback 2000000", res.Price.ReasoningMicrosPer1M)
	}
}

func TestPgScopedToOneWorkspace(t *testing.T) {
	svc := pgService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	orgA, orgB := newOrg(t, ctx), newOrg(t, ctx)

	const model = "pg-tenant-model"
	if _, err := svc.Set(ctx, orgA, "user-1", SetInput{
		Model: model, InputMicrosPer1M: i64(9_000_000), OutputMicrosPer1M: i64(9_000_000),
	}); err != nil {
		t.Fatalf("set in A: %v", err)
	}

	if _, err := svc.Get(ctx, orgB, model); !errors.Is(err, ErrNotFound) {
		t.Errorf("get from B: err = %v, want ErrNotFound", err)
	}
	list, err := svc.List(ctx, orgB)
	if err != nil {
		t.Fatalf("list in B: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("B sees %d of A's prices, want 0", len(list))
	}
	// Deleting from the wrong workspace must not remove A's row.
	if err := svc.Remove(ctx, orgB, model); !errors.Is(err, ErrNotFound) {
		t.Errorf("remove from B: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(ctx, orgA, model); err != nil {
		t.Errorf("A's row disappeared after B tried to delete it: %v", err)
	}
}

func TestPgListIsSortedByModel(t *testing.T) {
	svc := pgService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	org := newOrg(t, ctx)

	for _, m := range []string{"pg-zeta", "pg-alpha", "pg-mid"} {
		if _, err := svc.Set(ctx, org, "user-1", SetInput{
			Model: m, InputMicrosPer1M: i64(1), OutputMicrosPer1M: i64(1),
		}); err != nil {
			t.Fatalf("set %s: %v", m, err)
		}
	}
	list, err := svc.List(ctx, org)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].Model > list[i].Model {
			t.Fatalf("list not sorted: %q before %q", list[i-1].Model, list[i].Model)
		}
	}
}

func TestPgDeleteReportsWhetherItRemovedAnything(t *testing.T) {
	svc := pgService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	org := newOrg(t, ctx)

	const model = "pg-delete-model"
	if _, err := svc.Set(ctx, org, "user-1", SetInput{
		Model: model, InputMicrosPer1M: i64(1), OutputMicrosPer1M: i64(1),
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := svc.Remove(ctx, org, model); err != nil {
		t.Fatalf("first remove: %v", err)
	}
	// Second delete is a 404 over HTTP, so the count has to be exposed.
	if err := svc.Remove(ctx, org, model); !errors.Is(err, ErrNotFound) {
		t.Errorf("second remove: err = %v, want ErrNotFound", err)
	}
}
