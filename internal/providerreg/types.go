// Package providerreg owns the workspace provider registry (US-AD109,
// DECISIONS 6A.J).
//
// A provider is where an LLM endpoint and its credential live, once per
// workspace. Before this existed the pair sat on every agent
// (`agents.base_url` + `agents.provider_api_key_enc`), so one upstream key was
// copied once per agent that used it and rotating it meant editing all of them.
// Agents now reference a provider by id, and one edit reaches every user of it
// (AC6).
//
// The package name is not `provider`: internal/provider is the SSRF guard and
// the HTTP client that talks to an upstream endpoint. This one is the registry
// — CRUD, validation, and the lifecycle rules. They meet in the HTTP layer, not
// in each other.
package providerreg

import (
	"context"
	"errors"
	"time"
)

// Domain errors. Handlers map these to stable HTTP codes; the same failure must
// always yield the same code from every path.
var (
	// ErrProviderNotFound is the answer for an id that is absent *and* for an
	// id owned by another tenant: the two must be indistinguishable, which is
	// why every query carries org_id and a missing row maps here (US-AD07).
	ErrProviderNotFound = errors.New("provider not found in this workspace")
	// ErrNameTaken is the per-workspace name collision (providers_org_name_key)
	// — 409, because the operator fixes it by picking another name.
	ErrNameTaken = errors.New("a provider with this name already exists in this workspace")
	// ErrProviderInUse is AC5: a provider that agents still reference cannot be
	// deleted. It is a 409 whose body names them, so the caller knows what to
	// repoint rather than guessing.
	ErrProviderInUse = errors.New("provider is still used by agents")
	// ErrInvalidInput covers a malformed name, an unknown protocol, and a base
	// URL the SSRF guard refuses: all three are 400, because the payload is the
	// problem.
	ErrInvalidInput = errors.New("invalid input")
)

// Protocol is the dialect of an upstream endpoint. The DDL CHECK
// (providers_protocol_chk) lists exactly these three, so the API accepts what
// the database accepts and rejects what it would reject.
type Protocol string

const (
	// ProtocolOpenAICompatible is the only one with an implementation today:
	// probe and model fetch (AC3, AC7) are written for it.
	ProtocolOpenAICompatible Protocol = "openai_compatible"
	// ProtocolAnthropic and ProtocolGoogle exist in the schema from the start so
	// adding them later is code, not a data migration (DECISIONS 6A.J). They are
	// storable now — a row is valid data — they just have no probe yet.
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolGoogle    Protocol = "google"
)

// AcceptableProtocol reports whether p is one of the three the DDL allows.
func AcceptableProtocol(p string) bool {
	switch Protocol(p) {
	case ProtocolOpenAICompatible, ProtocolAnthropic, ProtocolGoogle:
		return true
	}
	return false
}

// Provider is one row of the workspace registry.
//
// It carries no credential: the sealed bytes stay behind the Repo boundary and
// only the handler that decrypts them ever asks for them. What a response needs
// is HasKey, and what the UI needs is MaskedKey, which is computed from the
// plaintext of the one request that supplied it and never persisted.
type Provider struct {
	ID string
	// OrgID is the tenant. Every read and write in this package is scoped by
	// it, so a caller cannot address another workspace's provider by id.
	OrgID string
	Name  string
	// Protocol is the dialect (AC1), not a vendor name. DECISIONS 6A.J moved
	// that meaning here: the vendor of a model is decided by the pricing
	// catalog, not by this column.
	Protocol Protocol
	BaseURL  string
	// Models is the last fetched model list (AC7). It is empty for a provider
	// nobody has fetched for yet, and ModelsFetchedAt is nil to say why: phase 3
	// reads nil as "never fetched, fetch now", not as "no refresh needed".
	Models          []string
	ModelsFetchedAt *time.Time
	// LastVerifiedAt is set only by a passing inference probe (AC3). A model
	// list fetch does not touch it, because it does not prove the credential.
	LastVerifiedAt *time.Time
	// IsDefault is the workspace's preselected provider (AC9). At most one row
	// per workspace carries it; deleting that row leaves none, which is legal.
	IsDefault bool
	// HasKey reports whether a credential is stored. It is derived from the
	// ciphertext, never a second column that could drift from it.
	HasKey    bool
	CreatedAt time.Time
}

// AgentRef is one row of AC5's "still used by" list. The name is what the
// operator reads; the id is what they navigate to in order to repoint it.
type AgentRef struct {
	ID   string
	Name string
}

// CreateInput is one create request after the handler has sealed any credential
// and decided the default flag. Empty (not pointer) fields mean "not supplied":
// a create has no stored value to fall back on.
type CreateInput struct {
	Name     string
	Protocol string
	BaseURL  string
	// SealedKey is the AES-256-GCM ciphertext, or nil for a provider with no
	// credential — a local Ollama endpoint is the case the DDL comment names.
	SealedKey []byte
	// IsDefault is tri-state on purpose. A create has no stored value to fall
	// back on, so "not supplied" and "explicitly false" would otherwise both
	// arrive as the zero value — and the first provider of a workspace must
	// become its default (AC9) while an explicit `false` must be honoured.
	IsDefault *bool
}

