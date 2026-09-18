# 00 — PRD: AgentDeck (AI Agent Fleet Orchestration Board)

| Field | Value |
|---|---|
| **Product** | **AgentDeck** |
| **Version** | 0.1 |
| **Status** | `draft` |
| **Owner** | Reza (solo dev) |
| **Date** | 2026-09-16 |
| **Architecture specs** | [ARCHITECTURE.md](ARCHITECTURE.md) |
| **Design tokens** | [DESIGN.md](DESIGN.md) |
| **Diagrams** | [DIAGRAMS.md](DIAGRAMS.md) |

> **Aturan dokumen:** PRD = apa & kenapa. DB schema, DDL, endpoint path, algoritma dispatcher, folder tree di ARCHITECTURE.md. Token visual di DESIGN.md. Metrik kuantitatif merujuk kode N (DECISIONS.md Seksi 7). Jangan ulang angka dengan nilai beda.

---

## 1. Problem Statement

Orkestrasi fleet AI agent skala tim dan produksi saat ini menghadapi krisis operasional dan visibilitas. Tim engineering menjalankan puluhan agen otonom (coding, research, QA, data scraper) menggunakan wrapper skrip tidak terstruktur atau papan kerja generik (Jira, Linear, Trello) yang buta terhadap konteks komputasi AI.

Masalah inti:
1. **Buta Biaya per Run** — Task bisa habis $0,05 atau membengkak $12 karena tool call loop, tagihan baru sadar akhir bulan di konsol OpenAI/Anthropic.
2. **Absen Approval Gate sebagai State Kelas Satu** — Aksi destruktif (commit ke master, migrasi DB) cuma andalkan flag CLI; kalau terminal tutup, agen mati tanpa jejak.
3. **Tidak Ada Replay Trace Terstruktur** — Agen gagal langkah ke-14 setelah 18 menit — operator cuma punya log teks ribuan baris tanpa metadata tool call.
4. **Isolasi Multi-Tenant Nihil** — Hermes Kanban dkk pakai SQLite lokal, tidak untuk multi-org, multi-project, RBAC.

Korban:
- **Rangga (Platform Lead)** — Tagihan membengkak karena agent looping tanpa circuit breaker.
- **Dian (Engineering Lead)** — Review PR tanpa visibilitas attempt count, tool call, aksi sampingan.
- **Faris (FinOps)** — Tidak bisa bebankan biaya per proyek karena tidak ada ledger micro-USD.

`ASSUMPTION:` Tim AI 5-20 agent membuang 15-30% anggaran pada loop kegagalan yang dapat dicegah (estimasi anekdot forum).

---

## 2. Goals & Non-Goals

### Goals

| ID | Goal | Outcome | Ukuran |
|---|---|---|---|
| G1 | Visibilitas finansial granular per Run/Step | Cost ledger micro-USD integer, atribusi 100% ke task/board/project | Setiap Run punya `cost_micros` != 0 |
| G2 | Tata kelola otonomi adaptif | Approval gate status kelas satu (`awaiting_approval`) + timeout + audit log | Semua aksi berisiko lewat approval |
| G3 | Observabilitas replay & trace | Timeline hierarkis Step → tool call → payload, 1 klik di Run detail | Timeline bisa dibuka < 1 detik |
| G4 | Ketahanan kegagalan cerdas | failure_kind + retry backoff eksponensial | Failure transient pulih >80% otomatis |
| G5 | Isolasi multi-tenant, transparan bagi pengguna solo | Filter org_id di SEMUA query, diverifikasi test; pengguna solo tidak pernah dipaksa berinteraksi dengan konsep org | US-AD07 — User A tidak lihat data Org B; pengguna solo menyelesaikan alur inti tanpa membuka settings org |
| G6 | Biaya hosting rendah | ≤ $10/bulan (N13) | Cek billing |
| G7 | Dwibahasa EN/ID | Full i18n + fallback aman | Tidak ada string keras di komponen |
| G8 | Performa responsif | P95 baca ≤ 150ms (N1), first paint ≤ 400ms (N2) | Grafana |

### Non-Goals

| ID | Bukan | Alasan |
|---|---|---|
| NG1 | Hosting LLM / inference engine sendiri | AgentDeck = orkestrasi, bukan inference |
| NG2 | Visual programming node canvas | Focus board, bukan workflow editor ala Make |
| NG3 | Mobile native apps | Desktop-first, CSS responsive cukup |
| NG4 | Payment gateway publik | Ledger internal; kalau jadi produk, PRD terpisah |
| NG5 | In-browser IDE / terminal emulator | Cukup log stream + artifact viewer |
| NG6 | CRDT / multi-kursor | Snapshot locking, bukan kolaborasi realtime |
| NG7 | Git push otonom tanpa kredensial | Agent pakai token delegate eksplisit |
| NG8 | SLA multi-region | Single-region Fly.io untuk v0.1 |
| NG9 | Enterprise SSO SAML/SCIM | Argon2id + session + API key cukup |
| NG10 | 3D di kanvas utama | 3D = modul opsional terpisah |

---

## 3. Personas

### P1 — Rangga, "Platform & AI Infrastructure Lead"
- **Konteks:** 35 agent 24/7, 4 tim produk dilayani.
- **Job to be Done:** Kuota harian, blast-radius, pagu anggaran fleet.
- **Workaround:** Dashboard OpenAI + skrip cron. Gagal: siapa yang loop-lewat limit.
- **Sukses:** 1 konsol — matikan agent saat limit, pantau health.
- **Level:** Sangat tinggi.

### P2 — Dian, "Engineering Manager & Quality Gatekeeper"
- **Konteks:** 40+ PR/minggu dari coding agent, harus review aksi berisiko.
- **Job to be Done:** Approve/reject aksi agent sebelum sentuh produksi.
- **Workaround:** Slack mentah — aksi berbahaya terlewat tanpa penahan.
- **Sukses:** Kartu approval + diff preview + approve/reject 1 klik.
- **Level:** Menengah-tinggi.

### P3 — Faris, "FinOps & Technical Ops"
- **Konteks:** Alokasi anggaran cloud/AI antar departemen.
- **Job to be Done:** Atribusi biaya token per proyek, proyeksi bulanan.
- **Workaround:** CSV + spreadsheet manual. 2 hari kerja.
- **Sukses:** Laporan biaya real-time per board, presisi micro-USD.
- **Level:** Menengah.

### P4 — Siti, "Autonomous AI Application Developer"
- **Konteks:** Build multi-step agent chain untuk ekstraksi + review.
- **Job to be Done:** Debug alur gagal di tengah tanpa restart.
- **Workaround:** Scroll ribuan baris log — input/output tool cuma teks.
- **Sukses:** Trace visual hierarkis — step gagal merah, payload rapi.
- **Level:** Tinggi.

### P5 — Gilang, "Solo Builder" (B2C — segmen utama masuk)
- **Konteks:** Developer individu, jalanin 5-20 agent otonom di mesin sendiri.
  Tidak punya tim, tidak punya procurement, tidak mau ngurus konsep organisasi.
- **Job to be Done:** Tahu tiap run habis berapa, dan bisa nahan aksi agent
  sebelum ngerusak sesuatu.
- **Workaround:** Log terminal + tagihan provider di akhir bulan. Gagal: baru
  sadar keputusan mahal setelah uangnya keluar.
- **Sukses:** Daftar sendiri, jalan dalam < 10 menit, biaya kelihatan per step.
- **Level:** Tinggi (teknis), rendah (administratif).
- **Catatan:** P5 adalah pintu masuk produk. Kolaborasi (P1-P3) adalah fitur
  yang menyusul, bukan syarat untuk memakai.

---

## 4. Core User Journeys

### J1 — Daftar Org, Project, Agent (Happy Path)
1. Rangga daftar akun → Argon2id → Org baru + `owner` role.
2. Buat Project "Core Migration" → Board kanban "Sprint 1".
3. Daftar Agent: nama GoMigrator, provider=anthropic, model=claude-3-5-sonnet, timeout=4h (N9), retry=transient_only.
4. Agent siap dengan API Key `adk_...`.

### J2 — Task Backlog → Done (Happy Path)
1. Dian buat Task → validasi → `backlog` → deps kelar → `ready`.
2. Dispatcher tick 2s (N19) → batch claim SKIP LOCKED (N20) → `running`.
3. Worker heartbeat 60s (N7) → tiap aksi = Step + ledger entry.
4. Selesai → upload artifact R2 → `review` → approve → `done`.

### J3 — Approval Gate (HITL)
1. Agent detect butuh hapus tabel DB dev. Panggil endpoint hold.
2. Task `running` → `awaiting_approval`. Freeze timer. Record `approvals` pending, expires=24h (N23).
3. SSE notif ke Dian. Buka drawer → preview_json → Approve.
4. Task `running` lagi.

### J4 — Retry Transient
1. Worker 502 dari provider LLM → failure_kind=transient.
2. Run outcome=failed. Cek retry=transient_only, max_attempts=3.
3. Task → `ready` + backoff (2s, 8s, 32s).
4. Attempt 2 sukses.

### J5 — Hard Stop Budget
1. Agent research loop — tiap token dicatat ke ledger.
2. Run cost cap $2 (N17) tercapai. Dispatcher tolak step berikutnya.
3. Run outcome=budget_exceeded. Task → `blocked` (block_kind=budget).
4. Event `budget.threshold_crossed` → SSE + webhook ke Faris.

### J6 — Reclaim Worker Mati
1. Worker OOM-killed. Heartbeat 60s (N7) berhenti.
2. Stale detect gap > 15 menit (N8). Claim dibatalkan.
3. Run outcome=reclaimed. Task → `ready`.

### J7 — Approval Expired
1. Task `awaiting_approval`. Dian tidak respon 24h (N23).
2. Cron detect expires_at < NOW() dan status pending.
3. Approval → expired. Task → blocked (needs_input).

### J8 — Trace Replay
1. Siti buka Run detail gagal → timeline Step vertikal.
2. Step 14 merah → klik → lihat LLM prompt + completion + tool payload.
3. Download artifact dari R2 untuk reproduksi lokal.

---

## 5. Scope & Priorities

| Fitur | Prioritas | Milestone |
|---|---|---|
| Multi-tenant auth & RBAC | Must | M0 |
| **B2C: onboarding tanpa setup org** | **Must** | **M0** |
| **Reset password & profil akun** | **Must** | **M0** |
| **Cost rail (panel biaya sisi kanan)** | **Must** | **M2** |
| **Halaman docs & pricing** | **Should** | **M6** |
| **Penutup akun sendiri** | **Should** | **M6** |
| Project & kanban board core | Must | M1 |
| Atomic dispatcher engine (SKIP LOCKED, heartbeat, reclaim) | Must | M1 |
| Dependency DAG engine | Must | M2 |
| Cost ledger & micro-USD | Must | M2 |
| Provider API key management (enkripsi+rotasi) | Must | M2 |
| Human-in-the-loop approval | Must | M3 |
| Realtime event stream SSE | Must | M3 |
| Failure taxonomy & auto-retry | Must | M4 |
| Artifact storage R2 | Must | M4 |
| Dwibahasa EN/ID | Must | M4 |
| Audit log & API keys | Must | M5 |
| Webhook delivery | Should | M5 |
| Dense data table view | Should | M5 |
| Command palette Cmd+K | Could | M6 |
| Ekspor FinOps CSV | Should | M6 |
| Bulk actions | Could | M6 |
| Visual 3D chibi opsional | Could | Post-launch |
| Enterprise SAML SSO | Won't | v2.0 |

## 6. User Stories & Acceptance Criteria

**US-AD01** — Registrasi akun baru `Must` · `M0`
Sebagai pengunjung, saya ingin mendaftar akun dengan email dan password, sehingga saya dapat mulai menggunakan AgentDeck tanpa perlu menyiapkan apa pun lebih dulu.
- [ ] AC1 Registrasi dengan email valid + password minimum 8 karakter mengembalikan 201, session token dikirim, dan workspace personal dibuat otomatis (transparan — pengguna tidak melihat form organisasi).
- [ ] AC2 (jalur gagal) Registrasi dengan email yang sudah terdaftar mengembalikan 409 dan tidak mengubah data.
- [ ] AC3 (permission) Endpoint registrasi tidak memerlukan token — endpoint publik.
- [ ] AC4 (idempotency) Dua request registrasi identik berturut-turut: yang pertama 201, yang kedua 409 (email sudah terpakai).
- [ ] AC5 (B2C) Setelah registrasi, pengguna dapat membuat project dan board pertama tanpa pernah membuka halaman settings organisasi.
- [ ] AC6 (jalur gagal) Registrasi tanpa mengirim nama tetap berhasil; `name` diisi dari bagian lokal email, bukan ditolak 400.

