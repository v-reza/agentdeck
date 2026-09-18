# AgentDeck — ARCHITECTURE

| Field | Value |
|---|---|
| Project | **AgentDeck** |
| Tagline | Orchestration board for AI agent fleets — every run priced, every action gated. |
| Version | 0.1 |
| Status | `draft` |
| Kontrak | [`DECISIONS.md`](./DECISIONS.md) — **beku**. Dokumen ini tidak boleh menyimpang dari §3 (state machine), §4 (enum), §5 (stack), §6 (skema DB), §7 (angka operasional N1 beserta parameter kuantitatif hingga N25). |
| PRD | [`00-PRD.md`](./00-PRD.md) |
| Design | [`DESIGN.md`](./DESIGN.md) |
| Diagram | [`DIAGRAMS.md`](./DIAGRAMS.md), [`diagrams/architecture.html`](./diagrams/architecture.html) |
| Owner | Reza (solo dev) |
| Target infra | ≤ $10/bulan (N13) |

Dokumen ini menjawab pertanyaan **"bagaimana sistemnya dibangun"**: prinsip, komponen, DDL final, query kritis, algoritma dispatcher, kontrak HTTP, realtime, approval gate, cost ledger, retry, keamanan, dan batas skala.

Angka operasional **tidak didefinisikan di sini**. Setiap angka ditulis dengan rujukan kode, mis. "(N19)", dan definisinya ada di `DECISIONS.md` §7. Kalau dokumen ini butuh sesuatu yang tidak ada di kontrak, itu masuk ke **§20 Usulan Perubahan Kontrak** — bukan diterapkan diam-diam.

---

## 1. Prinsip Arsitektur

Sepuluh prinsip. Masing-masing punya konsekuensi yang harus diterima, bukan cuma slogan.

### P1 — Nol framework web; routing pakai `net/http` ServeMux stdlib

Go 1.22+ `ServeMux` sudah mendukung method + wildcard path (`GET /api/v1/tasks/{id}`), jadi tidak ada alasan menambah `chi`/`gin`/`echo`.

**Konsekuensi:**
- Middleware dirangkai manual (`func(http.Handler) http.Handler`), urutannya eksplisit dan bisa dibaca dari satu fungsi `router()`.
- Tidak ada binding/validation otomatis — decoding JSON + validasi ditulis di paket `internal/api` dengan helper bersama (`decodeJSON[T]`, `validate()`), bukan disebar.
- Tidak ada auto-generated OpenAPI dari tag; spesifikasi endpoint hidup di **§6** dokumen ini dan diuji lewat contract test (§17).
- Ukuran binary tetap kecil (target N14 ≤ 30 MB) dan konsumsi RAM saat idle dijaga di bawah N15 (≤ 80 MB): tidak ada dependency yang bisa rusak saat major version naik, dan proses tetap murah di container 256 MB.

### P2 — Nol ORM; `pgx v5` langsung + `sqlc`

Query ditulis sebagai SQL di `internal/store/queries/*.sql`, `sqlc` meng-generate struct + fungsi Go yang type-safe. Tidak ada query builder runtime, tidak ada lazy loading, tidak ada N+1 yang tersembunyi.

**Konsekuensi:**
- Review PR = review SQL. Rencana eksekusi (`EXPLAIN`) bisa diperiksa langsung karena SQL-nya literal.
- Perubahan skema harus lewat migrasi + regenerate sqlc; build gagal kalau query tidak cocok dengan skema — itu fitur, bukan gangguan.
- Query dinamis (filter board + status + assignee + search) tidak bisa dipaksakan ke sqlc satu bentuk. Pola yang dipakai: satu query "wide filter" dengan `COALESCE`/`($1::text IS NULL OR ...)` untuk filter opsional yang jumlahnya tetap, dan paginasi cursor. Kalau kombinasi filter tumbuh melebihi itu, tambahkan query sqlc khusus per kombinasi — masih SQL literal.

### P3 — Nol Redis; Postgres `FOR UPDATE SKIP LOCKED` adalah queue-nya

Klaim task adalah satu transaksi pendek: `SELECT ... WHERE status='ready' ... FOR UPDATE SKIP LOCKED LIMIT n`, lalu `UPDATE ... SET status='running'`. Tidak ada broker, tidak ada visibility timeout yang harus disinkronkan dengan state aplikasi.

**Konsekuensi:**
- Lock dipegang hanya selama transaksi klaim (milidetik), **bukan** selama Run berjalan. Kepemilikan Run direpresentasikan oleh `runs.claim_lock` + `runs.claim_expires` + `runs.last_heartbeat_at`, bukan oleh row lock.
- Karena queue dan state hidup di DB yang sama, tidak ada dual-write: status task, claim run, dan event ditulis dalam **satu transaksi**.
- Throughput klaim dibatasi oleh jumlah koneksi write Postgres. Itu cukup untuk beban desain (N5) — lihat perhitungan di §19.
- Tidak ada delayed retry broker. Karena kontrak tidak menyediakan kolom jadwal (`ready_at` tidak ada), backoff dijalankan dispatcher secara in-memory: kandidat `ready` diperiksa terhadap `runs.ended_at` + delay per `failure_kind` (§10). Restart dispatcher hanya mempercepat/menunda retry beberapa detik, tidak merusak kebenaran state.

### P4 — Postgres adalah satu-satunya stateful dependency

Neon Postgres 16 menyimpan semua: tenant, task, run, event, ledger, approval, sesi, API key, antrean. R2 hanya blob (artifact), tidak pernah jadi sumber kebenaran.

**Konsekuensi:**
- Backup, PITR, dan disaster recovery = satu prosedur untuk satu sistem (§15).
- Kegagalan Postgres = kegagalan total. API mengembalikan `503` dengan envelope error yang sama, bukan degradasi parsial yang menyesatkan.
- Setiap optimasi besar berikutnya menyentuh Postgres dulu (§19), bukan menambah komponen baru.

### P5 — Satu binary: API + Dispatcher + Webhook Worker + SSE Hub

`cmd/agentdeck` menerima flag `-role=api|dispatcher|worker|all`. Untuk Fly.io (N13) dijalankan `-role=all` dalam satu proses 512 MB, tapi role dipisah secara kode (`internal/dispatcher` tidak boleh di-import oleh handler HTTP, kecuali lewat interface).

**Konsekuensi:**
- Deploy = satu image, satu healthcheck, satu rollback.
- Skala horizontal nanti = naikkan jumlah mesin dengan `-role=api` + satu mesin `-role=dispatcher`; tidak ada perubahan kode, karena semua koordinasi sudah lewat Postgres.
- Satu proses mati berarti semua peran mati; mitigasi: supervisor Fly restart + `max_machines_running=1` (§15), plus idempotensi klaim yang membuat restart aman.

### P6 — Isolasi org di level query, bukan RLS

Setiap tabel domain punya `org_id`. Setiap query di `internal/store` **wajib** menyertakan `org_id = $n` yang berasal dari konteks request (`authctx.OrgID`), bukan dari body/path yang bisa dipalsukan. Row Level Security tidak dipakai di v1.

**Konsekuensi:**
- Disiplin manual harus di-backup mesin: (a) sqlc di-generate dengan nama parameter `org_id` wajib ada di setiap query bertabel ber-`org_id`; (b) test isolasi tenant (§17) menjalankan setiap endpoint dengan token org lain dan mengharapkan `404`; (c) CI menolak query baru tanpa `org_id`.
- `404` dipakai untuk resource org lain (bukan `403`) supaya keberadaan ID tidak bocor.
- Superuser DB (koneksi migrasi) punya akses penuh; koneksi runtime memakai role `agentdeck_app` yang tidak punya `BYPASSRLS` — disiapkan untuk migrasi ke RLS kalau nanti dibutuhkan (§20, usulan opsional).

### P7 — Event log append-only

Tabel `events` tidak pernah `UPDATE`/`DELETE` selama masa retensi (N10). Setiap perubahan status, klaim, langkah, approval, artifact, dan entri ledger memancarkan satu baris `events`. UI, replay, webhook, dan audit membaca dari sana.

**Konsekuensi:**
- Sumber kebenaran timeline = satu tabel, bukan kumpulan log aplikasi.
- Replay run (§4d, §6) = memutar ulang `events` + `steps.payload_json`, tidak butuh fitur khusus.
- Retensi = job harian `DELETE FROM events WHERE created_at < now() - interval '30 days'` (N10), dijalankan dalam batch kecil supaya tidak mengunci. Agregat harian yang dipakai dashboard disimpan di tabel rollup (§9) dan bertahan 12 bulan (N11).
- Payload dibatasi 64 KB per event (N21); kalau lebih, payload dipotong dan diarahkan ke artifact.

### P8 — Uang adalah integer micro-USD; tidak pernah float

`cost_micros BIGINT`, 1 USD = 1.000.000 (kontrak §6). Harga model disimpan sebagai tabel statis `price_version` yang dikodekan di Go (`internal/pricing`) dan **disnapshot ke setiap baris** `ledger_entries.price_version`.

**Konsekuensi:**
- Penjumlahan biaya eksak dan bisa diaudit: mengubah harga model tidak mengubah arti baris lama.
- Semua agregasi (`SUM`, `>= cap`) dikerjakan di Postgres sebagai `BIGINT`; tidak ada pembulatan floating point yang membuat budget bocor.
- Tampilan membagi 1e6 di layer UI saja (font JetBrains Mono, sesuai DESIGN.md §8).

### P9 — State machine eksplisit, tertutup, dan dijaga di satu tempat

Transisi Task hanya boleh lewat satu fungsi `store.TransitionTask(ctx, tx, taskID, from, to, reason)`. Nilai status/outcome/enum lain dijaga `CHECK` constraint di DB dengan nama constraint eksplisit (§3). Tabel transisi legal ada di §5.

**Konsekuensi:**
- Dispatcher dan API tidak pernah menulis `UPDATE tasks SET status=...` langsung; keduanya memanggil fungsi yang sama, jadi event `task.status_changed` tidak bisa terlewat.
- Nilai ilegal dari bug kode gagal di DB (constraint violation), bukan diam-diam tersimpan.
- Menambah status = mengubah `DECISIONS.md` dulu, lalu migrasi constraint. Tidak ada jalan pintas.

### P10 — Gagal sadar jenis; observability sebagai kontrak

Setiap kegagalan Run diklasifikasi ke `failure_kind` (§4) dan keputusan retry diambil dari `agents.retry_policy` + `max_attempts` (§10), bukan `retry 3x` buta. Setiap request punya `trace_id`; log `log/slog` JSON; metrik Prometheus wajib tersedia (§14).

**Konsekuensi:**
- Retry hanya untuk yang layak di-retry; `needs_input` dan `policy` langsung masuk `blocked`/`failed` dan memunculkan approval/komentar, bukan menghabiskan biaya.
- Setiap percobaan tercatat sebagai baris `runs` sendiri dengan `attempt` naik, jadi biaya retry terlihat di ledger (tidak tersembunyi).
- Alert berbasis angka kontrak: `budget.threshold_crossed` pada 80% cap (N18), stale run 15 menit (N8), latensi baca p95 (N1: ≤ 150 ms) dan tulis (N24: ≤ 300 ms).

---

## 2. Gambaran Sistem

### 2.1 Komponen

| Komponen | Peran | Teknologi | Catatan |
|---|---|---|---|
| **Go API** | Satu-satunya pintu masuk HTTP: REST + SSE | Go 1.24, `net/http` ServeMux | Stateless; semua state di Postgres |
| **Dispatcher** | Loop tick: klaim task `ready`, evaluasi dependency, cek budget, tentukan gate, spawn worker, heartbeat, reclaim, dead-letter | Goroutine di binary yang sama | Tick 2 s (N19) dengan batas klaim batch 20 task (N20) |
| **Run Executor** | Menjalankan satu Run: siklus step, panggil LLM/tool, tulis step + ledger, hasilkan artifact | `os/exec` + HTTP client | Satu goroutine per Run; batas runtime (N9) |
| **Postgres 16** | State tunggal: queue, task, run, event, ledger, approval, sesi | Neon (0.5 GB free) | `FOR UPDATE SKIP LOCKED` = queue |
| **R2** | Blob artifact (patch, report, image) | S3 API | Egress gratis; key deterministik |
| **SSE Hub** | Fan-out event ke klien board | `text/event-stream` | Backpressure berbasis buffer per koneksi |
| **Webhook Worker** | Kirim event ke URL pelanggan dengan HMAC + retry | Goroutine + tabel `webhook_deliveries` | Dead-letter setelah batas usaha |
| **React 19 + Vite** | SPA board, inbox approval, cost ledger, trace viewer, SSE live log | React Router v7 SPA + Tailwind v4 + shadcn/ui | Baca lewat API; realtime lewat `EventSource` |
| **Pricing Registry** | Tabel harga per model/provider, versi | Konstanta Go + baris `price_version` di ledger | Tanpa layanan eksternal |

Tidak ada komponen lain. Tidak ada Redis, Kafka, Kubernetes, message broker, atau cache terpisah (aturan §5 DECISIONS). Kalau ada yang merasa butuh, itu dibahas di §19/§20.

### 2.2 Diagram komponen

```
                                 ┌──────────────────────────────────────────────┐
                                 │              React 19 + Vite (Cloudflare/Vercel)             │
                                 │  board · task detail · approval inbox ·      │
                                 │  cost dashboard · run trace/replay           │
                                 └───────┬──────────────────────────┬───────────┘
                                         │ REST (RTK Query)         │ SSE (EventSource)
                                         │ Idempotency-Key          │ Last-Event-ID
                                         ▼                          ▼
   ┌──────────────────────────────────────────────────────────────────────────────────┐
   │                        Go API  (net/http ServeMux, 1 binary)                     │
   │                                                                                  │
   │  middleware:  request_id → trace_id → authn → orgctx → rbac → ratelimit → log    │
   │                                                                                  │
   │  handlers: auth · orgs · projects · boards · agents · tasks · runs · steps ·     │
   │            events · approvals · ledger · artifacts · comments · webhooks ·       │
   │            api-keys · audit · search · health/metrics                            │
   │                                                                                  │
   │  ┌───────────────┐   ┌────────────────┐   ┌──────────────────────────────────┐   │
   │  │  SSE Hub      │   │ Run Executor   │   │  Dispatcher loop (tick 2s N19)   │   │
   │  │  per-board    │◄──┤ goroutine/Run  │◄──┤  claim batch 20 (N20)            │   │
   │  │  ring buffer  │   │ step/ledger    │   │  dep-DAG · budget · gate ·       │   │
   │  └───────┬───────┘   └───────┬────────┘   │  heartbeat 60s (N7) · reclaim    │   │
   │          │                   │            │  15m (N8) · dead-letter          │   │
   │          │                   │            └───────────────┬──────────────────┘   │
   └──────────┼───────────────────┼────────────────────────────┼──────────────────────┘
              │                   │                            │
              │                   │  ┌─────────────────────────┘
              ▼                   ▼  ▼
   ┌───────────────────────────────────────────────┐        ┌───────────────────────┐
   │            Postgres 16 (Neon, satu-satunya     │        │  Cloudflare R2        │
   │            stateful dependency)                │◄──────►│  artifacts/{org}/...  │
   │                                                │  S3 API└───────────────────────┘
   │  queue:  tasks FOR UPDATE SKIP LOCKED          │  (presigned PUT/GET)      ▲
   │  state:  tasks · runs · steps · approvals      │                           │
   │  log:    events (append-only)                  │                           │
   │  uang:   ledger_entries (BIGINT micros)        │                           │
   │  auth:   sessions · api_keys · memberships     │                           │
   └───────────────┬───────────────────────────────┘                           │
                   │ LISTEN/NOTIFY agentdeck_events                            │
                   │                                        ┌──────────────────┴────┐
                   │                                        │  Webhook worker       │
                   │                                        │  HMAC + retry + DLQ   │
                   │                                        └──────────┬────────────┘
                   │                                                   │ HTTPS POST
                   │                                                   ▼
                   │                                        ┌───────────────────────┐
                   └─── semua baca/tulis lewat pgx/sqlc ───►│  endpoint pelanggan   │
                                                           └───────────────────────┘
```

### 2.3 Aliran data

**A. Aliran tulis manusia (UI → DB).**
`POST /api/v1/boards/{id}/tasks` → middleware menetapkan `org_id` dari sesi/API key → handler validasi → satu transaksi: `INSERT tasks` + `INSERT events(task.created)` → commit → SSE hub menyiarkan `task.created` ke koneksi board → webhook worker membaca `events` baru dan mengantre pengiriman.

**B. Aliran eksekusi (dispatcher → Run).**
Tick 2 s (N19) → klaim batch ≤ 20 task (N20) (N20) task `ready` di board yang `project/org`-nya aktif → evaluasi dependency DAG (`task_links`), cek cap harian board (N16) dan cap per run (N17) → tentukan `gate_mode` → transisi `ready → running`, `INSERT runs` (attempt = max+1), set `tasks.current_run_id`, `INSERT events(run.claimed)` → spawn goroutine Run Executor → executor menulis `steps` + `ledger_entries` per pemakaian token/tool, memperbarui `runs.last_heartbeat_at` tiap 60 s (N7) → selesai: `runs.outcome` diisi, transisi task ke `review`/`done`/`failed`/`blocked`/`awaiting_approval` → event ditutup (`run.finished`).

**C. Aliran gate (run berhenti untuk manusia).**
Executor menemukan aksi yang butuh izin (`gate_mode = require`) → `INSERT approvals(decision='pending', preview_json=...)`, transisi task `running → awaiting_approval`, `INSERT events(approval.requested)` → SSE + webhook mengabari approver → approver memutuskan (`POST /approvals/{id}/approve`) → transaksi idempoten: `UPDATE approvals SET decision, decided_by, decided_at WHERE decision='pending'` → kalau `approved`, task kembali ke `ready` dan Run baru dilanjutkan; kalau `rejected`/`expired` (N23), task ke `blocked` dengan `block_kind='policy'`.

**D. Aliran baca (UI).**
`GET /api/v1/boards/{id}/tasks?cursor=...&limit=...` → query dengan filter opsional + keyset pagination pada `(created_at, id)` → response envelope JSON + `next_cursor`. Semua angka biaya dikirim sebagai integer micros; UI membagi 1e6.

**E. Aliran realtime.**
SSE hub menahan satu goroutine per koneksi. Sumber event: (1) `LISTEN/NOTIFY` Postgres pada channel `agentdeck_events` — payload hanya `id` event, jadi fan-out murah; (2) kalau `NOTIFY` terlewat (reconnect, restart hub), klien melanjutkan dari `Last-Event-ID` dan hub membaca `SELECT ... FROM events WHERE id > $last ORDER BY id`. Frame detail di §7.

**F. Aliran artifact.**
Executor menulis file ke workspace → menghitung `sha256` + `size` → `PUT` ke R2 dengan key `org/{org_id}/task/{task_id}/run/{run_id}/{ulid}-{filename}` → `INSERT artifacts` + `INSERT events(artifact.created)` → UI meminta `GET /artifacts/{id}/download` dan API mengembalikan presigned URL berumur pendek (5 menit). Batas 25 MB/file dan 100 MB/task (N22) diperiksa **sebelum** upload.

---

## 3. Skema Database

Postgres 16. Nama tabel dan kolom di bawah **persis** `DECISIONS.md` §6 — tidak ada penambahan, penggantian nama, atau kolom baru. Yang ditambahkan di sini hanya **tipe, constraint, index, dan komentar**, karena kontrak tidak menentukannya.

Aturan yang mengikat seluruh DDL:

- **Uang** = `BIGINT` micro-USD, 1 USD = 1.000.000. Tidak ada `FLOAT`, `REAL`, `DOUBLE PRECISION`, atau `NUMERIC` untuk nilai uang (kontrak §6).
- **ID** = ULID `TEXT` panjang 26 untuk entitas domain; `BIGSERIAL` untuk `events`, `steps`, `ledger_entries`, `audit_log` (kontrak §6). ULID dipilih karena bisa diurutkan secara leksikografis berdasarkan waktu — penting untuk cursor pagination dan `ORDER BY id`.
- **Enum** dijaga `CHECK` constraint dengan **nama eksplisit** (`{tabel}_{kolom}_chk`) supaya pelanggaran constraint terbaca jelas di log dan gampang diuji.
- **Waktu** = `TIMESTAMPTZ` (bukan `TIMESTAMP`), supaya perbandingan cap harian (N16) tidak ambigu terhadap zona waktu.
- **Isolasi** = setiap tabel domain punya `org_id NOT NULL`; seluruh query runtime wajib memfilternya (§11).
- **JSON** = `JSONB` (bukan `JSON`) supaya bisa di-index dan dinormalisasi; payload event dibatasi 64 KB (N21) di layer aplikasi.
- Semua constraint punya nama; `ON DELETE` ditulis eksplisit, tidak mengandalkan default.

