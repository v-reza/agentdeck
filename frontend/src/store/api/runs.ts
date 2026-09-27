import { baseApi } from './base'

/**
 * Runs and their steps (US-AD26, US-AD94, ARCHITECTURE 6.2.11).
 *
 * `GET /tasks/{id}/runs` and `GET /runs/{id}/steps` have both been live since
 * the run lifecycle landed, and nothing in the app called them — the Logs tab
 * was waiting on a client that did not exist, not on an endpoint.
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

export const runsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    /**
     * Every run of one task, oldest first — a task retried three times has four
     * runs and the Logs tab has to show them in the order they happened.
     */
    listTaskRuns: build.query<Run[], string>({
      query: (taskID) => `tasks/${taskID}/runs`,
      providesTags: (_r, _e, taskID) => [{ type: 'Run', id: taskID }],
    }),

    listRunSteps: build.query<Step[], string>({
      query: (runID) => `runs/${runID}/steps`,
      providesTags: (_r, _e, runID) => [{ type: 'Step', id: runID }],
    }),
  }),
})

export const { useListTaskRunsQuery, useListRunStepsQuery } = runsApi
