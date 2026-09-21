# Handoff — status AgentDeck per 2026-09-21

Buat session baru. Baca `.hermes.md` + `docs/DECISIONS.md` dulu, terus file ini.

---

## Kerjaan yang sedang jalan: Provider Registry

Konsepnya disetujui 2026-09-21. Tujuh fase, detail di `docs/CONCEPT-PROVIDER-REGISTRY.md`.

| Fase | Isi | Status |
|---|---|---|
| 0 | Kontrak: PRD + DECISIONS + ARCHITECTURE | **SELESAI** |
| 0.5 | Selaraskan §18 + gate backend · arsip dokumen mati | **SELESAI** |
| 1 | Migrasi `0010`: tabel `providers`, `agents.provider_id`, backfill | belum |
| 2 | Backend CRUD provider + RBAC | belum |
| 3 | Probe protocol-aware (openai_compatible dulu) | belum |
| 4 | Halaman Provider (nav baru) | belum |
| 5 | Form agent: provider jadi dropdown | belum |
| 6 | Buang `agents.base_url` + constraint `agents_base_url_chk` | belum |

Aturan mengikat: `DECISIONS.md` §6A.J. Endpoint: `ARCHITECTURE.md` §6.2.8 (total 123).

---

## Di mana kita sekarang

AgentDeck = board orkestrasi AI-agent fleet. Go + PostgreSQL 16 backend,
React 19 + Vite frontend. Repo publik: `https://github.com/v-reza/agentdeck`.

**Fase**: masih development, ngejar **fitur jalan + design match**, bukan
kelengkapan test. Rigor test diturunin (lihat `.hermes.md` bagian gate berlapis).

## Keputusan proses (2026-09-21)

1. **Gate berlapis.** Layout = screenshot elemen + ukur DOM, nol unit test.
   Logic = test. Security = test + mutation. Full suite 1× sehari, bukan tiap
   iterasi. Alasan: satu hari kepakai buat verifikasi layout pakai 92 e2e test.
2. **Session per milestone.** Session lama (`20260919_004317_ed3ea9`) 114.336 pesan /
   91MB — 2× lipat session terbesar kedua. Tiap turn bawa konteks yang udah
   di-compress, jadi detail ilang diam-diam. Pindah session per milestone.
3. **Design inventory sebelum bilang layar kelar.** Lihat `docs/DESIGN-INVENTORY.md`.

## Kerjaan yang BELUM di-commit

53 file kotor. Semua gate hijau (e2e 92/92, vitest 58, Go suite,
`verify_web` 0 FAIL, `verify_suite` 0 FAIL). **Nol commit** — user belum minta.

Termasuk fase 0 + 0.5: kontrak provider registry (`00-PRD.md` US-AD109,
`DECISIONS.md` §6A.J, `ARCHITECTURE.md` §3 + §6.2.8 + §6.2.20 + §18/§18.3),
gate baru `check_structure()` + cek kolom `Status`, dan arsip/hapus dokumen mati.

**Urutan commit yang diusulkan ada di `docs/COMMIT-PLAN.md`** — 4 commit, dipisah
per tema (BYO flow / kontrak / restructure dokumen / env). Belum dijalankan.

Yang berubah:

- **Backend**: `cmd/api/provider_models.go` (endpoint probe stateless baru),
  `internal/board/service.go` (`validateBaseURL` + `ValidateAddressOnly`),
  `internal/provider/provider.go` (pisah cek alamat vs cek DNS),
  `cmd/api/boards.go`, `internal/board/pgx.go`, `internal/store/queries*`
  (`base_url` + `reasoning_effort` masuk `CreateAgent`)
- **Frontend**: `components/ui/combobox.tsx` (BARU — semua select pakai ini),
  `routes/dashboard/agents/CreateAgentFields.tsx` (BARU), `lib/field-error.ts`,
  `AgentRegistry.tsx` (`table-fixed` + `<colgroup>`), `CreateAgentForm.tsx`,
  `StatusFilter.tsx`, `components/ui/input.tsx`, `components/ui/modal.tsx`,
  `lib/i18n.ts`, `store/api/agents.ts`
- **Test**: `e2e/combobox.ts`, `e2e/agent-form.spec.ts`, `e2e/agents.spec.ts`,
  `agents_baseurl_test.go`, `agents_model_gate_test.go`, `provider_models_test.go`,
  `queries_columns_test.go`, `agents.test.ts`

## Keputusan final yang mengikat

- **Provider BYO-only di form register** — **DIBATALKAN** oleh `DECISIONS.md` §6A.J.
  Provider sekarang datang dari daftar milik org. `CREATE_PROVIDER_CHOICES` akan diganti
  daftar itu di fase 5; jangan dipertahankan sebagai keputusan.
- **SSRF**: `localhost` + `127.0.0.1` **string persis** boleh, boleh `http`.
  Alias loopback (`2130706433`, `0x7f000001`, `[::1]`) + RFC1918 + link-local tetap ditolak.
  Mengalahkan US-AD106 AC3 — konflik dicatat di `DECISIONS.md` §6A.F
  (SSRF-nya sendiri masih berlaku; yang digantikan §6A.J cuma soal lokasi kredensial).
- **Jalur tulis nggak boleh resolve DNS.** `ValidateAddressOnly` (teks + IP literal)
  buat create/update; `ValidateOperatorBaseURL` (resolve) buat jalur yang konek.
