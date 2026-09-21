import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'
import type { BaseQueryFn, FetchArgs, FetchBaseQueryError } from '@reduxjs/toolkit/query'
import { setSession, clearSession } from '../slices/sessionSlice'

/**
 * The single RTK Query base (ARCHITECTURE 18.2: `store/api/base.ts`). Every
 * feature API injects into this instance, so there is exactly one cache, one
 * tag space, and one place where auth headers and 401 handling live.
 *
 * The active tenant travels in the `X-Org-ID` header. ARCHITECTURE 11.1 step 5
 * is explicit that the tenant is resolved server-side from the session and that
 * header, never from a path parameter — so this client never puts an org id in
 * a URL to select a tenant.
 */
const rawBaseQuery = fetchBaseQuery({
  baseUrl: '/api/v1',
  credentials: 'include',
  prepareHeaders: (headers, { getState }) => {
    const orgID = (getState() as { session: { activeOrgID: string | null } }).session.activeOrgID
    if (orgID) headers.set('X-Org-ID', orgID)
    return headers
  },
})

/**
 * On 401 the session is gone (cookie expired or revoked). Clearing the session
 * slice is a listener-safe, synchronous action; the router reacts to the now
 * anonymous session by redirecting to /login, so no redirect lives here.
 */
const baseQueryWithReauth: BaseQueryFn<string | FetchArgs, unknown, FetchBaseQueryError> = async (
  args,
  api,
  extraOptions,
) => {
  const result = await rawBaseQuery(args, api, extraOptions)
  if (result.error?.status === 401) {
    api.dispatch(clearSession())
  }
  return result
}

/** Tag types are the invalidation contract (ARCHITECTURE 18.2 tag list). */
export const TAG_TYPES = [
  'Session',
  'Org',
  'Member',
  'ApiKey',
  'Project',
  'Board',
  'Task',
  'TaskLink',
  'Event',
  'Run',
  'Step',
  'Agent',
  'Provider',
  'Approval',
  'Ledger',
  'CostSummary',
  'Artifact',
  'Comment',
  'Webhook',
  'AuditLog',
  'Notification',
] as const

export const baseApi = createApi({
  reducerPath: 'api',
  baseQuery: baseQueryWithReauth,
  tagTypes: TAG_TYPES,
  refetchOnMountOrArgChange: 30,
  refetchOnReconnect: true,
  endpoints: () => ({}),
})

export { setSession }
