# OVERNIGHT-LOG — AgentDeck

Append-only. Satu entri per fase, **ditulis sebelum lanjut ke fase berikutnya**.
Entri yang gagal tetap ditulis apa adanya — log yang cuma berisi keberhasilan
bukan log.

Format: `## <fase> — <judul>` lalu status, endpoint, file, bukti, masalah.

---

## F0 — Setup (2026-09-26 malam)

**Status:** selesai.

Yang diverifikasi sebelum loop dimulai, semuanya diukur bukan ditebak:

| Yang diukur | Hasil |
|---|---|
| Suite penuh (untuk gate) | **670s** — mustahil, plafon gate 300s |
| `go test ./... -short` | **490s** (nol `testing.Short()` di repo) |
| `go build ./...` / `go vet` (cache kosong) | 82.8s / 83.2s |
| Gate ringan (yang dipakai) | **20–52s** |
| Gate lawan pohon kotor | EXIT 1 dalam 0.9s, nyebut `gofmt` |
| Gate lawan pohon bersih | EXIT 0 |
| `verify_suite.py` | 0 FAIL, 78 ✅ / 51 ⬜ |

**Temuan yang ngeubah rencana:**

1. `tools/gate.cmd` (milik sesi lain) isinya suite penuh 670s → **nggak bisa jadi gate**.
   Diganti `tools/gate-overnight.cmd`, dan gate-nya diuji dua arah.
2. **§6.2.20 (ringkasan per modul) di dokumen itu basi.** Dia bilang Runs 0/6;
   kenyataannya cuma 2 ⬜. Hitungan dari tabel detail: **78 ✅ / 51 ⬜**, bukan 66/61.
   Urutan fase di brief diambil dari tabel detail, bukan ringkasan.
3. Jumlah sisa endpoint = **51**, bukan 61. Tabel ringkasan yang bikin angka 61 salah.

**Keputusan user yang mengikat malam ini:**

- Mesin: `/goal` di sesi ini + gate + cron watchdog.
- Urutan: urut modul, **termurah dulu** (lihat brief §2).
- Gate: terima versi ringan; `go test` manual per fase dan dilaporkan.
- Durasi: sampai dimatiin manual — budget tinggi.
- **Kanban tidak disentuh.** Board basi dibiarkan apa adanya.
- Dispatcher AgentDeck tetap **OFF**.

**Batas yang gw nggak bisa lewatin:** gw nggak bisa ngetik `/goal` sendiri — itu
slash command CLI. User yang mulai; setelah itu loop jalan sendiri.

---

## F1 — 6.2.1 Health, Liveness & Metrics (2 endpoint)

**Status:** selesai, ter-push.

| Endpoint | Status |
|---|---|
| `GET /livez` | ✅ |
| `GET /metrics` | ✅ |

**File:** `internal/metrics/{metrics.go,middleware.go}` (BARU),
`cmd/api/metrics.go` (BARU), `internal/config/config.go` (+`MetricsAuth`),
`cmd/api/main.go` (registry + middleware bungkus mux), `go.mod` + `Dockerfile.api`
(go 1.22 → 1.24), `compose.yaml` (+`AGENTDECK_METRICS_AUTH`),
`tools/probe-metrics.py` (BARU).

**Bukti:**
- `go test ./internal/metrics/` **12/12**, `./cmd/api/` hijau (6 tes baru), `./internal/config/` hijau.
- `tools/probe-metrics.py` **15/15 hijau lawan API nyata**: `/livez` 200,
  `/metrics` 401 tanpa kredensial / 200 dengan kredensial, 13 metrik §14.2
  terdeklarasi, label `path` pola route (bukan URL), `/healthz` dikecualikan,
  histogram punya `_bucket`/`_sum`/`_count`.
- Mutation **2/3 CAUGHT**: guard auth dibuang → CAUGHT; label pakai URL mentah →
  CAUGHT. Yang **LEWAT** (jujur): `subtle.ConstantTimeCompare` → `==` tidak bisa
  dibuktikan test — nol tes bisa mengukur timing. Dicatat di kode sebagai gap.
- `verify_suite.py` 0 FAIL · gate `tools/gate-overnight.cmd` rc=0.

**Tiga bug nyata yang ketemu dari test, bukan dari membaca:**
1. `r.Pattern` Go menyertakan **method** (`"GET /api/v1/tasks/{id}"`). Label jadi
   `path="GET /api/v1/tasks/{id}"` — duplikat label `method`, dan nol query
   dashboard yang cocok. Ditemukan lewat probe `t.Logf`, bukan dugaan.
2. `Observe` menyimpan bucket **sudah kumulatif**, `WriteTo` mengakumulasi lagi →
   kuadrat. Histogram 2 sampel tercetak 1,2,3,5,7,9,11,13. Ketemu karena tes
   kumulatif gagal.
3. `":"` (user kosong + password kosong) dianggap "configured" → endpoint terbuka
   dengan kredensial yang bisa ditebak siapa pun. Sekarang ditolak.

