# PRICING — Tabel harga model (port 9Router)

> **Dihasilkan otomatis** dari `tools/gen_pricing.py`. Jangan diedit tangan.
> Sumber: 9Router (MIT License) — `app/.next-cli-build/server/chunks/8920.js`,
> fungsi `getPricingForModel` + `calculateCostFromTokens`.

Dokumen ini adalah **isi** tabel harga yang dirujuk `DECISIONS.md` §6A dan
`ARCHITECTURE.md` §9.1. Angka di sini yang dipindahkan ke `internal/pricing`.

- Entri **exact**: **220**
- Entri **pattern**: **51** (urut — **first match wins**)
- Satuan: **USD per 1 juta token**
- `price_version` = **1**

## Aturan pembacaan

1. Resolusi 4 tingkat (DECISIONS §6A.C): override org → exact → pattern → `unpriced`.
2. Pencocokan nama: entri exact dicocokkan **apa adanya**, lalu **tanpa prefix provider**
   (`anthropic/claude-sonnet-5` → `claude-sonnet-5`). Pattern memakai wildcard `*`.
3. Tanda `—` berarti field itu **tidak ada** dan **jatuh ke fallback**, bukan nol:
   `cached ?? input` · `reasoning ?? output` · `cache_creation ?? input`.
4. Satuan internal Go: **micro-USD per 1.000.000 token** — `MicrosPer1M = round(usd_per_1m × 1e6)`,
   jadi `$5.00/1M → 5_000_000`. **Bukan** `round(usd_per_1m)`: pembulatan itu menolkan 397 harga
   (`deepseek-*` cached `$0.0028/1M` → `0`), sehingga cache hit jadi gratis tanpa ketahuan.

## Rumus biaya (5 komponen)

```
miss = max(0, prompt_tokens - cached_tokens - cache_creation_tokens)
cost = miss           * input/1e6
     + cached_tokens  * (cached         ?? input)/1e6
     + completion     * output/1e6
     + reasoning      * (reasoning      ?? output)/1e6
     + cache_creation * (cache_creation ?? input)/1e6
```

Terverifikasi eksak terhadap data produksi 9Router: `max_err = 0.0000000000`
pada 13.140 baris `usageHistory`.

---

## 1. Entri exact (220)

