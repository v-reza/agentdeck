package skill

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"agentdeck/internal/ulid"
)

// Service holds the invariants of the skill library. It owns validation and the
// lifecycle rules so handlers stay thin: parse, call a service method, map the
// error to a status code.
type Service struct {
	repo Repo
}

// NewService builds the domain service over a Repo.
func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

// List returns the org's library with the usage count on every row (PLAN-WAVE2
// 7.1: "dipakai N agent" is what stops a user editing a live skill blindly).
func (s *Service) List(ctx context.Context, orgID string) ([]Usage, error) {
	return s.repo.List(ctx, orgID)
}

// Get loads one skill, scoped by org: a foreign id is ErrSkillNotFound, the same
// answer as an absent one (US-AD07 — the two must be indistinguishable).
func (s *Service) Get(ctx context.Context, orgID, id string) (Skill, error) {
	return s.repo.Get(ctx, orgID, id)
}

// Create adds a skill owned by the org. actorUserID is the *user* who asked;
// there is no path here that accepts an agent identity, which is the code-level
// half of US-AD107 AC1 ("agent hanya boleh memakai skill").
func (s *Service) Create(ctx context.Context, orgID, actorUserID, slug, name, bodyMD string) (Skill, error) {
	if err := validate(slug, name, bodyMD); err != nil {
		return Skill{}, err
	}
	actor := actorUserID
	return s.repo.Create(ctx, Skill{
		ID:        ulid.Must(),
		OrgID:     orgID,
		Slug:      slug,
		Name:      strings.TrimSpace(name),
		BodyMD:    bodyMD,
		Version:   1,
		IsSystem:  false,
		CreatedBy: &actor,
	})
}

// Update rewrites the name and body; the repository raises version by one
// (US-AD107 AC6). A system skill may be edited — it is data, not a constant —
// it just may not be deleted. The slug is never changed: agents reference it,
// so renaming it is a different skill, not an edit of this one.
func (s *Service) Update(ctx context.Context, orgID, id, name, bodyMD string) (Skill, error) {
	if err := validateName(name); err != nil {
		return Skill{}, err
	}
	if err := validateBody(bodyMD); err != nil {
		return Skill{}, err
	}
	return s.repo.Update(ctx, orgID, id, strings.TrimSpace(name), bodyMD)
}

// Delete removes a user-created skill. A seeded system skill is refused with
// ErrSystemSkill (409) *before* the delete is attempted: the DDL-side query also
// refuses it, but a delete that silently affects zero rows would surface to the
// operator as a success, and to the next reader as a bug.
func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	existing, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return ErrSystemSkill
	}
	return s.repo.Delete(ctx, orgID, id)
}

// AgentsUsing lists the live agents of the org referencing the slug. The screen
// needs it under the editor, so the slug is resolved from the id by the caller
// (the id is what the URL carries, the slug is what agents store).
func (s *Service) AgentsUsing(ctx context.Context, orgID, slug string) ([]AgentRef, error) {
	return s.repo.AgentsUsing(ctx, orgID, slug)
}

// SeedSystemSkills gives a workspace the eight defaults (US-AD107 AC5).
//
// Idempotent by construction: each skill is inserted on its own, and a slug that
// already exists is skipped rather than overwritten. That is deliberate on two
// counts — seeding runs on a workspace that may already carry the baseline (a
// retried signup reuses the personal workspace), and a user who edited the body
// of a system skill must not have that edit clobbered by a later seed.
//
// Each row gets its own ULID and starts at version 1: agent_skills.id is the
// primary key, so a shared id would make the eight defaults overwrite each other
// and leave exactly one behind.
func (s *Service) SeedSystemSkills(ctx context.Context, orgID string) error {
	for _, seed := range SystemSkills() {
		seed.ID = ulid.Must()
		seed.OrgID = orgID
		seed.Version = 1
		if _, err := s.repo.Create(ctx, seed); err != nil {
			if errors.Is(err, ErrSlugTaken) {
				continue
			}
			return err
		}
	}
	return nil
}

// validate is the one gate every write passes through, so the API never accepts
// a payload the DDL would reject (agent_skills_slug_chk, name NOT NULL).
func validate(slug, name, bodyMD string) error {
	if !slugPattern.MatchString(slug) {
		return ErrInvalidInput
	}
	if err := validateName(name); err != nil {
		return err
	}
	return validateBody(bodyMD)
}

// validateName rejects an empty or whitespace-only name. A name made of only
// spaces would pass the NOT NULL constraint and render as a blank row.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidInput
	}
	return nil
}

// validateBody accepts an empty body (a skill may start as a stub) but not a
// string that is invalid UTF-8: Postgres text is UTF-8, so an invalid sequence
// would either be rejected by the driver or stored lossily.
func validateBody(bodyMD string) error {
	if !utf8.ValidString(bodyMD) {
		return ErrInvalidInput
	}
	return nil
}
