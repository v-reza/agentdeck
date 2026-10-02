import { baseApi } from './base'

/**
 * The audit log (US-AD95, ARCHITECTURE 6.2.19).
 *
 * `GET /api/v1/audit-log` has been live and role-gated since the system slice
 * landed; nothing in the app called it, so the workspace had a recorded history
 * nobody could read.
 *
 * Pagination is a CURSOR, not an offset, and that is the server's choice for a
 * reason worth keeping: rows are ordered by `id DESC`, and an offset page would
 * let a new audit row arriving between two requests push a row from page 2 onto
 * page 1, where the caller has already looked. `cursor` is the last id seen.
 *
 * `from`/`to` are RFC3339, not `YYYY-MM-DD`. The server rejects anything else
 * with a 400 rather than guessing, so the screen converts its date inputs.
 */

export interface AuditEntry {
  id: number
  /** Exactly one of the two actor fields is set in practice. */
  actor_user_id?: string
  actor_agent_id?: string
  action: string
  target_type: string
  target_id: string
  /** `before_json` / `after_json` are raw JSON, absent when the row has none. */
  before_json?: unknown
  after_json?: unknown
  ip?: string
  created_at: string
}

export interface AuditQuery {
  actor?: string
  action?: string
  /** RFC3339. Empty string means unbounded. */
  from?: string
  to?: string
  cursor?: number
  limit?: number
}

export const auditApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listAuditLog: build.query<{ entries: AuditEntry[] }, AuditQuery | void>({
      query: (args) => {
        const q = args ?? {}
        const params = new URLSearchParams()
        if (q.actor) params.set('actor', q.actor)
        if (q.action) params.set('action', q.action)
        if (q.from) params.set('from', q.from)
        if (q.to) params.set('to', q.to)
        if (q.cursor) params.set('cursor', String(q.cursor))
        if (q.limit) params.set('limit', String(q.limit))
        const qs = params.toString()
        return `audit-log${qs ? `?${qs}` : ''}`
      },
      providesTags: [{ type: 'AuditLog' as const, id: 'LIST' }],
    }),
  }),
})

export const { useListAuditLogQuery } = auditApi