**Keputusan yang gw ambil, dan kenapa:**
- **Hand-roll registry, bukan `prometheus/client_golang`.** P1 ("nol dependency
  yang bisa rusak saat major version naik") + N14 (binary ≤ 30 MB). Trade-off:
  nol quantile histogram, nol exemplar, nol process collector — dicatat di §14.2.
- **Metrik tanpa produsen tetap dideklarasikan, tapi tanpa baris nilai.** Kalau
  dicetak `0`, "belum ter-instrumentasi" tidak bisa dibedakan dari "jalan sepi".
  Kolom `Produsen` di §14.2 menyebut satu per satu mana yang belum.
- **`/metrics` tertutup default (404).** Label set-nya membawa `board_id`/`org_id`
  = topologi tenant. Instalasi self-hosted yang tidak dikeraskan tidak boleh
  mempublikasikannya hanya karena port-nya terbuka.
- **`/livez` tidak menyentuh DB.** Liveness yang gagal saat Postgres mati bikin
  container di-restart — itu tidak memperbaiki Postgres, dan mengubah gangguan
  dependency jadi restart loop. Cek DB tempatnya di `/readyz`.

**Konflik dokumen yang diselesaikan:** `go.mod` + Dockerfile + §2757 bilang Go
1.22, tapi §127 (tabel tech stack) bilang **Go 1.24**. Dipakai 1.24 — yang lebih
spesifik menang, dan "1.22+" di §1 tetap terpenuhi. Naik ke 1.24 juga yang
membuat `r.Pattern` tersedia (butuh ≥1.23). `go.mod` + Dockerfile ikut dinaikkan.

**Yang belum ter-instrumentasi (jujur):** seluruh kelompok dispatcher, SSE,
budget, dan db di §14.2. Endpoint-nya jalan dan metriknya terdeklarasi; yang belum
ada adalah pemanggilnya di jalur runtime. Itu kerjaan fase berikutnya, bukan klaim
bahwa ini sudah selesai.

## F2 — 6.2.9 Tasks (3 endpoint sisa)

**Status:** selesai, ter-push.

| Endpoint | Status |
|---|---|
| `POST /tasks/{id}/cancel` | ✅ |
| `POST /tasks/{id}/retry` | ✅ |
| `POST /tasks/{id}/archive` | ✅ |

**File:** `internal/migrate/0015.up.sql` (BARU — `runs.cancel_requested_at`),
`internal/board/lifecycle_test.go` (BARU), `cmd/api/tasks_lifecycle_test.go` (BARU),
`tools/probe-lifecycle.py` (BARU). Diedit: `internal/store/queries/queries.sql`
(+5 query), `internal/board/{pgx.go,service.go,types.go,runtime.go}`,
`internal/dispatcher/{run.go,tick.go,dispatcher.go,dispatcher_test.go}`,
`cmd/api/{runs.go,boards_columns_test.go}`, `docs/ARCHITECTURE.md`.

**Bukti:**
- `go test ./internal/board/` **+14 tes** hijau; `./cmd/api/` hijau (10 tes baru);
  `./internal/dispatcher/` hijau (2 tes baru).
- `tools/probe-lifecycle.py` **20/20 hijau lawan API nyata**: tiga route terdaftar,
  migrasi `0015` benar-benar ada di DB container, cancel idempoten, retry reset
  counter, archive 409 dari non-terminal + idempoten dari terminal, task tak
  dikenal 404 di ketiga route.
- Mutation **3/3 CAUGHT** (guard terminal dibuang; guard `running` dibuang;
  permintaan cancel tidak ditulis ke `runs`). Satu mutasi BUILD-FAIL — nol
  kontribusi, tidak dihitung.
- `verify_suite.py` 0 FAIL (83 ✅ / 46 ⬜) · gate rc=0.

**Keputusan yang gw ambil, dan kenapa:**
- **Cancel ditulis ke DB, bukan ke memori proses.** Cancel-nya cuma map
  `cancelled[runID]` di dispatcher, dan §66 mengizinkan `-role=api|dispatcher`
  sebagai proses terpisah. Dalam konfigurasi itu, `POST /tasks/{id}/cancel`
  menulis flag di proses API sementara run-nya jalan di proses dispatcher —
  pembatalannya dilaporkan 200 dan tidak pernah terjadi. Sinyalnya sekarang
  kolom `runs.cancel_requested_at`, dibaca dispatcher sebelum run dimulai dan di
  antara step.
- **`outcome=cancelled` dipisah dari `budget_exceeded`.** Dulu `sink.cancelled`
  cuma dipakai jalur budget, jadi memakai flag yang sama buat cancel manusia
  bikin run-nya tercatat `budget_exceeded` dan task-nya masuk `blocked(budget)` —
  salah alasan, dan memblokir task karena budget yang tidak bermasalah.
- **Retry manual = override operator, bukan retry otomatis.** §10.1 mengirim
  `capability`/`policy` ke No-Retry, tapi itu kebijakan *otomatis*. Endpoint ini
  ada untuk manusia yang sudah memperbaiki penyebabnya. Karena itu ia menerima
  semua status non-running; yang ditolak cuma `running`, supaya satu task tidak
  punya dua run.
- **Archive delegasi ke `MoveTask`**, bukan tulis status sendiri. Aturan
  terminal-only (AC2) dan idempoten (AC3) sudah ada di sana; jalur kedua = tempat
  kedua untuk melenceng. Gerbang Admin (AC4) di route.

**Tiga bug nyata yang ketemu, dua di antaranya dari probe lawan API nyata:**

1. **`GetTask` dan `GetRun` tidak memetakan `ErrNoRows` → `ErrNotFound`.**
   `GetBoard`/`GetAgent`/`GetProject` memetakannya, dua ini tidak. Akibatnya task
   atau run yang tidak ada balik **500**, bukan 404 — dan karena task milik tenant
   lain juga tidak ketemu, 500 vs 404 jadi **oracle keberadaan** lintas tenant.
   Diperbaiki di kelasnya, bukan di satu tempat.
2. **`retry` task tak dikenal balik 409, bukan 404.** Statement `RetryTask`
   menolak task `running` dengan mencocokkan nol baris, jadi "tidak ada" dan
   "sedang jalan" tidak bisa dibedakan — 409 untuk keduanya. Service sekarang
   membaca dulu. **Ini ketemu dari probe, bukan dari test**: fake-nya punya cacat
   yang sama, jadi tes unitnya lulus dua arah. Fake-nya ikut diperbaiki.
3. **Endpoint `archive` belum pernah ada.** Arsip sebelumnya cuma lewat
   `POST /tasks/{id}/move`, dan itulah kenapa US-AD59 AC4 sempat bocor (route-nya
   Member). Sekarang ada route khusus dengan gerbang Admin.

**Konflik dokumen:** nol. Baris `archive` di §6.2.9 sudah menulis Admin dan
kontraknya cocok dengan US-AD59 AC4.

**Catatan probe:** cookie sesinya `Secure: true`, dan `http.cookiejar` Python
menolak mengirim cookie Secure lewat HTTP — browser memperlakukan `localhost`
sebagai secure context, Python tidak. Probe-nya sekarang punya policy yang
menyatakan loopback aman. Tanpa itu setiap request setelah registrasi balik 401
dan probe-nya salah menyalahkan produk.

**Yang belum:** cancel baru efektif pada run yang **belum** membuat panggilan
provider pertama, atau pada step berikutnya. Executor cuma memanggil provider
sekali per run, jadi guard di antara step praktis tidak pernah punya langkah
kedua untuk menghentikan. Membatalkan di tengah panggilan HTTP butuh
`context.CancelFunc` per run — seam-nya jelas (executor sudah membuat ctx
per-run), tapi itu perubahan yang lebih besar daripada satu endpoint dan tidak
dikerjakan di fase ini. Dicatat sebagai kandidat F2b.

## F3 — 6.2.2 Auth & Sessions (4 endpoint sisa)

**Status:** selesai, ter-push.

| Endpoint | Status | Bukti |
|---|---|---|
| `DELETE /api/v1/auth/me` (US-AD98) | ✅ | probe 26/26 · `TestPgCloseAccountRemovesWorkspaceAndSessions` |
| `POST /api/v1/auth/password/change` (US-AD90 AC1/AC3) | ✅ | `TestPgChangePasswordKeepsOnlyTheCallersSession` |
| `GET /api/v1/auth/sessions` (US-AD90 AC2) | ✅ | `TestPgSessionsListShowsRecordedClientContext` |
| `DELETE /api/v1/auth/sessions/{id}` (US-AD90 AC4, US-AD05) | ✅ | `TestPgCrossTenantSessionRevokeDenied` |

**Migrasi:** `0016.up.sql` — `orgs.deleted_at` + index parsial. Tanpa ini US-AD98
AC2/AC5 mustahil: AC2 bilang workspace ikut ditutup, AC5 bilang penutupannya
lunak. Dua-duanya cuma bisa benar bersamaan kalau org punya penanda hapus.

**Gate:** `tools/gate-overnight.cmd` rc=0 · `verify_suite.py` 0 FAIL
(**87 ✅ / 42 ⬜**) · `go test ./internal/auth/ ./internal/store/... ./cmd/api/`
hijau · probe `tools/probe-sessions.py` **26/26** lawan API nyata.

**Mutation (7/7 CAUGHT):** lantai pemilik-sesi dibalik · gerbang admin dibalik ·
cek keanggotaan tenant dibalik · AC3 last-owner dibalik · AC2 hanya-anggota-sendiri
dibalik · verifikasi password lama dibalik · lantai panjang password dihapus.

**Temuan yang menentukan bentuk fase ini**

1. **`sessions.user_agent` dan `sessions.ip` sudah ada sejak awal, tapi tidak
   pernah ada penulisnya.** US-AD90 AC2 ("perangkat/user agent, IP") karena itu
   mustahil dipenuhi — endpoint-nya bisa mengembalikan daftar dengan kolom kosong.
   Penulisnya ditambahkan di `CreateSession`; `sessionMeta(r)` membacanya dari
   request. IP dari `RemoteAddr`, **bukan** `X-Forwarded-For`: header itu diisi
   pemanggil, dan daftar sesi yang bisa disuruh menampilkan alamat sembarang itu
   lebih buruk daripada daftar yang menampilkan alamat yang benar-benar terlihat.

2. **`GetSessionByTokenHash` (dan fake-nya) tidak menyaring sesi yang dicabut.**
   Akibatnya `DELETE /auth/sessions/{id}` melaporkan 204 sambil token itu tetap
   bisa dipakai: pencabutan hanya menghapus baris dari DAFTAR, bukan mematikan
   tokennya. Ini kelas bug yang sama dengan F2 — aksi yang melaporkan sukses tapi
   tidak mengubah apa pun. Fake-nya juga lebih permisif dari SQL (SQL sudah
   menyaring `deleted_at IS NULL`); ketidaksamaan itu yang bikin tes unit hijau.

3. **Route `DELETE /auth/sessions/{id}` salah middleware.** Middleware path-id
   membaca `{id}` sebagai id **organisasi**, jadi setiap pencabutan menjawab
   `404 workspace not found`. Diperbaiki ke `orgHeaderContextMiddleware`; id
   sesinya datang dari `PathValue("id")` di handler.

4. **Fake `memoryRepository` lebih permisif dari SQL di tiga tempat** — akun yang
   sudah ditutup tetap bisa login, org yang sudah ditutup tetap resolve, dan
   `ListOrgsForUser` tidak menyaring org mati. Ketiganya disamakan. Kelas bug ini
   berulang di F2 dan F3: **fake adalah double, bukan implementasi kedua dari
   kontrak**, dan tiap kali SQL berubah, fake-nya harus ikut.

5. **Satu assertion tes gw tidak mengukur apa yang dia klaim.** "Workspace ikut
   hilang" diuji lewat `ResolveWorkspace`, yang gagal untuk akun tertutup
   **baik org-nya ditandai maupun tidak** — jadi tesnya hijau walau AC2 tidak
   dikerjakan. Ketahuan dari mutation yang SURVIVED, bukan dari review. Diganti
   jadi pembacaan langsung `orgs.deleted_at`.

**Konflik kontrak yang diselesaikan:** US-AD05 AC3 bilang mencabut sesi SENDIRI
lewat endpoint cabut paksa harus ditolak ("pakai logout biasa"); US-AD90 AC4
bilang pengguna **dapat** mencabut sesinya sendiri. Baris §6.2.2 ARCHITECTURE
memilih AC4, dan itu yang diimplementasikan — daftar sesi yang barisnya sendiri
tidak bisa dihapus adalah daftar dengan tombol yang tidak melakukan apa-apa.
Dicatat di `docs/OPEN-ISSUES.md`.

**Batasan yang jujur:** ganti password dan tutup akun tidak bisa dipakai pemanggil
API key (nol baris `sessions`) dan menjawab 401. Untuk ganti password itu
konsekuensi AC1 ("sesi ini tetap aktif" tidak punya arti tanpa sesi); untuk tutup
akun itu pilihan yang lebih sempit dari yang mungkin — seharusnya bisa, tapi
butuh jalur "cabut semua sesi user ini" yang belum ada di permukaan API key.
Tidak dikerjakan di fase ini.

**Fase berikutnya:** 6.2.12 Steps (3 endpoint).

## F4 — 6.2.11 Runs (2 endpoint sisa)

**Status:** selesai, ter-push.

| Endpoint | Status | Bukti |
|---|---|---|
| `POST /api/v1/runs/{id}/cancel` | ✅ | probe 32/32 · `TestPgCancelRunRecordsAndReadsBackTheRequest` |
| `GET /api/v1/runs/{id}/summary` | ✅ | `TestRunSummaryReportsDuration` |

**Catatan urutan:** brief menaruh 6.2.11 di fase 3 dan 6.2.2 di fase 5. Sesi ini
mengerjakan 6.2.2 lebih dulu (commit `f17e848`) karena 6.2.12 sudah ✅ semua —
ringkasan §6.2.20 yang basi bikin gw salah kira. Isi brief-nya sendiri akurat.

**Gate:** `tools/gate-overnight.cmd` rc=0 · `verify_suite.py` 0 FAIL
(**89 ✅ / 40 ⬜**) · `go test ./internal/board/ ./cmd/api/ ./internal/auth/
./internal/store/... ./internal/metrics/` hijau · probe
`tools/probe-run-cancel.py` **32/32** lawan API nyata · mutation **5/5 CAUGHT**.

**Temuan**

1. **`Run.CancelRequestedAt` ada di domain sejak F2 tapi nol yang mengisinya.**
   Kolomnya ditulis, tapi `runRowShape`/`runRow` tidak membawanya, jadi nilai itu
   tersimpan di DB dan tidak pernah terlihat siapa pun yang membacanya lewat Go.
   Kelas yang sama dengan `sessions.user_agent`/`ip` di F3. Diperbaiki: 6 klausa
   SELECT/RETURNING diperluas + field-nya masuk row shape.

2. **`POST /runs/{id}/cancel` tidak boleh menutup run-nya sendiri.** Cancel
   menulis `cancel_requested_at`; yang mengakhiri run tetap dispatcher (§5.1:
   dispatcher satu-satunya penulis state run). Kalau handler-nya yang menutup,
   keputusan `outcome` pindah ke API dan §5.1 jadi tidak benar.

3. **Cabang balapan `RequestRunCancel` → `ErrNotFound` tidak teruji.** Mutation
   pertama SURVIVED: cabang itu hanya bisa terpicu kalau executor menutup run di
   antara baca dan tulis. Gw bikin double yang selalu kehilangan balapan, jadi
   cabangnya deterministik — dan mutasinya CAUGHT.

4. **Dua lubang di tes gw sendiri, ketahuan dari mutation yang SURVIVED:**
   `TestRunSummaryReportsDuration` menyetel `ended ≈ now()`, jadi "ukur pakai
   `ended_at`" dan "ukur pakai `now()`" memberi hasil sama dan tesnya tidak bisa
   membedakan. Digeser jadi berakhir 210 detik lalu. Yang kedua: penjepitan
   durasi negatif (skew jam) sama sekali tidak ada tesnya. Ditambahkan.

**Fase berikutnya:** 6.2.15 Cost Ledger & Budget (3 endpoint).

## F5 — 6.2.15 Cost Ledger & Budget (3 endpoint sisa)

**Status:** selesai, ter-push.

| Endpoint | Status | Bukti |
|---|---|---|
| `GET /api/v1/boards/{id}/budget` | ✅ | probe 34/34 · `TestPgBoardBudgetReportsTheAggregate` |
| `PATCH /api/v1/boards/{id}/budget` | ✅ | `TestPgUpdateBoardBudgetChangesWhatTheGateReads` |
| `GET /api/v1/orgs/{id}/cost-summary` | ✅ | `TestPgCostSummaryTotalsMatchTheirParts` |

**Gate:** `tools/gate-overnight.cmd` rc=0 · `verify_suite.py` 0 FAIL
(**92 ✅ / 37 ⬜**) · `go test ./internal/board/ ./cmd/api/ ./internal/auth/
./internal/store/... ./internal/metrics/` hijau · probe `tools/probe-budget.py`
**34/34** lawan API nyata · mutation **8/8 CAUGHT**.

**Temuan**

1. **`GET /boards/{id}/budget` harus membaca agregat, bukan ledger.** Ada dua
   sumber angka biaya board: `daily_board_costs` (agregat, di-upsert dispatcher
   tiap step selesai) dan `ledger_entries` (rincian per panggilan LLM).
   `BoardSpendToday` — yang dikomentari di kode sebagai "the number the US-AD32
   cost gate reads" — **salah**; gate-nya membaca `BoardBudgetToday`, yaitu
   agregatnya. Komentar itu diperbaiki, dan endpoint ini sengaja menyajikan
   agregat yang sama supaya layar finops dan guardrail tidak bisa beda angka.
   (Di DB yang jalan sekarang keduanya kebetulan sama, tapi itu kebetulan yang
   bergantung pada urutan penulisan, bukan jaminan.)

2. **Dua definisi "hari ini" di repo ini.** `BoardSpendToday` memakai
   `date_trunc('day', now())` (zona server), `BoardBudgetToday`/`UpsertDailyBoardCost`
   memakai `(now() AT TIME ZONE 'UTC')::date`. Di DB container (TZ=UTC) keduanya
   identik, jadi divergensinya laten — tapi begitu Postgres-nya bukan UTC, dua
   angka di layar yang sama akan berbeda. **Belum diperbaiki** (menyentuh baris
   yang sudah ✅ dan keputusan zona waktu itu milik user); dicatat di
   `docs/OPEN-ISSUES.md`.

3. **`GROUPING(l.model) = 1` bukan cara menandai baris total.** Grouping set
   board juga tidak memuat `l.model`, jadi baris per-board ikut berlabel `total`
   dan seluruh laporan per-board hilang. Ketahuan dari tes lawan Postgres, bukan
   dari membaca SQL-nya. Total itu satu-satunya set yang tidak memuat model
   maupun board.

4. **`SUM(...)` mengembalikan NULL untuk nol baris, bukan 0.** Tanpa `COALESCE`,
   org yang belum pernah belanja bikin `can't scan NULL into *int64` — laporan
   "belum ada pengeluaran" jadi error 500, padahal US-AD32 AC3 minta nol.

**Bentuk respons sengaja mengikuti `frontend/src/lib/domain.ts`.** Layar finops
(`use-cost-rail.ts`, `CostOverview.tsx`, `finops.ts`) sudah ada sejak lama dan
selama ini menampilkan empty state karena endpoint-nya 404. Nama field yang
meleset sedikit pun akan membuat halaman itu kosong lagi **tanpa error** — jadi
probe-nya memeriksa nama field satu per satu, bukan cuma status code.

**Fase berikutnya:** 6.2.4 Orgs & Memberships (1 endpoint — `DELETE /orgs/{id}`).

## F6 — 6.2.4 Orgs & Memberships (1 endpoint sisa)

**Status:** selesai, ter-push.

| Endpoint | Status | Bukti |
|---|---|---|
| `DELETE /api/v1/orgs/{id}` | ✅ | `TestDeleteOrg*` (7) · `TestPgDeleteWorkspace*` (6) |

**Gate:** `tools/gate-overnight.cmd` rc=0 · `verify_suite.py` 0 FAIL
(**93 ✅ / 36 ⬜**) · `go test ./internal/auth/ ./cmd/api/ ./internal/board/`
hijau · mutation **6/6 CAUGHT** · probe `tools/probe-delete-org.py` **14/14**
lawan API nyata (termasuk pembacaan `orgs.deleted_at` langsung ke Postgres —
soft delete tidak bisa dibuktikan dari status code saja).

**Catatan proses:** probe ini ditulis setelah commit F6 ter-push, jadi buktinya
masuk sebagai commit susulan. Untuk fase berikutnya urutannya dibalik: probe
dulu, verifikasi lawan container yang sudah di-rebuild, baru commit.

**Temuan**

1. **`GetOrgByIDIncludingDeleted` sudah ada di DB sejak F3, nol pemanggil.**
   Komentarnya menjelaskan persis kebutuhan endpoint ini: "dipakai jalur
   tutup-akun: ia harus tahu ruang kerja yang SUDAH ditandai hapus, supaya
   permintaan kedua tetap melihat ruang kerja yang sama." Query itu ditulis untuk
   satu pemanggil yang tidak pernah datang — dan endpoint yang membutuhkannya
   justru masih ⬜. Sekarang pemanggilnya ada.

2. **Route ini tidak boleh lewat `orgContextMiddleware`.** Middleware itu
   me-resolve tenant lewat `GetOrgByID`, yang menyaring `deleted_at IS NULL`.
   Kontraknya bilang idempoten, jadi request kedua harus sukses — lewat
   middleware itu, request kedua justru 404 untuk workspace yang baru saja
   ditutup pemanggilnya sendiri. Handler-nya me-resolve pemanggil dari sesi dan
   menyerahkan keputusannya ke store.

3. **Soft delete itu kewajiban kontrak, bukan preferensi.** US-AD98 AC5 memberi
   jendela pemulihan 30 hari; DELETE keras membuat AC itu mustahil dipenuhi.
   Cascade FK `ON DELETE CASCADE` yang ada di seluruh skema karena itu **tidak
   pernah jalan** di jalur ini — dan itu benar: baris di bawahnya harus selamat
   supaya "pulihkan dalam 30 hari" berarti sesuatu. Diuji langsung ke tabel
   `memberships`, bukan lewat status code.

4. **Guard idempoten di service kelihatan redundan, ternyata tidak.** SQL-nya
   sudah punya `AND deleted_at IS NULL`, jadi penulisan kedua memang no-op.
   Yang membedakan adalah keanggotaan: org yang ditutup lalu dibersihkan
   operator bisa kehilangan baris `memberships`-nya, dan tanpa guard itu DELETE
   berikutnya jatuh ke pemeriksaan peran → **403 untuk workspace yang sudah
   tertutup**. Ketahuan dari mutasi yang SURVIVED, bukan dari review; invariannya
   sekarang dipatok tes tersendiri.

**Catatan proses.** Dua mutasi pertama SURVIVED dan keduanya salah gw, bukan
lubang tes: (a) mutasi pada `queries.sql` tidak berpengaruh karena `go test`
memakai kode generated sqlc — mutasi harus mengenai `queries.sql.go`; (b) guard
idempoten memang redundan terhadap SQL, jadi tidak ada tes yang bisa menangkapnya
sampai invarian yang benar-benar membedakan (keanggotaan hilang) dipatok.

**Satu hazard yang kena lagi:** harness mutasi gw memulihkan file dengan
`pathlib.write_text`, yang di Windows menulis CRLF — `internal/store/queries.sql.go`
jadi CRLF dan `gofmt -l .` menandainya. Dikonversi balik ke LF sebelum gate.
Diff-nya nol, jadi tidak ada yang rusak, tapi ini kedua kalinya.

**Fase berikutnya:** 6.2.3 Projects & Boards (1 endpoint), lalu 6.2.5 Agents (1),
6.2.6 Agent Skills (2), 6.2.7 Providers (1) — sisanya modul besar (SSE, approvals,
webhooks, api-keys) yang butuh tabel baru.
## F7 — 6.2.14 Approvals (5 endpoint)

**Status:** selesai, ter-push. 98 ✅ / 31 ⬜ (naik dari 93/36).

**Endpoint:** `GET /approvals` (Viewer) · `GET /approvals/{id}` (Viewer) ·
`POST /approvals/{id}/approve` (Admin) · `POST /approvals/{id}/reject` (Admin) ·
`POST /tasks/{id}/approvals` (Member).

**Konflik kontrak — tiga, semuanya diputuskan sendiri dan dicatat di
`docs/OPEN-ISSUES.md`:**

1. **Ke mana task pergi setelah diputuskan.** PRD (US-AD34 AC1 / US-AD35 AC1)
   bilang approve → `running`, reject → `blocked(needs_input)`. Tabel §5.3 dan
   baris §6.2.14 bilang `ready` dan `blocked(policy)`. Dipakai **`ready` +
   `policy`**: `running` tidak bisa dipenuhi jujur (run-nya sudah ditutup saat
   gate dipasang, jadi "kembali ke running" = mengarang run baru), dan
   `needs_input` salah makna — di sini manusia yang menolak, bukan agen yang
   butuh pertanyaan dijawab.
2. **Gerbang peran.** PRD membolehkan Member memutuskan; matriks §11.3 bilang
   Admin. Dipakai **Admin**. Kalau Member boleh membuat *dan* memutuskan gate,
   gate itu tidak menghalangi apa pun — tinggal tekan approve sendiri.
3. **`POST /tasks/{id}/approvals` terdaftar Member, kontraknya Worker
   (`Internal/Key`).** Mekanisme `api_keys` belum ada, dan endpoint Worker lain
   yang sudah ✅ (`/runs/{id}/heartbeat`, `/end`, `/steps`) menghadapi hal yang
   sama dan diregistrasi `auth.Member` sejak awal. Diikuti supaya tidak
   menciptakan jalur autentikasi tandingan.

**Temuan utama — tiga bug nyata, ketiganya ketahuan dari probe, bukan dari
suite:**

- **Task yang disetujui tidak akan pernah diklaim lagi.** Predikat klaim adalah
  `current_run_id IS NULL` (§4b), dan satu-satunya yang melepasnya adalah
  `Service.EndRun`. Jalur gate pertama gw mengakhiri run lewat `repo.EndRun` —
  melewati pelepasan itu. Hasilnya: approve → task `ready` → dispatcher tidak
  pernah mengambilnya. **Kegagalan senyap**, tidak ada error di mana pun.
- **Urutan parkir-vs-tutup-run salah, dua kali.** Menutup run dulu berarti
  `applyOutcome` memindahkan task lebih dulu (dengan `failed`+`policy` → ia
  jatuh ke `blocked(policy)` sebelum gate terpasang). Akhirnya: **jalur approval
  tidak lewat `applyOutcome` sama sekali** — tujuan task-nya adalah gate, bukan
  tabel outcome, jadi membiarkan `applyOutcome` memilih dulu berarti bertarung
  dengan keputusan itu. Dipakai `repo.EndRun` + parkir eksplisit.
- **Gate kedua pada task yang sudah terparkir.** Task `awaiting_approval` tidak
  punya run hidup, dan `approvals.run_id` NOT NULL — jadi gate kedua **tidak
  bisa** disimpan. Ditolak 400 dengan pesan yang menyebut masalah sebenarnya,
  bukan gagal lewat `GetRun("")` yang membaca sebagai 404 "task tidak ada".

**Bug di verifikasi gw sendiri, dua, dan yang kedua serius:**

- Probe gw menulis header lalu membaca `call()` yang sudah diubah jadi
  mengembalikan teks → `AttributeError`. Bug probe, bukan kode.
- **Harness mutasi gw menjalankan filter tes yang tidak mencocokkan apa pun,
  lalu `exit 0` dibaca sebagai SURVIVED.** Lebih buruk lagi: dua tes nyata
  terhapus oleh rewrite gw sendiri (penanda `index()` yang salah), jadi
  suite-nya hijau karena tesnya sudah tidak ada. Harness diperbaiki: **nol tes
  yang jalan = INVALID, bukan SURVIVED.** Dua tes dipulihkan.

**Mutation: 13 CAUGHT, 0 SURVIVED, 0 INVALID** (setelah harness diperbaiki).
Dua di antaranya membuktikan invarian yang tadinya tidak diuji: predikat
`decision = 'pending'` di dalam `UPDATE` (balapan dua approver, §8.4) dan
syarat `expires_at < now()` di sapuan (tanpa itu, tick 2 detik menutup **semua**
gate yang masih terbuka).

**Tes:** `internal/board` 64 hijau · `cmd/api` hijau · `internal/dispatcher`
hijau. Gate `tools/gate-overnight.cmd` rc=0. `verify_suite.py` 0 FAIL.
Probe `tools/probe-approvals.py` **42/42** lawan API nyata (termasuk 401 di
kelima route, gerbang peran, preview utuh lewat JSONB, dan isolasi lintas
tenant).

**Catatan jujur:** `EndRun` dengan outcome `budget_exceeded` untuk run yang
dijeda gate. Pilihannya sengaja — `failed` akan dibaca sebagai "kerjanya rusak"
oleh siapa pun yang melihat `runs.outcome`, dan mereka tidak bisa melihat
gate-nya dari sana. Kalau ada konsumen yang memfilter `outcome='budget_exceeded'`
sebagai "kena cap biaya", ia akan salah baca gate ini.

## F8 — 6.2.17 Comments (4 endpoint)

**Status:** selesai, ter-push. 102 ✅ / 27 ⬜ (total 129).
**Deviasi urutan:** brief menaruh 6.2.19 di slot 8 dan 6.2.17 di slot 9. Yang
dikerjakan adalah **6.2.17**, karena brief sendiri menetapkan aturan "urut modul
termurah dulu" dan 6.2.19 bukan modul termurah: ia menuntut tabel `notifications`
yang belum ada, dua endpoint search yang butuh `pg_trgm` di jalur query, dan satu
`/system/info`. Comments nol dependensi baru — tabelnya sudah ditulis lengkap di
ARCHITECTURE §3.16 sejak awal, cuma belum pernah dibuat migrasinya.

**Temuan:** tabel `comments` **didokumentasikan lengkap** di §3.16 (DDL, dua
constraint, dua index, komentar "Melayani: ...") dan di DECISIONS §6 baris 194 —
tapi nol migrasi yang menyebutnya, jadi nol tabel di DB. Kelas yang sama dengan
`GetOrgByIDIncludingDeleted` (F6) dan `approvals` (F7): kontraknya sudah ditulis,
sambungannya belum ada. Migrasi `0017.up.sql` menyalin §3.16 apa adanya.

**Keputusan yang diambil sendiri** (kontrak diam, AC menuntut):
- `MaxCommentBody = 4096` karakter, dihitung **rune, bukan byte**. US-AD42 AC3
  minta "melebihi batas panjang" tanpa menyebut angkanya di dokumen mana pun.
  Rune dipilih karena byte akan menolak komentar Indonesia lebih awal daripada
  batas yang tertulis, dan pesan errornya tidak akan menjelaskan kenapa.
  Kolomnya tetap TEXT, jadi menaikkan batas nanti satu baris, bukan migrasi.
- Batasnya **inklusif** (4096 diterima, 4097 ditolak), dan itu dipatok tes.

**Migrasi `0017` harus `IF NOT EXISTS`** — bukan gaya, tapi syarat. `Apply`
mengulang migrasi di atas skema yang sudah terisi di jalur repair, dan
`TestPostgresRepairMigrationGrantsPrivilegesOnLegacySchema` langsung merah saat
`CREATE TABLE comments` tanpa `IF NOT EXISTS`. Konvensi itu sudah dipakai 0013
ke atas; F8 yang melanggarnya lebih dulu, dan tesnya yang menangkap.

**Tes:** `cmd/api` 6/6 (gerbang peran, batas 400, kepemilikan, id non-numerik);
`internal/board` 7/7 lawan Postgres (constraint penulis nyata, body round-trip,
predikat penulis **di SQL**, event timeline, task tak dikenal, scoping org).
**Mutation 7 CAUGHT, 0 SURVIVED** — dan **2 mutant setara** yang diakui: cek
kepemilikan di service untuk Edit/DeleteComment tidak bisa dibedakan dari versi
tanpa cek, karena predikat penulis di SQL sudah menangkap kasus yang sama. Itu
pertahanan berlapis, bukan lubang; yang dibuktikan adalah lapisan SQL-nya.

**Probe:** `tools/probe-comments.py` **25/25 hijau** lawan API nyata, termasuk
satu pemeriksaan yang cuma bisa dilakukan di sini: body 4096 rune non-ASCII
(8192 byte) diterima, jadi batasnya benar-benar rune di server yang jalan.

**Dua kesalahan proses gw sendiri yang perlu dicatat:**
1. **Harness mutation gw salah dua kali.** Pertama, `go test` tanpa `-v` tidak
   menulis baris `--- PASS`/`--- FAIL`, jadi harness menghitung nol tes dan
   melaporkan INVALID/SURVIVED untuk apa pun. Kedua, filter `-run` gw bolong:
   `TestPgCreateCommentStoresBothAuthorShapes` tidak cocok dengan pola mana pun,
   jadi tes yang justru menguji cabang penulis tidak pernah jalan. Dua-duanya
   sekarang dipatok: baseline wajib ≥13 PASS sebelum mutasi dimulai, dan
   `-v` wajib.
2. **Probe gw butuh empat putaran perbaikan**, tiga di antaranya asumsi endpoint
   yang salah (register butuh `org_name`; token sesi harus dikirim sebagai
   Bearer, cookie saja 401; bukan-anggota ditolak 403 oleh middleware sebelum
   handler jalan). Yang terakhir itu bukan bug produk — 403 lebih ketat daripada
   404, dan probe-nya yang salah mengharapkan. Isolasi tenant diuji lewat aktor
   yang punya workspace sendiri, dan itu yang benar-benar mengukur scoping org.

**Urutan yang dibetulin:** probe ditulis dan dijalankan **sebelum** commit. Di F6
probe ditulis setelah commit ter-push, jadi commit itu jalan tanpa bukti probe.

## F9 — 6.2.19 Audit, Search & System (6 endpoint)

**Status:** selesai, ter-push. 108 ✅ / 21 ⬜. Gate rc=0.

**Deviasi slot brief:** brief menaruh 6.2.19 di slot 8; F8 mengerjakan 6.2.17
lebih dulu (termurah dulu), jadi 6.2.19 jatuh ke slot 9. Dicatat, bukan didiemin.

**Yang dikerjakan**
- Migrasi `0018` — tabel `notifications` (§3.21). Ditulis lengkap di kontrak
  sejak awal, **nol migrasi** yang membuatnya. Kelima kalinya pola ini muncul.
- 9 query baru: `ListAuditLog` (filter + cursor), `SearchTasks`, `SearchRuns`,
  `CreateNotificationOnce`, `OrgAdminsAndOwners`, dll.
- `internal/auth/{audit.go,pgx.go,memory.go,repository.go}` — `AuditFilter`,
  `Notification`, `NotifyOnce`, adapter pgx + memory.
- `internal/board/{search.go,notify.go}` — `SearchTasks`/`SearchRuns` +
  producer notifikasi.
- `cmd/api/system.go` — 6 handler; 3 route `authAPI` (audit + notif), 2 route
  `boardAPI` (search), 1 route publik (`/system/info`).
- `/system/info` **tanpa DB**, `commit` dibaca dari `runtime/debug.ReadBuildInfo`
  — jadi bisa dipakai membuktikan container yang jalan memang build terbaru.

**Empat bug nyata yang ketangkap sebelum commit**
1. **`AGENTDECK_DISPAT` vs `AGENTDECK_DISPATCH`.** Empat dokumen kontrak
   (ARCHITECTURE §2813/§2838, OPEN-ISSUES, BRIEF, `.hermes.md`) bilang
   `AGENTDECK_DISPAT`; `cmd/api/main.go:452` membaca `AGENTDECK_DISPATCH`, dan
   log-nya menyuruh set nama yang salah. Operator yang ikut kontrak menyalakan
   dispatcher dan tidak terjadi apa-apa. Dibetulin ke `AGENTDECK_DISPAT`.
2. **`Store.Notify` tidak membuat id.** `notifications_id_ulid_chk` menolak
   string kosong ⇒ 500 di jalur pertama yang memakainya.
3. **`likePattern("")` = `"%%"`, bukan NULL.** `r.error ILIKE '%%'` pada baris
   yang `error`-nya NULL mengevaluasi NULL, jadi run tanpa error tersaring
   habis: filter `task_id` saja mengembalikan nol. Diganti `likePatternPtr`.
4. **Operator `%` pg_trgm salah arti.** `%` adalah *similarity* di atas ambang
   0.3, bukan "mengandung" — `deploy` tidak cocok dengan judul panjang. Diganti
   `ILIKE '%'||pattern||'%'`, yang justru diakselerasi index trigram.

**Notifikasi akhirnya punya produser.** Tabel tanpa penulis = inbox selalu
kosong. Dua `kind` disambungkan ke titik presisi, bukan kira-kira:
- `budget.warning` di `run.go:116` (`checkRunBudget`) — ambang 80% (N18),
  penerima = owner + admin org, dedup per (user, kind, target, hari) di SQL
  supaya tidak spam tiap step.
- `run.failed` setelah `EndRun` — hanya untuk `outcome == "failed"`.
Dua `kind` sisanya (`approval.requested`, `credential.invalid`) **belum punya
produser** dan dicatat di `docs/OPEN-ISSUES.md`.

**Verifikasi**
- Gate `tools/gate-overnight.cmd` rc=0 (vitest 68 passed, prettier bersih).
- `verify_suite.py` 0 FAIL, `STATUS ENDPOINT: 108 ✅ dan 21 ⬜ cocok dengan cmd/api`.
- `go test`: `./internal/board/` ok 76.8s · `./internal/auth/` ok 33.3s ·
  `./cmd/api/` ok 22.9s · `./internal/dispatcher/` ok 1.3s ·
  `./internal/migrate/` ok 95.6s (0018 idempoten).
- Mutation: `likePattern` 5/5 CAUGHT · ambang budget 5/5 CAUGHT.
- Probe `tools/probe-f9.py` lawan API nyata: **35/35 hijau**.
- Rebuild container, `healthz=200`, `/system/info` mengembalikan `go1.24.13`.

**Gap yang dicatat, bukan didiemin**
- Tiga endpoint §6.2.19 bertanda `Auth: Session/Key`; `api_keys` belum ada, jadi
  diregistrasi `auth.Member`/`auth.Viewer` mengikuti preseden F7.
- `credential.invalid` belum punya jalur probe provider yang gagal permanen.
- `approval.requested` belum ditembak karena dispatcher default MATI.

## F10 — 6.2.3 API Keys (5 endpoint)

**Status:** selesai, ter-push. 113 ✅ / 16 ⬜. Gate rc=0.

**Deviasi slot brief:** brief menaruh 6.2.19 di slot 8 dan 6.2.3 di slot 10;
urutan dikerjakan 6.2.17 (F8) → 6.2.19 (F9) → 6.2.3 (F10) karena brief sendiri
bilang "termurah dulu" dan ketiganya nol dependensi eksternal.

**Kenapa 6.2.3 lebih dari CRUD.** `api_keys` adalah tabel yang **menutup gap
Worker-auth** yang dicatat F7 dan F9 di `docs/OPEN-ISSUES.md`. Sebelum ini,
`contextMiddleware` hanya tahu sesi, jadi endpoint Worker (`heartbeat`, `end`,
`steps`, `POST /tasks/{id}/approvals`) terpaksa didaftarkan `auth.Member`.
Sekarang bearer `adk_...` diautentikasi di middleware dan key **mewarisi `role`
pemiliknya** dari `memberships` (2291) — jadi mekanismenya ada. Route-nya
sendiri belum dipindah, dan itu dicatat sebagai isu terbuka dengan alasannya.

**Pola kontrak yang keenam kali berulang.** Tabel `api_keys` ditulis LENGKAP di
ARCHITECTURE §3.18 sejak awal — DDL, tujuh constraint, dua index, komentar
"Melayani: autentikasi — SELECT ... WHERE prefix = $1 AND revoked_at IS NULL" —
dan **nol migrasi** yang membuatnya. Sama seperti `comments` (F8),
`notifications` (F9), `approvals` (F7), dan index trigram `tasks_title_trgm_idx`
(migrasi 0004, "Melayani: search full-text"). Kontraknya memang sudah
merancang endpoint ini; yang tidak ada adalah pembuatnya. Migrasi `0019`.

**Keputusan yang gw ambil:**
- **`prefix` = 8 karakter pertama, 4 karakter acak** (bukan dari slug org).
  §2291 mencontohkan `adk_myorg` (slug), tapi slug di DB nyata sampai 25
  karakter dan `api_keys_prefix_chk` menuntut tepat 8 — contoh di dokumen itu
  ilustratif, constraint-nya literal. Empat karakter acak hex selalu 8 total
  dan tidak butuh penyaringan karakter.
- **Token 48 byte acak, SHA-256, bukan bcrypt** — persis catatan §3.18: key
  high-entropy, jadi tidak ada ruang tebak yang perlu diperlambat. Bcrypt di
  jalur autentikasi programatik hanya menambah biaya per request.
- **Key's org menang atas `X-Org-ID`**, tidak dicocokkan. Key di-scope ke satu
  workspace saat dibuat, jadi tidak ada yang perlu dipilih; menuntut header
  akan merusak pemanggil CLI/SDK yang jadi alasan endpoint ini ada.
- **Revoke idempoten, hapus fisik.** Mencabut dua kali = 200; yang tidak ada =
  404. Keduanya dibedakan dengan membaca dulu, karena `RevokeAPIKey` mengubah
  nol baris di kedua kasus.
- **`TouchAPIKey` gagal ≠ autentikasi gagal.** `last_used_at` itu telemetri;
  menolak request sah karena kolom telemetri gagal ditulis akan membuat key
  tampak mati sesekali.

**Temuan di luar F10:** `cmd/api/main.go` membaca **`AGENTDECK_DISPATCH`**,
sementara empat dokumen kontrak (ARCHITECTURE §2813 + §2838, OPEN-ISSUES,
OVERNIGHT-BRIEF, `.hermes.md`) menyebut **`AGENTDECK_DISPAT`**. Operator yang
ikut kontrak menyalakan dispatcher dan tidak terjadi apa-apa; log-nya pun
menyuruh set nama yang salah. Diperbaiki ke `AGENTDECK_DISPAT`. Karena
defaultnya tetap OFF, perilaku runtime tidak berubah.

**Bukti:**
- `go build ./...` + `go vet ./...` bersih; `gofmt -l .` kosong.
- `go test ./internal/auth/` ok 40.6s · `./cmd/api/` ok 23.6s · `./internal/migrate/` ok 97.0s (0019 idempoten).
- Mutation 6 mutant (5 CAUGHT, 1 BUILD-FAIL artefak): prefix bukan 8 char,
  bandingkan hash dilewati, nama kosong diterima, `LooksLikeAPIKey` selalu true,
  prefix pendek tidak dijaga.
- `tools/probe-f10.py` **33/33 hijau** lawan API nyata — termasuk bearer `adk_`
  mengautentikasi tanpa X-Org-ID, peran diwarisi (key owner lolos audit-log,
  key member 403), plaintext tidak pernah muncul lagi, revoke langsung mematikan.
- `tools/gate-overnight.cmd` rc=0.

**Catatan proses:** probe ditulis **sebelum** commit (aturan sejak F6), dan
`tools/probe-f10.py` sudah hijau sebelum commit dibuat. Gate deteksi route
diverifikasi ulang: kelima route awalnya ditulis lewat closure `headerRoute`
lokal dan **tidak terdeteksi** `verify_suite.py` (yang hanya membaca
`mux.Handle`/`HandleFunc` + tujuh nama helper) — gate melaporkan "0 FAIL" untuk
endpoint yang ada. Ditulis ulang inline, lalu gate menandai kelimanya. Ini
kesalahan yang sama seperti F7.

## F11 — 6.2.13 Events & Realtime SSE (4 endpoint)

**Status:** selesai, ter-push. **117 ✅ / 12 ⬜**. Gate rc=0.

**Deviasi slot brief:** brief menaruh 6.2.18 di slot 11. Gw kerjakan 6.2.13
karena brief sendiri bilang "termurah dulu", dan 6.2.13 memang termurah dari
yang tersisa: tabel `events` sudah ada sejak migrasi awal dengan **62 baris
nyata**, tiga query sudah ditulis, dan producer-nya (`run.claimed`,
`run.finished`, `comment.created`, `approval.requested`, `ledger.entry`) sudah
tersambung di F1-F9. Yang hilang cuma transportnya. 6.2.16 Artifacts butuh
kredensial R2 yang tidak gw punya; 6.2.18 Webhooks butuh pengiriman HTTP keluar
plus HMAC. Keduanya tidak bisa gw buktikan jalan, jadi bukan "termurah".

### Yang dikerjakan

1. **Migrasi `0020`** — trigger `events_notify_trigger` yang memanggil
   `pg_notify('agentdeck_events', NEW.id::text)`.
   §7.2 menetapkan hub menerima event lewat `LISTEN agentdeck_events`, dan §7.4
   bilang polling bukan jalur utama — tapi tidak ada apa pun yang melakukan
   `NOTIFY`. Pola yang sama seperti `comments` (F8), `notifications` (F9),
   `api_keys` (F10): kontraknya lengkap, sisi yang menghubungkan tidak ada.
   Trigger-nya `AFTER INSERT` saja, karena `events` append-only (§3.12).
2. **`internal/sse/`** (kontrak menyebut `internal/sse/hub.go`) — `hub.go`,
   `hub_impl.go`, `frame.go`, `serve.go`. Frame W3C (`id`/`event`/`data` +
   baris kosong), heartbeat `: ping` 15 detik, buffer klien 128, slow-consumer
   di-drop bukan diblokir (7.3), replay `Last-Event-ID` LIMIT 500 (7.2).
3. **`cmd/api/events.go` + `sse_store.go`** — 4 handler + adapter LISTEN.
   Adapter butuh **koneksi khusus dari pool**, bukan `*store.Queries`: notifikasi
   Postgres hanya sampai ke koneksi yang menjalankan LISTEN.
4. Query `GetEvent` + `ListRunEventsAfter`; method `board.Service`
   `TaskHistory`/`BoardEventsAfter`/`RunEventsAfter`/`EventByID`.
5. Wiring di `main.go`: hub + goroutine `Run(ctx)`, channel sebagai konstanta
   `sseChannel` supaya trigger dan kode tidak bisa menyimpang.

### Tiga bug nyata yang ketemu saat probe

Ketiganya tidak akan ketemu dari unit test, dan tidak satu pun menghasilkan
error yang menunjuk ke penyebabnya.

1. **Middleware metrik menyembunyikan `http.Flusher`.** `statusRecorder`
   menyematkan `http.ResponseWriter`, dan interface itu tidak punya `Flush` —
   jadi `w.(http.Flusher)` **selalu** gagal dan setiap handler streaming
   menjawab 500 "streaming unsupported". Ini bug F1 yang baru muncul sekarang
   karena F1 belum punya handler streaming. Diperbaiki: `Flush()` yang
   meneruskan, plus `Unwrap()` supaya `http.ResponseController` juga bisa
   menembus. Tes regresi di `internal/metrics/middleware_test.go`.
2. **Event lifecycle task tidak pernah ditulis.** `task.created`,
   `task.status_changed`, dan `task.assigned` dinyatakan di DECISIONS §4, dan
   `task.status_changed` justru contoh frame di §7.1 sendiri — tapi tidak ada
   satu pun penulisnya. Timeline task kosong dan SSE tidak punya apa pun untuk
   dikirim, tanpa satu error pun di mana pun. Diperbaiki di `CreateTask`,
   `MoveTask`, `AssignTask`. Transisi yang **ditolak** tidak mencatat apa pun
   (dites).
3. **`last_event_id=0` diperlakukan sebagai "tidak ada".** §7.2 bilang replay
   dijalankan "jika ada `last_event_id`". `0` adalah checkpoint yang sah
   ("belum menerima apa pun") dan ids di `events` mulai dari 1 — jadi
   memperlakukannya sebagai absen membuat klien yang minta backlog penuh tidak
   mendapat apa pun. `LastEventID` sekarang mengembalikan `(id, ada)`.

### Verifikasi

- `gofmt -l .` bersih · `go build ./...` + `go vet ./...` bersih.
- `go test` per paket: `sse` ok 1.4s (14 tes) · `metrics` ok 1.2s (15 tes) ·
  `board` ok 81.1s · `cmd/api` ok 24.0s · `migrate` ok 96.2s (0020 idempoten).
- Mutation **security/business rule**: hub 7 mutant → 5 CAUGHT, 1 BUILD-FAIL
  (artefak mutant), 1 SURVIVED yang **ditemukan dan ditutup** (tes kelaparan
  flaky karena urutan map acak → ditulis ulang jadi deterministik; setelah itu
  2/2 CAUGHT pada dua mutan inti). Trigger migrasi 3/3 CAUGHT (notify dihapus,
  payload jadi `payload_json`, channel salah).
- **Probe lawan API nyata: `tools/probe-f11.py` 21/21 hijau.** Termasuk rantai
  penuh INSERT → trigger NOTIFY → LISTEN → frame SSE dengan `event: task.created`
  dan payload benar, plus replay `Last-Event-ID: 0`.
- Container di-rebuild; trigger terverifikasi ada di DB
  (`SELECT tgname FROM pg_trigger` → `events_notify_trigger`), skema versi 20.

### Yang TIDAK dikerjakan (jangan dianggap beres)

- **Frontend belum memakai stream ini.** Empat endpoint-nya jalan dan terbukti,
  tapi UI masih polling/revalidate seperti sebelumnya. Menyambung UI adalah
  pekerjaan terpisah dan belum dikerjakan.
- Server tidak memasang `WriteTimeout` sama sekali (jadi stream tidak terputus),
  tapi juga belum ada deadline tulis eksplisit per stream.

### Catatan proses

- Gate `verify_suite.py` melaporkan `DESIGN: designmd lint exit 1`. Bukan
  DESIGN.md: `npx` cache-nya rusak (`MODULE_NOT_FOUND` di
  `_npx/9cb06364208d5c89`). Dibuktikan dengan install bersih di scratch —
  DESIGN.md **exit 0, 0 error**. Flake environment, bukan regresi.
- Filter `-run` gw bolong lagi di percobaan pertama (`TestPgEventsNotify*`
  tidak memuat `TestPgEventsTriggerNotifiesOnInsert`), persis jebakan yang sudah
  dicatat di skill. Harness mutasi migrasi pertama juga jalan dengan
  `AGENTDECK_TEST_DATABASE_URL` kosong ⇒ semua tes **SKIP** dan terbaca
  "SURVIVED" — baseline 0 PASS yang seharusnya langsung ketahuan.

## F12 — 6.2.18 Webhooks (7 endpoint)

**Status:** selesai, ter-push. **124 ✅ / 5 ⬜**. Gate rc=0.

**Deviasi slot brief:** brief menaruh 6.2.18 di slot 11; gw kerjakan di slot 12
karena F11 (6.2.13) lebih murah dan brief sendiri bilang termurah dulu.
Deviasi dicatat, entri lama tidak ditulis ulang.

**Tiga konflik kontrak yang gw putuskan, dan alasannya.**

1. **§3.22 `CHECK (url ~ '^https://')` vs §16 pengecualian loopback.**
   §16 (keputusan user 2026-09-21, mengalahkan US-AD106 AC3) mengizinkan
   `localhost`, `127.0.0.1`, `host.docker.internal` sebagai **string persis**,
   boleh lewat `http`. §3.22 melarangnya. Yang gw pakai §16, dan check-nya
   menegakkan pengecualian itu **persis** — bukan `^https?://` yang akan
   membuka seluruh internet. Terbukti: `http://evil.example` ditolak,
   `http://127.0.0.1:9000` diterima, `http://127.0.0.1.evil.example` ditolak.

2. **`secret` TEXT vs BYTEA terenkripsi.** §3.22 mengetik `secret TEXT` tapi
   komentarnya sendiri bilang "dienkripsi di DB via AES-256-GCM, lihat §16".
   §16 yang mengikat: kolomnya jadi `secret_enc BYTEA`, sama seperti
   `providers.api_key_enc` dan `agents.provider_api_key_enc`.

3. **Jadwal retry §13.3 tidak rekonsiliasi dengan dirinya sendiri.** §13.3
   menulis enam jeda (1m, 5m, 15m, 30m, 1j, 2j) **dan** "total 6 percobaan";
   enam jeda berarti tujuh percobaan. Yang gw pakai aturan yang mengikat —
   kalimat dead-letter "setelah 6 percobaan dan tidak ada sukses" — jadi total
   enam percobaan dan jedanya lima. Nilai 2 jam dari daftar itu tidak terpakai.
   Dicatat di `docs/OPEN-ISSUES.md`.

**Temuan di luar F12: `compose.yaml` masih menyetel nama env yang salah.**

F10 membetulkan `cmd/api/main.go` membaca `AGENTDECK_DISPAT` (empat dokumen
kontrak memakai nama itu), tapi **`compose.yaml:63` masih `AGENTDECK_DISPATCH`**.
Jadi bug-nya masih hidup di jalur yang justru dipakai operator: menyalakan
dispatcher lewat compose memberi container variabel yang tidak dibaca siapa pun.
Dibetulkan, plus `AGENTDECK_WEBHOOKS` ditambahkan.

**Yang dikerjakan.**

- Migrasi `0021`: `webhooks` + `webhook_deliveries` dari §3.22/§3.23. `event_id`
  sengaja **tanpa FK**: `agentdeck_cleanup()` (§3.24) menghapus `events` lebih
  tua dari 30 hari, dan FK cascade akan menghapus riwayat pengiriman yang justru
  dibaca `GET /webhooks/{id}/deliveries`.
- `internal/webhook/`: `sign.go` (HMAC 13.2), `types.go` (jadwal + klasifikasi
  13.3), `worker.go` (LISTEN → fan-out → retry), `send.go` (pengiriman +
  validasi URL §16), `service.go` (CRUD), `pgx.go` (adapter).
- `cmd/api/webhooks.go` (7 handler, semua Admin) + `webhook_wiring.go`.
- Worker default **OFF** (`AGENTDECK_WEBHOOKS=1` untuk menyalakan), alasan sama
  dengan dispatcher: deployment yang tidak memakai webhook tidak boleh membuka
  koneksi keluar yang tidak diminta siapa pun.

**Keputusan desain yang gw ambil.**

- **Percobaan pertama tidak dikirim dari goroutine notifikasi.** Satu event bisa
  cocok dengan banyak webhook, dan mengirim di dalam callback LISTEN berarti
  setiap pengiriman lambat (batasnya 10 detik, 13.3) menahan notifikasi
  berikutnya — antrean menumpuk tepat saat sistem sibuk. Percobaan pertama dan
  retry lewat **satu jalur**: tick membaca baris yang jatuh tempo (`attempts = 0`
  berarti "sekarang").
- **4xx tidak di-retry, dan itu diputuskan di driver retry, bukan di kolom
  status.** §13.3 meminta 4xx berstatus `failed` **dan** tidak di-retry — tapi
  `failed` justru yang dibaca index retry, jadi kolom status saja tidak bisa
  membedakan "akan dicoba lagi" dari "sudah selesai". Yang membedakannya
  `response_code`: 4xx dilewati. Tanpa ini, retry jalan terus selamanya ke
  endpoint yang sudah menolak.
- **Event yang sudah dipurge retensi → `dead`, bukan `failed`.** Barisnya
  di-LEFT JOIN; tanpa itu ia menggantung `pending` selamanya tanpa jejak.
- **At-least-once, disengaja.** `attempts` dinaikkan setelah percobaan selesai,
  jadi proses yang mati di tengah pengiriman mengirim ulang. US-AD53 AC2
  berbunyi "Event tidak hilang"; penerima membedakan kiriman ganda lewat `id`.
- **Retry manual me-reset `attempts` ke 0.** Delivery `dead` punya attempts = 6;
  tanpa reset, percobaan berikutnya langsung dinilai habis jatah dan kembali
  `dead` tanpa pernah dikirim — tombol retry yang tidak mengirim apa pun.

**Bukti.**

- `tools/probe-f12.py` **30/30** lawan API nyata. Termasuk rantai penuh:
  `task.created` → trigger NOTIFY → worker LISTEN → POST ke receiver HTTP nyata
  → **HMAC diverifikasi ulang dari byte mentah yang diterima** → tercatat
  `delivered` dengan `response_code` 200.
- `internal/webhook/` 27 tes, `cmd/api/` 9 tes RBAC/tenant, `internal/migrate/`
  5 tes constraint. Semua hijau.
- Mutation 9 mutant: **6 CAUGHT** (HMAC dipotong, secret tidak dipakai, daftar
  loopback tidak diperiksa, http-ke-mana-pun diterima, 4xx di-retry, constraint
  URL dilonggarkan), **1 SURVIVED yang gw buktikan ekuivalen** (guard `attempts
  >= maxAttempts` redundant dengan cek index — identik untuk attempts 0..20,
  diukur ekshaustif), **1 SURVIVED yang gw tutup** (daftar loopback perlu diuji
  sebagai daftar, bukan lewat kasus yang kebetulan memakai host berbeda).
- `go test` paket tersentuh: webhook 1.9s, cmd/api 25.2s, migrate 102.1s.
- Gate `tools/gate-overnight.cmd` rc=0. `gofmt -l .` bersih.

**Catatan probe:** receiver-nya server HTTP di host, dan worker-nya jalan di
dalam container — jadi URL-nya `http://host.docker.internal:<port>/hook`, bukan
`127.0.0.1`. Percobaan pertama gagal karena itu, dan gagalnya justru membuktikan
worker-nya benar-benar POST keluar.

**Sisa: 5 ⬜ — 6.2.16 Artifacts.** Terhalang kredensial object storage
(presigned URL). Kerangkanya bisa ditulis, tapi tidak bisa dibuktikan jalan
lawan layanan nyata, jadi tidak dikerjakan sampai kredensialnya ada.

## F13 — 6.2.16 Artifacts (5 endpoint)

**Status:** selesai. **129 ✅ / 0 ⬜ — seluruh endpoint §6.2 terpasang.**
Gate rc=0.

**Koreksi penilaian sebelumnya.** Sesi ini beberapa kali menyatakan Artifacts
terhalang kredensial object storage dan karena itu tidak bisa dibuktikan.
**Itu salah.** §12.3 memakai presigned URL ke R2, dan presigning S3 itu murni
kripto lokal — tidak ada panggilan API ke Cloudflare yang dibutuhkan untuk
membuatnya. Seluruh siklusnya bisa dijalankan lawan server S3-compatible lokal
(MinIO), dan itulah yang dikerjakan: `tools/probe-f13.py` **28/28**.

Pelajaran yang layak dicatat: "butuh kredensial" bukan alasan berhenti sampai
kredensial itu benar-benar diperlukan. Di sini yang diperlukan cuma endpoint
S3 yang bisa dijangkau, dan itu bisa dibuat sendiri.

### Konflik kontrak

1. **§3.15 mendeklarasikan index yang mustahil.** Kontrak menulis
   `CREATE INDEX artifacts_retention_idx ON artifacts (created_at) WHERE created_at < now() - interval '85 days'`.
   Postgres menolaknya: `ERROR: functions in index predicate must be marked
   IMMUTABLE`. Jadi versi kontraknya bukan "belum dibuat" — **tidak bisa
   dijalankan**. Migrasi `0022` memakai btree biasa di `created_at`, yang
   melayani `DELETE ... WHERE created_at < now() - interval '90 days'` (N12)
   sama baiknya. Predikatnya memang tidak bisa menghemat apa pun di sini:
   semua baris tua adalah kandidat hapus.

2. **`artifacts_storage_key_idx` sengaja TIDAK dibuat.** §3.15 bilang index itu
   "melayani download artifact by storage_key", tapi endpoint-nya
   `GET /artifacts/{id}/download` — mencari lewat primary key lalu membaca
   `storage_key` dari barisnya. Tidak ada query yang memfilter
   `WHERE storage_key = ...`. Index mati hanya menambah biaya tulis.

3. **`run_id` wajib, walau §6.2.16 meringkas request upload-url sebagai
   `(filename, size)`.** Key objeknya berbentuk
   `artifacts/{org}/{task}/{run}/{id}-{filename}` (§3.15), jadi tanpa run
   key-nya tidak bisa dibentuk sesuai kontrak. Ringkasan itu tidak bisa
   dipenuhi bersamaan dengan DDL-nya, dan DDL yang mengikat.

4. **Role "Worker" tidak ada di kode.** §6.2.16 meminta `Internal/Key` + role
   Worker; `auth.Role` hanya Owner/Admin/Member/Viewer. Sama seperti isu
   terbuka sejak F7/F10, route-nya dipasang `Member`. Sudah tercatat di
   `docs/OPEN-ISSUES.md`.

### Yang dikerjakan

- Migrasi `0022`: index retensi (versi yang bisa dijalankan).
- `internal/storage/`: SigV4 presign PUT/GET/HEAD, URI-encoding SigV4, klien
  (HEAD + Fetch), key builder + sanitizer. **Tanpa SDK AWS** — yang dibutuhkan
  cuma HMAC-SHA256, dan menambah SDK untuk empat rumus berarti menambah pohon
  dependensi yang harus diaudit.
- `internal/artifact/`: service + adapter pgx. Batas 25 MB/file, kuota 100 MB
  per task (N22), verifikasi SHA-256.
- `cmd/api/artifacts.go` (5 handler) + `artifact_wiring.go` +
  `internal/config/storage.go`.
- Worker default **OFF** (`S3_*` kosong ⇒ endpoint balas 503, bukan 404:
  route-nya ada, konfigurasinya yang belum).

### Keputusan desain

- **Verifikasi digest benar-benar membaca objeknya.** §12.3 langkah 6 bilang
  "backend memverifikasi SHA-256 matching". Membandingkan string yang dikirim
  klien dengan string yang dikirim klien tidak memverifikasi apa pun, jadi
  objeknya diunduh dan digest-nya dihitung ulang. Di atas 8 MB isinya tidak
  dibaca ulang (`MaxVerifyBytes`) — `size` dan HEAD tetap diperiksa, tapi
  membaca ulang objek sebesar batas atas dikali beberapa pendaftaran paralel
  adalah tekanan memori yang tidak dijanjikan siapa pun.
- **`size` didaftarkan harus sama dengan yang dilaporkan storage.** Tanpa itu
  kuota N22 bisa dilewati dengan melaporkan angka apa pun.
- **`storage_key` harus di bawah prefix org DAN task.** Memeriksa org saja
  masih membolehkan artifact task lain didaftarkan ke task ini.
- **Key harus konsisten dengan `run_id` yang didaftarkan** — kalau tidak,
  barisnya benar dan isinya menunjuk pekerjaan orang lain.
- **Tanpa `S3_PATH_STYLE`.** Klien selalu path-style: R2 menerimanya, dan itu
  satu-satunya bentuk yang bekerja untuk endpoint non-DNS seperti `minio:9000`.
  Variabel yang tidak dibaca siapa pun adalah persis bagaimana nama env
  dispatcher melenceng dari manifest-nya.

### Bukti

- `tools/probe-f13.py` **28/28** lawan API nyata + MinIO. Termasuk siklus 12.3
  penuh: upload-url → PUT ke presigned URL (diterima storage) → register
  (digest dihitung ulang dari objek yang mendarat) → download 302 → isi unduhan
  **identik byte-per-byte**. Plus penolakan: digest salah, ukuran tidak cocok,
  key tenant lain, run tidak ada, file > 25 MB, viewer 403.
- `internal/storage/` 13 tes lawan MinIO nyata: round-trip, secret salah 403,
  URL kedaluwarsa ditolak, HEAD ukuran, vektor SigV4 resmi AWS untuk service
  `s3` DAN `iam` (supaya service-nya benar-benar kepaku).
- `internal/artifact/` 18 tes; `internal/migrate/` 3 tes (index ada, planner
  memakainya, versi partial kontrak benar-benar ditolak Postgres).
- Mutation 6/6 **CAUGHT**: digest tidak dihitung ulang, prefix org+task tidak
  diperiksa, ukuran storage tidak dibandingkan, key tidak dicocokkan dengan run,
  kuota task tidak ditegakkan, batas per file tidak ditegakkan.
- Gate `tools/gate-overnight.cmd` rc=0. `gofmt -l .` bersih.
- `go test`: cmd/api 26.0s, migrate 9.0s (tes baru), storage 1.9s, artifact 1.1s.

### Dua bug yang ditemukan server S3 nyata, bukan mock

1. **Presign path-style lupa menyertakan bucket di canonical path.** URL-nya
   menunjuk `/bucket/key` sementara yang ditandatangani `/key` ⇒
   `SignatureDoesNotMatch`. Tidak kelihatan sampai URL-nya benar-benar dikirim:
   URL-nya sendiri terlihat benar. Lalu perbaikan pertama salah —
   `canonicalPath` dipakai untuk merakit URL juga, jadi bucketnya ter-prefix dua
   kali (`/bucket/bucket/key`). Keduanya hanya ketahuan lawan server sungguhan.
2. **`sanitizeFilename("../../etc/passwd")` menghasilkan `_/_/etc/passwd`.**
   Segmen `..` diganti `_`, bukan dibuang, jadi key-nya masih terlihat seperti
   path. Sekarang segmen `..` dibuang seluruhnya ⇒ `etc/passwd`.

Satu lagi: vektor HMAC yang dipakai di tes pertama adalah vektor service
**`iam`**, bukan `s3`. Tesnya yang salah, bukan kodenya — sekarang keduanya
dipaku supaya `s3` benar-benar terbukti.

**Catatan operasional:** probe F13 menjalankan API **di host**, bukan di
container. Presigned URL punya satu host string dan yang membuatnya harus
menjangkaunya sama seperti yang memakainya; di container, host itu tidak punya
nama yang juga dikenal host. Postgres dan MinIO sama-sama di-publish ke
loopback, jadi API di host menjangkau keduanya. Skripnya di scratch
(`run-api-host.sh`), bukan di repo — ia meng-hardcode port dev lokal.

## F14 — audit CHECKLIST.md + brief baru (transisi ke frontend)

**Status:** selesai, ter-push. Gate rc=0.

**Kenapa ada fase ini.** User minta CHECKLIST.md dicek lawan kode, dan minta
overnight run lagi. Backend sudah 129 ✅ / 0 ⬜, jadi misinya pindah ke frontend —
dan brief lama masih menyuruh ngerjain endpoint §6.2 yang sudah tidak ada.

### Temuan audit (semua diverifikasi ke kode, bukan diklaim)

1. **`docs/CHECKLIST.md` BASI.** Bilang 13 PASS dari 89 story. Di-generate dari
   `tools/checklist_status.json` (17 entri) saat backend baru segelintir.
   **Jangan dipakai buat prioritas.**
2. **`docs/00-PRD.md` bukan pelacak progres:** 109 story, 387 item AC, **nol**
   tercentang sejak awal.
3. **Dua layar sudah ada tapi KOSONG (stub 21 baris):** `settings/ApiKeys.tsx`
   dan `settings/Webhooks.tsx` — dua-duanya bilang "not available yet" padahal
   backend-nya jalan (F10 5 endpoint, F12 7 endpoint).
4. **`store/api/stream.ts` (158 baris + test) nol pemakai.** Implementasi SSE
   lengkap: EventSource, patch cache RTK, `Last-Event-ID` resume. Tapi cuma
   `stream.test.ts` yang mengimpornya.
5. **`hooks/use-sse-cache.ts` nol pemanggil.** Didefinisikan, tidak pernah dipasang.
6. **`TaskDetailDrawer.tsx:44-46` alasannya basi.** Komentarnya bilang tab
   Logs/Artifacts/Approvals butuh endpoint `runs`/`artifacts`/`approvals` yang
   "do not exist". Ketiganya **sudah ada dan terbukti** lawan API nyata.
7. **22 dari 52 layar (`docs/COVERAGE.md`) belum ada filenya.** Daftar lengkap
   ditulis di `docs/OPEN-ISSUES.md`.

### Yang dikerjakan

- `docs/OVERNIGHT-BRIEF.md` **ditulis ulang**: misi → frontend, 8 fase termurah
  dulu (Artifacts tab → Logs tab → Approvals tab → pasang SSE → Webhooks → API
  Keys → assignee picker → task archived), sumber progres yang sah disebut
  eksplisit, CHECKLIST ditandai basi.
- `docs/OPEN-ISSUES.md`: bagian audit CHECKLIST.md + daftar 22 layar.
- `frontend/src/routes/dashboard/boards/TaskDetailDrawer.tsx`: komentar basi
  dibetulin — menyebut endpoint yang **sudah** ada, dan menyuruh cek §6.2 dulu
  sebelum menyalin alasan apa pun dari file itu.

### Verifikasi

- `tsc -b` rc=0; `prettier --check` pada file yang disentuh: bersih.
- `./tools/gate-overnight.cmd` rc=0 (4.59s).
- Nol file Go disentuh di fase ini, jadi `go test` tidak dijalankan — dan itu
  disebut di sini, bukan didiamkan.

### Catatan buat sesi berikutnya

Fase 2 (Logs tab) punya risiko duplikasi: drawer sudah menampilkan timeline dari
`useBoardEventsQuery`. **Cek dulu** apakah "Logs" di design beda dari "Timeline"
yang sudah ada; kalau sama, jangan bikin dua yang isinya sama.

## F15 — verifikasi per-story lawan kode (lanjutan audit CHECKLIST)

**Status:** selesai, ter-push.

**Kenapa ada fase ini.** F14 cuma mengaudit per-*file* (52 layar ada/nggak).
Itu belum menjawab "story mana yang sudah dikerjakan". Fase ini menghitung
**89 story M0–M4** satu per satu lawan kode.

### Hasil: 13 PASS → 68 PASS

| Verdict | Jumlah | Dasar |
|---|---:|---|
| PASS backend-only | 21 | endpoint terpasang & terverifikasi lawan API nyata |
| PASS UI | 47 | file layar ada **dan** komponennya terpasang (router / dirender induk) |
| dikerjakan (wip) | 4 | US-AD19, US-AD67, US-AD73, US-AD108 |
| belum (todo) | 16 | 14 layar belum ada filenya + 2 backend-only kosong |
| ditunda (defer) | 1 | US-AD92 (keputusan user) |

Rincian: `docs/CHECKLIST.md`, diregenerate dari `tools/checklist_status.json`
(17 → 98 entri).

### Dua story backend-only yang BENAR-BENAR kosong

- **US-AD85 (rate limit)** — nol `RateLimit` di seluruh repo. Dicari di 100 file
  Go non-test; nol hit.
- **US-AD50 (deteksi string keras di CI)** — nol `.github/workflows/`, nol
  pengecekan literal/keras di `tools/`.

### Tiga hipotesis yang gw buang setelah diverifikasi

Ditulis di sini supaya sesi berikutnya tidak mengulang tebakan yang sama:

1. **"US-AD49 (i18n) belum jalan"** — SALAH. `frontend/src/lib/i18n.ts` punya
   blok `en` (baris 509) dan `id` (baris 1000), 1383 key, `DICTIONARIES` dua-duanya.
2. **"US-AD05 (force logout) belum jalan"** — SALAH. `internal/auth/sessions.go`
   punya `Sessions`/`ChangePassword`/`RevokeSession`; route
   `GET /auth/sessions` + `DELETE /auth/sessions/{id}` terpasang di `main.go:407-409`.
3. **"18 story ber-layar kehilangan route-nya"** — SALAH, dan ini yang paling
   menipu. Nol komponen di-import-tanpa-dipakai di router. Yang terlihat
   "hilang" itu sub-komponen (`TaskCreateForm`, `TaskDrawerHost`,
   `CreateAgentForm`, `AgentProviderKeyPanel`, `ColumnEditor`, dst) yang
   dirender **di dalam** layar induk — diverifikasi satu per satu punya pemanggil.

### Batasan yang harus disebut

**"PASS" di sini = ada filenya + terpasang. BUKAN design match.** Kesamaan
dengan `design/stitch-output/v2/*.html` cuma diketahui untuk layar yang pernah
di-inventory; sisanya belum diukur. Jangan naikkan status jadi "selesai" tanpa
inventory elemen.

### Verifikasi

- Nol file Go/produksi disentuh — fase ini murni dokumentasi + sidecar JSON.
  `go test` **tidak** dijalankan, dan itu disebut di sini, bukan didiamkan.
- Diff sidecar lawan hasil audit komputasi: **0 selisih** dari 89 story
  (dicek ulang otomatis sebelum generate).
- `python tools/update_checklist.py` → `wrote docs\CHECKLIST.md (77 PASS, 98 tracked)`.

### F15 tambahan — full e2e dijalankan (user mengizinkan)

**128 tes, 0 gagal.** Terukur per shard: 1=42 (124s), 2=25 (47s), 3=38 (85s),
4=23 (51s). Total ~307 detik.

Perintahnya masuk brief §3b: `--shard=N/4 --workers=1 --trace=off`,
`--output` unik per run, foreground + redirect, artifact dihapus setelah selesai.

**Satu flake nyata, bukan regresi — dan buktinya hilang karena kesalahan gw.**
Run pertama shard 3: `column-editor.spec.ts:139 AC1` gagal,
`locator.fill` timeout 60 detik menunggu `getByLabel('Nama kolom baru')`.
- Label itu **ada** di komponen (`components/boards/ColumnEditor.tsx:326`,
  `aria-label`), dan form-nya selalu dirender di panel.
- Dijalankan sendiri: **7/7 hijau, AC1 2.6 detik**.
- Shard 3 diulang: **38/38 hijau**, 87 detik — run pertama 168 detik, dua kali
  lebih lambat.

Kesimpulan: tekanan resource (satu sesi Playwright lebih lama hidup), bukan
regresi. **Tapi screenshot + `error-context.md` kegagalan itu ketimpa** karena gw
memakai `--output=.e2e-out` yang sama untuk run standalone sesudahnya. Aturan
`--output` unik per run lahir dari kesalahan ini.

### Fase 0 — SSE dipakai: board hidup tanpa reload (US-AD39)

**Temuan: bridge-nya tidak pernah tersambung.** `stream.ts` (158 baris) sudah
lengkap sejak lama — `EventSource`, resume `Last-Event-ID`, batas percobaan
ulang — tapi nol komponen memakainya. `use-sse-cache.ts` juga nol pemanggil.
Yang dipakai hanya `useBoardEventsQuery` langsung di drawer, dan itu cuma
membaca tail; board-nya tidak pernah ikut ter-refresh.

**Dua jebakan nyata, keduanya kena:**

1. **`invalidatesTags` pada endpoint ber-`queryFn` diabaikan RTK.** Versi pertama
   perbaikan ini menulis `invalidatesTags` di `boardEvents`. Tipenya `never`,
   nilainya diabaikan, dan **tidak ada satu pun error runtime** — board tetap
   diam. Cuma `tsc` yang menangkap. Invalidasi sekarang di-dispatch dari handler
   event (`api.dispatch(api.util.invalidateTags(...))`).
2. **Satu event = satu refetch = badai.** Dispatcher yang menyelesaikan satu run
   mengeluarkan burst (`step.finished`, `ledger.entry`, `run.finished`).
   Refetch di-koalesce `REFRESH_COALESCE_MS = 150`, dan timer yang tertunda
   dibatalkan saat cache entry dilepas.

**Temuan sampingan yang nyata:** tag `Event` dideklarasikan di `TAG_TYPES` dengan
**nol provider**. Dua mutasi (`createTask`, `moveTask`) meng-invalidasinya —
refresh nol, tapi terbaca seperti me-refresh timeline. Dibuang.

**Tag jadi satu fungsi, bukan tiga literal.** `boardTaskTag(boardID)` di `base.ts`
dipakai `listTasks` (provides), `boardEvents` (dispatch), dan `createTask`
(invalidates). Sebelumnya literal `BOARD-${boardID}` ditulis di tiga tempat dan
hanya dua yang sepakat.

**UI:** indikator `Live` di `BoardToolbar` (satu mount, dua view — board & table,
jadi pindah view tidak memutus stream), i18n `boards.live`/`liveHint` di `en`+`id`,
token `--color-success` dari DESIGN.md.

Bukti:
- `src/store/api/stream-wiring.test.ts` — 12/12; mutation **7/7 CAUGHT**
  (refresh dihapus, `invalidatesTags` dikembalikan, guard `boardID` dilumpuhkan,
  cleanup timer dibuang, prefix helper diubah, `createTask` literal, `listTasks`
  literal dua cabang).
- `e2e/board-live.spec.ts` — **3/3**: indikator live; **task dari klien lain
  muncul tanpa reload**; burst 3 task semuanya muncul. Mutation e2e (refresh
  dihapus) → **1 failed**, jadi tesnya benar-benar mengukur fiturnya.
- Vitest 80/80 · `tsc -b` rc=0 · prettier `src/` bersih · gate rc=0 (4.14s).
- Full e2e **131 passed / 0 failed** (128 lama + 3 baru), 4 shard.
- `go test` tidak dijalankan: fase ini nol perubahan Go.

**Dua kesalahan gw sendiri, dicatat supaya tidak terulang:**
- Semua edit lewat Python menulis **CRLF** (7 file), dan gate tetap hijau —
  prettier/gate tidak memeriksa line ending. Ketahuan dari `file` + `git show`.
  Semua dinormalkan ke LF.
- Playwright `page.request` **tidak mengirim cookie `Secure` lewat HTTP**, jadi
  setiap panggilan API setelah register menjawab 401 sementara browser-nya
  mengirim dengan lancar. Itu sebabnya suite yang ada memakai `page.evaluate`.
  `signUp` sekarang me-re-`addCookies` dengan `secure: false` sehingga satu jar
  dipakai browser dan request context.

**Tidak ada deviasi urutan:** ini fase 0 di brief, dikerjakan pertama seperti
tertulis.

**Koreksi entri ini sendiri:** versi pertama entri ini menamainya "Fase 1" dan
mengaku ada deviasi dari brief. Salah dua-duanya — yang dikerjakan memang fase 0,
dan urutannya tidak menyimpang. Angka fase di dokumen ini harus cocok dengan
brief §2, kalau tidak sesi berikutnya mengira sesuatu sudah lewat padahal belum.

### Fase 1 — Tab Artifacts (US-AD48) + tab shell drawer

**Keputusan yang gw ambil dan alasannya.**

1. **Drawer belum punya tab sama sekali.** Design `20-task-drawer.html` baris
   1323 menggambar 4 tab (Timeline/Logs/Artifacts/Approvals) dengan `tab-count`;
   implementasinya menumpuk semuanya dalam satu kolom gulir. Jadi fase ini
   mengerjakan **shell tab** lebih dulu (Timeline + Artifacts hidup, Logs +
   Approvals placeholder terlihat tapi dinonaktifkan — biar tidak terlihat
   seperti fitur yang rusak).
2. **Fase 1 DIPECAH: upload ditunda ke 1b.** Klien `useRegisterArtifactMutation`
   butuh `run_id`, dan `run_id` wajib di kedua endpoint (keputusan F13). Artinya
   upload butuh UI pemilihan run — pekerjaan yang lebih besar, dan `runs` belum
   ada di klien sama sekali. AC US-AD48 sendiri hanya minta **daftar** (AC1) dan
   **perilaku URL unduh** (AC3, AC4). Deviasi ini dicatat, bukan didiemin.
3. **Unduh lewat `<a href>` ke `/download`, bukan `fetch` + blob.** Endpoint-nya
   menandatangani URL segar per request dan membalas 302; browser mengikuti dan
   tautan bertanda tangan tidak pernah mendarat di state komponen tempat dia
   bisa hidup lebih lama dari masa berlakunya. Itu inti AC4.
4. **`invalidatesTags: ['Artifact']` sengaja tidak dipasang.** Tag `Artifact`
   ada di `TAG_TYPES` tapi **nol pemakai** — pola yang sama dengan `Event` yang
   gw buang di fase 0. Dibiarkan: menambah tag mati baru bukan perbaikan.

**Tiga temuan yang terverifikasi lawan kode, bukan tebakan.**

- **US-AD48 AC2 (paginasi cursor) nol implementasi.** `ListTaskArtifacts` nol
  `LIMIT`. UI-nya sengaja tidak mengarang. Ada `ponytail:` di
  `store/api/artifacts.ts`.
- **Unduhan tampil inline, bukan tersimpan.** Awalnya test-nya
  `waitForEvent('download')` dan timeout 60 detik. Snapshot DOM saat gagal
  menunjukkan teks filenya ter-render di halaman: Chrome **menampilkan**
  `text/plain` inline. Penyebabnya dua — `Release` presigned tidak membawa
  `ResponseContentDisposition`, dan atribut `download` HTML diabaikan untuk URL
  beda origin (storage :9000 vs app :5174). Perbaikannya API change; dicatat di
  OPEN-ISSUES.
- **Semua endpoint artifact 503 kalau `S3_*` tidak diset**, termasuk `GET`
  daftar. Akibatnya suite e2e ini tidak bisa hijau di lingkungan biasa.

**Bug nyata yang ketemu lewat test (dan kelasnya layak dicatat):**
`error.status` dari RTK Query **bukan** kode HTTP di sini. Semua error API ditulis
`http.Error` = `text/plain`, sementara base query mem-parse JSON; parse gagal →
`status: 'PARSING_ERROR'`, kode aslinya di `originalStatus`. Cabang
`status === 503` **compile tanpa error dan tidak pernah cocok**. Diverifikasi
dengan probe: `{status:503, contentType:"text/plain", jsonParse:"THROW"}`.
Sekarang `Number(originalStatus ?? status)`.

**Line ending:** edit lewat python mengembalikan CRLF di 4 file lagi. Di-LF-kan,
`file` diverifikasi, `prettier --check src/` bersih. Jebakan ini berulang di
tiap fase; sudah tercatat di `.hermes.md`.

**Test baru:**
- `e2e/artifacts.spec.ts` — 2 test. "dua empty state" jalan di mana saja;
  "round-trip byte" `skip` otomatis kalau API 503, dan **dijalankan sungguhan
  lawan rig MinIO** (presign → PUT → register verifikasi SHA-256 → unduh 302 →
  byte identik).
- `playwright.config.ts` — `E2E_BASE_URL` opsional; tanpa itu semua spec jalan di
  :5173 seperti sebelumnya (diverifikasi: full e2e hijau).

**Mutation (5, semua CAUGHT):** cabang 503 dibuang; baca `status` bukan
`originalStatus` (bug aslinya); empty state dibuang; `href` bukan endpoint
signing; panel Artifacts tidak dirender.

**Bukti eksekusi:**
- gate `./tools/gate-overnight.cmd` rc=0.
- `vitest run` 80/80; `tsc -b` rc=0; `prettier --check src/` bersih.
- full e2e 4 shard: 42 / 29+1 skip / 28 / 33 = **132 passed, 1 skipped, 0 failed**.
- suite artifact lawan rig MinIO: **2/2 passed** (termasuk round-trip byte).
- Yang **tidak** dites: tab Logs dan Approvals masih placeholder; upload belum ada.

### Fase 2 — Tab Logs: timeline step per run (US-AD26, US-AD94)

**Dua endpoint sudah hidup, nol pemanggil.** `GET /tasks/{id}/runs` dan
`GET /runs/{id}/steps` plus tabel `steps` (append-only, `seq` unik per run)
sudah lengkap sejak run lifecycle mendarat. Yang tidak ada: klien `runs` di
frontend. Jadi tab ini menunggu klien, bukan endpoint.

Yang dibangun:
- `store/api/runs.ts` — `listTaskRuns`, `listRunSteps` (tag `Run`, `Step`).
- `components/boards/RunSteps.tsx` — setiap run task dengan step-nya, warna
  status, token/biaya `tabular-nums`, expand → payload.
- Panel Logs di drawer menggantikan placeholder.

**Tiga keputusan, dengan alasannya:**

1. **Dikelompokkan per run, bukan satu daftar step datar.** Run adalah unit yang
   di-retry, jadi task yang gagal dua kali lalu berhasil punya tiga run dan satu
   di antaranya yang penting. Daftar datar menggabungkannya dan menyembunyikan
   step itu milik percobaan ke berapa. (Klien AC94 minta `run_id` di step;
   route-nya belum ada — dikelompokkan lewat run, bukan lewat field yang tidak
   ada.)

2. **Upload artifact (Fase 1b) butuh klien ini, jadi dia jalan setelah ini.**
   `run_id` wajib di kedua endpoint artifact (keputusan F13), dan sekarang tinggal
   pakai `POST /runs/{id}/steps` yang sudah dipakai tab ini.

3. **`data-status` di ikon status.** Alasannya bukan gaya: `svg.text-[var(--color-success)]`
   **bukan selector CSS yang valid** dan `querySelectorAll` menolaknya dengan
   `SyntaxError` — itu kegagalan test pertama, dan pesannya menunjuk ke locator
   bukan ke tesnya. Atribut ini juga menghindari memaku nilai hex yang DESIGN.md
   yang punya.

**Tiga temuan yang diverifikasi lawan kode (semua dicatat, satu diperbaiki):**

- **`finishRunStep` membuang payload.** Handler menerima `payload` di body, lalu
  membangun `board.Step{Status, CostMicros}` saja. Payload hanya tersimpan saat
  step DIBUKA (`StartStep`). Tes gw sendiri yang ketahuan: mengirim payload lewat
  PATCH adalah no-op yang tidak berbunyi. Komentar di test sekarang menyebut
  perilakunya.
- **`steps` hanya punya SATU kolom `payload_json`**, padahal AC94 AC2 minta
  payload **masuk dan keluar**. Payload keluar tidak punya tempat. Panel
  menampilkan yang ada; menambah kolom + mengubah handler = perubahan kontrak,
  jadi tidak dikerjakan malam ini.
- **AC94 AC1 minta cache read/write per step** — kolomnya tidak ada di `steps`,
  ada di `ledger_entries`. Merender 0 adalah angka karangan, jadi panel
  menampilkan token dan biaya yang step benar-benar laporkan. Cache masuk fase
  Ledger explorer.

AC94 AC5 (payload disamarkan untuk viewer) belum dikerjakan: app ini belum punya
permukaan penyamaran per peran di mana pun. Diklaim di UI berarti aturan yang
tidak ditegakkan di mana pun.

**Verifikasi:** gate rc=0 · vitest 80/80 · `tsc -b` rc=0 · prettier bersih ·
e2e logs 3/3 · **full e2e 135 passed / 1 skipped / 0 failed** (128 → +3 Fase 0,
+3 Fase 2, +1 Fase 1 yang di-skip saat storage mati). CHECKLIST regenerasi:
**70 PASS / 89** (dari 68).

**Mutasi 6 CAUGHT, dan dua SURVIVED yang mengungkap asert kosong milik gw.**
Mutan "payload tidak di-pretty-print" SURVIVED dua kali:
- asert pertama `toContainText('"completion"')` cocok untuk JSON mentah MAUPUN
  pretty-print;
- versi berspasi (`'"completion": "selesai"'`) juga cocok, karena kolom
  `payload_json` adalah **JSONB** dan Postgres sudah mengembalikannya berspasi
  `{"a": 1}` persis seperti `JSON.stringify` untuk objek datar.

Bedanya baru muncul saat payloadnya **bersarang**: pretty-print menghasilkan baris
baru, JSONB tidak. Asert final menghitung jumlah baris. Mutan lain yang CAUGHT:
durasi "running" diganti 0ms (AC94 AC3), warna status dihapus (AC26 AC1), payload
kosong jadi error (AC26 AC3), step diurutkan terbalik di klien, empty state
diganti teks gagal.

**Kesalahan proses gw yang perlu dicatat:** satu sel `execute_code` gagal di
tengah setelah menerapkan 3 edit; karena `write_text` baru jalan di akhir,
ketiga edit itu tidak pernah tersimpan dan gw menemukannya dari kegagalan test
berikutnya (`write_file` ter-render dengan status `running`). Verifikasi edit
sebelum menjalankan test itu lebih murah daripada membaca log test.

### Fase 1b — Unggah artifact dari browser (US-AD46)

**Keputusan yang gw ambil:**

- **Rantai tiga panggilan dipertahankan apa adanya:** `upload-url` -> PUT →
  object storage -> `register`. Yang menentukan bentuk UI-nya adalah di mana
  byte-nya lewat: kalau lewat API, satu file 25 MB mendarat di heap Go. Jadi
  PUT-nya `fetch(ticket.upload_url)` langsung dari browser, **tanpa header
  Authorization** — URL-nya sudah ditandatangani, dan menambahkan kredensial di
  situ berarti menyerahkannya ke pihak ketiga tanpa alasan.
- **`sha256Hex(file)` bukan pembukuan.** Server meng-hash ulang objek yang
  benar-benar mendarat dan menolak barisnya kalau digest tidak cocok
  (`internal/artifact/service.go:299`). Hash yang dihitung klien adalah KLAIM
  yang diperiksa server — itu sebabnya mutan "hash dari string kosong" CAUGHT.
- **`run_id` diambil dari run terakhir**, bukan dipilih. Kontrak §6.2.16 cuma
  menyebut `(filename, size)`, tapi DDL §3.15 menaruh `run_id` di key dan
  `artifacts_run_fk` menegakkannya (keputusan F13). Picker run akan jadi UI
  yang bertanya hal yang jawabannya sudah jelas: artefak diproduksi oleh run
  terakhir.
- **Tiga penolakan ditampilkan, bukan didiamkan:** belum ada run, bukan Member,
  dan browser tanpa `crypto.subtle`. Masing-masing menggantikan bentuk gagal
  yang tidak berbunyi — form yang tampil lalu 400, tombol mati tanpa alasan.

**Bukti:**

- **e2e lawan rig MinIO (:5174 + API host):** `artifacts.spec.ts` **3/3**.
  Test barunya menempuh jalur penuh: `setInputFiles` ke input file yang sama
  dengan yang dipakai operator -> PUT ke MinIO **melewati CORS** -> `register`
  -> baris muncul dari invalidasi tag -> unduh ulang lewat API dan bandingkan
  byte-nya.
- **Full e2e: 135 passed / 2 skipped / 0 failed.** Dua yang skipped adalah tes
  yang butuh object storage; di container `S3_*` kosong, jadi API menjawab 503
  sebelum soal byte muncul. Keduanya di-skip dengan alasan terulis, bukan
  dibiarkan gagal — kegagalannya bukan kabar tentang fitur ini.
- **Mutasi: 3 CAUGHT / 1 SURVIVED.** CAUGHT: hash bukan dari isi file, PUT
  dihapus, `invalidatesTags` dibuang, tag di-invalidate ke id lain.
  **SURVIVED: guard `crypto.subtle`** — Chromium selalu punya WebCrypto, jadi
  cabang itu tidak bisa dijangkau e2e. Ia tetap benar sebagai penjaga, tapi
  **tidak terverifikasi**; dicatat di `docs/OPEN-ISSUES.md`.
- Gate rc=0 · tsc rc=0 · vitest 80/80 · prettier bersih.

**Pelajaran harness mutasi (gw sendiri yang kena):** versi pertama skrip mutasi
memulihkan file **setelah loop**, jadi begitu sel itu crash di tengah, mutant
terakhir **tetap di disk**. Sesi berikutnya membaca `if (false) {` di komponen
dan bakal menghabiskan waktu mengejar "bug" yang tidak pernah ada. Sekarang
restore-nya per-mutan di dalam `finally`, dan baseline wajib rc=0 sebelum mutan
pertama jalan — kalau baseline merah, hasilnya dibuang.

**Catatan lingkungan:** tes upload butuh API yang punya `S3_*` **dan** presigned
host yang dijangkau browser. Rig-nya: API di host + dev server kedua di :5174
(cara jalaninnya di brief §3c). **Setelah selesai rig-nya WAJIB dimatikan**;
kalau tidak, suite biasa lewat :5173 gagal karena container API sudah mati.

### Fase 3 — Tab Approvals di drawer (US-AD34, US-AD35)

**Keputusan yang gw ambil:**

- **Penyaringan per task di klien, dan itu bukan jalan pintas.** `GET
  /api/v1/approvals` tidak menerima `task_id`; ia mengembalikan antrean seluruh
  organisasi. Yang penting: **repository-nya PUNYA `ListTaskApprovals`**
  (`queries.sql:1646`, service `TaskApprovals` di `internal/board/approval.go:199`)
  — query-nya sudah ada, **tidak ada route yang memasangnya**. Jadi ini bukan
  parameter yang gw lewatkan; ini endpoint yang belum pernah didaftarkan. Kalau
  dipasang, ia juga akan membawa riwayat yang sudah diputus, yang justru tidak
  diinginkan tab ini. Dicatat di `docs/OPEN-ISSUES.md`.
- **Penyaring `pending` DIHAPUS setelah mutasi menunjukkan ia mubazir.** Server
  sudah menyaring `decision = 'pending' AND expires_at > now()`
  (`ListPendingApprovals`). Menyalin aturan itu ke klien berarti dua tempat yang
  bisa berbeda pendapat soal batas kedaluwarsa. Sekarang layar menampilkan apa
  yang diberikan; tepi kedaluwarsa tetap milik server.
- **Role gate membaca `admin`, mengikuti server, bukan cerita.** US-AD34 AC3
  bilang approve = owner/admin/**member**; US-AD35 AC4 bilang reject =
  owner/admin saja. Implementasi server menaruh **keduanya di `admin`**. Kalau
  layar memisahkan keduanya, seorang member akan melihat tombol setujui yang
  dijawab 403. Konflik ini dicatat di `OPEN-ISSUES.md`, bukan diselesaikan
  diam-diam dengan memilih satu sisi.
- **Tombol tolak mati sampai alasannya diisi**, karena server menolak reject
  tanpa `reason` (US-AD35 AC2). Tombol hidup yang selalu gagal tidak mengajarkan
  apa pun.
- **Tiga hal yang gw coba dan gagal, supaya tidak diulang:** (1) `GET
  /api/v1/members` **tidak ada** — route-nya `GET /api/v1/orgs/{id}/members`;
  (2) menurunkan owner terakhir lewat `PATCH .../members/{user_id}` dijawab
  `ErrLastOwner` (`internal/auth/errors.go:20`), jadi peran viewer **tidak bisa**
  disemai lewat API — dipakai stub `GET /auth/me`, cara yang sama dengan
  `approvals.spec.ts`; (3) label tab di bawah bahasa default (`id`) adalah
  **"Persetujuan"**, bukan "Approvals".

**Bukti:**

- **e2e `approvals-tab.spec.ts` 3/3.** Yang paling berharga: approval milik task
  LAIN harus tidak muncul — kesalahan tipe itu akan membuat tab menampilkan
  pekerjaan orang lain. Juga: payload bersarang dirender apa adanya, menyetujui
  dari drawer menghapus kartunya, tolak mati tanpa alasan, dan viewer tidak
  melihat satu tombol pun.
- **Full e2e: 138 passed / 2 skipped / 0 failed.**
- **Mutasi 3 CAUGHT / 0 SURVIVED:** penyaring `task_id` dibuang, role gate
  dibuang, payload tidak dirender.
- Gate rc=0 · tsc rc=0 · vitest 80/80 · prettier bersih · CHECKLIST 70 -> **72 PASS**.

### Fase 4 — Layar detail approval (US-AD34, US-AD35)

**Keputusan yang gw ambil:**

- **Dua hal di mockup sengaja TIDAK dibangun, dan alasannya lebih penting
  daripada yang dibuang:**
  - *"Field `reason` minimum 10 karakter"*. Server hanya menuntut **non-kosong
    setelah `TrimSpace`** (`internal/board/approval.go:218-220`). Aturan mockup
    itu akan menolak alasan sah yang diterima API, tanpa jalan bagi operator
    untuk tahu kenapa. Tesnya menyematkan alasan 6 karakter supaya aturan
    mockup tidak diam-diam ikut masuk. Dicatat di `OPEN-ISSUES.md`.
  - *Tabel hak akses penyetuju* (`US-AD34`, route `/approvals/:id/approve`,
    `RBAC US-AD37 AC4`). Itu narasi spec, bukan hal yang bisa ditindak pengguna
    layar. Role gate-nya sama dengan tab Fase 3.
- **Digest `sha256:` di mockup tidak gw tiru.** Mockup mencetak
  `sha256:4b8e21a...7f9c` — string yang tidak diverifikasi apa pun. Menampilkan
  potongan yang tidak bisa dicocokkan siapa pun itu teater; yang auditabel adalah
  payload apa adanya, dan itu sudah dirender verbatim.
- **Jejak event dibaca dari `useSseCache`, bukan tabel `events`.** Itu jalur yang
  sudah terbukti di Fase 0 dan menyegarkan board maupun antrean; tabel `events`
  tidak punya endpoint sendiri.
- **`reason` hanya ditampilkan selama gate terbuka.** Lihat temuan di bawah.

**Temuan baru (diverifikasi lawan kode):**

`DecideApproval` menulis `reason = COALESCE($5, reason)`
(`internal/store/queries/queries.sql:1664`). Jadi **kolom `reason` dipakai ulang**:
alasan pemohon memasang gate, lalu alasan penyetus menimpanya saat menolak —
alasan asli permintaan **hilang** setelah diputus. Karena itu panel aksi hanya
menampilkannya selagi gate terbuka, dan panel keputusan setelahnya: satu fakta,
satu label. Perbaikan sebenarnya butuh kolom `decision_reason` terpisah ->
keputusan produk, dicatat di `OPEN-ISSUES.md`.

**Bukti:**

- **e2e `approval-detail.spec.ts` 2/2** — dari antrean -> detail -> kembali, dan
  penolakan ber-alasan pendek diterima. Yang kedua sengaja menyematkan perilaku
  SERVER, bukan mockup.
- **Full e2e: 140 passed / 2 skipped / 0 failed.**
- **Mutasi 4 CAUGHT / 0 SURVIVED:** payload tidak dirender, guard alasan dibuang,
  panel keputusan selalu dirender (riwayat hilang), link antrean dibuang.
- Gate rc=0 · tsc rc=0 · vitest 80/80 · prettier bersih.

**Catatan:** `SectionTitle` ternyata komponen **lokal** di `TaskDetailDrawer`,
bukan ekspor `@/components/ui/card`; `formatDateTime` dari `@/lib/format` (bukan
`@/lib/formatters`); `useSseCache` dari `@/hooks/use-sse-cache`. Tiga impor yang
gw tebak salah dan ketahuan dari `tsc`.

### Fase 5 — Layar detail run (US-AD41)

**Keputusan yang gw ambil:**

- **AC2 minta 5 tab; gw bangun 2, dan sisanya gw sebut.** `GET /runs/{id}/steps`
  dan `GET /runs/{id}/ledger` memang per-run. Approvals dan Artifacts dilist per
  TASK — keduanya punya kolom `run_id`, jadi versi jujurnya menyaring, dan itu
  pekerjaan fase Ledger explorer. Untuk task yang di-retry, menyalinnya ke sini
  berarti menampilkan hasil run LAIN di bawah nama run ini. Event run malah tidak
  punya route sama sekali. Semuanya dicatat di `OPEN-ISSUES.md`.
- **Durasi diambil dari `/summary`, bukan dari `runs`.** Baris `runs` tidak
  menyimpan durasi: untuk run yang belum selesai, server mengukurnya terhadap
  `now()`. Kalau `/summary` gagal, layarnya tetap menampilkan data run (turunan
  boleh hilang, sumbernya tidak).
- **404 dibedakan dari gagal muat** (AC3/AC4). Server sengaja menjawab 404 yang
  sama untuk "tidak ada" dan "milik org lain" supaya id tenant lain tidak bisa
  dipancing; layarnya menyebut itu sebagai satu kalimat.
- **Layar ini diberi pintu masuk.** Baris run di tab Logs (Fase 2) sekarang link
  ke sini — rute tanpa pintu tidak bisa dicapai siapa pun, dan dites dari UI.
- **`StepTimeline` dipakai ulang** untuk tab Steps, bukan renderer kedua: bentuk
  yang dibacanya adalah `{kind, payload, created_at}`, jadi envelope-nya disintesis
  di batas komponen dan `StepTimeline` tetap tidak tahu soal run.

**Temuan baru (diverifikasi lawan kode):**

`stepRequest` membaca field **`payload`**, sedangkan respons mengembalikan
**`payload_json`** (`cmd/api/runs.go:473-481`). Mengirim `payload_json` dijawab
**201 dengan payload null** — sukses tanpa data. Ini kejadian **ketiga** soal
payload di jalur run, sesudah `finishRunStep` membuang payload dan `steps` hanya
punya satu kolom padahal AC94 AC2 minta dua. Ketiganya arah yang sama: payload
adalah bagian yang paling gampang hilang tanpa suara di modul ini.

**Bukti:**

- **e2e `run-detail.spec.ts` 3/3** — ringkasan + step nyata, pintu masuk dari
  drawer, dan 404 untuk run yang bukan milik org ini.
- **Full e2e: 143 passed / 2 skipped / 0 failed.**
- **Mutasi 3 CAUGHT / 0 SURVIVED:** durasi `/summary` dibuang, 404 tidak
  dibedakan, tab Steps tidak memuat step.
- Gate rc=0 · tsc rc=0 · vitest 80/80 · prettier bersih.

**Catatan harness:** `providesTags` `listTaskRuns` sempat gw ubah jadi
`TASK-${taskID}` tanpa perlu; gw balikin setelah memverifikasi tidak ada yang
meng-invalidate tag `Run` selain deklarasinya di `base.ts`. Perubahan tag yang
tidak diminta adalah cara paling sunyi untuk mematikan refresh tab Logs.

### Gate `/goal` rusak — butuh perbaikan dari dalam sesi

Bukan masalah repo. `tools/gate-overnight.cmd` sendiri **lulus** (`cmd.exe /c
"tools\gate-overnight.cmd"` -> rc=0, dijalankan dua kali). Yang rusak adalah
**command yang tersimpan di state goal**: `./tools/gate-overnight.cmd`.

Gate dijalankan `subprocess.run(command, shell=True)`, yang di Windows berarti
`cmd.exe /c <command>`, dan cmd.exe tidak menerima prefix `./`:

    $ cmd.exe /c "./tools/gate-overnight.cmd"
    '.' is not recognized as an internal or external command
    $ cmd.exe /c "tools\gate-overnight.cmd"
    rc=0

**Kenapa tidak bisa diperbaiki dari sesi ini.** `_session_bound_manager`
(`hermes_cli/cli_loops_mixin.py:450-455`) mengembalikan GoalManager dari MEMORI
proses selama `session_id` tidak berubah -- tidak membaca DB ulang. Sesi CLI yang
hidup masih memegang daftar gate lama (dimuat sebelum perbaikan), menjalankannya,
gagal, lalu `_save()` menulis balik state basi itu. Perbaikan out-of-process
(dibuktikan tersimpan: `attempts: 0`, `last_exit_code: null`) ditimpa dalam satu
siklus gate. `hermes` juga tidak punya subcommand `goal`, jadi tidak ada jalur
CLI dari luar.

**Perbaikan yang benar** -- dijalankan di dalam sesi ini sebagai slash command:

    /goal gate remove 1
    /goal gate add tools\gate-overnight.cmd

Verifikasi setelahnya: `/goal gate list` harus menampilkan gate itu tanpa
"✗ failing".
