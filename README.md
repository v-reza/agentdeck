<p align="center">
  <img src="assets/logo-wordmark.svg" alt="AgentDeck — self-hosted AI agent fleet orchestration" width="520">
</p>

<p align="center">
  <strong>A self-hosted board for AI agent fleets: every run's cost visible, every risky action behind an approval gate, one Go binary and Postgres.</strong>
</p>

<p align="center">
  <a href="https://github.com/v-reza/agentdeck/releases"><img src="https://img.shields.io/github/v/release/v-reza/agentdeck?color=0d7a70" alt="Release"></a>
  <img src="https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white" alt="Go 1.22">
  <img src="https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white" alt="PostgreSQL 16">
  <a href="CHANGELOG.md"><img src="https://img.shields.io/badge/changelog-keep%20a%20changelog-0d7a70" alt="Changelog"></a>
</p>

<p align="center">
  <a href="#quickstart">Quickstart</a> &bull;
  <a href="#architecture">Architecture</a> &bull;
  <a href="#verification">Verification</a> &bull;
  <a href="docs/ARCHITECTURE.md">Architecture doc</a> &bull;
  <a href="docs/00-PRD.md">PRD</a> &bull;
  <a href="CHANGELOG.md">Changelog</a>
</p>

---

AgentDeck is an orchestration board for AI agent fleets. You register agents, put
work on a board, and let a dispatcher claim and run it — while every step's token
spend and cost lands in a ledger, and anything sensitive stops at a human
approval gate.

It is built for people who run their own agents: solo builders and small teams who
self-host and do not want a per-seat SaaS bill. Deploy it as a single Go binary
plus PostgreSQL.

## Why this exists

| Problem | What AgentDeck does |
|---|---|
| Agent runs are a black box you pay for monthly | Per-run and per-step cost ledger in integer micro-USD — no floating-point money |
| "It did something in production" | Approval gates on sensitive actions, with an inbox and a full audit trail |
| Per-seat pricing punishes adding a viewer | Self-hosted; a Solo workspace is free and roles are a feature, not a paywall |
| Opaque vendor state you cannot inspect | PostgreSQL you own, with the schema, endpoints, and PRD in this repo |

## Status

Honest scope: **M0 is complete and M1 is in progress.** This is pre-1.0 software —
the API may change between minor versions.

- **M0 — Identity & Workspace: done.** Registration, sessions, the four-role RBAC
  matrix, tenant isolation, password reset, members, workspace settings with an
  atomic rename plus audit entry, and the multi-workspace switcher.
- **M1 — Board: in progress.** Projects, board creation, and board column editing
  are implemented. Tasks, drag-to-move, the task drawer, agent assignment, and the
  agent registry are next.
- **M2–M4 — Cost, approvals, and the run executor: not started.** The frozen
  contracts for them live in `docs/`.

Story-by-story progress is tracked in [docs/CHECKLIST.md](docs/CHECKLIST.md), and
every user-visible change is recorded in [CHANGELOG.md](CHANGELOG.md).

