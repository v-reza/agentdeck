package modelprice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"agentdeck/internal/store"
	"agentdeck/internal/ulid"
)

// pgxRepo is the sqlc-backed Repo. Org scoping lives in the queries themselves
// (every WHERE carries org_id), so a wrong tenant yields no row rather than
// someone else's.
type pgxRepo struct {
	q *store.Queries
}

// NewPgxRepository builds the repository over the shared connection pool. The
// parameter is store.DBTX rather than *store.Queries so it takes a pool or a
// transaction interchangeably — same shape as providerreg and skill.
func NewPgxRepository(pool store.DBTX) Repo { return &pgxRepo{q: store.New(pool)} }

func (r *pgxRepo) List(ctx context.Context, orgID string) ([]Override, error) {
	rows, err := r.q.ListModelPrices(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list model prices: %w", err)
	}
	out := make([]Override, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromRow(row))
	}
	return out, nil
}

func (r *pgxRepo) Get(ctx context.Context, orgID, model string) (Override, error) {
	row, err := r.q.GetModelPrice(ctx, store.GetModelPriceParams{OrgID: orgID, Model: model})
	if err != nil {
		// A model the workspace has no override for and a model owned by
		// another workspace are the same answer on purpose: the two must be
		// indistinguishable.
		if errors.Is(err, pgx.ErrNoRows) {
			return Override{}, ErrNotFound
		}
		return Override{}, fmt.Errorf("get model price: %w", err)
	}
	return fromRow(row), nil
}

func (r *pgxRepo) Upsert(ctx context.Context, orgID, actor string, o Override) (Override, error) {
	// The id is only used on the insert branch: the upsert conflicts on
	// (org_id, model) and keeps the existing row's id, so re-pricing a model
	// does not churn the primary key.
	id, err := ulid.New()
	if err != nil {
		return Override{}, fmt.Errorf("new id: %w", err)
	}
	row, err := r.q.UpsertModelPrice(ctx, store.UpsertModelPriceParams{
		ID:                    id,
		OrgID:                 orgID,
		Model:                 o.Model,
		InputMicrosPer1m:      o.InputMicrosPer1M,
		OutputMicrosPer1m:     o.OutputMicrosPer1M,
		CachedMicrosPer1m:     orAbsent(o.CachedMicrosPer1M),
		ReasoningMicrosPer1m:  orAbsent(o.ReasoningMicrosPer1M),
		CacheWriteMicrosPer1m: orAbsent(o.CacheWriteMicrosPer1M),
		CreatedBy:             nullable(actor),
	})
	if err != nil {
		return Override{}, fmt.Errorf("upsert model price: %w", err)
	}
	return fromRow(row), nil
}

func (r *pgxRepo) Delete(ctx context.Context, orgID, model string) (int64, error) {
	n, err := r.q.DeleteModelPrice(ctx, store.DeleteModelPriceParams{OrgID: orgID, Model: model})
	if err != nil {
		return 0, fmt.Errorf("delete model price: %w", err)
	}
	return n, nil
}

// fromRow maps the generated row into the domain type. The three optional rates
// come back as the -1 sentinel for "absent" and are turned into nil, so the
// rest of the package never has to know about the sentinel — except in
// AsPricingOverride, which passes it straight back through.
func fromRow(row store.AgentModelPrice) Override {
	return Override{
		Model:                 row.Model,
		InputMicrosPer1M:      row.InputMicrosPer1m,
		OutputMicrosPer1M:     row.OutputMicrosPer1m,
		CachedMicrosPer1M:     optional(row.CachedMicrosPer1m),
		ReasoningMicrosPer1M:  optional(row.ReasoningMicrosPer1m),
		CacheWriteMicrosPer1M: optional(row.CacheWriteMicrosPer1m),
		CreatedBy:             deref(row.CreatedBy),
		CreatedAt:             row.CreatedAt.Time,
		UpdatedAt:             row.UpdatedAt.Time,
	}
}

// optional turns the stored sentinel into nil. A stored -1 means the field is
// not part of the price; anything else is an honest rate, including 0.
func optional(v int64) *int64 {
	if v == MarkAbsent {
		return nil
	}
	out := v
	return &out
}

// orAbsent is the inverse of optional: nil is stored as the sentinel, never as
// NULL, because the column is NOT NULL and the CHECK accepts >= -1.
func orAbsent(v *int64) int64 {
	if v == nil {
		return MarkAbsent
	}
	return *v
}

func nullable(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