// UpdateInput is one patch request. Every field is a pointer so "absent" and
// "set to the zero value" stay distinguishable: `{"is_default": false}` must
// clear the default, while omitting the field must leave it alone.
type UpdateInput struct {
	Name      *string
	Protocol  *string
	BaseURL   *string
	SealedKey []byte
	IsDefault *bool
}

// Probe is the upstream call the registry needs: fetch a model list, or prove
// a credential with a minimal completion. It is an interface so the service's
// rules are testable without a live endpoint, and so the HTTP layer owns the
// concrete client and its timeouts.
//
// It takes the *plaintext* credential. The service is the only thing that can
// read one back (Repo.GetEncryptedKey), and passing it here rather than into
// the Repo keeps the decryption in one place.
type Probe interface {
	// ListModels returns the models an upstream advertises.
	ListModels(ctx context.Context, baseURL, apiKey string) ([]string, error)
	// ProbeInference proves a credential with the cheapest call that
	// authenticates. A model list does not prove it (AC3).
	ProbeInference(ctx context.Context, baseURL, apiKey, model string) error
}

// ProbeError wraps a failure that came from the upstream rather than from us.
// It is a distinct type because the HTTP code differs: a rejected credential or
// an unreachable host is a 502 (the caller's payload was fine, the upstream was
// not), while a malformed request is a 400.
type ProbeError struct {
	Err error
}

func (e *ProbeError) Error() string { return e.Err.Error() }
func (e *ProbeError) Unwrap() error { return e.Err }

// ErrNoKeyForProbe is the refusal to probe a provider that stores no
// credential. A local endpoint that checks nothing has nothing to prove, and
// reporting "verified" for it would be a claim the probe did not establish.
var ErrNoKeyForProbe = errors.New("provider has no credential to verify")

// ErrNoModelsToProbe is the refusal to verify when the provider has no model
// list yet: the probe needs a model name, and inventing one would test a model
// the operator does not serve.
var ErrNoModelsToProbe = errors.New("provider has no model list to probe with")

// Repo is the persistence boundary. It is expressed in domain types so the
// service owns validation and lifecycle rules, and so the HTTP tests can drive
// the real handlers against an in-memory double (the seam internal/skill and
// internal/board use). The production implementation is pgxRepo.
type Repo interface {
	// List returns every provider of the org, default first.
	List(ctx context.Context, orgID string) ([]Provider, error)
	// Get loads one provider scoped by org; a foreign or absent id is
	// ErrProviderNotFound.
	Get(ctx context.Context, orgID, id string) (Provider, error)
	// Count reports how many providers the org has, which decides whether a new
	// one becomes its default.
	Count(ctx context.Context, orgID string) (int, error)
	// Create inserts a provider. A name collision reports ErrNameTaken.
	Create(ctx context.Context, orgID, id string, in CreateInput) (Provider, error)
	// Update applies only the fields present in UpdateInput.
	Update(ctx context.Context, orgID, id string, in UpdateInput) (Provider, error)
	// ClearDefault unsets the org's current default, if any.
	ClearDefault(ctx context.Context, orgID string) error
	// Delete removes a provider scoped by org.
	Delete(ctx context.Context, orgID, id string) error
	// AgentsUsing lists the agents referencing the provider (AC5).
	AgentsUsing(ctx context.Context, orgID, providerID string) ([]AgentRef, error)
	// SyncAgentBaseURL carries a provider's address to the agents using it, so
	// AC6 holds everywhere an agent's endpoint is rendered before phase 6 drops
	// agents.base_url.
	SyncAgentBaseURL(ctx context.Context, orgID, providerID, baseURL string) error
	// GetEncryptedKey returns the stored ciphertext for a provider, or nil when
	// it has no credential. It is separate from Get because the domain Provider
	// deliberately carries no ciphertext: only the one path that needs to
	// decrypt can obtain the bytes, so no other code path can leak them.
	GetEncryptedKey(ctx context.Context, orgID, id string) ([]byte, error)
	// SetModels stores a freshly fetched model list and stamps
	// models_fetched_at with fetchedAt (AC7).
	SetModels(ctx context.Context, orgID, id string, models []string, fetchedAt time.Time) error
	// SetVerifiedAt stamps last_verified_at after a passing inference probe
	// (AC3).
	SetVerifiedAt(ctx context.Context, orgID, id string, at time.Time) error
	// StaleProviders lists providers across every workspace whose model list is
	// missing or older than before, capped at limit (AC7's automatic half). It
	// is the one read here that is not org-scoped, because the background
	// refresher has no tenant in hand; request-serving code must not call it.
	StaleProviders(ctx context.Context, before time.Time, limit int) ([]Provider, error)
}