Catatan tentang constraint `CHECK` yang menguji panjang ULID: `char_length(id) = 26` bukan sekadar formalitas — ia menangkap bug yang menulis UUID (36) atau slug (variabel) ke kolom ID, yang kalau lolos akan merusak asumsi pengurutan cursor.

### 3.1 Ekstensi dan peran

```sql
-- 0001_init.sql — AgentDeck schema v0.1 (Postgres 16)
-- Dijalankan oleh internal/migrate (embed.FS + advisory lock). Tidak ada tool migrasi eksternal.

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- dipakai untuk gen_random_uuid() pada kolom teknis non-domain (tidak ada di v1)
CREATE EXTENSION IF NOT EXISTS pg_trgm;    -- dipakai GIN index pencarian judul task (search, §6)

-- Peran runtime: tidak boleh DDL, tidak boleh BYPASSRLS.
-- (Disiapkan untuk migrasi ke Row Level Security kalau nanti dibutuhkan — §20.)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'agentdeck_app') THEN
        CREATE ROLE agentdeck_app LOGIN;
    END IF;
END $$;

GRANT USAGE ON SCHEMA public TO agentdeck_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO agentdeck_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO agentdeck_app;
```

### 3.2 `orgs` — tenant

```sql
CREATE TABLE orgs (
    id          TEXT        NOT NULL,   -- ULID 26; batas isolasi seluruh data
    slug        TEXT        NOT NULL,   -- dipakai di URL dan subdomain, lowercase
    name        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT orgs_pk                 PRIMARY KEY (id),
    CONSTRAINT orgs_id_ulid_chk        CHECK (char_length(id) = 26),
    CONSTRAINT orgs_slug_chk           CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'),
    CONSTRAINT orgs_name_chk           CHECK (btrim(name) <> '')
);

CREATE UNIQUE INDEX orgs_slug_key ON orgs (lower(slug));
```

`slug` unik case-insensitive; disimpan apa adanya (lowercase dipaksa validasi API) supaya pesan error bisa menyebut input asli.

### 3.3 `users` — identitas global

```sql
CREATE TABLE users (
    id            TEXT        NOT NULL,  -- ULID 26
    email         TEXT        NOT NULL,  -- login; dibandingkan case-insensitive
    name          TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,  -- argon2id PHC string: $argon2id$v=19$m=...,t=...,p=...$salt$hash
    avatar_url    TEXT,                  -- opsional; NULL = pakai avatar inisial (US-AD89)
    deleted_at    TIMESTAMPTZ,           -- soft-delete 30 hari saat akun ditutup (US-AD98 AC5)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_pk            PRIMARY KEY (id),
    CONSTRAINT users_id_ulid_chk   CHECK (char_length(id) = 26),
    CONSTRAINT users_email_chk     CHECK (position('@' IN email) > 1),
    CONSTRAINT users_password_chk  CHECK (password_hash LIKE '$argon2id$%')
);

CREATE UNIQUE INDEX users_email_key ON users (lower(email));
```

`users` sengaja **tidak** punya `org_id`: satu manusia bisa jadi anggota beberapa Org (kontrak §2: Org = tenant, User = orang). Keanggotaan ada di `memberships`.

### 3.4 `memberships` — jembatan user↔org

```sql
CREATE TABLE memberships (
    org_id     TEXT        NOT NULL,
    user_id    TEXT        NOT NULL,
    role       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT memberships_pk          PRIMARY KEY (org_id, user_id),
    CONSTRAINT memberships_role_chk    CHECK (role IN ('owner','admin','member','viewer')),
    CONSTRAINT memberships_org_fk      FOREIGN KEY (org_id)  REFERENCES orgs(id)  ON DELETE CASCADE,
    CONSTRAINT memberships_user_fk     FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Melayani: "org apa saja milik user ini" (GET /api/v1/orgs, pemilihan org aktif di UI).
CREATE INDEX memberships_user_idx ON memberships (user_id, org_id);
```

PK komposit `(org_id, user_id)` = satu user satu role per org (kontrak §6). Tidak ada kolom `updated_at` di kontrak; perubahan role = `UPDATE` baris ini + satu baris `audit_log`.

### 3.5 `projects` — pengelompokan board di dalam Org

```sql
CREATE TABLE projects (
    id         TEXT        NOT NULL,
    org_id     TEXT        NOT NULL,
    slug       TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT projects_pk          PRIMARY KEY (id),
    CONSTRAINT projects_id_ulid_chk CHECK (char_length(id) = 26),
    CONSTRAINT projects_slug_chk    CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'),
    CONSTRAINT projects_org_fk      FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE
);

-- Melayani: GET /api/v1/projects (daftar project per org), dan slug unik per org.
CREATE UNIQUE INDEX projects_org_slug_key ON projects (org_id, slug);

-- Melayani: pencarian project by slug lintas org (tidak dipakai runtime; jaga-jaga untuk audit manual).
CREATE INDEX projects_org_created_idx ON projects (org_id, created_at DESC);
```

### 3.6 `boards` — papan kanban + cap biaya

```sql
CREATE TABLE boards (
    id                   TEXT        NOT NULL,
    org_id               TEXT        NOT NULL,
    project_id           TEXT        NOT NULL,
    slug                 TEXT        NOT NULL,
    name                 TEXT        NOT NULL,
    columns_json         JSONB       NOT NULL DEFAULT
        '[{"key":"backlog","name":"Backlog"},{"key":"ready","name":"Ready"},
          {"key":"running","name":"Running"},{"key":"review","name":"Review"},
          {"key":"done","name":"Done"}]'::jsonb,   -- kolom default (DECISIONS §3); kolom = view dari status, bukan status baru
    budget_daily_micros  BIGINT      NOT NULL DEFAULT 20000000,  -- $20/hari (N16); micro-USD, BIGINT
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT boards_pk            PRIMARY KEY (id),
    CONSTRAINT boards_id_ulid_chk   CHECK (char_length(id) = 26),
    CONSTRAINT boards_slug_chk      CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'),
    CONSTRAINT boards_budget_chk    CHECK (budget_daily_micros >= 0),
    CONSTRAINT boards_columns_chk   CHECK (jsonb_typeof(columns_json) = 'array'),
    CONSTRAINT boards_org_fk        FOREIGN KEY (org_id)     REFERENCES orgs(id)     ON DELETE CASCADE,
    CONSTRAINT boards_project_fk    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Melayani: GET /api/v1/projects/{id}/boards dan resolusi slug board di URL.
CREATE UNIQUE INDEX boards_project_slug_key ON boards (project_id, slug);

-- Melayani: budget guardrail — ambil cap semua board org ini sebelum menghitung pemakaian harian (N16: $20/hari; lihat §9).
CREATE INDEX boards_org_budget_idx ON boards (org_id, id) WHERE budget_daily_micros > 0;

-- Melayani: daftar board per project untuk UI (urut terbaru dulu).
CREATE INDEX boards_project_created_idx ON boards (project_id, created_at DESC);
```

`columns_json` default memakai lima kolom default kontrak §3. `budget_daily_micros` default = cap $20/hari (N16); kolom ini adalah satu-satunya tempat cap disimpan, dan nilainya dibaca dispatcher pada setiap evaluasi budget.

### 3.7 `agents` — profil worker

```sql
CREATE TABLE agents (
    id                   TEXT        NOT NULL,
    org_id               TEXT        NOT NULL,
    project_id           TEXT        NOT NULL,
    name                 TEXT        NOT NULL,
    provider             TEXT        NOT NULL,   -- mis. 'anthropic', 'openai', 'openrouter', 'local'
    model                TEXT        NOT NULL,   -- string model apa adanya, dipakai sebagai kunci pricing (§9)
    reasoning_effort     TEXT        NOT NULL DEFAULT 'medium',  -- passthrough ke provider; bukan enum kontrak §4 → tidak di-CHECK
    skills_json          JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- daftar skill yang boleh dimuat agent
    tools_json           JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- allowlist tool; tool di luar ini = failure_kind 'capability'
    max_runtime_seconds  INTEGER     NOT NULL DEFAULT 14400,        -- 4 jam (N9)
    retry_policy         TEXT        NOT NULL DEFAULT 'transient_only',
    max_attempts         INTEGER     NOT NULL DEFAULT 3,
    provider_api_key_enc BYTEA,                  -- AES-256-GCM encrypted (nonce 12B + ciphertext + tag 16B); NULL jika pakai env default (§16)
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agents_pk               PRIMARY KEY (id),
    CONSTRAINT agents_id_ulid_chk      CHECK (char_length(id) = 26),
    CONSTRAINT agents_name_chk         CHECK (btrim(name) <> ''),
    CONSTRAINT agents_runtime_chk      CHECK (max_runtime_seconds BETWEEN 1 AND 86400),
    CONSTRAINT agents_retry_policy_chk CHECK (retry_policy IN ('never','transient_only','always')),
    CONSTRAINT agents_max_attempts_chk CHECK (max_attempts BETWEEN 1 AND 10),
    CONSTRAINT agents_skills_chk       CHECK (jsonb_typeof(skills_json) = 'array'),
    CONSTRAINT agents_tools_chk        CHECK (jsonb_typeof(tools_json)  = 'array'),
    CONSTRAINT agents_org_fk           FOREIGN KEY (org_id)     REFERENCES orgs(id)     ON DELETE CASCADE,
    CONSTRAINT agents_project_fk       FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Melayani: GET /api/v1/projects/{id}/agents, dan lookup agent per nama di UI.
CREATE UNIQUE INDEX agents_project_name_key ON agents (project_id, name);

-- Melayani: dropdown pemilihan assignee pada form task (hanya kolom ringan, tidak menarik tools_json).
CREATE INDEX agents_org_name_idx ON agents (org_id, name);
```

`retry_policy` (kontrak §4) dan `max_attempts` di sini adalah **sumber keputusan retry** untuk setiap Run yang dijalankan agent tersebut (§10). `reasoning_effort` tidak diberi `CHECK` karena nilainya milik provider masing-masing (Anthropic memakai `low/medium/high`, provider lain beda) dan tidak terdaftar di kontrak §4.

---

### 3.8 `tasks` — unit kerja inti

```sql
CREATE TABLE tasks (
    id                      TEXT        NOT NULL,
    org_id                  TEXT        NOT NULL,
    board_id                TEXT        NOT NULL,
    title                   TEXT        NOT NULL,
    body                    TEXT        NOT NULL DEFAULT '',
    status                  TEXT        NOT NULL DEFAULT 'backlog',
    priority                SMALLINT    NOT NULL DEFAULT 0,       -- lebih tinggi = lebih prioritas, sema-mata untuk sortir
    assignee_agent_id       TEXT,                                 -- NULL = belum diassign/agent dihapus
    created_by              TEXT        NOT NULL,                 -- user_id yang membuat
    idempotency_key         TEXT,                                 -- opsional; unik global selama 24 jam
    block_kind              TEXT,                                 -- hanya relevan kalau status='blocked'
    consecutive_failures    SMALLINT    NOT NULL DEFAULT 0,
    workspace_kind          TEXT        NOT NULL DEFAULT 'scratch',
    workspace_path          TEXT,                                 -- diisi executor saat Run dimulai
    branch_name             TEXT,                                 -- untuk git worktree kind
    completion_contract     TEXT,                                 -- JSON bebas: panduan untuk agent reviewer
    goal_mode               TEXT        NOT NULL DEFAULT 'auto',  -- 'auto'|'manual'; bukan enum kontrak → tidak di-CHECK
    goal_max_turns          INTEGER     NOT NULL DEFAULT 25,      -- guardrail: maks langkah eksekusi
    current_run_id          TEXT,                                 -- ULID run yang sedang memegang (kalau status='running')
    cost_micros             BIGINT      NOT NULL DEFAULT 0,       -- agregasi all runs task ini, untuk ditampilkan
    tokens_in               BIGINT      NOT NULL DEFAULT 0,
    tokens_out              BIGINT      NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at              TIMESTAMPTZ,                          -- first run.claimed → running
    completed_at            TIMESTAMPTZ,                          -- task masuk terminal (done/failed/cancelled)
    archived_at             TIMESTAMPTZ,                          -- status='archived'
    CONSTRAINT tasks_pk                      PRIMARY KEY (id),
    CONSTRAINT tasks_id_ulid_chk             CHECK (char_length(id) = 26),
    CONSTRAINT tasks_status_chk              CHECK (status IN (
        'backlog','ready','running','awaiting_approval','blocked','review','done','failed','cancelled','archived'
    )),
    CONSTRAINT tasks_block_kind_chk          CHECK (block_kind IN (
        'dependency','needs_input','capability','policy','budget','external'
    )),
    CONSTRAINT tasks_workspace_kind_chk      CHECK (workspace_kind IN ('scratch','dir','worktree','container')),
    CONSTRAINT tasks_goal_mode_chk           CHECK (goal_mode IN ('auto','manual')),
    CONSTRAINT tasks_cost_nonneg_chk         CHECK (cost_micros >= 0),
    CONSTRAINT tasks_tokens_chk              CHECK (tokens_in >= 0 AND tokens_out >= 0),
    CONSTRAINT tasks_priority_chk            CHECK (priority BETWEEN -10 AND 10),
    CONSTRAINT tasks_consecutive_fail_chk    CHECK (consecutive_failures BETWEEN 0 AND 10),
    CONSTRAINT tasks_board_fk                FOREIGN KEY (board_id)      REFERENCES boards(id) ON DELETE CASCADE,
    CONSTRAINT tasks_agent_fk                FOREIGN KEY (assignee_agent_id) REFERENCES agents(id) ON DELETE SET NULL,
    CONSTRAINT tasks_idempotency_uniq        UNIQUE (idempotency_key)    -- opsional; di-set NULL secara eksplisit untuk yang tidak pakai
);

-- Melayani: query kolom board — daftar task yang visible, diurutkan priority + created.
CREATE INDEX tasks_board_status_idx ON tasks (board_id, status, priority DESC, created_at DESC)
    WHERE status != 'archived';

-- Melayani: dispatcher — claim task status='ready' untuk board yang tidak di-archive (N19: tick 2 s, N20: batch 20 task).
CREATE INDEX tasks_ready_claim_idx ON tasks (board_id, priority DESC, created_at DESC)
    WHERE status = 'ready' AND current_run_id IS NULL;

-- Melayani: query "blocked by dependency" — cari task yang blocked oleh task tertentu (task_links, §4d).
CREATE INDEX tasks_blocked_status_idx ON tasks (id) WHERE status = 'blocked';

-- Melayani: budget guardrail — agregasi biaya per task yang pernah running di board tertentu.
CREATE INDEX tasks_board_cost_idx ON tasks (board_id) WHERE cost_micros > 0;

-- Melayani: search full-text (title + body) dengan pg_trgm untuk fuzzy match.
-- Filter wajib org_id dipasang di query aplikasi; partial index ini hanya untuk akselerasi.
CREATE INDEX tasks_title_trgm_idx ON tasks USING gin (title gin_trgm_ops);
CREATE INDEX tasks_body_trgm_idx ON tasks USING gin (body gin_trgm_ops);
```

Penjelasan index:

| Index | Query yang dilayani | Skema |
|---|---|---|
| `tasks_board_status_idx` | `SELECT ... FROM tasks WHERE board_id = $1 AND status != 'archived' ORDER BY priority DESC, created_at DESC` — papan board default |
| `tasks_ready_claim_idx` | `SELECT ... FROM tasks WHERE status = 'ready' AND current_run_id IS NULL AND board_id = $1 ORDER BY priority DESC, created_at DESC LIMIT $2 FOR UPDATE SKIP LOCKED` — klaim dispatcher |
| `tasks_blocked_status_idx` | `SELECT COUNT(*) FROM tasks WHERE status = 'blocked' AND id = ANY($1)` — setelah write task_links, dispatcher mengecek apakah induk ada yang masih diblokir |
| `tasks_board_cost_idx` | `SELECT SUM(cost_micros) FROM tasks WHERE board_id = $1` — ringkasan dashboard board |

### 3.9 `task_links` — DAG dependency

```sql
CREATE TABLE task_links (
    parent_id   TEXT        NOT NULL,
    child_id    TEXT        NOT NULL,
    CONSTRAINT task_links_pk         PRIMARY KEY (parent_id, child_id),
    CONSTRAINT task_links_parent_fk  FOREIGN KEY (parent_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT task_links_child_fk   FOREIGN KEY (child_id)  REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT task_links_no_self    CHECK (parent_id <> child_id)
);

-- Melayani: "apa saja dependensi task ini (ancestors)" — dispatcher evaluasi ready → check child dulu.
CREATE INDEX task_links_child_idx ON task_links (child_id, parent_id);

-- Melayani: "apa saja yang tergantung task ini (descendants)" — waktu task selesai, bangunkan anak.
CREATE INDEX task_links_parent_idx ON task_links (parent_id, child_id);
```

### 3.10 `runs` — satu eksekusi task

```sql
CREATE TABLE runs (
    id                   TEXT        NOT NULL,
    org_id               TEXT        NOT NULL,
    task_id              TEXT        NOT NULL,
    agent_id             TEXT        NOT NULL,
    attempt              SMALLINT    NOT NULL DEFAULT 1,
    status               TEXT        NOT NULL DEFAULT 'pending',   -- pending/claiming/running/ended
    outcome              TEXT,                                     -- hanya diisi saat ended; NULL = belum selesai
    failure_kind         TEXT,
    claim_lock           TEXT,                                     -- hostname + PID, untuk debugging reclaim saja, bukan kebenaran
    claim_expires        TIMESTAMPTZ,                              -- initial = now() + 2s (cukup untuk mutex transaksional)
    worker_pid           INTEGER,                                  -- PID dari goroutine executor (untuk debug; tidak dipakai logika)
    last_heartbeat_at    TIMESTAMPTZ,
    max_runtime_seconds  INTEGER     NOT NULL DEFAULT 14400,       -- disalin dari agents.max_runtime_seconds saat klaim
    cost_micros          BIGINT      NOT NULL DEFAULT 0,
    tokens_in            BIGINT      NOT NULL DEFAULT 0,
    tokens_out           BIGINT      NOT NULL DEFAULT 0,
    summary              TEXT,                                     -- ringkasan hasil Run, diisi executor sebelum selesai
    error                TEXT,                                     -- error message terakhir, dipotong 1 KB
    metadata_json        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at             TIMESTAMPTZ,
    CONSTRAINT runs_pk               PRIMARY KEY (id),
    CONSTRAINT runs_id_ulid_chk      CHECK (char_length(id) = 26),
    CONSTRAINT runs_status_chk       CHECK (status IN ('pending','claiming','running','ended')),
    CONSTRAINT runs_outcome_chk      CHECK (outcome IN ('succeeded','failed','timed_out','cancelled','reclaimed','budget_exceeded')),
    CONSTRAINT runs_failure_kind_chk CHECK (failure_kind IN (
        'transient','needs_input','capability','dependency','policy','budget','unknown'
    )),
    CONSTRAINT runs_cost_nonneg_chk  CHECK (cost_micros >= 0),
    CONSTRAINT runs_tokens_chk       CHECK (tokens_in >= 0 AND tokens_out >= 0),
    CONSTRAINT runs_attempt_chk      CHECK (attempt BETWEEN 1 AND 10),
    CONSTRAINT runs_task_fk          FOREIGN KEY (task_id)  REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT runs_agent_fk         FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);

-- Melayani: dispatcher reclaim — cari run yang status='running' dan heartbeat > 15 mnt lalu (N8).
CREATE INDEX runs_stale_claim_idx ON runs (status, last_heartbeat_at)
    WHERE status = 'running' AND claim_expires IS NOT NULL;

-- Melayani: daftar runs per task (run trace).
CREATE INDEX runs_task_idx ON runs (task_id, attempt DESC);

-- Melayani: agregasi biaya per board (lewat join task→board) untuk budget guardrail board (N16: $20/hari; detail di §4c dan §9).
CREATE INDEX runs_status_cost_idx ON runs (status) WHERE status = 'ended';
```

