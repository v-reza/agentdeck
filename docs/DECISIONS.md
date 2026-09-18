# AgentDeck — FROZEN DECISIONS (contract)

> Setiap dokumen di suite ini **wajib** mengikuti file ini. Kalau ada yang mau berbeda,
> ubah file ini dulu, jangan bikin keputusan lokal. Semua angka di Seksi 7 didefinisikan
> **di sini saja** — dokumen lain merujuk, tidak menyalin.

---

## 1. Identitas produk

| Field | Value |
|---|---|
| Product | **AgentDeck** |
| Tagline | Orchestration board for AI agent fleets — every run priced, every action gated. |
| Kategori | Agent orchestration / observability board (bukan project management generic) |
| Positioning | Hermes Kanban tapi multi-tenant, ada cost ledger, approval gate, dan run replay |
| Owner | Reza (solo dev) |
| Versi dokumen | 0.1 `draft` |
| Bahasa produk | EN default + ID (i18n dua bahasa) |
| URL | agentdeck.dev (placeholder) |

---

## 2. Kosakata domain (WAJIB dipakai konsisten)

| Istilah | Arti | Bukan |
|---|---|---|
| **Org** | tenant; batas isolasi data | team/workspace |
| **Project** | grup board + agent registry di dalam Org | app |
| **Board** | papan kanban; punya kolom & policy | sprint |
| **Task** | unit kerja; punya assignee agent | ticket/card |
| **Agent** | profil worker: model, provider, tools, skills | user/bot |
| **Run** | satu eksekusi Task oleh satu Agent | job/attempt |
| **Step** | span di dalam Run (tool call, LLM call, shell) | stage |
| **Event** | catatan append-only di timeline | log |
| **Approval** | permintaan izin manusia sebelum aksi | request |
| **Artifact** | file hasil (patch, report, image) | attachment |
| **Ledger** | catatan biaya/token per Run | invoice |

---

## 3. State machine (tunggal, tidak boleh ditambah)

### Task status — 10 nilai
```
backlog → ready → running → review → done
                    ↓         ↑
              awaiting_approval
                    
blocked  (dari status apa pun; punya block_kind)
failed   (dead-letter; bisa di-retry ke ready)
cancelled (terminal, manual)
archived  (terminal, tersembunyi default)
```

- `backlog` — belum siap (dependency belum kelar / belum di-approve plan)
- `ready` — siap diklaim dispatcher
- `running` — ada Run aktif yang memegang claim
- `awaiting_approval` — Run berhenti nunggu keputusan manusia
- `blocked` — butuh sesuatu; `block_kind` ∈ Seksi 4
- `review` — kerja kelar, nunggu review manusia/agent
- `done` / `failed` / `cancelled` / `archived` — terminal

### Run outcome — 6 nilai
`succeeded` · `failed` · `timed_out` · `cancelled` · `reclaimed` · `budget_exceeded`

### Run status — 4 nilai (siklus hidup baris `runs`)
`pending` · `claiming` · `running` · `ended`

- `pending` — baris dibuat, belum ada worker yang mengambil
- `claiming` — dispatcher sudah mengklaim, worker belum start
- `running` — worker aktif mengeksekusi step
- `ended` — terminal; `outcome` diisi salah satu dari 6 nilai di atas

### Kolom board default
`Backlog · Ready · Running · Review · Done` (kolom = view dari status, bukan status baru)

---

## 4. Enums (WAJIB sama persis di DB CHECK, API, dan AC)

**block_kind:** `dependency` · `needs_input` · `capability` · `policy` · `budget` · `external`

**failure_kind** (klasifikasi otomatis, dipakai retry policy):
`transient` (network/5xx/timeout) · `needs_input` (ambigu, butuh manusia) ·
`capability` (tool/credential gak ada) · `dependency` (task hulu gagal) ·
`policy` (dilarang policy) · `budget` (lewat cap) · `unknown`

**retry_policy:** `never` · `transient_only` · `always` (dengan `max_attempts`)

**approval_decision:** `pending` · `approved` · `rejected` · `expired`

**approval_gate_mode:** `auto` (jalan terus) · `require` (tunggu manusia) · `deny` (tolak)

**event_kind:** `task.created` · `task.status_changed` · `task.assigned` ·
`run.claimed` · `run.heartbeat` · `run.finished` · `run.reclaimed` ·
`step.started` · `step.finished` · `step.failed` ·
`approval.requested` · `approval.decided` · `approval.expired` ·
`artifact.created` · `ledger.entry` · `comment.created` · `budget.threshold_crossed`

**role:** `owner` · `admin` · `member` · `viewer`

**workspace_kind:** `scratch` · `dir` · `worktree` · `container`

---

## 5. Stack (beku)

