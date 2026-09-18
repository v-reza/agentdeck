# DIAGRAMS.md — Spesifikasi Diagram Arsitektur & Alur Sistem AgentDeck

Dokumen ini mendefinisikan spesifikasi visual berbasis ASCII untuk arsitektur sistem, alur orkestrasi task, state machine transisi status, model relasi data (ERD), dan alur pencatatan serta agregasi biaya (cost ledger) di AgentDeck. Seluruh spesifikasi mematuhi kontrak tunggal yang dibekukan di `DECISIONS.md`.

---

## 1. Diagram Arsitektur Komponen (Component Architecture)

### 1.1 Diagram ASCII

```
+========================================================================================================+
|                                     CLIENT BROWSER LAYER (Edge)                                        |
|  +--------------------------------------------------------------------------------------------------+  |
|  | React 19 + Vite + Redux Toolkit (React Router v7) (Single-Board Kanban, Task Drawer, Live Cost/Token Meters, Approval Modals)   |  |
|  +-----------------------------------+------------------------------------------+-------------------+  |
+======================================|==========================================|======================+
                                       | HTTPS REST API (JSON)                    | SSE (HTTP/2 Stream)
                                       | p95 read <= 150ms [N1]                   | Event->UI <= 1.5s [N3]
                                       v                                          ^
+=================================================================================|======================+
|                          AGENTDECK CORE SERVICE (Go Single Binary <= 30MB [N14])|                      |
|                                                                                 |                      |
|  +--------------------------+  +---------------------------------------------+  |                      |
|  | HTTP Ingress & REST API  |  | Server-Sent Events (SSE) Hub                |--+                      |
|  | - Chi Router             |  | - In-memory pub/sub per tenant / board      |                         |
|  | - Org / Board Auth       |  | - Fan-out 1M event/bulan [N6]               |                         |
|  | - CRUD Task & Run        |  | - Payload chunk limit 64 KB [N21]           |                         |
|  +-------------+------------+  +----------------------+----------------------+                         |
|                |                                      ^                                                |
|                v                                      | Broadcast SSE Event                            |
|  +----------------------------------------------------+----------------------+                         |
|  | Dispatcher Engine (Single Leader / PostgreSQL Advisory Lock)               |                         |
|  | - Periodic tick loop setiap 2 detik [N19]                                 |                         |
|  | - Batch atomic claim: max 20 task per tick [N20] via SELECT FOR UPDATE     |                         |
|  | - Concurrency gate: max 50 agent running bersamaan per org [N4]            |                         |
|  | - Budget guard: stop jika biaya board >= $20/hari [N16] atau Run >= $2 [N17] |                         |
|  | - Stale reclaim: reset runner tanpa heartbeat > 15 menit [N8] ke ready     |                         |
|  +-------------+--------------------------------------+----------------------+                         |
|                |                                      ^                                                |
|                | Webhook Dispatch (HMAC-SHA256)       | Callback API (Status / Step / Cost)            |
+================|======================================|================================================+
                 |                                      |
                 v                                      |
+=======================================================|================================================+
|                          EXTERNAL AGENT RUNNERS & CLOUD PLATFORMS                                      |
|                                                                                                        |
|  +--------------------------+  +--------------------------+  +--------------------------------------+  |
|  | Local / Self-Hosted      |  | Cloud Agent Sandbox      |  | Remote Webhook Agent                 |  |
|  | Hermes Agent / CLI       |  | Modal / Fly.io Container |  | Custom Agent HTTP Endpoint           |  |
|  | (Heartbeat tiap 60s [N7])|  | (Max 4 jam runtime [N9]) |  | (HMAC Signature Verified)            |  |
|  +-------------+------------+  +-------------+------------+  +-------------------+------------------+  |
|                |                             |                                   |                     |
|                +-----------------------------+-----------------------------------+                     |
|                                              |                                                         |
|                                              | Upload Artifacts (Max 25MB/file, 100MB/task [N22])      |
|                                              v                                                         |
|                               +------------------------------+                                         |
|                               | Cloudflare R2 / S3 Object St |                                         |
|                               | (Retensi artifact 90d [N12]) |                                         |
|                               +------------------------------+                                         |
+========================================================================================================+
                                       |
                                       | SQL Queries & Transactions (pgxpool)
                                       v
+========================================================================================================+
|                        PERSISTENCE LAYER (Managed PostgreSQL 16 on Neon / Supabase)                    |
|                                                                                                        |
|  +--------------------------------------------------------------------------------------------------+  |
|  | Core Operational Tables: orgs, boards, tasks, runs, steps, approvals, artifacts                  |  |
|  | Financial Ledger: cost_ledger (immutable, micro-cents precision, integer micros)                 |  |
|  | Event Stream: events (hot retention 30 hari [N10], append-only log)                             |  |
|  | Aggregates: daily_cost_aggregates (retensi harian 12 bulan [N11])                                 |  |
|  +--------------------------------------------------------------------------------------------------+  |
+========================================================================================================+
```

