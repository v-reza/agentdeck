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
