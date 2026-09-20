import { baseApi } from './base'
import type { Agent, ProviderKeyState } from '@/lib/domain'

/**
 * Agent-domain server state (ARCHITECTURE 18.2: `store/api/agents.ts`, tags
 * Agent, ProviderKey).
 *
 * Provider keys are write-only: PUT accepts a plaintext key, and no response
 * ever echoes it back. The API returns only presence and last-rotation
 * metadata, which is why ProviderKeyState has no `value` field — the type
 * makes leaking a key into the client cache impossible.
 */

export interface CreateAgentArgs {
  projectID: string
  name: string
  provider: string
  model: string
  reasoningEffort?: string
  maxRuntimeSeconds?: number
  retryPolicy?: string
  maxAttempts?: number
  tools?: string[]
  skills?: string[]
}

export interface UpdateAgentArgs {
  id: string
  name?: string
  model?: string
  reasoningEffort?: string
  maxRuntimeSeconds?: number
  retryPolicy?: string
  maxAttempts?: number
}

export const agentsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listAgents: build.query<Agent[], string>({
      query: (projectID) => `projects/${projectID}/agents`,
      providesTags: (result, _e, projectID) =>
        result
          ? [
              ...result.map((a) => ({ type: 'Agent' as const, id: a.id })),
              { type: 'Agent' as const, id: `PROJECT-${projectID}` },
            ]
          : [{ type: 'Agent' as const, id: `PROJECT-${projectID}` }],
    }),

    getAgent: build.query<Agent, string>({
      query: (id) => `agents/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'Agent', id }],
    }),

    createAgent: build.mutation<Agent, CreateAgentArgs>({
      query: ({ projectID, ...body }) => ({
        url: `projects/${projectID}/agents`,
        method: 'POST',
        body,
      }),
      invalidatesTags: (_r, _e, { projectID }) => [{ type: 'Agent', id: `PROJECT-${projectID}` }],
    }),

    updateAgent: build.mutation<Agent, UpdateAgentArgs>({
      query: ({ id, ...body }) => ({ url: `agents/${id}`, method: 'PATCH', body }),
      invalidatesTags: (_r, _e, { id }) => [{ type: 'Agent', id }],
    }),

    deleteAgent: build.mutation<void, string>({
      query: (id) => ({ url: `agents/${id}`, method: 'DELETE' }),
      invalidatesTags: [{ type: 'Agent', id: 'LIST' }],
    }),

    validateAgent: build.mutation<{ ok: boolean; detail?: string }, string>({
      query: (id) => ({ url: `agents/${id}/validate`, method: 'POST' }),
    }),

    /** PUT is a rotate: sending a key replaces whatever was stored. */
    setProviderKey: build.mutation<ProviderKeyState, { id: string; apiKey: string }>({
      query: ({ id, apiKey }) => ({
        url: `agents/${id}/provider-key`,
        method: 'PUT',
        body: { api_key: apiKey },
      }),
      invalidatesTags: (_r, _e, { id }) => [{ type: 'Agent', id }],
    }),

    deleteProviderKey: build.mutation<void, string>({
      query: (id) => ({ url: `agents/${id}/provider-key`, method: 'DELETE' }),
      invalidatesTags: (_r, _e, id) => [{ type: 'Agent', id }],
    }),
  }),
})

export const {
  useListAgentsQuery,
  useGetAgentQuery,
  useCreateAgentMutation,
  useUpdateAgentMutation,
  useDeleteAgentMutation,
  useValidateAgentMutation,
  useSetProviderKeyMutation,
  useDeleteProviderKeyMutation,
} = agentsApi