**US-AD02** — Login dan logout `Must` · `M0`
Sebagai pengguna terdaftar, saya ingin login dengan kredensial saya, sehingga saya mendapat session.
- [ ] AC1 Login dengan kredensial benar mengembalikan 200 + `Set-Cookie` session token (hash di DB, bukan JWT).
- [ ] AC2 (jalur gagal) Login dengan password salah 5x berturut-turut mengunci akun selama 15 menit.
- [ ] AC3 (permission) Endpoint `POST /api/v1/auth/login` publik; `POST /api/v1/auth/logout` butuh session valid.
- [ ] AC4 Session expired otomatis setelah 7 hari tanpa aktivitas (sliding window 24 jam).
- [ ] AC5 Halaman login menampilkan: field email, field password, tombol utama "Log in", dan tautan "Lupa password?". Tidak ada tombol login sosial.
- [ ] AC6 (jalur gagal) Kredensial salah menampilkan pesan galat inline di bawah field, bukan halaman galat terpisah; setelah 5 percobaan, muncul pesan penguncian 15 menit dengan sisa waktu.

**US-AD03** — Manajemen organisasi (CRUD) `Should` · `M0`
Sebagai owner, saya ingin mengelola ruang kerja dan berganti nama, sehingga saya bisa menyesuaikannya — opsional, karena ruang kerja sudah dibuat otomatis saat registrasi.
- [ ] AC1 Membuat Org mengembalikan 201 + slug unik. Pembuat otomatis menjadi `owner`.
- [ ] AC2 (permission) Hanya `owner` yang bisa mengganti nama Org; `admin` dan `member` dapat 403.
- [ ] AC3 (jalur gagal) Menghapus Org dengan project aktif di dalamnya mengembalikan 409.
- [ ] AC4 (B2C) Pengguna dengan satu ruang kerja (kasus umum solo) dapat menyelesaikan seluruh alur inti tanpa pernah membuka halaman ini.
- [ ] AC5 (B2C) Bila pengguna hanya punya satu ruang kerja, pemilih ruang kerja tidak ditampilkan di top bar (tidak ada dropdown berisi satu item).

**US-AD04** — Manajemen anggota & role `Should` · `M0`
Sebagai owner/admin, saya ingin mengundang anggota dan mengubah role mereka.
- [ ] AC1 Undangan mengirim email (dummy) + mencatat `memberships` dengan role yang ditentukan.
- [ ] AC2 (permission) `viewer` mendapat 403 di endpoint manajemen anggota.
- [ ] AC3 (jalur gagal) Mengubah role `owner` terakhir ke `member` ditolak (minimal satu owner harus ada).

**US-AD05** — Session logout paksa (server-side) `Could` · `M5`
Sebagai admin, saya ingin mencabut session pengguna lain dari jarak jauh.
- [ ] AC1 Mencabut session langsung menghapus baris `sessions`; request berikutnya dengan token itu mendapat 401.
- [ ] AC2 (permission) Hanya `owner` dan `admin` yang bisa mencabut session anggota lain.
- [ ] AC3 (jalur gagal) Mencabut session sendiri via endpoint cabut paksa: endpoint menolak (gunakan logout biasa).

**US-AD06** — API Key (CRUD) `Must` · `M5`
Sebagai anggota, saya ingin membuat API Key untuk integrasi programatik.
- [ ] AC1 Membuat key mengembalikan prefix 8 karakter + token `adk_...` panjang (sekali tampil). Hash key disimpan.
- [ ] AC2 (jalur gagal) Membuat key saat batas maksimum key aktif per org tercapai mengembalikan 409.
- [ ] AC3 (permission) `viewer` mendapat 403 di endpoint API keys.

**US-AD07** — Isolasi data antar Org (keamanan inti) `Must` · `M0`
Sebagai anggota Org A, saya tidak bisa mengakses data Org B.
- [ ] AC1 Request `GET /api/v1/projects` dengan session Org A tapi header `X-Org-ID: <org_b_id>` mengembalikan 403 (bukan daftar project Org B).
- [ ] AC2 Semua query SQL di backend memfilter `WHERE org_id = $1` — diverifikasi via code review.
- [ ] AC3 (jalur gagal) Percobaan akses lintas Org mengembalikan 404 (bukan 403) agar keberadaan resource tidak bocor.

**US-AD08** — Membuat project dalam Org `Must` · `M1`
Sebagai anggota, saya ingin membuat project di dalam Org saya.
- [ ] AC1 Membuat project mengembalikan 201 dengan `slug` unik dalam satu Org.
- [ ] AC2 (jalur gagal) Membuat project dengan slug yang sudah dipakai di Org yang sama mengembalikan 409.
- [ ] AC3 (permission) Hanya `owner` dan `admin` dapat membuat project; `member`/`viewer` mendapat 403.
- [ ] AC4 (B2C) Pembuatan project tidak memerlukan pemilihan organisasi; ruang kerja aktif dipakai otomatis.

**US-AD09** — Membuat board kanban `Must` · `M1`
Sebagai anggota, saya ingin membuat board di dalam project, dengan kolom default `Backlog`, `Ready`, `Running`, `Review`, `Done`.
- [ ] AC1 Membuat board mengembalikan 201 + kolom default tersebut. `columns_json` menyimpan urutan dan nama kolom.
- [ ] AC2 (jalur gagal) Nama board duplikat dalam satu project mengembalikan 409.
- [ ] AC3 (permission) Hanya `owner` dan `admin` dapat membuat board; `member`/`viewer` mendapat 403.
- [ ] AC4 (B2C) Pembuatan board tidak memerlukan pemilihan organisasi maupun project baru bila pengguna belum punya project.

**US-AD10** — Mengedit kolom board `Must` · `M1`
Sebagai anggota, saya ingin menambah, menghapus, dan mengurutkan ulang kolom board.
- [ ] AC1 Menambah kolom baru mengembalikan 200; kolom baru muncul di urutan terakhir.
- [ ] AC2 Menghapus kolom yang berisi task mengembalikan 409 (pindahkan task dulu).
- [ ] AC3 (jalur gagal) Mengubah nama kolom `Running` menjadi nama yang sudah ada: 409.
- [ ] AC4 (permission) Mengubah kolom board memerlukan minimal `admin`; `member`/`viewer` mendapat 403.
- [ ] AC5 (B2C) Mengubah kolom tersedia untuk pengguna tunggal tanpa memerlukan peran `admin`.

**US-AD11** — Membuat task `Must` · `M1`
Sebagai anggota, saya ingin membuat task baru di board, status awal `backlog`.
- [ ] AC1 Membuat task dengan title, body, priority, dan assignee_agent_id opsional → status `backlog`.
- [ ] AC2 (idempotency) `Idempotency-Key` mencegah duplikasi: request kedua mengembalikan 200 + data yang sama, bukan 201.
- [ ] AC3 (jalur gagal) Title kosong mengembalikan 400.
- [ ] AC4 (permission) `member` dan `admin` dapat membuat task; `viewer` mendapat 403.
- [ ] AC5 (B2C) Membuat task tidak memerlukan pemilihan assignee; task tanpa agent tetap valid dan berstatus `backlog`.

**US-AD12** — Drag task antar kolom `Must` · `M1`
Sebagai anggota, saya ingin menarik kartu task ke kolom lain, sehingga status berubah.
- [ ] AC1 Drag task dari `backlog` ke `ready` → status `ready`. Event `task.status_changed` tercatat.
- [ ] AC2 Status yang tidak legal secara state machine (langsung dari `backlog` ke `done`) ditolak 422.
- [ ] AC3 (jalur gagal) Drag task `running` yang punya Run aktif → 409 (selesaikan atau fail-kan dulu).
- [ ] AC4 (permission) `member` dapat memindahkan task; `viewer` mendapat 403 dan kartu tidak bergerak.

**US-AD13** — Task detail drawer `Must` · `M1`
Sebagai anggota, saya ingin mengklik kartu task dan melihat detail lengkap di panel samping.
- [ ] AC1 Drawer menampilkan: title, body, status, priority, assignee agent, dependencies, recent events (5 terakhir), total cost.
- [ ] AC2 Drawer memiliki tab: Timeline, Logs, Artifacts, Approvals.
- [ ] AC3 (jalur gagal) Task ID yang tidak ada atau di luar org pengguna mengembalikan 404 tanpa membocorkan keberadaan task.
- [ ] AC4 (permission) `viewer` hanya dapat melihat drawer; tombol aksi mutasi tidak dirender.

**US-AD14** — Assign task ke agent `Must` · `M1`
Sebagai anggota, saya ingin menetapkan task ke agent tertentu.
- [ ] AC1 Assign agent yang valid → field `assignee_agent_id` terisi, event `task.assigned`.
- [ ] AC2 (jalur gagal) Assign agent dari Org lain → 403.

**US-AD15** — Filter task berdasarkan status dan assignee `Must` · `M1`
Sebagai anggota, saya ingin memfilter task di board berdasarkan status dan agent.
- [ ] AC1 Query parameter `?status=running&agent_id=x` hanya mengembalikan task yang cocok.
- [ ] AC2 Kombinasi beberapa filter diterapkan sebagai AND; hasil kosong bukan error (200 + array kosong).
- [ ] AC3 (jalur gagal) Filter dengan nilai enum tidak dikenal mengembalikan 400, bukan hasil kosong.
- [ ] AC4 (jalur gagal) Pencarian task dengan query kosong mengembalikan 400.
- [ ] AC5 (permission) Filter hanya mengembalikan task dalam org pengguna; percobaan lintas org diabaikan tanpa membocorkan keberadaan task.

**US-AD16** — Cari task `Should` · `M1`
Sebagai anggota, saya ingin mencari task berdasarkan title.
- [ ] AC1 Pencarian teks pada title menggunakan `ILIKE` mengembalikan kecocokan partial.

**US-AD17** — Priority task (urgent, high, medium, low) `Should` · `M1`
Sebagai anggota, saya ingin menetapkan prioritas pada task.
- [ ] AC1 Task default prioritas `medium`. Urgent task mendapat border merah di board.

**US-AD18** — Dependency DAG antar task `Must` · `M2`
Sebagai anggota, saya ingin menetapkan bahwa sebuah task bergantung pada task lain.
- [ ] AC1 Menambah dependency parent-child → record `task_links`, child tidak bisa `ready` sebelum parent `done`.
- [ ] AC2 (jalur gagal) Menambah dependency yang membentuk siklus (A→B→A) → 422.
- [ ] AC3 (jalur gagal) Menambah dependency ke task yang sudah `done` → 409 (dependency masa lalu tidak berguna).
- [ ] AC4 (permission) Menambah/menghapus dependency memerlukan minimal `member`; `viewer` mendapat 403.

**US-AD19** — Visualisasi dependency di board `Should` · `M2`
Sebagai anggota, saya ingin melihat garis dependency antar task card di board.
- [ ] AC1 Kartu yang memiliki dependensi menampilkan badge jumlah dependency + garis tipis ke parent.

**US-AD20** — Agent registry (CRUD) `Must` · `M1`
Sebagai anggota, saya ingin mendaftarkan profil agent dengan konfigurasi model, provider, tools, dan max_runtime.
- [ ] AC1 Membuat agent mengembalikan 201. Field: name, provider, model, reasoning_effort, skills_json, tools_json, max_runtime_seconds, retry_policy, max_attempts.

- [ ] AC2 (permission) `viewer` mendapat 403 di endpoint agent registry.

- [ ] AC3 (jalur gagal) Nama agent duplikat dalam satu project mengembalikan 409.

- [ ] AC4 (jalur gagal) Menghapus agent yang masih memegang task `running` mengembalikan 409 dan tidak mengubah data.
- [ ] AC5 (B2C) Mendaftarkan agent pertama tidak memerlukan kredensial provider; agent dibuat dalam keadaan belum siap dan dapat dilengkapi kemudian.


