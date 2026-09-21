package main

// The wiring that turns phase 2's CRUD-only registry into the seven endpoints of
// ARCHITECTURE 6.2.8: the upstream probe and the credential decryption.
//
// Both live here rather than in internal/providerreg because both are about the
// *deployment* rather than the domain: the probe owns an http.Client with the
// SSRF-guarded redirect policy, and decryption needs AGENTDECK_MASTER_KEY. The
// service takes them as interfaces, which is what keeps its rules testable
// without a live endpoint or a real key.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"agentdeck/internal/crypto"
	"agentdeck/internal/provider"
	"agentdeck/internal/providerreg"
	"agentdeck/internal/store"
)

// providerProbe is the production providerreg.Probe. It is a thin adapter over
// internal/provider, which owns validation, timeouts, and redirect refusal — the
// adapter exists so the service never imports net/http.
type providerProbe struct {
	client *http.Client
}

func newProviderProbe() *providerProbe {
	return &providerProbe{client: provider.NewClient()}
}

func (p *providerProbe) ListModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	return provider.ListModels(ctx, p.client, baseURL, apiKey)
}

// ProbeInference classifies the upstream's answer so the service can decide
// whether another model is worth trying.
//
// The split is made here, not in internal/provider, because the classification
// is a registry policy: internal/provider reports what happened (an error that
// wraps ErrCredentialRejected, ErrUpstreamError, or ErrUnreachable) and this
// adapter turns that into the service's ProbeResult. The two sentinels are the
// contract between them.
func (p *providerProbe) ProbeInference(ctx context.Context, baseURL, apiKey, model string) (providerreg.ProbeResult, error) {
	err := provider.ProbeInference(ctx, p.client, baseURL, apiKey, model)
	if err == nil {
		return providerreg.ProbeResult{}, nil
	}
	// A malformed address is the caller's mistake and keeps travelling as an
	// error; only a real upstream answer becomes a result.
	if !errors.Is(err, provider.ErrCredentialRejected) &&
		!errors.Is(err, provider.ErrUpstreamError) &&
		!errors.Is(err, provider.ErrUnreachable) {
		return providerreg.ProbeResult{}, err
	}
	return providerreg.ProbeResult{
		CredentialRejected: errors.Is(err, provider.ErrCredentialRejected),
		Err:                err,
	}, nil
}

// credentialDecrypter builds the service's decrypt function from the raw master
// key.
//
// The key is decoded per call, not once at boot, so a deployment that starts
// without AGENTDECK_MASTER_KEY still serves the five CRUD endpoints: only the
// two that need a credential fail, and they fail as a probe error (502) rather
// than by refusing to start. The error text never carries key material.
func credentialDecrypter(rawKey string) func([]byte) (string, error) {
	return func(sealed []byte) (string, error) {
		key, err := crypto.LoadKey(rawKey)
		if err != nil {
			return "", fmt.Errorf("master key unavailable: %w", err)
		}
		plaintext, err := crypto.Open(key, sealed)
		if err != nil {
			return "", errors.New("stored credential could not be opened")
		}
		return plaintext, nil
	}
}

// newProviderService wires the registry with both phase-3 dependencies.
func newProviderService(pool store.DBTX, masterKey string) *providerreg.Service {
	return providerreg.NewServiceWithProbe(
		providerreg.NewPgxRepository(pool),
		providerreg.ServiceOptions{
			Probe:   newProviderProbe(),
			Decrypt: credentialDecrypter(masterKey),
		},
	)
}