> **License: not yet declared.** This repository ships no `LICENSE` file, so the
> default "all rights reserved" applies. See [License](#license) before you reuse
> the code.

## Architecture

```
  browser ──► web (Vite dev server / static build)
                  │
                  │  /api/v1  (session cookie + X-Org-ID)
                  ▼
              api (single Go binary) ──► PostgreSQL 16
                  │                        sessions, orgs, boards, tasks, ledger
                  └──► dispatcher ──► runs ──► LLM provider
                         (claims ready tasks, enforces budget + retry)
```

- **Go single binary** — `cmd/api`. No sidecar services to deploy.
- **PostgreSQL 16** is the only stateful dependency, accessed with `pgx/v5` and
  `sqlc`-generated queries. Migrations apply on boot inside one advisory-locked
  transaction and are recorded in `schema_migrations`.
- **React 19 + Vite** SPA in `frontend/` — Tailwind v4, Redux Toolkit with RTK
  Query for all server state, React Router v7, dnd-kit.
- **Money is integer micro-USD** (1 USD = 1,000,000) end to end; a float never
  crosses the boundary. Costs are rendered at two decimals, except a sub-cent
  amount, which keeps its full micro precision so real spend cannot round to
  `$0.00`.
- **Tenant scope is server-side.** Every org-scoped handler resolves the tenant
  through one middleware chain — session → `X-Org-ID` membership → role gate — and
  never trusts a path id as the scope. A foreign org id is `403`; an unknown one is
  `404`.

## Quickstart

### Docker Compose

```bash
docker compose up -d
curl -s localhost:8080/readyz   # readyz=200 once the schema is applied
```

The stack is `db` (PostgreSQL 16), `api` (Go), and `web` (Vite).

### Local Go API

Requires Go 1.22+ and PostgreSQL 16. Bring up the database first — the exact
image the test suite is validated against:

```bash
docker run -d --name agentdeck-postgres \
  -e POSTGRES_USER=agentdeck -e POSTGRES_PASSWORD=*** \
  -e POSTGRES_DB=agentdeck -p 5433:5432 postgres:16
```

```bash
cp .env.example .env            # then edit the two DSNs
go run ./cmd/api
```

The API loads `.env` without an extra dependency; real process environment
variables take precedence. `DATABASE_URL` is the restricted runtime role, while
`MIGRATION_DATABASE_URL` is the owner role used only to apply migrations at
startup. If both roles are the same locally, point both variables at one DSN.

`GET /healthz` is process health. `GET /readyz` verifies the live PostgreSQL
connection and returns `503` when it is unavailable.

### Frontend

```bash
cd frontend
npm install
npm run dev
```

Then open `http://127.0.0.1:5173/`.

### Public pages

| Route | What it is |
|---|---|
| `/` | Landing page |
| `/pricing` | Solo and Pro pricing ($5/month flat, not per seat) |
| `/docs/quickstart`, `/docs/api`, `/docs/telemetry` | Quickstart, REST API reference, run telemetry schema |
| `/github` | Repository, release history, and the product roadmap |
| `/community`, `/changelog` | Support channels and release history |
| `/login`, `/register`, `/reset` | Authentication |

## Verification

```bash
# Go — unit tests need no database
gofmt -l . && go vet ./... && go test ./...

# Go — Postgres-backed suites (migrations, tenant isolation, auth path).
# -p 1 keeps the suites from racing on the migration advisory lock.
export AGENTDECK_TEST_DATABASE_URL=postgres://agentdeck:***@localhost:5433/agentdeck
go test ./... -count=1 -p 1

# Frontend
cd frontend
npx tsc -b && npx vitest run && npx playwright test

# Specification gates (PRD coverage, design tokens, render structure)
cd ..
python tools/verify_prd.py docs
python tools/verify_suite.py
python tools/verify_web.py
```

The Postgres-backed suites skip silently when `AGENTDECK_TEST_DATABASE_URL` is
unset, so a checkout without a database still runs everything else.

The Playwright suite drives the real Vite dev server against the real Go API — a
pass means the browser talked to the backend, not that a mock answered. Start the
API on `:8080` first.

## Repository layout

| Path | Contents |
|---|---|
| `cmd/api/` | Go entrypoint, HTTP handlers, and route registration |
| `internal/auth/` | Sessions, RBAC, and the membership model |
| `internal/board/` | Projects, boards, columns, tasks, and the dependency DAG |
| `internal/migrate/` | Versioned `*.up.sql` migrations |
| `internal/store/` | `sqlc`-generated queries and models |
| `frontend/` | React 19 + Vite SPA, including the Playwright suite in `frontend/e2e/` |
| `docs/` | PRD, architecture, decisions, design tokens, and coverage |
| `design/` | Design source artifacts and screen prompts |
| `tools/` | Specification and render verification scripts |

## Contributing

Issues and pull requests are welcome at
[github.com/v-reza/agentdeck](https://github.com/v-reza/agentdeck).

Two conventions the repository enforces rather than suggests:

- **The contract documents win.** `docs/ARCHITECTURE.md`, `docs/DECISIONS.md`, and
  `docs/00-PRD.md` are constraints, not suggestions. Where a document and a
  convenience disagree, the document wins and the disagreement is written down.
- **A fix ships with the test that fails without it.** Prefer a test that fails
  for the *right* reason over one that merely passes.

## License

**No license has been declared yet.** There is no `LICENSE` file in this
repository, which means the default copyright applies: the code is public to read
and evaluate, but not licensed for reuse, modification, or redistribution.

If you need a license for a specific use, open an issue and ask. Until one is
chosen, treat this as "source available", not open source.
