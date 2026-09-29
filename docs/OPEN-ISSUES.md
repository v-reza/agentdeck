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

- **§13.3 tidak rekonsiliasi dengan dirinya sendiri.** Enam jeda retry
  (1m, 5m, 15m, 30m, 1j, 2j) berarti tujuh percobaan, sementara kalimat
  berikutnya bilang "total 6 percobaan" dan "setelah 6 percobaan" jadi dead.
  Yang dipakai implementasi: enam percobaan, lima jeda — aturan dead-letter
  yang mengikat. Nilai 2 jam dari daftar itu karena itu tidak terpakai.
  **Belum diperbaiki**: memperbaikinya berarti memilih salah satu angka, dan
  itu keputusan produk.

- **Frontend belum memakai webhook.** Tujuh endpoint §6.2.18 jalan dan
  terbukti lawan API nyata (`tools/probe-f12.py` 30/30, termasuk pengiriman
  keluar dengan HMAC terverifikasi), tapi belum ada layar untuk mendaftarkan
  atau membaca riwayat pengiriman. **Belum dikerjakan** — itu perubahan di
  `frontend/`, bukan penambahan endpoint.

- **Tidak ada role "Worker" di kode.** §6.2.14 dan §6.2.16 meminta
  `Internal/Key` dengan role Worker, tapi `auth.Role` hanya Owner/Admin/Member/
  Viewer. Route worker (approval, artifact upload) karena itu dipasang `Member`.
  **Butuh keputusan produk**: apakah Worker jadi peran kelima, atau kredensial
  worker tetap diwakili API key berperan Member selamanya.

- **`artifacts_storage_key_idx` (§3.15) sengaja tidak dibuat.** Kontrak bilang
  index itu melayani download by storage_key, tapi endpoint-nya
  `/artifacts/{id}/download` — lewat primary key. Dicatat supaya tidak ada yang
  menambahkannya diam-diam demi mencocokkan dokumen.

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

---

## Audit CHECKLIST.md (2026-09-27) — SUDAH DIPERBAIKI

**Status: selesai.** `docs/CHECKLIST.md` sekarang **68 PASS / 4 dikerjakan /
16 belum / 1 ditunda** dari 89 story M0–M4, dihitung ulang lawan kode
(sebelumnya 13 PASS). Sidecar `tools/checklist_status.json` naik dari 17 → 98
entri, dan tiap verdict diverifikasi: file layar ada + komponennya terpasang di
router atau dirender induknya.

Regenerate: `python tools/update_checklist.py` (**jangan** edit CHECKLIST.md
tangan — ubah sidecar-nya).

### Temuan Fase 2 (tab Logs)

- **`finishRunStep` menerima `payload` di body lalu membuangnya.** Handler cuma
  membangun `board.Step{Status, CostMicros}`. Payload hanya tersimpan saat step
  dibuka (`StartStep`). Mengirimnya lewat `PATCH .../steps/{seq}` adalah no-op
  yang tidak berbunyi. Sumber: `cmd/api/runs.go`.
- **`steps` cuma punya satu kolom `payload_json`** (migrasi `0013`), padahal
  US-AD94 AC2 minta payload **masuk dan keluar** ditampilkan sebagai dua blok.
  Memperbaikinya = kolom baru + perubahan handler (kontrak). Panel sekarang
  menampilkan yang satu ada.
- **US-AD94 AC1 minta cache read/write per step, `steps` tidak punya kolomnya.**
  Ada di `ledger_entries` (`CacheReadTokens`, `CacheWriteTokens`). Merender 0
  berarti mengarang angka; panel menampilkan token dan biaya yang step laporkan.
  Cache ditangani di fase Ledger explorer.
- **US-AD94 AC5 (payload mentah disamarkan untuk `viewer`) belum dikerjakan.**
  App belum punya permukaan penyamaran per peran di mana pun; gerbang peran hidup
  di endpoint. Mengklaimnya di UI = aturan yang tidak ditegakkan di mana pun.
- **Definisi "PASS" di CHECKLIST**: file layar ada DAN komponennya dirender.
  **Bukan** design match. Untuk layout ketat, ukur elemennya (`DESIGN-INVENTORY.md`).

