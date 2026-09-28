import { baseApi } from './base'
import type { Approval, Task } from '@/lib/domain'

/**
 * The dashboard's read-only queries (US-AD76).
 *
 * Three of the four metric cards are org-wide, and the contract has exactly one
 * org-wide endpoint per card — no aggregation endpoint, and no need for one:
 *
 *  - `GET /search/runs` is org-scoped by `runs.org_id` (query `SearchRuns`),
 *    so `?outcome=` and `?limit=` answer "running now" and "failed in the last
 *    24h" without a per-board fan-out.
 *  - `GET /approvals` is the org-wide pending queue (`ApprovalInbox(orgID)`).
 *
 * Spend comes from `finops.ts`, which already defines `orgCostSummary` for the
 * cost screen. Redefining it here would register a second endpoint under the
 * same name in the same `baseApi` — the later injection wins and the earlier
 * call sites silently change shape. One endpoint, one definition.
 *
 * `search/runs` returns `{runs: [...]}` and `search/tasks` returns
 * `{tasks: [...]}` — both wrapped, unlike the list endpoints elsewhere. The
 * wrappers are kept rather than unwrapped so the response shape stays visible
 * at the call site.
 */

/** One run as `GET /search/runs` returns it. */
export interface RunSearchRow {
  id: string
  org_id: string
  task_id: string
  agent_id: string
  attempt: number
  status: string
  outcome: string
  failure_kind: string
  cost_micros: number
  tokens_in: number
  tokens_out: number
  summary: string
  error: string
  started_at: string
  ended_at: string
}

/** `GET /search/runs` filters. Empty strings are omitted by the query builder. */
export interface RunSearchArgs {
  outcome?: string
  failureKind?: string
  taskID?: string
  q?: string
  limit?: number
}

const runsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    searchRuns: build.query<RunSearchRow[], RunSearchArgs | void>({
      query: (args) => {
        const params = new URLSearchParams()
        if (args?.outcome) params.set('outcome', args.outcome)
        if (args?.failureKind) params.set('failure_kind', args.failureKind)
        if (args?.taskID) params.set('task_id', args.taskID)
        if (args?.q) params.set('q', args.q)
        if (args?.limit) params.set('limit', String(args.limit))
        const query = params.toString()
        return query ? `search/runs?${query}` : 'search/runs'
      },
      transformResponse: (response: { runs: RunSearchRow[] }) => response.runs ?? [],
      providesTags: [{ type: 'Run', id: 'SEARCH' }],
    }),
    searchTasks: build.query<Task[], { q: string; boardID?: string; limit?: number }>({
      query: ({ q, boardID, limit }) => {
        const params = new URLSearchParams({ q })
        if (boardID) params.set('board_id', boardID)
        if (limit) params.set('limit', String(limit))
        return `search/tasks?${params.toString()}`
      },
      transformResponse: (response: { tasks: Task[] }) => response.tasks ?? [],
      providesTags: [{ type: 'Task', id: 'SEARCH' }],
    }),
    approvalInbox: build.query<Approval[], void>({
      query: () => 'approvals',
      providesTags: (result) =>
        result
          ? [...result.map((a) => ({ type: 'Approval' as const, id: a.id })), { type: 'Approval' as const, id: 'LIST' }]
          : [{ type: 'Approval' as const, id: 'LIST' }],
    }),
  }),
})

export const { useSearchRunsQuery, useSearchTasksQuery, useApprovalInboxQuery } = runsApi
