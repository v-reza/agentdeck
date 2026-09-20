/**
 * The domain vocabulary, mirrored from DECISIONS 3 and 4. These unions are the
 * contract: a status or enum value that is not listed here does not exist, and
 * the compiler refuses to render it. Never widen one of these to `string` to
 * silence a type error — a status the API cannot return is a bug upstream.
 */

/** Task lifecycle, DECISIONS 3. Ten values, no more. */
export const TASK_STATUSES = [
  'backlog',
  'ready',
  'running',
  'awaiting_approval',
  'blocked',
  'review',
  'done',
  'failed',
  'cancelled',
  'archived',
] as const
export type TaskStatus = (typeof TASK_STATUSES)[number]

/** Terminal statuses: the dispatcher will never move these again. */
export const TERMINAL_STATUSES: readonly TaskStatus[] = ['done', 'failed', 'cancelled', 'archived']

/** DECISIONS 3: a column is a VIEW of status, not a status of its own. */
export type ColumnKey = 'backlog' | 'ready' | 'running' | 'review' | 'done'

export const COLUMN_ORDER: readonly ColumnKey[] = ['backlog', 'ready', 'running', 'review', 'done']

export const COLUMN_LABELS: Record<ColumnKey, string> = {
  backlog: 'Backlog',
  ready: 'Ready',
  running: 'Running',
  review: 'Review',
  done: 'Done',
}

/** Which column renders a status. Mirrors internal/board/types.go ColumnKey(). */
export function columnForStatus(status: TaskStatus): ColumnKey {
  switch (status) {
    case 'ready':
      return 'ready'
    case 'running':
    case 'awaiting_approval':
      return 'running'
    case 'review':
      return 'review'
    case 'done':
    case 'failed':
    case 'cancelled':
      return 'done'
    case 'backlog':
    case 'blocked':
    case 'archived':
      return 'backlog'
  }
}

/** The colour token for a status, DESIGN.md `status-*`. */
export function statusColorVar(status: TaskStatus): string {
  return `var(--color-status-${status.replace(/_/g, '-')})`
}

/** block_kind, DECISIONS 4. */
export const BLOCK_KINDS = ['dependency', 'needs_input', 'capability', 'policy', 'budget', 'external'] as const
export type BlockKind = (typeof BLOCK_KINDS)[number]

/** failure_kind, DECISIONS 4. */
export const FAILURE_KINDS = [
  'transient',
  'needs_input',
  'capability',
  'dependency',
  'policy',
  'budget',
  'unknown',
] as const
export type FailureKind = (typeof FAILURE_KINDS)[number]

/** retry_policy, DECISIONS 4. */
export const RETRY_POLICIES = ['never', 'transient_only', 'always'] as const
export type RetryPolicy = (typeof RETRY_POLICIES)[number]

/** approval_gate_mode, DECISIONS 4. */
export const APPROVAL_GATE_MODES = ['auto', 'require', 'deny'] as const
export type ApprovalGateMode = (typeof APPROVAL_GATE_MODES)[number]

/** approval_decision, DECISIONS 4. */
export const APPROVAL_DECISIONS = ['pending', 'approved', 'rejected', 'expired'] as const
export type ApprovalDecision = (typeof APPROVAL_DECISIONS)[number]

/** role, DECISIONS 4. `worker` is an agent credential, not a membership role. */
export const ROLES = ['owner', 'admin', 'member', 'viewer'] as const
export type Role = (typeof ROLES)[number]

/** workspace_kind, DECISIONS 4. */
export const WORKSPACE_KINDS = ['scratch', 'dir', 'worktree', 'container'] as const
export type WorkspaceKind = (typeof WORKSPACE_KINDS)[number]

/** run outcome, DECISIONS 3. */
export const RUN_OUTCOMES = ['succeeded', 'failed', 'timed_out', 'cancelled', 'reclaimed', 'budget_exceeded'] as const
export type RunOutcome = (typeof RUN_OUTCOMES)[number]

/** run status, DECISIONS 3. */
export const RUN_STATUSES = ['pending', 'claiming', 'running', 'ended'] as const
export type RunStatus = (typeof RUN_STATUSES)[number]

/** event_kind, DECISIONS 4 — the SSE stream carries exactly these. */
export const EVENT_KINDS = [
  'task.created',
  'task.status_changed',
  'task.assigned',
  'run.claimed',
  'run.heartbeat',
  'run.finished',
  'run.reclaimed',
  'step.started',
  'step.finished',
  'step.failed',
  'approval.requested',
  'approval.decided',
  'approval.expired',
  'artifact.created',
  'ledger.entry',
  'comment.created',
  'budget.threshold_crossed',
] as const
export type EventKind = (typeof EVENT_KINDS)[number]

/** Type guard for values arriving from the wire (SSE payloads, query params). */
export function isTaskStatus(value: unknown): value is TaskStatus {
  return typeof value === 'string' && (TASK_STATUSES as readonly string[]).includes(value)
}