**US-AD21** — Dispatcher mengklaim task `Must` · `M1`
Sebagai dispatcher, saya ingin mengambil task `ready` secara atomik dan mengubahnya ke `running`.
- [ ] AC1 Query `UPDATE tasks SET status='running' WHERE id IN (SELECT id FROM tasks WHERE status='ready' FOR UPDATE SKIP LOCKED LIMIT 20)` mengembalikan task yang diklaim.
- [ ] AC2 Konkurensi: 10 dispatcher paralel tidak saling mengklaim task yang sama.
- [ ] AC3 (jalur gagal) Tidak ada task `ready` → dispatcher tidak melakukan apa-apa (query kosong).
- [ ] AC4 (permission) Claim hanya dilakukan oleh worker terautentikasi (API key valid); request tanpa kredensial ditolak 401.

**US-AD22** — Run lifecycle: claim → finish `Must` · `M1`
Sebagai worker, saya ingin mencatat Run saat mengklaim task dan menutupnya saat selesai.
- [ ] AC1 Mengklaim task membuat record `runs` dengan status `running`, `claim_lock` diisi.
- [ ] AC2 Worker menyelesaikan → `runs.outcome='succeeded'`, task ke `review`.
- [ ] AC3 (jalur gagal) Worker yang kehilangan klaim (heartbeat basi) tidak bisa menutup run; request ditolak 409.
- [ ] AC4 (permission) Perpindahan status run hanya dapat dilakukan oleh worker pemegang klaim (`claim_lock`); worker lain ditolak.

**US-AD23** — Heartbeat berkala `Must` · `M1`
Sebagai worker, saya harus mengirim heartbeat setiap 60 detik (N7) agar task tidak di-reclaim.
- [ ] AC1 Heartbeat memperbarui `last_heartbeat_at`. Jika gap > 15 menit (N8), task dianggap stale.
- [ ] AC2 (jalur gagal) Task tanpa heartbeat dalam 15 menit di-reclaim, Run outcome `reclaimed`.

**US-AD24** — Reclaim task stale `Must` · `M1`
Sebagai backend, saya ingin mendeteksi dan membatalkan klaim task yang heartbeating berhenti.
- [ ] AC1 Cron setiap 60s mengecek runs dengan `last_heartbeat_at < NOW() - INTERVAL '15 menit'`.
- [ ] AC2 Task yang di-reclaim kembali ke `ready`, `consecutive_failures` naik 1, `current_run_id` di-null-kan.
- [ ] AC3 (jalur gagal) Reclaim tidak menyentuh run yang sudah `ended`; operasi idempoten tanpa efek ganda.
- [ ] AC4 (permission) Reclaim hanya dapat dipicu oleh dispatcher internal atau `admin`; user lain mendapat 403.

**US-AD25** — Menulis step (trace) dalam run `Must` · `M2`
Sebagai worker, saya ingin mencatat setiap langkah eksekusi (LLM call, tool call, shell) sebagai Step.
- [ ] AC1 Setiap step menyimpan: `seq`, `kind`, `name`, `tokens_in`, `tokens_out`, `cost_micros`, `payload_json`.
- [ ] AC2 (jalur gagal) Step gagal → `status='failed'`, step berhenti, run dilanjutkan ke failure handling.
- [ ] AC3 (permission) Menulis step hanya sah bagi worker pemegang klaim run aktif; step dari run yang sudah selesai ditolak.

**US-AD26** — Melihat timeline step per run `Must` · `M3`
Sebagai pengguna, saya ingin melihat timeline hierarkis step dalam sebuah run.
- [ ] AC1 Timeline menampilkan step berurutan, warna hijau = sukses, merah = gagal.
- [ ] AC2 Klik step → drawer menampilkan prompt, completion, tool call payload (dari `payload_json`).
- [ ] AC3 (jalur gagal) Step dengan `payload_json` kosong menampilkan panel kosong bertanda jelas, bukan error.
- [ ] AC4 (permission) `viewer` dapat melihat timeline; `viewer` tidak dapat memicu retry dari halaman ini.

**US-AD27** — Cost ledger: mencatat pemakaian token per step `Must` · `M2`
Sebagai worker, saya ingin mencatat token dan biaya step ke `ledger_entries`.
- [ ] AC1 Setiap step yang selesai menulis record `ledger_entries` dengan `tokens_in`, `tokens_out`, `cost_micros`, `price_version`.
- [ ] AC2 Biaya dalam micro-USD integer — tidak ada float. 1 USD = 1.000.000 micro-USD.
- [ ] AC3 (jalur gagal) Step dengan `tokens_in`/`tokens_out` negatif ditolak; `cost_micros` tidak pernah negatif.
- [ ] AC4 Halaman ledger menampilkan tabel padat per baris ledger: waktu, agent, model, token masuk, token keluar, cache read/write, biaya, dan versi harga. Angka memakai font monospace dengan tabular figures.
- [ ] AC5 (jalur gagal) Bila tidak ada baris pada rentang filter, halaman menampilkan empty state dengan ajakan mengubah rentang tanggal, bukan tabel kosong.

**US-AD28** — Agregasi biaya harian per board `Must` · `M2`
Sebagai sistem, saya ingin menghitung total biaya board hari ini untuk guardrail budget.
- [ ] AC1 Query `SELECT SUM(cost_micros) FROM ledger_entries JOIN tasks ON ... WHERE board_id = $1 AND created_at >= today` mengembalikan total.
- [ ] AC2 Hasil agregasi < 15 ms untuk board dengan 10.000 ledger entries hari ini.
- [ ] AC3 (jalur gagal) Run yang melewati hard stop (N17) ditandai `budget_exceeded`, bukan `succeeded`.

**US-AD29** — Budget guardrail: hard stop per run ($2) `Must` · `M2`
Sebagai dispatcher, saya ingin menghentikan run yang melebihi batas biaya $2 (N17).
- [ ] AC1 Setiap step baru: cek `SUM(cost_micros) + step.cost_micros <= N17` → jika tidak, tolak.
- [ ] AC2 Run outcome = `budget_exceeded`, task → `blocked` (block_kind=budget).
- [ ] AC3 (jalur gagal) Board dengan pagu harian terlewati menolak claim baru; task tetap `ready`.
- [ ] AC4 (permission) Hanya `owner`/`admin` dapat mengubah hard stop per run; `member` mendapat 403.

**US-AD30** — Budget harian board ($20) `Must` · `M2`
Sebagai dispatcher, saya ingin mencegah task baru di board yang sudah melebihi pagu harian $20 (N16).
- [ ] AC1 Sebelum claim task: cek total biaya board hari ini. Jika ≥ N16, claim ditolak, task tetap `ready`.
- [ ] AC2 (jalur gagal) Ketika pagu tercapai, event `budget.threshold_crossed` dikirim.
- [ ] AC3 (permission) Hanya `owner`/`admin` dapat mengubah pagu harian board.

**US-AD31** — Alert budget 80% `Must` · `M2`
Sebagai sistem, saya ingin mengirim peringatan ketika biaya harian mencapai 80% dari N16.
- [ ] AC1 Threshold `0.8 * N16` → jika tercapai, event `budget.threshold_crossed` dengan payload `"level":"warning"`.
- [ ] AC2 Alert bersifat idempoten per hari per board: crossing kedua di hari yang sama tidak mengirim ulang.
- [ ] AC3 (jalur gagal) Jika pagu harian board belum diset, tidak ada alert dan tidak ada error 500.
**US-AD32** — Melihat total biaya task di card `Must` · `M2`
Sebagai pengguna, saya ingin melihat biaya total task di kartu board.
- [ ] AC1 Kartu menampilkan badge biaya (format: `$0.04` atau `$1.23`).
- [ ] AC2 Badge berwarna hijau jika < $0.50, kuning $0.50-$1.50, merah > $1.50.
- [ ] AC3 (jalur gagal) Rollup untuk hari tanpa data menghasilkan baris nol, bukan error.
- [ ] AC4 (permission) Total biaya hanya ditampilkan untuk task dalam org pengguna; `viewer` melihat angka, tidak melihat detail per-step.

**US-AD33** — Approval gate: request approval `Must` · `M3`
Sebagai worker, saya ingin mengirim permintaan approval dengan preview aksi.
- [ ] AC1 Panggil endpoint approval → task `running` → `awaiting_approval`, record `approvals` dengan `gate_mode='require'`.
- [ ] AC2 (jalur gagal) Request approval tanpa `preview_json` → 400.
- [ ] AC3 (permission) Approval request dibuat oleh sistem/worker; `viewer` tidak dapat membuat approval manual.

**US-AD34** — Approval: approve `Must` · `M3`
Sebagai approver, saya ingin menyetujui aksi agent.
- [ ] AC1 Approve → `approvals.decision='approved'`, `decided_by` terisi, task kembali `running`.
- [ ] AC2 (jalur gagal) Approve dua kali pada approval yang sudah `decided` mengembalikan 409.
- [ ] AC3 (permission) Hanya `owner`/`admin`/`member` Org yang bisa approve; `viewer` 403.

**US-AD35** — Approval: reject `Must` · `M3`
Sebagai approver, saya ingin menolak aksi agent.
- [ ] AC1 Reject → `approvals.decision='rejected'`, task → `blocked` (block_kind=needs_input).
- [ ] AC2 Alasan penolakan wajib diisi (field `reason`).
- [ ] AC3 (jalur gagal) Menolak approval yang sudah kedaluwarsa (`expired`) mengembalikan 409.
- [ ] AC4 (permission) Hanya `owner`/`admin` dapat mereject approval; `member` dan `viewer` mendapat 403.

**US-AD36** — Approval: expired otomatis `Must` · `M3`
Sebagai sistem, approval yang tidak direspons dalam 24 jam (N23) harus kedaluwarsa.
- [ ] AC1 Cron setiap 5 menit: `UPDATE approvals SET decision='expired' WHERE status='pending' AND expires_at < NOW()`.
- [ ] AC2 Task yang approval-nya expired → `blocked` (block_kind=needs_input). Event `approval.expired`.
- [ ] AC3 (jalur gagal) Approval yang sudah `decided` tidak bisa diputuskan ulang -> 409.
- [ ] AC4 (permission) Expiry dipicu sistem; tidak ada endpoint publik untuk memaksa expire approval milik org lain.

**US-AD37** — Melihat antrean approval `Must` · `M3`
Sebagai approver, saya ingin melihat daftar task yang menunggu approval saya.
- [ ] AC1 Halaman khusus "Approvals" menampilkan semua task `awaiting_approval` di org.
- [ ] AC2 Setiap kartu menampilkan preview aksi, waktu tersisa, dan tombol approve/reject.
- [ ] AC3 (jalur gagal) Memutuskan approval milik org lain mengembalikan 404.
- [ ] AC4 (permission) `member` dan `viewer` dapat melihat antrean; hanya `owner`/`admin` melihat tombol keputusan.

**US-AD38** — Approval gate mode per agent `Must` · `M3`
Sebagai admin, saya ingin menentukan mode approval default per agent: `auto`, `require`, atau `deny`.
- [ ] AC1 Agent dengan mode `auto` → task tidak pernah berhenti di `awaiting_approval`.
- [ ] AC2 Agent mode `deny` → semua aksi berisiko langsung ditolak.
- [ ] AC3 (jalur gagal) Approval yang lewat N23 ditandai `expired`; task terkait masuk `blocked` (`needs_input`).

**US-AD39** — Event stream SSE: perubahan status task `Must` · `M3`
Sebagai pengguna, saya ingin menerima perubahan status task secara realtime.
- [ ] AC1 Koneksi SSE ke `GET /api/v1/events?board_id=x` → menerima event saat task berubah status.

- [ ] AC2 (jalur gagal) Koneksi terputus → reconnecting dengan `Last-Event-ID` → melanjutkan dari event terakhir.

- [ ] AC3 (permission) Stream SSE hanya mengirim event org pengguna; event org lain tidak pernah terkirim.

- [ ] AC4 (jalur gagal) Klien yang tidak membaca stream sampai buffer penuh diputus, bukan memblokir hub SSE.

