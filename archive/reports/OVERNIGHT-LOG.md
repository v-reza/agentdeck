# OVERNIGHT LOG — AgentDeck M0 → M4

> Append-only. One block per run. Dibaca ulang di setiap run sebagai memori
> lintas-sesi (brief: `archive/reports/OVERNIGHT-BRIEF.md`).

---

## 2026-09-20 07:00–07:36 — US-AD89 (M0) → PASS

**Story:** `US-AD89` — Profil akun sendiri (`15-profile`, Must, M0).

**Keadaan awal run.** `archive/reports/OVERNIGHT-LOG.md` belum ada (ini run pertama yang
menulisnya). `tools/checklist_status.json` menandai US-AD89 `todo`; M0 lain
sudah `pass` kecuali `US-AD92` (`defer`, keputusan user). Working tree berisi
kerja M0 yang belum di-commit.

**Temuan: gate merah saat run dimulai.** Kerja US-AD89 sudah setengah jalan dari
sesi sebelumnya dan meninggalkan repo dalam keadaan **tidak bisa di-build**:

1. `frontend/src/components/layout/IconRail.tsx` merender `<AccountMenu />`
   **tanpa import** → `tsc -b` gagal: `TS2304: Cannot find name 'AccountMenu'`.
   Ini blocker keras: `npm run build` dan seluruh e2e tidak bisa jalan.
2. `frontend/src/components/layout/AccountMenu.tsx` diakhiri komentar menggantung
   (`/** Kept as a named export ... */`) tanpa deklarasi → prettier gagal.
3. `frontend/src/routes/dashboard/settings/Profile.tsx` belum diformat prettier.

**Yang dikerjakan:**

- Perbaiki import `AccountMenu` di `IconRail.tsx`; buang komentar menggantung di
  `AccountMenu.tsx`; jalankan prettier pada dua file itu.
- **Bug dokumen:** doc-comment `Profile.tsx` mengklaim spec banner *tidak*
  di-clone ("the same call the Members screen made"), padahal kodenya memang
  merender badge `US-AD89` + chip `Must • M0`. Komentar diperbaiki supaya
  menyebut apa yang benar-benar di-clone dan apa yang sengaja dibuang
  (`State: default`, `Blueprint C`, klaim `COMPLIANT`), plus alasan `<h1>`
  memakai nama section (topbar sudah merender judul layar).
- **Gap test yang nyata:** US-AD89 satu-satunya layar M0 yang **tidak punya
  e2e design-match**, padahal layar M0 lain punya (`members.spec.ts`,
  `workspace-settings.spec.ts`, `project-directory.spec.ts`,
  `auth-design.spec.ts`). Ditulis `frontend/e2e/profile.spec.ts` (6 test):
  geometri panel/tile/badge/chip vs `15-profile.html`, AC1 (4 atribut identik
  dengan respons `GET /auth/me`, termasuk monogram dari server), AC2 (rename
  terlihat di topbar **tanpa reload**), AC3 (email terpakai → 409 inline, data
  tidak berubah), AC4 (id orang lain → 404, bukan 403), AC5 (terbuka dari menu
  avatar, role `viewer` tetap bisa baca **dan** tulis).
- Test-id ditambahkan ke `Profile.tsx` / `avatar.tsx` agar assertion mengukur
  elemen yang benar (bukan selector tebakan).

**Mutation check (test-nya benar-benar bisa gagal).** Tiga assertion diuji dengan
merusak kode produksi lalu memastikan test gagal, lalu kode dipulihkan:

| Mutasi | Hasil |
|---|---|
| padding panel header `10px` → `12px` | FAIL pada `design: p-[10px] on the header card` ✓ |
| `updateMe` memanggil `window.location.reload()` | FAIL pada marker `__profileNoReload` ✓ |
| marker `window` sengaja dinamai salah | FAIL pada `the rename must not reload the document` ✓ |

Sumber dipulihkan byte-identik (`diff` vs backup bersih; `session.ts` nol
`window.location.reload`/`setTimeout`).

**Gate lengkap — SEKUENSIAL, semuanya hijau:**

