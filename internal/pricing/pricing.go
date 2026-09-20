// Package pricing menghitung estimasi biaya LLM dari tabel harga port 9Router.
//
// Seluruh jalur uang memakai integer: rate disimpan sebagai **micro-USD per
// 1.000.000 token** (`$5.00/1M → 5_000_000`) dan biaya per step adalah
// **micro-USD** (1 USD = 1_000_000 micro-USD). Tidak ada float, dan pembulatan
// dilakukan satu kali di akhir baris — bukan per komponen.
//
// Lihat ARCHITECTURE.md §9.1 (tipe + rumus) dan DECISIONS.md §6A (resolusi 4
// tingkat). Isi tabelnya di docs/PRICING.md, di-generate tools/gen_pricing.py
// bersama table_gen.go dari satu sumber yang sama.
package pricing

import "strings"

// PriceVersion adalah versi snapshot tabel harga. Disimpan di setiap baris
// ledger sebagai bukti audit algoritma harga yang dipakai saat baris dicatat.
const PriceVersion = 1

// microsPerUSD mengubah (token × micro-USD per 1M token) menjadi micro-USD.
const microsPerUSD = 1_000_000

// missing menandai field yang TIDAK ADA di tabel sumber (tanda `—` di
// docs/PRICING.md). Field seperti ini jatuh ke fallback, bukan dianggap nol:
// memperlakukannya sebagai nol membuat model tersebut gratis tanpa ketahuan.
const missing = -1

// ModelPrice adalah harga satu entri tabel, dalam micro-USD per 1.000.000 token.
type ModelPrice struct {
	PriceVersion          int    // Versi snapshot tabel harga
	Provider              string // "anthropic", "openai", "deepseek", ... ("" bila entri tanpa prefix)
	Model                 string // entri exact, mis. "claude-opus-4-6"
	InputMicrosPer1M      int64  // $5.00 / 1M  =  5_000_000
	OutputMicrosPer1M     int64  // $25.00 / 1M = 25_000_000
	CachedMicrosPer1M     int64  // prompt caching HIT
	ReasoningMicrosPer1M  int64  // reasoning punya harga SENDIRI, bukan disamakan ke output
	CacheWriteMicrosPer1M int64  // prompt caching WRITE (cache_creation)
}

// PricePattern adalah aturan fallback bila model tidak ada di tabel exact.
type PricePattern struct {
	Pattern string     // "deepseek-v*", "glm-5*", "kimi-k3*", ... — URUT, first match wins
	Price   ModelPrice // harga yang dipakai bila pattern cocok
}

// PriceSource menandai tingkat resolusi yang benar-benar dipakai (DECISIONS §6A.C).
// Disimpan di baris ledger bersama PricingModel: tanpa keduanya, baris lama tidak
// bisa dibuktikan karena tabel pattern berubah dan override berbeda antar tenant.
type PriceSource string

const (
	SourceManual   PriceSource = "manual"   // 1. override per-model milik org
	SourceCatalog  PriceSource = "catalog"  // 2. tabel exact
	SourcePattern  PriceSource = "pattern"  // 3. pattern regex
	SourceUnpriced PriceSource = "unpriced" // 4. tidak ada yang cocok
)

// Resolution adalah hasil resolusi harga: harga yang dipakai plus asal-usulnya.
// Harga nol dengan Source = SourceUnpriced berarti "tidak diketahui", BUKAN
// "gratis" — UI wajib membedakan keduanya.
type Resolution struct {
	Price        ModelPrice  // rate yang dipakai; field yang absen sudah jatuh ke fallback
	Source       PriceSource // manual | catalog | pattern | unpriced
	PricingModel string      // nama entri/pattern yang benar-benar dipakai, mis. "deepseek-*"
}

// Tokens adalah pemakaian token satu step LLM seperti dilaporkan provider.
type Tokens struct {
	In         int64 // prompt_tokens
	Out        int64 // completion_tokens
	Cached     int64 // cached_tokens / cache_read_input_tokens (cache HIT)
	Reasoning  int64 // reasoning_tokens
	CacheWrite int64 // cache_creation_input_tokens (cache WRITE)
}