### 3.11 `steps` — span aktivitas di dalam Run

```sql
CREATE TABLE steps (
    id          BIGSERIAL   NOT NULL,
    org_id      TEXT        NOT NULL,
    run_id      TEXT        NOT NULL,
    seq         SMALLINT    NOT NULL,            -- urutan langkah; mulai dari 1 untuk tiap Run
    kind        TEXT        NOT NULL,             -- 'llm' / 'tool' / 'shell' / 'think' / 'response'
    name        TEXT        NOT NULL DEFAULT '',  -- nama tool/LLM call, untuk trace
    status      TEXT        NOT NULL DEFAULT 'running',
    tokens_in   BIGINT      NOT NULL DEFAULT 0,
    tokens_out  BIGINT      NOT NULL DEFAULT 0,
    cost_micros BIGINT      NOT NULL DEFAULT 0,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at    TIMESTAMPTZ,
    payload_json JSONB,                           -- request/response (dipotong per step; total step payload = 1 MB max per Run)
    CONSTRAINT steps_pk          PRIMARY KEY (id),
    CONSTRAINT steps_status_chk  CHECK (status IN ('running','succeeded','failed')),
    CONSTRAINT steps_cost_chk    CHECK (cost_micros >= 0),
    CONSTRAINT steps_tokens_chk  CHECK (tokens_in >= 0 AND tokens_out >= 0),
    CONSTRAINT steps_seq_chk     CHECK (seq BETWEEN 1 AND 5000),   -- 5000 langkah maks per Run, guardrail
    CONSTRAINT steps_run_fk      FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

-- Melayani: timeline Run trace — SELECT ... FROM steps WHERE run_id = $1 ORDER BY seq.
CREATE INDEX steps_run_seq_idx ON steps (run_id, seq);

-- Melayani: agregasi biaya per hari (join ke run → task → board) untuk budget (§9).
CREATE INDEX steps_started_idx ON steps (org_id, started_at) WHERE status = 'succeeded';
```

### 3.12 `events` — append-only event log

```sql
CREATE TABLE events (
    id          BIGSERIAL   NOT NULL,    -- BIGSERIAL untuk urutan global; cursor pagination
    org_id      TEXT        NOT NULL,
    board_id    TEXT,                     -- opsional; NULL kalau event bukan bagian board (mis. user.created)
    task_id     TEXT,
    run_id      TEXT,
    kind        TEXT        NOT NULL,
    payload_json JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT events_pk            PRIMARY KEY (id),
    CONSTRAINT events_kind_chk      CHECK (kind IN (
        'task.created','task.status_changed','task.assigned',
        'run.claimed','run.heartbeat','run.finished','run.reclaimed',
        'step.started','step.finished','step.failed',
        'approval.requested','approval.decided','approval.expired',
        'artifact.created','ledger.entry','comment.created','budget.threshold_crossed'
    )),
    CONSTRAINT events_payload_chk   CHECK (pg_column_size(payload_json) <= 65536)  -- N21: 64 KB max
);

-- Melayani: timeline task — SELECT ... FROM events WHERE task_id = $1 ORDER BY id.
CREATE INDEX events_task_idx ON events (task_id, id);

-- Melayani: timeline run — replay (4d).
CREATE INDEX events_run_idx ON events (run_id, id);

-- Melayani: timeline board (SSE resume) — SELECT ... FROM events WHERE board_id = $1 AND id > $2 ORDER BY id.
CREATE INDEX events_board_id_idx ON events (board_id, org_id, id);

-- Melayani: retensi hot events (N10) — DELETE ... WHERE created_at < now() - interval '30 days'.
-- Partial: hanya event yang sudah cukup tua yang di-index, supaya index tidak terus membesar.
-- (Evaluasi di v1 murni seq scan DELETE kecil — di-deploy kalau perlu.)
-- CREATE INDEX events_retention_idx ON events (created_at) WHERE created_at < now() - interval '25 days';
```

Index `events_board_id_idx` adalah index paling kritis untuk SSE resume: membaca event baru sejak `Last-Event-ID` untuk satu board tertentu.

### 3.13 `approvals` — gate manusia

```sql
CREATE TABLE approvals (
    id             TEXT        NOT NULL,
    org_id         TEXT        NOT NULL,
    task_id        TEXT        NOT NULL,
    run_id         TEXT        NOT NULL,
    requested_by   TEXT        NOT NULL,  -- user_id yang request (bisa agent_id? — kontrak: TEXT, dibaca sebagai user_id)
    decided_by     TEXT,                  -- user_id yang memutuskan
    decision       TEXT        NOT NULL DEFAULT 'pending',
    gate_mode      TEXT        NOT NULL,  -- disalin dari policy saat gate diciptakan
    reason         TEXT,                  -- alasan dari approver (kalau ditolak) atau system (kalau expired)
    preview_json   JSONB,                -- snapshot preview apa yang akan dilakukan (patch/komando)
    expires_at     TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',  -- N23
    decided_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT approvals_pk              PRIMARY KEY (id),
    CONSTRAINT approvals_id_ulid_chk     CHECK (char_length(id) = 26),
    CONSTRAINT approvals_decision_chk    CHECK (decision IN ('pending','approved','rejected','expired')),
    CONSTRAINT approvals_gate_mode_chk   CHECK (gate_mode IN ('auto','require','deny')), -- approval_gate_mode: auto | require | deny (DECISIONS §4)
    CONSTRAINT approvals_task_fk         FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT approvals_run_fk          FOREIGN KEY (run_id)  REFERENCES runs(id)  ON DELETE CASCADE
);

-- Melayani: approval inbox — cari approval pending yang belum expired.
CREATE INDEX approvals_pending_idx ON approvals (org_id, decision, created_at DESC)
    WHERE decision IN ('pending') AND (expires_at IS NULL OR expires_at > now());

-- Melayani: approval by task (run trace).
CREATE INDEX approvals_task_idx ON approvals (task_id, created_at DESC);
```

### 3.14 `ledger_entries` — cost ledger per-step

```sql
CREATE TABLE ledger_entries (
    id                BIGSERIAL   NOT NULL,
    org_id            TEXT        NOT NULL,
    run_id            TEXT        NOT NULL,
    task_id           TEXT        NOT NULL,
    provider          TEXT        NOT NULL,   -- 'anthropic', 'openai', 'openrouter', dll.
    model             TEXT        NOT NULL,   -- 'claude-sonnet-4-20250514', dll.
    kind              TEXT        NOT NULL DEFAULT 'llm',   -- 'llm' / 'cache_read' / 'cache_write' / 'tool'
    tokens_in         BIGINT      NOT NULL DEFAULT 0,
    tokens_out        BIGINT      NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT      NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT     NOT NULL DEFAULT 0,
    cost_micros       BIGINT      NOT NULL, -- HASIL PERHITUNGAN harga × kuantitas, bukan harga satuan
    price_version     INTEGER     NOT NULL, -- versi snapshot harga yang dipakai untuk hitung baris ini
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ledger_entries_pk                PRIMARY KEY (id),
    CONSTRAINT ledger_entries_kind_chk          CHECK (kind IN ('llm','cache_read','cache_write','tool')),
    CONSTRAINT ledger_entries_cost_chk          CHECK (cost_micros >= 0),
    CONSTRAINT ledger_entries_tokens_chk        CHECK (tokens_in >= 0 AND tokens_out >= 0 AND cache_read_tokens >= 0 AND cache_write_tokens >= 0),
    CONSTRAINT ledger_entries_run_fk            FOREIGN KEY (run_id)  REFERENCES runs(id) ON DELETE CASCADE,
    CONSTRAINT ledger_entries_task_fk           FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

-- Melayani: agregasi biaya per board per hari (§4c) — join runs → tasks → boards, WHERE events.created_at ≥ hari ini.
CREATE INDEX ledger_entries_org_time_idx ON ledger_entries (org_id, created_at);

-- Melayani: inspeksi biaya per run.
CREATE INDEX ledger_entries_run_idx ON ledger_entries (run_id);
```
---

### 3.15 `artifacts` — file hasil eksekusi

```sql
CREATE TABLE artifacts (
    id           TEXT        NOT NULL,
    org_id       TEXT        NOT NULL,
    task_id      TEXT        NOT NULL,
    run_id       TEXT        NOT NULL,
    filename     TEXT        NOT NULL,
    content_type TEXT        NOT NULL DEFAULT 'application/octet-stream',
    size         INTEGER     NOT NULL,           -- bytes; max 25 MB per file, 100 MB per task (N22) — diperiksa aplikasi
    storage_key  TEXT        NOT NULL,            -- key di R2: `artifacts/{org_id}/{task_id}/{run_id}/{ulid}-{filename}`
    sha256       TEXT        NOT NULL,            -- hex digest, diverifikasi aplikasi sebelum INSERT
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT artifacts_pk        PRIMARY KEY (id),
    CONSTRAINT artifacts_id_ulid_chk CHECK (char_length(id) = 26),
    CONSTRAINT artifacts_size_chk  CHECK (size BETWEEN 1 AND 26214400),  -- 25 MB max per file
    CONSTRAINT artifacts_sha256_chk CHECK (char_length(sha256) = 64),     -- SHA-256 hex
    CONSTRAINT artifacts_task_fk   FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT artifacts_run_fk    FOREIGN KEY (run_id)  REFERENCES runs(id)  ON DELETE CASCADE
);

-- Melayani: daftar artifact per task.
CREATE INDEX artifacts_task_idx ON artifacts (task_id, created_at DESC);

-- Melayani: cleanup retensi 90 hari (N12) — DELETE ... WHERE created_at < now() - interval '90 days'.
CREATE INDEX artifacts_retention_idx ON artifacts (created_at) WHERE created_at < now() - interval '85 days';

-- Melayani: download artifact by storage_key (presigned URL).
CREATE INDEX artifacts_storage_key_idx ON artifacts (storage_key);
```

### 3.16 `comments` — diskusi task

```sql
CREATE TABLE comments (
    id             BIGSERIAL   NOT NULL,
    org_id         TEXT        NOT NULL,
    task_id        TEXT        NOT NULL,
    author_user_id TEXT,                     -- NULL kalau komentar dari agent (via author_agent_id)
    author_agent_id TEXT,                    -- NULL kalau dari user
    body           TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT comments_pk        PRIMARY KEY (id),
    CONSTRAINT comments_body_chk  CHECK (btrim(body) <> ''),
    CONSTRAINT comments_author_chk CHECK (
        (author_user_id IS NOT NULL AND author_agent_id IS NULL)
        OR (author_user_id IS NULL AND author_agent_id IS NOT NULL)
    ),
    CONSTRAINT comments_task_fk  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT comments_org_fk   FOREIGN KEY (org_id)  REFERENCES orgs(id)  ON DELETE CASCADE
);

-- Melayani: timeline diskusi per task.
CREATE INDEX comments_task_idx ON comments (task_id, id);

-- Melayani: notifikasi — ambil komentar terbaru untuk board.
CREATE INDEX comments_org_created_idx ON comments (org_id, created_at DESC);
```

### 3.17 `audit_log` — catatan perubahan modifikasi

```sql
CREATE TABLE audit_log (
    id             BIGSERIAL   NOT NULL,
    org_id         TEXT        NOT NULL,
    actor_user_id  TEXT,                    -- NULL = aksi oleh agent
    actor_agent_id TEXT,                    -- NULL = aksi oleh user
    action         TEXT        NOT NULL,    -- 'membership.change_role', 'board.delete', 'api_key.create', dll.
    target_type    TEXT        NOT NULL,    -- 'membership', 'board', 'task', 'api_key', etc.
    target_id      TEXT        NOT NULL,    -- ULID resource
    before_json    JSONB,                   -- snapshot sebelum (opsional, terutama untuk UPDATE)
    after_json     JSONB,                   -- snapshot sesudah (opsional)
    ip             TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audit_log_pk PRIMARY KEY (id)
);

-- Melayani: audit trail per org.
CREATE INDEX audit_log_org_action_idx ON audit_log (org_id, created_at DESC);

-- Melayani: investigasi satu resource.
CREATE INDEX audit_log_target_idx ON audit_log (target_type, target_id, created_at DESC);
```

### 3.18 `api_keys` — akses programatik

```sql
CREATE TABLE api_keys (
    id           TEXT        NOT NULL,
    org_id       TEXT        NOT NULL,
    user_id      TEXT        NOT NULL,       -- pemilik key; untuk RBAC dan audit
    name         TEXT        NOT NULL,       -- label yang dikenali pemilik
    prefix       TEXT        NOT NULL,       -- 8 karakter pertama `adk_...` prefiks
    token_hash   TEXT        NOT NULL,       -- SHA-256 (bukan bcrypt — key sudah high entropy)
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,               -- NULL = aktif; diisi saat revoke
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT api_keys_pk           PRIMARY KEY (id),
    CONSTRAINT api_keys_id_ulid_chk  CHECK (char_length(id) = 26),
    CONSTRAINT api_keys_prefix_chk   CHECK (char_length(prefix) = 8),
    CONSTRAINT api_keys_hash_chk     CHECK (char_length(token_hash) = 64),
    CONSTRAINT api_keys_name_chk     CHECK (btrim(name) <> ''),
    CONSTRAINT api_keys_user_fk      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT api_keys_org_fk       FOREIGN KEY (org_id)  REFERENCES orgs(id)  ON DELETE CASCADE
);

-- Melayani: autentikasi — SELECT ... WHERE prefix = $1 AND revoked_at IS NULL (prefix adalah 8 karakter pertama token).
CREATE UNIQUE INDEX api_keys_prefix_uniq ON api_keys (prefix) WHERE revoked_at IS NULL;

-- Melayani: daftar key untuk user.
CREATE INDEX api_keys_user_idx ON api_keys (user_id, org_id);
```

### 3.19 `sessions` — login user

```sql
CREATE TABLE sessions (
    id         TEXT        NOT NULL,
    user_id    TEXT        NOT NULL,
    token_hash TEXT        NOT NULL,       -- SHA-256 dari opaque token 64-byte
    user_agent TEXT,                       -- ditampilkan di halaman Sesi Aktif (US-AD90 AC2)
    ip         INET,                       -- ditampilkan di halaman Sesi Aktif (US-AD90 AC2)
    last_seen_at TIMESTAMPTZ,              -- diperbarui saat token dipakai; dasar sliding window
    expires_at TIMESTAMPTZ NOT NULL,       -- max 30 hari sejak create
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT sessions_pk              PRIMARY KEY (id),
    CONSTRAINT sessions_id_ulid_chk     CHECK (char_length(id) = 26),
    CONSTRAINT sessions_hash_chk        CHECK (char_length(token_hash) = 64),
    CONSTRAINT sessions_user_fk         FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Melayani: verify session — SELECT ... WHERE id = $1 AND token_hash = $2 AND expires_at > now().
CREATE INDEX sessions_expires_idx ON sessions (expires_at) WHERE expires_at < now();
-- Index di atas untuk cleaner job: DELETE FROM sessions WHERE expires_at < now() - interval '7 days'.
```

### 3.20 `password_reset_tokens` — token reset password sekali-pakai

```sql
CREATE TABLE password_reset_tokens (
    id         TEXT        NOT NULL,
    user_id    TEXT        NOT NULL,
    token_hash TEXT        NOT NULL,       -- SHA-256; token mentah hanya ada di email
    expires_at TIMESTAMPTZ NOT NULL,       -- 30 menit sejak dibuat (US-AD88 AC3)
    used_at    TIMESTAMPTZ,                -- non-NULL = sudah dipakai, tolak dengan 410
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT prt_pk           PRIMARY KEY (id),
    CONSTRAINT prt_id_ulid_chk  CHECK (char_length(id) = 26),
    CONSTRAINT prt_hash_chk     CHECK (char_length(token_hash) = 64),
    CONSTRAINT prt_user_fk      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Melayani: validasi token — SELECT ... WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now().
CREATE INDEX prt_hash_idx ON password_reset_tokens (token_hash);
CREATE INDEX prt_expires_idx ON password_reset_tokens (expires_at) WHERE used_at IS NULL;
```

Permintaan reset untuk email yang tidak terdaftar tetap mengembalikan `202` dan **tidak** membuat baris di sini — respons identik agar keberadaan akun tidak dapat ditebak (US-AD88 AC5).

### 3.21 `notifications` — notifikasi in-app

```sql
CREATE TABLE notifications (
    id         TEXT        NOT NULL,
    user_id    TEXT        NOT NULL,
    org_id     TEXT        NOT NULL,
    kind       TEXT        NOT NULL,       -- approval.requested, budget.warning, run.failed, credential.invalid
    title      TEXT        NOT NULL,
    body       TEXT,
    target_type TEXT,                      -- 'task' | 'run' | 'board'
    target_id  TEXT,                       -- untuk deep-link dari notifikasi
    read_at    TIMESTAMPTZ,                -- NULL = belum dibaca
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notifications_pk         PRIMARY KEY (id),
    CONSTRAINT notifications_id_ulid_chk CHECK (char_length(id) = 26),
    CONSTRAINT notifications_kind_chk   CHECK (kind IN ('approval.requested','budget.warning','run.failed','credential.invalid')),
    CONSTRAINT notifications_user_fk    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT notifications_org_fk     FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE
);

-- Melayani: badge belum dibaca + daftar notifikasi (US-AD61).
CREATE INDEX notifications_unread_idx ON notifications (user_id, created_at DESC) WHERE read_at IS NULL;
```

### 3.22 `webhooks` — konfigurasi pengiriman event ke URL eksternal

```sql
CREATE TABLE webhooks (
    id          TEXT        NOT NULL,
    org_id      TEXT        NOT NULL,
    board_id    TEXT        NOT NULL,
    url         TEXT        NOT NULL,        -- URL HTTPS (diperiksa aplikasi; SSRF guardrail §16)
    secret      TEXT        NOT NULL,        -- HMAC secret; disimpan dalam bentuk yang sama seperti dikirim (plain untuk HMAC — dienkripsi di DB via AES-256-GCM, lihat §16)
    events_json JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- daftar event_kind yang dipantau
    active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT webhooks_pk              PRIMARY KEY (id),
    CONSTRAINT webhooks_id_ulid_chk     CHECK (char_length(id) = 26),
    CONSTRAINT webhooks_url_chk         CHECK (url ~ '^https://'),
    CONSTRAINT webhooks_events_chk      CHECK (jsonb_typeof(events_json) = 'array'),
    CONSTRAINT webhooks_board_fk        FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
    CONSTRAINT webhooks_org_fk          FOREIGN KEY (org_id)   REFERENCES orgs(id)  ON DELETE CASCADE
);

-- Melayani: pengiriman event — SELECT ... WHERE board_id = $1 AND active = true AND events_json ? $kind.
CREATE INDEX webhooks_board_active_idx ON webhooks (board_id, active);
```

### 3.23 `webhook_deliveries` — catatan pengiriman

```sql
CREATE TABLE webhook_deliveries (
    id            BIGSERIAL   NOT NULL,
    webhook_id    TEXT        NOT NULL,
    event_id      BIGINT      NOT NULL,        -- FK ke events.id
    status        TEXT        NOT NULL DEFAULT 'pending',  -- 'pending'|'delivered'|'failed'|'dead'
    attempts      SMALLINT    NOT NULL DEFAULT 0,
    response_code SMALLINT,                    -- HTTP status dari endpoint
    last_error    TEXT,                         -- pesan error (dipotong 500 karakter)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT webhook_deliveries_pk          PRIMARY KEY (id),
    CONSTRAINT webhook_deliveries_status_chk  CHECK (status IN ('pending','delivered','failed','dead')),
    CONSTRAINT webhook_deliveries_attempt_chk CHECK (attempts BETWEEN 0 AND 10),
    CONSTRAINT webhook_deliveries_webhook_fk  FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE
);

-- Melayani: retry driver — ambil pending/failed delivery yang belum dead.
CREATE INDEX webhook_deliveries_retry_idx ON webhook_deliveries (webhook_id, status)
    WHERE status IN ('pending','failed');

-- Melayani: daftar riwayat pengiriman per webhook.
CREATE INDEX webhook_deliveries_webhook_idx ON webhook_deliveries (webhook_id, id DESC);
```

### 3.24 Fungsi Cleanup (Retensi)

