# Audit PRD & Coverage Design — AgentDeck

Tanggal: 2026-09-18. Dijalankan sebelum generasi layar massal.

## Ringkasan

| Item | Status |
|---|---|
| Story | 87 (US-AD01..US-AD87, nol gap, nol duplikat) |
| Acceptance Criteria | 268 (semua di §6, nol story tanpa AC) |
| Gate `verify_prd.py` | PASS (exit 0) — diperkuat, lihat §1 |
| Gate `verify_suite.py` | PASS (exit 0) |
| Prompt pack | 18 layar, semua v2, gate PASS |
| **Coverage story → layar** | **40 dari 87 story belum punya layar** |
| **Kesesuaian B2C** | **BELUM — PRD masih B2B penuh** |

---

## 1. Bug yang ditemukan dan diperbaiki

**AC milik `US-AD87` nyasar ke §14 (Lampiran: Peta Kompetitif).**

Empat AC (`AC1`..`AC4`) berada di baris 864–867, di dalam bagian lampiran
kompetitif, jauh setelah section katalog story berakhir. Akibatnya:

- `US-AD87` (Must, M4) **punya 0 acceptance criteria** — story tanpa kontrak.
- Counter lama tetap melaporkan 268 AC karena ia menghitung seluruh file,
  sehingga gate melaporkan "ok" untuk suite yang rusak.

Diperbaiki: AC dipindahkan kembali ke bawah `US-AD87`. Kini 268 AC semuanya
berada di §6, dan nol story tanpa AC.

**Gate diperkuat** (`verify_prd.py`) dengan dua cek baru:

1. AC di luar section katalog (§6) → FAIL.
2. Nomor AC di dalam katalog harus berurutan `AC1..ACn` tanpa lompatan → FAIL.

Diverifikasi dengan negative test: satu AC dipindah ke §14 → gate FAIL
(`AC3 in §14`, `numbering not sequential: [[1, 2, 4]]`); setelah dipulihkan →
PASS. Gate yang tidak pernah gagal tidak membuktikan apa pun.

---

## 2. Gap PRD — fitur ada di spec lain, tapi tanpa story

Ini yang paling penting: **kemampuan ini sudah dirancang dan diimplementasikan
di kontrak, tetapi tidak ada satu pun user story yang memilikinya.**

| Gap | Bukti keberadaan | Story |
|---|---|---|
| Reset password (request + reset) | `POST /api/v1/auth/password/reset-request`, `POST /api/v1/auth/password/reset` ada di ARCHITECTURE | **0** |
| Cost rail (panel biaya kanan) | `shell-costrail`, `costrail-panel`, `budget-meter`, `budget-meter-warning`, `budget-meter-over`, `sparkline-bar` — 6 komponen di DESIGN.md | **0** |
| Profil & pengaturan akun sendiri | `GET /api/v1/auth/me`; `avatar-user` di DESIGN | **0** |
| Board list / project list sebagai layar | `GET /api/v1/projects`, `GET /api/v1/projects/{id}/boards` | **0** |
| Navigasi (rail + sidebar) sebagai kemampuan | `shell-rail`, `rail-icon`, `sidebar-item` di DESIGN | **0** |
| Ganti password, tutup akun | tidak ada di mana pun | **0** |

`POST /api/v1/auth/password/reset` adalah temuan paling tajam: endpoinnya
dikontrak, tapi tidak ada story, tidak ada AC, dan tidak ada layar. Fitur
yang tidak bisa dijual dan tidak bisa dites.

---

## 3. Coverage story → layar

Pemetaan 87 story ke 18 layar yang ada. **40 story tidak terpetakan.**

Setelah dipisah berdasarkan sifat pekerjaannya:

| Kategori | Jumlah | Butuh layar baru? |
|---|---|---|
| Backend-only | 31 | Tidak |
| Campuran (UI + backend) | 39 | Sebagian |
| UI-only | 17 | Ya |

Artinya **"87 story = 87 prompt" adalah asumsi yang salah.** Banyak story
adalah kontrak server (dispatcher, heartbeat, rate limit, retry backoff, deteksi
string keras di CI) yang tidak punya wujud visual.

### Layar yang jelas hilang

Berdasarkan story, layar berikut dibutuhkan tetapi belum ada prompt-nya:

- Reset password + konfirmasi reset
- Profil / pengaturan akun sendiri (ganti password, sesi aktif, tutup akun)
- Board list (daftar board lintas project)
- Run detail (US-AD41, US-AD26 — timeline step per run)
- Audit log viewer (US-AD51 — ada endpoint `GET /api/v1/audit-log`)
- Agent create/edit form (US-AD20, US-AD67, US-AD86 — model, provider, kredensial)
- Board create/edit + kolom editor (US-AD09, US-AD10, US-AD83, US-AD84)
- Project create/edit (US-AD08)
- Org/workspace create + settings (US-AD03, US-AD75, US-AD77)
- Command palette (US-AD55)
- Halaman Docs, Pricing, GitHub (dari landing page)
- Notifikasi in-app (US-AD61)

---

## 4. Kesesuaian B2C — BELUM

`00-PRD.md` belum diubah sejak 2026-09-16 22:04. Perubahan B2B → B2C sejauh ini
baru menyentuh prompt pack dan landing page, **belum PRD**.

PRD saat ini masih B2B penuh:

| Indikator | Nilai |
|---|---|
| Story menyebut `org` | 22 |
| Story menyebut `anggota` | 32 |
| Story menyebut `owner` / `admin` / `viewer` | 21 / 27 / 29 |
| Goal G5 | "Isolasi multi-tenant", filter `org_id` di SEMUA query |
| `US-AD01` AC1 | "…dan Org default dibuat" — pendaftar otomatis jadi `owner` |
| `US-AD03` | Manajemen organisasi (CRUD) — Must |
| `US-AD04` | Manajemen anggota & role — Must |
| `US-AD78` | Invite link — Should |
| `self-serve` / `free tier` / `tanpa org` | 0 |

Untuk "B2C bisa semua orang", ini berarti:

1. Alur pendaftaran tidak boleh memaksa pemahaman konsep org.
2. Fitur kolaborasi (`US-AD03`, `US-AD04`, `US-AD05`, `US-AD78`) harus jadi
   opsional, bukan prasyarat untuk memakai produk.
3. Pricing harus punya jalur individual, bukan hanya per-seat.

**Catatan penting:** `US-AD75` sudah memperkenalkan `workspace_kind`, yang
mengindikasikan desain personal/kolaboratif sudah pernah dipikirkan. Itu
titik masuk yang tepat untuk B2C — bukan membongkar multi-tenant, melainkan
membuat org transparan bagi pengguna solo.

---

## 5. Batas penting: "full interactive"

268 AC saat ini adalah **kontrak API** — status code HTTP, baris database,
header, idempotency. Contoh:

> AC1 Registrasi dengan email valid + password minimum 8 karakter
> mengembalikan 201, session token dikirim, dan Org default dibuat.

Itu tidak dapat dipakai untuk membuat UI yang interaktif. Yang belum ada:

- state UI per layar (default / hover / focus / active / disabled / loading /
  empty / error / partial)
- urutan interaksi (apa yang terjadi setelah tombol ditekan)
- transisi antar layar dan kondisi baliknya
- validasi inline dan pesan galat yang terlihat pengguna
- perilaku saat data kosong, gagal, atau lambat

Untuk "lihat design full interactive sesuai user story", dibutuhkan satu
lapisan baru: **UI State Contract per layar**. Tanpa itu, 87 prompt hanya
menghasilkan tangkapan layar statis.
