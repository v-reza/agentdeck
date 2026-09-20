# PLAN — AgentDeck M0 → M4

> **Status hidup.** Update file ini setiap milestone selesai. File ini yang jadi
> memori lintas-sesi; context chat boleh hilang, file ini tidak.
>
> Sumber angka: `docs/COVERAGE.md` (105 story), `docs/ROADMAP.md` (7 milestone),
> `design/stitch-output/v2/*.html` (ground truth visual).
>
> Terakhir diperbarui: 2026-09-19 (M0 dikerjakan).

## Kontrak yang mengikat (jangan dilanggar)

Baca ulang sebelum mulai milestone baru:

| Dokumen | Yang mengikat |
|---|---|
| `docs/ARCHITECTURE.md` §18.2 | React 19 + Vite, React Router v7, Tailwind v4 + shadcn/ui, Redux Toolkit + RTK Query, dnd-kit, Vitest + Playwright, folder `frontend/src/{routes,store,components,hooks,lib}` |
| `docs/DESIGN.md` | Token warna/radius/spacing. **Menang** atas `DECISIONS.md` §8 soal warna |
| `docs/DECISIONS.md` | Go 1.27, pgx v5 + sqlc, PostgreSQL 16, satu statement per baris |
| `docs/00-PRD.md` | 105 story, 359 AC. **Copy = kontrak**, bukan selera |
| `design/stitch-output/v2/*.html` | Ground truth visual. Clone **class-for-class** |

Aturan operasional:

- **Redux Toolkit satu-satunya state management.** Server state = RTK Query; client state = `createSlice`; realtime = `onCacheEntryAdded`. **Tidak ada Context untuk application data.**
- **Tidak ada `window.alert()` / `confirm()` / `prompt()`.** Modal harus komponen asli.
- **Routing wajib React Router v7.** Dilarang `window.location.pathname`.
- **Frontend diformat Prettier.** `gofmt -l .` harus kosong.
- **Error render di tempat kejadian** (inline). Toast hanya untuk error yang tidak punya permukaan inline.
- **Credential/token/password/connection string tidak boleh masuk commit/summary** — pakai `[REDACTED]`.
- **Bahasa UI: Indonesia** (design Indonesia, PRD Indonesia). Kamus EN+ID dijaga paritas tipe, default `id`.

## Gate — harus lulus sebelum klaim selesai

```bash
# backend
export PATH="/c/Program Files/Go/bin:$PATH"
cd /d/Project/agentdeck && gofmt -l . && go build ./... && go vet ./... && go test ./...

# frontend (SEKUENSIAL — Vitest timeout kalau barengan Playwright)
export PATH="/c/Program Files/nodejs:$PATH"
cd /d/Project/agentdeck/frontend
npx prettier --check src e2e
npx tsc -b --pretty false
npx vitest run
npx playwright test --reporter=line
npm run build
cd /d/Project/agentdeck && python tools/verify_web.py   # wajib 0 FAIL 0 warn
```

**Design match bukan klaim, tapi bukti.** Screenshot + `vision_analyze` dengan
pertanyaan element-level, lalu bandingkan dengan `design/stitch-output/v2/*.html`.
Screenshot tulis **di luar repo** (`$LOCALAPPDATA/Temp/agentdeck-shots`).

## Pitfall yang sudah memakan korban (jangan terulang)

1. **Reset CSS tanpa `@layer` mematikan SEMUA spacing Tailwind.** `public.css` pernah punya
   `*, *::before, *::after { margin: 0; padding: 0 }` di luar `@layer` → unlayered menang
   atas `@layer utilities` → `p-6` jadi `0px`, blok nempel. **Sudah dibungkus `@layer base`.**
2. **`vite.config.ts` harus ikut di-bind-mount.** Tanpa itu container jalan pakai config
   di image, edit apa pun ke dev server diabaikan diam-diam. **Sudah ditambah + `usePolling`.**
   Gejala: "fix gw gak ngefek" dua kali berturut-turut. Cek dulu:
   `docker compose exec -T web grep -c '<marker>' vite.config.ts` — `0` berarti bukan file lu.
3. **Design sekeluarga tidak seragam.** 4 layar auth punya angka berbeda (radius 10 vs 14,
   judul 17/600 vs 18/700, tombol 36/500 vs 38/600). **Baca CSS masing-masing file**, jangan
   rata-ratakan.
4. **Assert geometri, bukan cuma copy.** Radius, shadow, font size/weight, input padding,
   tinggi tombol, gap antar blok — semua di-assert di DOM.