### 1.2 Penjelasan Komponen
Arsitektur AgentDeck mengadopsi pola *monolith modular* berbasis Go yang dikemas dalam satu binary statis berukuran tidak lebih dari 30 MB [N14] dengan konsumsi memori idle di bawah 80 MB [N15]. Lapisan antarmuka pengguna dibangun menggunakan React 19 + Vite (React Router v7) dengan dukungan Server-Sent Events (SSE) dua arah untuk menyajikan data papan secara reaktif tanpa polling berat, memenuhi target first paint <= 400 ms untuk 5.000 task [N2]. Dispatcher Engine berjalan sebagai proses background terkoordinasi dengan PostgreSQL advisory locks, bertugas mengklaim task berkala setiap 2 detik [N19] dalam batch 20 task [N20] dan membatasi maksimal 50 agent berjalan bersamaan per organisasi [N4]. Agen eksternal (Hermes, Claude Code, custom worker) berkomunikasi melalui webhook bertanda tangan HMAC-SHA256, mengirim heartbeat periodik setiap 60 detik [N7], serta melaporkan pemakaian biaya model dan token yang dicatat ke tabel immutable ledger.

### 1.3 Invariant yang Wajib Dijaga
1. **Single Binary Footprint**: Seluruh fungsi REST API, Dispatcher, dan SSE hub harus berjalan dalam satu proses binary Go (`agentdeck-core`) tanpa dependensi eksternal selain PostgreSQL dan penyimpanan objek S3/R2 [N14, N15].
2. **Zero Polling UI**: Klien UI tidak boleh melakukan periodic HTTP polling untuk memperbarui kartu Kanban; seluruh mutasi task, perubahan status, dan pembaruan biaya didorong melalui SSE stream dengan p95 latency <= 1.5 detik [N3].
3. **Dispatcher Concurrency Cap**: Dispatcher dilarang meluncurkan run baru jika jumlah agen aktif pada organisasi yang sama telah mencapai 50 agent [N4], atau jika batas biaya harian board $20 [N16] atau batas Run $2 [N17] telah terlampaui.

---

## 2. Alur Dispatch Satu Task (Single Task Dispatch Flow with Approval Gate)

### 2.1 Diagram Alur Sekuensial (Sequence Diagram ASCII)

