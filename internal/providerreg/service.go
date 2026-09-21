package providerreg

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"agentdeck/internal/provider"
	"agentdeck/internal/ulid"
)

// Service holds the registry's invariants. It owns validation and the lifecycle
// rules so handlers stay thin: parse, call a service method, map the error to a
// status code.
type Service struct {
	repo Repo
	// probe is the upstream call. It may be nil: the CRUD paths never touch it,
	// and a deployment without it still serves five of the seven endpoints. The
	// two phase-3 methods refuse rather than panic when it is absent.
	probe Probe
	// decrypt turns stored ciphertext back into a credential. It is a field so
	// the service never imports internal/crypto directly and the tests can
	// supply a real AES key without touching the environment.
	decrypt func(sealed []byte) (string, error)
}

// ServiceOptions are the phase-3 dependencies. They are passed separately from
// the Repo because they are about reaching an upstream, not about persistence.
type ServiceOptions struct {
	Probe   Probe
	Decrypt func(sealed []byte) (string, error)
}

// NewService builds the domain service over a Repo.
func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

// NewServiceWithProbe builds the service with the upstream probe wired in.
// Without it, RefreshModels and Verify answer ErrProbeUnavailable — the CRUD
// endpoints work either way, which is what lets the two features land
// independently.
func NewServiceWithProbe(repo Repo, opts ServiceOptions) *Service {
	return &Service{repo: repo, probe: opts.Probe, decrypt: opts.Decrypt}
}

// ErrProbeUnavailable reports a probe call on a service built without one. It
// is a wiring fault, not a caller mistake, so it maps to a 500.
var ErrProbeUnavailable = errors.New("provider probe is not configured")

// List returns the workspace's providers, default first (AC9: the agent form
// preselects it).
func (s *Service) List(ctx context.Context, orgID string) ([]Provider, error) {
	return s.repo.List(ctx, orgID)
}

// Get loads one provider, scoped by org: a foreign id is ErrProviderNotFound,
// the same answer as an absent one (US-AD07 — the two must be indistinguishable).
func (s *Service) Get(ctx context.Context, orgID, id string) (Provider, error) {
	return s.repo.Get(ctx, orgID, id)
}

// Create adds a provider to the workspace.
//
// The default flag is decided here rather than by the caller alone: the first
// provider of a workspace becomes its default, which is the same rule migration
// 0010 applies to the providers it backfilled. An explicit true always wins.
//
// The address is checked with the address-only guard, never the resolving one:
// DECISIONS 6A.F says a write path must not do DNS, because the request would
// then block on a name the operator has not proven reachable, and because the
// check that matters for SSRF is on the connection, not on the save.
func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Provider, error) {
	name := strings.TrimSpace(in.Name)
	if err := validate(name, in.Protocol, in.BaseURL); err != nil {
		return Provider{}, err
	}

	if in.IsDefault == nil {
		count, err := s.repo.Count(ctx, orgID)
		if err != nil {
			return Provider{}, err
		}
		first := count == 0
		in.IsDefault = &first
	}
	if *in.IsDefault {
		// Two statements rather than one: providers_org_default_key is a
		// non-deferrable partial unique index, so a single UPDATE that unsets
		// the old default and sets the new one can still raise 23505 — Postgres
		// checks the index per row and may visit the new default first.
		if err := s.repo.ClearDefault(ctx, orgID); err != nil {
			return Provider{}, err
		}
	}

	in.Name = name
	return s.repo.Create(ctx, orgID, ulid.Must(), in)
}

// Update applies a partial change. Absent fields keep their stored value; see
// UpdateInput and the UpdateProvider query for why that is not the full-replace
// shape UpdateAgent uses.
//
// When the address changes, the agents using this provider are carried with it.
// That is AC6 ("an agent keeps no copy of the base URL") expressed as far as it
// can be today: until phase 6 drops agents.base_url, that column is still what
// the agent screens render, so leaving it stale would make the edit invisible.
func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Provider, error) {
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return Provider{}, ErrInvalidInput
		}
		in.Name = &trimmed
	}
	if in.Protocol != nil && !AcceptableProtocol(*in.Protocol) {
		return Provider{}, ErrInvalidInput
	}
	if in.BaseURL != nil {
		if err := validateBaseURL(*in.BaseURL); err != nil {
			return Provider{}, err
		}
	}

	// The default move needs the same clear-then-set shape as Create. The read
	// is also what makes a no-op patch (is_default already true) not clear
	// itself on the way to setting itself.
	if in.IsDefault != nil && *in.IsDefault {
		current, err := s.repo.Get(ctx, orgID, id)
		if err != nil {
			return Provider{}, err
		}
		if !current.IsDefault {
			if err := s.repo.ClearDefault(ctx, orgID); err != nil {
				return Provider{}, err
			}
		}
	}

	updated, err := s.repo.Update(ctx, orgID, id, in)
	if err != nil {
		return Provider{}, err
	}

	if in.BaseURL != nil {
		if err := s.repo.SyncAgentBaseURL(ctx, orgID, id, updated.BaseURL); err != nil {
			return Provider{}, err
		}
	}
	return updated, nil
}

