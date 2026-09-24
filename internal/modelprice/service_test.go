package modelprice

// Unit coverage for the two things the handler cannot prove: the validation
// rules, and the conversion into what `pricing.Resolve` takes.
//
// The conversion is the one that matters. `Resolve` ignores an override whose
// PriceVersion is zero, and its guard exists because a zero-valued struct
// reaching it would bill every model at nothing — silently, since zero is also
// a legal cost. So the test that pins "a stored row becomes a usable override"
// is the test that stands between this table and a ledger full of free rows.

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentdeck/internal/pricing"
)

// fakeRepo records what the service asked it to store.
type fakeRepo struct {
	stored  []Override
	deleted []string
	deleteN int64
}

func (f *fakeRepo) List(context.Context, string) ([]Override, error) { return f.stored, nil }

func (f *fakeRepo) Get(context.Context, string, string) (Override, error) {
	if len(f.stored) == 0 {
		return Override{}, ErrNotFound
	}
	return f.stored[0], nil
}

func (f *fakeRepo) Upsert(_ context.Context, _, _ string, o Override) (Override, error) {
	o.CreatedAt, o.UpdatedAt = time.Unix(0, 0).UTC(), time.Unix(0, 0).UTC()
	f.stored = append(f.stored, o)
	return o, nil
}

func (f *fakeRepo) Delete(_ context.Context, _, model string) (int64, error) {
	f.deleted = append(f.deleted, model)
	return f.deleteN, nil
}

func i64(v int64) *int64 { return &v }

