"""Hasilkan docs/PRICING.md + internal/pricing/table_gen.go dari tabel harga 9Router.

Kedua output datang dari SATU pembacaan sumber yang sama, jadi tabel MD dan tabel Go
tidak bisa berbeda.

Tabel harga AgentDeck di-port dari 9Router (MIT License). Sumbernya ada di build
chunk aplikasi 9Router, bukan di file JSON mana pun — katalog yang ada
(`model-catalog.json`) hanya berisi kapabilitas (vision/pdf/context), TANPA harga.

Jalankan:
    python tools/gen_pricing.py                 # pakai lokasi instalasi default
    python tools/gen_pricing.py <path-chunk>    # bila 9Router dipasang di tempat lain

`<path-chunk>` adalah file `8920.js` di dalam `.next-cli-build/server/chunks/`.
"""

import json
import os
import re
import sys

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

DEFAULT_CHUNK = os.path.join(
    os.environ.get("APPDATA", ""),
    "npm",
    "node_modules",
    "9router",
    "app",
    ".next-cli-build",
    "server",
    "chunks",
    "8920.js",
)

FIELDS = ["input", "output", "cached", "reasoning", "cache_creation"]

# Nama field sumber 9Router -> nama field ModelPrice di Go (ARCHITECTURE §9.1).
GO_FIELD = {
    "input": "InputMicrosPer1M",
    "output": "OutputMicrosPer1M",
    "cached": "CachedMicrosPer1M",
    "reasoning": "ReasoningMicrosPer1M",
    "cache_creation": "CacheWriteMicrosPer1M",
}


def parse_chunk(path):
    """Ambil tabel exact + pattern dari source 9Router."""
    src = open(path, encoding="utf-8", errors="replace").read()

    # Tabel exact: `let d={...}` — diakhiri tepat sebelum `,f=[` (array pattern).
    i = src.find("let d={")
    if i < 0:
        raise SystemExit("FAIL: tabel exact (`let d={`) tidak ditemukan di chunk")
    j = src.find(",f=[", i)
    if j < 0:
        raise SystemExit("FAIL: awal tabel pattern (`f=[`) tidak ditemukan")
    exact_src = src[i + len("let d={") : j]

    # Array pattern: dari `f=[` sampai `];` penutup.
    k = j + len(",f=[")
    m = re.search(r"\];", src[k:])
    if not m:
        raise SystemExit("FAIL: akhir tabel pattern (`];`) tidak ditemukan")
    pat_src = src[k : k + m.start()]

    exact = {}
    for mm in re.finditer(r'(?:"([^"]+)"|([A-Za-z_][\w.]*))\s*:\s*\{([^}]*)\}', exact_src):
        name = mm.group(1) or mm.group(2)
        exact[name] = {
            kk: float(vv) for kk, vv in re.findall(r"(\w+)\s*:\s*([\d.]+)", mm.group(3))
        }

    pats = []
    for mm in re.finditer(
        r'pattern\s*:\s*"([^"]+)"\s*,\s*pricing\s*:\s*\{([^}]*)\}', pat_src
    ):
        pats.append(
            {
                "pattern": mm.group(1),
                "pricing": {
                    kk: float(vv)
                    for kk, vv in re.findall(r"(\w+)\s*:\s*([\d.]+)", mm.group(2))
                },
            }
        )

    return exact, pats


def fmt(v):
    """Cetak angka harga tanpa nol berekor; None jadi '—' (artinya jatuh ke fallback)."""
    if v is None:
        return "—"
    s = f"{v:.6f}".rstrip("0").rstrip(".")
    return s if s else "0"


def micros(v):
    """USD per 1M token -> micro-USD per 1M token. $5.00 -> 5_000_000; $0.0028 -> 2800.

    BUKAN round(usd): pembulatan itu menolkan 397 harga (mis. `deepseek-*` cached
    $0.0028/1M) sehingga cache hit jadi gratis tanpa ketahuan.
    """
    if v is None:
        return -1  # -1 = field tidak ada di sumber; resolver yang menjatuhkannya ke fallback
    return int(round(v * 1_000_000))


