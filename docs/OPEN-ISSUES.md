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

- **US-AD03**: `DELETE /api/v1/orgs/{id}` — nol route. Diverifikasi: hanya
  `/orgs/{id}/members/{user_id}` yang terdaftar.
- **`LICENSE`, `NOTICE`, `THIRD_PARTY` belum ada.** Plus konflik lisensi di design:
  `09b-github.html` menulis Apache-2.0 sementara `05-landing.html` menulis MIT.
  Repo ini publik, jadi ini utang yang kelihatan.
- **`models_json` belum pernah dipakai backend** untuk memvalidasi `model` saat
  registrasi agent. Validasi cuma hidup di UI; jalur API menerima apa pun yang
  bentuknya benar.

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