- [ ] AC5 (jalur gagal) Reconnect tanpa `Last-Event-ID` memulai dari posisi sekarang, tidak melempar error.


**US-AD40** — Timeline event per task `Must` · `M3`
Sebagai pengguna, saya ingin melihat kronologi event sebuah task.
- [ ] AC1 Tab Timeline menampilkan event berurutan (`created`, `status_changed`, `assigned`, `run.claimed`, `step.finished`, `approval.requested`, `approval.decided`, `comment.created`).
- [ ] AC2 Timeline dipaginasi dengan cursor, urutan `created_at DESC`, beban awal maksimum 50 event.
- [ ] AC3 (jalur gagal) Task yang tidak ada atau milik org lain mengembalikan 404, bukan timeline kosong.
**US-AD41** — Halaman detail run `Must` · `M3`
Sebagai pengguna, saya ingin melihat halaman detail run dengan summary dan metadata.
- [ ] AC1 Menampilkan: status, outcome, durasi, total biaya, total token, attempt number.
- [ ] AC2 Tab: Steps (timeline), Logs, Approvals, Artifacts, Ledger.
- [ ] AC3 (jalur gagal) Run yang sudah dihapus atau tidak ada mengembalikan 404, bukan halaman kosong.
- [ ] AC4 (permission) Halaman detail run hanya dapat dibuka untuk run dalam org pengguna (404 jika bukan).

**US-AD42** — Menambahkan komentar ke task `Must` · `M3`
Sebagai anggota, saya ingin meninggalkan komentar pada task.
- [ ] AC1 Komentar tersimpan di `comments` dengan `author_user_id` atau `author_agent_id`.
- [ ] AC2 (permission) `viewer` dapat membaca tetapi tidak dapat menulis komentar.
- [ ] AC3 (jalur gagal) Komentar kosong atau melebihi batas panjang mengembalikan 400.

**US-AD43** — Failue taxonomy: klasifikasi otomatis `Must` · `M4`
Sebagai backend, error dari worker harus diklasifikasikan ke `failure_kind` yang terstandar.
- [ ] AC1 HTTP 5xx / timeout → `transient`. HTTP 403 → `capability`. Task hulu gagal → `dependency`.
- [ ] AC2 `failure_kind` tersimpan di `runs.failure_kind`.
- [ ] AC3 (jalur gagal) Kegagalan tanpa pola yang dikenal diklasifikasikan `unknown` dan tidak dipetakan ke retry otomatis.

**US-AD44** — Retry otomatis berdasarkan failure_kind `Must` · `M4`
Sebagai dispatcher, saya ingin meretry task sesuai kebijakan agent.
- [ ] AC1 `retry_policy=transient_only` dan `failure_kind=transient` → task kembali ke `ready` dengan backoff.
- [ ] AC2 `retry_policy=never` → task langsung `failed` tanpa percobaan ulang; `consecutive_failures` tidak bertambah.
- [ ] AC3 (jalur gagal) Jika jadwal backoff melewati `max_runtime_seconds`, task di-dead-letter, bukan menunggu tanpa batas.
**US-AD45** — Max attempts dan dead letter `Must` · `M4`
Sebagai dispatcher, saya ingin berhenti meretry setelah mencapai batas.
- [ ] AC1 `consecutive_failures >= agents.max_attempts` → task → `failed` (dead letter). Alert dikirim ke `owner`/`admin`.
- [ ] AC2 `max_attempts=1` → kegagalan pertama langsung berstatus `failed`.
- [ ] AC3 (jalur gagal) Dead letter tetap dapat dipindahkan ke `backlog` oleh `admin` untuk penanganan manual.
**US-AD46** — Upload artifact ke R2 `Must` · `M4`
Sebagai worker, saya ingin mengunggah file hasil eksekusi (patch, log, image) ke R2.
- [ ] AC1 Upload → backend menyimpan ke R2, record `artifacts` dengan `sha256`, `size`, `storage_key`.
- [ ] AC2 (jalur gagal) File melebihi 25MB → 413.
- [ ] AC3 (permission) Upload artifact memerlukan kredensial worker atau minimal `member`; `viewer` mendapat 403.

**US-AD47** — Download artifact `Must` · `M4`
Sebagai pengguna, saya ingin mengunduh artifact task.
- [ ] AC1 Klik → presigned URL sementara dari R2 → download.
- [ ] AC2 (permission) Hanya anggota Org task tersebut yang bisa download.
- [ ] AC3 (jalur gagal) Download artifact milik org lain mengembalikan 404.

**US-AD48** — Menampilkan daftar artifact per task `Must` · `M4`
Sebagai pengguna, saya ingin melihat daftar file yang dihasilkan task.
- [ ] AC1 Tab Artifacts menampilkan: nama, tipe, ukuran, dan waktu upload.
- [ ] AC2 Daftar dipaginasi (cursor); task dengan lebih dari 50 artifact menampilkan halaman berikutnya.
- [ ] AC3 (jalur gagal) Presigned URL yang kedaluwarsa ditolak oleh storage, bukan oleh API.
- [ ] AC4 (permission) `viewer` dapat melihat daftar artifact, tetapi URL unduh ditandatangani per-request dan hanya untuk anggota org task tersebut.

**US-AD49** — UI dwibahasa EN/ID `Must` · `M4`
Sebagai pengguna, saya ingin mengganti bahasa UI antara Inggris dan Indonesia.
- [ ] AC1 Semua teks UI inti, label status, pesan error, tombol, dan tooltip memiliki terjemahan.
- [ ] AC2 (jalur gagal) String yang belum diterjemahkan menampilkan fallback language (EN) atau key — tidak pernah blank.

**US-AD50** — Deteksi string keras (hardcoded) di CI `Must` · `M4`
Sebagai developer, saya ingin CI menolak komponen yang berisi string teks tanpa wrapper i18n.
- [ ] AC1 Script CI `check-i18n.js` men-scan semua file `.tsx` dan mendeteksi string literal > 3 karakter di luar `<Trans>` atau `t()`. Script gagal jika ada temuan.
- [ ] AC2 (jalur gagal) CI gagal (exit non-zero) begitu satu string keras terdeteksi; build tidak lanjut.

**US-AD51** — Audit log: mencatat mutasi administratif `Must` · `M5`
Sebagai sistem, setiap perubahan administratif (ubah role, hapus project, cabut API key) dicatat di `audit_log`.
- [ ] AC1 Record `audit_log` menyimpan actor, target, before/after JSON, IP, timestamp.
- [ ] AC2 (jalur gagal) Kegagalan penulisan audit log membatalkan mutasi yang sedang diproses (fail-closed).
- [ ] AC3 (permission) Hanya `owner` dan `admin` yang bisa membaca audit log org.

**US-AD52** — Webhook: daftar webhook per board `Should` · `M5`
Sebagai admin, saya ingin mendaftarkan webhook URL yang dipanggil saat event tertentu terjadi.
- [ ] AC1 CRUD webhook dengan field: url, secret, events_json (daftar event_kind yang dipantau).
- [ ] AC2 Webhook di-signing dengan HMAC-SHA256 menggunakan secret.

**US-AD53** — Webhook delivery dan retry `Should` · `M5`
Sebagai sistem, setiap event yang cocok dikirim ke webhook dengan retry 3 kali.
- [ ] AC1 Pengiriman HTTP POST dengan body JSON + header `X-Hub-Signature-256`.
- [ ] AC2 (jalur gagal) Gagal 3x → webhook_deliveries.status='failed'. Event tidak hilang.

**US-AD54** — Tampilan data table (list view) `Should` · `M5`
Sebagai pengguna, saya ingin melihat task dalam format tabel padat sebagai alternatif board.
- [ ] AC1 Tabel 32px/baris, kolom: ID, Title, Status, Priority, Agent, Cost, Created.
- [ ] AC2 Multi-sort: klik header kolom untuk sorting asc/desc.

**US-AD55** — Command palette (Cmd+K) `Should` · `M6`
Sebagai pengguna, saya ingin membuka command palette dengan shortcut `Cmd+K` dan menavigasi cepat.
- [ ] AC1 Palette menampilkan: navigasi ke project/board, pencarian task, aksi cepat.
- [ ] AC2 Navigasi keyboard: arrow keys + Enter untuk memilih.

**US-AD56** — Ekspor laporan biaya CSV `Should` · `M6`
Sebagai Faris, saya ingin mengekspor rekap biaya per board dalam CSV.
- [ ] AC1 CSV mencakup: Task ID, Run ID, Agent, Model, Tokens In, Tokens Out, Cost.
- [ ] AC2 Filter berdasarkan rentang tanggal.

**US-AD57** — Bulk action: pindahkan task `Could` · `M6`
Sebagai anggota, saya ingin memilih banyak task dan memindahkannya ke kolom lain.
- [ ] AC1 Pilih 5 task → pindah ke `ready` → semua 5 berubah status.
- [ ] AC2 (jalur gagal) Jika salah satu task dalam batch memiliki Run aktif, seluruh batch ditolak 409.

**US-AD58** — Menandai task selesai (done) `Must` · `M1`
Sebagai anggota, saya ingin menandai task sebagai selesai.
- [ ] AC1 Task dari `review` ke `done` → `completed_at` diisi.
- [ ] AC2 (jalur gagal) Task yang masih `running` tidak bisa ditandai `done`.
- [ ] AC3 (permission) `member` dapat menandai done; `viewer` mendapat 403.

**US-AD59** — Task archived `Must` · `M1`
Sebagai anggota, saya ingin mengarsipkan task yang sudah tidak relevan.
- [ ] AC1 Arsip → task status `archived`, tidak muncul di board default (toggle filter `archived`).
- [ ] AC2 Arsip hanya berlaku untuk task terminal (`done`/`failed`/`cancelled`); task aktif mengembalikan 409.
- [ ] AC3 (jalur gagal) Mengarsipkan task yang sudah `archived` bersifat idempoten (200), bukan error.
- [ ] AC4 (permission) Mengarsipkan task memerlukan minimal `admin`; `member`/`viewer` mendapat 403.

**US-AD60** — View toggle: tampilan mobile `Must` · `M1`
Sebagai pengguna mobile, board harus tetap bisa dipakai tanpa horizontal scroll.
- [ ] AC1 Pada viewport < 768px kolom board berubah menjadi accordion vertikal; memilih kolom menampilkan task di kolom itu.
- [ ] AC2 Perpindahan antar kolom pada mobile menyimpan posisi pilihan saat rotasi layar.
- [ ] AC3 (jalur gagal) Pada viewport desktop (>= 768px) tetap memakai tampilan kolom, bukan accordion.
**US-AD61** — Notifikasi in-app `Must` · `M3`
Sebagai pengguna, saya ingin menerima notifikasi saat task yang saya assign berubah status.
- [ ] AC1 Notifikasi di bell icon pojok kanan atas, badge jumlah unread.
- [ ] AC2 Klik notifikasi → navigasi ke task yang dimaksud.
- [ ] AC3 (jalur gagal) Jika kanal realtime putus, notifikasi tetap tersimpan dan tampil saat reconnect.
- [ ] AC4 (permission) Notifikasi hanya berisi data org pengguna; tidak ada kebocoran lintas tenant.

**US-AD62** — Filter tasks berdasarkan tanggal `Should` · `M1`
Sebagai anggota, saya ingin memfilter task yang dibuat dalam rentang tanggal.
- [ ] AC1 Param `?created_after=...&created_before=...` mengembalikan task dalam rentang.

**US-AD63** — Skeleton loading state `Must` · `M3`
Sebagai pengguna, saya melihat skeleton/placeholder saat board sedang load, bukan spinner kosong.
- [ ] AC1 Tiap kolom menampilkan 3 skeleton card abu-abu dengan animasi pulse selama fetching pertama.
- [ ] AC2 Skeleton muncul hanya saat fetch pertama; refetch latar (RTK Query) tidak menampilkan ulang skeleton.
- [ ] AC3 (jalur gagal) Jika fetch gagal, skeleton diganti error state dengan tombol "Coba lagi".
**US-AD64** — Empty state board `Must` · `M1`
Sebagai anggota, saat board kosong saya melihat ilustrasi/teks "Belum ada task" + tombol "Buat task pertama".
- [ ] AC1 Empty state hanya muncul jika board benar-benar kosong; bila ada filter aktif, teksnya berbeda ("Tidak ada task yang cocok dengan filter").
- [ ] AC2 CTA "Create your first task" membuka form task baru dengan kolom `backlog` terpilih.
- [ ] AC3 (jalur gagal) Board dengan error pemuatan menampilkan error state, bukan empty state palsu.