// Resolve memilih harga untuk sebuah model lewat resolusi 4 tingkat, berhenti di
// tingkat pertama yang cocok (DECISIONS §6A.C):
//
//  1. override manual milik org (parameter; package ini tidak menyentuh DB)
//  2. tabel exact, dicocokkan APA ADANYA
//  3. tabel exact, dicocokkan TANPA prefix provider ("anthropic/x" -> "x")
//  4. pattern, URUT — urutan array rawPatterns itu load-bearing, bukan kosmetik
//  5. tidak ada yang cocok -> unpriced (harga nol, tapi sumbernya ditandai)
//
// Override dengan PriceVersion nol diabaikan: struct kosong yang lolos ke sini
// akan menagih semua model dengan nol, dan itu gagal senyap.
func Resolve(model string, override *ModelPrice) Resolution {
	if override != nil && override.PriceVersion > 0 {
		return Resolution{
			Price:        withFallback(*override),
			Source:       SourceManual,
			PricingModel: override.Model,
		}
	}

	bare := model
	if i := strings.LastIndex(model, "/"); i >= 0 {
		bare = model[i+1:]
	}

	if p, ok := rawModelPrices[model]; ok {
		return catalog(p)
	}
	if bare != model {
		if p, ok := rawModelPrices[bare]; ok {
			return catalog(p)
		}
	}

	for _, pat := range rawPatterns {
		if globMatch(pat.Pattern, bare) || globMatch(pat.Pattern, model) {
			return Resolution{
				Price:        withFallback(pat.Price),
				Source:       SourcePattern,
				PricingModel: pat.Pattern,
			}
		}
	}

	return Resolution{
		Price: ModelPrice{
			PriceVersion: PriceVersion,
			Provider:     providerOf(model),
			Model:        model,
		},
		Source:       SourceUnpriced,
		PricingModel: model,
	}
}

// Cost menghitung biaya satu step dalam micro-USD: 5 komponen, `reasoning`
// dihitung TERPISAH dengan rate-nya sendiri, dan pembulatan dilakukan satu kali
// di akhir — bukan per komponen (ARCHITECTURE §9.1).
//
//	miss = max(0, In - Cached - CacheWrite)
//	Cost = (miss*Input + Cached*CachedRate + Out*Output + Reasoning*ReasoningRate
//	        + CacheWrite*CacheWriteRate) / 1e6
func (r Resolution) Cost(t Tokens) int64 {
	in, out := clamp(t.In), clamp(t.Out)
	cached, reasoning, cacheWrite := clamp(t.Cached), clamp(t.Reasoning), clamp(t.CacheWrite)

	miss := in - cached - cacheWrite
	if miss < 0 {
		miss = 0
	}

	p := r.Price
	num := miss*p.InputMicrosPer1M +
		cached*p.CachedMicrosPer1M +
		out*p.OutputMicrosPer1M +
		reasoning*p.ReasoningMicrosPer1M +
		cacheWrite*p.CacheWriteMicrosPer1M
	if num < 0 {
		return 0 // rate negatif tidak ada di tabel; biaya negatif selalu salah
	}

	return (num + microsPerUSD/2) / microsPerUSD // pembulatan tunggal, half-up
}

func catalog(p ModelPrice) Resolution {
	return Resolution{Price: withFallback(p), Source: SourceCatalog, PricingModel: p.Model}
}

// withFallback mengganti field yang absen (`—` di docs/PRICING.md) dengan
// fallback-nya, bukan nol: cached ?? input, reasoning ?? output, cache_write ?? input.
func withFallback(p ModelPrice) ModelPrice {
	if p.CachedMicrosPer1M == missing {
		p.CachedMicrosPer1M = p.InputMicrosPer1M
	}
	if p.ReasoningMicrosPer1M == missing {
		p.ReasoningMicrosPer1M = p.OutputMicrosPer1M
	}
	if p.CacheWriteMicrosPer1M == missing {
		p.CacheWriteMicrosPer1M = p.InputMicrosPer1M
	}
	return p
}

// globMatch mencocokkan wildcard `*` (hanya itu; tidak ada `?` atau kelas
// karakter) secara case-insensitive, sama seperti RegExp(..., "i") di 9Router —
// itulah yang membuat `MiniMax-*` dan `minimax-*` sama-sama kena.
func globMatch(pattern, name string) bool {
	p, s := strings.ToLower(pattern), strings.ToLower(name)
	parts := strings.Split(p, "*")
	if len(parts) == 1 {
		return p == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(s, mid)
		if i < 0 {
			return false
		}
		s = s[i+len(mid):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

func providerOf(model string) string {
	if i := strings.Index(model, "/"); i >= 0 {
		return model[:i]
	}
	return ""
}

func clamp(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}