### Vite tidak menginvalidasi file yang DIUBAH di bawah bind mount Windows

Temuan yang membatalkan satu putaran bukti. Container `agentdeck-web` me-mount
`frontend/src` sebagai bind mount, dan **bind mount-nya jalan** (container
langsung melihat tulisan dari host). Yang tidak jalan adalah **file watching**-nya:
event inotify tidak menyeberang dari filesystem Windows ke container Linux, jadi
Vite menyajikan hasil transform yang sudah basi.

Bentuk salahnya menipu:

- Mengubah file -> Vite **tetap menyajikan versi lama** (byte-identik, menunggu
  15 detik pun tidak berubah).
- File **baru** tetap terbaca (request pertama 404 -> Vite membaca ulang), jadi
  e2e yang menambah berkas baru tetap sah.
- **Mutation testing jadi bohong.** Mutan yang tidak pernah dirender = app jalan
  seperti kode asli = tes lolos = dilaporkan SURVIVED. Di Fase 6, tiga dari empat
  mutan dilaporkan SURVIVED padahal semuanya CAUGHT setelah `docker compose
  restart web`.

Arah amannya satu arah: cache basi membuat kode tampak **lebih benar** daripada
dirinya, jadi ia bisa memproduksi SURVIVED palsu tapi **tidak bisa** memproduksi
CAUGHT palsu. Hasil "N CAUGHT" tetap sah; hanya SURVIVED yang wajib dicurigai.

**Aturan:** setiap mutasi wajib `docker compose restart web` sebelum tes, dan
verifikasi mutan benar-benar tersaji (bukan sekadar tersimpan) sebelum
menyimpulkan apa pun. Fase 5 diulang penuh dengan cara ini: 3 CAUGHT / 0 SURVIVED.
Guard `crypto.subtle` di Fase 1b diuji ulang dengan cara ini dan **tetap**
SURVIVED -- kali ini karena sebabnya benar (Chromium selalu punya WebCrypto).

### Notifikasi: design meminta dua hal yang API tidak punya

- **Filter "Sukses".** `notifications_kind_chk` (migrasi 0018) hanya mengizinkan
  empat kind: `approval.requested`, `budget.warning`, `run.failed`,
  `credential.invalid`. Tidak ada yang berarti sukses. Chip "Sukses" akan selalu
  kosong, jadi filter yang dibangun adalah empat kind yang benar-benar bisa
  disimpan.
- **Tombol dismiss per baris.** Tidak ada endpoint hapus; `POST
  /notifications/read` satu-satunya penulisan. "Tandai terbaca" nyata, "close"
  tidak.

### Webhooks: design meminta tiga hal yang API tidak punya (Fase 7)

Layar `/settings/webhooks` sebelumnya **stub yang salah** — menulis "endpoint
webhook belum diimplementasikan" padahal tujuh endpoint hidup di
`cmd/api/webhooks.go` sejak F12. Sekarang stub itu diganti layar nyata.

Tiga hal di mockup `40-webhooks.html` yang **sengaja tidak dibangun**, karena
tidak bisa direpresentasikan tanpa mengarang:

1. **Kolom `SECRET` per baris** (`whsec_••••9a1f`). `webhookResponse`
   (`cmd/api/webhooks.go:34-42`) tidak punya field secret, dan tidak ada endpoint
   reveal. Secret hanya masuk sekali saat create. Menampilkan nilai bertopeng =
   mengarang. Yang ditegakkan tes: `whsec_` **tidak boleh** muncul di DOM.
2. **Kolom `LAST DELIVERY` per baris.** `webhookResponse` tidak membawa info
   delivery. Dilipat ke panel delivery (satu `GET /webhooks/{id}/deliveries` saat
   baris dibuka), bukan N request demi satu kolom tabel.
3. **Opsi scope "Semua Board (Global)".** `webhooks.board_id` NOT NULL + FK, jadi
   tidak ada webhook lintas-board untuk dipilih. Pemilihnya dua Combobox
   (Proyek → Board), bukan satu daftar board sedunia.

