package board

import "testing"

// US-AD109 AC1: `agents.provider` is a **protocol** — the DDL allows exactly
// three values (`providers_protocol_chk`), and the agent form stopped offering a
// vendor list once the provider registry became the source of endpoint and
// credential (AC6).
//
// This validator used to read `pricing.Providers()` instead, which is the set of
// vendor ids the *price table* happens to know (openai, deepseek, qwen, ...).
// Those are different vocabularies, and accepting the vendor one meant
// `POST /projects/{id}/agents` stored `provider="deepseek"` — a value the schema
// does not allow and the UI never sends.
//
// Why this is not merely cosmetic: `agents.provider` has no CHECK in the DDL
// (0004 declares the column without one; 0008 only ever added the base_url
// constraint that 0011 dropped), so this function is the *only* thing standing
// between the API and a row no other layer would accept.
func TestValidateProviderAcceptsOnlyProtocols(t *testing.T) {
	// The three the DDL allows, plus the BYO protocol the app writes itself.
	for _, protocol := range []string{"openai_compatible", "anthropic", "google"} {
		t.Run("accepts_"+protocol, func(t *testing.T) {
			if err := ValidateProvider(protocol); err != nil {
				t.Fatalf("protocol %q the schema allows was rejected: %v", protocol, err)
			}
		})
	}

	// Vendor ids the price table prices. Every one of these must be refused:
	// they are not protocols, and a row carrying one is outside the CHECK the
	// schema declares for `providers.protocol`.
	for _, vendor := range []string{"openai", "deepseek", "anthropic/via-reseller", "qwen", "mistralai"} {
		t.Run("refuses_vendor_"+vendor, func(t *testing.T) {
			if err := ValidateProvider(vendor); err == nil {
				t.Fatalf("vendor id %q was accepted as a protocol", vendor)
			}
		})
	}

	if err := ValidateProvider("not-a-provider"); err == nil {
		t.Fatal("ValidateProvider accepted an id that is in no vocabulary at all")
	}
}