def provider_of(name):
    """Prefix provider dari kunci tabel (`anthropic/claude-opus-4.6` -> `anthropic`)."""
    return name.split("/")[0] if "/" in name else ""


def row(name, price, code=True):
    cells = [f"`{name}`" if code else name] + [fmt(price.get(f)) for f in FIELDS]
    return "| " + " | ".join(cells) + " |"


def render(exact, pats):
    L = []
    A = L.append
    A("# PRICING — Tabel harga model (port 9Router)")
    A("")
    A("> **Dihasilkan otomatis** dari `tools/gen_pricing.py`. Jangan diedit tangan.")
    A("> Sumber: 9Router (MIT License) — `app/.next-cli-build/server/chunks/8920.js`,")
    A("> fungsi `getPricingForModel` + `calculateCostFromTokens`.")
    A("")
    A("Dokumen ini adalah **isi** tabel harga yang dirujuk `DECISIONS.md` §6A dan")
    A("`ARCHITECTURE.md` §9.1. Angka di sini yang dipindahkan ke `internal/pricing`.")
    A("")
    A(f"- Entri **exact**: **{len(exact)}**")
    A(f"- Entri **pattern**: **{len(pats)}** (urut — **first match wins**)")
    A("- Satuan: **USD per 1 juta token**")
    A("- `price_version` = **1**")
    A("")
    A("## Aturan pembacaan")
    A("")
    A("1. Resolusi 4 tingkat (DECISIONS §6A.C): override org → exact → pattern → `unpriced`.")
    A("2. Pencocokan nama: entri exact dicocokkan **apa adanya**, lalu **tanpa prefix provider**")
    A("   (`anthropic/claude-sonnet-5` → `claude-sonnet-5`). Pattern memakai wildcard `*`.")
    A("3. Tanda `—` berarti field itu **tidak ada** dan **jatuh ke fallback**, bukan nol:")
    A("   `cached ?? input` · `reasoning ?? output` · `cache_creation ?? input`.")
    A("4. Satuan internal Go: **micro-USD per 1.000.000 token** — `MicrosPer1M = round(usd_per_1m × 1e6)`,")
    A("   jadi `$5.00/1M → 5_000_000`. **Bukan** `round(usd_per_1m)`: pembulatan itu menolkan 397 harga")
    A("   (`deepseek-*` cached `$0.0028/1M` → `0`), sehingga cache hit jadi gratis tanpa ketahuan.")
    A("")
    A("## Rumus biaya (5 komponen)")
    A("")
    A("```")
    A("miss = max(0, prompt_tokens - cached_tokens - cache_creation_tokens)")
    A("cost = miss           * input/1e6")
    A("     + cached_tokens  * (cached         ?? input)/1e6")
    A("     + completion     * output/1e6")
    A("     + reasoning      * (reasoning      ?? output)/1e6")
    A("     + cache_creation * (cache_creation ?? input)/1e6")
    A("```")
    A("")
    A("Terverifikasi eksak terhadap data produksi 9Router: `max_err = 0.0000000000`")
    A("pada 13.140 baris `usageHistory`.")
    A("")
    A("---")
    A("")
    A(f"## 1. Entri exact ({len(exact)})")
    A("")
    A("| Model | input | output | cached | reasoning | cache_creation |")
    A("|---|---|---|---|---|---|")
    for k in sorted(exact):
        A(row(k, exact[k]))
    A("")
    A("---")
    A("")
    A(f"## 2. Entri pattern ({len(pats)}) — URUT, first match wins")
    A("")
    A("| # | Pattern | input | output | cached | reasoning | cache_creation |")
    A("|---|---|---|---|---|---|---|")
    for i, p in enumerate(pats, 1):
        A("| " + str(i) + " | " + row(p["pattern"], p["pricing"], code=False)[2:])
    A("")
    return "\n".join(L)