```
User/UI             Go Dispatcher          PostgreSQL DB         Agent Runner           Approval Gate
   |                      |                      |                     |                      |
   |-- 1. Create/Move --->|                      |                     |                      |
   |      status='ready'  |-- 2. INSERT/UPDATE ->|                     |                      |
   |                      |      status='ready'  |                     |                      |
   |                      |                      |                     |                      |
   |                      |=== 3. Tick Loop (tiap 2s [N19]) ===        |                      |
   |                      |-- 4. Claim Task ---->|                     |                      |
   |                      |   SELECT FOR UPDATE  |                     |                      |
   |                      |   LIMIT 20 [N20]     |                     |                      |
   |                      |   WHERE active < 50  |                     |                      |
   |                      |   AND budget_ok [N16]|                     |                      |
   |                      |<- 5. Return Tasks ---|                     |                      |
   |                      |                      |                     |                      |
   |                      |-- 6. Start Run ----->|                     |                      |
   |                      |   INSERT runs (...)  |                     |                      |
   |                      |   UPDATE tasks SET   |                     |                      |
   |                      |   status='running'   |                     |                      |
   |<- 7. SSE: running ---|                      |                     |                      |
   |                      |-- 8. Webhook POST Dispatch (HMAC) -------->|                      |
   |                      |                                            |-- 9. Send Heartbeat  |
   |                      |<- 10. POST /heartbeat (tiap 60s [N7]) -----|      (every 60s)     |
   |                      |                                            |                      |
   |                      |                                            |-- 11. Tool requires  |
   |                      |                                            |       gated approval |
   |                      |<- 12. POST /runs/:id/approvals ------------|       (e.g. bash/db) |
   |                      |       { tool_name, args_redacted, ... }    |                      |
   |                      |-- 13. INSERT approvals (status='pending')  |                      |
   |                      |       UPDATE tasks SET                     |                      |
   |                      |       status='awaiting_approval' --------->|                      |
   |<- 14. SSE: modal ----|                                            | (runner paused)      |
   |       awaiting_appr  |                                            |                      |
   |                      |                                            |                      |
   |-- 15. User clicks -->|                                            |                      |
   |       APPROVE /      |-- 16. UPDATE approvals (status='approved') |                      |
   |       REJECT         |       UPDATE tasks SET status='ready'      |                      |
   |                      |       (atau 'cancelled' jika rejected) --->|                      |
   |                      |                                            |                      |
   |                      |-- 17. POST /runs/:id/resume -------------->|                      |
   |                      |       { decision: 'approved' }             |-- 18. Execute tool & |
   |                      |                                            |       continue steps |
   |                      |<- 19. POST /runs/:id/steps ----------------|                      |
   |                      |       (duration, tokens, cost_micros)      |                      |
   |                      |-- 20. INSERT steps & cost_ledger --------->|                      |
   |                      |                                            |                      |
   |                      |<- 21. POST /runs/:id/finish ---------------|                      |
   |                      |       { status: 'completed' }              |                      |
   |                      |-- 22. UPDATE runs status='completed' ------>|                      |
   |                      |       UPDATE tasks status='done'           |                      |
   |<- 23. SSE: done -----|                                            |                      |
   |                      |                                            |                      |
```

### 2.2 Penjelasan Alur
Alur kerja dimulai ketika pengguna atau sistem mengubah status task menjadi `ready`. Dispatcher Engine yang berdetak setiap 2 detik [N19] mengeksekusi kueri `SELECT ... FOR UPDATE SKIP LOCKED` untuk mengklaim hingga 20 task [N20] secara aman dan konkuren, memastikan batas 50 agen aktif [N4] serta pagu anggaran harian board $20 [N16] tidak terlampaui. Begitu task berpindah ke status `running`, Webhook bertanda tangan HMAC dikirimkan ke agen eksternal, yang kemudian wajib mengirimkan heartbeat setiap 60 detik [N7]. Jika agen hendak menjalankan perintah sensitif yang memerlukan otorisasi manusia, agen memanggil endpoint approval sehingga task masuk ke status `awaiting_approval` dan runner memasuki status jeda hingga pengguna memberikan persetujuan atau masa berlaku 24 jam terlewati [N23]. Setelah persetujuan diberikan, runner melanjutkan eksekusi step, mencatat penggunaan token dan biaya ke cost ledger, lalu menyelesaikan task ke status `done`.

### 2.3 Invariant yang Wajib Dijaga
1. **Pessimistic Locking**: Klaim task dari status `ready` ke `running` harus dilakukan dengan transaksi berorientasi baris `SELECT FOR UPDATE` untuk mencegah *double-dispatch* jika terdapat lebih dari satu replika dispatcher.
2. **Approval Expiry Hard-Stop**: Otorisasi yang berstatus `pending` memiliki batas waktu maksimal 24 jam [N23]; apabila terlewati tanpa keputusan pengguna, task otomatis dinyatakan kedaluwarsa (`expired`) dan run dialihkan ke status `cancelled` atau `failed`.
3. **Heartbeat Reclaim**: Jika runner agen berhenti mengirimkan heartbeat lebih dari 15 menit [N8], dispatcher berhak melakukan *stale reclaim*, menandai run lama sebagai `failed`, dan mengembalikan status task ke `ready` untuk dieksekusi ulang.

