import { baseApi } from './base'

/**
 * The provider registry client (ARCHITECTURE 6.2.8, US-AD109).
 *
 * `api_key` is write-only: it travels on POST/PATCH and is never returned by any
 * endpoint. The read shape carries `masked_key` instead, and only on the answer
 * to a request that supplied one — a later GET cannot reproduce it, because the
 * server never reads the plaintext back (AC2).
 */
export interface Provider {
  id: string
  org_id: string
  name: string
  protocol: 'openai_compatible' | 'anthropic' | 'google' | string
  base_url: string
  /** Last fetched model list (AC7). Empty until one is fetched. */
  models: string[]
  /**
   * Absent means "never fetched", which is not the same as "fetched, nothing to
   * refresh" — the UI reads it as stale and offers the refresh (AC7).
   */
  models_fetched_at?: string
  /** Absent means the credential has never passed the inference probe (AC3). */
  last_verified_at?: string
  is_default: boolean
  has_key: boolean
  masked_key?: string
  created_at: string
}

export interface CreateProviderArgs {
  name: string
  protocol: string
  base_url: string
  /** Optional: a local endpoint that checks nothing (Ollama) has no credential. */
  apiKey?: string
  /** Omitted means "auto": the server makes the first provider of a workspace the default. */
  isDefault?: boolean
}

export interface UpdateProviderArgs {
  id: string
  name?: string
  protocol?: string
  base_url?: string
  apiKey?: string
  isDefault?: boolean
}

/** DELETE answers 409 with this body when agents still point at the provider (AC5). */
export interface ProviderInUse {
  error: string
  agents: { id: string; name: string }[]
  total: number
}

export const providersApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listProviders: build.query<Provider[], void>({
      query: () => 'providers',
      providesTags: (result) =>
        result ? [...result.map((p) => ({ type: 'Provider' as const, id: p.id })), 'Provider'] : ['Provider'],
    }),

    getProvider: build.query<Provider, string>({
      query: (id) => `providers/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'Provider', id }],
    }),

    createProvider: build.mutation<Provider, CreateProviderArgs>({
      query: ({ apiKey, isDefault, ...rest }) => ({
        url: 'providers',
        method: 'POST',
        body: { ...rest, api_key: apiKey, is_default: isDefault },
      }),
      invalidatesTags: ['Provider'],
    }),

    updateProvider: build.mutation<Provider, UpdateProviderArgs>({
      query: ({ id, apiKey, isDefault, ...rest }) => ({
        url: `providers/${id}`,
        method: 'PATCH',
        body: { ...rest, api_key: apiKey, is_default: isDefault },
      }),
      invalidatesTags: (_r, _e, { id }) => [{ type: 'Provider', id }, 'Provider'],
    }),

    deleteProvider: build.mutation<void, string>({
      query: (id) => ({ url: `providers/${id}`, method: 'DELETE' }),
      invalidatesTags: ['Provider'],
    }),

    /**
     * AC3. Fires a minimal inference call upstream (`max_tokens: 1`) and returns
     * the provider with `last_verified_at` set only if it passed. A failure is an
     * HTTP error carrying the upstream detail, not a 200 with a false flag — the
     * badge state is derived from `last_verified_at`, so a failed probe must not
     * look like a successful one.
     */
    verifyProvider: build.mutation<Provider, string>({
      query: (id) => ({ url: `providers/${id}/verify`, method: 'POST' }),
      invalidatesTags: (_r, _e, id) => [{ type: 'Provider', id }, 'Provider'],
    }),

    /** AC7's manual half. The 24-hour rule is the automatic refresher's job. */
    refreshProviderModels: build.mutation<Provider, string>({
      query: (id) => ({ url: `providers/${id}/models`, method: 'POST' }),
      invalidatesTags: (_r, _e, id) => [{ type: 'Provider', id }, 'Provider'],
    }),
  }),
})

export const {
  useListProvidersQuery,
  useGetProviderQuery,
  useCreateProviderMutation,
  useUpdateProviderMutation,
  useDeleteProviderMutation,
  useVerifyProviderMutation,
  useRefreshProviderModelsMutation,
} = providersApi
