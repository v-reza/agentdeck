// Package modelprice owns the per-model manual price overrides of a workspace:
// tier 1 of the four-tier price resolution (DECISIONS 6A.C, US-AD108 AC4).
//
// Why it has to exist. `pricing.Resolve` has always accepted an override
// parameter, and both production call sites passed `nil`, so tier 1 was
// unreachable. That is not a cosmetic gap for a BYO operator: none of the 51
// patterns is a catch-all (they are all vendor-specific — `grok-*`,
// `minimax-*`, `*-codex-mini`), so a model name from the operator's own
// endpoint resolves to tier 4 `unpriced` with zero rates. Measured against a
// real name in the dev database, `my-own-llama-70b` → `in=0 out=0`.
//
// A zero in `ledger_entries.cost_micros` cannot be told apart from "free", and
// the cost gate (US-AD32) never fires for a BYO agent. `price_source` has
// carried a `manual` value since migration 0008; what was missing was the row
// it describes.
//
// The unit is the same one the rest of the pricing path uses: micro-USD per
// 1,000,000 tokens, an integer. `$5.00/1M` is `5_000_000`. Nothing here is a
// float — ARCHITECTURE §9.1 forbids it, and a float would round the cheap
// cached rates (`$0.0028/1M`) to zero without saying so.
package modelprice

import (
	"context"
	"errors"
	"strings"
	"time"

	"agentdeck/internal/pricing"
)

// Domain errors. Handlers map these to stable HTTP codes, and every path that
// can produce the same failure must produce the same code.
var (
	// ErrNotFound is the answer for a model the workspace has no override for.
	// An id from another tenant is the same answer: the two must be
	// indistinguishable, which is why every query carries org_id.
	ErrNotFound = errors.New("no manual price for this model in this workspace")
	// ErrInvalidInput covers a malformed model name and a rate outside the
	// accepted range: both are 400, because the payload is the problem.
	ErrInvalidInput = errors.New("invalid input")
)

// Service is the read/write surface over the override table. It exists to hold
// the validation and the lookup the HTTP layer shares, not to hide the Repo —
// there is no state here beyond it.
type Service struct {
	repo Repo
}

// NewService builds the service over a Repo.
func NewService(repo Repo) *Service { return &Service{repo: repo} }

// List returns every override in the workspace, sorted by model.
func (s *Service) List(ctx context.Context, orgID string) ([]Override, error) {
	return s.repo.List(ctx, orgID)
}

// Get returns one override, or ErrNotFound.
func (s *Service) Get(ctx context.Context, orgID, model string) (Override, error) {
	return s.repo.Get(ctx, orgID, model)
}

// Set validates and stores one override. Validation lives here rather than in
// the handler so every path that writes — the HTTP route today, anything that
// calls the service later — is held to the same rules.
func (s *Service) Set(ctx context.Context, orgID, actor string, in SetInput) (Override, error) {
	model := strings.TrimSpace(in.Model)
	if model == "" {
		return Override{}, ErrInvalidInput
	}
	// Input and output are required: an override that prices no output would
	// record zero for every completion, which is indistinguishable from the
	// unpriced tier this table exists to replace.
	if in.InputMicrosPer1M == nil || in.OutputMicrosPer1M == nil {
		return Override{}, ErrInvalidInput
	}
	for _, r := range []*int64{in.InputMicrosPer1M, in.OutputMicrosPer1M,
		in.CachedMicrosPer1M, in.ReasoningMicrosPer1M, in.CacheWriteMicrosPer1M} {
		if r != nil && (*r < 0 || *r > MaxMicrosPer1M) {
			return Override{}, ErrInvalidInput
		}
	}

	return s.repo.Upsert(ctx, orgID, actor, Override{
		Model:                 model,
		InputMicrosPer1M:      *in.InputMicrosPer1M,
		OutputMicrosPer1M:     *in.OutputMicrosPer1M,
		CachedMicrosPer1M:     in.CachedMicrosPer1M,
		ReasoningMicrosPer1M:  in.ReasoningMicrosPer1M,
		CacheWriteMicrosPer1M: in.CacheWriteMicrosPer1M,
	})
}