---

## 3. State Machine Task (Complete Legal Transitions)

### 3.1 Diagram Transisi Status Task (ASCII State Machine)

```
                           +------------------------+
                           |        BACKLOG         |
                           +------------------------+
                                       |
                                       | Manual move / Unblock / Triaged
                                       v
        +----------------------------> +------------------------+ <---------------------------+
        |                              |         READY          |                             |
        |  +-------------------------> +------------------------+ <------------------------+  |
        |  |                                       |                                       |  |
        |  | Unblocked                             | Dispatcher claim                      |  |
        |  |                                       | (tick 2s, batch 20)                   |  |
        |  |                                       v                                       |  |
        |  |   +--------------------> +------------------------+ <---------------------+   |  |
        |  |   |                      |        RUNNING         |                       |   |  |
        |  |   |                      +------------------------+                       |   |  |
        |  |   |                                |  |  |  |                             |   |  |
        |  |   | Step paused                    |  |  |  | Task finished               |   |  |
        |  |   | (requires tool approval)       |  |  |  | successfully                |   |  |
        |  |   |                                |  |  |  +-----------------+           |   |  |
        |  |   v                                |  |  |                    |           |   |  |
        |  | +---------------------+            |  |  |                    v           |   |  |
        |  | |  AWAITING_APPROVAL  |            |  |  |        +---------------------+ |   |  |
        |  | +---------------------+            |  |  |        |       REVIEW        | |   |  |
        |  |   |             |                  |  |  |        +---------------------+ |   |  |
        |  |   | Approved    | Rejected /       |  |  |           |               |    |   |  |
        |  |   | (resume)    | Expired (24h)    |  |  |           | Approved      |    |   |  |
        +------|             |                  |  |  |           v               |    |   |  |
               |             +---------------+  |  |  |    +-------------+        |    |   |  |
               |                             |  |  |  |    |    DONE     |        |    |   |  |
               |                             |  |  |  |    |  (terminal) |        |    |   |  |
               |      External block         |  |  |  |    +-------------+        |    |   |  |
               |      (dependency wait)      |  |  |  |                           |    |   |  |
               |   +-------------------------+  |  |  |                           |    |   |  |
               |   |                            |  |  |   Requires rework         |    |   |  |
               |   v                            |  |  +---------------------------+    |   |  |
        +------------------+                    |  |                                   |   |  |
        |     BLOCKED      |                    |  | Error / Exception / Timeout (>4h) |   |  |
        +------------------+                    |  +-----------------------------+     |   |  |
               |                                |                                |     |   |  |
               +--------------------------------+                                v     |   |  |
                                                                   +---------------+   |   |
                                                                   |    FAILED     |   |   |
                                                                   |  (terminal)   |   |   |
                                                                   +---------------+   |   |
                                                                                       |   |
  ================================ CANCEL RULE ======================================= | = |
  Setiap status non-terminal dapat ditransisikan langsung ke CANCELLED oleh pengguna:  |   |
  [backlog, ready, running, awaiting_approval, blocked, review] -----------------------+   |
                                                                                           v
                                                                   +---------------+
                                                                   |   CANCELLED   |
                                                                   |  (terminal)   |
                                                                   +---------------+
```

### 3.2 Matriks Transisi Legal dan Penjelasan