**US-AD65** — Error boundary UI `Must` · `M3`
Sebagai pengguna, error dari server tidak menampilkan layar putih atau error mentah.
- [ ] AC1 Komponen React memiliki error boundary → menampilkan pesan "Terjadi kesalahan" + tombol "Coba lagi".
- [ ] AC2 (jalur gagal) Error boundary menangkap error render anak dan menampilkan fallback tanpa memutihkan seluruh app.

**US-AD66** — Max runtime per task (N9) `Must` · `M1`
Sebagai dispatcher, task yang berjalan melebihi `max_runtime_seconds` harus dihentikan.
- [ ] AC1 Cron setiap 60 detik memeriksa run dengan `started_at + max_runtime_seconds < NOW()`; bila ada, `outcome='timed_out'` dan task kembali ke `ready` bila retry diizinkan.
- [ ] AC2 Run yang dihentikan mencatat `outcome='timed_out'` dan `ended_at`; durasi tercatat di `runs`.
- [ ] AC3 (jalur gagal) Jika retry tidak diizinkan (`retry_policy=never`), task langsung `failed`, bukan `ready`.
- [ ] AC4 (permission) Hanya dispatcher internal yang menegakkan max runtime; user tidak dapat memperpanjang lewat API biasa.

**US-AD67** — Menentukan model dan provider per agent `Must` · `M1`
Sebagai admin, saat mendaftarkan agent saya ingin menentukan provider dan model yang digunakan.
- [ ] AC1 Field `provider` dan `model` wajib diisi; kombinasi tidak dikenal di daftar harga `internal/pricing` ditolak 400.
- [ ] AC2 Kombinasi `provider`+`model` yang tidak ada di tabel harga Go ditolak 400 saat pembuatan agent.
- [ ] AC3 (jalur gagal) Agent tanpa kredensial provider valid ditolak saat claim (`failure_kind='capability'`, `retry_policy='never'`).
- [ ] AC4 (permission) Mengubah `provider`/`model` agent memerlukan `owner`/`admin`; `member`/`viewer` mendapat 403.

**US-AD68** — Provider harga dinamis (price_version) `Must` · `M2`
Sebagai backend, harga model bisa berubah; setiap ledger entry mencatat versi harga yang dipakai.
- [ ] AC1 Kolom `price_version` (integer) tersimpan di `ledger_entries` untuk setiap baris biaya.
- [ ] AC2 `price_version` adalah integer yang naik setiap kali konstanta harga berubah antar rilis binary.
- [ ] AC3 (jalur gagal) Entry ledger ber-`price_version` lama tetap dapat dibaca untuk audit, tanpa dihitung ulang.
**US-AD69** — Menampilkan total biaya project `Should` · `M2`
Sebagai Faris, saya ingin melihat total biaya semua board dalam satu project.
- [ ] AC1 Halaman project menampilkan ringkasan: total biaya hari ini, 7 hari, 30 hari.

**US-AD70** — Proteksi siklus dependency `Must` · `M2`
Sebagai backend, dependency graph harus diuji siklus sebelum ditambahkan.
- [ ] AC1 Saat menambah `task_links`, lakukan DFS/cycle detection. Jika siklus terdeteksi → 422.
- [ ] AC2 (jalur gagal) Graf dengan siklus yang sudah tersimpan (data lama) terdeteksi saat validasi dan ditandai, bukan crash.

**US-AD71** — Comment dengan mention `Should` · `M3`
Sebagai anggota, saya ingin menyebut (`@`) anggota lain di komentar.
- [ ] AC1 Mention `@rangga` mengirim notifikasi ke Rangga.

**US-AD72** — Prepopulate board dari template `Could` · `M6`
Sebagai anggota, saya ingin membuat board dengan kolom dan contoh task dari template.
- [ ] AC1 Pilih template "Software Development" → board dengan kolom Backlog, Ready, In Progress, Review, Done.

**US-AD73** — Menonaktifkan (archive) agent `Must` · `M1`
Sebagai admin, saya ingin menonaktifkan agent agar tidak menerima task baru.
- [ ] AC1 Agent diarsip → tidak muncul di daftar assign; task yang sedang `running` tetap diselesaikan.
- [ ] AC2 Agent yang diarsip tidak dapat di-assign ke task baru; pilihan tidak muncul di dropdown.
- [ ] AC3 (jalur gagal) Mengarsipkan agent yang sedang memegang run aktif mengembalikan 409, bukan memutus run.
- [ ] AC4 (permission) Mengarsipkan agent memerlukan `owner`/`admin`; `member`/`viewer` mendapat 403.

**US-AD74** — Error handling: provider LLM down `Must` · `M4`
Sebagai sistem, saat provider LLM mengembalikan 503, worker harus melaporkan sebagai transient dan retry.
- [ ] AC1 Klasifikasi HTTP 503 → `failure_kind=transient` → retry backoff.
- [ ] AC2 Setelah 3 gagal berturut-turut ke provider yang sama → circuit breaker mental (tunda claim task provider itu 5 menit).
- [ ] AC3 (jalur gagal) Setelah provider pulih, task tidak otomatis jalan kembali tanpa aksi eksplisit (hindari retry storm).
- [ ] AC4 (permission) Notifikasi kegagalan provider hanya dikirim ke `owner`/`admin` org terkait.

**US-AD75** — Workspace management: menentukan workspace_kind `Must` · `M1`
Sebagai admin, saat membuat task saya dapat menentukan jenis workspace (`scratch`, `dir`, `worktree`, `container`).
- [ ] AC1 Task dengan `workspace_kind=worktree` otomatis membuat git worktree pada `path` yang ditentukan.
- [ ] AC2 `workspace_kind=dir` memakai direktori yang sudah ada; `scratch` membuat direktori baru yang dibersihkan setelah run.
- [ ] AC3 (jalur gagal) `path` wajib untuk `dir`/`worktree`; absen `path` mengembalikan 400.
- [ ] AC4 (permission) Menentukan `workspace_kind` dan `path` memerlukan minimal `member`; `viewer` mendapat 403.

**US-AD76** — Halaman dashboard `Should` · `M1`
Sebagai pengguna, saya ingin melihat ringkasan kerja saya: jumlah task per status, total biaya hari ini, agent aktif.
- [ ] AC1 Dashboard menampilkan 4 kartu metrik + grafik biaya 7 hari.
- [ ] AC2 (jalur gagal) Scraper metrik tanpa token yang benar ditolak 401.
- [ ] AC3 (B2C) Untuk pengguna dengan satu ruang kerja, judul halaman memakai nama pengguna, bukan nama organisasi.
- [ ] AC4 (B2C) Dashboard tidak menampilkan seksi khusus kolaborasi (anggota, undangan) bila ruang kerja hanya berisi satu pengguna.


**US-AD77** — Halaman settings org `Must` · `M0`
Sebagai owner, saya ingin mengubah nama dan preferensi org.
- [ ] AC1 Mengganti nama org memperbarui `name`, sedangkan `slug` tetap tidak berubah.
- [ ] AC2 Perubahan preferensi org tercatat di `audit_log` dengan aktor dan nilai sebelum/sesudah.
- [ ] AC3 (jalur gagal) `member` melakukan perubahan settings mengembalikan 403; `viewer` hanya dapat membaca.
**US-AD78** — Invite link `Could` · `M6`
Sebagai owner, saya ingin membagikan link undangan yang kedaluwarsa.
- [ ] AC1 Link undangan valid 7 hari. Setelah didaftarkan, membership aktif.

**US-AD79** — Reassign task ke agent lain `Must` · `M1`
Sebagai anggota, saya ingin mengubah assignee agent task.
- [ ] AC1 Reassign ke agent lain → field `assignee_agent_id` berubah dan event `task.assigned` dipancarkan.
- [ ] AC2 Reassign task yang sedang `running` tidak memutus run aktif; berlaku untuk eksekusi berikutnya.
- [ ] AC3 (jalur gagal) Reassign ke agent yang di-archive mengembalikan 409.
- [ ] AC4 (permission) Reassign task memerlukan minimal `member`; `viewer` mendapat 403.

**US-AD80** — Menghapus task (soft delete) `Must` · `M2`
Sebagai anggota, saya ingin menghapus task. Task yang sudah dihapus dapat dipulihkan dalam 30 hari.
- [ ] AC1 Soft delete → task tidak muncul di board. Admin bisa restore dalam 30 hari.
- [ ] AC2 (permission) Hanya `owner` dan `admin` yang bisa menghapus task.
- [ ] AC3 (jalur gagal) Soft-delete task yang masih punya dependen mengembalikan 409.

**US-AD81** — Filter board berdasarkan kolom `Must` · `M1`
Sebagai anggota, saya ingin melihat satu kolom saja di board (mode fullscreen kolom).
- [ ] AC1 Klik header kolom → board menampilkan hanya kolom itu; "Show all" mengembalikan tampilan penuh.
- [ ] AC2 Mode kolom tunggal hanya mengubah presentasi; filter dan status task tidak berubah.
- [ ] AC3 (jalur gagal) Kolom tanpa task tetap dapat dipilih dan menampilkan empty state kolom.
**US-AD82** — Scroll sinkron antar kolom `Must` · `M1`
Saat kolom memiliki banyak task, scroll vertikal independen per kolom.
- [ ] AC1 Scroll kolom A tidak mempengaruhi posisi scroll kolom B.
- [ ] AC2 Sinkronisasi scroll dapat dimatikan lewat preferensi pengguna dan diingat antar sesi.
- [ ] AC3 (jalur gagal) Kolom dengan tinggi berbeda tetap sinkron tanpa memicu loop scroll.
**US-AD83** — Mengubah nama board `Must` · `M1`
Sebagai anggota, saya ingin mengganti nama board.
- [ ] AC1 Nama baru divalidasi: tidak boleh kosong dan tidak duplikat dalam project yang sama.
- [ ] AC2 Nama baru maksimal 64 karakter; spasi dirapikan (`trim`), nama kosong ditolak 400.
- [ ] AC3 (jalur gagal) Nama duplikat dalam project mengembalikan 409.
- [ ] AC4 (permission) Mengubah nama board memerlukan minimal `admin`; `member`/`viewer` mendapat 403.

**US-AD84** — Menghapus board `Must` · `M1`
Sebagai owner/admin, saya ingin menghapus board beserta semua task di dalamnya.
- [ ] AC1 Semua task di board di-soft-delete.
- [ ] AC2 (jalur gagal) Konfirmasi wajib: checkbox "Saya yakin ingin menghapus N task". Tanpa centang → 200 tapi tidak ada perubahan.
- [ ] AC3 (permission) Menghapus board memerlukan `owner` saja; `admin`/`member`/`viewer` mendapat 403.

**US-AD85** — Rate limit per endpoint `Must` · `M3`
Sebagai sistem, request berlebih harus ditolak dengan 429.
- [ ] AC1 Endpoint publik (login, register): 10 req/menit per IP. Endpoint auth: 100 req/menit per session.
- [ ] AC2 Response 429 menyertakan header `Retry-After`.
- [ ] AC3 (jalur gagal) Rate limit per-IP tidak bisa dilewati dengan mengganti session token.
- [ ] AC4 (permission) Rate limit diterapkan per-identity (session/API key), bukan hanya per-IP.

**US-AD86** — Simpan kredensial provider LLM per agent `Must` · `M2`
Sebagai admin, saya ingin menyimpan API key provider (OpenAI, Anthropic, OpenRouter) untuk setiap agent, sehingga agent dapat mengakses LLM.
- [ ] AC1 Menyimpan kredensial: nilai API key provider dienkripsi AES-256-GCM sebelum disimpan ke kolom `agents.provider_api_key_enc` (BYTEA). Ciphertext + nonce (12B) + tag (16B) disimpan; plaintext tidak pernah di-log maupun dikembalikan API.
- [ ] AC2 (permission) Hanya `owner` dan `admin` yang bisa membaca (memperlihatkan masked `sk-...XXXX`) atau memperbarui kredensial. `member` dan `viewer` mendapat 403.
- [ ] AC3 (jalur gagal) Menyimpan kredensial dengan provider yang tidak dikenal mengembalikan 400.
- [ ] AC4 (rotasi) Memperbarui kredensial menimpa ciphertext lama. Riwayat kredensial tidak disimpan.
- [ ] AC5 Ketika agent di-assign ke task dan API key tidak ada/kosong, dispatcher menolak claim task dengan `failure_kind='capability'`.