### Backend — Go, murah
| Bagian | Pilihan | Alasan |
|---|---|---|
| Bahasa | **Go 1.24** | 1 binary, memory kecil, cold start instan |
| HTTP | **`net/http` ServeMux** (pattern `POST /api/v1/x/{id}`) | stdlib cukup; nol dependency router |
| DB driver | **pgx v5** + **sqlc** | type-safe, no ORM, query jadi Go code |
| DB | **Postgres 16** (Neon free tier) | `FOR UPDATE SKIP LOCKED` = queue tanpa Redis |
| Queue | **Postgres SKIP LOCKED** | hemat: gak perlu Redis/broker |
| Realtime | **SSE** (Server-Sent Events) | lebih murah dari WebSocket, lewat proxy |
| Auth | opaque session token (hash di DB) + API key `adk_...` | gak perlu JWT infra |
| Password | **argon2id** (golang.org/x/crypto) | standar |
| Object store | **Cloudflare R2** (S3 API) | egress gratis, 10 GB free |
| Log | `log/slog` JSON | stdlib |
| Metrik | Prometheus `/metrics` | text exposition, no agent |
| Test | `testing` + `testcontainers-go` (opsional) | stdlib dulu |

**Aturan:** nol framework web, nol ORM, nol Redis, nol Kafka, nol Kubernetes.
Kalau butuh sesuatu yang bukan stdlib, harus ada alasan tertulis di ARCHITECTURE.md.

### Frontend
React 19 + Vite · TypeScript · Tailwind v4 · shadcn/ui · **Redux Toolkit + RTK Query**
(server state, cache, SSE cache-patching) · dnd-kit (drag kartu) · SSE via `EventSource` ·
Vitest + Playwright

**Redux Toolkit = satu-satunya manajemen state.** Tidak ada TanStack Query, tidak ada Zustand,
tidak ada Context untuk data aplikasi.

| Kebutuhan | Solusi Redux |
|---|---|
| Server state (board, task, run, agent, ledger) | RTK Query `createApi` + tag invalidation |
| Realtime SSE | RTK Query `onCacheEntryAdded` menambal cache dari stream |
| Client state (view mode, filter, drawer, bahasa) | `createSlice` + Redux DevTools |
| Side effect (alert cap 80% N18, toast) | `createListenerMiddleware` |
| Optimistic drag kartu | RTK Query `optimisticUpdate` + `useOptimistic` (React 19) |
| Form mutasi | `useActionState` (React 19) men-dispatch thunk |

### Deploy — target ≤ $10/bulan
| Komponen | Layanan | Biaya |
|---|---|---|
| Go API + dispatcher | Fly.io 1 shared-cpu-1x 512MB | ~$3 |
| Postgres | Neon free (0.5 GB) | $0 |
| Artifacts | Cloudflare R2 free (10 GB) | $0 |
| Frontend | Cloudflare Pages / Vercel (static SPA) | $0 |
| Domain | .dev | ~$1/bulan |
| **Total** | | **≤ $10/bulan** |

---

## 6. Skema database (nama tabel & kolom FINAL)

```
orgs(id, slug, name, created_at)
users(id, email, name, password_hash, avatar_url, deleted_at, created_at)
memberships(org_id, user_id, role, created_at)              PK(org_id,user_id)
projects(id, org_id, slug, name, created_at)
boards(id, org_id, project_id, slug, name, columns_json, budget_daily_micros, created_at)
daily_board_costs(org_id, board_id, day, total_micros, run_count, tokens_in, tokens_out, updated_at)
agents(id, org_id, project_id, name, provider, model, reasoning_effort,
       skills_json, tools_json, max_runtime_seconds, retry_policy, max_attempts,
       provider_api_key_enc, created_at)
tasks(id, org_id, board_id, title, body, status, priority, assignee_agent_id,
      created_by, idempotency_key, block_kind, consecutive_failures,
      workspace_kind, workspace_path, branch_name, completion_contract,
      goal_mode, goal_max_turns, current_run_id, cost_micros, tokens_in, tokens_out,
      created_at, started_at, completed_at, archived_at)
task_links(parent_id, child_id)                             PK(parent_id,child_id)
runs(id, org_id, task_id, agent_id, attempt, status, outcome, failure_kind,
     claim_lock, claim_expires, worker_pid, last_heartbeat_at, max_runtime_seconds,
     cost_micros, tokens_in, tokens_out, summary, error, metadata_json,
     started_at, ended_at)
steps(id, org_id, run_id, seq, kind, name, status, tokens_in, tokens_out,
      cost_micros, started_at, ended_at, payload_json)
events(id, org_id, board_id, task_id, run_id, kind, payload_json, created_at)
approvals(id, org_id, task_id, run_id, requested_by, decided_by, decision,
          gate_mode, reason, preview_json, expires_at, decided_at, created_at)
ledger_entries(id, org_id, run_id, task_id, provider, model, kind,
               tokens_in, tokens_out, cache_read_tokens, cache_write_tokens,
               cost_micros, price_version, created_at)
artifacts(id, org_id, task_id, run_id, filename, content_type, size, storage_key,
          sha256, created_at)
comments(id, org_id, task_id, author_user_id, author_agent_id, body, created_at)
audit_log(id, org_id, actor_user_id, actor_agent_id, action, target_type,
          target_id, before_json, after_json, ip, created_at)
api_keys(id, org_id, user_id, name, prefix, token_hash, last_used_at, revoked_at, created_at)
sessions(id, user_id, token_hash, user_agent, ip, last_seen_at, expires_at, created_at)
password_reset_tokens(id, user_id, token_hash, expires_at, used_at, created_at)
notifications(id, user_id, org_id, kind, title, body, target_type, target_id, read_at, created_at)
webhooks(id, org_id, board_id, url, secret, events_json, active, created_at)
webhook_deliveries(id, webhook_id, event_id, status, attempts, response_code,
                   last_error, created_at)
```

