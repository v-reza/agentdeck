## AgentDeck v0.2.0

Milestone **M1 — the board** is underway. This release makes the board real: you
can shape its columns, and a new workspace no longer starts empty.

Pre-1.0: the API may change between minor versions until the milestone plan in
[`archive/reports/PLAN-M0-M4.md`](archive/reports/PLAN-M0-M4.md) completes.

### Added

- **Board column editing** — add, rename, remove, and reorder a board's columns
  through `PATCH /api/v1/boards/{id}/columns`, with a panel that follows the
  `22-column-editor` design: drag handles, per-column task counts, and an inline
  error surface.
- **Column editor panel** on Board Settings, reachable from the Columns section.
- **Solo workspace seeding** — registering now creates one `Getting started`
  project, so a single-user workspace has somewhere to put its first board instead
  of facing an empty screen.

### Fixed

- **Column edits could be smuggled through a member-level route.** `PATCH
  /api/v1/boards/{id}` accepted a `columns` field while being gated at `Member`,
  so the `admin` requirement on column edits was bypassable by any member — the
  gate was decorative. The layout now has its own route with its own gate, and the
  board update route rejects the field outright.
- **Board rename silently did nothing.** `UpdateBoardName` existed in the
  repository but no service method called it, and the handler ignored the `name`
  field: the Board Settings form accepted a new name and discarded it.
- **Removing a column that holds tasks no longer orphans the work.** It is refused
  with `409 Conflict`. The check maps statuses to columns through the same
  `ColumnKey()` mapping the board renders with, so a column holding
  `awaiting_approval`, `failed`, or `cancelled` work is not mistaken for empty and
  silently deleted.
- **Duplicate column names are refused** with `409 Conflict`. Only the internal
  column *key* was checked before, so two columns could both read `Review` on the
  board while carrying different keys.
- **The README no longer claims the backend is unimplemented.** It described the
  API as "planned next" and the auth screens as "presentation-only", which stopped
  being true when M0 landed.

### Notes

- The column editor is gated at `admin` per US-AD10 AC4, while a solo workspace
  (the B2C case) reaches it as `owner` with no RBAC setup.
- The README header uses a new brand mark in `assets/`. The previous
  `frontend/public/favicon.svg` path draws a letter "E", not the "AD" the app's own
  rail badge shows; the new `assets/logo.svg` is a geometric "A" and is the one to
  reuse.
- **No license is declared.** This repository ships no `LICENSE` file, so the
  default copyright applies — public to read and evaluate, not licensed for reuse.

### Verification

- `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` passing.
- Frontend: `tsc -b` clean, Vitest passing, Playwright suite passing against the
  real Go API.
- `python tools/verify_web.py` — 0 FAIL, 0 warnings.
- Each US-AD10 fix was mutation-checked: reverting the fix makes the new test fail,
  so the tests are load-bearing rather than merely green.