**US-AD87** — Agent gagal karena kredensial invalid `Must` · `M4`
Sebagai worker, ketika kredensial provider agent ditolak (HTTP 401), kegagalan harus diklasifikasikan agar tidak di-retry otomatis dan operator mendapat notifikasi yang jelas.



- [ ] AC1 Worker mendapat HTTP 401 dari provider → `failure_kind='capability'` → task tidak otomatis di-retry (`retry_policy` menjadi `never` untuk attempt ini).
- [ ] AC2 Task masuk status `blocked` dengan `block_kind=needs_input` dan pesan: "API key provider untuk agent X invalid atau expired. Periksa di Settings > Agent."
- [ ] AC3 Event `run.finished` dengan outcome `failed` dan `failure_kind='capability'` tercatat. Notifikasi dikirim ke owner/admin.
- [ ] AC4 (jalur gagal) Kredensial yang dicabut saat run berjalan menghentikan run pada step berikutnya, bukan menunggu timeout.

**US-AD88** — Reset password (lupa password) `Must` · `M0`
Sebagai pengguna yang lupa password, saya ingin meminta tautan reset lewat email, sehingga saya bisa masuk kembali tanpa kehilangan data.
- [ ] AC1 `POST /api/v1/auth/password/reset-request` dengan email terdaftar mengembalikan 202 dan mengirim email berisi token sekali-pakai; token disimpan sebagai hash, bukan plaintext.
- [ ] AC2 `POST /api/v1/auth/password/reset` dengan token valid + password baru minimum 8 karakter mengembalikan 200, memperbarui `users.password_hash`, dan mencabut seluruh baris `sessions` milik pengguna itu.
- [ ] AC3 (jalur gagal) Token kedaluwarsa (lebih dari 30 menit) atau sudah dipakai mengembalikan 410 dan tidak mengubah password.
- [ ] AC4 (permission) Kedua endpoint publik dan tidak memerlukan token sesi.
- [ ] AC5 (keamanan) Permintaan reset untuk email yang TIDAK terdaftar tetap mengembalikan 202 dengan pesan identik, sehingga keberadaan akun tidak dapat ditebak.

**US-AD89** — Profil akun sendiri `Must` · `M0`
Sebagai pengguna, saya ingin melihat dan mengubah profil saya (nama tampilan, email, avatar), sehingga akun terasa milik saya sendiri.
- [ ] AC1 `GET /api/v1/auth/me` mengembalikan id, email, name, dan `avatar_user`; halaman Profil menampilkan nilai yang sama.
- [ ] AC2 Mengubah `name` mengembalikan 200 dan langsung terlihat di top bar tanpa reload penuh.
- [ ] AC3 (jalur gagal) Mengubah email ke alamat yang sudah dipakai akun lain mengembalikan 409 dan tidak mengubah data.
- [ ] AC4 (permission) Pengguna hanya dapat membaca dan mengubah profilnya sendiri; mengakses id pengguna lain mengembalikan 404, bukan 403 (tidak membocorkan keberadaan akun).
- [ ] AC5 (B2C) Halaman Profil dapat dibuka dari menu avatar dan tidak memerlukan peran `owner`/`admin`.

**US-AD90** — Ganti password dan sesi aktif `Must` · `M1`
Sebagai pengguna, saya ingin mengganti password saya dan melihat daftar perangkat yang sedang masuk, sehingga saya bisa mengusir sesi yang tidak saya kenali.
- [ ] AC1 Ganti password memerlukan password lama yang benar; berhasil → 200 dan seluruh sesi LAIN dicabut, sesi saat ini tetap aktif.
- [ ] AC2 Halaman menampilkan daftar sesi aktif: perangkat/user agent, IP, waktu masuk terakhir, dan penanda "perangkat ini".
- [ ] AC3 (jalur gagal) Password lama salah mengembalikan 401 dan tidak mengubah apa pun.
- [ ] AC4 (permission) Pengguna hanya dapat mencabut sesinya sendiri; mencabut sesi pengguna lain memerlukan `owner`/`admin` (lihat US-AD05).

**US-AD91** — Daftar board lintas project `Must` · `M1`
Sebagai pengguna, saya ingin melihat semua board saya dalam satu daftar, sehingga saya dapat berpindah kerja tanpa menghafal struktur project.
- [ ] AC1 Halaman Board List menampilkan tiap board dengan: nama, project induk, jumlah task, jumlah task `running`, biaya hari ini.
- [ ] AC2 Baris board mengikuti kontrak density (baris 28px, header 32px) dan dapat diurutkan berdasarkan biaya atau jumlah task.
- [ ] AC3 (jalur gagal) Pengguna tanpa board melihat empty state dengan CTA "Create your first board", bukan tabel kosong tanpa penjelasan.
- [ ] AC4 (permission) Hanya board dalam ruang kerja pengguna yang tampil; board ruang kerja lain tidak pernah muncul di daftar maupun di hasil pencarian.

**US-AD92** — Alur pertama kali (first-run onboarding) `Must` · `M0`
Sebagai pengguna yang baru mendaftar, saya ingin dibimbing sampai punya board pertama yang jalan, sehingga saya tidak menghadapi layar kosong tanpa arah.
- [ ] AC1 Setelah registrasi pertama kali, pengguna diarahkan ke alur tiga langkah: buat project → buat board → daftarkan agent.
- [ ] AC2 Tiap langkah menampilkan status selesai/belum; alur dapat ditutup dan tidak muncul lagi setelah ketiga langkah selesai.
- [ ] AC3 (B2C) Alur tidak pernah meminta pengguna membuat organisasi, mengundang anggota, atau membuka halaman settings.
- [ ] AC4 (jalur gagal) Pengguna yang menutup alur di tengah jalan tetap dapat kembali lewat tautan "Lanjutkan penyiapan" di halaman daftar board.
- [ ] AC5 (permission) Alur hanya tampil untuk pengguna dengan nol project; pengguna yang sudah punya data tidak pernah melihatnya.

**US-AD93** — Konteks ruang kerja aktif `Must` · `M0`
Sebagai pengguna, saya ingin aplikasi memakai ruang kerja saya secara otomatis, sehingga saya tidak perlu memilih apa pun saat hanya punya satu.
- [ ] AC1 Ruang kerja aktif disimpan di session dan dipakai untuk seluruh permintaan tanpa parameter tambahan dari klien.
- [ ] AC2 (B2C) Bila pengguna hanya anggota satu ruang kerja, pemilih ruang kerja tidak dirender di top bar sama sekali.
- [ ] AC3 (B2C) Bila pengguna anggota lebih dari satu ruang kerja, pemilih muncul dan berpindah ruang kerja memuat ulang data board tanpa logout.
- [ ] AC4 (permission) Berpindah ke ruang kerja yang bukan milik pengguna ditolak 403 dan konteks aktif tidak berubah.
- [ ] AC5 (jalur gagal) Bila ruang kerja aktif dihapus saat pengguna sedang membukanya, permintaan berikutnya mengembalikan 404 dan aplikasi memilih ruang kerja lain secara otomatis, bukan menampilkan layar rusak.

**US-AD94** — Panel detail step dan payload `Must` · `M2`
Sebagai pengguna, saya ingin membuka satu step dan memeriksa payload masuk/keluar serta biayanya, sehingga saya bisa tahu persis di mana token terpakai.
- [ ] AC1 Panel step menampilkan: nama, jenis, status, durasi, token masuk/keluar, cache read/write, dan biaya step dalam font monospace tabular.
- [ ] AC2 Payload masuk dan keluar ditampilkan sebagai blok kode yang dapat dilipat; payload besar dipotong dengan penanda "tampilkan selengkapnya", bukan membanjiri layar.
- [ ] AC3 (jalur gagal) Step yang belum selesai menampilkan durasi sebagai "sedang berjalan" dan biaya sementara, bukan 0 yang menyesatkan.
- [ ] AC4 (jalur gagal) Bila payload tidak tersimpan (kebijakan retensi), panel menampilkan "payload tidak tersedia" dan tetap menampilkan biaya serta token.
- [ ] AC5 (permission) `viewer` melihat angka biaya dan token tetapi payload mentah disamarkan.

**US-AD95** — Penampil audit log `Must` · `M5`
Sebagai pemilik ruang kerja, saya ingin membaca catatan perubahan administratif, sehingga saya dapat mengetahui siapa mengubah apa dan kapan.
- [ ] AC1 Halaman menampilkan baris `audit_log` dengan: waktu, aktor, aksi, target, dan nilai sebelum/sesudah.
- [ ] AC2 Baris dapat difilter berdasarkan aktor, jenis aksi, dan rentang tanggal, serta diekspor ke CSV.
- [ ] AC3 (jalur gagal) Bila tidak ada catatan pada rentang yang dipilih, tampilkan empty state "tidak ada aktivitas pada rentang ini".
- [ ] AC4 (permission) Hanya `owner` dan `admin` yang dapat membuka halaman ini; `member` dan `viewer` mendapat 403 pada level halaman, bukan hanya pada level endpoint.

**US-AD96** — Formulir agent (buat dan ubah) `Must` · `M2`
Sebagai pengguna, saya ingin mendaftarkan agent lewat formulir: nama, provider, model, dan kredensial, sehingga saya tidak perlu menyusun permintaan API sendiri.
- [ ] AC1 Formulir memuat pilihan `provider` dan `model` yang valid; kombinasi di luar daftar harga ditolak sebelum dikirim.
- [ ] AC2 Kolom kredensial bersifat tulis-saja: setelah tersimpan, nilai ditampilkan ter-mask (`sk-...XXXX`) dan tidak pernah dikembalikan utuh oleh API.
- [ ] AC3 (jalur gagal) Menyimpan tanpa kredensial tetap diizinkan, tetapi agent ditandai "belum siap" dan tidak dapat diklaim dispatcher (`failure_kind='capability'`).
- [ ] AC4 (permission) Kolom kredensial hanya tampil untuk `owner`/`admin`; `member` dan `viewer` melihat formulir tanpa bagian kredensial.
- [ ] AC5 Terdapat tombol "Uji kredensial" yang memanggil `POST /api/v1/agents/{id}/validate` dan menampilkan hasil berhasil/gagal secara inline.

**US-AD97** — Cost rail: panel biaya sisi kanan `Must` · `M2`
Sebagai pengguna, saya ingin melihat biaya hari ini, tren 7 hari, agent paling boros, dan run yang sedang jalan, tanpa meninggalkan board.
- [ ] AC1 Panel kanan lebar tetap 264px, selalu tampil di halaman board, berisi empat blok: TODAY, 7-DAY, TOP SPENDERS, RUNNING NOW.
- [ ] AC2 Blok TODAY menampilkan biaya hari ini dalam format monospace dan meter pagu harian; meter berubah warna pada 80% (`budget-meter-warning`) dan saat lewat pagu (`budget-meter-over`).
- [ ] AC3 Blok 7-DAY menampilkan 7 batang `sparkline-bar` dengan hari yang melewati pagu ditandai warna peringatan.
- [ ] AC4 (jalur gagal) Bila pagu harian board belum diset, meter tidak ditampilkan sama sekali dan blok TODAY tetap menampilkan angka biaya tanpa error.
- [ ] AC5 (permission) Panel hanya menampilkan angka dari ruang kerja pengguna; `viewer` melihat total biaya tetapi tidak rincian per-step.