**Aturan uang:** `cost_micros` = BIGINT micro-USD (1 USD = 1_000_000). **JANGAN pakai float.**
**Aturan ID:** ULID (TEXT 26) untuk entitas domain; BIGSERIAL untuk `events`, `steps`, `ledger_entries`, `audit_log`.
**Aturan isolasi:** setiap tabel ber-`org_id`; semua query WAJIB filter `org_id` (dicek test).

---

## 7. ANGKA (single source of truth — jangan diulang beda di dokumen lain)

| Kode | Metrik | Nilai |
|---|---|---|
| N1 | p95 latensi API baca | ≤ 150 ms |
| N2 | Board first paint, 5.000 task | ≤ 400 ms |
| N3 | Event → UI (SSE) p95 | ≤ 1.5 s |
| N4 | Agent running bersamaan per org | 50 |
| N5 | Run per bulan (kapasitas desain) | 100.000 |
| N6 | Event per bulan | 1.000.000 |
| N7 | Heartbeat interval | 60 s |
| N8 | Stale reclaim (tanpa heartbeat) | 15 menit |
| N9 | Max runtime default per Run | 4 jam |
| N10 | Retensi event hot | 30 hari |
| N11 | Retensi agregat (harian) | 12 bulan |
| N12 | Retensi artifact | 90 hari |
| N13 | Infra bulanan | ≤ $10 |
| N14 | Ukuran binary API | ≤ 30 MB |
| N15 | RAM API saat idle | ≤ 80 MB |
| N16 | Cap biaya default per board per hari | $20 |
| N17 | Cap biaya per Run (hard stop) | $2 |
| N18 | Ambang alert budget | 80% dari cap |
| N19 | Interval dispatcher tick | 2 s |
| N20 | Batch claim per tick | 20 task |
| N21 | Ukuran payload event maksimum | 64 KB |
| N22 | Batas artifact per task | 25 MB / file, 100 MB / task |
| N23 | Approval expiry default | 24 jam |
| N24 | p95 latensi tulis API | ≤ 300 ms |
| N25 | Uptime target | 99.5% / bulan |

---

## 8. Design direction (beku)

- **Light-first, putih.** Halaman `#f7f8f9`, panel `#ffffff`, border hairline `rgba(15,23,42,0.08)`.
- **Aksen tunggal: Signal Teal `#0f766e`.** Sengaja beda dari Portico (violet) biar punya identitas sendiri.
- Density tinggi: tinggi baris tabel **32px**, padding kartu **12px**, radius **6/8/12px**.
- Status color: `backlog` slate `#64748b` · `ready` blue `#2563eb` · `running` amber `#d97706` ·
  `awaiting_approval` violet `#7c3aed` · `blocked` orange `#ea580c` · `review` cyan `#0891b2` ·
  `done` green `#16a34a` · `failed` red `#dc2626` · `cancelled` slate `#94a3b8` ·
  `archived` slate muda `#cbd5e1` (paling redup — task lama, tersembunyi default)
- Font: **Inter** (UI) + **JetBrains Mono** (angka biaya/token, ID, log).
- **Tidak ada**: gradient dekoratif, glassmorphism, shadow tebal, ilustrasi 3D di dalam app.
- 3D presence layer = modul v2 terpisah, bukan bagian shell UI.

---

## 9. ID & penomoran dokumen

| Artefak | Pola |
|---|---|
| User story | `US-AD01` … (dua digit, zero-padded, **tidak pernah di-renumber**) |
| Acceptance criteria | `AC1`, `AC2`, … per story |
| Functional requirement | `FR-01` … |
| Non-functional requirement | `NFR-01` … (angka merujuk Seksi 7: `NFR-01 (N1)`) |
| Goal / Non-goal | `G1`… / `NG1`… |
| Milestone | `M0` … `M6` |
| Seksi PRD | `## 1.` … `## 14.` (urutan wajib, sama di semua dokumen suite) |

---

## 10. File suite

| File | Isi | Pemilik |
|---|---|---|
| `00-PRD.md` | PRD lengkap Seksi 1–Seksi 14 + semua user story | Reza |
| `ARCHITECTURE.md` | skema, endpoint, dispatcher, deploy, keamanan | — |
| `DESIGN.md` | token DESIGN.md (lintable) | — |
| `DIAGRAMS.md` | diagram arsitektur + ERD + state machine | — |
| `diagrams/architecture.html` | diagram SVG dark, standalone | — |
| `verify_suite.py` | gate: PRD + token + cross-doc | Reza |