Tambahan yang tidak diklaim: mockup menulis "Exponential Backoff (3x)" sebagai
protokol pengiriman. Ambang retry milik worker, bukan kontrak API — UI menampilkan
`attempts` + `status` apa adanya, bukan janji yang tidak bisa diverifikasi layar.

### US-AD19: graf dependency tidak punya endpoint per-board

`GET /tasks/{id}/links` dan `/tasks/{id}/dag` hanya per task. Graf per board lewat
jalur itu = N+1 request, dan badge jumlah dependency di tiap kartu butuh angka per
task dari satu panggilan. Endpoint `GET /api/v1/boards/{id}/dependencies` (Viewer)
ditambahkan di Fase 10 — aditif, tidak mengubah bentuk yang sudah ada.

Catatan kontrak yang perlu diingat kalau menyentuh jalur ini:

- Scope-nya **child**, bukan dua ujungnya: edge yang parent-nya di board lain tetap
  dilaporkan. Dispatcher tetap menolak promote task itu, dan menyembunyikan sebabnya
  bikin board kelihatan macet tanpa alasan.
- `TaskLink` (internal/board/types.go) tidak punya json tag dan hanya membawa
  `ParentID`/`ChildID`. Query `ListTaskParents`/`ListTaskChildren` sebenarnya sudah
  JOIN `tasks` untuk judul+status, tapi pgx membuangnya. Tipe baru `BoardDependency`
  membawa keduanya, karena graf harus melabeli dan mewarnai edge.
- `block_kind` **tidak bisa di-set lewat API**. Nilainya hanya ditulis dispatcher
  (`applyOutcome`/`retryOrFail`): `budget_exceeded` → `budget`, atau `failure_kind`
  yang memblokir → kind-nya. Untuk e2e, satu-satunya jalur jujur adalah run gagal
  dengan `failure_kind=needs_input` lewat `POST /runs/{id}/end`.
- Kolom board tidak 1:1 dengan status. `columnForStatus` (frontend) /
  `ColumnKey()` (backend) memetakan `blocked`→backlog, `failed`/`cancelled`→done,
  `awaiting_approval`→running. Membandingkan `task.status === column.key` menjatuhkan
  task-task itu dari layar.

### Full e2e: dua flake lingkungan, bukan regresi

Dua-duanya gejalanya sama — `locator` timeout 5s di tes **pertama** sebuah spec,
seolah board belum ter-render — dan dua-duanya **hijau saat shard-nya dijalankan
sendiri**. Terukur: `--shard=3/4` 47/47 dan `--shard=1/4` 42/42 saat dijalankan
terpisah. Nol file berubah di jendela run, jadi run hijau itu sah.

1. **Vite module graph basi.** Vite di bind mount Windows tidak menginvalidasi
   file yang DIUBAH, jadi app bisa gagal boot → halaman blank. Ini yang bikin
   `column-editor` gagal beruntun (2-3 tes per run) sampai `docker compose
   restart web` ditambahkan ke script full e2e. Sesudah itu `column-editor` hijau
   penuh.
2. **Cold start transform Vite.** Restart web mengosongkan transform cache, jadi
   permintaan pertama membayar kompilasi seluruh graph. Kalau itu jatuh ke tes
   pertama sebuah shard, tes itu kalah balapan dengan timeout 5s. Ditambahkan
   langkah warmup (`e2e/dashboard.spec.ts`) sebelum shard berjalan; sesudah itu
   shard2 dan shard3 hijau penuh.

Sisa satu kegagalan berpindah spec antar run (`agents.spec.ts` sekali,
`board-live.spec.ts` sekali) dan tidak pernah terulang. Itu sisa flake cold-start
yang belum tertutup, bukan regresi kode: tidak ada yang berubah di antara run
yang gagal dan run yang lulus.

**Docker Desktop bisa mati di tengah run.** Satu full e2e menghasilkan 0 passed /
144 failed dengan `connect ECONNREFUSED 127.0.0.1:8080` di semua shard — daemon
mati, bukan kode. `docker compose up -d` diblokir heuristik; jalur yang bekerja:
`docker start agentdeck-db agentdeck-minio` lalu `docker start agentdeck-api
agentdeck-web`.

