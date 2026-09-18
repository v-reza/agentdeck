# AgentDeck

AgentDeck is a self-hosted orchestration board for AI agent fleets. It keeps every run's cost visible, gates risky actions behind approval, and runs as a single Go + Postgres deployment.

This repository is currently at **v0.1**. The public landing/docs frontend is implemented first; backend services are planned next.

## Frontend

The public web app lives in `apps/web` and uses Vite, React, and TypeScript.

## Backend (M0)

The Go API requires Go 1.22+ and PostgreSQL. Set `DATABASE_URL` and start the
API; the baseline schema applies on boot and is safe to re-apply:

```bash
export DATABASE_URL=postgres://agentdeck:agentdeck@localhost:5432/agentdeck
go run ./cmd/api
```

`GET /healthz` is process health; `GET /readyz` verifies the live PostgreSQL connection and returns 503 when it is unavailable. Auth domain primitives and executable tests live in `internal/auth`; `internal/migrate` applies `internal/migrate/*.up.sql` inside one advisory-locked transaction and records each applied version in `schema_migrations`.

Authorization (M0) covers the frozen four-role matrix and tenant isolation:

- `GET/POST /api/v1/orgs` — list and create orgs; the creator becomes `owner`.
- `GET/PATCH /api/v1/orgs/{id}` — org detail (viewer+) and rename (owner only).
- `GET/POST /api/v1/orgs/{id}/members` — roster (viewer+) and invite (admin+).
- `PATCH/DELETE /api/v1/orgs/{id}/members/{user_id}` — change role or remove (admin+).

Every org-scoped handler resolves the tenant through the same middleware chain:
session -> `X-Org-ID` membership check -> role gate. The `{id}` in the path is
never trusted as the tenant scope, so a foreign org id yields `403` (not a
foreign roster) and an unknown id yields `404` (US-AD07 AC1/AC3). Role values are
validated against the frozen enum; `owner` is only assigned at registration or
org creation, so no admin can escalate anyone past their own rank.

Unit tests run anywhere; the Postgres-backed migration and tenant-isolation tests need a live database and are gated on `AGENTDECK_TEST_DATABASE_URL`:

```bash
go test ./...                                    # unit tests, no database needed
AGENTDECK_TEST_DATABASE_URL=$DATABASE_URL \
  go test ./internal/migrate/ -count=1           # migration + tenant isolation
```


```bash
cd apps/web
npm install
npm run dev
```

Then open `http://127.0.0.1:5173/`.

Available public pages include:

- `/` — landing page
- `/pricing` — Solo and Pro pricing (`$5/month flat`)
- `/docs/quickstart` — quickstart
- `/docs/api` — REST API reference
- `/docs/telemetry` — run telemetry schema
- `/github` — repository, release history, and product roadmap
- `/community` — support channels
- `/changelog` — release history
- `/login` and `/register` — auth UI shells

## Verification

```bash
# Frontend
cd apps/web
npm run build

cd ../..
python tools/verify_prd.py docs
python tools/verify_suite.py

# Backend unit tests (no database needed)
rtk go test ./... -count=1

# Backend Postgres tests (needs a live Postgres 16)
export AGENTDECK_TEST_DATABASE_URL=postgres://agentdeck:agentdeck@localhost:5433/agentdeck
rtk go test ./internal/migrate/ -count=1
```

## Repository layout

- `apps/web` — public React/Vite frontend
- `docs` — PRD, architecture, decisions, design tokens, and coverage
- `design` — Stitch source artifacts, prompts, references, and landing source
- `tools` — specification and render verification scripts
- `archive` — historical reports

## Status

The landing and public documentation surfaces are implemented and responsive. Authentication forms and public pages are currently presentation-only; API and backend wiring will follow in the next implementation phase.