```sql
-- AgentDeck cleanup job — dijalankan dispatcher tiap jam (Cron oleh Tick: sekali per jam, bukan per 2 detik).
-- Retensi: events 30 hari (N10), artifacts 90 hari (N12), sessions expired 7 hari.

CREATE OR REPLACE FUNCTION agentdeck_cleanup() RETURNS integer AS $$
DECLARE
    purged integer := 0;
BEGIN
    -- Events hot 30 hari (N10): hapus batch 1000 per eksekusi supaya vacuum tidak terganggu
    DELETE FROM events
    WHERE id IN (
        SELECT id FROM events
        WHERE created_at < now() - interval '30 days'
        ORDER BY id LIMIT 1000
        FOR EACH ROW
    );
    GET DIAGNOSTICS purged = ROW_COUNT;

    -- Artifact meta (R2 dikelola terpisah — lihat §12)
    -- Hanya hapus meta artifact untuk file yang sudah dihapus dari R2.
    -- Catatan: referensi R2 cleanup di §12.

    -- Sessions expired > 7 hari
    DELETE FROM sessions
    WHERE expires_at < now() - interval '7 days';

    -- Ledger entries > 12 bulan pindah ke tabel agregat (bagian dari §9, bukan di sini)

    RETURN purged;
END;
$$ LANGUAGE plpgsql;
```

---

## 4. Query Kritis

### 4a. Klaim task (dispatcher tick, N19 (tick 2 s) dan N20 (batch 20 task))

**Tujuan**: Dispatcher mengambil batch task `ready` dan mengklaimnya secara atomik. Aman untuk N19 (multi-instance) dan N20 (konkurensi tinggi) karena `FOR UPDATE SKIP LOCKED` membuat tiap instance lock row yang berbeda.

**Index yang dipakai**: `tasks_ready_claim_idx` (board_id, priority DESC, created_at DESC WHERE status='ready' AND current_run_id IS NULL)

```sql
-- Klaim batch — satu transaksi per batch (default 20 task per tick, N20)
-- Di-execute oleh dispatcher.go dalam transaksi READ COMMITTED.
WITH claimable AS (
    SELECT t.id
    FROM tasks t
    WHERE t.board_id = $1                    -- hanya board milik kita
      AND t.status = 'ready'
      AND t.current_run_id IS NULL            -- tidak sedang dirunning
      AND (t.assignee_agent_id IS NULL OR EXISTS (
          SELECT 1 FROM agents a WHERE a.id = t.assignee_agent_id
      ))                                      -- agent valid atau di-unassign (dibaca dispatcher terpisah)
      AND t.id NOT IN (                       -- cek dependency yang belum selesai
          SELECT tl.parent_id
          FROM task_links tl
          JOIN tasks pt ON pt.id = tl.parent_id
          WHERE tl.child_id = t.id AND pt.status NOT IN ('done','cancelled','archived')
      )
    ORDER BY t.priority DESC, t.created_at ASC
    LIMIT $2                                  -- batch size (N20: 20)
    FOR UPDATE SKIP LOCKED
    FOR UPDATE OF t
)
UPDATE tasks t
SET
    status = 'running',
    current_run_id = gen_random_uuid()::text,   -- ULID dibikin aplikasi, placeholder di sini
    started_at = COALESCE(t.started_at, now())  -- first start; tidak overwrite
FROM claimable c
WHERE t.id = c.id
RETURNING t.id, t.current_run_id;
```

**Catatan**: `FOR UPDATE SKIP LOCKED` dipasang di CTE `claimable` bukan di `UPDATE langsung` karena Postgres tidak mendukung `SKIP LOCKED` di target `UPDATE`. CTE lock kemudian `UPDATE` baris yang sama — lock yang dipegang CTE tetap berlaku sampai transaksi commit.

**Kompleksitas**: `O(N log M)` dengan N batch size (≤5) dan M task claimable. Biaya dominan adalah seq scan partial index `tasks_ready_claim_idx` — index tersebut partial di `WHERE status = 'ready' AND current_run_id IS NULL`, jadi hanya baris yang relevan yang di-traverse.

### 4b. Heartbeat & reclaim task basi (N7 (heartbeat 60 s) dan N8 (stale reclaim 15 menit))

**Tujuan**: Dispatcher secara periodik memeriksa run yang `status = 'running'` tapi tidak ada heartbeat dalam 15 menit (N7). Run yang basi di-reclaim: run outcome 'reclaimed', task kembali ke 'ready' (kalau masih layak), dan status run di-set 'ended'. N8: agent yang hang juga di-reclaim.

**Index yang dipakai**: `runs_stale_claim_idx` (status, last_heartbeat_at WHERE status='running' AND claim_expires IS NOT NULL)

```sql
-- Reclaim basi — dijalankan dispatcher tiap 30 detik (di luar tick utama).
-- READ COMMITTED; transaksi singkat.
WITH stale_runs AS (
    SELECT r.id, r.task_id, r.org_id
    FROM runs r
    WHERE r.status = 'running'
      AND r.claim_expires IS NOT NULL
      AND r.last_heartbeat_at < now() - interval '15 minutes'   -- N8: 15 menit batas stale reclaim (heartbeat interval N7: 60 s)
    ORDER BY r.last_heartbeat_at ASC
    LIMIT 50                                                      -- batch reclaim
    FOR UPDATE SKIP LOCKED
    FOR UPDATE OF r
    SKIP LOCKED
),
updated_runs AS (
    UPDATE runs r
    SET
        status = 'ended',
        outcome = 'reclaimed',
        ended_at = now()
    FROM stale_runs s
    WHERE r.id = s.id
    RETURNING r.task_id, r.org_id
)
-- Kembalikan task ke 'ready' untuk bisa diklaim ulang.
UPDATE tasks t
SET
    status = 'ready',
    current_run_id = NULL,
    consecutive_failures = t.consecutive_failures + 1
FROM updated_runs u
WHERE t.id = u.task_id AND t.status = 'running';
```

**Catatan**: CTE pertama memilih run basi. CTE kedua meng-update runs. CTE ketiga mengembalikan task ke ready. Run yang sudah di-reclaim otomatis bisa dicoba lagi oleh dispatcher (consecutive_failures naik; jika sudah ≥ max_attempts, task jadi 'failed' bukan 'ready' — §10). `FOR UPDATE SKIP LOCKED` di CTE pertama mencegah dua dispatcher mereclaim run yang sama.

**Kompleksitas**: `O(S)` dengan S = jumlah run basi. Index `runs_stale_claim_idx` menyaring ribuan run menjadi puluhan yang basi.

### 4c. Agregasi biaya harian per board — budget guardrail (N16 ($20/hari), N17 (hard stop $2), dan N18 (alert 80%))

**Tujuan**: Membaca total cost semua task di suatu board selama hari ini (UTC), lalu membandingkan dengan `boards.budget_daily_micros`. Dipanggil dispatcher sebelum mengklaim task baru dan setiap kali step selesai (N17: hard stop per run, N18: ambang alert).

**Index yang dipakai**: `ledger_entries_org_time_idx` (org_id, created_at), `tasks_board_cost_idx` (board_id WHERE cost_micros>0)

```sql
-- Agregasi biaya harian board hari ini.
-- Dipanggil dari dispatcher setelah setiap step selesai (dan tiap tick).
SELECT
    b.id AS board_id,
    b.budget_daily_micros AS cap,
    COALESCE(SUM(le.cost_micros), 0) AS spent_today,
    -- Ambang alert: 80% cap (N18)
    CASE
        WHEN COALESCE(SUM(le.cost_micros), 0) >= b.budget_daily_micros THEN 'exceeded'
        WHEN COALESCE(SUM(le.cost_micros), 0) >= (b.budget_daily_micros * 0.8) THEN 'warning'
        ELSE 'ok'
    END AS budget_status
FROM boards b
LEFT JOIN tasks t ON t.board_id = b.id
LEFT JOIN runs r ON r.task_id = t.id
LEFT JOIN ledger_entries le ON le.run_id = r.id
    AND le.created_at >= date_trunc('day', now() AT TIME ZONE 'UTC')
WHERE b.id = $1
GROUP BY b.id, b.budget_daily_micros;
```

**Optimasi (v1.1 — bukan v0.1)**: Query di atas melakukan triple join yang mahal. Untuk v0.1, budget guardrail bisa menggunakan **materized view** yang di-refresh setiap 5 menit, atau **tabel agregat harian**:

```sql
-- Tabel agregat harian (di-populate oleh cron jam 00:05 UTC setiap hari, atau oleh dispatcher setiap kali step selesai lewat UPSERT).
-- Tabel ini DITAMBAHKAN ke kontrak oleh arsitektur karena join penuh terlalu berat untuk tick 2 detik (N19).
-- Lihat §20 untuk pembenaran.

CREATE TABLE daily_board_costs (
    org_id          TEXT        NOT NULL,
    board_id        TEXT        NOT NULL,
    day             DATE        NOT NULL,          -- UTC date
    total_micros    BIGINT      NOT NULL DEFAULT 0,
    run_count       INTEGER     NOT NULL DEFAULT 0,
    tokens_in       BIGINT      NOT NULL DEFAULT 0,
    tokens_out      BIGINT      NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT daily_board_costs_pk PRIMARY KEY (board_id, day),
    CONSTRAINT daily_board_costs_org_fk FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE,
    CONSTRAINT daily_board_costs_fk FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
    CONSTRAINT daily_board_costs_nonneg_chk CHECK (total_micros >= 0 AND run_count >= 0)
);

-- Query budget dengan tabel agregat:
SELECT
    b.id AS board_id,
    b.budget_daily_micros AS cap,
    COALESCE(dbc.total_micros, 0) AS spent_today,
    CASE ... END AS budget_status
FROM boards b
LEFT JOIN daily_board_costs dbc ON dbc.board_id = b.id
    AND dbc.day = current_date AT TIME ZONE 'UTC'
WHERE b.id = $1;
```

**Kompleksitas**: `O(1)` — satu b-tree lookup ke PK `(board_id, day)`. Tidak ada join ke ledger_entries. Ini penting untuk N1 (latensi baca p95 ≤ 150 ms) serta kapasitas konkurensi N4 (50 agent).

### 4d. Timeline event per task (replay trace)

**Tujuan**: Menghasilkan urutan kronologis event untuk satu task — dari dibuat sampai selesai. Dipakai untuk UI timeline, run trace, dan debugging.

**Index yang dipakai**: `events_task_idx` (task_id, id)

```sql
-- Timeline lengkap satu task — diurutkan oleh event global id (yang = urutan waktu).
-- Cursor pagination: setelah id_cursor = $2.
SELECT
    e.id,
    e.kind,
    e.created_at,
    e.payload_json,
    e.run_id,
    -- Denormalisasi untuk UI: tampilkan nama step/agent tanpa join tambahan
    e.payload_json->>'agent_name' AS agent_label,
    e.payload_json->>'step_name' AS step_label
FROM events e
WHERE e.task_id = $1
  AND ($2 IS NULL OR e.id > $2)     -- cursor pagination (id BIGSERIAL = urutan kronologis)
ORDER BY e.id ASC
LIMIT $3;                           -- page size, default 200
```

**Kompleksitas**: `O(P)` dengan P = page size. Index `events_task_idx` mencari task_id lalu scan forward id. Tidak perlu sortir karena id BIGSERIAL sudah urut.

### 4e. Subtask / dependency resolution

**Tujuan**: Ketika sebuah task selesai, dispatcher perlu membangunkan task anak yang diblokir oleh task itu. Query ini juga dipakai untuk menampilkan DAG di UI.

**Index yang dipakai**: `task_links_parent_idx` (parent_id, child_id), `tasks_status_chk`

```sql
-- 4e.1: Bangunkan task yang diblokir setelah parent selesai.
-- Dipanggil sesaat setelah task $1 masuk status 'done'.
WITH dependents AS (
    SELECT tl.child_id
    FROM task_links tl
    WHERE tl.parent_id = $1
)
UPDATE tasks t
SET
    status = 'ready',
    block_kind = NULL     -- hapus blokir kalau sebelumnya karena dependency
WHERE t.id IN (SELECT child_id FROM dependents)
  AND t.status = 'blocked'
  AND t.block_kind = 'dependency';
```

```sql
-- 4e.2: DAG lengkap (ancestors) untuk UI / validasi dispatcher.
-- Query rekursif: semua task yang menjadi prasyarat (transitif) untuk task $1.
WITH RECURSIVE ancestors AS (
    -- Base: direct parents
    SELECT tl.parent_id AS id, 1 AS depth
    FROM task_links tl
    WHERE tl.child_id = $1
    UNION ALL
    -- Recurse: parents of parents
    SELECT tl.parent_id, a.depth + 1
    FROM task_links tl
    JOIN ancestors a ON a.id = tl.child_id
    WHERE a.depth < 20    -- guardrail: max 20 level kedalaman
)
SELECT
    t.id, t.title, t.status, a.depth
FROM ancestors a
JOIN tasks t ON t.id = a.id
WHERE t.org_id = $2
ORDER BY a.depth DESC, t.created_at ASC;
```

**Kompleksitas**: `O(D × B)` dengan D = depth DAG dan B = branching factor. Guardrail 20 level mencegah rekursi tak terbatas. Index `task_links_parent_idx` dan `task_links_child_idx` membuat join recursive efisien.

---

## 5. Algoritma Dispatcher

### 5.1 Arsitektur Loop

Dispatcher adalah satu-satunya goroutine di AgentDeck yang **menulis state task/runs**. Loop utama berjalan tiap 2 detik (N19) — satu instance per board yang aktif.

```
┌─────────────────────────────────────────────────┐
│              Dispatcher Tick (2s)               │
├─────────────────────────────────────────────────┤
│  1. Heartbeat — perbarui last_heartbeat_at      │
│     untuk semua run yang sedang dirunning        │
│     oleh instance ini.                           │
│  2. Reclaim — cari run basi (N7: heartbeat 60 s, N8: reclaim 15 menit)             │
│  3. Evaluate candidates — untuk tiap task        │
│     yang status='ready' di board:                │
│     a. Cek dependency DAG (task_links)           │
│     b. Cek budget cap (N16)                      │
│     c. Cek approval gate (kontrak §3)            │
│     d. Pilih agent terbaik atau fail             │
│  4. Claim batch — FOR UPDATE SKIP LOCKED (N20: 20 task) │
│  5. Spawn worker goroutine per run claimed       │
│  6. Dead letter — run ended + outcome=failed     │
│     yang no more retries                         │
│  7. Cleanup (tiap jam sekali)                    │
└─────────────────────────────────────────────────┘
```

### 5.2 Pseudocode

```go
// dispatcher.go — tick loop (N19)
package dispatcher

import (
    "context"
    "log/slog"
    "sync"
    "time"

    "agentdeck/internal/db"
    "agentdeck/internal/executor"
)

type Dispatcher struct {
    pool      *db.Pool
    boardID   string
    tick      time.Duration        // 2 detik
    workers   sync.WaitGroup
    cancel    context.CancelFunc
    hostname  string               // untuk claim_lock identifikasi instance
}

func (d *Dispatcher) Run(ctx context.Context) {
    ctx, d.cancel = context.WithCancel(ctx)
    ticker := time.NewTicker(d.tick)
    defer ticker.Stop()

    lastCleanup := time.Now()

    for {
        select {
        case <-ctx.Done():
            d.workers.Wait()
            return
        case <-ticker.C:
            d.tickOnce(ctx, &lastCleanup)
        }
    }
}

func (d *Dispatcher) tickOnce(ctx context.Context, lastCleanup *time.Time) {
    // === Fase 1: Heartbeat — perbarui runs yang dikelola instance ini ===
    d.heartbeatOwned(ctx)

    // === Fase 2: Reclaim — cari runs yang basi (N7: heartbeat 60 s, N8: reclaim 15 menit) ===
    d.reclaimStale(ctx)

    // === Fase 3: Evaluasi dan Klaim ===

    // 3a: Ambil board budget cap (daily_board_costs, §4c)
    budget := d.loadBudget(ctx, d.boardID)

    // 3b: Ambil kandidat task 'ready' dari DB (max N20 batch)
    candidates := d.fetchCandidates(ctx, d.boardID)     // §4a: query dengan tasks_ready_claim_idx

    for _, task := range candidates {
        // 3c: Skip kalau budget exceeded
        if budget.Exceeded() {
            d.logBudgetExceeded(ctx, task.ID)
            continue
        }

        // 3d: Cek dependency DAG — masih ada parent yang belum done?
        if d.hasUnfinishedDependencies(ctx, task.ID) {
            // Kembalikan task ke 'blocked' dengan block_kind = 'dependency'
            d.setBlocked(ctx, task.ID, "dependency")
            continue
        }

        // 3e: Cek approval mode
        gateMode := d.determineApprovalMode(ctx, task)
        switch gateMode {
        case "deny":
            // Task tidak boleh dijalankan — set 'failed', reason policy
            d.failTask(ctx, task, "policy_denied")
            continue
        case "require":
            // Buat approval record. Task tetap 'ready'; Run belum dibuat.
            d.createApprovalGate(ctx, task)
            d.setStatus(ctx, task.ID, "awaiting_approval")
            logEvent(ctx, "approval.requested", task.ID, nil)
            continue
        case "auto":
            // ✅ Lanjut ke klaim (no-op fallthrough)
        }

        // 3f: Verifikasi agent valid
        agent, err := d.resolveAgent(ctx, task.AssigneeAgentID)
        if err != nil {
            d.failTask(ctx, task, "capability")
            continue
        }

        // 3g: Cek retry limit
        if task.ConsecutiveFailures >= agent.MaxAttempts {
            d.failTask(ctx, task, "failed")
            continue
        }

        // === Fase 4: Klaim batch (§4a) ===
        runID, claimed := d.claimTask(ctx, task.ID, agent.ID, task.Attempt+1)
        if !claimed {
            continue        // task sudah diambil instance lain atau gagal lock
        }

        // === Fase 5: Spawn worker ===
        d.workers.Add(1)
        go d.runWorker(ctx, task, agent, runID, task.Attempt+1)
    }

    // Fase 6: Dead-letter — finalisasi task yang sudah tidak mungkin retry
    d.deadLetter(ctx)

    // Fase 7: Cleanup (tiap jam)
    if time.Since(*lastCleanup) > time.Hour {
        d.runCleanup(ctx)     // panggil agentdeck_cleanup()
        *lastCleanup = time.Now()
    }
}
```

### 5.3 Worker Goroutine: Eksekusi Run

```go
// runWorker menjalankan satu Run. Dipanggil sebagai goroutine.
func (d *Dispatcher) runWorker(ctx context.Context, task Task, agent Agent, runID string, attempt int) {
    defer d.workers.Done()
    runCtx, cancel := context.WithTimeout(ctx, time.Duration(agent.MaxRuntimeSeconds)*time.Second)
    defer cancel()

    // Signal start ke DB
    d.setRunStatus(runCtx, runID, "running")
    logEvent(runCtx, "run.claimed", task.ID, &runID)

    // Loop step di dalam Run
    executor := executor.New(agent, task)
    result := executor.Execute(runCtx, func(step executor.Step) {
        // Setiap step selesai → catat ke DB
        d.insertStep(runCtx, runID, step.Seq, step.Kind, step.Status,
            step.TokensIn, step.TokensOut, step.CostMicros)

        // Catat biaya ke ledger
        d.insertLedgerEntry(runCtx, runID, task.ID, agent.Provider, agent.Model,
            step.TokensIn, step.TokensOut, step.CacheRead, step.CacheWrite,
            step.CostMicros, step.PriceVersion)

        // Agregasi budget harian (UPSERT daily_board_costs)
        d.upsertDailyCost(runCtx, task.BoardID, step.CostMicros)

        // Cek budget hard stop (N17)
        if d.isBudgetExceeded(runCtx, task.BoardID) {
            cancel()
        }

        // Heartbeat tiap 10 langkah
        if step.Seq%10 == 0 {
            d.heartbeatRun(runCtx, runID)
        }
    })

    // Finalisasi Run
    if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
        d.finalizeRun(runCtx, runID, "ended", "timed_out", nil)
    } else if errors.Is(runCtx.Err(), context.Canceled) {
        d.finalizeRun(runCtx, runID, "ended", "budget_exceeded", nil)
    } else {
        outcome := "succeeded"
        failureKind := (*string)(nil)
        if result.Error != "" {
            outcome = "failed"
            failureKind = classifyFailure(result.Error)  // §10
        }
        d.finalizeRun(runCtx, runID, "ended", outcome, failureKind)
    }

    // Task status setelah Run selesai
    d.updateTaskAfterRun(ctx, task.ID, runID)
}

func (d *Dispatcher) updateTaskAfterRun(ctx context.Context, taskID, runID string) {
    // Baca status run
    run := d.getRun(ctx, runID)
    if run == nil { return }

    switch run.Outcome {
    case "succeeded":
        d.setTaskStatus(ctx, taskID, "done")
        d.resetConsecutiveFailures(ctx, taskID)
        d.wakeDependents(ctx, taskID)       // §4e.1
    case "budget_exceeded":
        d.setTaskStatus(ctx, taskID, "ready")  // Bisa dicoba lagi nanti
    case "failed", "timed_out":
        task := d.getTask(ctx, taskID)
        if task.ConsecutiveFailures >= task.MaxAttempts {
            d.setTaskStatus(ctx, taskID, "failed")
        } else {
            d.setTaskStatus(ctx, taskID, "ready")
        }
    case "reclaimed":
        // Task sudah dikembalikan ke ready oleh §4b
    }
}
```