### US-AD76: dua dari empat metrik AC1 tidak punya sumber org-wide

Diverifikasi dengan probe ke API yang jalan, bukan dari membaca kode:

- **Task per status.** `GET /search/tasks` menuntut `q` non-kosong — `?q=` dan
  `?limit=100` saja sama-sama menjawab `400 invalid input`. Satu-satunya daftar
  task adalah `GET /boards/{id}/tasks`, per board.
- **Agent aktif.** `GET /search/runs` tidak punya filter `status`, dan
  `runs.outcome` NULL sepanjang umur run yang hidup: `runs_outcome_chk`
  (`0008.up.sql:88`) tidak mengizinkan `'running'` karena `outcome` ditulis
  `EndRun`. `?outcome=running` menjawab `{"runs":[]}` selamanya.
- **Grafik biaya 7 hari.** `GET /orgs/{id}/cost-summary` adalah total 30 hari
  menggelinding; data harian hanya ada di `GET /boards/{id}/budget`.

Layar mengisi empat kartu dari yang ada (run gagal 24 jam, biaya 30 hari,
approval tertunda, proyek) dan menyebut yang tidak ada di layar. Menambah
endpoint adalah perubahan kontrak, jadi tidak dikerjakan overnight.

Catatan: `GET /search/runs` juga menolak request **tanpa filter** (400); `limit`
sendirian tidak cukup.

### Satu gate per task, dan hanya selama run-nya hidup

`RequestApproval` (`internal/board/approval.go:54`) memarkir task di
`awaiting_approval` lalu **menutup run**-nya — sengaja, supaya run yang tidak
dieksekusi siapa pun tidak di-reclaim sebagai basi lalu di-retry (membelanjakan
uang untuk kerja yang belum disetujui). Akibatnya `task.CurrentRunID` jadi kosong
dan gate **kedua** ke task yang sama dijawab 400 `invalid input` sampai task itu
di-claim lagi. Ini perilaku kontrak, bukan bug; tapi tidak kelihatan dari PRD, dan
`POST /tasks/{id}/approvals` menuntut `preview_json` sebagai `json.RawMessage`
(objek JSON, bukan string berisi JSON).

### `ledger_entries` tidak punya jalur tulis di API

Tidak ada endpoint yang menulis ledger — dispatcher yang menuliskannya saat run
berjalan. Satu-satunya cara mengisi panel biaya dengan baris nyata di e2e adalah
`psql` ke container DB, pola yang sama dengan `notifications.spec.ts`. Perlu
diingat kalau menyemai: `id` adalah **BIGSERIAL** (bukan ULID), `run_id` dan
`task_id` **NOT NULL** (butuh run nyata), dan `price_version` **NOT NULL**.

### US-AD06 AC2 tidak ada di kode: nol limit key aktif

AC2 minta `409` saat batas maksimum key aktif per org tercapai. Yang ada:
`CreateAPIKey` (`internal/auth/apikey.go`) hanya memvalidasi nama 1–64 karakter
setelah trim, lalu membuat key. Tidak ada hitungan, tidak ada konstanta batas,
tidak ada cabang 409 di `writeAPIKeyError`. Jadi AC ini tidak bisa diklaim dari
UI — dan tidak dibuatkan batas di klien, karena batas yang hanya ada di klien
bukan batas.

### API key: tiga kolom design tidak punya data

`design/stitch-output/v2/39-api-keys.html` menggambar tabel dengan HASH
(`sha256:7f4d...31e2`), ROLE / SCOPE (`admin:write`), dan BIAYA HARI INI
(`$3.140`). Tidak satu pun bisa dibangun dari kontrak yang ada:

- `apiKeyResponse` (`cmd/api/api_keys.go`) hanya memuat id, name, prefix,
  last_used_at, revoked_at, created_at. Hash **sengaja** tidak dikembalikan.
- Key tidak punya scope per-key.
- `ledger_entries` (migrasi 0008) tidak punya kolom yang menunjuk API key, jadi
  tidak ada jalur biaya per-key. Dicek ke migrasi dan `queries.sql`, bukan
  diasumsikan.

