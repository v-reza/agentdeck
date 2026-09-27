# OPEN-ISSUES — AgentDeck

Isu yang belum beres, dipindahkan dari `docs/HANDOFF.md` waktu file itu diarsipkan.
**Tiap baris di sini diverifikasi ulang ke kode/DB pada 2026-09-27** — bukan dikutip
dari dokumen lama. Kalau sebuah klaim di sini berhenti benar, betulin **di sini**;
jangan pindahkan kembali ke dokumen status.

Ini bukan daftar keinginan. Yang di bawah semuanya masih terbuka kecuali ditandai
coret.

## Endpoint & modul yang belum ada

- **51 endpoint ⬜**, dihitung dari tabel detail `ARCHITECTURE.md` §6.2.1–§6.2.19
  (bukan §6.2.20 — ringkasan itu basi). Dijaga dua arah oleh `verify_suite.py`.
  Yang paling utuh belum disentuh: API keys (5), SSE (4), approvals (5),
  artifacts (5), comments (4), webhooks (7), audit/notif/search/system (6).
- **Runtime**: workspace container/worktree per run (US-AD12 — sekarang semua run
  jalan di `scratch`), `internal/sse`, `internal/storage`, `internal/webhook`.
- `POST /boards/{id}/budget` (M5).

## Belum dibangun / belum ada


## Bahasa & konsistensi

- **Pesan error backend berbahasa Inggris**, UI default Indonesia. Contoh terukur:
  `unsupported task status`, `malformed JSON body`, `insufficient role`,
  `api_key must not be empty`. Bukan blocker, tapi terbaca sebagai dua produk.

## Design (tercatat, bukan blocker)

- **Icon**: impl memakai ~3 `<svg>` di tempat design memakai 319. Sisa kelas visual
  terbesar; angkanya keluar dari `tools/design_audit.py`, bukan hitungan tangan.
- **`US-AD54 AC2`** — multi-sort dengan klik header kolom. Milik M5, belum dikerjakan;
  `TableView.tsx` header belum bisa diklik dan belum punya ikon sort.
- **Halaman agent registry belum nemu "titik enaknya".** Pertanyaan "informasi apa
  yang harus ada di layar ini" belum dijawab, dan itu **butuh sesi design non-coding**,
  bukan tambahan kode.
- Dua gap ikon `05-landing` (check bergaris vs terisi, `→` teks vs `ArrowRight`)
  **sudah dibetulin** — commit `5100e22`.

## Konflik kontrak yang masih berdiri