### 5.4 State Machine Task

| Status sebelumnya | Trigger | Status baru | Transisi legal? |
|---|---|---|---|
| `backlog` | User/SDK → PATCH status='ready' | `ready` | ✅ |
| `backlog` | User/SDK → PATCH status='cancelled' | `cancelled` | ✅ |
| `ready` | Dispatcher: claim (auto) | `running` | ✅ |
| `ready` | Dispatcher: approval gate | `awaiting_approval` | ✅ |
| `ready` | User/SDK → PATCH status='backlog' | `backlog` | ✅ |
| `running` | Dispatcher: step budget exceeded | `ready` | ✅ (N17) |
| `running` | Dispatcher: run selesai sukses | `done` | ✅ |
| `running` | Dispatcher: run gagal + retry | `ready` | ✅ |
| `running` | Dispatcher: run gagal + no retry | `failed` | ✅ |
| `running` | Dispatcher: run pending (approval) | `awaiting_approval` | ✅ |
| `running` | Dispatcher: reclaim | `ready` | ✅ (via §4b) |
| `awaiting_approval` | User → approve | `ready` | ✅ (kembali ke ready untuk klaim) |
| `awaiting_approval` | User → reject | `blocked` (policy) | ✅ |
| `awaiting_approval` | Expiry (24h, N23) | `blocked` (policy) | ✅ |
| `awaiting_approval` | User → cancel | `cancelled` | ✅ |
| `blocked` | Parent selesai (dispatcher) | `ready` | ✅ |
| `blocked` (`dependency`) | User/SDK → PATCH status='ready' setelah hulu beres | `ready` | ✅ |
| `blocked` (`needs_input`) | User memberikan input lalu PATCH status='ready' | `ready` | ✅ |
| `blocked` (`capability`) | Admin memperbaiki kredensial provider (`US-AD86`) lalu PATCH status='ready' | `ready` | ✅ |
| `blocked` (`budget`) | Pagu board dinaikkan (`US-AD30`) lalu PATCH status='ready' | `ready` | ✅ |
| `blocked` (`policy`/`external`) | User/SDK → PATCH status='backlog' untuk ditangani ulang | `backlog` | ✅ |
| `blocked` | User/SDK → PATCH status='cancelled' | `cancelled` | ✅ |
| `review` | User → approve hasil | `done` | ✅ |
| `review` | User → minta perbaikan | `ready` | ✅ |
| `done` | — | terminal (kecuali arsip di bawah) | ✅ |
| `failed` | — | terminal (kecuali arsip di bawah) | ✅ |
| `cancelled` | — | terminal (kecuali arsip di bawah) | ✅ |
| `done` | User/SDK → PATCH status='archived' (`US-AD59`) | `archived` | ✅ |
| `failed` | User/SDK → PATCH status='archived' (`US-AD59`) | `archived` | ✅ |
| `cancelled` | User/SDK → PATCH status='archived' (`US-AD59`) | `archived` | ✅ |
| `archived` | — | terminal | ✅ |

### 5.5 State Machine Run

| Status sebelumnya | Trigger | Status baru |
|---|---|---|
| _created_ | (Saat INSERT) | `pending` |
| `pending` | Dispatcher: klaim mulai | `claiming` |
| `claiming` | Dispatcher: worker start | `running` |
| `running` | Worker: selesai normal | `ended` |
| `running` | Worker: timeout/kill | `ended` |
| `running` | Dispatcher reclaim: timeout | `ended` |
| `running` | Dispatcher: budget hard stop | `ended` |
| `ended` | — | terminal |

Transisi `outcome` di `runs` (hanya diisi saat `status='ended'`): `succeeded`, `failed`, `timed_out`, `cancelled`, `reclaimed`, `budget_exceeded`.

---

## 6. Kontrak HTTP API

### 6.1 Konvensi Umum

- **Go 1.22+ ServeMux routing**: Pola `METHOD /path/{id}` dipakai langsung oleh `net/http` tanpa router eksternal (P1).
- **Versioning**: Seluruh endpoint domain diawali `/api/v1/`. Endpoint non-versi hanya `/healthz`, `/livez`, `/readyz`, `/metrics`.
- **Header Autentikasi**:
  - Web UI: Cookie session `agentdeck_session=<opaque_token_64_bytes>` (HttpOnly, Secure, SameSite=Lax).
  - Programatik/CLI/SDK: `Authorization: Bearer adk_<org_prefix>_<48_chars>`.
- **Header Idempotency**:
  - Klien boleh menyertakan `Idempotency-Key: <string>` (N24) pada `POST` mutatif. Disimpan di `tasks.idempotency_key` atau cache in-memory kunci 24 jam. Request identik kedua mengembalikan response yang sama persis tanpa mutasi ulang.
- **Paginasi Cursor**:
  - `GET` koleksi mendukung parameter `?limit=50&cursor=<id_terakhir>`.
  - Default limit = 50, maksimum = 200.
  - Envelope response koleksi:
    ```json
    {
      "data": [ ... ],
      "meta": { "limit": 50, "has_more": true, "next_cursor": "01J7ABCDEF1234567890ABCDEF" }
    }
    ```
- **Error Envelope JSON**:
  - Status HTTP 4xx/5xx selalu mengembalikan format seragam:
    ```json
    {
      "error": {
        "code": "TASK_NOT_FOUND",
        "message": "Task dengan ID 01J7... tidak ditemukan di org ini.",
        "target": "id",
        "doc_url": "https://agentdeck.dev/docs/errors/TASK_NOT_FOUND"
      }
    }
    ```
- **Rate Limit**:
  - In-memory token bucket per IP dan per API Key (rate limit standar 100 req/menit (target reliabilitas N25: 99.5% / bulan), 60 req/menit untuk unauthenticated login/register). Melebihi batas mengembalikan `429 Too Many Requests` + header `Retry-After: 60`.

---

### 6.2 Tabel Endpoint Lengkap (109 Endpoint)

**Kolom `Role Min` hanya berlaku untuk aktor manusia.** Nilai yang sah:
`None` (publik) · `Viewer` · `Member` · `Admin` · `Owner` — persis enum `role`
di DECISIONS §4. Tidak ada nilai lain.

Endpoint ber-`Auth: Internal/Key` memakai aktor **`Worker`**, yaitu *bukan* role
keanggotaan melainkan **kredensial agent** (`api_keys`, format `adk_...`,
§11.2). Endpoint ini dipanggil dispatcher/agent yang memegang run, dan
wewenangnya dibatasi oleh kepemilikan run (`runs.agent_id` cocok dengan
`api_keys`), bukan oleh `memberships.role`. Karena itu `Role Min`-nya ditulis
`Worker` — di luar enum `role`, dan sengaja begitu.

#### 6.2.1 Health, Liveness & Metrics (4 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/healthz` | Public | None | Ya | `200 OK` ping liveness container |
| `GET` | `/livez` | Public | None | Ya | `200 OK` process running |
| `GET` | `/readyz` | Public | None | Ya | Cek koneksi DB pool & R2 reachability |
| `GET` | `/metrics` | Basic Auth / Int | Admin | Ya | Prometheus text format scrape metrics (§14) |

#### 6.2.2 Auth & Sessions (11 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `POST` | `/api/v1/auth/register` | Public | None | Tidak | Body `{email, password, name?, org_name?}` → `201` + cookie. `name` absen → diisi bagian lokal email; `org_name` absen → workspace personal dibuat otomatis (B2C, US-AD01 AC5/AC6) |
| `POST` | `/api/v1/auth/login` | Public | None | Tidak | Body `{email, password}` → `200` + cookie session |
| `POST` | `/api/v1/auth/logout` | Session | Viewer | Ya | Hapus baris di `sessions` → `204 No Content` |
| `GET` | `/api/v1/auth/me` | Session/Key | Viewer | Ya | Return profile `{id, email, name, workspaces: [...]}` — US-AD89 |
| `PATCH` | `/api/v1/auth/me` | Session | Viewer | Ya | Update profil sendiri `{name, email, avatar}` → `200`; email duplikat → `409` (US-AD89 AC3) |
| `DELETE` | `/api/v1/auth/me` | Session | Owner | Tidak | Tutup akun sendiri; wajib konfirmasi `{confirm_email}` → `202`; soft-delete 30 hari (US-AD98) |
| `POST` | `/api/v1/auth/password/change` | Session | Viewer | Tidak | Body `{old_password, new_password}` → `200`; cabut seluruh sesi LAIN, sesi ini tetap (US-AD90 AC1) |
| `GET` | `/api/v1/auth/sessions` | Session | Viewer | Ya | List sesi aktif milik user: `{id, user_agent, ip, last_seen_at, current}` (US-AD90 AC2) |
| `DELETE` | `/api/v1/auth/sessions/{id}` | Session | Viewer | Ya | Cabut sesi sendiri; sesi user lain butuh `owner`/`admin` (US-AD90 AC4, US-AD05) |
| `POST` | `/api/v1/auth/password/reset-request` | Public | None | Tidak | Body `{email}` → `202 Accepted` — selalu `202`, email tak terdaftar pun (US-AD88 AC5) |
| `POST` | `/api/v1/auth/password/reset` | Public | None | Tidak | Body `{token, new_password}` → `200 OK`; token kedaluwarsa/dipakai → `410` (US-AD88 AC3) |

#### 6.2.3 API Keys (5 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/api-keys` | Session | Member | Ya | List key milik user (`id, name, prefix, last_used_at`) |
| `POST` | `/api/v1/api-keys` | Session | Member | Ya (Key) | Body `{name}` → `201` + plaintext `adk_...` (hanya sekali) |
| `GET` | `/api/v1/api-keys/{id}` | Session | Member | Ya | Detail key + statistik pemakaian |
| `DELETE` | `/api/v1/api-keys/{id}` | Session | Member | Ya | Hapus fisik baris api_key |
| `POST` | `/api/v1/api-keys/{id}/revoke` | Session | Member | Ya | Update `revoked_at = now()` → `200 OK` |

#### 6.2.4 Orgs & Memberships (9 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/orgs` | Session | Viewer | Ya | List org di mana user menjadi anggota |
| `POST` | `/api/v1/orgs` | Session | Viewer | Ya (Key) | Buat org baru `{name, slug}` → user jadi `owner` |
| `GET` | `/api/v1/orgs/{id}` | Session/Key | Viewer | Ya | Detail org `{id, slug, name, created_at}` |
| `PATCH` | `/api/v1/orgs/{id}` | Session/Key | Owner | Ya | Update nama/slug org |
| `DELETE` | `/api/v1/orgs/{id}` | Session/Key | Owner | Ya | Soft/hard delete org + cascade seluruh data |
| `GET` | `/api/v1/orgs/{id}/members` | Session/Key | Viewer | Ya | List user di org + role masing-masing |
| `POST` | `/api/v1/orgs/{id}/members` | Session/Key | Admin | Ya | Invite user `{email, role}` |
| `PATCH` | `/api/v1/orgs/{id}/members/{user_id}` | Session/Key | Admin | Ya | Ubah role anggota `{role: "admin"|"member"|"viewer"}` |
| `DELETE` | `/api/v1/orgs/{id}/members/{user_id}` | Session/Key | Admin | Ya | Hapus keanggotaan user dari org |

#### 6.2.5 Projects (5 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/projects` | Session/Key | Viewer | Ya | List project dalam org aktif |
| `POST` | `/api/v1/projects` | Session/Key | Member | Ya (Key) | Body `{name, slug}` → `201 Created` |
| `GET` | `/api/v1/projects/{id}` | Session/Key | Viewer | Ya | Detail project |
| `PATCH` | `/api/v1/projects/{id}` | Session/Key | Admin | Ya | Update `{name, slug}` |
| `DELETE` | `/api/v1/projects/{id}` | Session/Key | Admin | Ya | Hapus project + cascade board & task |

#### 6.2.6 Boards (7 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/projects/{project_id}/boards` | Session/Key | Viewer | Ya | List board di dalam project |
| `POST` | `/api/v1/projects/{project_id}/boards` | Session/Key | Member | Ya (Key) | Body `{name, slug, budget_daily_micros}` |
| `GET` | `/api/v1/boards/{id}` | Session/Key | Viewer | Ya | Detail board + kolom + ringkasan status |
| `PATCH` | `/api/v1/boards/{id}` | Session/Key | Member | Ya | Update `{name, slug, budget_daily_micros}` |
| `DELETE` | `/api/v1/boards/{id}` | Session/Key | Admin | Ya | Hapus board + task |
| `GET` | `/api/v1/boards/{id}/columns` | Session/Key | Viewer | Ya | Get array `columns_json` |
| `PATCH` | `/api/v1/boards/{id}/columns` | Session/Key | Member | Ya | Update urutan/label kolom di `columns_json` |

#### 6.2.7 Agents (8 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/projects/{project_id}/agents` | Session/Key | Viewer | Ya | List agent worker per project |
| `POST` | `/api/v1/projects/{project_id}/agents` | Session/Key | Member | Ya (Key) | Register agent baru (model, tools, runtime, retry) |
| `GET` | `/api/v1/agents/{id}` | Session/Key | Viewer | Ya | Detail konfigurasi agent |
| `PATCH` | `/api/v1/agents/{id}` | Session/Key | Member | Ya | Update model, max_runtime_seconds, retry_policy |
| `DELETE` | `/api/v1/agents/{id}` | Session/Key | Admin | Ya | Hapus agent (tasks.assignee_agent_id jadi NULL) |
| `POST` | `/api/v1/agents/{id}/validate` | Session/Key | Member | Ya | Uji coba handshake / test ping LLM provider |
| `PUT` | `/api/v1/agents/{id}/provider-key` | Session | Admin | Ya (Key) | Simpan / rotasi API key provider LLM (enkripsi AES-256-GCM, US-AD86) |
| `DELETE` | `/api/v1/agents/{id}/provider-key` | Session | Admin | Ya | Hapus kredensial provider agent (agent kembali pakai env default) |

#### 6.2.8 Tasks (11 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/boards/{board_id}/tasks` | Session/Key | Viewer | Ya | List task di board (filter `status, assignee, search`) |
| `POST` | `/api/v1/boards/{board_id}/tasks` | Session/Key | Member | Ya (Key) | Buat task baru (`title, body, workspace_kind`, dll.) |
| `GET` | `/api/v1/tasks/{id}` | Session/Key | Viewer | Ya | Detail lengkap task + active run ID |
| `PATCH` | `/api/v1/tasks/{id}` | Session/Key | Member | Ya | Edit task (`title, body, priority, completion_contract`) |
| `DELETE` | `/api/v1/tasks/{id}` | Session/Key | Admin | Ya | Hapus task permanen |
| `POST` | `/api/v1/tasks/{id}/move` | Session/Key | Member | Ya | Geser task ke kolom/status lain (`{to_status: "ready"}`) |
| `POST` | `/api/v1/tasks/{id}/assign` | Session/Key | Member | Ya | Assign/unassign agent (`{agent_id: "..."}`) |
| `POST` | `/api/v1/tasks/{id}/claim` | Session/Key | Member | Ya | Manual force claim (bypass loop dispatcher) |
| `POST` | `/api/v1/tasks/{id}/cancel` | Session/Key | Member | Ya | Batalkan task & abort active run jika ada |
| `POST` | `/api/v1/tasks/{id}/retry` | Session/Key | Member | Ya | Reset failure count, pindah status ke `ready` |
| `POST` | `/api/v1/tasks/{id}/archive` | Session/Key | Member | Ya | Set status ke `archived`, sembunyikan dari view board |

#### 6.2.9 Task Links / Dependencies (4 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/tasks/{id}/links` | Session/Key | Viewer | Ya | List parent dan child task untuk ID tersebut |
| `POST` | `/api/v1/tasks/{id}/links` | Session/Key | Member | Ya | Tambah edge dependency (`{parent_id: "..."}`) |
| `DELETE` | `/api/v1/tasks/{id}/links/{parent_id}` | Session/Key | Member | Ya | Hapus edge dependency tertentu |
| `GET` | `/api/v1/tasks/{id}/dag` | Session/Key | Viewer | Ya | Tree rekursif seluruh prasyarat task (§4e.2) |

#### 6.2.10 Runs (6 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/tasks/{task_id}/runs` | Session/Key | Viewer | Ya | List seluruh run historis task ini (attempt 1..N) |
| `GET` | `/api/v1/runs/{id}` | Session/Key | Viewer | Ya | Detail status run, outcome, total tokens, cost |
| `POST` | `/api/v1/runs/{id}/cancel` | Session/Key | Member | Ya | Cancel run yang sedang `running` (abort context) |
| `POST` | `/api/v1/runs/{id}/heartbeat` | Internal/Key | Worker | Ya | Worker kirim heartbeat `now()` (N7: 60 s, N8: 15 menit) |
| `POST` | `/api/v1/runs/{id}/end` | Internal/Key | Worker | Ya | Worker laporkan hasil akhir (`outcome, error, summary`) |
| `GET` | `/api/v1/runs/{id}/summary` | Session/Key | Viewer | Ya | Ringkasan teks hasil eksekusi run |

#### 6.2.11 Steps (3 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/runs/{run_id}/steps` | Session/Key | Viewer | Ya | List trace seluruh step dalam run urut `seq` |
| `POST` | `/api/v1/runs/{run_id}/steps` | Internal/Key | Worker | Tidak | Catat step baru (`seq, kind, name, payload`) |
| `PATCH` | `/api/v1/runs/{run_id}/steps/{seq}` | Internal/Key | Worker | Ya | Selesaikan step (`status, tokens, cost_micros`) |

#### 6.2.12 Events & Realtime SSE (4 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/boards/{id}/events` | Session/Key | Viewer | Ya | **SSE Stream** event realtime board (§7) |
| `GET` | `/api/v1/events?board_id={id}` | Session/Key | Viewer | Ya | **SSE Stream** kanal generik (§7, FR-06, US-AD39 AC1); `board_id` wajib |
| `GET` | `/api/v1/tasks/{id}/events` | Session/Key | Viewer | Ya | Event log khusus satu task (JSON / SSE stream) |
| `GET` | `/api/v1/runs/{id}/events` | Session/Key | Viewer | Ya | Replay event log run trace |

#### 6.2.13 Approvals (5 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/approvals` | Session/Key | Member | Ya | Inbox daftar approval berstatus `pending` |
| `GET` | `/api/v1/approvals/{id}` | Session/Key | Member | Ya | Detail diff preview proposal agent (`preview_json`) |
| `POST` | `/api/v1/approvals/{id}/approve` | Session/Key | Member | Ya | Setujui aksi agent → task kembali ke `ready` |
| `POST` | `/api/v1/approvals/{id}/reject` | Session/Key | Member | Ya | Tolak proposal `{reason}` → task jadi `blocked` |
| `POST` | `/api/v1/tasks/{id}/approvals` | Internal/Key | Worker | Ya | Worker meminta approval gate baru |

#### 6.2.14 Cost Ledger & Budget (5 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/boards/{id}/budget` | Session/Key | Viewer | Ya | Realtime usage vs cap harian board (N16: $20/hari, N18: alert 80%) |
| `PATCH` | `/api/v1/boards/{id}/budget` | Session/Key | Admin | Ya | Ubah `budget_daily_micros` board |
| `GET` | `/api/v1/boards/{id}/ledger` | Session/Key | Viewer | Ya | Laporan rincian pemakaian token & mikro-USD |
| `GET` | `/api/v1/runs/{id}/ledger` | Session/Key | Viewer | Ya | Ledger entry terperinci per LLM call di suatu run |
| `GET` | `/api/v1/orgs/{id}/cost-summary`| Session/Key | Admin | Ya | Total pengeluaran per model & board 30 hari |