// Remove deletes one override. A row that is already gone is ErrNotFound, so a
// caller cannot tell "gone" from "never yours".
func (s *Service) Remove(ctx context.Context, orgID, model string) error {
	n, err := s.repo.Delete(ctx, orgID, strings.TrimSpace(model))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MaxMicrosPer1M is the highest accepted rate, 10,000,000 micro-USD per 1M
// tokens — $10,000 for a million tokens.
//
// It exists to catch a unit mistake rather than to price anything: the most
// expensive entry in the shared table is under $200/1M, so a value an order of
// magnitude past it is a typo, and a typo here silently inflates every cost
// report that follows. The bound is deliberately far above any real price, so
// it never rejects an honest rate.
const MaxMicrosPer1M int64 = 10_000_000

// MarkAbsent is the sentinel for "this field is not part of the price, fall
// back to another one". It is re-exported from the pricing package's own
// `missing` constant so callers do not restate the number, and it is what the
// three optional columns default to: `cached ?? input`, `reasoning ?? output`,
// `cache_write ?? input`.
//
// NULL would have been the obvious storage for this and is wrong: NULL reads as
// "not set" and 0 reads as "free", and this needs a third meaning. The DDL CHECK
// (`>= -1`) is what keeps the sentinel out of the range of honest rates.
const MarkAbsent int64 = -1

// Override is one stored manual price. The three optional rates are `*int64`
// because "absent" is a real third state here, not a missing value.
type Override struct {
	Model                 string
	InputMicrosPer1M      int64
	OutputMicrosPer1M     int64
	CachedMicrosPer1M     *int64
	ReasoningMicrosPer1M  *int64
	CacheWriteMicrosPer1M *int64
	CreatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// SetInput is one write request after the handler has decoded the JSON body.
//
// Input and Output are pointers so "not supplied" is distinguishable from
// zero: an override with no output rate would price every completion at
// nothing, which is the failure this package exists to prevent. The three
// optional rates are nil-able and mean "absent", not "free".
type SetInput struct {
	Model                 string
	InputMicrosPer1M      *int64
	OutputMicrosPer1M     *int64
	CachedMicrosPer1M     *int64
	ReasoningMicrosPer1M  *int64
	CacheWriteMicrosPer1M *int64
}

// Repo is the persistence boundary. Every method is org-scoped by argument, so
// a caller cannot address another workspace's row.
type Repo interface {
	List(ctx context.Context, orgID string) ([]Override, error)
	Get(ctx context.Context, orgID, model string) (Override, error)
	Upsert(ctx context.Context, orgID, actor string, o Override) (Override, error)
	Delete(ctx context.Context, orgID, model string) (int64, error)
}

// AsPricingOverride converts a stored row into what `pricing.Resolve` takes.
//
// PriceVersion is set to the current table version because `Resolve` ignores an
// override with a zero version — that guard exists so an empty struct cannot
// silently bill everything at nothing, and a row read from the database is
// never that. The absent sentinel passes through untouched so `Resolve` still
// applies its fallbacks.
func (o Override) AsPricingOverride() pricing.ModelPrice {
	out := pricing.ModelPrice{
		PriceVersion:          pricing.PriceVersion,
		Model:                 o.Model,
		InputMicrosPer1M:      o.InputMicrosPer1M,
		OutputMicrosPer1M:     o.OutputMicrosPer1M,
		CachedMicrosPer1M:     MarkAbsent,
		ReasoningMicrosPer1M:  MarkAbsent,
		CacheWriteMicrosPer1M: MarkAbsent,
	}
	if o.CachedMicrosPer1M != nil {
		out.CachedMicrosPer1M = *o.CachedMicrosPer1M
	}
	if o.ReasoningMicrosPer1M != nil {
		out.ReasoningMicrosPer1M = *o.ReasoningMicrosPer1M
	}
	if o.CacheWriteMicrosPer1M != nil {
		out.CacheWriteMicrosPer1M = *o.CacheWriteMicrosPer1M
	}
	return out
}

// AsPricingOverridePtr is AsPricingOverride for a row that may be absent: a
// workspace with no override must hand `Resolve` a nil, not a zero-valued
// struct, or the override tier would win with a price of nothing.
func (o *Override) AsPricingOverridePtr() *pricing.ModelPrice {
	if o == nil {
		return nil
	}
	p := o.AsPricingOverride()
	return &p
}
