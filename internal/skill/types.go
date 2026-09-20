// Package skill owns the per-org skill library (US-AD107, DECISIONS 6A.G).
//
// A skill is *data*, not a constant: its body is markdown a human reads and
// edits, and an agent only ever reads it (the slug is what agents.skills_json
// references). Nothing in this package exposes a write an agent credential
// could reach — the HTTP layer gates every write behind an owner/admin *user*
// session, and the domain has no path that accepts an agent identity at all.
package skill

import (
	"context"
	"errors"
	"regexp"
	"time"
)

// Domain errors. Handlers map these to stable HTTP codes; the same failure must
// always yield the same code from every path.
var (
	// ErrSkillNotFound is the answer for an id that is absent *and* for an id
	// owned by another tenant: the two must be indistinguishable, which is why
	// every query carries org_id and the missing row maps here (US-AD07).
	ErrSkillNotFound = errors.New("skill not found in this workspace")
	// ErrSlugTaken is the per-org slug collision (agent_skills_org_slug_key) —
	// 409, because the operator fixes it by picking another slug.
	ErrSlugTaken = errors.New("slug is already taken in this workspace")
	// ErrInvalidInput covers a malformed slug, an empty name, and a body that is
	// not valid UTF-8: all three are 400, because the payload is the problem.
	ErrInvalidInput = errors.New("invalid input")
	// ErrSystemSkill is the refusal to delete a seeded default. System skills
	// are the baseline every workspace starts from; removing one would silently
	// strip capability from agents that already reference its slug.
	ErrSystemSkill = errors.New("a system skill cannot be deleted")
)

// slugPattern mirrors the DDL CHECK agent_skills_slug_chk exactly, so a slug
// the API accepts is a slug the database accepts and a malformed one is a 400
// before any write is attempted.
var slugPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// Skill is one row of the org's library.
type Skill struct {
	ID string
	// OrgID is the tenant. Every read and write in this package is scoped by
	// it, so a caller cannot address another workspace's skill by id.
	OrgID     string
	Slug      string
	Name      string
	BodyMD    string
	Version   int
	IsSystem  bool
	CreatedBy *string // nil for a seeded system skill
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Usage is a skill plus the number of live agents referencing its slug. The UI
// needs the count on every list row (PLAN-WAVE2 7.1): it is what stops a user
// from editing a skill that agents are already running.
type Usage struct {
	Skill
	UsedBy int
}

// AgentRef is one row of the "dipakai oleh" list under the editor. The name is
// what routes to the agent detail page.
type AgentRef struct {
	ID   string
	Name string
}

// Repo is the persistence boundary. It is expressed in domain types so the
// service owns validation and lifecycle rules, and so the HTTP tests can drive
// the real handlers against an in-memory double (the same seam internal/board
// uses). The production implementation is pgxRepo, over store.Queries.
type Repo interface {
	// List returns every skill of the org with its usage count, ordered
	// system-first then by slug.
	List(ctx context.Context, orgID string) ([]Usage, error)
	// Get loads one skill scoped by org; a foreign or absent id is
	// ErrSkillNotFound.
	Get(ctx context.Context, orgID, id string) (Skill, error)
	// Create inserts a skill. A slug collision reports ErrSlugTaken.
	Create(ctx context.Context, s Skill) (Skill, error)
	// Update rewrites name and body and raises version by one. An id that is
	// absent or foreign is ErrSkillNotFound.
	Update(ctx context.Context, orgID, id, name, bodyMD string) (Skill, error)
	// Delete removes a skill scoped by org. Callers must have already rejected
	// system skills; the query also refuses them, which is why a no-op delete is
	// reported rather than silently ignored.
	Delete(ctx context.Context, orgID, id string) error
	// AgentsUsing lists the live agents of the org whose skills_json contains
	// the slug.
	AgentsUsing(ctx context.Context, orgID, slug string) ([]AgentRef, error)
}