def go_str(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def go_price(p):
    """Literal ModelPrice ber-key. Keyed (bukan posisional) supaya urutan field struct
    tidak pernah bisa menggeser harga diam-diam."""
    return "{" + ", ".join(
        [
            f"PriceVersion: {p.get('price_version', 1)}",
            f"Provider: {go_str(p.get('provider', ''))}",
            f"Model: {go_str(p.get('model', ''))}",
        ]
        + [
            f"{GO_FIELD[f]}: {go_int(micros(p.get(f)))}"
            for f in FIELDS
        ]
    ) + "}"


def go_int(v):
    return f"{v:,}".replace(",", "_")


def render_go(exact, pats):
    """Tabel Go — dibaca dari parse yang SAMA dengan docs/PRICING.md, jadi tak bisa beda."""
    L = []
    A = L.append
    A("// Code generated by tools/gen_pricing.py. DO NOT EDIT.")
    A("//")
    A("// Sumber: 9Router (MIT License) app/.next-cli-build/server/chunks/8920.js")
    A("// (getPricingForModel + calculateCostFromTokens). Isi yang sama persis dengan")
    A("// docs/PRICING.md — keduanya keluar dari satu pembacaan sumber.")
    A("//")
    A("// Satuan: micro-USD per 1.000.000 token. MicrosPer1M = round(usd_per_1m * 1e6),")
    A("// jadi $5.00/1M -> 5_000_000 dan $0.0028/1M -> 2800. BUKAN round(usd_per_1m):")
    A("// pembulatan itu menolkan 397 harga sehingga cache hit jadi gratis.")
    A("//")
    A("// Field bernilai -1 = field itu TIDAK ADA di sumber (tanda `—` di")
    A("// docs/PRICING.md). Resolver yang menjatuhkannya ke fallback, bukan nol:")
    A("// cached ?? input, reasoning ?? output, cache_creation ?? input.")
    A("")
    A("package pricing")
    A("")
    A(f"// rawModelPrices — {len(exact)} entri exact, dicocokkan apa adanya.")
    A("var rawModelPrices = map[string]ModelPrice{")
    # Rata-kan kolom key seperti gofmt, supaya `gofmt -l .` bersih tanpa perlu
    # menjalankan gofmt dari generator.
    keys = sorted(exact)
    w = max(len(go_str(k)) + 1 for k in keys)
    for k in keys:
        p = dict(exact[k])
        p["provider"] = provider_of(k)
        p["model"] = k
        label = go_str(k) + ":"
        A(f"\t{label.ljust(w)} {go_price(p)},")
    A("}")
    A("")
    A(f"// rawPatterns — {len(pats)} pattern, URUT, first match wins.")
    A("var rawPatterns = []PricePattern{")
    for p in pats:
        pr = dict(p["pricing"])
        pr["model"] = p["pattern"]
        A(f"\t{{Pattern: {go_str(p['pattern'])}, Price: ModelPrice{go_price(pr)}}},")
    A("}")
    A("")
    return "\n".join(L)



def main(argv):
    chunk = argv[1] if len(argv) > 1 else DEFAULT_CHUNK
    if not os.path.isfile(chunk):
        print(f"FAIL: chunk 9Router tidak ditemukan di {chunk}")
        print("      Berikan path ke `8920.js` sebagai argumen pertama.")
        return 1

    exact, pats = parse_chunk(chunk)

    # Angka ini diklaim ARCHITECTURE §9.1 + DECISIONS §6A dan digate verify_suite.py.
    # Kalau sumber 9Router berubah, dokumen + test harus diubah dulu — bukan diam-diam.
    if len(exact) != 220 or len(pats) != 51:
        print(f"FAIL: sumber berisi {len(exact)} exact + {len(pats)} pattern, "
              "bukan 220 + 51 seperti yang diklaim ARCHITECTURE.md §9.1 / DECISIONS.md §6A.")
        print("      Perbarui kedua dokumen itu (dan test internal/pricing) lebih dulu.")
        return 1

    outs = [
        (os.path.join(HERE, "docs", "PRICING.md"), render(exact, pats)),
        (os.path.join(HERE, "internal", "pricing", "table_gen.go"), render_go(exact, pats)),
    ]
    for path, text in outs:
        os.makedirs(os.path.dirname(path), exist_ok=True)
        open(path, "w", encoding="utf-8", newline="\n").write(text)
        print(f"-> {os.path.relpath(path, HERE)}")

    print(f"exact   : {len(exact)}")
    print(f"pattern : {len(pats)}")
    return 0



if __name__ == "__main__":
    sys.exit(main(sys.argv))