#### 6.2.15 Artifacts (5 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/tasks/{id}/artifacts` | Session/Key | Viewer | Ya | List metadata artifact milik task |
| `POST` | `/api/v1/tasks/{id}/artifacts/upload-url` | Internal/Key | Worker | Tidak | Minta presigned PUT URL R2 (`filename, size`) |
| `POST` | `/api/v1/tasks/{id}/artifacts` | Internal/Key | Worker | Ya | Daftarkan file sukses di-upload (`sha256, key`) |
| `GET` | `/api/v1/artifacts/{id}` | Session/Key | Viewer | Ya | Metadata satu artifact |
| `GET` | `/api/v1/artifacts/{id}/download`| Session/Key | Viewer | Ya | Redirect `302` ke presigned GET URL R2 |

#### 6.2.16 Comments (4 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/tasks/{id}/comments` | Session/Key | Viewer | Ya | List komentar diskusi pada task |
| `POST` | `/api/v1/tasks/{id}/comments` | Session/Key | Member | Tidak | Kirim komentar baru (dari user atau agent) |
| `PATCH` | `/api/v1/comments/{id}` | Session/Key | Member | Ya | Edit teks komentar milik sendiri |
| `DELETE` | `/api/v1/comments/{id}` | Session/Key | Member | Ya | Hapus komentar |

#### 6.2.17 Webhooks & Deliveries (7 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/boards/{board_id}/webhooks` | Session/Key | Admin | Ya | List webhook subscriptions pada board |
| `POST` | `/api/v1/boards/{board_id}/webhooks` | Session/Key | Admin | Ya (Key) | Daftarkan webhook `{url, events_json, secret}` |
| `GET` | `/api/v1/webhooks/{id}` | Session/Key | Admin | Ya | Detail webhook dan status aktif |
| `PATCH` | `/api/v1/webhooks/{id}` | Session/Key | Admin | Ya | Aktifkan/nonaktifkan webhook atau ubah URL |
| `DELETE` | `/api/v1/webhooks/{id}` | Session/Key | Admin | Ya | Hapus subscription webhook |
| `GET` | `/api/v1/webhooks/{id}/deliveries` | Session/Key | Admin | Ya | Log riwayat pengiriman event & HTTP response code |
| `POST` | `/api/v1/webhooks/{id}/deliveries/{delivery_id}/retry` | Session/Key | Admin | Ya | Kirim ulang webhook delivery yang gagal |

#### 6.2.18 Audit, Search & System (6 Endpoint)
| METHOD | Path | Auth | Role Min | Idempotent | Ringkasan Request/Response |
|---|---|---|---|:---:|---|
| `GET` | `/api/v1/audit-log` | Session/Key | Admin | Ya | Paginasi cursor riwayat modifikasi resource workspace; filter `actor, action, from, to` (US-AD95) |
| `GET` | `/api/v1/notifications` | Session | Viewer | Ya | List notifikasi in-app milik user + `unread_count` (US-AD61) |
| `POST` | `/api/v1/notifications/read` | Session | Viewer | Ya | Body `{ids: [...]}` atau `{all: true}` → `200`; tandai terbaca (US-AD61 AC1) |
| `GET` | `/api/v1/search/tasks` | Session/Key | Viewer | Ya | Full-text trigram search task (`?q=...&board_id=...`) |
| `GET` | `/api/v1/search/runs` | Session/Key | Viewer | Ya | Cari run berdasarkan kegagalan atau metadata |
| `GET` | `/api/v1/system/info` | Public | None | Ya | Info versi backend Go & commit SHA |

*Total endpoint terdefinisi: 109 endpoint.*

## 7. Realtime (SSE)

AgentDeck menggunakan Server-Sent Events (SSE) murni melalui endpoint `GET /api/v1/boards/{id}/events` untuk mendistribusikan perubahan status task, log run trace, dan event sistem ke browser React 19 tanpa WebSocket atau overhead Redis. Latensi end-to-end event dari Postgres ke UI p95 dijaga sangat cepat (N3: ≤ 1.5 s).

### 7.1 Protokol Event & Format Frame

Format frame mematuhi spesifikasi standar W3C SSE text stream (`Content-Type: text/event-stream`):

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache, no-transform
Connection: keep-alive
X-Accel-Buffering: no

id: 1048576
event: task.status_changed
data: {"task_id":"01J7ABCDEF1234567890ABCDEF","from":"ready","to":"running","run_id":"01J7RUN0000000000000000000","board_id":"01J7BOARD00000000000000000"}

: keep-alive ping 1716300000
```

- **Field `id`**: Diisi `events.id` (`BIGSERIAL`). ID ini strictly monotonic dan digunakan sebagai checkpoint pemulihan koneksi.
- **Field `event`**: Nama jenis event sesuai kontrak enum `DECISIONS.md` §4 (misalnya `task.status_changed`, `step.finished`, `approval.requested`).
- **Field `data`**: JSON payload padat (maks 64 KB sesuai N21).
- **Heartbeat Comments (`: ping`)**: Dikirim setiap 15 detik untuk mencegah reverse proxy, Cloudflare, atau load balancer memutus koneksi idle. Target event per bulan (N6: 1.000.000).

### 7.2 Resume via `Last-Event-ID`

Saat koneksi internet pengguna terputus atau tab browser di-refresh, browser secara otomatis menyertakan header `Last-Event-ID: <id_terakhir>` pada request reconnect.

Alur penanganan di backend Go (`internal/sse/hub.go`):
1. Parser membaca header `Last-Event-ID` (atau query param `?last_event_id=...` jika via fetch manual).
2. Jika ada `last_event_id`:
   - Hub mengeksekusi query replay:
     ```sql
     SELECT id, kind, payload_json, created_at
     FROM events
     WHERE board_id = $1 AND id > $2
     ORDER BY id ASC
     LIMIT 500;
     ```
   - Semua event yang terlewat segera di-flush ke response stream secara sinkron.
3. Setelah backlog terkirim, koneksi didaftarkan ke in-memory broadcaster channel untuk menerima event baru secara real-time via Postgres `LISTEN agentdeck_events`.

### 7.3 Backpressure & Batas Koneksi

- **Batas Klien per Node (N6)**: Server Go dirancang untuk menangani hingga 1.000 koneksi SSE aktif simultan per 1 CPU/256 MB RAM dengan alokasi buffer minimal (`bufio.Writer` dengan buffer 4 KB).
- **Buffer Channel Client**: Setiap koneksi klien memiliki buffer channel internal Go berukuran 128 event: `chan []byte(128)`.
- **Drop Policy (Slow Consumer)**: Jika klien browser mengalami lag jaringan parah sehingga channel 128 penuh:
  1. Server mencatat log warning: `slow consumer dropped, closing connection`.
  2. Server sengaja menutup koneksi HTTP klien.
  3. Klien frontend React 19 akan mendeteksi penutupan, lalu otomatis reconnect dengan `Last-Event-ID` terakhir yang berhasil diterimanya, memicu mekanisme replay database tanpa membebani memori RAM server.

### 7.4 Perilaku Pemutusan Proxy

- Header respons wajib: `Cache-Control: no-cache, no-transform` dan `X-Accel-Buffering: no` (mematikan buffering pada Nginx / Cloudflare CDN).
- Ketika TCP connection ditutup sepihak oleh klien/proxy, `http.Request.Context().Done()` langsung menyala via deteksi `io.Writer` error, memicu pembersihan goroutine listener dan deregistrasi channel dari Hub.

---

## 8. Approval Gate

Approval Gate adalah mekanisme penahanan eksekusi AI agent ketika menghadapi aksi kritis yang membutuhkan persetujuan manusia. Gate ini dimodelkan sebagai state mesin eksplisit dan tercatat di tabel `approvals`.

### 8.1 Alur Transisi State

```
[Agent Eksekusi Step Kritis]
           │
           ▼
[Cek Policy: require?] ─── Ya ───► Simpan proposal ke `approvals.preview_json`
           │                       Update `tasks.status = 'awaiting_approval'`
           │                       Update `runs.status = 'running'` (blocked in-memory)
           │                       Kirim event SSE `approval.requested`
           │                                    │
           │                       ┌────────────┴────────────┐
           │                       ▼                         ▼
           │               [Manusia Approve]         [Manusia Reject]
           │                       │                         │
           │                       ▼                         ▼
           │             `decision = 'approved'`   `decision = 'rejected'`
           │             `tasks.status = 'ready'`  `tasks.status = 'blocked'`
           │             Klaim ulang worker        `tasks.block_kind = 'policy'`
           │                       │                         │
           ▼                       ▼                         ▼
   [Lanjut Eksekusi]     [Worker Lanjut Step]        [Run Ended / Gagal]
```

1. **Deteksi Aksi Sensitif**: Agent mengusulkan step berisiko (misal eksekusi migrasi DB, destructive shell command, pembayaran API eksternal) atau profil agent memiliki `gate_mode = 'require'`.
2. **Pembekuan Task**:
   - Dispatcher/Worker meng-insert baris ke tabel `approvals`:
     - `task_id`, `run_id`, `requested_by` (user ID pembuat task/agent).
     - `gate_mode = 'require'`.
     - `expires_at = now() + interval '24 hours'` (N23).
     - `preview_json` berisi snapshot perubahan (misalnya diff patch file, detail perintah shell, payload RPC).
   - `tasks.status` diubah menjadi `awaiting_approval`.
   - Event `approval.requested` di-publish ke event stream.

### 8.2 Struktur `preview_json`

Format `preview_json` dibakukan agar UI React 19 dapat merender diff visual atau peringatan yang jelas:

```json
{
  "type": "shell_command",
  "command": "terraform apply -auto-approve",
  "working_directory": "/workspaces/infra",
  "risk_level": "high",
  "estimated_cost_micros": 5000000,
  "diff_summary": {
    "files_changed": 3,
    "additions": 42,
    "deletions": 12
  }
}
```

### 8.3 Expiry & Timeout (N23)

- Sesuai batas kontrak **24 jam (N23)**, setiap record approval memiliki tenggat waktu `expires_at`.
- Pada setiap tick dispatcher (atau evaluasi berkala), sistem mencari approval kedaluwarsa:
  ```sql
  UPDATE approvals
  SET decision = 'expired', decided_at = now(), reason = 'Auto-expired after 24h'
  WHERE decision = 'pending' AND expires_at < now()
  RETURNING id, task_id, run_id;
  ```
- Task terkait langsung ditransisikan:
  - `tasks.status = 'blocked'`
  - `tasks.block_kind = 'policy'`
  - Run aktif ditandai `outcome = 'failed'` dengan `failure_kind = 'policy'`.
  - Event `approval.expired` dicatat di log.

### 8.4 Aturan Idempotensi Keputusan

Keputusan manusia dikirim melalui `POST /api/v1/approvals/{id}/approve` atau `/reject`.

- **Aturan Transisi Tunggal**: Status `decision` hanya boleh berubah dari `pending` ke nilai terminal (`approved` atau `rejected`).
- **Atomic State Lock**:
  ```sql
  UPDATE approvals
  SET decision = $1, decided_by = $2, decided_at = now(), reason = $3
  WHERE id = $4 AND decision = 'pending'
  RETURNING id, task_id, run_id;
  ```
- Jika ada dua admin menekan tombol bersamaan:
  - Eksekusi pertama berhasil mengembalikan 1 baris (`200 OK`).
  - Eksekusi kedua mengembalikan 0 baris yang diperbarui → API merespons dengan aman `409 Conflict` (atau idempotent `200 OK` dengan payload status saat ini tanpa mengubah `decided_by` pertama).

## 9. Cost Ledger & Budget Guardrail

AgentDeck memposisikan akuntansi biaya LLM sebagai entitas kelas satu. Tidak ada pembulatan mengambang (floating-point rounding error); seluruh kalkulasi menggunakan integer `BIGINT` satuan micro-USD ($1,00 USD = 1.000.000 micro-USD).

### 9.1 Model Harga & `price_version`

Harga model LLM bersifat dinamis seiring waktu, namun catatan ledger historis harus bersifat permanen dan tidak boleh berubah surut. Untuk itu, sistem menggunakan tabel konfigurasi versi harga statis di kode Go (`internal/pricing`):

```go
type ModelPrice struct {
    PriceVersion       int    // Versi snapshot harga (dimulai dari 1)
    Provider           string // "anthropic", "openai", "deepseek"
    Model              string // "claude-3-5-sonnet-20241022", "gpt-4o"
    InputMicrosPer1k   int64  // Misal $3.00 / 1M token = 3 micro-USD / 1k token
    OutputMicrosPer1k  int64  // Misal $15.00 / 1M token = 15 micro-USD / 1k token
    CacheReadPer1k     int64  // Prompt caching hit
    CacheWritePer1k    int64  // Prompt caching write
}
```

Rumus perhitungan mikro-USD per step LLM:
$$\text{CostMicros} = \frac{(\text{tokens\_in} \times \text{InputRate}) + (\text{tokens\_out} \times \text{OutputRate}) + (\text{cache\_read} \times \text{CacheReadRate}) + (\text{cache\_write} \times \text{CacheWriteRate})}{1000}$$

Nilai `price_version` disimpan di setiap baris `ledger_entries` sebagai bukti audit algoritma harga yang dipakai saat transaksi dicatat.

### 9.2 Pelaporan Pemakaian Token per Step

Ketika LLM invocation selesai:
1. Provider API mengembalikan rincian token usage (`prompt_tokens`, `completion_tokens`, `cache_creation_input_tokens`, dsb.).
2. Executor Go memanggil engine harga untuk mendapatkan `cost_micros`.
3. Dalam satu transaksi database atomik:
   - Dibuat 1 baris di `steps` dengan token dan biaya.
   - Dibuat 1 baris di `ledger_entries`.
   - Diakumulasikan ke `runs.tokens_in`, `runs.tokens_out`, dan `runs.cost_micros`.
   - Diakumulasikan ke `tasks.cost_micros`, `tasks.tokens_in`, `tasks.tokens_out`.
   - Diakumulasikan ke `daily_board_costs` (tanggal UTC saat ini).

### 9.3 Budget Guardrail & Perilaku Hard Stop

Sistem memberlakukan 3 lapisan perlindungan anggaran:

```
                  ┌───────────────────────────────┐
                  │ Board Budget Cap: $20/hari    │ (N16)
                  └───────────────┬───────────────┘
                                  │
          ┌───────────────────────┴───────────────────────┐
          │                                               │
          ▼ (80% tercapai)                                ▼ (100% tercapai)
  ┌─────────────────────────┐                     ┌─────────────────────────┐
  │ Alerting Threshold      │                     │ Hard Stop Execution     │
  │ Event:                  │                     │ - Abort Run aktif       │
  │ budget.threshold_crossed│                     │ - Outcome:              │
  │ Webhook sent (N18)      │                     │   budget_exceeded (N17) │
  └─────────────────────────┘                     │ - Task status: blocked  │
                                                  │   block_kind: budget    │
                                                  └─────────────────────────┘
```

1. **Cap Harian Board (N16)**:
   - Default cap adalah $20/hari = `20.000.000` micro-USD (`boards.budget_daily_micros`).
   - Setiap kali dispatcher mengevaluasi task `ready`, dispatcher mengecek total pengeluaran hari ini. Jika `spent_today >= budget_daily_micros`, task baru tidak akan diklaim dan ditandai status `blocked` dengan `block_kind = 'budget'`.

2. **Ambang Peringatan 80% (N18)**:
   - Jika akumulasi biaya harian melampaui 80% dari pagu anggaran ($16 dari $20), sistem menembakkan event:
     `budget.threshold_crossed` dengan payload:
     `{"board_id": "...", "spent_micros": 16200000, "threshold": "80%"}`
   - Webhook notifikasi dikirim ke channel Slack/Discord/Email pengelola.

3. **Hard Stop per Run (N17)**:
   - Setiap run dibatasi batas absolut hard stop (default sama dengan cap harian atau batas konfigurasi board).
   - Segera setelah sebuah step menyelesaikan kalkulasi dan mendeteksi total pengeluaran board telah melebihi 100% cap harian:
     - Worker langsung membatalkan konteks eksekusi (`context.CancelFunc()`).
     - Run ditutup dengan `status = 'ended'`, `outcome = 'budget_exceeded'`, `failure_kind = 'budget'`.
     - Task diubah statusnya menjadi `blocked` dengan `block_kind = 'budget'` (atau kembali ke `ready` jika diatur ulang setelah budget ditambah/reset harian).
     - Event `run.finished` mencatat metrik over-budget tersebut.

---

## 10. Retry & Failure Taxonomy

AgentDeck menerapkan pembedaan tegas antara kegagalan sementara (transient) dan kegagalan terstruktur/deterministik. Retry buta tanpa memahami akar kegagalan dihindari agar tidak membakar token LLM secara sia-sia.

### 10.1 Taksonomi Kegagalan (`failure_kind`)

Kontrak `DECISIONS.md` §4 mendefinisikan 7 jenis kegagalan resmi:

| `failure_kind` | Definisi Akar Masalah | Contoh Kasus | Keputusan Default |
|---|---|---|:---:|
| `transient` | Gangguan jaringan, 429 rate limit provider, 503 gateway timeout, socket reset | Provider Anthropic 529 Overloaded, ECONNRESET | **Retry Otomatis** |
| `needs_input` | Agent mandek karena instruksi ambigu atau membutuhkan kredensial/jawaban manusia | Prompt meminta password database yang tidak disediakan | **Block / No-Retry** |
| `capability` | Model tidak memiliki tool/akses/konteks yang diperlukan untuk tugas | Agent butuh browser engine tapi tool tidak terdaftar | **Fail / No-Retry** |
| `dependency` | Task prasyarat gagal atau output artifact rusak | File input dari parent task tidak ditemukan/invalid | **Block / No-Retry** |
| `policy` | Pelanggaran guardrail keamanan, penolakan approval gate, timeout approval (N23) | Approval ditolak admin, prompt injection terdeteksi | **Fail / No-Retry** |
| `budget` | Pengeluaran board telah menyentuh batas harian (N16: cap $20/hari, N17: hard stop $2/run) | Saldo board habis di tengah jalan | **Block / Hold** |
| `unknown` | Panic runtime tak tertangani, crash sistem tak terduga | Out of memory process, segfault executor | **Retry 1x lalu Fail** |

### 10.2 Matriks Kebijakan Retry & Backoff

Kebijakan retry dikontrol oleh atribut `retry_policy` pada tabel `agents`:
- `never`: Tidak pernah retry, apa pun kegagalannya.
- `transient_only`: Hanya retry jika `failure_kind = 'transient'` (default aman).
- `always`: Retry untuk semua failure kind kecuali `policy` dan `budget`.

Tabel pemetaan operasional:

| `failure_kind` | `retry_policy = transient_only` | `retry_policy = always` | Delay Backoff | Status Akhir Task (Jika Limit Habis) |
|---|:---:|:---:|---|---|
| `transient` | **Retry** (jika attempts < max) | **Retry** (jika attempts < max) | Eksponensial: $2^{\text{attempt}} \times 5\text{s}$ (maks 60s) + jitter | `failed` |
| `needs_input` | **No Retry** | **No Retry** | - | `blocked` (`block_kind='needs_input'`) |
| `capability` | **No Retry** | **Retry** (jika attempts < max) | Konstan: 10s | `failed` (`consecutive_failures++`) |
| `dependency` | **No Retry** | **No Retry** | - | `blocked` (`block_kind='dependency'`) |
| `policy` | **No Retry** | **No Retry** | - | `blocked` (`block_kind='policy'`) |
| `budget` | **No Retry** | **No Retry** | Tahan hingga pergantian hari UTC (00:00) | `blocked` (`block_kind='budget'`) |
| `unknown` | **No Retry** | **Retry** (maks 1x) | 30s | `failed` |

### 10.3 Aturan Backoff & Batas Percobaan (`max_attempts`)

- Setiap task memiliki counter `consecutive_failures`.
- Nilai awal percobaan adalah `attempt = 1`.
- Jika retry diizinkan:
  1. Status task dikembalikan ke `ready`.
  2. `consecutive_failures` dinaikkan 1.
  3. Dispatcher menghitung waktu jeda berdasarkan `runs.ended_at` terakhir ditambah delay backoff eksponensial.
  4. Begitu `consecutive_failures >= agents.max_attempts` (default 3, maks 10):
     - Sistem menghentikan seluruh percobaan ulang.
     - Task dipindahkan ke status terminal `failed`.
     - Event `run.finished` dengan outcome `failed` dikirim ke webhook dead-letter.

## 11. Auth, RBAC & Isolasi Tenant

### 11.1 Session Management

- **Login**: Setelah verifikasi `password_hash` dengan `argon2id`, sesi disimpan di tabel `sessions` dan klien menerima cookie HTTP-Only berisi opaque random token (64 byte dari `crypto/rand`):
  - `Set-Cookie: agentdeck_session=<base64_64bytes>; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=2592000` (30 hari).
- **Verifikasi**: Tiap permintaan HTTP yang diautentikasi:
  1. Middleware membaca `agentdeck_session` cookie atau header `Authorization: Bearer <key>`.
  2. Sistem menghitung `SHA-256` token masuk.
  3. Mencocokkan dengan token hash di `sessions` atau `api_keys`.
  4. Validasi `expires_at > now()` (sessions) atau `revoked_at IS NULL` (api_keys).
  5. **Resolusi org aktif** (FR-05) — dua jalur, urutan prioritas:
     - **Header `X-Org-ID`** (eksplisit, menang kalau ada): middleware memvalidasi user
       adalah anggota org tersebut lewat `memberships`. Bukan anggota → `403`.
       Org tidak ada → `404`. Header ini yang dipakai untuk berpindah ruang kerja di UI
       dan untuk menguji isolasi lintas-tenant.
     - **Fallback session**: kalau header tidak dikirim, pakai org aktif terakhir yang
       dipilih user (disimpan di sesi); kalau belum ada, org pertama berdasarkan
       `memberships.created_at`.
  6. Setiap query data wajib menyertakan `org_id` hasil resolusi di atas sebagai
     filter. Tidak ada query data tanpa `org_id` (US-AD07, G5).

### 11.2 API Key Pattern

**Dua jenis aktor, dua jalur wewenang.** Jangan campur keduanya:

| Aktor | Kredensial | Wewenang ditentukan oleh |
|---|---|---|
| Manusia | cookie `agentdeck_session` | `memberships.role` ∈ `owner`/`admin`/`member`/`viewer` |
| Worker (agent) | API key `adk_<org_prefix>_<hex>` | kepemilikan run: `runs.agent_id` harus cocok |

Endpoint internal (`/runs/{id}/heartbeat`, `/runs/{id}/end`, `/runs/{run_id}/steps`,
`/tasks/{id}/approvals`, `/tasks/{id}/artifacts*`) menulis `Role Min = Worker`.
`Worker` **bukan** nilai enum `role` dan tidak pernah muncul di `memberships`.
Worker hanya boleh menyentuh run miliknya sendiri; mencoba run milik agent lain
mengembalikan `403` walau berada di org yang sama.


- **Format Key**: `adk_<org_prefix>_<48byte_random_hex>`. Contoh: `adk_myorg_7a9f3c2b1d0e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f`
- **Penyimpanan**: Di database, hanya disimpan `prefix` (8 karakter pertama `adk_myor`) dan `SHA-256` hash dari full token.
- **Scope**: API Key tidak bisa login ke Web UI, hanya untuk programatik access. Key mewarisi `role` dari `memberships` pemiliknya.
- **Revoke**: `api_keys.revoked_at = now()`; key tidak bisa dipakai immediate.

### 11.3 RBAC Matrix

Kontrak `DECISIONS.md` §4 mendefinisikan 4 role:

| Resource / Action | owner | admin | member | viewer |
|---|---|---|---|---|
| Hapus Org (`DELETE /orgs/{id}`) | ✅ | ❌ | ❌ | ❌ |
| Ubah Org (`PATCH /orgs/{id}`) | ✅ | ❌ | ❌ | ❌ |
| Kelola Member (`POST members`) | ✅ | ✅ | ❌ | ❌ |
| Hapus Member | ✅ | ✅ | ❌ | ❌ |
| Hapus Project / Board | ✅ | ✅ | ❌ | ❌ |
| Buat/Edit Task | ✅ | ✅ | ✅ | ❌ |
| Hapus Task | ✅ | ✅ | ✅ | ❌ |
| Manage Webhooks | ✅ | ✅ | ❌ | ❌ |
| Lihat Audit Log | ✅ | ✅ | ❌ | ❌ |
| Ubah Budget Board | ✅ | ✅ | ❌ | ❌ |
| Baca Board / Task / Run / Ledger | ✅ | ✅ | ✅ | ✅ |
| Manage API Key (milik sendiri) | ✅ | ✅ | ✅ | ✅ |
| Approve/Reject Approval Gate | ✅ | ✅ | ✅ | ❌ |

### 11.4 Pola Isolasi Tenant (org_id Wajib)

Seluruh query database runtime **WAJIB** memfilter `org_id`. Ini bukan kewajiban dokumentasi — ini ditegakkan oleh pola kode setiap repository/DAO.

```go
// TIDAK BOLEH (SQL injection tenant):
rows, _ := pool.Query(ctx, "SELECT * FROM tasks WHERE board_id = $1", boardID)