- **Endpoint probe** `POST /api/v1/provider/models`, floor Admin, stateless, nol DB.
  `400` user-error / `502` upstream.
- **Satu dropdown**: `Combobox` buat semua select.
- **Modal register 560px** (`size="lg"`), panel kredensial tetap 420px.

## Masalah yang BELUM beres

1. **Design match — ini yang paling gede.** Detail + angka di `docs/DESIGN-INVENTORY.md`
   (50 layar diaudit, 36 ada impl, 14 nol impl). Lima kelas masalah, urut dari
   paling murah dibetulin:

   | # | Masalah | design | impl | Biaya fix |
   |---|---|---|---|---|
   | 1 | **Warna status beda 10/10** | `#16a34a` (done) | `#15803d` | 10 baris `index.css` |
   | 2 | Icon Lucide | 319 `<svg>` | 3 | 1 dependency, 319 titik |
   | 3 | Grafik batang / sparkline | 119 `<rect>` | 3 | ikut nomor 2 |
   | 4 | Skeleton loading | 55 `animate-pulse` | 4 | per layar |
   | 5 | State archived | 6 `line-through` | 0 | per layar |
   | 6 | Token `--spacing-*` mati | 6 token | **0 dipakai** | hapus atau pakai |

   **Akar masalahnya**: nol gate yang ngecek `index.css` terhadap `DESIGN.md`.
   `verify_suite.py` cuma `designmd lint` DESIGN.md sendiri. Persis pola §18 —
   klaim tanpa gate = drift diam-diam. Kalau bikin gate baru, taruh di sini.
2. **Layar agent registry belum nemu "titik enaknya".** Satu hari lebih ngotak-atik
   implementasi, tapi pertanyaan "informasi apa yang harus ada di layar ini" belum
   dijawab. **Perlu sesi khusus nggak ngoding.**
3. `statusFilter` + `AgentDetailForm` masih ada `<select>` native sisa.
4. Pesan backend 409/403 masih Inggris, UI default Indonesia.
5. Belum ada `LICENSE`/`NOTICE`/`THIRD_PARTY`. Konflik lisensi di design
   (`09b-github.html` Apache-2.0 vs `05-landing.html` MIT).
6. ~~Audit `livez`/`metrics` + tabel tanpa DDL~~ **SELESAI** — lihat
   "Kemajuan nyata" di bawah. Angkanya sekarang dijaga gate, bukan dihitung manual.
7. `US-AD03` TODO — `DELETE /api/v1/orgs/{id}` belum ada.

## Environment

- Docker hidup: `agentdeck-api` :8080, `agentdeck-db` :5432, `agentdeck-web` :5173.
- **Ubah kode Go → rebuild container**, kalau nggak route baru nggak ada (ini
  yang bikin 404 dan gw buang waktu nyari bug di test).
- Playwright baseURL `http://127.0.0.1:5173`. Cookie `Secure` → `page.request` 401,
  browser `fetch` 200. Test pakai `page.evaluate` + `credentials: 'include'`.
- Go: `/c/Program Files/Go/bin`. sqlc: `/c/Users/Reza/go/bin`. Node: `/c/Program Files/nodejs`.

## Wajib

**API key 9Router sudah dirotasi user — insiden ditutup.** Nilai lama jangan
pernah ditulis di file mana pun (repo ini publik).

## Mulai dari mana (buat session baru)

1. Baca `.hermes.md` (auto-load) → `docs/DECISIONS.md` §6A.J → file ini.
2. **Kalau nggak ada instruksi lain: fase 1** — migrasi `0010` (tabel `providers`,
   `agents.provider_id`, backfill). Tujuh fase di tabel atas.
3. **Yang paling murah + paling kerasa kalau mau cepat**: warna status. 10 baris
   `index.css` — lihat `docs/DESIGN-INVENTORY.md` §2.
4. Yang **jangan** dikerjain dulu: layar Provider (fase 4) sebelum fase 1–3 kelar.

## Kemajuan nyata (dihitung dari kode, bukan dari niat)

| | Jumlah |
|---|---|
| Endpoint terdefinisi di kontrak | 123 |
| **Endpoint jalan** (route terdaftar di `cmd/api`) | **55** |
| Endpoint belum | 68 |
| Tabel kontrak | 26 |
| Tabel ada migrasi + DB | 17 |

Per modul (detail di `ARCHITECTURE.md` §6.2.20): Agents **15/15** · Boards **7/7** ·
Projects **5/5** · Task Links **4/4** · Orgs **8/9** · Auth **7/11** · Tasks **7/11** ·
Health **2/4** · API Keys **0/5** · Providers **0/7** · Runs **0/6** · Steps **0/3** ·
SSE **0/4** · Approvals **0/5** · Ledger **0/5** · Artifacts **0/5** · Comments **0/4** ·
Webhooks **0/7** · Audit **0/6**.

Modul runtime (`dispatcher`, `executor`, `sse`, `storage`, `webhook`) **nol kode**.
Runtime belum pernah memanggil LLM (`grep chat/completions` → nol).

Angka ini dijaga gate: kolom `Status` di §6.2 diperiksa `verify_suite.py` dua arah,
jadi ✅ palsu dan ⬜ palsu dua-duanya FAIL.