- **US-AD05 AC3 vs US-AD90 AC4 — mencabut sesi sendiri.** AC3 (US-AD05, "Session
  logout paksa", `Could` · M5) bilang mencabut sesi sendiri lewat endpoint cabut
  paksa **harus ditolak** ("gunakan logout biasa"). AC4 (US-AD90) bilang pengguna
  **dapat** mencabut sesinya sendiri dari daftar perangkat. Dua-duanya mengatur
  endpoint yang sama, `DELETE /api/v1/auth/sessions/{id}`.
  **Arah operasional:** baris §6.2.2 ARCHITECTURE memilih AC4, dan itu yang
  diimplementasikan (F3) — daftar sesi yang barisnya sendiri tidak bisa dihapus
  adalah daftar dengan tombol yang tidak melakukan apa-apa. AC3 tetap berlaku
  untuk `DELETE /api/v1/auth/me` (tutup akun), yang memang menutup akun, bukan
  "cabut paksa sesi sendiri lalu pakai logout".
  **Belum diputuskan user.** Kalau AC3 yang diinginkan, yang berubah cuma satu
  cabang di `internal/auth/sessions.go:RevokeSession`.

- **Approval gate: US-AD34 AC1 / US-AD35 AC1 vs ARCHITECTURE §5.3 — ke mana task
  pergi setelah diputuskan.** PRD bilang approve mengembalikan task ke `running`
  dan reject memblokirnya dengan `needs_input`. Tabel transisi §5.3 dan baris
  tabel §6.2.14 bilang `ready` dan `blocked(policy)`.
  **Arah operasional:** `ready` + `blocked(policy)` (F7). Kontrak yang lebih
  spesifik menang, dan `running` tidak bisa dipenuhi jujur: run-nya sudah
  ditutup saat gate dipasang (task tidak boleh menahan run hidup sementara
  tidak ada yang menjalankannya), jadi "kembali ke running" hanya bisa berarti
  mengarang run baru. `needs_input` juga salah makna — `needs_input` berarti
  agen butuh pertanyaan dijawab; di sini **manusia** yang menolak, dan
  `policy` itu namanya.
  **Belum diputuskan user.** Yang berubah kalau PRD menang: dua nilai status di
  `internal/board/approval.go` (`ApprovalDecision` → status tujuan).

- **Approval gate: PRD US-AD33/34/35 (Member) vs ARCHITECTURE §11.3 (Admin).**
  PRD membolehkan Member membuat **dan** memutuskan gate; matriks RBAC §11.3
  memberi Admin untuk approve/reject.
  **Arah operasional:** buat gate = Member (US-AD33 AC3 memang melarang Viewer,
  dan worker yang meminta gate bukan Admin), putuskan gate = Admin (§11.3).
  Alasannya satu baris: Member yang boleh membuat gate sekaligus memutuskan
  gate-nya sendiri berarti gate itu tidak menghalangi apa pun — ia tinggal
  menekan approve. Itu membatalkan alasan fitur ini ada.
  **Belum diputuskan user.** Yang berubah: satu argumen role di
  `cmd/api/approvals.go` (dua route).

- **`POST /tasks/{id}/approvals` terdaftar sebagai Member, kontraknya Worker.**
  §6.2.14 + §1684 bilang endpoint ini `Auth: Internal/Key`, aktor `Worker`
  (`api_keys`, `adk_...`). Mekanisme itu **belum ada** (`api_keys` belum
  dibuat), dan endpoint Worker lain yang sudah ✅ (`runs/{id}/heartbeat`,
  `/end`, `/steps`) menghadapi hal yang sama dan diregistrasi `auth.Member`
  sejak awal. F7 mengikuti preseden itu supaya tidak menciptakan jalur
  autentikasi tandingan. **Yang menahan ini:** begitu `api_keys` dibangun,
  keempat route Worker itu harus pindah bersama-sama, bukan satu-satu.

- **`api_keys` sudah ada (F10, 6.2.3) — route Worker belum dipindah.**
  Mekanismenya kini jalan: bearer `adk_...` diautentikasi di
  `contextMiddleware`, dan key mewarisi `role` pemiliknya dari `memberships`
  (2291). Tapi keempat route Worker (`POST /tasks/{id}/approvals` dari F7, plus
  `runs/{id}/heartbeat`, `/end`, `/steps`) **masih** terdaftar `auth.Member`
  dari preseden lama. Pemindahannya belum dikerjakan dan itu disengaja:
  mengubah gerbang otentikasi empat endpoint sekaligus adalah perubahan
  tersendiri dengan tesnya sendiri, bukan efek samping penambahan `api_keys`.
  **Yang menahan ini:** belum ada tes yang membuktikan key agent tidak bisa
  dipakai untuk endpoint sesi, dan sebaliknya — itu yang harus ada lebih dulu.

- **Tiga endpoint 6.2.19 terdaftar `auth.Viewer`, kontraknya `Session/Key`.**
  §6.2.19 bilang `audit-log`, `search/tasks`, dan `search/runs` menerima API key
  (`Auth: Session/Key`). Isu yang sama seperti `POST /tasks/{id}/approvals` di
  atas: `api_keys` belum ada, jadi ketiganya diregistrasi sesi saja. Ini lebih
  ringan daripada kasus Worker — endpoint ini baca-saja, dan API key nanti
  otomatis punya role yang sama — tapi tetap satu keluarga: **semua route
  `Session/Key` harus pindah bersama-sama begitu `api_keys` dibangun.**

- **`notifications` cuma punya dua produser dari empat `kind`.** Kontrak §3.21
  menyebut empat: `approval.requested`, `budget.warning`, `run.failed`,
  `credential.invalid`. Yang benar-benar ditulis sekarang cuma
  `budget.warning` (ambang 80% di `checkRunBudget`) dan `run.failed` (akhir run
  gagal). `approval.requested` belum disambungkan ke `POST
  /tasks/{id}/approvals`; `credential.invalid` belum punya titik picu (yang
  paling masuk akal: kegagalan dekripsi kredensial provider, atau probe model
  yang ditolak). Keduanya tinggal memanggil `NotifyOperational`, bukan pekerjaan
  skema.

- **`notifications` tidak punya retensi.** Tidak ada sapuan yang menghapus baris
  lama, jadi inbox tumbuh tanpa batas. `GET /notifications` sudah punya `limit`,
  jadi ini bukan masalah kebenaran — hanya pertumbuhan. Pola yang sama sudah ada
  untuk approval (`ExpireDueApprovals` di tick), jadi tempatnya jelas kalau
  nanti perlu.

## Isu terbuka yang baru diverifikasi

- **Dua definisi "hari ini" untuk biaya board.** `BoardSpendToday` (jumlah
  `ledger_entries`) memakai `date_trunc('day', now())` — zona server.
  `BoardBudgetToday` + `UpsertDailyBoardCost` memakai `(now() AT TIME ZONE 'UTC')::date`
  — UTC. Di DB container sekarang (TZ=UTC) hasilnya identik, jadi ini laten, bukan
  aktif. Begitu Postgres-nya bukan UTC, `GET /boards/{id}/ledger`
  (`spend_today_micros`) dan `GET /boards/{id}/budget` (`spent_micros`) akan
  menampilkan dua angka berbeda untuk hari yang sama. **Belum diperbaiki**:
  menyentuh `BoardSpendToday` yang sudah ✅, dan memilih zona waktu itu keputusan
  produk (UTC vs zona pengguna), bukan keputusan implementasi.

- **Frontend belum memakai stream SSE.** Empat endpoint §6.2.13 jalan dan
  terbukti lawan API nyata (`tools/probe-f11.py` 21/21, termasuk rantai penuh
  INSERT → trigger NOTIFY → LISTEN → frame), tapi UI masih memakai pola
  revalidate/polling yang sama seperti sebelumnya. Backend-nya siap; yang belum
  ada adalah kliennya. **Belum dikerjakan** — menyambung UI adalah perubahan di
  `frontend/`, bukan penambahan endpoint.

- **`events` tidak punya retensi.** §3.12 tidak menyebut TTL, dan N6 menyebut
  target 1.000.000 event/bulan. Dengan trigger NOTIFY sekarang, setiap INSERT
  juga membangunkan setiap hub yang LISTEN. Belum ada apa pun yang memangkas
  tabel ini, jadi pertumbuhannya linier terhadap aktivitas selamanya. **Belum
  dikerjakan**: kontrak tidak menetapkan kebijakan retensi, dan memilihnya
  (mis. 90 hari) adalah keputusan produk.

## Batasan yang diketahui (bukan bug)

- **Gerbang peran tidak bisa dibuktikan lewat e2e** — fixture selalu owner. Cakupan
  peran cuma unit test (`use-can-act.test.tsx`). Menambah fixture role = kerjaan
  tersendiri.
- **`steps.payload_json` itu JSONB**: Postgres menormalkan urutan kunci + spasi.
  Klaimnya "setia pada isi, bukan byte" — jangan ada yang menulis ulang jadi "verbatim".
- **Dispatcher AgentDeck mati** kecuali `AGENTDECK_DISPAT` di-set. Tick yang hidup
  membelanjakan kredensial operator.

## Arsip

- Status per 2026-09-21 (provider registry fase 0–6, audit F3, restructure dokumen)
  → `archive/reports/HANDOFF-2026-09-21.md`. **Jangan** dipakai sebagai sumber kebenaran.
- `OVERNIGHT-BRIEF.md` / `OVERNIGHT-LOG.md` versi lama → `archive/reports/`.
