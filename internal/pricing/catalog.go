package pricing

import "sort"

// The price table is generated and unexported, and Resolve needs a name the
// caller already knows. The agent catalog endpoint needs the opposite view:
// everything the table prices. These two accessors are that view — a read-only
// projection, so the table stays the single source and no caller restates a
// price.
//
// Both apply withFallback before returning, so a caller never sees the internal
// -1 "field absent" sentinel and cannot mistake it for a free model.

// Catalog returns every exact model name, sorted so a dropdown is stable across
// requests. Names are returned as stored, including any provider prefix
// ("anthropic/claude-opus-4.6"), because that is the key Resolve matches.
func Catalog() []string {
	out := make([]string, 0, len(rawModelPrices))
	for name := range rawModelPrices {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Patterns returns the pattern table in evaluation order. The order is
// load-bearing (DECISIONS 6A.C: first match wins), so it is never sorted. The
// slice is a copy, so a caller cannot reorder the table the resolver walks.
func Patterns() []PricePattern {
	out := make([]PricePattern, 0, len(rawPatterns))
	for _, p := range rawPatterns {
		p.Price = withFallback(p.Price)
		out = append(out, p)
	}
	return out
}

// Providers returns the distinct provider ids the price table knows, sorted.
//
// It exists so "is this provider known?" has one answer, derived from the table
// itself: a hand-maintained list in a validator would drift from the table the
// moment a provider is added or renamed there. US-AD86 AC3 rejects an unknown
// provider with 400, and this is the set it is checked against.
func Providers() []string {
	seen := make(map[string]bool)
	for _, p := range rawModelPrices {
		if p.Provider != "" {
			seen[p.Provider] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