func TestSetRequiresInputAndOutput(t *testing.T) {
	svc := NewService(&fakeRepo{})

	// An override that prices no output records zero for every completion, which
	// is the same number the unpriced tier produces — the failure this table
	// exists to remove. It is refused rather than defaulted to something.
	cases := map[string]SetInput{
		"no output rate": {Model: "m", InputMicrosPer1M: i64(1)},
		"no input rate":  {Model: "m", OutputMicrosPer1M: i64(1)},
		"empty model":    {InputMicrosPer1M: i64(1), OutputMicrosPer1M: i64(1)},
		"blank model":    {Model: "   ", InputMicrosPer1M: i64(1), OutputMicrosPer1M: i64(1)},
	}
	for name, in := range cases {
		if _, err := svc.Set(context.Background(), "org", "user", in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestSetAcceptsZeroButNotNegative(t *testing.T) {
	// Zero is a legal rate — a locally hosted model really can be free to run —
	// so it must survive validation. A negative rate is not a price.
	svc := NewService(&fakeRepo{})
	if _, err := svc.Set(context.Background(), "org", "user", SetInput{
		Model: "local", InputMicrosPer1M: i64(0), OutputMicrosPer1M: i64(0),
	}); err != nil {
		t.Fatalf("zero rates rejected: %v", err)
	}

	for _, bad := range []int64{-1, MaxMicrosPer1M + 1} {
		if _, err := svc.Set(context.Background(), "org", "user", SetInput{
			Model: "m", InputMicrosPer1M: i64(bad), OutputMicrosPer1M: i64(1),
		}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("rate %d: err = %v, want ErrInvalidInput", bad, err)
		}
	}
}

func TestSetTrimsModelName(t *testing.T) {
	repo := &fakeRepo{}
	if _, err := NewService(repo).Set(context.Background(), "org", "user", SetInput{
		Model: "  my-own-llama-70b  ", InputMicrosPer1M: i64(1), OutputMicrosPer1M: i64(2),
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	// The name is what `Resolve` matches on. A leading space would store a row
	// that no lookup ever finds, which reads as "my price was ignored".
	if got := repo.stored[0].Model; got != "my-own-llama-70b" {
		t.Errorf("stored model = %q, want trimmed", got)
	}
}

func TestRemoveMissingIsNotFound(t *testing.T) {
	svc := NewService(&fakeRepo{deleteN: 0})
	if err := svc.Remove(context.Background(), "org", "m"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// The load-bearing one: a stored row must actually win over the catalog.
//
// Without a positive PriceVersion, `Resolve` treats the override as absent and
// falls through to the table — so an operator's price would be accepted, stored,
// listed, and then ignored at billing time. The test measures the billed number,
// not the struct.
func TestStoredOverrideWinsOverCatalog(t *testing.T) {
	known := pricing.Catalog()[0]

	stored := Override{
		Model:             known,
		InputMicrosPer1M:  1_000_000, // $1.00 / 1M
		OutputMicrosPer1M: 2_000_000,
	}
	table := pricing.Resolve(known, nil)
	overridden := pricing.Resolve(known, stored.AsPricingOverridePtr())

	if overridden.Source != pricing.SourceManual {
		t.Fatalf("source = %q, want %q", overridden.Source, pricing.SourceManual)
	}
	if overridden.Price.InputMicrosPer1M != 1_000_000 {
		t.Errorf("input = %d, want the override's 1000000", overridden.Price.InputMicrosPer1M)
	}
	if table.Price.InputMicrosPer1M == overridden.Price.InputMicrosPer1M {
		t.Skipf("catalog already prices %s at $1/1M; override would be indistinguishable", known)
	}

	// And the cost that follows, because the source field is not the deliverable.
	tokens := pricing.Tokens{In: 1_000_000}
	if got := overridden.Cost(tokens); got != 1_000_000 {
		t.Errorf("cost = %d micros, want 1000000", got)
	}
}

// A model the shared table does not price is the BYO case. A stored override is
// the only thing that makes its cost non-zero, and that is what this asserts.
func TestOverridePricesAModelTheTableDoesNot(t *testing.T) {
	const byo = "my-own-llama-70b"

	before := pricing.Resolve(byo, nil)
	if before.Source != pricing.SourceUnpriced {
		t.Fatalf("fixture drifted: %s now resolves to %q", byo, before.Source)
	}
	if got := before.Cost(pricing.Tokens{In: 1_000_000, Out: 1_000_000}); got != 0 {
		t.Fatalf("fixture drifted: unpriced cost = %d, want 0", got)
	}

	row := Override{Model: byo, InputMicrosPer1M: 500_000, OutputMicrosPer1M: 1_500_000}
	after := pricing.Resolve(byo, row.AsPricingOverridePtr())

	if after.Source != pricing.SourceManual {
		t.Errorf("source = %q, want %q", after.Source, pricing.SourceManual)
	}
	if got := after.Cost(pricing.Tokens{In: 1_000_000, Out: 1_000_000}); got != 2_000_000 {
		t.Errorf("cost = %d micros, want 2000000", got)
	}
}

// An absent optional rate must fall back, not read as free. `cached` absent
// means "charge a cache hit like input"; if the sentinel leaked through as a
// real zero, cache hits would be recorded as costing nothing.
func TestAbsentOptionalRateFallsBack(t *testing.T) {
	byo := "my-own-llama-70b"
	row := Override{
		Model:             byo,
		InputMicrosPer1M:  1_000_000,
		OutputMicrosPer1M: 1_000_000,
		// cached, reasoning, cache_write all absent
	}
	res := pricing.Resolve(byo, row.AsPricingOverridePtr())

	if got := res.Price.CachedMicrosPer1M; got != 1_000_000 {
		t.Errorf("cached = %d, want the input fallback 1000000", got)
	}
	if got := res.Price.ReasoningMicrosPer1M; got != 1_000_000 {
		t.Errorf("reasoning = %d, want the output fallback 1000000", got)
	}
}

// A nil override has to stay nil: a workspace with no override must fall through
// to the table, and a zero-valued struct would win with a price of nothing.
func TestNoOverrideIsNil(t *testing.T) {
	if got := (*Override)(nil).AsPricingOverridePtr(); got != nil {
		t.Errorf("nil override converted to %+v, want nil", got)
	}
}