5. **Normalisasi `boxShadow`.** Tailwind v4 pakai 4 slot `--tw-shadow`; Chromium lapor yang
   kosong sebagai `rgba(0, 0, 0, 0) 0px 0px 0px 0px`. Buang sebelum banding.
6. **`Button` default variant = `secondary`** (putih/outline). Tombol utama **wajib**
   `variant="primary"`. Ini penyebab tombol "New project" tidak hijau.
7. **Vite container bisa serve modul basi.** `curl` modulnya dan grep identifier baru.

## Struktur eksekusi

**Keputusan user 2026-09-19 (mengikat):** kerjakan **M0 → M1 → M2 → M3 → M4 lurus**, tanpa
menyisipkan milestone perantara. Run Executor **bukan** milestone terpisah — dia bagian dari
**M4** (§12 kontrak sudah memuatnya lengkap: 4 mode workspace, batas resource, siklus
artifact). Jangan pecah M4 jadi "M4a/M4b", jangan tunda M4 ke belakang.

Dua track yang jalan berdampingan per milestone:

- **Track A — backend.** Endpoint + migrasi + sqlc + test Go. Selesai kalau `go test ./...` hijau.
- **Track B — frontend.** Layar **clone design dulu**, baru fungsionalkan. Selesai kalau
  guard design-match hijau + screenshot diverifikasi.

**Aturan urutan tiap layar (jangan dibalik):**

1. Baca penuh `design/stitch-output/v2/<screen>.html` — seluruh file, bukan sebagian.
2. Tulis komponen clone class-for-class (belum ada logic).
3. Tulis guard E2E design-match (copy + geometri) → **jalankan, harus lulus**.
4. Baru sambungkan RTK Query + form.
5. `vision_analyze` screenshot vs design.
6. Gate penuh.

Kalau dibalik (fungsional dulu, design belakangan), kerjaannya diulang dua kali — itu yang
kejadian di auth.

---

## M0 — Identity and workspace foundation

**Outcome:** pengguna bisa daftar, login, dan langsung masuk ke workspace personal tanpa setup
organisasi manual. **10 story (8 Must).** 8 screen.

| # | Story | Pri | Layar | Backend | Status |
|---|---|---|---|---|---|
| 1 | US-AD01 Registrasi akun baru | Must | `02-register` | ada | ✅ SELESAI |
| 2 | US-AD02 Login dan logout | Must | `01-login` | ada | ✅ SELESAI |
| 3 | US-AD88 Reset password | Must | `03-reset-request`, `04-reset-confirm` | ada (migrasi 0005) | ✅ SELESAI |
| 4 | US-AD04 Manajemen anggota & role | Should | `38-members` | **endpoint lengkap** (GET/POST/PATCH/DELETE) | ⬜ UI read-only |
| 5 | US-AD03 Manajemen organisasi (CRUD) | Should | `37-workspace-settings` | `GET/PATCH /orgs/{id}` ada | ⬜ layar belum ada |
| 6 | US-AD77 Halaman settings org | Must | `37-workspace-settings` | **`audit_log` BELUM ADA** (AC2 minta before/after) | ⬜ |
| 7 | US-AD89 Profil akun sendiri | Must | `15-profile` | **`PATCH /auth/me` BELUM ADA** + `avatar_url` | ⬜ |
| 8 | US-AD92 First-run onboarding | Must | `12-onboarding` | langkah 3 butuh agent (M1) | ⏸️ **DITUNDA user** |
| 9 | US-AD07 Isolasi data antar Org | Must | backend-only | perlu verifikasi | ⬜ |
| 10 | US-AD93 Konteks ruang kerja aktif | Must | backend-only | perlu verifikasi | ⬜ |

**Gap backend M0:** migrasi `0006` (`audit_log`), `PATCH /api/v1/auth/me`, `avatar_url` di `GET /auth/me`.

**Definisi M0 selesai:** 9 story di atas PASS (US-AD92 ditunda atas keputusan user),
`audit_log` ada, gate hijau, tiap layar punya guard design-match + screenshot terverifikasi.

---

## M1 — Board and task loop

**Outcome:** satu developer bisa membuat project, board, task, dan menjalankan lifecycle task
dari backlog sampai done. **32 story (28 Must).** 14 screen.

