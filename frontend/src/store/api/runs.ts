import { baseApi } from './base'

/**
 * Runs and their steps (US-AD26, US-AD94, US-AD41, ARCHITECTURE 6.2.11).
 *
 * `GET /tasks/{id}/runs`, `GET /runs/{id}`, `GET /runs/{id}/summary`, and
 * `GET /runs/{id}/steps` have all been live since the run lifecycle landed, and
 * none of them had a caller — the Logs tab and the run screen were waiting on a
 * client that did not exist, not on an endpoint.
 */

export interface Run {
  id: string
  org_id: string
  task_id: string
  agent_id: string
  attempt: number
  status: string
  outcome: string
  failure_kind: string
  last_heartbeat_at: string
  max_runtime_seconds: number
  cost_micros: number
  tokens_in: number
  tokens_out: number
  summary: string
  error: string
  started_at: string
  ended_at: string
  cancel_requested_at: string
}

/**
 * One step of a run. `payload_json` arrives as a JSON STRING, not an object:
 * the column is JSONB and the handler echoes it as the text Postgres returned
 * rather than parsing and re-encoding it (see `toStepResponse`). Parsing is
 * therefore the reader's job, and the parse can fail — a payload is only
 * required to be valid JSON *at the moment it was written*.
 */
export interface Step {
  seq: number
  kind: string
  name: string
  status: 'running' | 'succeeded' | 'failed'
  tokens_in: number
  tokens_out: number
  cost_micros: number
  started_at: string
  ended_at: string
  payload_json: string | null
}

/**
 * What `GET /runs/{id}/summary` answers (US-AD41 AC1).
 *
 * The run row carries `started_at` and `ended_at`, so this looks redundant and
 * is not: for a run that has NOT ended, the server measures the duration against
 * `now()`, which is not a field the row holds.
 */
export interface RunSummary {
  run_id: string
  task_id: string
  attempt: number
  status: string
  outcome: string
  failure_kind: string
  duration_seconds: number
  cost_micros: number
  tokens_in: number
  tokens_out: number
  summary: string
  error: string
  started_at: string
  ended_at: string
}

export const runsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    /**
     * Every run of one task, oldest first — a task retried three times has four
     * runs and the Logs tab has to show them in the order they happened.
     */
    listTaskRuns: build.query<Run[], string>({
      query: (taskID) => `tasks/${taskID}/runs`,
      // `id: taskID`, not a prefixed id: this tag shape existed before this
      // client grew a run screen, and the Logs tab (US-AD26) is wired to it.
      // Renaming it would be an unrequested change that silently stops the tab
      // refreshing the day something invalidates this tag.
      providesTags: (_r, _e, taskID) => [{ type: 'Run', id: taskID }],
    }),

    listRunSteps: build.query<Step[], string>({
      query: (runID) => `runs/${runID}/steps`,
      providesTags: (_r, _e, runID) => [{ type: 'Step', id: runID }],
    }),

    /** `GET /runs/{id}` — one run (US-AD41 AC1). */
    getRun: build.query<Run, string>({
      query: (runID) => `runs/${runID}`,
      providesTags: (_r, _e, runID) => [{ type: 'Run', id: `RUN-${runID}` }],
    }),

    /**
     * `GET /runs/{id}/summary` — the same run plus a computed duration.
     *
     * Tagged with the same `Run` id as `getRun` so the two cannot drift apart in
     * the cache: they describe one run, and an invalidation that refreshed one
     * without the other would show a duration from one state beside a cost from
     * another.
     */
    getRunSummary: build.query<RunSummary, string>({
      query: (runID) => `runs/${runID}/summary`,
      providesTags: (_r, _e, runID) => [{ type: 'Run', id: `RUN-${runID}` }],
    }),
  }),
})

export const { useListTaskRunsQuery, useListRunStepsQuery, useGetRunQuery, useGetRunSummaryQuery } = runsApi
