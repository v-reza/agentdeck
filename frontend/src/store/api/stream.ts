import { baseApi } from './base'
import type { Approval, EventEnvelope } from '@/lib/domain'

/** The SSE endpoint, ARCHITECTURE 6.2.13 / 7.2. */
export const SSE_EVENTS_PATH = '/api/v1/events'

/**
 * Approval-gate server state and the SSE stream bridge (ARCHITECTURE 18.2:
 * `store/api/stream.ts`, tag Approval; ARCHITECTURE 7 for the wire format).
 *
 * The stream is *not* polled. `onCacheEntryAdded` opens one EventSource per
 * subscribed board, patches the RTK Query cache directly from each event, and
 * tears the connection down when the last subscriber unmounts. There is no
 * second copy of board state living in a component: the cache is the single
 * source, and the stream only mutates it.
 *
 * Reconnect uses the browser's own `Last-Event-ID` resume: the server replays
 * from the last delivered id (ARCHITECTURE 7.3), so a dropped connection
 * catches up without a full refetch.
 */

/**
 * Consecutive failures after which the stream stops reconnecting.
 *
 * `EventSource` retries on its own indefinitely, so an endpoint that is not
 * deployed yet turns into an unbounded request loop (SSE per ARCHITECTURE
 * 6.2.13 is M2 and answers 404 today — verified against the running API). Three
 * failures is enough to ride out a restart or a dropped socket without leaving
 * the operator with a console full of 404s.
 */
export const STREAM_FAILURE_LIMIT = 3

/** Whether the board stream should stop reconnecting after `failures` in a row. */
export function streamShouldGiveUp(failures: number): boolean {
  return failures >= STREAM_FAILURE_LIMIT
}

export const streamApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listApprovals: build.query<Approval[], void>({
      query: () => 'approvals',
      providesTags: (result) =>
        result
          ? [...result.map((a) => ({ type: 'Approval' as const, id: a.id })), { type: 'Approval' as const, id: 'LIST' }]
          : [{ type: 'Approval' as const, id: 'LIST' }],
    }),

    getApproval: build.query<Approval, string>({
      query: (id) => `approvals/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'Approval', id }],
    }),

    approveApproval: build.mutation<Approval, string>({
      query: (id) => ({ url: `approvals/${id}/approve`, method: 'POST' }),
      invalidatesTags: (_r, _e, id) => [
        { type: 'Approval', id },
        { type: 'Approval', id: 'LIST' },
        { type: 'Task', id: 'LIST' },
      ],
    }),

    rejectApproval: build.mutation<Approval, { id: string; reason: string }>({
      query: ({ id, ...body }) => ({ url: `approvals/${id}/reject`, method: 'POST', body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'Approval', id },
        { type: 'Approval', id: 'LIST' },
        { type: 'Task', id: 'LIST' },
      ],
    }),

    /**
     * The board event stream. `boardID` is the subscription argument; the
     * cache entry stays open as long as a component reads it, and the
     * EventSource closes with the last subscriber.
     *
     * Query data is `EventEnvelope[]` — the append-only tail the operator has
     * seen this session. It is deliberately bounded: the drawer shows recent
     * history, and the durable record is the events table, not this array.
     */
    boardEvents: build.query<EventEnvelope[], string>({
      queryFn: () => ({ data: [] }),
      async onCacheEntryAdded(boardID, { updateCachedData, cacheDataLoaded, cacheEntryRemoved }) {
        // Nothing to stream for an unresolved tenant.
        if (!boardID) return

        await cacheDataLoaded

        const source = new EventSource(`/api/v1/boards/${boardID}/events`, {
          withCredentials: true,
        })

        const append = (event: MessageEvent<string>) => {
          try {
            const parsed = JSON.parse(event.data) as EventEnvelope
            updateCachedData((draft) => {
              draft.push(parsed)
              // Keep the live tail bounded; the durable log is server-side.
              if (draft.length > 500) draft.splice(0, draft.length - 500)
            })
          } catch {
            // A malformed frame must not kill the stream; drop it and keep
            // reading so one bad event cannot blind the operator.
          }
        }

        source.addEventListener('message', append as EventListener)
        for (const kind of STREAMED_EVENT_KINDS) {
          source.addEventListener(kind, append as EventListener)
        }

        // A stream that cannot connect is closed rather than left retrying, so a
        // missing endpoint fails once per subscription instead of forever.
        let failures = 0
        source.addEventListener('error', () => {
          failures += 1
          if (streamShouldGiveUp(failures)) source.close()
        })
        // The server's first frame proves the connection is real, so the bound
        // counts consecutive failures rather than total ones.
        source.addEventListener('open', () => {
          failures = 0
        })

        await cacheEntryRemoved
        source.close()
      },
    }),
  }),
})

/** The event kinds the board stream is allowed to emit (DECISIONS 4). */
const STREAMED_EVENT_KINDS = [
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

export const {
  useListApprovalsQuery,
  useGetApprovalQuery,
  useApproveApprovalMutation,
  useRejectApprovalMutation,
  useBoardEventsQuery,
} = streamApi