// WAJIB:
rows, _ := pool.Query(ctx, `
    SELECT t.* FROM tasks t
    JOIN boards b ON b.id = t.board_id
    WHERE t.board_id = $1 AND b.org_id = $2
`, boardID, orgID)
```

**Cara uji isolasi** (§18):
1. Buat 2 org berbeda, masing-masing dengan 1 board dan 1 task dengan judul identik.
2. Login sebagai user org1 → daftar task hanya berisi 1 task milik org1.
3. Query manual: pastikan `user_id` user org2 tidak bisa melihat task org1, bahkan jika parameter `board_id` sengaja diisi board milik org lain.

---

## 12. Workspace & Eksekusi Agent

### 12.1 Tipe Workspace (`workspace_kind`)

Kontrak `DECISIONS.md` §4 mendefinisikan 4 mode workspace:

| `workspace_kind` | Arti | Siklus Hidup | Isolasi |
|---|---|---|---|
| `scratch` | Direktori sementara baru (`os.MkdirTemp`). Paling murah. | Dibuat saat Run dimulai, dihapus saat Run berakhir (`defer os.RemoveAll`). | OS process-level (no sandbox hard). Aman untuk tasks umum. |
| `dir` | Path direktori spesifik di filesystem yang sudah ada (`workspace_path`). | Dibaca saja atau inisialisasi git repo. Tidak otomatis dihapus. | OS process-level. |
| `worktree` | Git worktree (`git worktree add <path> <branch_name>`). | Dibuat saat Run dimulai, dihapus saat Run selesai (`git worktree remove`). | Docker tidak wajib — hanya depends on git CLI. |
| `container` | Eksekusi di Docker/k3s ephemeral container. | `docker run --rm` — container dibuat, kode disalin, output diekstrak, container dihapus. | **Terisolasi penuh** (namespace, cgroups). Wajib untuk tasks yang butuh menginstall paket sistem atau menjalankan kode tak terpercaya dari model. |

### 12.2 Batas Resource

Untuk semua mode workspace, executor memberlakukan pembatasan (guardrail) berikut:

| Resource | Batas | Sanksi pelanggaran |
|---|---|---|
| Disk (total output artifact, N22) | 25 MB per file, 100 MB per task | Step.failed, `failure_kind = 'capability'` |
| Heap Memory | 512 MB (soft), 1 GB (hard limit) | Panic OOM → `unknown` |
| Runtime (N9) | `agents.max_runtime_seconds` (default 4 jam) | Run `timed_out` |
| Step payload | 64 KB per payload (N21) | Step.failed (potong di applicasi) |
| Total steps | 5.000 langkah per run (guardrail DB) | Run otomatis dihentikan |

### 12.3 Siklus Artifact & Unggah ke R2

1. Agent menghasilkan file di dalam `workspace_path` (misal: `task_{id}/report.pdf`).
2. Executor (dalam worker Run) menghitung SHA-256 file.
3. Executor memanggil `POST /api/v1/tasks/{id}/artifacts/upload-url` → server Go membalas dengan **presigned PUT URL** Cloudflare R2 yang berlaku 15 menit.
4. Executor meng-**upload** binary file langsung ke R2 via presigned URL (tidak melewati server Go untuk bandwidth murah, durasi max per run N9: 4 jam).
5. Executor memanggil `POST /api/v1/tasks/{id}/artifacts` dengan `{filename, sha256, storage_key}`.
6. Backend memverifikasi SHA-256 matching dan menulis row ke tabel `artifacts`.
7. **Cleanup (N12)**: Setiap jam, cron job menandai artifact dengan `created_at < now() - interval '90 days'` (90 hari, N12) untuk dihapus dari R2 dan meta dihapus dari Postgres.

---

## 13. Webhook

### 13.1 Skema Arsitektur

```
[Worker / Dispatcher]
        │
        ▼  INSERT events.id
[Postgres: events] ──► LISTEN/NOTIFY agentdeck_events
                              │
                              ▼
                    [Webhook Worker]
                          │
                          ├─► HMAC SHA-256 header (X-AgentDeck-Signature: v1=<hex>)
                          ├─► POST HTTPS ke endpoint pelanggan
                          │   
                          ├─► [2xx] → webhook_deliveries.status = 'delivered'
                          ├─► [4xx] → webhook_deliveries.status = 'failed' (no retry)
                          └─► [5xx/timeout] → retry eksponensial
```

### 13.2 Penandatanganan HMAC

Setiap request HTTPS ke endpoint pelanggan menyertakan header:

```http
POST /webhooks/agentdeck
Content-Type: application/json
X-AgentDeck-Signature: v1=ab2ef8f1c5d97bc2836df64d8a9b1ec08c9a35f6a29b42b83e712d69c599af3c
```

- **Metode**: HMAC-SHA256 dari body JSON.
- **Secret yang digunakan**: `webhooks.secret` dan `agents.provider_api_key_enc` — disimpan di Postgres dalam bentuk AES-256-GCM encrypted bersama nonce (12B) + ciphertext + tag (16B) menggunakan kunci master `AGENTDECK_MASTER_KEY` (lihat §16).
- **Payload yang di-tandatangani**: Body HTTP mentah persis seperti yang dikirim (tanpa whitespace modifikasi).

**Verifikasi di sisi pelanggan**:
```python
import hmac, hashlib
body = request.get_data()
sig = request.headers.get('X-AgentDeck-Signature')  # "v1=..."
secret = os.environ['AGENTDECK_WEBHOOK_SECRET']
expected = f"v1={hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()}"
if not hmac.compare_digest(sig, expected):
    raise PermissionError("Invalid signature")
```

### 13.3 Retry & Backoff

- **Jadwal retry**: Menit ke-1 → ke-5 → ke-15 → ke-30 → jam ke-1 → ke-2 (total 6 percobaan dalam ~3 jam).
- **Respon yang dianggap gagal**:
  - Timeout koneksi > 10 detik.
  - HTTP status `5xx`.
- **Respon yang tidak di-retry**:
  - HTTP status `4xx` (klien error — kemungkinan URL sudah mati atau endpoint signature mismatch).
- **Dead-letter**: Setelah 6 percobaan dan tidak ada sukses, `webhook_deliveries.status = 'dead'`.
  - Dead-letter entries titelnya dapat di-retry manual via `POST /api/v1/webhooks/{id}/deliveries/{delivery_id}/retry`.

### 13.4 Tabel Pengiriman

Lihat tabel `webhook_deliveries` di §3.23 untuk skema lengkap.

## 14. Observabilitas

### 14.1 Structured Logging (`log/slog`)

Semua log runtime menggunakan `log/slog` (stdlib Go 1.21+) dengan format JSON terstruktur.

**Format baris log JSON**:
```json
{
  "time": "2025-09-16T14:22:31.123456+07:00",
  "level": "ERROR",
  "msg": "klaim task gagal — transaksi rollback",
  "trace_id": "adtr_d7a904e12b8f43c0",
  "span_id": "span_3c9b2a87",
  "org_id": "01J7ABC...",
  "board_id": "01J7BOARD...",
  "task_id": "01J7TASK...",
  "run_id": "01J7RUN...",
  "error": "ERROR #23505 duplicate key (pq: duplicate key violates unique constraint)",
  "duration_ms": 234,
  "caller": "internal/dispatcher/claim.go:81"
}
```

**Aturan logging wajib**:
- Setiap permintaan HTTP menyertakan `trace_id` (ULID yang digenerate oleh middleware atau diteruskan dari header `X-Trace-Id`).
- Log level yang digunakan: `DEBUG` (verbose), `INFO` (state changes, request success), `WARN` (rate limit, retry), `ERROR` (kegagalan yang memerlukan intervensi manusia).
- `caller` disertakan secara otomatis pada tingkat `WARN` ke atas.

### 14.2 Metrik Prometheus Yang Wajib Ada

Semua metrik teregister di bawah prefix `agentdeck_`:

| Nama Metrik | Tipe | Label | Deskripsi |
|---|---|---|---|
| `agentdeck_http_requests_total` | Counter | `method`, `path`, `status` (int kode HTTP) | Hit seluruh request selain `/healthz` |
| `agentdeck_http_request_duration_seconds` | Histogram | `method`, `path` | Buckets: 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5 (latensi p95 baca N1: ≤ 150 ms) |
| `agentdeck_tasks_claimed_total` | Counter | `board_id`, `agent_id` | Task yang berhasil diklaim dispatcher |
| `agentdeck_tasks_status_total` | Gauge | `board_id`, `status` | Jumlah task per status pada board |
| `agentdeck_runs_total` | Counter | `outcome` (succeeded/failed/timed_out/reclaimed/budget_exceeded) | Run yang mencapai terminal state |
| `agentdeck_runs_active` | Gauge | `org_id` | Run yang sedang running saat ini |
| `agentdeck_dispatcher_loop_duration_ms` | Gauge | `board_id` | Durasi satu siklus tick dispatcher (N19: interval 2 s) |
| `agentdeck_sse_connections_active` | Gauge | - | Koneksi SSE aktif saat ini (alarm jika > 1000 per instance) |
| `agentdeck_sse_events_sent_total` | Counter | `event_kind` | Jumlah frame SSE yang dikirim ke klien |
| `agentdeck_budget_exceeded_total` | Counter | `board_id` | Jumlah kali budget harian board exceeded |
| `agentdeck_db_pool_connections` | Gauge | `state` (idle/in_use) | Status pool koneksi pgx |
| `agentdeck_db_query_duration_seconds` | Histogram | `query_name` (label nama kueri, §4) | Durasi eksekusi SQL kritis |
| `agentdeck_heartbeat_lag_seconds` | Gauge | `run_id` | Seberapa lama sejak heartbeat terakhir (alarm jika > 300s) |

### 14.3 Trace ID Propagation

- `trace_id` di-generate sebagai ULID 26 karakter pada middleware HTTP pertama (atau diteruskan dari header inbound `X-Trace-Id`).
- `trace_id` dipropagasikan ke:
  - Semua log line (field `trace_id`).
  - Semua database query (via `pgx` context).
  - Semua panggilan ke provider LLM (header `x-agentdeck-trace-id`).
  - Semua panggilan webhook (header `X-AgentDeck-Trace-Id`).
- Span ID digunakan untuk internal timing di dalam satu request/run.

### 14.4 Dashboard Minimal (Grafana)

Dashboard wajib diimplementasikan pada 4 panel:

1. **Overview**: TPS (`agentdeck_http_requests_total` rate), error rate (`5xx` percentage), latency P50/P95/P99.
2. **Dispatcher Health**: Loop duration, task claimed rate, reclaim count, stale run count.
3. **Budget & Cost**: Board budget spending board-by-board (cumulative today), run cost distribution, cost per model/provider.
4. **Agent Faults**: Failure kind pie chart, retry attempt histogram, top failing agents.

### 14.5 Alert Yang Wajib

| Nama Alert | Kondisi | Urgensi | Aksi |
|---|---|---|---|
| `HighErrorRate` | `rate(agentdeck_http_requests_total{status=~"5.."}[5m]) > 0.05` | Pager | Investigasi error rate backend |
| `DispatcherStuck` | `agentdeck_dispatcher_loop_duration_ms > 5000` | Warning | Dispatcher kotak; restart instance |
| `BudgetExhausted` | `rate(agentdeck_budget_exceeded_total[1h]) > 0` | Info | Kapasitas anggaran habis; notifikasi owner |
| `HeartbeatDrift` | `max(agentdeck_heartbeat_lag_seconds) > 300` | Warning | Agent tidak mengirim heartbeat; reclaim terpicu |
| `SSEBottleneck` | `agentdeck_sse_connections_active > 800` | Warning | Mendekati batas koneksi per instance (N6) |

---

## 15. Deployment & Biaya

### 15.1 Topologi

```
                            ┌─────────────────────────────┐
                            │   Cloudflare DNS + Caching   │
                            └──────────┬──────────────────┘
                                       │
                  ┌────────────────────┼────────────────────┐
                  │                    │                     │
                  ▼                    ▼                     ▼
        ┌─────────────────┐ ┌────────────────┐ ┌──────────────────────┐
        │  Fly.io App     │ │  Cloudflare R2 │ │ CF Pages/Vercel (React19+Vite)  │
        │  agentdeck-api  │ │  artifacts/    │ │  agentdeck-web      │
        │  - 1× shared-cpu│ │  - $5 gratis   │ │  - Hobby tier       │
        │  - 256 MB RAM   │ │  - $0/gb egress│ │  - Free $0/mo       │
        │  - $1.94/mo     │ │  - no compute  │ │  - 100k req/mo free │
        └────────┬────────┘ └────────────────┘ └──────────────────────┘
                 │                    ▲
                 │  artifactory       │
                 ▼                    │
        ┌───────────────────────────────────────┐
        │  Neon (Postgres 16 Serverless)        │
        │  - 0.5 GB compute, 2 GB storage       │
        │  - $0.00 base (free tier sleep)       │
        │  - + $1.50/mo for always-on compute   │
        │  - 10 GB storage included              │
        └───────────────────────────────────────┘
```

### 15.2 Tabel Biaya Bulanan (Target: ≤ $10, N13)

| Komponen | Layanan | Spesifikasi | Biaya/Bulan (USD) |
|---|---|---|---|
| API Backend | Fly.io | 1× shared-cpu-1x (1 vCPU, 256 MB), 2 GB persistent disk | $1.94 |
| Domain & DNS | Cloudflare | Free plan (0 domain fee) | $0.00 |
| Postgres | Neon | Free tier "Launch" (0.5 GB compute, sleep after 5 min idle) | $0.00 |
| Postgres Compute (Always-on) | Neon | Add-on always-on compute (non-sleep) | $1.50 |
| Object Storage | Cloudflare R2 | 10 GB storage, 1M class A ops, 10M class B | $0.00 |
| CDN / SSL | Cloudflare free | Termasuk di atas | $0.00 |
| Frontend | Cloudflare Pages / Vercel Hobby | Static SPA React 19 + Vite, 100 GB bandwidth | $0.00 |
| Monitoring | Grafana Cloud Free | 10k series, 14 day retention | $0.00 |
| LLM API (Development Internal) | Variatif | Insidental untuk debugging | $3.00 |
| **Total** | | | **~$6.44** |

Biaya bulanan **sengaja dijaga di bawah $10** (N13). Pada titik awal (pengguna < 100), Neon free tier (tidur setelah 5 menit tidak dipakai) sudah cukup.

### 15.3 Deploy, Migrasi, Rollback, Backup

#### Deploy Pipeline (GitHub Actions)
```yaml
name: deploy
on:
  push:
    branches: [main]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '>=1.22' }
      - run: go build -o agentdeck ./cmd/api
      - run: |
          # Migrasi DB: embed.FS dijalankan via subcommand
          ./agentdeck migrate up
      - uses: superfly/flyctl-actions@1.5
        with:
          args: "deploy --strategy immediate"
