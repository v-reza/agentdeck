package pricing

import (
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

const docPath = "../../docs/PRICING.md"

// docRow adalah satu baris tabel di docs/PRICING.md, apa adanya.
type docRow struct {
	key   string   // nama model, atau pattern
	cells []string // 5 field: input, output, cached, reasoning, cache_creation ("—" = absen)
}

// parseDoc membaca docs/PRICING.md. Tabel Go di-generate dari sumber yang sama,
// jadi dokumen ini bisa dipakai sebagai acuan independen untuk membuktikan
// keduanya setia — bukan cuma sepanjang yang diklaim di komentar.
func parseDoc(t *testing.T) (exact []docRow, pattern []docRow) {
	t.Helper()
	b, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("baca %s: %v", docPath, err)
	}
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if !strings.HasPrefix(ln, "| ") || strings.HasPrefix(ln, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(ln, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		switch {
		case strings.HasPrefix(cells[0], "`"): // | `model` | i | o | c | r | w |
			if len(cells) != 6 {
				t.Fatalf("baris exact tidak 6 kolom: %s", ln)
			}
			exact = append(exact, docRow{strings.Trim(cells[0], "`"), cells[1:]})
		case isNumber(cells[0]): // | 1 | pattern | i | o | c | r | w |
			if len(cells) != 7 {
				t.Fatalf("baris pattern tidak 7 kolom: %s", ln)
			}
			pattern = append(pattern, docRow{cells[1], cells[2:]})
		}
	}
	return exact, pattern
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

// microsOf mengubah sel dokumen (USD per 1M token, atau "—") jadi micro-USD per
// 1M token. "—" -> missing, artinya field itu tidak ada dan resolver yang
// menjatuhkannya ke fallback.
func microsOf(t *testing.T, cell string) int64 {
	t.Helper()
	if cell == "—" {
		return missing
	}
	usd, err := strconv.ParseFloat(cell, 64)
	if err != nil {
		t.Fatalf("harga %q tidak bisa dibaca: %v", cell, err)
	}
	return int64(math.Round(usd * 1e6))
}

// TestTableLoads: jumlah entri di Go harus sama dengan yang benar-benar ada di
// docs/PRICING.md, urutannya sama, dan tiap harga identik.
func TestTableLoads(t *testing.T) {
	mdExact, mdPattern := parseDoc(t)

	if len(rawModelPrices) != len(mdExact) {
		t.Errorf("entri exact: Go=%d, PRICING.md=%d", len(rawModelPrices), len(mdExact))
	}
	if len(rawPatterns) != len(mdPattern) {
		t.Errorf("pattern: Go=%d, PRICING.md=%d", len(rawPatterns), len(mdPattern))
	}
	// Angka yang diklaim ARCHITECTURE §9.1 / DECISIONS §6A dan digate verify_suite.py.
	if len(rawModelPrices) != 220 {
		t.Errorf("entri exact = %d, seharusnya 220", len(rawModelPrices))
	}
	if len(rawPatterns) != 51 {
		t.Errorf("pattern = %d, seharusnya 51", len(rawPatterns))
	}
}

// TestTableFaithfulToSource membandingkan SETIAP harga di tabel Go dengan USD di
// docs/PRICING.md, dikonversi round(usd × 1e6). Ini sekaligus regresi bug 1000x:
// kalau satuannya salah (round(usd) saja), 397 harga di sini langsung jadi nol
// dan ketahuan. Field absen (`—`) harus tetap absen di Go, bukan jadi nol.
func TestTableFaithfulToSource(t *testing.T) {
	mdExact, mdPattern := parseDoc(t)

	for _, row := range mdExact {
		got, ok := rawModelPrices[row.key]
		if !ok {
			t.Errorf("entri %q ada di PRICING.md tapi tidak di tabel Go", row.key)
			continue
		}
		assertPriceMatchesDoc(t, "exact "+row.key, got, row.cells)
	}
	for i, row := range mdPattern {
		if i >= len(rawPatterns) {
			break
		}
		got := rawPatterns[i]
		if got.Pattern != row.key {
			t.Errorf("pattern #%d: Go=%q, PRICING.md=%q — urutan bergeser", i+1, got.Pattern, row.key)
			continue
		}
		assertPriceMatchesDoc(t, "pattern "+row.key, got.Price, row.cells)
	}
}

func assertPriceMatchesDoc(t *testing.T, label string, got ModelPrice, cells []string) {
	t.Helper()
	fields := []struct {
		name string
		got  int64
	}{
		{"input", got.InputMicrosPer1M},
		{"output", got.OutputMicrosPer1M},
		{"cached", got.CachedMicrosPer1M},
		{"reasoning", got.ReasoningMicrosPer1M},
		{"cache_creation", got.CacheWriteMicrosPer1M},
	}
	for i, f := range fields {
		want := microsOf(t, cells[i])
		if f.got != want {
			t.Errorf("%s %s: Go=%d, dokumen=%s (-> %d)", label, f.name, f.got, cells[i], want)
		}
	}
}

// TestMicrosScaleIsPerMillion menegakkan satuan yang paling gampang salah:
// micro-USD per 1.000.000 token. $0.0028/1M harus 2800, BUKAN 0 — pembulatan
// round(usd) menolkan 397 harga dan membuat cache hit gratis tanpa ketahuan.
func TestMicrosScaleIsPerMillion(t *testing.T) {
	ds := Resolve("deepseek-chat", nil)
	if ds.Price.CachedMicrosPer1M != 2800 {
		t.Errorf("deepseek-chat cached = %d, seharusnya 2800 ($0.0028/1M), bukan 0",
			ds.Price.CachedMicrosPer1M)
	}
	if ds.Price.InputMicrosPer1M != 140_000 {
		t.Errorf("deepseek-chat input = %d, seharusnya 140_000 ($0.14/1M)", ds.Price.InputMicrosPer1M)
	}

	opus := Resolve("claude-opus-4-6", nil)
	if opus.Price.InputMicrosPer1M != 5_000_000 {
		t.Errorf("claude-opus-4-6 input = %d, seharusnya 5_000_000 ($5.00/1M)",
			opus.Price.InputMicrosPer1M)
	}
	if opus.Price.OutputMicrosPer1M != 25_000_000 {
		t.Errorf("claude-opus-4-6 output = %d, seharusnya 25_000_000", opus.Price.OutputMicrosPer1M)
	}

	// Tidak ada field yang sumbernya > 0 tapi jadi 0 di Go. Satu-satunya nol yang
	// sah adalah yang memang nol di sumber (`z-ai/glm-5.3-free`).
	mdExact, mdPattern := parseDoc(t)
	check := func(label string, got ModelPrice, cells []string) {
		for i, v := range []int64{got.InputMicrosPer1M, got.OutputMicrosPer1M,
			got.CachedMicrosPer1M, got.ReasoningMicrosPer1M, got.CacheWriteMicrosPer1M} {
			want := microsOf(t, cells[i])
			if want > 0 && v == 0 {
				t.Errorf("%s: harga kolaps jadi nol (sumber %s)", label, cells[i])
			}
		}
	}
	for _, row := range mdExact {
		if got, ok := rawModelPrices[row.key]; ok {
			check("exact "+row.key, got, row.cells)
		}
	}
	for i, row := range mdPattern {
		check("pattern "+row.key, rawPatterns[i].Price, row.cells)
	}
}

// TestReasoningPricedSeparately: `reasoning` punya harga SENDIRI, bukan disamakan
// ke output. Dipakai gemini-3.8-flash-medium karena output 7.5 != reasoning 11.25;
// dengan model yang keduanya sama, test ini tidak membuktikan apa-apa.
func TestReasoningPricedSeparately(t *testing.T) {
	r := Resolve("gemini-3.8-flash-medium", nil)
	if r.Price.OutputMicrosPer1M != 7_500_000 {
		t.Fatalf("output = %d, seharusnya 7_500_000 ($7.50/1M)", r.Price.OutputMicrosPer1M)
	}
	if r.Price.ReasoningMicrosPer1M != 11_250_000 {
		t.Fatalf("reasoning = %d, seharusnya 11_250_000 ($11.25/1M)", r.Price.ReasoningMicrosPer1M)
	}
	if r.Price.ReasoningMicrosPer1M == r.Price.OutputMicrosPer1M {
		t.Fatal("reasoning == output; reasoning tidak dihitung dengan rate sendiri")
	}

	// 1M token reasoning = $11.25, bukan $7.50 (rate output).
	if got := r.Cost(Tokens{Reasoning: 1_000_000}); got != 11_250_000 {
		t.Errorf("cost(reasoning 1M) = %d, seharusnya 11_250_000 (rate output akan memberi 7_500_000)", got)
	}
}

// TestPatternOrderFirstMatchWins: urutan array pattern load-bearing.
// `gpt-5.3-codex-high` cocok ke `*-codex-high` (#2, $8/1M) DAN `gpt-5.3-*` (#22,
// $1.75/1M). Yang lebih awal harus menang.
func TestPatternOrderFirstMatchWins(t *testing.T) {
	early, late := -1, -1
	for i, p := range rawPatterns {
		switch p.Pattern {
		case "*-codex-high":
			early = i
		case "gpt-5.3-*":
			late = i
		}
	}
	if early < 0 || late < 0 {
		t.Fatalf("pattern acuan tidak ditemukan (early=%d, late=%d)", early, late)
	}
	if early >= late {
		t.Fatalf("urutan tabel berubah: #%d harus < #%d", early+1, late+1)
	}

	r := Resolve("gpt-5.3-codex-high", nil)
	if r.PricingModel != "*-codex-high" {
		t.Errorf("pricing_model = %q, seharusnya %q (yang lebih awal menang)", r.PricingModel, "*-codex-high")
	}
	if r.Price.InputMicrosPer1M != 8_000_000 {
		t.Errorf("input = %d, seharusnya 8_000_000 dari pattern #2, bukan 1_750_000 dari #22",
			r.Price.InputMicrosPer1M)
	}
	if r.Source != SourcePattern {
		t.Errorf("price_source = %q, seharusnya %q", r.Source, SourcePattern)
	}
}

// TestFallbackWhenFieldMissing: 84 dari 220 entri exact tidak punya `cache_creation`
// (tanda `—` di docs/PRICING.md). Field itu harus jatuh ke rate INPUT, bukan nol.
func TestFallbackWhenFieldMissing(t *testing.T) {
	// Cari entri exact yang benar-benar absen cache_creation di dokumen, jangan
	// menebak satu nama.
	mdExact, _ := parseDoc(t)
	subject := ""
	for _, row := range mdExact {
		if row.cells[4] == "—" && row.cells[2] != "—" {
			subject = row.key
			break
		}
	}
	if subject == "" {
		t.Fatal("tidak ada entri exact tanpa cache_creation — asumsi test salah")
	}

	if rawModelPrices[subject].CacheWriteMicrosPer1M != missing {
		t.Fatalf("%s: cache_creation harus absen (%d) di tabel mentah", subject, missing)
	}

	r := Resolve(subject, nil)
	if r.Price.CacheWriteMicrosPer1M != r.Price.InputMicrosPer1M {
		t.Errorf("%s: cache_write = %d, seharusnya jatuh ke input %d",
			subject, r.Price.CacheWriteMicrosPer1M, r.Price.InputMicrosPer1M)
	}
	if r.Price.CacheWriteMicrosPer1M == 0 {
		t.Errorf("%s: cache_write jatuh ke NOL, bukan ke input — cache write jadi gratis", subject)
	}

	// 1M token cache write = harga input, bukan nol.
	if got := r.Cost(Tokens{CacheWrite: 1_000_000}); got != r.Price.InputMicrosPer1M {
		t.Errorf("%s: cost(cache_write 1M) = %d, seharusnya %d (harga input)",
			subject, got, r.Price.InputMicrosPer1M)
	}

	// Setelah resolusi, tidak boleh ada field yang masih -1.
	for _, row := range mdExact {
		p := Resolve(row.key, nil).Price
		if p.CachedMicrosPer1M == missing || p.ReasoningMicrosPer1M == missing || p.CacheWriteMicrosPer1M == missing {
			t.Errorf("%s: field absen tidak dijatuhkan ke fallback: %+v", row.key, p)
		}
	}
}

// TestExactBeatsPattern: nama yang ada di tabel exact tidak boleh diambil pattern,
// walaupun pattern juga cocok.
func TestExactBeatsPattern(t *testing.T) {
	// `claude-opus-4.6` ada di exact; pattern `claude-opus-*` juga cocok.
	r := Resolve("claude-opus-4.6", nil)
	if r.Source != SourceCatalog {
		t.Errorf("price_source = %q, seharusnya %q", r.Source, SourceCatalog)
	}
	if r.PricingModel != "claude-opus-4.6" {
		t.Errorf("pricing_model = %q, seharusnya %q", r.PricingModel, "claude-opus-4.6")
	}
	// Exact punya reasoning 37.5; pattern `claude-opus-*` hanya 25.
	if r.Price.ReasoningMicrosPer1M != 37_500_000 {
		t.Errorf("reasoning = %d, seharusnya 37_500_000 dari tabel exact, bukan 25_000_000 dari pattern",
			r.Price.ReasoningMicrosPer1M)
	}
}

// TestProviderPrefixStripped: tingkat 3 — nama tanpa prefix provider harus
// ditemukan di tabel exact; kalau tidak ada, barulah pattern (dengan nama tanpa
// prefix, sama seperti `b.split("/").pop()` di 9Router).
func TestProviderPrefixStripped(t *testing.T) {
	// `claude-sonnet-5` tidak ada di exact, `anthropic/claude-sonnet-5` ada.
	if _, ok := rawModelPrices["claude-sonnet-5"]; ok {
		t.Fatal("claude-sonnet-5 ada di exact — test ini kehilangan maknanya")
	}
	if _, ok := rawModelPrices["anthropic/claude-sonnet-5"]; !ok {
		t.Fatal("anthropic/claude-sonnet-5 tidak ada di exact — asumsi test salah")
	}

	r := Resolve("claude-sonnet-5", nil)
	if r.Source != SourcePattern {
		t.Errorf("price_source = %q, seharusnya %q", r.Source, SourcePattern)
	}
	if r.PricingModel != "claude-sonnet-*" {
		t.Errorf("pricing_model = %q, seharusnya %q", r.PricingModel, "claude-sonnet-*")
	}

	// Kunci exact ber-prefix tetap dicocokkan apa adanya.
	e := Resolve("anthropic/claude-sonnet-5", nil)
	if e.Source != SourceCatalog {
		t.Errorf("price_source = %q, seharusnya %q", e.Source, SourceCatalog)
	}
	if e.Price.InputMicrosPer1M != 2_000_000 {
		t.Errorf("anthropic/claude-sonnet-5 input = %d, seharusnya 2_000_000 (entri exact ber-prefix)",
			e.Price.InputMicrosPer1M)
	}
}

// TestUnpricedModel: model tak dikenal -> biaya nol TAPI ditandai `unpriced`,
// supaya UI bisa bilang "tidak diketahui", bukan "gratis".
func TestUnpricedModel(t *testing.T) {
	r := Resolve("acme-mystery-9000", nil)
	if r.Source != SourceUnpriced {
		t.Errorf("price_source = %q, seharusnya %q", r.Source, SourceUnpriced)
	}
	if got := r.Cost(Tokens{In: 1_000_000, Out: 1_000_000, Cached: 500, Reasoning: 500}); got != 0 {
		t.Errorf("cost = %d, seharusnya 0", got)
	}
	if r.PricingModel != "acme-mystery-9000" {
		t.Errorf("pricing_model = %q, seharusnya nama model yang dicoba", r.PricingModel)
	}
}

// TestCostSingleRounding memakai nilai eksak yang sudah diverifikasi manual.
// claude-opus-4-6: input $5, output $25, cached $0.50, reasoning $25, cache_write $6.25.
//
//	miss = 1000 - 400 - 100 = 500
//	500×5e6 + 400×0.5e6 + 200×25e6 + 150×25e6 + 100×6.25e6 = 12_075_000_000
//	/ 1e6 = 12_075 micro-USD = $0.012075
func TestCostSingleRounding(t *testing.T) {
	r := Resolve("claude-opus-4-6", nil)
	got := r.Cost(Tokens{In: 1000, Cached: 400, Out: 200, Reasoning: 150, CacheWrite: 100})
	if got != 12075 {
		t.Fatalf("cost_micros = %d, seharusnya 12075 ($0.012075)", got)
	}
	if r.Source != SourceCatalog || r.PricingModel != "claude-opus-4-6" {
		t.Errorf("sumber audit salah: %q / %q", r.Source, r.PricingModel)
	}
}

// TestMissNeverNegative: cache read + write melebihi prompt tidak boleh membuat
// komponen miss negatif (yang akan memotong biaya).
func TestMissNeverNegative(t *testing.T) {
	r := Resolve("claude-opus-4-6", nil)
	got := r.Cost(Tokens{In: 100, Cached: 10_000, CacheWrite: 10_000})
	if got < 0 {
		t.Errorf("cost negatif: %d", got)
	}
	// Hanya komponen cached + cache_write yang dihitung: 10_000×0.5e6 + 10_000×6.25e6 = 67_500_000_000 /1e6
	if got != 67_500 {
		t.Errorf("cost = %d, seharusnya 67_500", got)
	}
}

// TestManualOverride: override org menang atas seluruh tabel (tingkat 1), dan
// override kosong diabaikan — struct nol akan menagih nol ke semua model.
func TestManualOverride(t *testing.T) {
	ov := ModelPrice{
		PriceVersion:      PriceVersion,
		Provider:          "openai_compatible",
		Model:             "claude-opus-4-6",
		InputMicrosPer1M:  1_000_000,
		OutputMicrosPer1M: 2_000_000,
	}
	r := Resolve("claude-opus-4-6", &ov)
	if r.Source != SourceManual {
		t.Errorf("price_source = %q, seharusnya %q", r.Source, SourceManual)
	}
	if got := r.Cost(Tokens{In: 1_000_000}); got != 1_000_000 {
		t.Errorf("cost = %d, seharusnya 1_000_000 dari override", got)
	}

	empty := Resolve("claude-opus-4-6", &ModelPrice{})
	if empty.Source != SourceCatalog {
		t.Errorf("override kosong dipakai (source=%q) — seharusnya jatuh ke katalog", empty.Source)
	}
}

// TestGlobCaseInsensitive: 9Router memakai RegExp(..., "i"), jadi `MiniMax-*` dan
// `minimax-*` sama-sama kena nama ber-campur huruf besar-kecil.
func TestGlobCaseInsensitive(t *testing.T) {
	for _, name := range []string{"MiniMax-M3", "minimax-m3", "MINIMAX-M3"} {
		if !globMatch("minimax-*", name) {
			t.Errorf("globMatch(minimax-*, %q) = false, seharusnya true", name)
		}
	}
	for _, c := range []struct{ pattern, name string }{
		{"minimax-*", "acme-m3"},         // prefix salah
		{"*-codex", "codex-high-x"},      // suffix salah
		{"claude-opus-*", "claude-opus"}, // `*` boleh kosong, tapi `-` wajib
		{"deepseek-*", "deepseek"},       // ditto
		{"gpt-4o", "gpt-4o-mini"},        // tanpa `*` harus exact
	} {
		if globMatch(c.pattern, c.name) {
			t.Errorf("globMatch(%q, %q) = true, seharusnya false", c.pattern, c.name)
		}
	}
	if !globMatch("claude-opus-*", "claude-opus-4.6") {
		t.Error("globMatch(claude-opus-*, claude-opus-4.6) = false, seharusnya true")
	}
}

// TestCacheHitNotFree: cache hit harus lebih murah dari miss, tapi BUKAN nol.
func TestCacheHitNotFree(t *testing.T) {
	r := Resolve("deepseek-chat", nil)
	hit := r.Cost(Tokens{In: 1_000_000, Cached: 1_000_000})
	miss := r.Cost(Tokens{In: 1_000_000})
	if hit == 0 {
		t.Fatal("cache hit gratis — bug 1000x/1e6 kembali")
	}
	if hit >= miss {
		t.Errorf("cache hit (%d) tidak lebih murah dari miss (%d)", hit, miss)
	}
}