| Status Asal (`from`) | Status Tujuan (`to`) | Trigger / Kondisi Bisnis | Aktor |
|:---|:---|:---|:---|
| `backlog` | `ready` | Task ditandai siap dieksekusi, prioritas ditetapkan | User / Scheduler |
| `ready` | `running` | Dispatcher mengklaim task, kapasitas org < 50 [N4], budget aman | Dispatcher Engine |
| `running` | `awaiting_approval` | Agen memicu tool sensitif (bash/db/eksternal) yang digate | Agent Runner |
| `awaiting_approval` | `ready` | Otorisasi disetujui (approved), task diantrekan kembali untuk resume | User (Human Operator) |
| `awaiting_approval` | `cancelled` | Otorisasi ditolak (rejected) atau kedaluwarsa > 24 jam [N23] | User / Timeout Guard |
| `running` | `blocked` | Ketergantungan I/O tertahan, limit rate eksternal, atau resource lock | Agent Runner |
| `blocked` | `ready` | Ketergantungan eksternal terpenuhi, siap di-dispatch ulang | User / Webhook Trigger |
| `running` | `review` | Eksekusi langkah selesai, memerlukan inspeksi hasil oleh manusia | Agent Runner |
| `review` | `done` | Hasil kerja diverifikasi dan diterima | User (Reviewer) |
| `review` | `ready` | Hasil kerja ditolak, memerlukan revisi / run ulang | User (Reviewer) |
| `running` | `done` | Task selesai otomatis tanpa memerlukan manual review | Agent Runner |
| `running` | `failed` | Kegagalan eksekusi, runtime melebihi 4 jam [N9], atau budget habis [N17] | Dispatcher / Runner |
| *(semua kecuali terminal)* | `cancelled` | Pembatalan eksplisit oleh pengguna pada task yang sedang/akan berjalan | User (Admin / Operator) |
| `done` | `archived` | Task lama disembunyikan dari board default; baris tetap immutable sebagai arsip | User / Scheduler |
| `failed` | `archived` | Task gagal yang sudah ditangani (tidak akan di-retry) diarsipkan | User / Admin |
| `cancelled` | `archived` | Task batal diarsipkan setelah masa retensi | User / Scheduler |

### 3.3 Invariant yang Wajib Dijaga
1. **Terminal State Immutability**: Status `done`, `failed`, `cancelled`, dan `archived` adalah terminal state permanen. Task yang telah berada pada status ini tidak boleh ditransisikan ke status lain mana pun tanpa membuat entitas task baru. **Satu-satunya pengecualian** adalah transisi `done`/`failed`/`cancelled` → `archived`, yang murni operasi visibilitas (menyembunyikan task lama dari board default) dan tidak mengubah isi, riwayat, maupun hasil task.
2. **Approval Suspension**: Ketika task memasuki `awaiting_approval`, penghitungan batas waktu runtime maksimum (4 jam) [N9] ditangguhkan (*paused*) agar waktu tunggu manusia tidak menghabiskan kuota runtime agen.
3. **No Direct Jump to Done**: Task dilarang melompat langsung dari `backlog`, `ready`, atau `blocked` menuju `done` tanpa pernah melalui proses `running` dan menghasilkan minimal satu record `run`.

---

## 4. Entity Relationship Diagram (ERD) Ringkas

### 4.1 Diagram Hubungan Entitas (ASCII ERD)

