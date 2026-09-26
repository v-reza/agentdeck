package board

import (
	"context"
	"encoding/json"
	"errors"

	"agentdeck/internal/store"
)

// ErrNoDecrypter reports a missing master key. Refusing is deliberate: without a
// key the sealed bytes are useless, and returning them as a "credential" would
// send ciphertext as a bearer token.
var ErrNoDecrypter = errors.New("no credential decrypter configured")

// ClaimableBoards lists the boards the tick should look at.
func (s *Service) ClaimableBoards(ctx context.Context) ([]Board, error) {
	return s.repo.ClaimableBoards(ctx)
}

// ResolveAgentPrompt fills in the parts of an agent that only exist after a read:
// the skill bodies named by `skills_json` and the declared tool list.
//
// The credential is NOT resolved here. It is a separate call
// (`AgentProviderKey`) so a caller that only wants the prompt never receives a
// secret, and so the one place that decrypts is easy to find.
func (s *Service) ResolveAgentPrompt(ctx context.Context, agent Agent) (Agent, error) {
	var slugs []string
	if len(agent.SkillsJSON) > 0 {
		if err := json.Unmarshal(agent.SkillsJSON, &slugs); err != nil {
			// A malformed skills_json is a write-time bug, but failing the whole run
			// over it would hide the task behind an opaque error. The prompt falls
			// back to no skills, and the run still happens.
			slugs = nil
		}
	}
	agent.Skills = nil
	if len(slugs) > 0 {
		rows, err := s.repo.AgentSkillBodies(ctx, agent.OrgID, slugs)
		if err != nil {
			return agent, err
		}
		for _, r := range rows {
			agent.Skills = append(agent.Skills, AgentSkill{Slug: r.Slug, Name: r.Name, BodyMD: r.BodyMD})
		}
	}

	// tools_json is an allowlist of tool names. It is read so the executor can
	// report the mismatch rather than send the model a prompt promising tools it
	// cannot call; see internal/executor for why none are deliverable yet.
	var tools []string
	if len(agent.ToolsJSON) > 0 {
		_ = json.Unmarshal(agent.ToolsJSON, &tools)
	}
	agent.Tools = tools
	return agent, nil
}

// AgentProviderKey returns the decrypted credential for an agent, or an error the
// caller can branch on. It is the only method that decrypts.
func (s *Service) AgentProviderKey(ctx context.Context, id, orgID string) (string, error) {
	sealed, err := s.repo.AgentProviderKey(ctx, id, orgID)
	if err != nil {
		return "", err
	}
	if s.decrypt == nil {
		// Without a master key the sealed bytes are useless, and returning them as
		// a "key" would send ciphertext as a bearer token.
		return "", ErrNoDecrypter
	}
	return s.decrypt(sealed)
}

// WithDecrypter installs the AES-GCM opener used for provider credentials. Set
// once at wiring time from the same master key cmd/api already uses.
func (s *Service) WithDecrypter(fn func([]byte) (string, error)) *Service {
	s.decrypt = fn
	return s
}

// ------------------------------------------------------------------ repo ----

func (r *pgxRepository) ClaimableBoards(ctx context.Context) ([]Board, error) {
	rows, err := r.q.ListClaimableBoards(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(rows))
	for _, row := range rows {
		out = append(out, Board{
			ID: row.ID, OrgID: row.OrgID, ProjectID: row.ProjectID,
			Slug: row.Slug, Name: row.Name, BudgetDailyMicros: row.BudgetDailyMicros,
		})
	}
	return out, nil
}

func (r *pgxRepository) AgentSkillBodies(ctx context.Context, orgID string, slugs []string) ([]AgentSkill, error) {
	rows, err := r.q.AgentSkillBodies(ctx, store.AgentSkillBodiesParams{OrgID: orgID, Slugs: slugs})
	if err != nil {
		return nil, err
	}
	out := make([]AgentSkill, 0, len(rows))
	for _, row := range rows {
		out = append(out, AgentSkill{Slug: row.Slug, Name: row.Name, BodyMD: row.BodyMd})
	}
	return out, nil
}

// ProviderAddress reads a provider's base URL for the dispatcher. It returns only
// the address: the credential belongs to the provider row, and a caller that needs
// it goes through providerreg, which owns the decryption.
func (s *Service) ProviderAddress(ctx context.Context, orgID, id string) (string, error) {
	if s.providers == nil {
		return "", ErrNoProviderRegistry
	}
	p, err := s.providers.Get(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	return p.BaseURL, nil
}

// WithProviderRegistry wires the provider registry shown to the dispatcher. Nil
// means an agent with its own provider cannot resolve an endpoint, which is
// reported rather than guessed.
func (s *Service) WithProviderRegistry(reg ProviderRegistry) *Service {
	s.providers = reg
	return s
}

// ProviderRegistry is what the dispatcher needs from providerreg: where a provider
// lives, and the credential that authenticates against it.
type ProviderRegistry interface {
	Get(ctx context.Context, orgID, id string) (ProviderRef, error)
	ProviderKey(ctx context.Context, orgID, id string) (string, error)
}

// ProviderRef is the resolved endpoint of a provider row.
type ProviderRef struct {
	ID      string
	BaseURL string
}

// ProviderCredential reads the credential stored on a provider. §6A.J moved secrets
// to this entity, so this — not the per-agent column — is what a run authenticates
// with. An empty key is not an error: a local model server needs none.
func (s *Service) ProviderCredential(ctx context.Context, orgID, id string) (string, error) {
	if s.providers == nil {
		return "", ErrNoProviderRegistry
	}
	return s.providers.ProviderKey(ctx, orgID, id)
}

// ErrNoProviderRegistry reports that no registry was wired in.
var ErrNoProviderRegistry = errors.New("no provider registry configured")