| Model | input | output | cached | reasoning | cache_creation |
|---|---|---|---|---|---|
| `MiniMax-M2.1` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `MiniMax-M2.5` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `MiniMax-M2.7` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `MiniMax-M3` | 0.3 | 1.2 | 0.06 | 1.8 | 0.3 |
| `anthropic/claude-fable-5` | 10 | 50 | 1 | 50 | 12.5 |
| `anthropic/claude-haiku-4.5` | 1 | 5 | 0.1 | 5 | 1.25 |
| `anthropic/claude-opus-4.5` | 5 | 25 | 0.5 | 25 | 6.25 |
| `anthropic/claude-opus-4.6` | 5 | 25 | 0.5 | 25 | 6.25 |
| `anthropic/claude-opus-4.7` | 5 | 25 | 0.5 | 25 | 6.25 |
| `anthropic/claude-opus-4.7-fast` | 30 | 150 | 3 | 150 | — |
| `anthropic/claude-opus-4.8` | 5 | 25 | 0.5 | 25 | 6.25 |
| `anthropic/claude-opus-4.8-fast` | 10 | 50 | 1 | 50 | 12.5 |
| `anthropic/claude-opus-5` | 5 | 25 | 0.5 | 25 | 6.25 |
| `anthropic/claude-opus-5-fast` | 10 | 50 | 1 | 50 | 12.5 |
| `anthropic/claude-sonnet-4` | 3 | 15 | 0.3 | 15 | 3.75 |
| `anthropic/claude-sonnet-4.5` | 3 | 15 | 0.3 | 15 | 3.75 |
| `anthropic/claude-sonnet-4.6` | 3 | 15 | 0.3 | 15 | 3.75 |
| `anthropic/claude-sonnet-5` | 2 | 10 | 0.2 | 10 | — |
| `auto` | 2 | 8 | 1 | 12 | 2 |
| `claude-3-5-sonnet-20241022` | 3 | 15 | 1.5 | 15 | 3 |
| `claude-fable-5` | 10 | 50 | 1 | 50 | 12.5 |
| `claude-haiku-4-5-20251001` | 1 | 5 | 0.1 | 5 | 1.25 |
| `claude-haiku-4.5` | 0.5 | 2.5 | 0.05 | 3.75 | 0.5 |
| `claude-opus-4-20250514` | 15 | 25 | 7.5 | 112.5 | 15 |
| `claude-opus-4-5-20251101` | 5 | 25 | 0.5 | 25 | 6.25 |
| `claude-opus-4-5-thinking` | 5 | 25 | 0.5 | 37.5 | 5 |
| `claude-opus-4-6` | 5 | 25 | 0.5 | 25 | 6.25 |
| `claude-opus-4-6-thinking` | 5 | 25 | 0.5 | 37.5 | 5 |
| `claude-opus-4-8-m-aws` | 5 | 25 | 0.5 | 25 | 6.25 |
| `claude-opus-4.1` | 5 | 25 | 0.5 | 37.5 | 5 |
| `claude-opus-4.5` | 5 | 25 | 0.5 | 37.5 | 5 |
| `claude-opus-4.6` | 5 | 25 | 0.5 | 37.5 | 5 |
| `claude-sonnet-4` | 3 | 15 | 0.3 | 22.5 | 3 |
| `claude-sonnet-4-20250514` | 3 | 15 | 1.5 | 15 | 3 |
| `claude-sonnet-4-5-20250929` | 3 | 15 | 0.3 | 15 | 3.75 |
| `claude-sonnet-4-6` | 3 | 15 | 0.3 | 15 | 3.75 |
| `claude-sonnet-4.5` | 3 | 15 | 0.3 | 22.5 | 3 |
| `claude-sonnet-4.6` | 3 | 15 | 0.3 | 22.5 | 3 |
| `coder-model` | 1.5 | 6 | 0.75 | 9 | 1.5 |
| `deepseek-chat` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek-flash` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek-r1` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek-reasoner` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek-v3.2-chat` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek-v3.2-reasoner` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek-v4-flash` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek-v4-pro` | 0.435 | 0.87 | 0.003625 | 0.87 | 0.435 |
| `deepseek-v4.1-flash` | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| `deepseek/deepseek-v3.2` | 0.26 | 0.38 | 0.13 | 0.38 | — |
| `deepseek/deepseek-v4-flash` | 0.14 | 0.28 | 0.0028 | 0.28 | — |
| `deepseek/deepseek-v4-flash-0731` | 0.14 | 0.28 | 0.0028 | 0.28 | — |
| `deepseek/deepseek-v4-pro` | 0.435 | 0.87 | 0.003625 | 0.87 | — |
| `ex/gpt-5.4` | 2.5 | 15 | 0.25 | 15 | — |
| `gemini-2.5-flash` | 0.3 | 2.5 | 0.03 | 3.75 | 0.3 |
| `gemini-2.5-flash-lite` | 0.15 | 1.25 | 0.015 | 1.875 | 0.15 |
| `gemini-2.5-pro` | 2 | 12 | 0.25 | 18 | 2 |
| `gemini-3-flash` | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| `gemini-3-flash-agent` | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| `gemini-3-flash-preview` | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| `gemini-3-pro-preview` | 2 | 12 | 0.25 | 18 | 2 |
| `gemini-3.1-pro-high` | 4 | 18 | 0.5 | 27 | 4 |
| `gemini-3.1-pro-low` | 2 | 12 | 0.25 | 18 | 2 |
| `gemini-3.5-flash-extra-low` | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| `gemini-3.5-flash-high` | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| `gemini-3.5-flash-lite` | 0.3 | 2.5 | 0.03 | 3.75 | 0.375 |
| `gemini-3.5-flash-low` | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| `gemini-3.6-flash` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.6-flash-high` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.6-flash-low` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.6-flash-medium` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.7-flash` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.7-flash-high` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.7-flash-low` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.7-flash-medium` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.8-flash` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.8-flash-high` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.8-flash-low` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-3.8-flash-medium` | 1.5 | 7.5 | 0.15 | 11.25 | 1.875 |
| `gemini-pro-agent` | 4 | 18 | 0.5 | 27 | 4 |
| `gh` | 1.75 | 14 | 0.175 | 14 | 1.75 |
| `glm-4.6` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `glm-4.6v` | 0.75 | 3 | 0.375 | 4.5 | 0.75 |
| `glm-4.7` | 0.75 | 3 | 0.375 | 4.5 | 0.75 |
| `glm-5` | 1 | 4 | 0.5 | 6 | 1 |
| `google/gemini-2.5-flash-image` | 0.3 | 2.5 | — | 2.5 | — |
| `google/gemini-3-flash-preview` | 0.5 | 3 | 0.05 | 3 | 0.08333 |
| `google/gemini-3-pro-image-preview` | 2 | 12 | — | 12 | — |
| `google/gemini-3.1-flash-image-preview` | 0.5 | 3 | — | 3 | — |
| `google/gemini-3.1-flash-lite-image` | 0.25 | 1.5 | — | 1.5 | — |
| `google/gemini-3.1-pro-preview` | 2 | 12 | 0.2 | 12 | 0.375 |
| `google/gemini-3.5-flash` | 1.5 | 9 | 0.15 | 9 | 0.08333 |
| `google/gemini-3.5-flash-lite` | 0.3 | 2.5 | 0.03 | 2.5 | 0.08333 |
| `google/gemini-3.6-flash` | 1.5 | 7.5 | 0.15 | 7.5 | 0.08333 |
| `google/gemini-embedding-2` | 1 | 6 | 0.1 | 6 | — |
| `google/gemma-4-26b-a4b-it` | 0.06 | 0.33 | — | 0.33 | — |
| `gpt-3.5-turbo` | 0.5 | 1.5 | 0.25 | 2.25 | 0.5 |
| `gpt-4` | 2.5 | 10 | 1.25 | 15 | 2.5 |
| `gpt-4-turbo` | 10 | 30 | 5 | 45 | 10 |
| `gpt-4.1` | 2.5 | 10 | 1.25 | 15 | 2.5 |
| `gpt-4o` | 2.5 | 10 | 1.25 | 15 | 2.5 |
| `gpt-4o-mini` | 0.15 | 0.6 | 0.075 | 0.9 | 0.15 |
| `gpt-5` | 1.25 | 10 | 0.625 | 10 | 1.25 |
| `gpt-5-codex` | 1.25 | 10 | 0.625 | 10 | 1.25 |
| `gpt-5-mini` | 0.25 | 2 | 0.125 | 2 | 0.25 |
| `gpt-5.1` | 1.25 | 10 | 0.625 | 10 | 1.25 |
| `gpt-5.1-codex` | 1.25 | 10 | 0.625 | 10 | 1.25 |
| `gpt-5.1-codex-max` | 8 | 32 | 4 | 48 | 8 |
| `gpt-5.1-codex-mini` | 1.5 | 6 | 0.75 | 9 | 1.5 |
| `gpt-5.1-codex-mini-high` | 2 | 8 | 1 | 12 | 2 |
| `gpt-5.2` | 1.75 | 14 | 0.175 | 14 | 1.75 |
| `gpt-5.2-codex` | 1.75 | 14 | 0.175 | 14 | 1.75 |
| `gpt-5.3-codex` | 1.75 | 14 | 0.175 | 14 | 1.75 |
| `gpt-5.3-codex-spark` | 3 | 12 | 0.3 | 12 | 3 |
| `gpt-5.6` | 2.5 | 15 | 0.25 | 15 | 2.5 |
| `gpt-5.6-luna` | 1 | 6 | 0.1 | 6 | 1 |
| `gpt-5.6-sol` | 5 | 30 | 0.5 | 30 | 5 |
| `gpt-5.6-terra` | 2.5 | 15 | 0.25 | 15 | 2.5 |
| `gpt-6-astra` | 5 | 30 | 0.5 | 30 | 5 |
| `gpt-oss-120b-medium` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `grok-code-fast-1` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `k3` | 3 | 15 | 0.3 | 15 | 3 |
| `kimi-for-coding` | 0.95 | 4 | 0.19 | 4 | 0.95 |
| `kimi-for-coding-highspeed` | 1.9 | 8 | 0.38 | 8 | 1.9 |
| `kimi-k2` | 1 | 4 | 0.5 | 6 | 1 |
| `kimi-k2-thinking` | 1.5 | 6 | 0.75 | 9 | 1.5 |
| `kimi-k2.5` | 1.2 | 4.8 | 0.6 | 7.2 | 1.2 |
| `kimi-k2.5-thinking` | 1.8 | 7.2 | 0.9 | 10.8 | 1.8 |
| `kimi-k2.6` | 1 | 4 | 0.5 | 6 | 1 |
| `kimi-k2.7-code` | 0.95 | 4 | 0.19 | 4 | 0.95 |
| `kimi-k2.7-code-highspeed` | 1.9 | 8 | 0.38 | 8 | 1.9 |
| `kimi-k3` | 3 | 15 | 0.3 | 15 | 3 |
| `kimi-latest` | 1 | 4 | 0.5 | 6 | 1 |
| `kling-3.0-turbo` | 2.1 | 2.1 | — | 2.1 | — |
| `microsoft/mai-image-2.5` | 5 | 47 | — | 47 | — |
| `minimax-m2.1` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `minimax-m2.5` | 0.6 | 2.4 | 0.3 | 3.6 | 0.6 |
| `minimax/minimax-m2-her` | 0.3 | 1.2 | 0.03 | 1.2 | — |
| `minimax/minimax-m2.1` | 0.3 | 1.2 | 0.03 | 1.2 | — |
| `minimax/minimax-m2.1-highspeed` | 0.6 | 2.4 | 0.06 | 2.4 | — |
| `minimax/minimax-m2.5` | 0.3 | 1.2 | 0.03 | 1.2 | — |
| `minimax/minimax-m2.7` | 0.3 | 1.2 | 0.06 | 1.2 | — |
| `minimax/minimax-m2.7-highspeed` | 0.6 | 2.4 | 0.06 | 2.4 | — |
| `miromind/mirothinker-1-7-deepresearch` | 4 | 25 | — | 25 | — |
| `miromind/mirothinker-1-7-deepresearch-mini` | 1.25 | 10 | — | 10 | — |
| `mistralai/devstral-2512` | 0.4 | 2 | 0.04 | 2 | — |
| `mistralai/mistral-medium-3-5` | 1.5 | 7.5 | — | 7.5 | — |
| `mistralai/mistral-small-2603` | 0.15 | 0.6 | 0.015 | 0.6 | — |
| `mistralai/voxtral-small-24b-2507` | 0.1 | 0.3 | 0.01 | 0.3 | — |
| `moonshotai/kimi-k2.5` | 0.6 | 3 | 0.1 | 3 | — |
| `moonshotai/kimi-k2.6` | 0.95 | 4 | 0.16 | 4 | — |
| `moonshotai/kimi-k2.7-code` | 0.9286 | 3.8571 | 0.1857 | 3.8571 | — |
| `moonshotai/kimi-k3` | 3 | 15 | 0.3 | 15 | — |
| `nvidia/nemotron-3-super-120b-a12b` | 0.3 | 0.9 | 0.1 | 0.9 | — |
| `o1` | 15 | 60 | 7.5 | 90 | 15 |
| `o1-mini` | 3 | 12 | 1.5 | 18 | 3 |
| `openai/gpt-4o-mini` | 0.15 | 0.6 | 0.075 | 0.6 | — |
| `openai/gpt-5` | 1.25 | 10 | 0.125 | 10 | — |
| `openai/gpt-5-image` | 10 | 40 | 2.5 | 40 | — |
| `openai/gpt-5-image-mini` | 2.5 | 8 | 0.25 | 8 | — |
| `openai/gpt-5-mini` | 0.25 | 2 | 0.025 | 2 | — |
| `openai/gpt-5.2` | 1.75 | 14 | 0.175 | 14 | — |
| `openai/gpt-5.3-codex` | 1.75 | 14 | 0.175 | 14 | — |
| `openai/gpt-5.4` | 2.5 | 15 | 0.25 | 15 | — |
| `openai/gpt-5.4-image-2` | 8 | 30 | 2 | 30 | — |
| `openai/gpt-5.4-mini` | 0.75 | 4.5 | 0.075 | 4.5 | — |
| `openai/gpt-5.4-nano` | 0.2 | 1.25 | 0.02 | 1.25 | — |
| `openai/gpt-5.4-pro` | 30 | 180 | — | 180 | — |
| `openai/gpt-5.5` | 5 | 30 | 0.5 | 30 | — |
| `openai/gpt-5.5-pro` | 30 | 180 | — | 180 | — |
| `openai/gpt-5.6-luna` | 0.2 | 1.2 | 0.02 | 1.2 | 0.25 |
| `openai/gpt-5.6-sol` | 5 | 30 | 0.5 | 30 | 6.25 |
| `openai/gpt-5.6-terra` | 2 | 12 | 0.2 | 12 | 2.5 |
| `openai/gpt-audio` | 2.5 | 10 | — | 10 | — |
| `openai/gpt-audio-mini` | 0.6 | 2.4 | — | 2.4 | — |
| `openai/gpt-oss-120b` | 0.039 | 0.18 | — | 0.18 | — |
| `oswe-vscode-prime` | 1 | 4 | 0.5 | 6 | 1 |
| `qwen/qwen3-coder-next` | 0.12 | 0.75 | 0.06 | 0.75 | — |
| `qwen/qwen3.5-122b-a10b` | 0.26 | 2.08 | — | 2.08 | — |
| `qwen/qwen3.5-35b-a3b` | 0.1625 | 1.3 | — | 1.3 | — |
| `qwen/qwen3.5-397b-a17b` | 0.39 | 2.34 | — | 2.34 | — |
| `qwen/qwen3.5-9b` | 0.1 | 0.15 | — | 0.15 | — |
| `qwen/qwen3.5-flash` | 0.1048 | 0.4194 | — | 0.4194 | — |
| `qwen/qwen3.5-plus-02-15` | 0.26 | 1.56 | — | 1.56 | — |
| `qwen/qwen3.6-plus` | 0.54 | 3.21 | — | 3.21 | — |
| `qwen/qwen3.7-max` | 1.25 | 3.75 | 0.25 | 3.75 | — |
| `qwen/qwen3.7-plus` | 0.4 | 1.6 | 0.08 | 1.6 | — |
| `qwen/qwen3.8-max` | 2 | 6 | 0.25 | 6 | 2.5 |
| `qwen3-coder-flash` | 0.5 | 2 | 0.25 | 3 | 0.5 |
| `qwen3-coder-plus` | 1 | 4 | 0.5 | 6 | 1 |
| `qwen3.5-omni-plus` | 1 | 5.7143 | — | 5.7143 | — |
| `qwen3.6-flash` | 0.171 | 1.029 | 0.017 | 1.029 | 0.214 |
| `sakana/fugu-ultra` | 5 | 30 | 0.5 | 30 | — |
| `seed-2-0-code-preview-260328` | 1 | 6 | 0.2 | 6 | 0.008333 |
| `seed-2-0-lite-260428` | 0.5 | 4 | 0.1 | 4 | 0.008333 |
| `seed-2-0-mini-260428` | 0.2 | 0.8 | 0.04 | 0.8 | 0.00833 |
| `seed-2-0-pro-260328` | 1 | 6 | 0.2 | 6 | 0.008333 |
| `stepfun/step-3.5-flash` | 0.1 | 0.3 | 0.02 | 0.3 | — |
| `stepfun/step-3.7-flash` | 0.2 | 1.15 | 0.04 | 1.15 | — |
| `tencent/hy3-preview` | 0.066 | 0.26 | 0.029 | 0.26 | — |
| `tokenrouter` | 0.3 | 1.2 | 0.06 | 1.2 | — |
| `vision-model` | 1.5 | 6 | 0.75 | 9 | 1.5 |
| `x-ai/grok-4.1-fast` | 0.2 | 0.5 | 0.05 | 0.5 | — |
| `x-ai/grok-4.20-beta` | 2 | 6 | 0.2 | 6 | — |
| `x-ai/grok-4.3` | 1.25 | 2.5 | 0.2 | 2.5 | — |
| `x-ai/grok-4.5` | 2 | 6 | 0.5 | 6 | — |
| `x-ai/grok-build-0.1` | 1 | 2 | 0.2 | 2 | — |
| `xiaomi/mimo-v2-flash` | 0.1 | 0.3 | 0.01 | 0.3 | — |
| `xiaomi/mimo-v2-omni` | 0.4 | 2 | 0.08 | 2 | — |
| `xiaomi/mimo-v2-pro` | 1 | 3 | 0.2 | 3 | — |
| `xiaomi/mimo-v2.5` | 0.4 | 2 | 0.08 | 2 | — |
| `xiaomi/mimo-v2.5-pro` | 1 | 3 | 0.2 | 3 | — |
| `z-ai/glm-4.5-air` | 0.13 | 0.85 | 0.025 | 0.85 | — |
| `z-ai/glm-4.6` | 0.6 | 2.2 | 0.11 | 2.2 | — |
| `z-ai/glm-4.6v` | 0.3 | 0.9 | — | 0.9 | — |
| `z-ai/glm-4.7` | 0.6 | 2.2 | 0.11 | 2.2 | — |
| `z-ai/glm-5` | 1 | 3.2 | 0.2 | 3.2 | — |
| `z-ai/glm-5-turbo` | 1.2 | 4 | 0.24 | 4 | — |
| `z-ai/glm-5.1` | 1.05 | 3.5 | 0.525 | 3.5 | — |
| `z-ai/glm-5.2` | 1.4 | 4.4 | 0.26 | 4.4 | — |
| `z-ai/glm-5.3-free` | 0 | 0 | 0 | 0 | — |

