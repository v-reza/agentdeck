package skill

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"agentdeck/internal/store"
)

// pgxRepo is the production Repo: queries are the sqlc-generated bindings, and
// every one of them carries org_id explicitly so no code path can forget the
// tenant scope. Nothing here writes on behalf of an agent.
type pgxRepo struct {
	q *store.Queries
}

// NewPgxRepository builds the Postgres-backed Repo.
func NewPgxRepository(pool store.DBTX) Repo {
	return &pgxRepo{q: store.New(pool)}
}

// slugTakenConstraint is the unique index from migration 0008. Postgres is the
// arbiter of the collision (not a pre-flight SELECT) because two concurrent
// creates would both pass a SELECT and one would still hit the index.
const slugTakenConstraint = "agent_skills_org_slug_key"

func slugTakenError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == slugTakenConstraint {
		return ErrSlugTaken
	}
	return err
}

// noRowsError maps pgx.ErrNoRows to ErrSkillNotFound. A query scoped by
// `WHERE id = $1 AND org_id = $2` returns no rows for an absent id *and* for an
// id owned by another tenant, and US-AD07 requires those to be indistinguishable.
func noRowsError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSkillNotFound
	}
	return err
}

func skillFrom(row store.AgentSkill) Skill {
	return Skill{
		ID:        row.ID,
		OrgID:     row.OrgID,
		Slug:      row.Slug,
		Name:      row.Name,
		BodyMD:    row.BodyMd,
		Version:   int(row.Version),
		IsSystem:  row.IsSystem,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func (r *pgxRepo) List(ctx context.Context, orgID string) ([]Usage, error) {
	rows, err := r.q.ListAgentSkillsWithUsage(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Usage, 0, len(rows))
	for _, row := range rows {
		out = append(out, Usage{
			Skill: skillFrom(store.AgentSkill{
				ID:        row.ID,
				OrgID:     row.OrgID,
				Slug:      row.Slug,
				Name:      row.Name,
				BodyMd:    row.BodyMd,
				Version:   row.Version,
				IsSystem:  row.IsSystem,
				CreatedBy: row.CreatedBy,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
			}),
			UsedBy: int(row.UsedBy),
		})
	}
	return out, nil
}

func (r *pgxRepo) Get(ctx context.Context, orgID, id string) (Skill, error) {
	row, err := r.q.GetAgentSkill(ctx, store.GetAgentSkillParams{ID: id, OrgID: orgID})
	if err != nil {
		return Skill{}, noRowsError(err)
	}
	return skillFrom(row), nil
}

func (r *pgxRepo) Create(ctx context.Context, s Skill) (Skill, error) {
	row, err := r.q.CreateAgentSkill(ctx, store.CreateAgentSkillParams{
		ID:        s.ID,
		OrgID:     s.OrgID,
		Slug:      s.Slug,
		Name:      s.Name,
		BodyMd:    s.BodyMD,
		IsSystem:  s.IsSystem,
		CreatedBy: s.CreatedBy,
	})
	if err != nil {
		return Skill{}, slugTakenError(err)
	}
	return skillFrom(row), nil
}

func (r *pgxRepo) Update(ctx context.Context, orgID, id, name, bodyMD string) (Skill, error) {
	row, err := r.q.UpdateAgentSkill(ctx, store.UpdateAgentSkillParams{
		ID:     id,
		OrgID:  orgID,
		Name:   name,
		BodyMd: bodyMD,
	})
	if err != nil {
		return Skill{}, noRowsError(err)
	}
	return skillFrom(row), nil
}

// Delete relies on the service's pre-check for the system-skill refusal and for
// the 404 on a foreign id: the generated query is `:exec` and discards the
// affected-row count, so a no-op delete is not observable here.
func (r *pgxRepo) Delete(ctx context.Context, orgID, id string) error {
	return r.q.DeleteAgentSkill(ctx, store.DeleteAgentSkillParams{ID: id, OrgID: orgID})
}

func (r *pgxRepo) AgentsUsing(ctx context.Context, orgID, slug string) ([]AgentRef, error) {
	rows, err := r.q.ListAgentsUsingSkill(ctx, store.ListAgentsUsingSkillParams{
		OrgID: orgID,
		// The generated param is []byte because the query is `skills_json ? $2`;
		// Postgres resolves the right operand of `?` to text, and the text codec
		// sends a byte slice as the raw string. Passing the slug as bytes is
		// therefore the slug, not a JSON document.
		SkillsJson: []byte(slug),
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