// Delete removes a provider, refusing while agents still reference it (AC5).
//
// The refusal is a domain error carrying the agents rather than a bare 409: the
// contract says the response names them, and a handler that had to re-query to
// find out would be a second source of truth for the same fact.
func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	if _, err := s.repo.Get(ctx, orgID, id); err != nil {
		return err
	}
	users, err := s.repo.AgentsUsing(ctx, orgID, id)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return &InUseError{Agents: users}
	}
	return s.repo.Delete(ctx, orgID, id)
}

// InUseError is AC5's refusal. It carries the agents pinning the provider so the
// 409 body can name them without a second query.
type InUseError struct {
	Agents []AgentRef
}

func (e *InUseError) Error() string { return ErrProviderInUse.Error() }

// Unwrap makes errors.Is(err, ErrProviderInUse) hold, so the handler maps it by
// sentinel and reads the agents only when it needs to render them.
func (e *InUseError) Unwrap() error { return ErrProviderInUse }

// validate is the one gate every write passes through, so the API never accepts
// a payload the DDL would reject (providers_name_chk, providers_protocol_chk,
// providers_base_url_chk).
func validate(name, protocol, baseURL string) error {
	if name == "" {
		return ErrInvalidInput
	}
	if !AcceptableProtocol(protocol) {
		return ErrInvalidInput
	}
	return validateBaseURL(baseURL)
}

// validateBaseURL applies the SSRF guard at the address level: scheme, host
// shape, and literal IPs, with localhost and 127.0.0.1 allowed as exact strings
// (DECISIONS 6A.F). ValidateAddressOnly never resolves DNS, which is what keeps
// a save from blocking on a name.
//
// The localhost allowance is a pre-check rather than a call to
// ValidateOperatorBaseURL, mirroring internal/board: that validator also
// *resolves* the host to prove it is not loopback, and resolving on save would
// make registering a provider depend on live DNS — a host that is down, behind a
// VPN, or not deployed yet is the ordinary state of a form being filled in. The
// predicate is the same one (`provider.IsLocalProviderHost`), so the two paths
// cannot disagree about which hosts are allowed.
func validateBaseURL(baseURL string) error {
	if strings.TrimSpace(baseURL) == "" {
		return ErrInvalidInput
	}
	if _, err := provider.ValidateAddressOnly(baseURL); err == nil {
		return nil
	}
	if isOperatorLocalBaseURL(baseURL) {
		return nil
	}
	return ErrInvalidInput
}

// isOperatorLocalBaseURL reports whether the URL points at one of the two
// loopback spellings an operator may run a provider on (DECISIONS 6A.F).
// Text-only: no DNS, no dial.
func isOperatorLocalBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return provider.IsLocalProviderHost(u.Hostname())
}