**US-AD98** — Menutup akun sendiri `Should` · `M6`
Sebagai pengguna, saya ingin menutup akun saya beserta seluruh datanya, sehingga saya tidak meninggalkan data yang tidak saya inginkan.
- [ ] AC1 Permintaan penutupan memerlukan pengetikan ulang email akun sebagai konfirmasi, bukan sekadar menekan tombol.
- [ ] AC2 Penutupan akun menghapus data pengguna dan data ruang kerja yang hanya dimiliki pengguna itu, serta mencabut seluruh sesi.
- [ ] AC3 (jalur gagal) Penutupan ditolak 409 bila pengguna adalah `owner` terakhir pada ruang kerja yang masih berisi pengguna lain.
- [ ] AC4 (permission) Pengguna hanya dapat menutup akunnya sendiri; tidak ada endpoint untuk menutup akun pengguna lain.
- [ ] AC5 Penutupan bersifat lunak selama 30 hari: data dapat dipulihkan oleh operator sebelum dihapus permanen.

**US-AD99** — Halaman dokumentasi `Should` · `M6`
Sebagai pengguna baru, saya ingin membaca cara memasang dan memakai AgentDeck, sehingga saya bisa jalan tanpa bertanya ke siapa pun.
- [ ] AC1 Halaman Docs dapat diakses dari tautan "Docs" di navigasi dan footer, serta tersedia tanpa login.
- [ ] AC2 Memuat minimal: pemasangan (satu biner + Postgres), perintah `agentdeck migrate`/`serve`/`agent add`, penjelasan sepuluh status task, dan cara memasang webhook.
- [ ] AC3 (jalur gagal) Tautan ke halaman Docs yang belum ditulis menampilkan halaman "sedang disusun" dengan tautan balik, bukan 404 mentah.
- [ ] AC4 Setiap blok perintah dapat disalin dengan satu klik dan ditampilkan dalam font monospace.

**US-AD100** — Halaman harga `Should` · `M6`
Sebagai calon pengguna, saya ingin melihat paket dan harga dengan jelas, sehingga saya bisa memutuskan sendiri tanpa menghubungi sales.
- [ ] AC1 Halaman Pricing dapat diakses tanpa login dan memuat dua paket: gratis (solo) dan berbayar, dengan daftar isi tiap paket.
- [ ] AC2 Tidak ada satu pun elemen yang meminta pengguna menghubungi sales: tanpa "Contact sales", tanpa "Book a demo", tanpa "Request access".
- [ ] AC3 Harga Pro ditampilkan sebagai `$5` per bulan, flat — tidak boleh diganti placeholder atau nominal lain.
- [ ] AC4 Harga dinyatakan sebagai biaya tetap per bulan, bukan per kursi, dan mencantumkan bahwa penyelenggaraan bersifat self-hosted.

**US-AD101** — Halaman repositori & rilis `Should` · `M6`
Sebagai pengguna teknis, saya ingin melihat kode sumber dan riwayat rilis, sehingga saya bisa menilai sendiri sebelum menjalankan binary orang lain.
- [ ] AC1 Halaman menampilkan tautan repositori, lisensi, versi terbaru, tanggal rilis, dan daftar perubahan singkat per rilis.
- [ ] AC2 Tautan ke halaman ini tersedia di navigasi landing page (label "GitHub") dan tidak menuju halaman kosong.
- [ ] AC3 (jalur gagal) Bila metadata rilis tidak dapat dimuat, halaman menampilkan tautan repositori langsung, bukan layar kosong.

**US-AD102** — Halaman komunitas & dukungan `Should` · `M6`
Sebagai pengguna, saya ingin tahu ke mana harus bertanya saat tersangkut.
- [ ] AC1 Halaman menampilkan kanal dukungan (server komunitas, issue tracker) dan jam respons yang dijanjikan.
- [ ] AC2 Tautan "Discord" di footer landing menuju halaman ini, bukan anchor kosong.
- [ ] AC3 (jalur gagal) Bila kanal komunitas belum aktif, halaman menampilkan kanal alternatif, bukan tautan mati.

**US-AD103** — Dokumentasi: mulai cepat `Should` · `M6`
Sebagai pengguna baru, saya ingin langkah instalasi yang bisa saya ikuti, sehingga saya bisa menjalankan AgentDeck tanpa menebak.
- [ ] AC1 Halaman menampilkan langkah berurutan: prasyarat, migrasi database, menjalankan binary, dan mendaftarkan agent pertama.
- [ ] AC2 Setiap perintah ditampilkan dalam blok kode monospace dengan tombol salin.
- [ ] AC3 (jalur gagal) Bila perintah gagal, setiap langkah menyertakan galat umum dan cara mengatasinya.

**US-AD104** — Dokumentasi: referensi REST API `Should` · `M6`
Sebagai integrator, saya ingin melihat daftar endpoint beserta bentuk request/response-nya.
- [ ] AC1 Halaman menampilkan daftar endpoint dikelompokkan per sumber daya, dengan metode HTTP, jalur, dan peran minimum.
- [ ] AC2 Setiap endpoint menampilkan contoh body request dan contoh response dalam blok kode monospace.
- [ ] AC3 (jalur gagal) Endpoint yang memerlukan peran lebih tinggi ditandai jelas, termasuk bentuk respons 403.

**US-AD105** — Dokumentasi: skema telemetri `Should` · `M6`
Sebagai integrator, saya ingin memahami bentuk payload event dan step sebelum menulis worker.
- [ ] AC1 Halaman menampilkan skema event SSE dan skema payload step beserta tipe setiap field.
- [ ] AC2 Field biaya dan token ditandai satuannya (micro-USD, jumlah token) dan tipenya (integer).
- [ ] AC3 (jalur gagal) Field yang bersifat opsional atau dapat bernilai null ditandai eksplisit.

## 7. Functional Requirements

**FR-01** Sistem mendukung registrasi dengan email + password (Argon2id). Password minimum 8 karakter, maksimum 128.
**FR-02** Session token adalah opaque string random 64 byte, disimpan sebagai hash SHA-256 di tabel `sessions`. Tidak ada JWT.
**FR-03** Semua endpoint kecuali yang ditandai publik memerlukan header `Authorization: Bearer <session_token>` atau cookie.
**FR-04** API Key berbentuk `adk_<prefix:8char>:<secret:48char>`. Hanya hash secret yang disimpan; prefix untuk identifikasi.
**FR-05** Setiap request API wajib menyertakan konteks `org_id` (dari session/API key atau header `X-Org-ID`). Backend memvalidasi user adalah anggota org tersebut.
**FR-06** Endpoint `GET /api/v1/events` menyediakan koneksi SSE. Server mengirim event sebagai `data: {json}\n\n`. Klien mengirim `Last-Event-ID` saat reconnect.
**FR-07** Dispatcher adalah loop Go `for { select { case <-tick:N19: } }` yang setiap siklus memanggil query klaim batch (N20).
**FR-08** Setiap pembuatan, perubahan, atau penghapusan task, run, agent, project, board, membership, API key, role, dan webhook dicatat di `audit_log` dengan before/after JSON snapshot.
**FR-09** Rate limiter per endpoint publik (10/menit/IP) dan endpoint terautentikasi (100/menit/session). Response 429 menyertakan `Retry-After`.
**FR-10** Task yang dalam status `running` tidak bisa diedit title/body/priority/dependency oleh pengguna.
**FR-11** Approval gate: task yang di-hold di `awaiting_approval` membekukan timer runtime; durasi hold tidak dihitung ke `max_runtime_seconds`.
**FR-12** Setiap perubahan status task memicu event `task.status_changed` yang disiarkan ke SSE dan diproses webhook.
**FR-13** Harga model LLM didefinisikan sebagai **snapshot versi statis di konstanta Go** (`internal/pricing`), dengan atribut per model: `provider`, `model`, `price_per_million_input_tokens`, `price_per_million_output_tokens`, cache read/write rate, dan `price_version` (integer, dimulai 1). Setiap baris `ledger_entries` menyimpan `price_version` sebagai bukti sanitasi audit biaya historis; riwayat harga dipertahankan secara tak-berubah (`immutable`). Tidak ada tabel runtime `pricing` — versi dikodekan di binary untuk menghindari sumber harga yang bisa berubah tak terduga.
**FR-14** Artifact diunggah melalui presigned URL dari backend, dikonfirmasi dengan callback setelah upload selesai.
**FR-15** Pencarian task menggunakan `pg_trgm` atau `ILIKE` dengan batas 100 hasil per query.
**FR-16** Semua input dari pengguna divalidasi di server (panjang maksimum, karakter yang diizinkan, tipe data) sebelum disimpan.
**FR-17** Semua input dari agent/worker (step payload, artifact, error message) disimpan sebagaimana adanya dan ditampilkan di UI dengan escaping HTML.
**FR-18** Dependency graph diuji siklus dengan DFS. Kompleksitas O(V+E).
**FR-19** Dua task tidak boleh memiliki `idempotency_key` yang sama dalam satu board. Implementasi: UNIQUE constraint `(board_id, idempotency_key)` dengan key nullable — key NULL diizinkan duplikat.
**FR-20** Session memiliki sliding expiry: 24 jam idle, maksimum 7 hari absolute.

---

## 8. Non-Functional Requirements

| ID | Rujukan | Persyaratan | Cara Ukur |
|---|---|---|---|
| NFR-01 | N1 | p95 latensi endpoint baca ≤ 150 ms | Grafana, middleware latency metric |
| NFR-02 | N2 | First paint board dengan 5.000 task ≤ 400 ms | Lighthouse / Playwright trace |
| NFR-03 | N3 | Event SSE dari backend ke UI p95 ≤ 1,5 detik | Grafana, event enqueue → SSE deliver |
| NFR-04 | N4 | Mendukung 50 agent running bersamaan per org | Test beban |
| NFR-05 | N5 | Kapasitas desain 100.000 run per bulan | Test migrasi + agregasi biaya |
| NFR-06 | N6 | Kapasitas 1.000.000 event per bulan | Test throughput event log |
| NFR-07 | N7 | Heartbeat interval 60 detik | Log monitor |
| NFR-08 | N8 | Stale reclaim jika tanpa heartbeat 15 menit | Test integrasi |
| NFR-09 | N9 | Max runtime per run default 4 jam | Konfigurasi |
| NFR-10 | N13 | Biaya infrastruktur ≤ $10/bulan | Cek billing |
| NFR-11 | N14 | Ukuran binary API ≤ 30 MB | `ls -lh` output |
| NFR-12 | N15 | RAM API saat idle ≤ 80 MB | `top` / container metrics |
| NFR-13 | N16 | Cap biaya default per board per hari $20 | Diverifikasi test |
| NFR-14 | N17 | Hard stop per run $2 | Diverifikasi test |
| NFR-15 | N19 | Interval dispatcher tick 2 detik | Test timing |

---

## 9. Success Metrics

| Metrik | Baseline | Target | Cara Ukur | Waktu Ukur |
|---|---|---|---|---|
| Biaya hosting bulanan | - | ≤ $10 | Fly.io + Neon billing dashboard | Akhir bulan |
| p95 latensi baca | - | ≤ 150 ms | Grafana, middleware metric | Minggu 2 |
| First paint board (5K task) | - | ≤ 400 ms | Lighthouse CI | Minggu 3 |
| Approved run berhasil lewat approval gate | - | 100% | Audit log review | Minggu 4 |
| Automated retry recovery rate | `ASSUMPTION: 80%` | > 80% | Run outcome aggregation | Minggu 6 |
| Dwibahasa coverage | - | 100% UI string | Script scan komponen vs translation keys | Minggu 5 |
| Test coverage backend | - | > 70% | `go test -cover` | Minggu 6 |

---

## 10. Risks & Mitigations

| Risiko | Dampak | Mitigasi |
|---|---|---|
| R1 — Biaya LLM membengkak karena agent loop | Finansial, kepercayaan | Hard stop N17 + N16 + alert N18 |
| R2 — Approval tidak direspons, task mandek | Produktivitas | Expiry N23 → task otomatis ke blocked + notif ulang |
| R3 — Postgres SKIP LOCKED jadi bottleneck di N4/N5 | Performa | Partial index + query plan review + test beban |
| R4 — SSE mati di reverse proxy | Realtime loss | Fallback polling + resume via Last-Event-ID |
| R5 — API key bocor | Keamanan | Revoke endpoint + audit + rotasi berkala |
| R6 — Adopsi rendah karena UI tidak intuitif | Produktivitas | User test internal sebelum M4, iterasi cepat |
| R7 — Worker agent berbahaya (command injection) | Keamanan | Sandbox container + validasi input + audit log |
| R8 — Data loss karena kegagalan migrasi DB | Data | Backup otomatis Neon, rollback script |
| R9 — Harga LLM berubah tak terduga | Finansial | `price_version` + riwayat harga + notifikasi perubahan |
| R10 — Waktu pengembangan tidak cukup | Jadwal | Prioritaskan Must stories, Should/Could ke M5+ |