```

Migrasi DB menggunakan file SQL `*.up.sql` dan `*.down.sql` yang di-embed dalam binary (`internal/migrate`). Migrasi adalah idempoten: setiap file memiliki checksum dan hanya dijalankan sekali.

#### Rollback
```bash
flyctl releases list
flyctl deploy --image registry.fly.io/agentdeck@sha256:<previous_hash>
./agentdeck migrate down 1
```

#### Backup
- **Postgres (Neon)**: Point-in-Time Recovery (PITR) otomatis 7 hari. Tidak ada biaya tambahan.
- **R2**: Tidak ada fitur versioning di R2 gratis; gunakan `rclone copy R2:artifacts backup-bucket:R2-backup` mingguan jika dibutuhkan.

## 16. Keamanan

### 16.1 Checklist Keamanan

- [ ] **Secret Management**:
  - Semua secret sistem (kunci enkripsi DB, token Fly.io, token R2) dibaca HANYA dari environment variable (`os.Getenv`).
  - Tidak ada secret di kode sumber. `.env` di-`.gitignore`.
- [ ] **Enkripsi Kredensial Agent**:
  - Provider LLM API keys (Anthropic, OpenAI) dan webhook secret dienkripsi di level aplikasi sebelum disimpan ke DB menggunakan **AES-256-GCM**.
  - Nonce 12 byte dibuat acak per enkripsi dan diprefiks ke ciphertext: `nonce(12) + ciphertext + tag(16)`.
- [ ] **Validasi Input**:
  - Ukuran payload event diperiksa di batas HTTP: `pg_column_size` maksimal 64 KB (N21).
  - Judul task dibersihkan dari karakter kontrol null byte `\0`.
  - Slug hanya boleh `[a-z0-9-]` sepanjang 3-40 karakter.
- [ ] **Pencegahan SSRF (Webhook & Tool Executor)**:
  - Validasi ketat terhadap target URL webhook: skema wajib `https://`.
  - Resolusi DNS IP target diverifikasi sebelum koneksi dibuka: IP privat (RFC 1918: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), loopback (`127.0.0.0/8`, `::1`), link-local (`169.254.0.0/16`), dan metadata Cloud provider (`169.254.169.254`) **DIBLOKIR**.
- [ ] **Rate Limiting**:
  - In-memory token bucket per IP dan API Key (reliabilitas uptime N25: 99.5% / bulan). Mencegah brute force login dan serangan DoS terhadap endpoint mahal (seperti LLM proxy).
- [ ] **Audit Trail**:
  - Setiap mutasi peran membership, penghapusan board/project, pencabutan token API, dan pembuatan webhook dicatat ke tabel `audit_log` dengan snapshot before/after.
- [ ] **Penanganan PII**:
  - Password di-hash menggunakan Argon2id (memory 64MB, iterations 3, parallelism 2). Plaintext password tidak pernah dicatat di log.
  - Snapshot `preview_json` memotong data sensitif seperti authorization header atau secret token sebelum di-commit ke database.

---

## 17. Strategi Pengujian

Sistem pengujian AgentDeck dibangun untuk menjamin kebenaran state machine, ketatnya isolasi tenant, dan stabilitas performa di bawah anggaran minimal.

### 17.1 Piramida Pengujian

```
                     ┌───────────────────────┐
                     │   Load Tests (k6)     │  Target konkurensi dan volume (N4: 50 agen running, N5: 100.000 run/bulan, N6: 1.000.000 event/bulan)
                     ├───────────────────────┤
                     │  API Contract Tests   │  109 Endpoint coverage
                     ├───────────────────────┤
                     │ Integration (Pg test) │  Testcontainers Postgres 16
                     ├───────────────────────┤
                     │   Unit Tests (Go)     │  State machine, pricing, DAG
                     └───────────────────────┘
```

1. **Unit Tests (`go test ./internal/...`)**:
   - Algoritma pricing token LLM ke micro-USD (§9.1).
   - Deteksi siklus DAG pada `task_links` (pemberian error jika task merujuk dirinya sendiri atau cyclic).
   - Parser state machine dan transisi legal (§5.4).
2. **Integration Tests (Testcontainers Postgres 16)**:
   - Menjalankan instance Docker Postgres 16 asli di mesin lokal/CI.
   - Uji coba klaim konkuren: 10 worker goroutine mengklaim 50 task siap kerja secara bersamaan untuk membuktikan `FOR UPDATE SKIP LOCKED` tidak menghasilkan klaim ganda.
   - Reclaim basi: simulasi run yang ditinggalkan worker dan pengembalian state ke `ready`.
3. **Tenant Isolation Test Suite (`tests/tenant_isolation_test.go`)**:
   - Memverifikasi bahwa Org B **mustahil** membaca task, run, artifact, atau ledger milik Org A, bahkan jika ID ULID-nya ditebak dengan sengaja.
4. **Load & Stress Tests (k6 / vegeta)**:
   - **Target konkurensi agen (N4: 50)**: Pengujian beban 50 agen aktif serentak per org.
   - **Target kapasitas run (N5: 100.000)**: Pengujian beban desain 100.000 run per bulan.
   - **Target N6**: 1.000 koneksi SSE aktif simultan stabil di bawah 256 MB RAM tanpa connection leak.

### 17.2 Perintah Menjalankan Test

```bash
# 1. Jalankan unit test cepat
go test -v -race -cover ./internal/...

# 2. Jalankan integration test (membutuhkan Docker berjalan)
TEST_INTEGRATION=true go test -v -race ./tests/integration/...

# 3. Jalankan pengujian isolasi tenant secara spesifik
go test -v -run TestTenantIsolation ./tests/tenant_isolation_test.go

# 4. Jalankan load testing k6
k6 run tests/load/sse_connections.js --vus 1000 --duration 5m
```

---

## 18. Struktur Folder

Repositori Go disusun mengikuti prinsip idiomatik Standard Go Project Layout tanpa lapisan abstraksi yang berlebihan:

```
agentdeck/
├── cmd/
│   └── api/
│       └── main.go              # Entrypoint binary tunggal: ServeMux, config init, graceful shutdown
├── internal/
│   ├── api/                     # HTTP Handlers (109 endpoints)
│   │   ├── auth_handler.go      # Login, register, logout, session middleware
│   │   ├── org_handler.go       # Orgs & memberships
│   │   ├── board_handler.go     # Boards, columns, and budget settings
│   │   ├── task_handler.go      # Task CRUD, movement, assignment, and DAG
│   │   ├── run_handler.go       # Run trace, steps, and heartbeat
│   │   ├── approval_handler.go  # Man-in-the-loop gate decisions
│   │   ├── ledger_handler.go    # Cost inspection & reporting
│   │   ├── artifact_handler.go  # Presigned URL generation & download redirect
│   │   ├── webhook_handler.go   # Webhook subscriptions & manual retry
│   │   ├── middleware.go        # Auth, RBAC, Rate-limit, Trace-ID, Org-Context
│   │   └── routes.go            # Go 1.22+ ServeMux route registration table
│   ├── config/                  # Environment variable parser (os.Getenv)
│   ├── db/                      # Koneksi database & query sqlc/pgx
│   │   ├── pool.go              # pgxpool initialization & healthcheck
│   │   ├── queries.sql.go       # Generated code dari sqlc (tiap query di §4)
│   │   └── models.go            # Struct representasi tabel DB (§3)
│   ├── dispatcher/              # Jantung orkestrasi agent
│   │   ├── dispatcher.go        # Tick loop 2 detik (N19) & worker pool
│   │   ├── claim.go             # FOR UPDATE SKIP LOCKED batch claiming (N20)
│   │   ├── reclaim.go           # Heartbeat check & stale cleaner (N7: 60 s, N8: 15 menit)
│   │   └── budget.go            # Guardrail calculation & hard stop (N16: $20/hari, N17: $2/run)
│   ├── executor/                # Eksekutor workspace
│   │   ├── workspace.go         # Scratch, Dir, Git Worktree, & Container driver
│   │   └── runner.go            # Step loop & LLM invocation wrapper
│   ├── pricing/                 # Engine akuntansi biaya
│   │   ├── calculator.go        # Token to micro-USD mapping (§9.1)
│   │   └── models.go            # Snapshot tabel harga model LLM
│   ├── sse/                     # Server-Sent Events real-time broker
│   │   ├── hub.go               # In-memory subscription manager & drop policy (N6)
│   │   └── listener.go          # Postgres LISTEN/NOTIFY agentdeck_events consumer
│   ├── webhook/                 # Background webhook delivery
│   │   ├── worker.go            # HMAC signer & HTTP dispatcher
│   │   └── retry.go             # Exponential backoff scheduler
│   ├── storage/                 # Cloudflare R2 / S3 Client
│   │   └── r2.go                # Presigned PUT/GET generator & SHA verification
│   └── migrate/                 # Database migrations (embed.FS)
│       ├── fs.go                # embed.FS SQL scripts
│       └── 0001_init.sql        # DDL lengkap sesuai §3
├── tests/                       # Suite pengujian
│   ├── integration/             # Integration tests dengan Testcontainers
│   ├── tenant_isolation_test.go # Verifikasi kebocoran tenant
│   └── load/                    # Script k6 untuk target kapasitas N4 (50 agen), N5 (100.000 run), dan N6 (1.000.000 event)
├── Dockerfile                   # Multi-stage build (distroless final image < 25 MB)
├── Makefile                     # Build, test, run, migrate automation
├── go.mod                       # Go 1.22 module definition
└── go.sum
```

### 18.2 Frontend (`frontend/` — React 19 + Vite + Redux Toolkit)

SPA murni (Vite 6 + React 19) dideploy sebagai static asset ke Cloudflare Pages/Vercel — biaya $0. Tidak ada SSR runtime; latensi first-paint dijaga di bawah N2 (≤ 400 ms).

#### Fitur React 19 yang dipakai

| Fitur | Dipakai untuk |
|---|---|
| `useActionState` | Semua form mutasi (`TaskCreateForm`, `AgentConfigForm`, `ApprovalDecisionForm`) → `[state, formAction, isPending]`, tanpa `useState` manual |
| `useOptimistic` | Drag kartu Kanban lintas kolom render instan; rollback otomatis bila mutasi ditolak |
| `useFormStatus` | `SubmitButton`, `ApproveRejectButtons` membaca status pending parent form secara deklaratif |
| `use()` | Unwrapping Promise resource (kamus i18n EN/ID, dynamic metadata) dan pembacaan Context secara kondisional |
| Ref as a Prop | Semua primitif shadcn menerima `ref` langsung, nol `forwardRef` |
| `<Context>` direct | Provider ditulis `<X value={...}>` tanpa `.Provider` |
| Document Metadata | `<title>` / `<meta>` deklaratif per halaman, di-hoist otomatis (react-helmet pensiun) |
| `preload` / `preinit` | Preload font JetBrains Mono & icon set sebelum board dibuka |
| Compiler-ready | Memoization diserahkan ke React Compiler; nol `useMemo`/`useCallback` berlebihan |

#### Redux Toolkit sebagai satu-satunya manajemen state

Tidak ada TanStack Query, tidak ada Zustand, tidak ada Context untuk data aplikasi. `createContext` hanya untuk dependency injection (instance RTK Query pada test), bukan state.

| Kebutuhan | Solusi Redux |
|---|---|
| Server state (board, task, run, agent, ledger) | **RTK Query** `createApi` + tag invalidation |
| Realtime SSE | RTK Query `onCacheEntryAdded` → stream menambal cache langsung |
| Client state (view mode, filter, drawer, bahasa) | `createSlice` (DevTools time-travel) |
| Side effect (alert budget N18, toast) | `createListenerMiddleware` |
| Optimistic drag kartu | RTK Query `optimisticUpdate` + `useOptimistic` |
| Form mutasi | `useActionState` men-dispatch thunk |

```
frontend/
├── index.html                                 # entry point SPA Vite
├── vite.config.ts                             # Vite 6 + Tailwind v4 + React Compiler
├── src/
│   ├── main.tsx                               # createRoot + <Provider store>
│   ├── App.tsx                                # routing, layout, lang provider
│   ├── routes/
│   │   ├── auth/{Login,Register}.tsx
│   │   ├── dashboard/
│   │   │   ├── Layout.tsx                     # sidebar, org picker, breadcrumb
│   │   │   ├── boards/
│   │   │   │   ├── BoardList.tsx
│   │   │   │   ├── KanbanBoard.tsx            # drag-and-drop + useOptimistic
│   │   │   │   ├── TableView.tsx              # dense view (virtualized)
│   │   │   │   ├── TaskDetailDrawer.tsx       # timeline step + live SSE log
│   │   │   │   └── BoardSettings.tsx
│   │   │   ├── approvals/ApprovalInbox.tsx    # diff payload + 1-klik keputusan
│   │   │   ├── agents/{AgentRegistry,AgentDetail}.tsx
│   │   │   ├── finops/{CostOverview,LedgerExplorer}.tsx
│   │   │   └── settings/{Members,ApiKeys,Webhooks}.tsx
│   ├── store/                                 # Redux Toolkit
│   │   ├── index.ts                           # configureStore + listenerMiddleware
│   │   ├── hooks.ts                           # typed useAppDispatch/useAppSelector
│   │   ├── api/
│   │   │   ├── base.ts                        # createApi + fetchBaseQuery (401 refresh)
│   │   │   ├── boards.ts                      # tag: Board, Task, Run, Event, Approval
│   │   │   ├── agents.ts                      # tag: Agent, ProviderKey
│   │   │   ├── finops.ts                      # tag: Ledger, CostSummary
│   │   │   └── stream.ts                      # onCacheEntryAdded → SSE patch cache
│   │   ├── slices/{uiSlice,langSlice,sessionSlice}.ts
│   │   └── listeners/{budgetAlert,toast}.ts
│   ├── components/
│   │   ├── ui/                                # shadcn primitives (ref as prop)
│   │   ├── kanban/                            # Column, TaskCard, DragOverlay
│   │   ├── approvals/                         # DiffViewer, RiskBadge, ActionButtons
│   │   ├── terminal/                          # SSE log reader, StepTimeline
│   │   └── layout/                            # Sidebar, Header, LangToggle (EN/ID)
│   ├── hooks/
│   │   ├── use-optimistic-card.ts             # useOptimistic + RTK Query
│   │   ├── use-action-form.ts                 # useActionState → thunk
│   │   └── use-sse-cache.ts                   # EventSource → cache patch
│   └── lib/formatters.ts                      # formatMicroUSD, formatDuration
```

---

## 19. Risiko Teknis & Batas Skala

### 19.1 Titik Kritis Postgres Sebagai Bottleneck

Karena AgentDeck menganut prinsip P4 (Postgres sebagai satu-satunya stateful dependency), batas kapasitas Postgres menentukan batas seluruh sistem.

| Komponen Beban | Titik Kritis / Bottleneck | Gejala yang Muncul | Sinyal Metrik |
|---|---|---|---|
| **Queue Klaim (`tasks` FOR UPDATE SKIP LOCKED)** | > 500 klaim per detik atau > 100 instance dispatcher aktif | Row-lock contention, autovacuum ketinggalan karena update `status` yang sering | `agentdeck_db_query_duration_seconds{query="claim_task"}` P95 > 50ms, bloat index pada `tasks` |
| **Event Log (`events`)** | > 10.000 event per detik | IOPS disk jenuh, ukuran tabel tembus > 50 GB sebelum 30 hari rotasi (N10) | Disk write IOPS > 90%, lonjakan replikasi lag jika ada replica |
| **LISTEN/NOTIFY Connection** | > 2.000 klien koneksi simultan pada 1 node | Slot payload NOTIFY Postgres terbatas (maks 8 KB), connection leak pada klien SSE | Error `payload too long for NOTIFY` atau crash pool pgx |
| **Budget Calculation Join** | Ratusan board aktif mengeksekusi LLM secara bersamaan | Triple join `boards` → `tasks` → `runs` → `ledger_entries` membebani CPU | Query duration `agregasi_biaya` melonjak melanggar target batas baca (N1: ≤ 150 ms) pada kapasitas beban (N4: 50 agen) |

### 19.2 Kapan Harus Pecah (Scaling Triggers)

Arsitektur monolitik + Postgres tunggal ini sangat memadai hingga **100 agent berjalan paralel dan 1.000 user online simultan**. Namun, pemecahan komponen harus dipertimbangkan jika tanda-tanda berikut terpenuhi:

1. **Pemisahan Queue Engine (Kapan Redis / NATS masuk?)**:
   - *Trigger*: Ketika throughput task melebihi **200 tasks/detik** dan Postgres autovacuum tidak lagi mampu mengejar churn pada tabel `tasks`.
   - *Solusi*: Pindahkan queue transien ke Redis Streams atau NATS JetStream, jadikan Postgres murni sebagai store state final.
2. **Pemisahan Event Store / Log (Kapan ClickHouse / Kafka masuk?)**:
   - *Trigger*: Ketika volume `events` dan `ledger_entries` melebihi **10 juta baris per bulan** sehingga biaya storage Postgres Neon membengkak.
   - *Solusi*: Arahkan append-only event stream ke object storage berbasis Parquet / DuckDB / ClickHouse untuk analisis analitik murah.
3. **Pemisahan Realtime Broker (Kapan SSE hub harus dipisah?)**:
   - *Trigger*: Jumlah koneksi browser aktif melebihi **10.000 koneksi bersamaan** (N6: 1.000.000 event terlampaui secara drastis).
   - *Solusi*: Jalankan node edge khusus SSE (misalnya via Cloudflare Durable Objects atau Go SSE edge service terisolasi) yang mendengarkan stream event via pub/sub.

---

## 20. Usulan Perubahan Kontrak

Sesuai aturan kerja, skema DB (§6), enum (§4), state machine (§3), dan angka (§7) pada
`DECISIONS.md` diikuti persis. **Penambahan berikut sudah diformalkan ke kontrak
DECISIONS §6 dan bukan lagi terselubung:**

| Tabel/Kolom | Status | Sebab |
|---|---|---|
| `daily_board_costs` | ditambahkan (§5, §3 lampiran) | query budget O(1), hindari triple join per tick 2 s (N19) |
| `password_reset_tokens` | ditambahkan (§3.20) | reset password (US-AD88) butuh token sekali-pakai |
| `notifications` | ditambahkan (§3.21) | notifikasi in-app (US-AD61) butuh baris yang bisa dibaca/ditandai |
| `users.avatar_url`, `users.deleted_at` | ditambahkan (§3.3) | profil akun (US-AD89) + penutupan akun (US-AD98) |
| `sessions.user_agent`, `sessions.ip`, `sessions.last_seen_at` | ditambahkan (§3.19) | daftar sesi aktif (US-AD90) butuh identitas perangkat |

Berikut rekomendasi teknis untuk rilis arsitektur berikutnya (`v0.2`):

| No | Elemen Kontrak | Usulan Perubahan | Alasan Arsitektural |
|---|---|---|---|
| 1 | `tasks` (§6) | Tambahkan kolom `ready_at TIMESTAMPTZ` | **Mendukung Backoff Retry di Level DB**: Saat ini penundaan retry ditangani secara in-memory oleh dispatcher dengan membandingkan `runs.ended_at` + jeda backoff (§10). Adanya kolom `ready_at` memungkinkan query klaim `FOR UPDATE SKIP LOCKED` menyaring task yang belum saatnya jalan langsung dengan klausa `WHERE status = 'ready' AND (ready_at IS NULL OR ready_at <= now())`, sehingga dispatcher tidak perlu memfilter manual di memori. |
| 2 | `boards` / Akuntansi Biaya (§6) | Formalisasikan tabel `daily_board_costs` | **Kinerja Budget Guardrail**: Untuk mematuhi SLA latensi baca (N1: ≤ 150 ms) pada beban agen serentak (N4: 50), query agregasi biaya harian board tidak boleh melakukan triple join ke jutaan baris `ledger_entries` setiap tick dispatcher (2 detik, N19). Tabel ringkasan harian `daily_board_costs` memungkinkan pembacaan `O(1)` instan. |
| 3 | `events` (§6) | Partisi tabel `events` berdasarkan rentang waktu (`PARTITION BY RANGE (created_at)`) | **Efisiensi Purge 30 Hari (N10)**: Menghapus baris kadaluarsa dengan `DELETE ... WHERE created_at < now() - interval '30 days'` menghasilkan disk I/O tinggi dan bloat tabel. Menggunakan partisi bulanan/mingguan memungkinkan pembersihan instan dengan perintah `DROP TABLE events_y2025m08` dengan nol biaya vacuum. |
| 4 | `tasks.completion_contract` (§6) | Ubah tipe dari `TEXT` menjadi `JSONB` | **Validasi & Struktur Evaluasi**: Agar agen reviewer dapat mengevaluasi kriteria penyelesaian task (acceptance criteria) secara terprogram tanpa perlu parsing string JSON manual di kode Go. |
| 5 | `events` (§6) | Tambahkan index `events_kind_created_idx ON events (kind, created_at)` | Webhook worker menyaring event berdasarkan `kind` per tick; tanpa index komposit, penyaringan menyapu seluruh partisi. |