- `gofmt -l .` kosong · `go build ./...` · `go vet ./...` · `go test ./...` → ok
  (`cmd/api` 10.6s, `internal/auth`, `internal/config`, `internal/migrate` ok).
- `npx prettier --check src e2e` → All matched files use Prettier code style.
- `npx tsc -b --pretty false` → bersih.
- `npx vitest run` → **56/56 passed** (7 files).
- `npx playwright test --workers=1` → **43/43 passed** (37 lama + 6 baru).
- `npm run build` → sukses.
- `python tools/verify_web.py` → **SEMUA GATE BERSIH (0 FAIL, 0 warn)**.

Catatan: Playwright gagal sekali di `_seed.spec.ts` saat run pertama (timeout
cold-start seeder), lulus 17.4s saat dijalankan sendiri, dan lulus di run penuh
berikutnya. Bukan regresi — tapi `_seed.spec.ts` memang generator fixture, bukan
test, dan ikut terhitung di suite.

**Bukti visual.** Screenshot ke
`$LOCALAPPDATA/Temp/agentdeck-shots/15-profile.png` (di luar repo), lalu
`vision_analyze` element-level: badge `US-AD89` teal pill bercentang, chip abu
`Must · M0`, judul panel `Profil akun mandiri`, 4 tile AC1 (ID/EMAIL row 1,
NAME/AVATAR_USER row 2), lingkaran monogram gelap, kartu `Keanggotaan ruang
kerja` + chip `OWNER`, form email/name editable + tombol `Simpan perubahan`.
Sesuai `15-profile.html`.

**Checklist:** `python tools/update_checklist.py US-AD89 pass` →
`docs/CHECKLIST.md` (9 PASS, 10 tracked).

**Status milestone:** **M0 SELESAI.** Sisa M0 hanya `US-AD92` yang ditunda atas
keputusan user — tidak dikerjakan sesuai brief.

**Hambatan:** tidak ada yang butuh manusia. Satu perintah (heredoc Python)
tertahan approval dan tidak dijalankan; diganti tool `patch` tanpa kehilangan
pekerjaan.

**Berikutnya:** `US-AD08` (Membuat project dalam Org, Must, M1) — story pertama
M1. Catatan awal: `ProjectDirectory.tsx`, `CreateProjectForm.tsx`, dan e2e
`project-directory.spec.ts` (termasuk `creating a project through the modal
persists it — US-AD08 AC1`) **sudah ada**, jadi M1 kemungkinan sudah dikerjakan
sebagian tetapi belum ditandai PASS. Run berikutnya harus memverifikasi AC
US-AD08 terhadap kontrak, bukan menulis ulang dari nol.

---
## 2026-09-20 08:46–09:05 — US-AD09 (M1) → tetap 🔨 dikerjakan (TIDAK di-PASS-kan)

**Konteks run.** Brief dibaca ulang; status dibaca dari `docs/CHECKLIST.md` +
`tools/checklist_status.json`. Story berikutnya yang belum PASS di milestone
terendah = `US-AD09` (Membuat board kanban, Must, M1) — sudah ditandai `wip`
oleh sesi sebelumnya (working tree memuat `internal/board/`, `cmd/api/boards.go`,
migrasi `0007`). Run ini **memverifikasi** pekerjaan itu, bukan menulis ulang.
Waktu host saat run mulai 08:51 → masuk jendela penutupan, jadi run ini
dijalankan sebagai **verifikasi + penutupan**, bukan story baru.

**Lingkungan.** `docker compose ps`: `agentdeck-api` healthy (17 mnt),
`agentdeck-db` healthy, `agentdeck-web` up. `curl /readyz` → 200.

### Verifikasi US-AD09 terhadap AC (bukti, bukan klaim)

Backend diuji langsung ke API hidup (bukan hanya unit test), dengan user baru:

