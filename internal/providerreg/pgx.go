package providerreg

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"agentdeck/internal/store"
)

// pgxRepo is the production Repo: queries are the sqlc-generated bindings, and
// every one of them carries org_id explicitly so no code path can forget the
// tenant scope.
type pgxRepo struct {
	q *store.Queries
}

// NewPgxRepository builds the Postgres-backed Repo.
func NewPgxRepository(pool store.DBTX) Repo {
	return &pgxRepo{q: store.New(pool)}
}

// nameTakenConstraint is the unique index from migration 0010. Postgres is the
// arbiter of the collision (not a pre-flight SELECT) because two concurrent
// creates would both pass a SELECT and one would still hit the index.
const nameTakenConstraint = "providers_org_name_key"

func nameTakenError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == nameTakenConstraint {
		return ErrNameTaken
	}
	return err
}

// noRowsError maps pgx.ErrNoRows to ErrProviderNotFound. A query scoped by
// `WHERE id = $1 AND org_id = $2` returns no rows for an absent id *and* for an
// id owned by another tenant, and US-AD07 requires those to be indistinguishable.
func noRowsError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProviderNotFound
	}
	return err
}

// timePtr converts a nullable timestamptz to a pointer, so "never fetched" and
// "fetched at the zero instant" stay distinguishable in the domain type.
func timePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func providerFrom(row store.Provider) Provider {
	return Provider{
		ID:              row.ID,
		OrgID:           row.OrgID,
		Name:            row.Name,
		Protocol:        Protocol(row.Protocol),
		BaseURL:         row.BaseUrl,
		Models:          DecodeModels(row.ModelsJson),
		ModelsFetchedAt: timePtr(row.ModelsFetchedAt),
		LastVerifiedAt:  timePtr(row.LastVerifiedAt),
		IsDefault:       row.IsDefault,
		HasKey:          len(row.ApiKeyEnc) > 0,
		CreatedAt:       row.CreatedAt.Time,
	}
}

func (r *pgxRepo) List(ctx context.Context, orgID string) ([]Provider, error) {
	rows, err := r.q.ListProviders(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(rows))
	for _, row := range rows {
		out = append(out, providerFrom(row))
	}
	return out, nil
}

func (r *pgxRepo) Get(ctx context.Context, orgID, id string) (Provider, error) {
	row, err := r.q.GetProvider(ctx, store.GetProviderParams{ID: id, OrgID: orgID})
	if err != nil {
		return Provider{}, noRowsError(err)
	}
	return providerFrom(row), nil
}

func (r *pgxRepo) Count(ctx context.Context, orgID string) (int, error) {
	n, err := r.q.CountProviders(ctx, orgID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *pgxRepo) Create(ctx context.Context, orgID, id string, in CreateInput) (Provider, error) {
	row, err := r.q.CreateProvider(ctx, store.CreateProviderParams{
		ID:        id,
		OrgID:     orgID,
		Name:      in.Name,
		Protocol:  in.Protocol,
		BaseUrl:   in.BaseURL,
		ApiKeyEnc: in.SealedKey,
		// The service always resolves this before the write: nil means "the
		// workspace decides", and by here that decision has been made.
		IsDefault: in.IsDefault != nil && *in.IsDefault,
	})
	if err != nil {
		return Provider{}, nameTakenError(err)
	}
	return providerFrom(row), nil
}

// Update binds only the fields the caller supplied. The generated params are
// pointers because the query uses sqlc.narg for each one, which is exactly what
// makes an absent field a no-op instead of a write of the zero value.
//
// ponytail: clear-then-set for the default flag is two statements outside a
// transaction, so a crash between them leaves the workspace with no default.
// AC9 makes that state legal rather than a half-write, and the unique index
// turns a concurrent collision into an honest 409. Wrap both in a transaction
// if a workspace ever reports losing its default.
func (r *pgxRepo) Update(ctx context.Context, orgID, id string, in UpdateInput) (Provider, error) {
	row, err := r.q.UpdateProvider(ctx, store.UpdateProviderParams{
		ID:        id,
		OrgID:     orgID,
		Name:      in.Name,
		Protocol:  in.Protocol,
		BaseUrl:   in.BaseURL,
		ApiKeyEnc: in.SealedKey,
		IsDefault: in.IsDefault,
	})
	if err != nil {
		return Provider{}, nameTakenError(noRowsError(err))
	}
	return providerFrom(row), nil
}

func (r *pgxRepo) ClearDefault(ctx context.Context, orgID string) error {
	return r.q.ClearDefaultProvider(ctx, orgID)
}

func (r *pgxRepo) Delete(ctx context.Context, orgID, id string) error {
	return r.q.DeleteProvider(ctx, store.DeleteProviderParams{ID: id, OrgID: orgID})
}

func (r *pgxRepo) AgentsUsing(ctx context.Context, orgID, providerID string) ([]AgentRef, error) {
	rows, err := r.q.ListAgentsUsingProvider(ctx, store.ListAgentsUsingProviderParams{
		OrgID:      orgID,
		ProviderID: &providerID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]AgentRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, AgentRef{ID: row.ID, Name: row.Name})
	}
	return out, nil
}

func (r *pgxRepo) SyncAgentBaseURL(ctx context.Context, orgID, providerID, baseURL string) error {
	return r.q.SyncAgentBaseURLForProvider(ctx, store.SyncAgentBaseURLForProviderParams{
		OrgID:      orgID,
		ProviderID: &providerID,
		BaseUrl:    &baseURL,
	})
}
