# AgentDeck

AgentDeck is a self-hosted orchestration board for AI agent fleets. It keeps every run's cost visible, gates risky actions behind approval, and runs as a single Go + Postgres deployment.

This repository is currently at **v0.1**. The public landing/docs frontend is implemented first; backend services are planned next.

## Frontend

The public web app lives in `apps/web` and uses Vite, React, and TypeScript.

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
cd apps/web
npm run build

cd ../..
python tools/verify_prd.py docs
python tools/verify_suite.py
```

## Repository layout

- `apps/web` — public React/Vite frontend
- `docs` — PRD, architecture, decisions, design tokens, and coverage
- `design` — Stitch source artifacts, prompts, references, and landing source
- `tools` — specification and render verification scripts
- `archive` — historical reports

## Status

The landing and public documentation surfaces are implemented and responsive. Authentication forms and public pages are currently presentation-only; API and backend wiring will follow in the next implementation phase.
