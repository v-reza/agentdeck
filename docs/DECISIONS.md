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
org_kinds(org_id, kind, created_at)
users(id, email, name, password_hash, avatar_url, deleted_at, is_shadow, created_at)
memberships(org_id, user_id, role, created_at)              PK(org_id,user_id)
projects(id, org_id, slug, name, created_at)
boards(id, org_id, project_id, slug, name, columns_json, budget_daily_micros, created_at)
daily_board_costs(org_id, board_id, day, total_micros, run_count, tokens_in, tokens_out, updated_at)
agents(id, org_id, project_id, name, provider, model, reasoning_effort,
       skills_json, tools_json, max_runtime_seconds, retry_policy, max_attempts,
       provider_api_key_enc, base_url, archived_at, has_provider_key, created_at)
agent_skills(id, org_id, slug, name, body_md, version, created_by, created_at, updated_at)
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
               reasoning_tokens, cost_micros, price_version, price_source,
               pricing_model, created_at)
artifacts(id, org_id, task_id, run_id, filename, content_type, size, storage_key,
          sha256, created_at)
comments(id, org_id, task_id, author_user_id, author_agent_id, body, created_at)
audit_log(id, org_id, actor_user_id, actor_agent_id, action, target_type,
          target_id, before_json, after_json, ip, created_at)
api_keys(id, org_id, user_id, name, prefix, token_hash, last_used_at, revoked_at, created_at)
sessions(id, user_id, token_hash, user_agent, ip, last_seen_at, expires_at, created_at)
password_resets(token_hash, user_id, expires_at, used_at, created_at)
notifications(id, user_id, org_id, kind, title, body, target_type, target_id, read_at, created_at)
webhooks(id, org_id, board_id, url, secret, events_json, active, created_at)
webhook_deliveries(id, webhook_id, event_id, status, attempts, response_code,
                   last_error, created_at)
```

**Aturan uang:** `cost_micros` = BIGINT micro-USD (1 USD = 1_000_000). **JANGAN pakai float.**
**Aturan ID:** ULID (TEXT 26) untuk entitas domain; BIGSERIAL untuk `events`, `steps`, `ledger_entries`, `audit_log`.
**Aturan isolasi:** setiap tabel ber-`org_id`; semua query WAJIB filter `org_id` (dicek test).

---

## 6A. Pricing, provider BYO, dan skill library (workstream Agent Registry)

**Keputusan ini mengikat. Angka & rumus di sini yang dipakai semua dokumen lain.**

### A. Biaya adalah ESTIMATE, bukan tagihan

`cost_micros` di seluruh ledger adalah **estimasi**, dihitung dari token × tabel harga internal.
AgentDeck **tidak pernah** mengklaim angka ini sebagai uang yang benar-benar keluar.

- Sumber tabel harga: port dari **9Router** (MIT License, `app/.next-cli-build/server/chunks/8920.js`),
  **220 entri exact + 51 pattern regex**, per 1 juta token, USD float.
  **Isi tabelnya ada di `docs/PRICING.md`** (di-generate `tools/gen_pricing.py` dari source 9Router).
  `docs/PRICING.md` adalah sumber angka; dokumen ini hanya aturannya.
- Satuan internal: **micro-USD per 1.000.000 token** (`MicrosPer1M = round(usd_per_1m × 1e6)`,
  `$5.00/1M → 5_000_000`). **Bukan** `round(usd_per_1m)` — pembulatan itu menolkan **397 harga**
  (mis. `deepseek-*` cached `$0.0028/1M`), sehingga cache hit jadi gratis tanpa ketahuan.
- Alasan: provider OAuth/langganan (Antigravity, Kiro, Codex) tidak mengekspos harga flat,
  dan model BYO tidak mengekspos harga sama sekali. Estimasi konsisten lebih berguna
  daripada nol atau angka karangan.
- **UI wajib menulis "estimate"** pada setiap angka biaya. Ini bukan hiasan: 9Router mencatat
  $62.97 pada 2026-09-06 untuk 1.987 request lewat `antigravity` — uang itu tidak pernah keluar.
- Jalur BYO: user melihat biaya sebenarnya di dashboard providernya sendiri.

### B. Rumus kanonik (5 komponen)

```
miss      = max(0, prompt_tokens − cached_tokens − cache_creation_tokens)
cost      = miss            × input/1e6
          + cached_tokens   × (cached         ?? input)/1e6
          + completion      × output/1e6
          + reasoning       × (reasoning      ?? output)/1e6
          + cache_creation  × (cache_creation ?? input)/1e6