| AC | Bukti | Hasil |
|---|---|---|
| AC1 `POST /projects/{id}/boards` → 201 + kolom default | `201`, body `columns` = `[{backlog,Backlog},{ready,Ready},{running,Running},{review,Review},{done,Done}]`, `budget_daily_micros=20000000`; `GET /boards/{id}` mengembalikan urutan yang sama dari `columns_json` | ✅ |
| AC2 nama duplikat dalam satu project → 409 | Board kedua `name="Sprint 24"`, slug berbeda → `409 a board with this name already exists in this project` | ✅ |
| AC3 hanya owner/admin → member/viewer 403 | `cmd/api/boards_create_rbac_test.go` (owner/admin lolos gate, member/viewer 403, viewer+body rusak tetap 403 sebelum body dibaca) · `go test ./cmd/api` ok | ✅ |
| AC4 (B2C) tidak perlu pilih org/project baru bila belum punya project | Tidak perlu org picker: board dibuat dengan `X-Org-ID` dari sesi, tidak ada field org di body. **Tapi** `POST /auth/register` **tidak** membuat project default — user baru punya `GET /projects` = `[]`, jadi "project baru" masih wajib. `POST` board ke project milik org lain → `404` (isolasi tenant benar) | ⚠️ **belum terpenuhi** |

**Bukti DB-level (migrasi 0007).** `boards_project_name_key UNIQUE (project_id, name)`
ada; Postgres jadi arbiter (bukan pre-flight SELECT yang bisa race), dan duplikat
lama di-disambiguasi dengan suffix ULID 6 char tanpa menghapus baris.

**Gap yang membuat story ini TIDAK di-PASS-kan** (aturan jujur: jangan tandai
PASS kalau belum lengkap):

1. **Tidak ada UI pembuatan board sama sekali.** `useCreateBoardMutation`
   didefinisikan di `store/api/boards.ts:106` tetapi **nol pemakaian** di
   `frontend/src/**/*.tsx`. Design `10-board-list.html` menampilkan CTA
   `New board` (accent-filled, ikon plus) dan `From template`; CTA itu belum ada
   di `BoardList.tsx`. Story ini berscreen `18-kanban`, jadi gate-nya termasuk
   design match — dan design match belum bisa dibuktikan untuk alur yang belum ada.
2. **AC4 belum benar-benar terpenuhi** (lihat tabel di atas).
3. **Tidak ada e2e khusus board-creation.** `frontend/e2e/` tidak punya
   `kanban.spec.ts`; `18-kanban` hanya tersentuh lewat screenshot `_seed.spec.ts`
   (generator fixture, bukan test) dan satu test drawer di `dashboard.spec.ts`.
   Board di semua e2e lain dibuat lewat `fetch` langsung (lihat komentar di
   `dashboard.spec.ts`: "the board-creation form is still on the M1 list").

`tools/checklist_status.json` **tidak diubah** (`US-AD09` tetap `wip`).

### Gate lengkap — SEKUENSIAL, semuanya hijau (dijalankan run ini)

- `gofmt -l .` kosong · `go build ./...` ok · `go vet ./...` ok ·
  `go test ./...` ok (`cmd/api`, `internal/auth`, `internal/board`,
  `internal/config`, `internal/migrate`).
- `npx prettier --check src e2e` → All matched files use Prettier code style.
- `npx tsc -b --pretty false` → bersih.
- `npx vitest run` → **56/56 passed** (7 files).
- `npx playwright test --workers=1 --reporter=line` → **45/45 passed (2.4m)**
  (43 dari run sebelumnya + 2 yang bertambah sejak log terakhir).
- `npm run build` → sukses (1.21s).
- `python tools/verify_web.py` → **SEMUA GATE BERSIH (0 FAIL, 0 warn)**.

### Kerapian working tree (bagian "rapikan" saat tutup)

- 4 screenshot lepas di `frontend/*.png` (`auth-login/register/reset-*`) →
  dipindah **keluar repo** ke `$LOCALAPPDATA/Temp/agentdeck-shots/`.
- `.gitignore` ditambah: `/api.exe` + `*.exe` (binary build tidak boleh masuk
  commit, sesuai brief), `frontend/*.png`, `docs/.backup/` (snapshot pra-format
  `00-PRD.pre-format.*.md` bukan bagian kontrak). Ketiganya sebelumnya
  **tidak** ter-ignore dan muncul sebagai untracked.
- `docs/CHECKLIST.md` di-regenerate ulang (`update_checklist.py`) → hash identik
  (`011f51ab…`), jadi tabel di repo sudah sinkron dengan sidecar.