Layarnya dibangun tanpa tiga kolom itu. `data-status` dipakai untuk status
(struktural) supaya label yang diterjemahkan tidak jadi kontrak tes.

### API key ter-scope ke pemilik, bukan ke workspace

Design menyebut tabelnya "Daftar Kunci Akses Workspace" dan menampilkan key milik
agent lain. Yang benar: `APIKeys(ctx, orgID, userID)` memfilter ke pembuatnya, dan
`GET /api-keys/{id}` menjawab 404 untuk key milik anggota lain — bukan 403,
karena keberadaannya bukan urusan pemanggil. Ini perilaku isolasi yang benar
(US-AD07); yang salah adalah judul design-nya. Layar menyebut scope-nya eksplisit.

### US-AD41 AC2: tiga dari lima tab tidak punya endpoint per-RUN

AC2 minta tab **Steps, Logs, Approvals, Artifacts, Ledger** di layar run. Yang
dibangun: **Steps** dan **Ledger** — keduanya benar-benar per-run
(`GET /runs/{id}/steps`, `GET /runs/{id}/ledger`).

Tiga sisanya tidak punya sumber per-run:

- **Approvals** dan **Artifacts** dilist per TASK (`GET /tasks/{id}/approvals`,
  `GET /tasks/{id}/artifacts`). Keduanya **punya** kolom pembeda `run_id`, jadi
  versi jujurnya menyaring di klien — itu pekerjaan fase yang memiliki Ledger
  explorer, bukan alasan untuk menampilkan keputusan/artefak task sebagai milik
  run ini. Untuk task yang di-retry, itu menampilkan hasil run LAIN.
- **Logs** (event) tabel `events` **tidak punya route sama sekali**; jalur yang
  ada hanya stream SSE per board, dan itu tidak bisa diputar ulang untuk satu
  run yang sudah selesai.

### `payload` vs `payload_json`: request dan respons memakai nama berbeda

`stepRequest` (`cmd/api/runs.go:473-481`) membaca field **`payload`**, sedangkan
respons (`toStepResponse`) mengembalikan **`payload_json`**. Mengirim
`payload_json` dijawab **201 dengan payload null** — sukses, tapi tanpa data.
Bentuk gagal yang paling mahal: tidak ada error untuk ditemukan, dan payload-nya
baru terlihat kosong jauh di layar lain.

Ini kejadian KETIGA soal payload di jalur run: (1) `finishRunStep` menerima
`payload` lalu membuangnya, (2) `steps` hanya punya satu kolom `payload_json`
padahal AC94 AC2 minta payload masuk DAN keluar, (3) nama field request berbeda
dari nama field respons. Tiga-tiganya arahnya sama: payload adalah bagian yang
paling gampang hilang tanpa suara di modul ini.

### `approvals.reason` dipakai ulang: alasan permintaan hilang saat ditolak

`DecideApproval` (`internal/store/queries/queries.sql:1664`) menulis
`reason = COALESCE($5, reason)`. Kolom yang sama menyimpan dua fakta berbeda:
alasan **pemohon** memasang gate (diisi `RequestApproval`), lalu alasan
**penyetus** menolaknya. Karena `COALESCE` menimpanya, alasan asli permintaan
**hilang** begitu keputusan diambil.

Akibatnya di layar: setelah ditolak, tidak ada satu pun tempat untuk menampilkan
kedua alasan dengan label yang benar. Untuk sekarang panel aksi hanya
menampilkan `reason` selama gate masih terbuka, dan panel keputusan
menampilkannya setelah diputus — satu fakta, satu label, tidak ambigu.

Perbaikannya kolom terpisah (`decision_reason`) plus perubahan handler dan
mungkin kontrak, jadi keputusan produk. Ini sekamar dengan temuan Fase 2 soal
`finishRunStep` yang menerima `payload` lalu membuangnya: dua-duanya bentuk
"kolom dipakai ulang / data diterima lalu hilang".

### Endpoint per-task yang query-nya ada tapi route-nya tidak

