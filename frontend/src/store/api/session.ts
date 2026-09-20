import { baseApi } from './base'
import { setSession, clearSession } from '../slices/sessionSlice'
import type { MeResponse, Workspace, Role } from '@/lib/domain'

/**
 * Auth and workspace server state. Endpoints mirror ARCHITECTURE 6.2.1 and
 * 6.2.4 verbatim.
 *
 * `register` and `login` both answer with a session cookie, and `login` answers
 * with an empty 200 body. The response transformer therefore tolerates an empty
 * payload instead of assuming JSON — a `response.json()` on an empty body throws
 * `Unexpected end of JSON input` and was a real bug here once.
 */

export interface RegisterArgs {
  email: string
  password: string
  name: string
  org_name: string
}

export interface LoginArgs {
  email: string
  password: string
}

export interface ResetRequestBody {
  email: string
}

export interface ResetConfirmBody {
  token: string
  password: string
}

/**
 * PATCH /auth/me body (ARCHITECTURE 6.2.2: `{name, email, avatar}`).
 *
 * Every field is optional because PATCH is partial: an absent field is left
 * alone, and the profile screen sends only what actually changed. The server
 * rejects a blank value with 400 rather than silently keeping the old one.
 */
export interface ProfilePatchBody {
  name?: string
  email?: string
  avatar?: string
}

export interface MemberRow {
  user_id: string
  email: string
  name: string
  role: Role
  /** RFC 3339, when the membership row was written — not the account's age. */
  created_at: string
}

export const sessionApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    /**
     * GET /auth/me. On success the session slice is seeded so the X-Org-ID
     * header and the sidebar have a tenant before any other query fires. On
     * 401 the slice is cleared; the router then sends the visitor to /login.
     */
    me: build.query<MeResponse, void>({
      query: () => 'auth/me',
      providesTags: ['Session'],
      async onQueryStarted(_arg, { dispatch, queryFulfilled }) {
        try {
          const { data } = await queryFulfilled
          dispatch(setSession(data))
        } catch {
          dispatch(clearSession())
        }
      },
    }),

    register: build.mutation<{ user_id: string; workspace_id: string }, RegisterArgs>({
      query: (body) => ({ url: 'auth/register', method: 'POST', body }),
      invalidatesTags: ['Session'],
    }),

    login: build.mutation<void, LoginArgs>({
      query: (body) => ({ url: 'auth/login', method: 'POST', body }),
      invalidatesTags: ['Session'],
    }),

    logout: build.mutation<void, void>({
      query: () => ({ url: 'auth/logout', method: 'POST' }),
      invalidatesTags: ['Session'],
      async onQueryStarted(_arg, { dispatch, queryFulfilled }) {
        await queryFulfilled.catch(() => undefined)
        dispatch(clearSession())
      },
    }),

    /**
     * POST /auth/password/reset-request (US-AD88 AC1/AC5).
     *
     * The server always answers 202 — for a registered address, an unknown one,
     * and a malformed body alike — so the screen shows one neutral confirmation
     * and never learns whether the account exists. The response is therefore
     * read as "accepted", never as "sent".
     */
    requestPasswordReset: build.mutation<void, ResetRequestBody>({
      query: (body) => ({ url: 'auth/password/reset-request', method: 'POST', body }),
    }),

    /** POST /auth/password/reset (US-AD88 AC2/AC3). */
    resetPassword: build.mutation<void, ResetConfirmBody>({
      query: (body) => ({ url: 'auth/password/reset', method: 'POST', body }),
    }),

    /**
     * PATCH /auth/me (US-AD89 AC2/AC3).
     *
     * The response is the full identity, so the mutation writes it into both the
     * `me` cache entry and the session slice. That is what makes AC2's "visible
     * in the top bar without a full reload" true without a follow-up GET: the
     * top bar renders `session.name`, and this is the dispatch that changes it.
     */
    updateMe: build.mutation<MeResponse, ProfilePatchBody>({
      query: (body) => ({ url: 'auth/me', method: 'PATCH', body }),
      async onQueryStarted(_arg, { dispatch, queryFulfilled }) {
        try {
          const { data } = await queryFulfilled
          dispatch(sessionApi.util.updateQueryData('me', undefined, () => data))
          dispatch(setSession(data))
        } catch {
          // A rejected patch (409 on a taken email, 400 on a blank name) leaves
          // the cache and the session exactly as they were; the screen renders
          // the server's message inline.
        }
      },
    }),

    listOrgs: build.query<Workspace[], void>({
      query: () => 'orgs',
      providesTags: ['Org'],
    }),

    createOrg: build.mutation<Workspace, { name: string; slug: string }>({
      query: (body) => ({ url: 'orgs', method: 'POST', body }),
      invalidatesTags: ['Org', 'Session'],
    }),

    /** PATCH /orgs/{id} — owner-only workspace rename (US-AD77 AC1/AC2). */
    updateOrg: build.mutation<{ id: string; name: string; slug: string }, { orgID: string; name: string }>({
      query: ({ orgID, name }) => ({
        url: `orgs/${orgID}`,
        method: 'PATCH',
        body: { name },
      }),
      invalidatesTags: ['Org', 'Session'],
    }),

    /** One roster row. `created_at` is the membership timestamp (AC1's record). */
    listMembers: build.query<MemberRow[], string>({
      query: (orgID) => `orgs/${orgID}/members`,
      // Tagged by org id: a mutation in one workspace must not invalidate
      // another's roster, and the active-org switch refetches only its own.
      providesTags: (_result, _error, orgID) => [{ type: 'Member', id: orgID }],
    }),

    /** POST /orgs/{id}/members (US-AD04 AC1). Admin and above; 403 below. */
    addMember: build.mutation<{ email: string; role: Role }, { orgID: string; email: string; role: Role }>({
      query: ({ orgID, email, role }) => ({
        url: `orgs/${orgID}/members`,
        method: 'POST',
        body: { email, role },
      }),
      invalidatesTags: (_result, _error, { orgID }) => [{ type: 'Member', id: orgID }],
    }),

    /**
     * PATCH /orgs/{id}/members/{user_id} (US-AD04 AC3).
     *
     * Body is `{user_id, role}`: the handler validates the role from the body and
     * takes the target from the path, and a body shape it cannot decode is a 400.
     */
    updateMemberRole: build.mutation<{ user_id: string; role: Role }, { orgID: string; userID: string; role: Role }>({
      query: ({ orgID, userID, role }) => ({
        url: `orgs/${orgID}/members/${userID}`,
        method: 'PATCH',
        body: { user_id: userID, role },
      }),
      invalidatesTags: (_result, _error, { orgID }) => [{ type: 'Member', id: orgID }],
    }),

    /** DELETE /orgs/{id}/members/{user_id}. Admin and above. */
    removeMember: build.mutation<void, { orgID: string; userID: string }>({
      query: ({ orgID, userID }) => ({
        url: `orgs/${orgID}/members/${userID}`,
        method: 'DELETE',
      }),
      invalidatesTags: (_result, _error, { orgID }) => [{ type: 'Member', id: orgID }],
    }),
  }),
})

export const {
  useMeQuery,
  useRegisterMutation,
  useLoginMutation,
  useLogoutMutation,
  useRequestPasswordResetMutation,
  useResetPasswordMutation,
  useUpdateMeMutation,
  useListOrgsQuery,
  useCreateOrgMutation,
  useUpdateOrgMutation,
  useListMembersQuery,
  useAddMemberMutation,
  useUpdateMemberRoleMutation,
  useRemoveMemberMutation,
} = sessionApi