- **Tidak di-commit.** Working tree masih memuat kerja M0+M1 yang belum
  di-commit (210 file baru, 46 penghapusan `apps/web/*` yang sudah dipindah ke
  `frontend/`). Brief tidak meminta commit, dan 256 path sekaligus terlalu besar
  untuk diputuskan unattended — ini keputusan manusia, dicatat di sini.

### Sisa milestone

**M0 SELESAI** (9 PASS, 1 ditunda `US-AD92`). **M1: 1 PASS, 31 sisa.**
Story M1 berikutnya yang harus dikerjakan: **`US-AD09` dilanjutkan** — prioritas
pertama adalah CTA `New board` + modal di `BoardList.tsx` (clone
`10-board-list.html` class-for-class) lalu e2e design-match, atau lanjut ke
`US-AD10` (editor kolom) bila user memutuskan urutan lain.

**Hambatan:** tidak ada yang butuh manusia untuk gate; yang butuh keputusan
manusia hanya (a) apakah `register` harus membuat project default demi AC4, dan
(b) kapan kerja M0+M1 ini di-commit.

---

## 2026-09-20 09:10–09:22 — PENUTUPAN (tidak buka story baru)

**Konteks run.** Waktu host saat run mulai **09:10** → sudah ≥ 09:00, jadi sesuai
bagian "Batas waktu" brief run ini **tidak membuka pekerjaan baru**. Isinya:
verifikasi kondisi repo, kerapian, dan ringkasan penutup. Tidak ada story yang
diubah statusnya.

**Lingkungan.** `agentdeck-api` healthy, `agentdeck-db` healthy, `agentdeck-web`
up; `curl /readyz` → `ready` (200). Bridge WhatsApp `127.0.0.1:3000` sehat.

**Tidak ada pekerjaan menggantung yang setengah jadi.** Dicek dengan
`find … -newermt '2026-09-20 09:06'`: **nol** file source berubah sejak gate hijau
run sebelumnya (09:05). Jadi tidak ada edit liar yang perlu dibersihkan.

### Gate lengkap dijalankan ULANG run ini (SEKUENSIAL) — semua hijau

Bukan mengutip hasil run sebelumnya; semuanya dieksekusi ulang di run ini:

| Gate | Hasil |
|---|---|
| `gofmt -l .` | kosong (OK) |
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `go test ./...` | OK — `cmd/api`, `internal/auth`, `internal/board`, `internal/config`, `internal/migrate` (cached, nol FAIL) |
| `npx prettier --check src e2e` | All matched files use Prettier code style |
| `npx tsc -b --pretty false` | bersih |
| `npx vitest run` | **56/56 passed** (7 files, 5.8s) |
| `npx playwright test --workers=1` | **45/45 passed (2.3m)** |
| `npm run build` | sukses (1.31s) |
| `python tools/verify_web.py` | **SEMUA GATE BERSIH (0 FAIL, 0 warn)** — 9 cek (router, nav, dep, format, banned, SPA, struct 38 path, fetch, react19) |

### Kerapian working tree

- Screenshot lepas di `frontend/*.png` → sudah bersih (0 file); `api.exe` dan
  `frontend/*.png` sudah ter-ignore (`.gitignore:6`, `.gitignore:16`).
- Nol `.bak`/`.orig`/`.rej` sisa mutation-test di repo. `docs/.backup/`
  (snapshot pra-format) ada tapi ter-ignore (`.gitignore:20`).
- Nol secret ter-hardcode di source (grep `sk-`/`xpl_`/`AKIA`/private key →
  kosong). `.env` **tidak** ter-tracked (hanya `.env.example` yang untracked, isi
  placeholder `***`).
- `docs/CHECKLIST.md` di-regenerate dari sidecar → **10 PASS, 12 tracked** (naik
  dari 9 PASS karena `US-AD08` M1 kini tercatat PASS). Tabel sudah sinkron dengan
  `tools/checklist_status.json`.
