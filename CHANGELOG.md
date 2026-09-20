# Changelog

All notable changes to AgentDeck are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Versions below `1.0.0` are pre-release: the API may change between minor versions
until the milestone plan in [docs/PLAN-M0-M4.md](docs/PLAN-M0-M4.md) completes.

## [0.2.0] - 2026-09-20

Milestone M1 — the board itself. Tracked story by story in
[docs/CHECKLIST.md](docs/CHECKLIST.md).

### Added

- **Board column editing** (`PATCH /api/v1/boards/{id}/columns`) — add, rename,
  remove, and reorder the columns of a board. The request carries the whole
  ordered layout, so a concurrent reorder cannot silently drop a column.
  ([US-AD10](docs/00-PRD.md))
- **Solo workspace seeding** — registering creates one `Getting started` project,
  so a single-user workspace has somewhere to put its first board instead of an
  empty screen. ([US-AD09](docs/00-PRD.md) AC4)

### Fixed

- **Column edits are admin-only, and can no longer be smuggled through a
  member-level route.** `PATCH /api/v1/boards/{id}` accepted a `columns` field
  while being gated at `Member`, which made the `admin` requirement on column
  edits bypassable by any member. The layout now has its own route with its own
  gate, and the board update route rejects the field. ([US-AD10](docs/00-PRD.md) AC4)
- **Board rename actually renames.** `UpdateBoardName` existed in the repository
  but no service method called it and the handler ignored the `name` field, so
  the Board Settings form accepted a new name and discarded it.
- **Removing a column that holds tasks is refused** with `409 Conflict` instead of
  orphaning the tasks. The check maps statuses to columns through the same
  `ColumnKey()` mapping the board renders with, so a column holding
  `awaiting_approval` / `failed` / `cancelled` work is not mistaken for empty.
  ([US-AD10](docs/00-PRD.md) AC2)
- **Duplicate column names are refused** with `409 Conflict`. Only the internal
  column *key* was checked before, so two columns could both read `Review` on the
  board. ([US-AD10](docs/00-PRD.md) AC3)

## [0.1.0] - 2026-09-18

The first public preview: the public surfaces, the frozen contracts, and the M0
identity and workspace foundation.

### Added

- Public landing page covering AgentDeck's positioning: per-run cost visibility,
  approval gates for risky actions, and self-hosted operation.
- Public docs for Quickstart, REST API, and the run telemetry schema.
- Pricing page with a free Solo tier and Pro at **$5/month flat** (not per seat).
- GitHub, community, changelog, about, contact, download, login, and register
  surfaces.
- Product roadmap from M0 Identity & Workspace through M6 Operator
  Quality-of-Life.
- Frozen product contracts for the Go + PostgreSQL runtime shape.
- PRD, architecture, design-token, database, endpoint, and traceability
  documentation.
- Verification tooling for PRD coverage, design tokens, render structure, and
  suite consistency.
- Responsive public UI for desktop and mobile.
- **M0 backend** — authentication, sessions, RBAC, and tenant isolation on
  PostgreSQL 16 with `pgx` and `sqlc`.
- **M0 workspace features** — members and roles, workspace settings with an atomic
  rename plus audit entry, and the multi-workspace context switcher.
- **Password reset** — single-use tokens with expiry, a uniform `410` for a
  refused token, and a log fallback when no mailer is configured.
- **Frontend foundation** — React 19 + Vite, Tailwind v4, Redux Toolkit with RTK
  Query for all server state, React Router v7, and a shared modal primitive with
  focus trap, Escape, and scroll lock.

[0.2.0]: https://github.com/v-reza/agentdeck/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/v-reza/agentdeck/releases/tag/v0.1.0