```
       +------------------------------------+
       |                orgs                |
       |------------------------------------|
       | PK id                 UUID         |
       |    name               VARCHAR(100) |
       |    slug               VARCHAR(50)  |
       |    plan               VARCHAR(20)  |
       |    created_at         TIMESTAMPTZ  |
       +-----------------+------------------+
                         |
                         | 1:N
                         v
       +------------------------------------+
       |               boards               |
       |------------------------------------|
       | PK id                 UUID         |
       | FK org_id             UUID         |---+
       |    name               VARCHAR(100) |   |
       |    slug               VARCHAR(50)  |   |
       |    default_agent_id   VARCHAR(50)  |   |
       |    budget_daily_micros BIGINT      |   | [N16: $20 default = 20_000_000]
       |    created_at         TIMESTAMPTZ  |   |
       +-----------------+------------------+   |
                         |                      |
                         | 1:N                  |
                         v                      |
       +------------------------------------+   |
       |               tasks                |   |
       |------------------------------------|   |
       | PK id                 UUID         |   |
       | FK board_id           UUID         |   |
       | FK org_id             UUID         |<--+
       |    title              VARCHAR(255) |
       |    description        TEXT         |
       |    status             task_status  | (enum 10 status)
       |    priority           priority_lvl | (low, medium, high, urgent)
       |    assigned_agent_id  VARCHAR(50)  |
       | FK current_run_id     UUID         |---+ (nullable, 1:1 active)
       |    position           DOUBLE PREC  |   |
       |    created_by         VARCHAR(100) |   |
       |    created_at         TIMESTAMPTZ  |   |
       |    updated_at         TIMESTAMPTZ  |   |
       +-----------------+------------------+   |
                         |                      |
                         | 1:N                  |
                         v                      |
       +------------------------------------+   |
       |                runs                |<--+
       |------------------------------------|
       | PK id                 UUID         |
       | FK task_id            UUID         |
       | FK board_id           UUID         |
       | FK org_id             UUID         |
       |    agent_id           VARCHAR(50)  |
       |    status             run_status   | (starting, running, paused, completed, failed)
       |    started_at         TIMESTAMPTZ  |
       |    finished_at        TIMESTAMPTZ  |
       |    duration_ms        INTEGER      |
       |    cost_micros        BIGINT       | (total run cost [N17: cap $2 = 2_000_000])
       |    tokens_in          INTEGER      |
       |    tokens_out         INTEGER      |
       |    trigger            VARCHAR(30)  | (manual, dispatcher, webhook)
       +--------+---------------+-----------+
                |               |
       +--------+               +-----------------------+
       | 1:N                                            | 1:N
       v                                                v
+------------------------------------+   +------------------------------------+
|               steps                |   |             approvals              |
|------------------------------------|   |------------------------------------|
| PK id                 UUID         |   | PK id                 UUID         |
| FK run_id             UUID         |   | FK task_id            UUID         |
| FK task_id            UUID         |   | FK run_id             UUID         |
|    seq                INTEGER      |   | FK org_id             UUID         |
|    name               VARCHAR(100) |   |    step_seq           INTEGER      |
|    tool_call          JSONB        |   |    tool_name          VARCHAR(100) |
|    tool_output        JSONB        |   |    tool_args_redacted JSONB        |
|    duration_ms        INTEGER      |   |    requested_by_agent VARCHAR(50)  |
|    cost_micros        BIGINT       |   |    status             appr_status  | (pending, approved, rejected, expired)
|    tokens_in          INTEGER      |   |    decision_by        VARCHAR(100) |
|    tokens_out         INTEGER      |   |    decided_at         TIMESTAMPTZ  |
|    status             step_status  |   |    reason             TEXT         |
|    created_at         TIMESTAMPTZ  |   |    expires_at         TIMESTAMPTZ  | [N23: 24h default]
+-----------------+------------------+   +------------------------------------+
                  |
                  | 1:1
                  v
+------------------------------------+   +------------------------------------+
|            cost_ledger             |   |               events               |
|------------------------------------|   |------------------------------------|
| PK id                 UUID         |   | PK id                 UUID         |
| FK org_id             UUID         |   | FK org_id             UUID         |
| FK board_id           UUID         |   | FK task_id            UUID         |
| FK task_id            UUID         |   | FK run_id             UUID         |
| FK run_id             UUID         |   |    event_type         VARCHAR(50)  |
| FK step_id            UUID         |   |    payload            JSONB        | [N21: max 64KB]
|    agent_id           VARCHAR(50)  |   |    seq                BIGSERIAL    |
|    model              VARCHAR(100) |   |    created_at         TIMESTAMPTZ  | [N10: retensi 30d]
|    tokens_in          INTEGER      |   +------------------------------------+
|    tokens_out         INTEGER      |
|    cost_micros        BIGINT       |   +------------------------------------+
|    recorded_at        TIMESTAMPTZ  |   |             artifacts              |
+------------------------------------+   |------------------------------------|
                                         | PK id                 UUID         |
                                         | FK task_id            UUID         |
                                         | FK run_id             UUID         |
                                         |    name               VARCHAR(255) |
                                         |    s3_key             VARCHAR(500) |
                                         |    mime_type          VARCHAR(100) |
                                         |    size_bytes         BIGINT       | [N22: max 25MB file]
                                         |    sha256             CHAR(64)     |
                                         |    created_at         TIMESTAMPTZ  | [N12: retensi 90d]
                                         +------------------------------------+
```