- **Sisa folder kosong `apps/web/`** (46 file tracked sudah dihapus dan dipindah
  ke `frontend/`, folder-nya tinggal kosong). Percobaan hapus ditolak dua kali:
  `rmdir` → *Device or resource busy* (sisa handle penjelajah/editor), dan
  `cmd /c rd /s /q apps` **ditahan approval** dan tidak dijalankan — konsisten
  dengan aturan brief untuk tidak menunggu approval. **Tidak masalah**: git tidak
  melacak folder kosong, jadi `git add -A` tetap menghasilkan 46 penghapusan yang
  benar. Boleh dihapus manual kapan saja (`rmdir apps\web apps`).

### Working tree — sengaja TIDAK di-commit

Masih ada **100 path** (33 untracked, 46 dihapus, 21 diubah). Volume ini
(kerja M0+M1 sekaligus, termasuk migrasi DB 0004–0007) bukan keputusan yang boleh
diambil unattended, dan brief tidak memintanya. `origin` =
`https://github.com/v-reza/agentdeck.git` (publik) — jadi ini memang aksi keluar
yang butuh konfirmasi manusia. Dicatat sebagai keputusan manusia, bukan
hambatan gate.

### Ringkasan penutup

- **M0: SELESAI** — 9 PASS, 1 ditunda (`US-AD92`, keputusan user).
- **M1: 1 PASS dari 32** (`US-AD08`). **M2–M4: 0.**
- **Total: 10 PASS dari 85 story.**
- Story M1 yang sedang berjalan: **`US-AD09` (membuat board kanban) tetap `wip`
  — TIDAK di-PASS-kan** karena gate story belum lengkap (lihat blok run
  sebelumnya): CTA `New board` + modal belum ada di `BoardList.tsx`
  (`useCreateBoardMutation` masih nol pemakaian di `*.tsx`, diverifikasi ulang
  run ini), belum ada e2e `kanban.spec.ts`, dan AC4 belum terpenuhi.
- **Berikutnya (run pagi/manusia):** lanjutkan `US-AD09` — clone
  `design/stitch-output/v2/10-board-list.html` class-for-class untuk CTA
  `New board`, tulis `frontend/e2e/kanban.spec.ts` design-match, lalu selesaikan
  AC4 (keputusan: apakah `POST /auth/register` membuat project default).

**Hambatan butuh manusia (tidak bisa diselesaikan unattended):**
1. Commit + push kerja M0+M1 (100 path; remote publik).
2. Keputusan AC4 `US-AD09`: `register` bikin project default atau tidak.
3. Hapus sisa folder kosong `apps/web/` (opsional, kosmetik).

**Ping WhatsApp:** terkirim 1× (status berubah) via bridge lokal →
`{"success":true,"messageId":"3EB0060C045A0B20E2B050"}`.

---

## 2026-09-20 09:20 — VERIFIKASI TUTUP (tidak buka story baru)

Waktu host **09:20 ≥ 09:00** → tetap dalam mode penutupan. Tidak ada story dibuka.

- **Tidak ada perubahan sejak penutupan 09:22:** `find … -newermt '2026-09-20 09:22'`
  → **nol** file source berubah; `git status --short` tetap **100 path**.
- **Lingkungan hidup:** `agentdeck-api` healthy (46m), `agentdeck-db` healthy,
  `agentdeck-web` up; `curl /readyz` → `ready` (200). Bridge WhatsApp
  `127.0.0.1:3000` merespons (HTTP 404 di `/`, artinya port hidup).
- **Working tree bersih dari artefak:** nol `.png`/`.exe`/`.log`/`node_modules`
  ter-tracked atau lepas di repo; screenshot hanya di
  `$LOCALAPPDATA/Temp/agentdeck-shots` (7 file, di luar repo).
- **Status tidak berubah** → **10 PASS** (9 M0 + `US-AD08`), `US-AD09` tetap
  `wip`, `US-AD92` ditunda. Sidecar `tools/checklist_status.json` sinkron dengan
  `docs/CHECKLIST.md` (85 story).
- **Ping WhatsApp: tidak dikirim** — aturan brief: hanya kalau status berubah.
- Sisa `apps/` masih kosong dan *device busy* saat di-`rmdir` (kosmetik, sudah
  dicatat; git tidak melacak folder kosong).