```

**`reasoning` punya harga sendiri**, terpisah dari `output`. Terverifikasi eksak
(`max_err = 0.0000000000`, 13.140 baris) terhadap data produksi 9Router.
Di Claude selisihnya besar: `claude-opus-4.6` output 25 vs reasoning 37.5.

### C. Resolusi harga — 4 tingkat

| Tingkat | Sumber | `price_source` |
|---|---|---|
| 1 | override per-model milik org (`agent_model_prices`) | `manual` |
| 2 | tabel exact (220 model) | `catalog` |
| 3 | pattern regex (51 aturan) | `pattern` |
| 4 | tidak ada | `unpriced` |

**Default tingkat 3**: model tak dikenal tetap dapat estimasi lewat pattern generic.
User boleh menimpa per model lewat tingkat 1. Tingkat 4 (`unpriced`) hanya terjadi bila
nama model tidak cocok pattern mana pun — `cost_micros = 0`, ledger menandai `unpriced`.

### D. Satuan & pembulatan

- Tabel harga asal **USD per 1 juta token (float)**. Disimpan sebagai **micro-USD per 1.000.000 token**
  (`MicrosPer1M = round(usd_per_1m × 1e6)`, jadi `$5.00/1M → 5_000_000`). **Bukan**
  `round(usd_per_1m)`: pembulatan itu menolkan **397 harga** (mis. `deepseek-*` cached
  `$0.0028/1M`), sehingga cache hit jadi gratis tanpa ketahuan.
- Pembagian `1e6` di rumus §9.1 mengubah (token × micro-USD per 1M token) menjadi micro-USD.
- `cost_micros` = integer BIGINT micro-USD (1 USD = 1_000_000). **JANGAN float di DB.**
- Pembulatan dilakukan **satu kali** di akhir perhitungan baris, bukan per komponen.

### E. Snapshot harga di baris ledger

`ledger_entries` menyimpan `price_source` + `pricing_model` (nama entri/pattern yang benar-benar
dipakai). `price_version` integer **tidak cukup**: tabel pattern bisa berubah, dan override
per-org berbeda antar tenant. Tanpa dua kolom ini, baris lama tidak bisa dibuktikan.

### F. Provider BYO = provider terpisah

- `provider = 'openai_compatible'` + kolom `agents.base_url`.
- Kolom `provider` yang ada **TIDAK diganti** — US-AD67 (validasi `provider`+`model` ke tabel harga)
  dan US-AD68 (`price_version`) tetap berlaku apa adanya.
- **SSRF guard wajib**: `https` only, tolak IP private/loopback/link-local
  (termasuk `169.254.169.254`), jangan ikut redirect ke alamat private.
- Katalog model BYO diisi dari `GET {base_url}/models` (hanya **nama** model — endpoint itu
  tidak mengembalikan harga), lalu di-resolve lewat tingkat C di atas.

### G. Skill library — org-scoped, agent TIDAK boleh menulis

- Skill adalah **data**, bukan konstanta: tabel `agent_skills` berisi `body_md` (markdown).
- Cakupan **per org**. Default disediakan sistem (seed), user boleh menambah/mengubah miliknya.
- **Hanya owner/admin yang boleh menulis.** Agent **tidak pernah** boleh menulis skill.
  Alasan: `body_md` masuk ke prompt agent; agent yang bisa menulis skill = agent yang bisa
  menulis ulang instruksinya sendiri, lalu dipakai agent lain. Ini privilege escalation.
- Markdown **wajib di-sanitize** saat dirender. Render sebagai teks atau HTML tersanitasi, bukan `innerHTML` mentah.
- `agents.skills_json` tetap daftar slug; resolusi slug → `agent_skills` per org.

### H. Tools tertutup (enum, 9 primitif)

`read_file` · `write_file` · `edit_file` · `list_dir` · `search_files` · `bash` · `sql_query` · `http_fetch` · `git`

`bash` wajib lewat approval gate. `tools_json` adalah allowlist yang menggerbang eksekusi:
nama di luar daftar ini **ditolak 400**. Daftar ini adalah kontrak yang wajib diimplementasikan
executor (M4) — checkbox yang tidak punya tool nyata adalah grant palsu.

Skill default (seed, 8): `code_review` · `e2e_test` · `debug` · `refactor` · `test_write` · `docs` · `migration` · `security_review`

### I. `has_provider_key` adalah generated column, bukan kolom tersimpan

`agents.has_provider_key` = `GENERATED ALWAYS AS (provider_api_key_enc IS NOT NULL) STORED`.

Registry perlu tahu apakah agent membawa kredensial sendiri tanpa pernah membaca kredensialnya.
Dua alternatif ditolak:

1. **Kolom BOOLEAN biasa.** Harus ditulis ulang di setiap jalur yang mengubah kredensial. Satu
   jalur yang lupa (mis. `DELETE /provider-key`) meninggalkan `true` yang basi, dan agent
   berlabel "siap" padahal tidak punya key. Generated column tidak bisa melenceng dari sumbernya.
2. **Hitung di Go.** Berarti setiap query agent harus ikut men-`SELECT provider_api_key_enc`
   supaya mapper bisa mengujinya — dan ciphertext lalu mengalir ke setiap struct list/get, satu
   tag JSON atau satu baris log dari keluar dari proses. Dengan generated column, ciphertext
   tetap terkurung di satu pembaca: `GetAgentProviderKey`.

`IS NOT NULL` atas BYTEA bersifat immutable, jadi kolom ini sah sebagai `STORED`.

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