---

## 11. Open Questions

`OPEN:` Apakah approval gate untuk v0.1 hanya manual atau ada integrasi Slack/webhook notification?
**Rekomendasi:** v0.1 manual via notifikasi in-app + SSE. Webhook notifikasi di M5 (US-AD52).

`OPEN:` Worker eksekusi — apakah AgentDeck menyediakan runner sendiri atau hanya API endpoint yang dipanggil worker eksternal?
**Rekomendasi:** v0.1 hanya menyediakan API orchestrasi (dispatcher + heartbeat + artifact). Runner eksternal terpisah.

`OPEN:` Apakah Hermes Kanban sendiri bisa dipakai sebagai supplier event/feed back ke AgentDeck?
`ASSUMPTION:` Tidak — Hermes Kanban adalah alat terpisah. AgentDeck punya event stream sendiri.

`OPEN:` Siapa yang memperbarui snapshot harga LLM di `internal/pricing` saat provider mengubah tarif?
**Rekomendasi:** Rilis binary baru (sesuai FR-13 — tidak ada tabel `pricing` runtime). Perubahan tarif diumumkan di changelog; `price_version` naik satu tiap rilis harga.

`OPEN:` Apakah perlu UI agent builder untuk menyusun tool/skill agent?
**Rekomendasi:** Tidak untuk v0.1. Cukup JSON editor di form registrasi agent.

`OPEN:` Bagaimana cara handle run yang memakan waktu > 4 jam (N9)?
**Rekomendasi:** Max runtime bisa di-override per agent sampai 24 jam. Tapi task yang melebihi 24 jam di-reclaim.

---

## 12. Release Plan & Definition of Done

### Milestones

| Milestone | Target | Exit Criteria |
|---|---|---|
| M0 — Auth, Akun & Masuk Pertama (B2C) | Minggu 1 | US-AD01..05, US-AD77..78, US-AD88..93 lulus test |
| M1 — Board Core & Dispatcher | Minggu 2-3 | US-AD08..17, US-AD20..25, US-AD58..60, US-AD64, US-AD66..67, US-AD73, US-AD75, US-AD79..85 lulus test |
| M2 — DAG & Cost Ledger & Provider Keys | Minggu 4 | US-AD18..19, US-AD27..32, US-AD68..70, US-AD80, US-AD86 lulus test |
| M3 — Approval & Realtime | Minggu 5-6 | US-AD26, US-AD33..42, US-AD61..63, US-AD65, US-AD71, US-AD85 lulus test |
| M4 — Failure Handling & Artifact | Minggu 7 | US-AD43..50, US-AD74, US-AD87 lulus test |
| M5 — Audit, Webhook, Table View | Minggu 8 | US-AD06, US-AD51..54 lulus test |
| M6 — Polish, Halaman Publik & Fitur Tambahan | Minggu 9 | US-AD55..57, US-AD72, US-AD98..105 lulus test |

### Definition of Done

- [ ] Kode sudah di-review (PR + minimal 1 reviewer)
- [ ] Test unit + integrasi lulus (`go test ./...`)
- [ ] Test beban untuk `Must` endpoint lulus (N1 target)
- [ ] DESIGN.md lint lulus (jika ada perubahan token)
- [ ] Semua `Must` story AC terverifikasi lulus
- [ ] Tidak ada data PII di log debug
- [ ] Dokumentasi endpoint minimal (README + ARCHITECTURE.md) sudah diperbarui
- [ ] Binary backend ≤ 30 MB (N14)

---

> **Catatan cakupan AC (permission).** Seluruh story `Must` memiliki minimal satu AC `(jalur gagal)`; 22 story `Must` tidak memiliki AC `(permission)` terpisah, dan itu disengaja. Empat kelompok alasannya:
> 1. **Dijalankan worker lewat API key** (US-AD23, US-AD27, US-AD28, US-AD38, US-AD43, US-AD44, US-AD45, US-AD68) — otorisasinya adalah kredensial worker pemegang `claim_lock`, diuji di US-AD21/US-AD22/US-AD25/US-AD46.
> 2. **Sistem otomatis tanpa permukaan mutasi user** (US-AD31, US-AD40, US-AD50, US-AD70) — alert, timeline baca-saja, lint build-time, validasi graf.
> 3. **UI murni / presentasi** (US-AD49, US-AD60, US-AD63, US-AD64, US-AD65, US-AD81, US-AD82) — tidak menyentuh mutasi ber-scope org; yang mengikat mereka adalah isolasi tenant di US-AD07 dan US-AD39.
> 4. **AC `(jalur gagal)` sudah menyatakan 403/404** (US-AD07, US-AD14, US-AD77) — AC1 US-AD07 memang uji lintas-Org (403/404), sementara US-AD14 dan US-AD77 menyatakan penolakan role di AC jalur gagalnya.
> Story yang memutasi/membaca resource org-scoped dan tidak masuk empat kelompok di atas tetap punya AC `(permission)` eksplisit: US-AD08..AD15, US-AD48, US-AD75, US-AD79, US-AD83.

---

## 13. Traceability

| Rentang Story | Seksi ARCHITECTURE.md | Milestone |
|---|---|---|
| US-AD01..05, US-AD77..78 | Seksi 11 (ARCHITECTURE.md) — Auth, RBAC & Isolasi Tenant · Seksi 6 Kontrak HTTP API (auth) | M0 |
| US-AD07, US-AD03 | Seksi 11 (ARCHITECTURE.md) — Auth, RBAC & Isolasi Tenant · Seksi 5 Query Kritis | M0 |
| US-AD08..17, US-AD58..60, US-AD64, US-AD76, US-AD79, US-AD81..85 | Seksi 6 Kontrak HTTP API · Seksi 2 (ARCHITECTURE.md) — Gambaran Sistem | M1 |
| US-AD20..25, US-AD66..67, US-AD73, US-AD75 | Seksi 5 (ARCHITECTURE.md) — Algoritma Dispatcher · Seksi 12 (ARCHITECTURE.md) — Workspace & Eksekusi Agent · Seksi 6 Kontrak HTTP API (agents, tasks, runs) | M1 |
| US-AD18..19, US-AD70, US-AD80 | Seksi 5 (ARCHITECTURE.md) — Algoritma Dispatcher (DAG) · Seksi 6 Kontrak HTTP API (task_links) | M2 |
| US-AD27..32, US-AD68..69, US-AD86 | Seksi 9 (ARCHITECTURE.md) — Cost Ledger & Budget Guardrail · Seksi 5 Query Kritis | M2 |
| US-AD33..38 | Seksi 8 (ARCHITECTURE.md) — Approval Gate · Seksi 6 Kontrak HTTP API (approvals) | M3 |
| US-AD26, US-AD39..42, US-AD61..63, US-AD65, US-AD71 | Seksi 7 Realtime (SSE) · Seksi 6 Kontrak HTTP API (events, comments) | M3 |
| US-AD43..48, US-AD74, US-AD87 | Seksi 10 (ARCHITECTURE.md) — Retry & Failure Taxonomy · Seksi 6 Kontrak HTTP API (artifacts) | M4 |
| US-AD49..50 | Seksi 2 (ARCHITECTURE.md) — Gambaran Sistem (Frontend) | M4 |
| US-AD06, US-AD51..54 | Seksi 14 (ARCHITECTURE.md) — Observabilitas · Seksi 13 (ARCHITECTURE.md) — Webhook · Seksi 16 Keamanan · Seksi 6 Kontrak HTTP API | M5 |
| US-AD55..57, US-AD72 | Seksi 6 Kontrak HTTP API (bulk, template) · Seksi 17 Struktur Folder (Frontend) | M6 |
| US-AD88, US-AD90 | Seksi 6 Kontrak HTTP API (auth) · Seksi 3.20 `password_reset_tokens` · Seksi 11 Auth & RBAC | M0 |
| US-AD89, US-AD98 | Seksi 6 Kontrak HTTP API (auth/me) · Seksi 3.3 `users` (avatar_url, deleted_at) · Seksi 11 Auth & RBAC | M0/M6 |
| US-AD91 | Seksi 6 Kontrak HTTP API (boards, projects) · Seksi 3.5/3.6 | M1 |
| US-AD92..93 | Seksi 6 Kontrak HTTP API (auth/me, projects) · Seksi 11 Auth & RBAC (konteks ruang kerja) · Seksi 17 Struktur Folder (Frontend) | M0 |
| US-AD94, US-AD97 | Seksi 6 Kontrak HTTP API (steps, cost ledger) · Seksi 9 Cost Ledger & Budget Guardrail | M2 |
| US-AD95 | Seksi 6 Kontrak HTTP API (audit-log) · Seksi 3.17 `audit_log` | M5 |
| US-AD96 | Seksi 6 Kontrak HTTP API (agents, provider-key) · Seksi 12 Workspace & Eksekusi Agent | M2 |
| US-AD99..105 | Seksi 17 Struktur Folder (Frontend, halaman statis & dokumentasi) | M6 |

---

## 14. Lampiran: Peta Kompetitif

| Dimensi | AgentDeck | Hermes Kanban | Linear | LangSmith | Langfuse | Temporal UI |
|---|---|---|---|---|---|---|
| **Kanban board** | ✅ Kolom status × view board | ✅ Kanban board lengkap | ✅ Issue board | ❌ Dashboard/daftar | ❌ Dashboard/trace | ❌ Workflow list |
| **Cost per run (ledger)** | ✅ **Micro-USD, per step, price_version** | ❌ Tidak ada | ❌ Tidak ada | ❌ Token usage aggregated | ❌ Token cost (agregat) | ❌ Tidak ada |
| **Approval gate built-in** | ✅ **State mesin `awaiting_approval`** | ❌ Tidak ada (block external) | ❌ Issue approval workflow | ❌ Tidak ada | ❌ Tidak ada | ❌ Tidak ada |
| **Run trace/replay** | ✅ **Step timeline hierarkis** | ❌ Tidak ada | ❌ Tidak ada | ✅ Trace/langchain view | ✅ Trace + cost | ✅ Workflow history |
| **Multi-tenant** | ✅ Org → Project → Board | ❌ Single-user SQLite | ❌ Workspace-based | ✅ Organization | ✅ Organization | ✅ Namespace |
| **Self-host biaya rendah** | ✅ **Go 1 binary, ≤ $10/bulan** | ✅ Docker, $0-10 | ❌ Managed only | ❌ Cloud only (mahal) | ✅ Open-source | ✅ Open-source (berat) |
| **Agent dispatch engine** | ✅ **Postgres SKIP LOCKED, N19/N20** | ✅ Kanban dispatcher | ❌ Issue routing | ❌ Tidak ada | ❌ Tidak ada | ✅ Worker dispatch |
| **Budget guardrail** | ✅ **Hard N17 + soft N18** | ❌ Tidak ada | ❌ Tidak ada | ❌ Tidak ada | ❌ Tidak ada | ❌ Tidak ada |
| **Retry berdasarkan failure_kind** | ✅ **failure taxonomy + backoff** | ❌ Simple retry | ❌ Tidak ada | ❌ Tidak ada | ❌ Tidak ada | ❌ Retry sederhana |
| **Dukungan dwibahasa** | ✅ EN/ID | ✅ EN only | ✅ EN only | ✅ EN only | ✅ EN only | ✅ EN only |

**Keterangan:** AgentDeck unggul di dimensi cost ledger, approval gate, budget guardrail, dan biaya self-host. Kekalahan: trace/detail di LangSmith/Langfuse lebih matang (perusahaan dedicated observability). Hermes Kanban unggul di kedalaman fitur kanban (goal_mode, skills, completion contract). `ASSUMPTION:` Perbandingan harga LangSmith/Langfuse berdasarkan tier publik; mungkin ada self-host option yang tidak terdokumentasi dengan baik.

