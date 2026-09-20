import { baseApi } from './base'
import type { Agent } from '@/lib/domain'

/**
 * Agent-domain server state (ARCHITECTURE 18.2: `store/api/agents.ts`, tags
 * Agent, ProviderKey).
 *
 * Only the endpoints that exist today are declared. `PATCH /agents/{id}`,
 * `POST /agents/{id}/validate` and the provider-key pair belong to US-AD67 and
 * US-AD86 (M2) and have no route yet — declaring them would put a call in the
 * cache that can only ever answer 404, and the first person to reach it would
 * read that as a bug in this story. They come back with the routes that serve
 * them, and the provider-key pair returns to `lib/domain` at the same time.
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
        body: {
          name: body.name,
          provider: body.provider,
          model: body.model,
          reasoning_effort: body.reasoningEffort,
          max_runtime_seconds: body.maxRuntimeSeconds,
          retry_policy: body.retryPolicy,
          max_attempts: body.maxAttempts,
          skills: body.skills,
          tools: body.tools,
        },
      }),
      invalidatesTags: (_r, _e, { projectID }) => [{ type: 'Agent', id: `PROJECT-${projectID}` }],
    }),

    deleteAgent: build.mutation<void, string>({
      query: (id) => ({ url: `agents/${id}`, method: 'DELETE' }),
      // 'LIST' was not a tag any endpoint provides, so the old value refetched
      // nothing and a deleted agent stayed on screen until a manual reload.
      // Invalidating `{type:'Agent'}` with no id drops every cached agent list,
      // which is what a delete owes: the caller only knows the agent id, not the
      // project whose list is now stale.
      invalidatesTags: (_r, _e, id) => [{ type: 'Agent' as const, id }, 'Agent'],
    }),
  }),
})

export const { useListAgentsQuery, useGetAgentQuery, useCreateAgentMutation, useDeleteAgentMutation } = agentsApi
