import { baseApi } from './base'
import type { Agent } from '@/lib/domain'

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

/** The complete mutable agent profile accepted by PATCH /agents/{id}. */
export interface UpdateAgentArgs {
  id: string
  name: string
  provider: string
  model: string
  reasoning_effort: string
  skills: string[]
  tools: string[]
  max_runtime_seconds: number
  retry_policy: string
  max_attempts: number
  base_url?: string
}

export interface ArchiveAgentArgs {
  id: string
  archived: boolean
}

export interface CatalogRate {
  micros_per_1m: number
  usd_per_1m: number
}

export interface CatalogModel {
  model: string
  price_source: 'manual' | 'catalog' | 'pattern' | 'unpriced' | string
  pricing_model: string
  input: CatalogRate
  output: CatalogRate
  cached: CatalogRate
  reasoning: CatalogRate
  cache_creation: CatalogRate
  price_version: number
  estimate: boolean
  disclaimer: string
}

export interface AgentCatalog {
  estimate: boolean
  disclaimer: string
  price_version: number
  models: CatalogModel[]
}

export interface AgentSkill {
  id: string
  org_id: string
  slug: string
  name: string
  body_md: string
  version: number
  is_system: boolean
  used_by: number
  created_by: string
  created_at: string
  updated_at: string
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

    getAgentCatalog: build.query<AgentCatalog, void>({
      query: () => 'agent-catalog',
      providesTags: [{ type: 'Agent', id: 'CATALOG' }],
    }),

    listAgentSkills: build.query<AgentSkill[], void>({
      query: () => 'agent-skills',
      providesTags: [{ type: 'Agent', id: 'SKILLS' }],
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

    updateAgent: build.mutation<Agent, UpdateAgentArgs>({
      query: ({ id, ...body }) => ({ url: `agents/${id}`, method: 'PATCH', body }),
      invalidatesTags: (_r, _e, { id }) => ['Agent', { type: 'Agent', id }],
    }),

    archiveAgent: build.mutation<Agent, ArchiveAgentArgs>({
      query: ({ id, archived }) => ({ url: `agents/${id}`, method: 'PATCH', body: { archived } }),
      invalidatesTags: (_r, _e, { id }) => ['Agent', { type: 'Agent', id }],
    }),

    deleteAgent: build.mutation<void, string>({
      query: (id) => ({ url: `agents/${id}`, method: 'DELETE' }),
      invalidatesTags: (_r, _e, id) => [{ type: 'Agent' as const, id }, 'Agent'],
    }),
  }),
})

export const {
  useListAgentsQuery,
  useGetAgentQuery,
  useGetAgentCatalogQuery,
  useListAgentSkillsQuery,
  useCreateAgentMutation,
  useUpdateAgentMutation,
  useArchiveAgentMutation,
  useDeleteAgentMutation,
} = agentsApi
