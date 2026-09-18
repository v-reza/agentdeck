## AgentDeck v0.1.0

AgentDeck v0.1.0 is the first public preview of the self-hosted AI agent fleet orchestration board.

### Included

- Public landing page for AgentDeck's core positioning: cost visibility, approval gates, and self-hosted operation.
- Public docs pages for Quickstart, REST API, and run telemetry schema.
- Pricing page with Solo free tier and Pro at **$5/month flat**.
- GitHub, community, changelog, about, contact, download, login, and register public surfaces.
- Product roadmap covering M0 Identity & Workspace through M6 Operator Quality-of-Life.
- Frozen product contracts for the Go + PostgreSQL runtime shape.
- PRD, architecture, design-token, database, endpoint, and traceability documentation.
- Verification tooling for PRD coverage, design tokens, render structure, and suite consistency.
- Responsive public UI for desktop and mobile.

### Architecture direction

- Go single binary
- PostgreSQL as the stateful dependency
- PostgreSQL `SKIP LOCKED` for dispatcher claims
- SSE for realtime events
- Integer micro-USD cost accounting
- Human approval as a first-class run state
- No Redis, Kafka, or Kubernetes in the core deployment

### Current limitations

This release is a public frontend and product-contract preview. The backend runtime, authentication API, dispatcher, worker execution, cost ledger persistence, approvals API, SSE server, and artifact storage are not shipped yet. They are tracked in `docs/ROADMAP.md` across milestones M0–M6.

### Verification

- `npm run build` passes.
- `python tools/verify_suite.py` passes with 0 failures.
- Public routes render on desktop and mobile without horizontal overflow.
- GitHub release metadata is consumed from the public Releases API on `/github`.
