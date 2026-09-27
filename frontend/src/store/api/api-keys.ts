import { baseApi } from './base'

/**
 * API keys (US-AD06, ARCHITECTURE 6.2.3).
 *
 * Five endpoints, all under the *user's session* — not the key itself. A key is
 * how something else authenticates; this is how the key is made, seen, revoked,
 * and deleted.
 *
 * Two server rules shape this client:
 *
 *  - The plaintext `adk_...` exists in exactly one response, the 201 from
 *    `createAPIKey`. Nothing can return it again, so there is no "reveal" query
 *    to write and the UI has to treat the create response as the only sighting.
 *  - Every call is scoped to the caller. `APIKeys(ctx, orgID, userID)` lists the
 *    caller's own keys; another member's key in the same workspace answers 404,
 *    not 403. That is why the list is described as "yours", not "the
 *    workspace's" — the two are different sets and only one of them is reachable.
 *
 * Admin-gating is deliberately absent: the routes are registered at
 * `auth.Member` (`cmd/api/rbac_test.go:132-134`), so a member who is not an admin
 * still gets their own keys. A viewer does not (US-AD06 AC3).
 */

/** One key, as the server describes it. Never carries the token or the hash. */
export interface APIKey {
  id: string
  name: string
  /** The first 8 characters of the token (`adk_xxxx`). */
  prefix: string
  last_used_at: string | null
  revoked_at: string | null
  created_at: string
}

/**
 * The create response: the key plus the one and only sighting of the plaintext.
 *
 * `key` is the full `adk_...`. It is not persisted anywhere in this client — the
 * component holds it in state long enough to show it and copy it, and drops it
 * when the dialog closes.
 */
export interface CreatedAPIKey extends APIKey {
  key: string
}

export const apiKeysApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listAPIKeys: build.query<APIKey[], void>({
      query: () => 'api-keys',
      providesTags: (result) =>
        result
          ? [...result.map((k) => ({ type: 'ApiKey' as const, id: k.id })), { type: 'ApiKey' as const, id: 'LIST' }]
          : [{ type: 'ApiKey' as const, id: 'LIST' }],
    }),

    /**
     * Creates a key and returns the only copy of its plaintext.
     *
     * The name is validated server-side (1-64 characters after trimming,
     * `internal/auth/apikey.go:125`). The client checks the same bound so the
     * error arrives before the request, but the server is still the decider.
     *
     * Note what is *not* handled here: US-AD06 AC2 expects a 409 when the active
     * key limit is reached. There is no such limit in the code — `CreateAPIKey`
     * validates the name and nothing else — so no 409 branch is invented.
     */
    createAPIKey: build.mutation<CreatedAPIKey, { name: string }>({
      query: (body) => ({ url: 'api-keys', method: 'POST', body }),
      invalidatesTags: [{ type: 'ApiKey', id: 'LIST' }],
    }),

    getAPIKey: build.query<APIKey, string>({
      query: (id) => `api-keys/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'ApiKey', id }],
    }),

    /**
     * Revokes a key: it stays in the list, marked revoked, and stops
     * authenticating. Idempotent in outcome — revoking an already-revoked key
     * answers 200, not 404.
     */
    revokeAPIKey: build.mutation<{ revoked: boolean }, string>({
      query: (id) => ({ url: `api-keys/${id}/revoke`, method: 'POST' }),
      invalidatesTags: (_r, _e, id) => [
        { type: 'ApiKey', id },
        { type: 'ApiKey', id: 'LIST' },
      ],
    }),

    /**
     * Deletes a key outright. Distinct from revoke on purpose: revoke keeps the
     * row for audit, delete removes it. The UI offers both because the server
     * offers both, and the difference is visible in the list afterwards.
     */
    deleteAPIKey: build.mutation<void, string>({
      query: (id) => ({ url: `api-keys/${id}`, method: 'DELETE' }),
      invalidatesTags: (_r, _e, id) => [
        { type: 'ApiKey', id },
        { type: 'ApiKey', id: 'LIST' },
      ],
    }),
  }),
})

export const {
  useListAPIKeysQuery,
  useCreateAPIKeyMutation,
  useGetAPIKeyQuery,
  useRevokeAPIKeyMutation,
  useDeleteAPIKeyMutation,
} = apiKeysApi