/**
 * Wire entities. These mirror the JSON the Go handlers actually emit
 * (`cmd/api/boards.go` `toTaskResponse`, `toBoardResponse`, `toProjectResponse`),
 * field name for field name. They are deliberately not shared with the server
 * types: the API is the contract, and a rename on either side has to be a
 * deliberate edit here rather than an accidental structural match.
 */

/** One lane of a board. A column groups statuses; it is never a status. */
export interface Column {
  key: string
  name: string
}

export interface Project {
  id: string
  org_id: string
  slug: string
  name: string
  created_at: string
}

export interface Board {
  id: string
  org_id: string
  project_id: string
  slug: string
  name: string
  columns: Column[]
  budget_daily_micros: number
  created_at: string
}

export interface Task {
  id: string
  org_id: string
  board_id: string
  title: string
  body: string
  status: TaskStatus
  priority: number
  assignee_agent_id: string
  created_by: string
  idempotency_key: string
  block_kind: string
  consecutive_failures: number
  workspace_kind: string
  goal_mode: string
  goal_max_turns: number
  current_run_id: string
  cost_micros: number
  tokens_in: number
  tokens_out: number
  created_at: string
  started_at: string
  completed_at: string
  archived_at: string
}

/** One dependency edge of the task DAG. */
export interface TaskLink {
  ParentID: string
  ChildID: string
}

/** One append-only row of the board timeline. */
export interface TaskEvent {
  ID: number
  OrgID: string
  BoardID: string
  TaskID: string
  RunID: string
  Kind: EventKind | string
  PayloadJSON: string | null
  CreatedAt: string
}

export interface Agent {
  id: string
  org_id: string
  project_id: string
  name: string
  provider: string
  model: string
  reasoning_effort: string
  skills: string[]
  tools: string[]
  max_runtime_seconds: number
  retry_policy: RetryPolicy | string
  max_attempts: number
  has_provider_key: boolean
  created_at: string
}

/**
 * Provider key metadata. There is intentionally no `value`/`api_key` field: the
 * server never returns the plaintext key, so the client cache cannot hold one.
 */
export interface ProviderKeyState {
  agent_id: string
  has_key: boolean
  last_rotated_at: string
}

/** One membership row of GET /orgs. */
export interface Workspace {
  id: string
  name: string
  slug: string
  role: Role
  kind: WorkspaceKind
}

/** GET /auth/me. */
export interface MeResponse {
  id: string
  email: string
  name: string
  /**
   * US-AD89 AC1: the four attributes the payload carries are id, email, name,
   * and `avatar_user`. The avatar is a server object — the screen draws the
   * monogram the API returned instead of computing a second one that could
   * disagree with it.
   */
  avatar_user: AvatarUser
  workspaces: Workspace[]
}

/**
 * The `avatar_user` object of GET /auth/me. `kind` is `monogram` for an account
 * with no uploaded image (then `initials`/`bg_color`/`size_px` describe the disc
 * the rail draws) and `image` once `url` is set.
 */
export interface AvatarUser {
  kind: 'monogram' | 'image'
  initials: string
  bg_color: string
  size_px: number
  url?: string
}

/** One row of the approvals inbox (ARCHITECTURE 6.2.13). */
export interface Approval {
  id: string
  org_id: string
  task_id: string
  run_id: string
  requested_by: string
  decided_by: string
  decision: ApprovalDecision
  gate_mode: ApprovalGateMode
  reason: string
  /** The exact proposal the agent wants to apply; rendered verbatim. */
  preview_json: string | null
  expires_at: string
  decided_at: string
  created_at: string
}

/** One immutable LLM-call cost row. Integer micro-USD only (DECISIONS 6). */
export interface LedgerEntry {
  id: number
  org_id: string
  run_id: string
  task_id: string
  provider: string
  model: string
  kind: string
  tokens_in: number
  tokens_out: number
  cache_read_tokens: number
  cache_write_tokens: number
  cost_micros: number
  price_version: string
  created_at: string
}

/** GET /boards/{id}/budget — realtime usage against the N16 daily cap. */
export interface BoardBudget {
  board_id: string
  day: string
  budget_daily_micros: number
  spent_micros: number
  run_count: number
  tokens_in: number
  tokens_out: number
  /** N18: the 80% alert threshold, reported by the server, not recomputed. */
  threshold_crossed: boolean
}

/** GET /orgs/{id}/cost-summary — 30-day totals grouped by model and board. */
export interface CostSummary {
  total_micros: number
  by_model: { model: string; provider: string; cost_micros: number; runs: number }[]
  by_board: { board_id: string; name: string; cost_micros: number; runs: number }[]
}

/**
 * One frame of the SSE stream (ARCHITECTURE 7.2). `id` is the resume cursor the
 * browser replays with Last-Event-ID, so it must be kept even though the UI
 * does not render it.
 */
export interface EventEnvelope {
  id: string
  kind: EventKind | string
  board_id: string
  task_id: string
  run_id: string
  payload: unknown
  created_at: string
}

/** EN/ID, ARCHITECTURE 18.2 langSlice. */
export type Lang = 'en' | 'id'