### 4.2 Penjelasan ERD
Struktur data berakar pada entitas `orgs` yang menaungi multi-tenant board, di mana setiap `boards` menetapkan alokasi anggaran harian dalam satuan integer micro-cents (`budget_daily_micros`) dengan nilai bawaan $20 [N16]. Entitas `tasks` mencatat siklus hidup pekerjaan individual beserta referensi ke run aktif melalui kolom `current_run_id`. Setiap eksekusi agen dimodelkan sebagai `runs`, yang memecah aktivitas menjadi urutan `steps` atomik, permintaan otorisasi sensitif pada `approvals`, dan file luaran pada `artifacts` dengan batas 25 MB per berkas [N22]. Seluruh penggunaan model AI dicatat tanpa kemungkinan mutasi ke `cost_ledger`, sedangkan riwayat audit dan pembaruan streaming dicatat pada tabel append-only `events` yang memiliki batas muatan 64 KB [N21] dan masa simpan operasional selama 30 hari [N10].

### 4.3 Invariant yang Wajib Dijaga
1. **Tenant Isolation Foreign Keys**: Seluruh tabel transaksi (`tasks`, `runs`, `approvals`, `cost_ledger`, `events`) wajib memuat kolom `org_id` untuk memastikan isolasi tenant yang ketat pada tingkat kueri SQL dan pengindeksan gabungan `(org_id, ...)`.
2. **Immutable Cost Records**: Baris pada tabel `cost_ledger` dan `events` bersifat *append-only*. Tidak boleh ada perintah SQL `UPDATE` maupun `DELETE` operasional pada tabel-tabel ini selain mekanisme background pruning berdasarkan kebijakan retensi data [N10, N11].
3. **Integer Currency Consistency**: Biaya moneter tidak boleh disimpan dalam tipe data desimal floating-point (`FLOAT` atau `DOUBLE`), melainkan wajib disimpan sebagai `BIGINT` dalam satuan mikro-dollar (1 USD = 1.000.000 micros) untuk mencegah kesalahan pembulatan komputasi.

---

## 5. Alur Cost Ledger (Step Recording to Daily Aggregation)

### 5.1 Diagram Alur Pencatatan & Agregasi Biaya (ASCII Flowchart)