// DecodeModels reads models_json. A malformed value is an internal fault rather
// than a caller error, so it degrades to an empty list: the registry screen must
// still render the provider the operator is trying to fix.
func DecodeModels(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// EncodeModels writes a model list for storage. A nil or empty list becomes
// `[]`, never null: the DDL has `DEFAULT '[]'::jsonb` and a CHECK that the value
// is an array, so null would be rejected by the database rather than stored.
func EncodeModels(models []string) []byte {
	if len(models) == 0 {
		return []byte("[]")
	}
	out, err := json.Marshal(models)
	if err != nil {
		// A []string always marshals; this is unreachable without a programming
		// error, and `[]` keeps the row valid instead of writing nothing.
		return []byte("[]")
	}
	return out
}

// ModelsStaleAfter is AC7's freshness window: a model list older than this is
// refetched rather than trusted. The contract fixes the number at 24 hours
// (DECISIONS 6A.J), chosen so a provider is not polled hourly for a list that
// almost never changes.
const ModelsStaleAfter = 24 * time.Hour

// ModelsStale reports whether a fetched model list should be refetched (AC7).
//
// A nil ModelsFetchedAt means "never fetched", which is stale — not fresh. The
// distinction is load-bearing: reading null as "nothing to refresh" would leave
// a provider that has never been polled permanently empty.
func ModelsStale(p Provider, now time.Time) bool {
	if p.ModelsFetchedAt == nil {
		return true
	}
	return now.Sub(*p.ModelsFetchedAt) > ModelsStaleAfter
}

// RefreshModels fetches the upstream model list and stores it with a fresh
// models_fetched_at (AC7).
//
// This is discovery, not proof: DECISIONS 6A.J measured a gateway that answers
// /models with no credential at all, so a success here says the endpoint is up
// and nothing about the key. Verify is what establishes that.
func (s *Service) RefreshModels(ctx context.Context, orgID, id string) (Provider, error) {
	p, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return Provider{}, err
	}
	if s.probe == nil {
		return Provider{}, ErrProbeUnavailable
	}
	apiKey, err := s.plaintextKey(ctx, orgID, id)
	if err != nil {
		return Provider{}, err
	}

	models, err := s.probe.ListModels(ctx, p.BaseURL, apiKey)
	if err != nil {
		return Provider{}, &ProbeError{Err: err}
	}

	fetchedAt := time.Now().UTC()
	if err := s.repo.SetModels(ctx, orgID, id, models, fetchedAt); err != nil {
		return Provider{}, err
	}
	// Re-read rather than patching the local copy: the row is the source of
	// truth for models_json and the timestamp, and returning a hand-assembled
	// value is how a response drifts from what was stored.
	return s.repo.Get(ctx, orgID, id)
}

// Verify proves a provider's credential with a minimal inference call (AC3).
//
// It deliberately does not accept a model name from the caller. The probe has
// to name a model the endpoint actually serves, and the only list this service
// trusts is the provider's own — a caller-supplied name would turn the probe
// into "is this model available", which is a different question with a
// different answer, and a wrong one would report a good key as broken.
func (s *Service) Verify(ctx context.Context, orgID, id string) (Provider, error) {
	p, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return Provider{}, err
	}
	if s.probe == nil {
		return Provider{}, ErrProbeUnavailable
	}
	if len(p.Models) == 0 {
		return Provider{}, ErrNoModelsToProbe
	}
	apiKey, err := s.plaintextKey(ctx, orgID, id)
	if err != nil {
		return Provider{}, err
	}
	if apiKey == "" {
		// A provider with no stored credential has nothing to prove. Reporting
		// success would be a "verified" badge backed by no evidence.
		return Provider{}, ErrNoKeyForProbe
	}

	if err := s.probe.ProbeInference(ctx, p.BaseURL, apiKey, p.Models[0]); err != nil {
		return Provider{}, &ProbeError{Err: err}
	}

	verifiedAt := time.Now().UTC()
	if err := s.repo.SetVerifiedAt(ctx, orgID, id, verifiedAt); err != nil {
		return Provider{}, err
	}
	return s.repo.Get(ctx, orgID, id)
}

// plaintextKey decrypts a provider's stored credential. An absent credential is
// ("", nil): "no key" is a state the caller decides about, not an error, since
// a local endpoint that checks nothing is legitimate.
func (s *Service) plaintextKey(ctx context.Context, orgID, id string) (string, error) {
	sealed, err := s.repo.GetEncryptedKey(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	if len(sealed) == 0 {
		return "", nil
	}
	if s.decrypt == nil {
		return "", ErrProbeUnavailable
	}
	plaintext, err := s.decrypt(sealed)
	if err != nil {
		// The ciphertext is unreadable: wrong master key, or a tampered row.
		// This is an operator problem, not a caller mistake, so it is a probe
		// failure rather than a 400 — and the message never carries the bytes.
		return "", &ProbeError{Err: errors.New("stored credential could not be decrypted")}
	}
	return plaintext, nil
}

// StaleProviders returns the providers whose model list needs refetching
// (AC7's automatic half), across every workspace.
//
// The cutoff is computed here from ModelsStaleAfter rather than taken from the
// caller, so the 24-hour rule has exactly one definition: a caller that passed
// its own cutoff could quietly disagree with ModelsStale, and the two would then
// answer differently about the same row.
func (s *Service) StaleProviders(ctx context.Context, now time.Time, limit int) ([]Provider, error) {
	if limit <= 0 {
		return nil, nil
	}
	return s.repo.StaleProviders(ctx, now.Add(-ModelsStaleAfter), limit)
}
