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