Backend-only (15): US-AD21 dispatcher claim (`FOR UPDATE SKIP LOCKED`), US-AD22 run lifecycle,
US-AD23 heartbeat, US-AD24 reclaim stale, US-AD66 max runtime, US-AD75 `workspace_kind`.

Layar: `10-board-list`, `11-project-list`, `16-security`, `18-kanban`, `20-task-drawer`,
`21-task-create`, `22-column-editor`, `23-board-settings`, `25-agent-registry`, `26-agent-form`,
`28-agent-detail`, `42-dashboard`, `44-state-empty`, `46-mobile-board`.

**Komponen baru yang dibutuhkan M1:**
- **`Modal`** — belum ada di codebase. Wajib: `role="dialog"`, `aria-modal`, `aria-labelledby`,
  focus masuk saat buka + balik ke trigger saat tutup, `Escape` tutup, klik backdrop tutup,
  focus trap, scroll lock, `prefers-reduced-motion`. Backdrop `rgba(15,23,42,0.45)`.
- **Kanban board + dnd-kit** — drag task antar kolom, `DragOverlay`, scroll sinkron antar kolom.

**Definisi M1 selesai:** task bisa dibuat, di-drag antar kolom, dispatcher mengklaim dengan
`SKIP LOCKED`, heartbeat + reclaim jalan, semua 14 layar match design.

---

## M2 — Cost and agent control

**Outcome:** operator tahu biaya setiap run dan bisa mengatur agent/provider tanpa hidden spend.
**17 story (15 Must).** 8 screen.

Termasuk **agent registry lengkap** (provider, model, tools, skills, retry policy),
**encrypted provider API key** (AES-256-GCM, `provider_api_key_enc BYTEA`), dan
**immutable cost ledger** dalam integer micro-USD dengan `price_version` snapshot.

Layar: `24-dependency-view`, `27-agent-provider-key`, `29-cost-overview`, `30-ledger-explorer`,
`34-step-payload`, + `18-kanban` (cost rail US-AD97), `20-task-drawer` (US-AD32),
`26-agent-form` (US-AD96).

**Definisi M2 selesai:** agent punya kredensial provider terenkripsi, tiap step menulis baris
ledger, budget harian board + cap per run menegakkan hard stop, cost rail tampil di board.

---

## M3 — Human approval and realtime operations

**Outcome:** aksi berisiko berhenti menunggu manusia, sementara operator melihat perubahan tanpa
refresh. **16 story (15 Must).** 9 screen.

Layar: `13-notifications`, `32-run-detail`, `33-run-timeline`, `35-approval-inbox`,
`36-approval-detail`, `43-state-loading`, `45-state-error` + `18-kanban`, `20-task-drawer`.

Realtime = SSE hub (`onCacheEntryAdded`), approval gate transisi
`running → awaiting_approval → ready|blocked`.

**Definisi M3 selesai:** aksi bergerbang muncul di inbox approval, keputusan idempoten,
perubahan status terlihat live tanpa refresh.

---

## M4 — Reliability and artifact loop

**Outcome:** kegagalan transient pulih otomatis dan hasil run bisa direproduksi.
**10 story (10 Must).** 2 screen (`20-task-drawer`, `27-agent-provider-key`).

Termasuk **Run Executor** — komponen yang benar-benar menjalankan kerja: siklus step, panggil
LLM/tool, tulis step + ledger, hasilkan artifact. 4 mode `workspace_kind` (`scratch`, `dir`,
`worktree`, `container`), batas resource (disk 25 MB/file & 100 MB/task, heap 512 MB soft /
1 GB hard, runtime N9, step payload 64 KB, 5000 step/run), retry + failure taxonomy.

**Definisi M4 selesai:** run gagal transient pulih otomatis, artifact terunggah + SHA-256
terverifikasi, run bisa direproduksi.

---

## Setelah M4 (bukan scope sekarang)

- M5 (7 story) — governance & integrasi: API key, webhook, audit
- M6 (13 story) — operator QoL
- **22 layar sisanya masih DRIFT** dari design (copy/i18n + struktur). Reset CSS sudah
  membetulkan spacing global; sisanya per layar.

## Catatan dogfood (tujuan user)

User mau memakai AgentDeck sendiri untuk melanjutkan AgentDeck, lewat board-nya. Supaya itu
mungkin, yang wajib ada: M1 (board + claim) → **M4 Run Executor** (yang benar-benar mengerjakan)
→ M2 (agent + provider key + ledger). Urutan pengerjaan tetap M0 → M1 → M2 → M3 → M4 sesuai
keputusan user.
