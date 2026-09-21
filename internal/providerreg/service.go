package providerreg

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"agentdeck/internal/provider"
	"agentdeck/internal/ulid"
)

// Service holds the registry's invariants. It owns validation and the lifecycle
// rules so handlers stay thin: parse, call a service method, map the error to a
// status code.
type Service struct {
	repo Repo
}

// NewService builds the domain service over a Repo.
func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

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
