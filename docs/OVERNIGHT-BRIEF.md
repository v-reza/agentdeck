# OVERNIGHT-BRIEF — AgentDeck

Kontrak kerja buat run semalaman. Dibaca **di awal tiap turn**, bukan sekali.
Kalau brief ini dan ingatan gw berbeda, brief ini yang benar.

- **Mulai:** 2026-09-26 malam (sesi `20260921_155418_6ef46c`)
- **Gate:** `tools\gate-overnight.cmd` (wajib, sudah diuji dua arah)
- **Jejak:** `docs/OVERNIGHT-LOG.md` — satu entri per fase, ditulis sebelum lanjut

---

## 1. Misi

Ngerjain endpoint ⬜ dari `docs/ARCHITECTURE.md` §6.2, **urut dari yang termurah**,
satu fase = satu commit + push. 51 endpoint sisa; **tidak** harus kelar semua —
yang penting tiap fase selesai utuh, bukan setengah.

## 2. Urutan fase (dari tabel DETAIL §6.2, bukan ringkasan)

> §6.2.20 (ringkasan per modul) **basi** — dia bilang Runs 0/6, padahal cuma 2 ⬜.
> Hitung dari tabel detail. Angka otoritatif: **78 ✅ / 51 ⬜** (dijaga `verify_suite.py`).

| # | Modul | ⬜ | Catatan biaya |
|---|---|---:|---|
| 1 | 6.2.1 Health, Liveness & Metrics | 2 | `/livez` trivial; `/metrics` teks Prometheus |
| 2 | 6.2.9 Tasks | 3 | `cancel`/`retry`/`archive` — logic board sudah ada |
| 3 | 6.2.11 Runs | 2 | `cancel` (abort) + `summary` |
| 4 | 6.2.15 Cost Ledger & Budget | 3 | `daily_board_costs` sudah ada (migrasi `0014`); `GET /budget` menutup 404 yang sudah lama ada di FE |
| 5 | 6.2.2 Auth & Sessions | 4 | butuh soft-delete user (kolom `deleted_at`) |
| 6 | 6.2.4 Orgs & Memberships | 1 | `DELETE /orgs/{id}` + cascade |
| 7 | 6.2.14 Approvals | 5 | tabel `approvals` **sudah ada** (migrasi `0013`) |
| 8 | 6.2.19 Audit, Search & System | 6 | `audit_log` ada; `notifications` belum; search butuh `pg_trgm` |
| 9 | 6.2.17 Comments | 4 | tabel `comments` belum ada |
| 10 | 6.2.3 API Keys | 5 | tabel `api_keys` belum ada + auth lewat key |
| 11 | 6.2.13 Events & Realtime SSE | 4 | modul `internal/sse/` dari nol |
| 12 | 6.2.18 Webhooks & Deliveries | 7 | tabel belum ada + worker kirim + retry |
| 13 | 6.2.16 Artifacts | 5 | butuh `internal/storage/` (R2/S3) — **kemungkinan besar di luar jangkauan malam ini** |

Kalau fase 1–8 kelar, itu hasil yang bagus. Fase 9+ dikerjakan **hanya** kalau
masih ada waktu dan fase sebelumnya sudah ter-push — jangan mulai yang baru
sebelum yang lama ter-push.

## 3. Loop tiap fase

1. Baca kontraknya dulu: §6.2 baris endpoint + section rujukannya (`US-AD`, `§3`, `§10`).
2. Tulis plan singkat di `docs/OVERNIGHT-LOG.md` (fase, endpoint, file, risiko).
3. Migrasi dulu kalau butuh tabel (`internal/migrate/00NN.up.sql`), lalu `sqlc generate`.
4. Kode + test **paket itu**. Bug fix → RED dulu.
5. `go test` paket yang kesentuh (manual — **nggak masuk gate**, lihat §4).
6. Mutation kalau menyentuh security/business rule.
7. Rebuild container + probe lawan API nyata kalau ada endpoint baru.
8. Update `docs/ARCHITECTURE.md` baris ⬜ → ✅ **di commit yang sama** + angka §6.2.20.
9. `tools\gate-overnight.cmd` → harus EXIT 0.
10. Commit + push + tulis hasilnya di `docs/OVERNIGHT-LOG.md`.

## 4. Kontrak gate (terukur, bukan tebakan)

`tools\gate-overnight.cmd` = `gofmt -l .` → `go build ./...` → `tsc -b` →
`vitest run` → `prettier --check src/`. Terukur **20–52 detik**, di bawah plafon
300 detik. Sudah diuji: pohon kotor → EXIT 1 (0.9s, nyebut `gofmt`), pohon bersih → EXIT 0.

**`go test ./...` DIKELUARKAN dari gate, dan itu disengaja.** Terukur 490 detik
(`-short` pun sama — nol `testing.Short()` di repo), jadi mustahil masuk plafon
300 detik. Konsekuensinya: gate hijau **tidak** berarti test Go hijau. Gw yang
harus jalanin test paket yang kesentuh, dan **lapor di log kalau ada fase yang
nggak gw test**. Jangan pernah bilang "gate hijau jadi aman".

`prettier` sengaja di-scope ke `src/` — `prettier --check .` merah karena file
milik sesi lain (`frontend/e2e/demo/measure-aksi.mjs`), dan gate gw nggak boleh
merah karena kerjaan orang lain.

## 5. Aturan keras

- **Jangan** nyalain dispatcher AgentDeck (`AGENTDECK_DISPAT`). Keputusan user: OFF.
- **Jangan** sentuh board kanban, `tools/gate.cmd`, `frontend/e2e/demo/`,
  `tools/demo/`, atau `.gitignore`. Itu milik sesi lain.
- **Jangan** commit: `node_modules`, build output, cache, screenshot sementara, secrets.
- Kredensial/token/connection string → `[REDACTED]`. Nol nomor WA/JID/chatId di file.
- Jargon spec (`US-AD`, `AC`, `M1`–`M6`, `HTTP \d{3}`) **dilarang** masuk UI yang dirender.
- `git ls-remote origin refs/heads/main` = satu-satunya bukti push yang sah.
- Edit lewat python bisa ngubah line-ending → repo ini LF. Cek `file <path>` setelahnya.
- `curl` di MSYS itu program Windows native: `-o /tmp/x` nulis ke `C:\tmp\`. Pakai path native.

## 6. Kapan berhenti

- Fase 1–8 kelar dan masih ada waktu → lanjut fase 9+, tetap satu commit per fase.
- Ada blocker yang butuh keputusan produk → **stop**, tulis di log, jangan ngarang
  jawaban. Kontrak yang bertabrakan: sebut konfliknya, pakai keputusan terbaru.
- Gate merah dan nggak kelar dalam 3 percobaan → stop, tulis apa adanya.
- Kehabisan turn (`goals.max_turns`) → goal auto-pause; watchdog yang ngabarin.

## 7. Yang TIDAK dijamin malam ini

- 51 endpoint kelar. Realistis 6–12 endpoint per malam dengan disiplin kontrak.
- Full e2e Playwright (butuh ~5 menit dan Docker hidup; bukan gate).
- `internal/storage/` (R2/S3) — fase 13 kemungkinan besar nggak kesentuh.