- **`ListTaskApprovals` tidak ter-expose.** Query-nya ada di
  `internal/store/queries/queries.sql:1646` dan service-nya di
  `internal/board/approval.go:199` (`TaskApprovals`), tapi **tidak ada satu pun
  route** yang mendaftarkannya — daftar `approvals` di `cmd/api/runs.go:63-70`
  hanya punya `/approvals` (antrean org), `/{id}`, approve, reject, dan
  `POST /tasks/{id}/approvals`. Akibatnya tab Approvals di drawer menyaring
  antrean organisasi di klien. Itu benar hari ini, tapi ia membaca seluruh
  antrean untuk menampilkan satu task. Memasang route-nya juga akan membawa
  riwayat yang sudah diputus, jadi keputusan produknya: tab ini **hanya
  keputusan tertunda**, dan itu memang yang dikirim endpoint antrean.

### Konflik peran: story vs implementasi (approval)

- **US-AD34 AC3** bilang approve boleh `owner`/`admin`/**`member`**.
  **US-AD35 AC4** bilang reject hanya `owner`/`admin`. **Implementasi server
  menaruh keduanya di `admin`** (`cmd/api/runs.go:65-66`). Ini keputusan produk
  yang belum diambil, bukan bug yang bisa gw putuskan sendiri: kalau approve
  memang boleh member, route-nya harus turun ke `auth.Member`; kalau tidak,
  AC3 di PRD yang salah. Layar mengikuti server supaya tidak menampilkan tombol
  yang dijawab 403.

### Guard yang tidak bisa dijangkau tes

- `TabArtifacts.tsx` — cabang `crypto.subtle` tidak ada. Chromium selalu punya
  WebCrypto, jadi e2e tidak bisa menjangkaunya: mutan yang membuang guard itu
  **SURVIVED** (Fase 1b). Penjaganya tetap benar — browser tanpa WebCrypto
  memang tidak bisa menghitung digest yang diverifikasi server — tapi
  **belum terverifikasi**, dan itu disebut apa adanya, bukan diklaim tertutup.
  Kalau mau ditutup: test unit yang men-stub `crypto.subtle` jadi `undefined`.

### Yang masih terbuka setelah audit

**"PASS" belum berarti design match.** Verdict di atas mengukur
*ada filenya + terpasang*, bukan kesamaan dengan design. `18-kanban` dan
`20-task-drawer` sudah diketahui belum di-inventory lawan
`design/stitch-output/v2/*.html`. Layar lain belum diperiksa sama sekali.

**Dua story backend-only yang benar-benar kosong (nol kode):**
- `US-AD85` rate limit per endpoint — **nol rate limiter di seluruh repo**.
- `US-AD50` deteksi string keras di CI — **nol `.github/workflows/`**, nol cek.

**Empat story separuh jalan:**
- `US-AD19` — `18-kanban` ada, `24-dependency-view` belum ada filenya.
- `US-AD67` — `POST /agents/{id}/validate` ada, penegakan harga di create/update
  belum (nol panggilan ke `internal/pricing` dari jalur itu).
- `US-AD73` — backend PASS, AC1 nunggu executor.
- `US-AD108` — label estimasi di UI selesai, jalur `manual`/`pattern` belum
  terbukti tercatat di baris ledger.

**14 layar UI belum ada filenya:** `33-run-timeline`, `30-ledger-explorer`,
`36-approval-detail` (2 story), `32-run-detail`, `46-mobile-board`,
`13-notifications`, `43-state-loading`, `44-state-empty`, `45-state-error`,
`42-dashboard`, `16-security`, `34-step-payload`, `26b-agent-skills`.

**Pola yang ditemukan berulang — ditulis lalu tidak disambungkan:**
- `store/api/stream.ts` (158 baris + test) — SSE lengkap, **nol komponen memakainya**.
- `hooks/use-sse-cache.ts` — **nol pemanggil**.
- `settings/ApiKeys.tsx` + `settings/Webhooks.tsx` — stub 21 baris, bilang
  "not available yet", padahal backend-nya jalan (5 dan 7 endpoint).

**Diperbarui 2026-09-27 (fase 0–1):**
- ✅ `stream.ts` **sudah dipakai** — board hidup dari event stream (commit
  `d4078f1`). Sebelumnya nol pemakai.
- ✅ **Tab Artifacts** terpasang (commit fase 1), termasuk tab shell
  Timeline/Logs/Artifacts/Approvals sesuai `20-task-drawer.html`.
- ⬜ Tab **Logs** dan **Approvals** masih placeholder; assignee picker belum ada.
- ⬜ `ApiKeys.tsx` dan `Webhooks.tsx` masih stub.

**Temuan baru yang terverifikasi lawan kode:**
1. **US-AD48 AC2 (paginasi cursor) tidak ada di backend.** `ListTaskArtifacts`
   tidak punya `LIMIT`/`cursor` — daftarnya mengembalikan seluruh artifact satu
   task. UI-nya sengaja tidak mengarang paginasi (lihat `ponytail:` di
   `store/api/artifacts.ts`). Konsekuensi praktis kecil hari ini karena kuota
   artifact per task 100 MB, tapi AC-nya berbunyi 50+ item.
2. **Unduhan artifact tampil inline, bukan tersimpan.** `Release` presigned
   membawa `ResponseContentType` tapi bukan `ResponseContentDisposition`, jadi
   `text/plain` dirender di tab alih-alih diunduh. Atribut `download` HTML
   diabaikan browser karena URL storage beda origin dari app. Perbaikan = API
   change (tambah `response-content-disposition=attachment` saat menandatangani).
3. **Semua endpoint artifact balas 503 kalau `S3_*` tidak diset** — termasuk
   `GET` daftar. `artifactService()` mengembalikan nil dan `artifactContext`
   menolak semua. UI sekarang membedakan "storage belum dikonfigurasi" dari
   "belum ada artefak". **Runner e2e yang menjalankan suite artifact butuh
   `S3_*`** — lihat `docs/OVERNIGHT-BRIEF.md` §3c.

> Catatan lama di F13 yang bilang "22 dari 52 layar belum ada" itu menghitung
> **semua** milestone. Dalam cakupan CHECKLIST (M0–M4) jumlahnya 14.


## US-AD63/64/65: state layar — yang tidak bisa diklaim

- **US-AD65 AC3** (boundary mereset state aplikasi setelah error) dikerjakan
  sebagai reset saat pindah layar (`resetKey={pathname}`). Boundary **tidak
  melaporkan ke mana pun**: tidak ada endpoint client-error di API, dan
  mengarang satu berarti mengubah kontrak backend. Kalau nanti ada
  `POST /client-errors`, itu tempatnya.
- **Boundary kelas tidak menangkap error di luar render** (event handler,
  promise, `setTimeout`). Batas React, bukan bug implementasi. Layar yang gagal
  di jalur itu tetap perlu penanganannya sendiri.
- **US-AD63 AC1 minta pulse, mock `43-state-loading.html` tidak.** Diselesaikan
  dengan `pulse` opt-in: skeleton board pulse, permukaan lain tidak. Kalau
  nanti ada keputusan bahwa SEMUA skeleton harus pulse, ubah default-nya, bukan
  pemanggilnya.

## Flake `column-editor.spec.ts` — akarnya, akhirnya

Tiga fase terakhir, kegagalan `column-editor.spec.ts` dicatat sebagai "flake
lintas-shard, bukan regresi" tanpa akar yang jelas. Akarnya ketemu di Fase 11:

`openEditor` menunggu heading "Editor Kolom Board". Heading itu **juga dirender
cabang placeholder** — cabang yang dipakai `ColumnEditor` selama `board` belum
termuat (header sama, lima baris skeleton, nol input). Jadi tes lanjut mengetik
ke panel yang belum siap, dan `locator.fill` menunggu input yang tidak ada
sampai timeout 60s. Intermiten karena tergantung apakah query board selesai
sebelum baris berikutnya jalan — dan makin sering begitu DB membesar.

Diperbaiki: wait-nya sekarang menunggu `Nama kolom backlog`, input yang **absen**
di placeholder, timeout 30s. Bukti: `column-editor.spec.ts` 7/7 solo, dan shard3
50/50 saat diulang.