```
[ Agent Runner / Step Completion ]
                |
                | 1. Kirim payload Step:
                |    model='claude-3-5-sonnet', tokens_in=1250, tokens_out=420
                v
+========================================================================================+
|                              AGENTDECK BACKEND INGESTION                               |
|                                                                                        |
|  2. Lookup Model Pricing Table (In-memory cache):                                      |
|     - Prompt: $3.00 / 1M token = 3.0 micros / token                                    |
|     - Completion: $15.00 / 1M token = 15.0 micros / token                              |
|                                                                                        |
|  3. Hitung Biaya Langkah (Step Cost Calculation):                                      |
|     cost_micros = (1250 * 3.0) + (420 * 15.0) = 3750 + 6300 = 10.050 micros ($0.01005)  |
|                                                                                        |
|  4. Eksekusi Atomic Transaction (PostgreSQL):                                          |
|     +-------------------------------------------------------------------------------+  |
|     | BEGIN;                                                                        |  |
|     |   INSERT INTO steps (id, run_id, seq, cost_micros, tokens_in, tokens_out, ...);  |  |
|     |   INSERT INTO cost_ledger (org_id, board_id, task_id, run_id, cost_micros, ..);|  |
|     |   UPDATE runs SET cost_micros = cost_micros + 10050 WHERE id = :run_id;      |  |
|     | COMMIT;                                                                       |  |
|     +-------------------------------------------------------------------------------+  |
|                                                                                        |
|  5. Evaluasi Hard Cap & Alert Budget:                                                  |
|     - Apakah Run cost >= $2.00 [N17]? -> Jika YA: trigger STOP / FAIL RUN              |
|     - Apakah Board daily cost >= 80% [N18] dari $20 [N16]? -> Jika YA: broadcast alert |
|     - Apakah Board daily cost >= $20.00 [N16]? -> Jika YA: throttle / pause task ready |
+========================================================================================+
                |
                | 6. Push real-time update via SSE Hub
                v
+----------------------------------------------------------------------------------------+
|                             CLIENT DASHBOARD (Live Meters)                             |
|  - Task Drawer Cost Counter bertambah seketika                                         |
|  - Board Daily Budget Gauge terbarui (< 1.5s p95 latency [N3])                         |
+----------------------------------------------------------------------------------------+
                |
                | 7. Cron Job Malam Hari (Scheduled Background Worker 00:05 UTC)
                v
+========================================================================================+
|                        DAILY LEDGER AGGREGATION & ROLLUP ENGINE                        |
|                                                                                        |
|  8. Query raw ledger dari hari sebelumnya (T-1):                                       |
|     SELECT org_id, board_id, agent_id, model,                                          |
|            SUM(tokens_in) AS sum_tokens_in,                                            |
|            SUM(tokens_out) AS sum_tokens_out,                                          |
|            SUM(cost_micros) AS total_cost_micros                                       |
|     FROM cost_ledger                                                                   |
|     WHERE recorded_at >= :start_of_day AND recorded_at < :end_of_day                   |
|     GROUP BY org_id, board_id, agent_id, model;                                        |
|                                                                                        |
|  9. Simpan ke Tabel Agregat:                                                           |
|     INSERT INTO daily_cost_aggregates (...) VALUES (...)                               |
|     ON CONFLICT (org_id, board_id, agent_id, model, date) DO UPDATE ...;               |
|                                                                                        |
|  10. Retensi & Pembersihan Data:                                                       |
|      - Tabel agregat harian dipertahankan selama 12 bulan [N11].                       |
|      - Baris event log mentah dipangkas setelah melewati 30 hari [N10].                |
+========================================================================================+
```

### 5.2 Penjelasan Alur
Setiap kali sebuah langkah dieksekusi oleh agen runner, data pemakaian model serta jumlah token input dan output dilaporkan ke backend. Sistem AgentDeck secara deterministik menghitung biaya dalam satuan integer `cost_micros` berdasarkan katalog tarif model yang tersimpan di cache memori, kemudian menuliskan entri tersebut ke tabel `steps` dan `cost_ledger` secara atomik di dalam satu transaksi database. Setelah penulisan berhasil, mekanisme budget guard langsung mengevaluasi apakah run telah menyentuh batas hard-stop $2 [N17] atau apakah board telah mencapai ambang peringatan 80% [N18] dari batas harian $20 [N16]. Pada penghujung hari, mesin agregator terjadwal merangkum seluruh transaksi ledger mentah ke dalam tabel `daily_cost_aggregates` untuk analitik jangka panjang dengan masa retensi 12 bulan [N11], sementara log event mentah dibersihkan secara bertahap setelah 30 hari [N10].

### 5.3 Invariant yang Wajib Dijaga
1. **Integer Monotonicity**: Kolom biaya kumulatif pada `runs.cost_micros` dan `daily_cost_aggregates.total_cost_micros` hanya boleh bertambah secara monoton positif selama eksekusi berlangsung; nilai negatif dilarang keras.
2. **Instant Hard-Cap Termination**: Apabila akumulasi biaya satu run mencapai atau melampaui 2.000.000 micros ($2,00) [N17], sistem wajib menolak langkah agen berikutnya dan segera menghentikan eksekusi dengan status `failed` bertanda `budget_exceeded`.
3. **Audit Trail Retention**: Data agregat biaya historis wajib dipertahankan minimal selama 12 bulan [N11] untuk kebutuhan audit keuangan organisasi, kendati rincian event payload mentah telah dibersihkan setelah 30 hari [N10].