---

## 2. Entri pattern (51) — URUT, first match wins

| # | Pattern | input | output | cached | reasoning | cache_creation |
|---|---|---|---|---|---|---|
| 1 | *-codex-xhigh | 10 | 40 | 5 | 60 | 10 |
| 2 | *-codex-high | 8 | 32 | 4 | 48 | 8 |
| 3 | *-codex-max | 8 | 32 | 4 | 48 | 8 |
| 4 | *-codex-mini-* | 1.5 | 6 | 0.75 | 9 | 1.5 |
| 5 | *-codex-mini | 1.5 | 6 | 0.75 | 9 | 1.5 |
| 6 | *-codex-low | 1.75 | 14 | 0.175 | 14 | 1.75 |
| 7 | *-codex-none | 1.75 | 14 | 0.175 | 14 | 1.75 |
| 8 | *-codex-spark | 3 | 12 | 0.3 | 12 | 3 |
| 9 | codex-* | 1.75 | 14 | 0.175 | 14 | 1.75 |
| 10 | *-codex | 1.75 | 14 | 0.175 | 14 | 1.75 |
| 11 | claude-opus-* | 5 | 25 | 0.5 | 25 | 6.25 |
| 12 | claude-sonnet-* | 3 | 15 | 0.3 | 15 | 3.75 |
| 13 | claude-haiku-* | 1 | 5 | 0.1 | 5 | 1.25 |
| 14 | claude-* | 3 | 15 | 0.3 | 15 | 3.75 |
| 15 | gemini-*-flash-lite | 0.15 | 1.25 | 0.015 | 1.875 | 0.15 |
| 16 | gemini-*-flash | 0.3 | 2.5 | 0.03 | 3.75 | 0.3 |
| 17 | gemini-*-pro | 2 | 12 | 0.25 | 18 | 2 |
| 18 | gemini-3-* | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| 19 | gemini-2.5-* | 0.3 | 2.5 | 0.03 | 3.75 | 0.3 |
| 20 | gemini-* | 0.5 | 3 | 0.03 | 4.5 | 0.5 |
| 21 | gpt-5.6-* | 2.5 | 15 | 0.25 | 15 | 2.5 |
| 22 | gpt-5.3-* | 1.75 | 14 | 0.175 | 14 | 1.75 |
| 23 | gpt-5.2-* | 1.75 | 14 | 0.175 | 14 | 1.75 |
| 24 | gpt-5.1-* | 1.25 | 10 | 0.625 | 10 | 1.25 |
| 25 | gpt-5-* | 1.25 | 10 | 0.625 | 10 | 1.25 |
| 26 | gpt-5* | 1.25 | 10 | 0.625 | 10 | 1.25 |
| 27 | gpt-4o-* | 0.15 | 0.6 | 0.075 | 0.9 | 0.15 |
| 28 | gpt-4o | 2.5 | 10 | 1.25 | 15 | 2.5 |
| 29 | gpt-4* | 2.5 | 10 | 1.25 | 15 | 2.5 |
| 30 | o1-* | 3 | 12 | 1.5 | 18 | 3 |
| 31 | o1 | 15 | 60 | 7.5 | 90 | 15 |
| 32 | o3-* | 10 | 40 | 5 | 60 | 10 |
| 33 | o4-* | 2 | 8 | 1 | 12 | 2 |
| 34 | qwen3-coder-* | 1 | 4 | 0.5 | 6 | 1 |
| 35 | qwen*-coder-* | 1 | 4 | 0.5 | 6 | 1 |
| 36 | qwen* | 0.5 | 2 | 0.25 | 3 | 0.5 |
| 37 | kimi-*-thinking | 1.8 | 7.2 | 0.9 | 10.8 | 1.8 |
| 38 | kimi-k3* | 3 | 15 | 0.3 | 15 | 3 |
| 39 | kimi-k2* | 1.2 | 4.8 | 0.6 | 7.2 | 1.2 |
| 40 | kimi-* | 1 | 4 | 0.5 | 6 | 1 |
| 41 | deepseek-*reasoner* | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| 42 | deepseek-r* | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| 43 | deepseek-v* | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| 44 | deepseek-* | 0.14 | 0.28 | 0.0028 | 0.28 | 0.14 |
| 45 | glm-5* | 1 | 4 | 0.5 | 6 | 1 |
| 46 | glm-4* | 0.75 | 3 | 0.375 | 4.5 | 0.75 |
| 47 | glm-* | 0.5 | 2 | 0.25 | 3 | 0.5 |
| 48 | MiniMax-* | 0.5 | 2 | 0.25 | 3 | 0.5 |
| 49 | minimax-* | 0.5 | 2 | 0.25 | 3 | 0.5 |
| 50 | grok-code-* | 0.5 | 2 | 0.25 | 3 | 0.5 |
| 51 | grok-* | 0.5 | 2 | 0.25 | 3 | 0.5 |
