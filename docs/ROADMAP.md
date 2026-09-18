# AgentDeck roadmap

AgentDeck dibangun sebagai control plane kecil untuk fleet AI agent: board untuk kerja, ledger untuk biaya, trace untuk observabilitas, dan approval gate untuk aksi berisiko.

## Current release — v0.1

**Status:** public preview. Landing page, public docs, pricing, repository page, community page, dan changelog sudah tersedia. Backend Go, database runtime, auth API, dispatcher, dan worker execution belum diimplementasikan di repository ini.

| Area | Yang sudah ada | Status |
|---|---|---|
| Product contract | PRD, architecture, frozen decisions, database/API contract | Design complete |
| Public web | Landing, pricing, docs quickstart/API/telemetry, GitHub, community, changelog | Frontend preview |
| Deployment shape | Single Go binary + PostgreSQL, tanpa Redis/Kafka/Kubernetes | Architecture locked |
| Runtime | Auth, board, dispatcher, ledger, approvals, SSE | Next implementation |

## Milestones

### M0 — Identity and workspace foundation

**Outcome:** pengguna bisa daftar, login, dan langsung masuk ke workspace personal tanpa setup organisasi manual.

- Email/password registration dan opaque server-side sessions
- Logout, password reset, profile, dan forced session expiry
- Personal workspace otomatis saat registrasi
- Multi-tenant isolation dengan `org_id` di setiap query
- Owner/admin/member/viewer RBAC
- Member invitations, role changes, dan last-admin/owner protection

**Priority:** Must · **State:** backend next

### M1 — Board and task loop

**Outcome:** satu developer bisa membuat project, board, task, dan menjalankan lifecycle task dari backlog sampai done.

- Project dan board CRUD
- Default columns: Backlog, Ready, Running, Review, Done
- Task create/edit, priority, assignee, comments, dan dependencies
- Drag task antar kolom dengan event append-only
- Task detail drawer dengan history dan cost summary
- Atomic dispatcher claim menggunakan PostgreSQL `SKIP LOCKED`
- Worker heartbeat, lease expiry, reclaim, dan idempotency

**Priority:** Must · **State:** backend next

### M2 — Cost and agent control

**Outcome:** operator tahu biaya setiap run dan bisa mengatur agent/provider tanpa hidden spend.

- Agent registry: provider, model, tools, skills, retry policy
- Encrypted provider API keys dan rotation
- Immutable per-step cost ledger dalam integer micro-USD
- Versioned price snapshots agar old runs tetap reproducible
- Daily budget per board dan budget threshold events
- Cost rail, cost overview, ledger explorer, dan CSV export

**Priority:** Must · **State:** contract/design complete, backend next

### M3 — Human approval and realtime operations

**Outcome:** aksi berisiko berhenti menunggu manusia, sementara operator melihat perubahan tanpa refresh.

- Approval gate mode: `auto`, `require`, `deny`
- Approval inbox dan approval detail dengan exact diff/tool payload
- Approve, reject, expire, dan timeout behavior
- SSE event stream untuk board, task, run, approval, dan ledger
- Run detail dengan timeline Step → tool call → payload
- Replay trace untuk debugging dan audit

**Priority:** Must · **State:** contract/design complete, backend next

### M4 — Reliability and artifact loop

**Outcome:** kegagalan transient pulih otomatis dan hasil run bisa direproduksi.

- Failure taxonomy: transient, needs_input, capability, dependency, policy, budget
- Exponential backoff dan max-attempt policy
- Dead-letter/failed state dengan retry to ready
- R2 artifact storage dengan size limits dan presigned downloads
- Run timeline, artifact viewer, dan structured error states
- EN/ID i18n dengan safe fallback

**Priority:** Must · **State:** contract/design complete, backend next

### M5 — Governance and integrations

**Outcome:** tim kecil bisa mengaudit akses, mengintegrasikan AgentDeck, dan mengelola programmatic access.

- Audit log dan activity explorer
- API key CRUD dengan one-time secret display
- Webhook delivery, signatures, retries, dan delivery history
- Dense table view untuk operasi volume tinggi
- Session revocation untuk admin/owner
- Security settings dan retention controls

**Priority:** Must/Should · **State:** contract/design complete, backend later

### M6 — Operator quality-of-life

**Outcome:** operasi harian lebih cepat tanpa menambah platform sprawl.

- Command palette (`Cmd+K`)
- Bulk task actions
- Advanced filters dan saved views
- Better empty/loading/error states
- Public docs, pricing, changelog, dan self-serve account closure
- Performance hardening: read P95 ≤150ms, first paint ≤400ms

**Priority:** Should/Could · **State:** partial public frontend, backend later

## After v1

- Enterprise SAML/SCIM SSO
- Multi-region SLA
- Optional visual/3D module outside the main board
- Additional provider adapters and deployment installers

## Product guardrails

- AgentDeck mengorkestrasi agent; bukan inference host dan bukan generic project management.
- Satu Go binary + PostgreSQL tetap menjadi baseline deployment.
- Tidak ada Redis, Kafka, Kubernetes, atau payment gateway di core v0.x.
- Setiap fitur runtime harus punya tenant isolation, permission rule, failure path, dan audit/event behavior.
- Roadmap status tidak berarti shipped sampai ada implementation dan verification evidence.
