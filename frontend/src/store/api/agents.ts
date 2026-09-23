import { baseApi } from './base'
import type { Agent } from '@/lib/domain'

export interface CreateAgentArgs {
  projectID: string
  name: string
  model: string
  /**
   * The workspace provider the agent draws its endpoint and credential from
   * (US-AD109 AC6). The server derives `provider` and `base_url` from it, so
   * the client never sends either — an agent that carried its own copy of the
   * endpoint is exactly what the registry exists to remove.
   *
   * Omitted means the agent has no provider of its own and uses the
   * deployment's environment default, which is a valid permanent state.
   */
  providerID?: string
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
  /**
   * The workspace provider the agent draws its endpoint and credential from
   * (US-AD109 AC6). The server derives `provider` and `base_url` from it, so
   * neither is sent — an agent that carried its own copy of the endpoint is
   * what the registry exists to remove.
   *
   * Empty means "no provider": a valid permanent state for a row the backfill
   * deliberately skipped, not a half-finished one.
   */
  provider_id: string
  model: string
  reasoning_effort: string
  skills: string[]
  tools: string[]
  max_runtime_seconds: number
  retry_policy: string
  max_attempts: number
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

export interface AgentAssignment {
  /** The board the picker was resolved for. Empty when the agent holds no task. */
  board_id: string
  tasks: AssignedTask[]
  /** Derived server-side so the chip and the rows are one snapshot. */
  running_count: number
  /** What the picker left out (US-AD73 AC2). Stated, never dropped silently. */
  hidden_agents: number
  picker: AssignableAgent[]
}

export interface AssignedTask {
  id: string
  board_id: string
  title: string
  status: string
  /** Integer micro-USD, like the ledger: the UI formats it, nothing rounds it. */
  cost_micros: number
  created_at: string
  estimate: boolean
}

export interface AssignableAgent {
  id: string
  name: string
  has_provider_key: boolean
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

/**
 * The three credential endpoints of ARCHITECTURE 6.2.7 (US-AD86) were absent
 * from this file: the backend landed them in `cmd/api/agents_provider_key.go`
 * and no client ever called them. They are added here rather than fetched
 * directly because `verify_web.py` fails any `fetch(` outside `store/api/`.
 *
 * `masked_key` is derived from the plaintext in that one request and never
 * stored, so it is only present on the PUT answer — a later GET cannot
 * reproduce it. `has_provider_key` is the generated column (DECISIONS 6A.I).
 */
export interface ProviderKeyArgs {
  id: string
  apiKey: string
  /** Applied only when the agent does not carry a provider yet (test-before-save). */
  provider?: string
  model?: string
  baseUrl?: string
}

export interface ProviderKeyResult {
  id: string
  has_provider_key: boolean
  masked_key?: string
}

/** The handshake answer: `ok: false` is a successful test with a negative result. */
export interface ValidateResult {
  ok: boolean
  detail: string
  models?: string[]
}

/**
 * The stateless probe (ARCHITECTURE 6.2.7, US-AD106 AC2). The register form has
 * a provider and a pasted key but no agent id yet, so the key travels with the
 * request instead of being read back from the database. Nothing is stored.
 */
export interface ProbeModelsArgs {
  baseUrl: string
  apiKey: string
}

export interface ProbeModelsResult {
  models: string[]
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

    /**
     * Stateless probe. Deliberately a mutation, not a query: it carries a raw
     * credential in the body, and RTK Query caches query results — a cached
     * response would leave a provider key sitting in the store after the form
     * closed. A mutation is never cached, so the key lives for one request.
     */
    probeProviderModels: build.mutation<ProbeModelsResult, ProbeModelsArgs>({
      query: ({ baseUrl, apiKey }) => ({
        url: 'provider/models',
        method: 'POST',
        body: { base_url: baseUrl, api_key: apiKey },
      }),
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
          model: body.model,
          reasoning_effort: body.reasoningEffort,
          max_runtime_seconds: body.maxRuntimeSeconds,
          retry_policy: body.retryPolicy,
          max_attempts: body.maxAttempts,
          skills: body.skills,
          tools: body.tools,
          // This list is spelled out rather than spread on purpose — the server
          // rejects unknown keys, and the two that used to be here (`provider`
          // and `base_url`) are now derived server-side from `provider_id`
          // (US-AD109 AC6). Sending them would be a second writer for a fact
          // the registry owns, so the omission is the point rather than an
          // oversight. `src/store/api/agents.test.ts` pins the list.
          provider_id: body.providerID,
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

    /**
     * US-AD86 AC1/AC2/AC4 — store or rotate the credential. The answer reports
     * presence and a masked shape, never the value: `masked_key` is derived from
     * the plaintext inside that one request and discarded, so it cannot be
     * reproduced by a later read.
     *
     * The `Agent` tag is invalidated so `has_provider_key` on the registry row
     * and on the detail page refetches instead of showing a stale badge.
     */
    putProviderKey: build.mutation<ProviderKeyResult, ProviderKeyArgs>({
      query: ({ id, apiKey, provider, model, baseUrl }) => ({
        url: `agents/${id}/provider-key`,
        method: 'PUT',
        body: { api_key: apiKey, provider, model, base_url: baseUrl },
      }),
      invalidatesTags: (_r, _e, { id }) => ['Agent', { type: 'Agent', id }],
    }),

    /** US-AD86 AC2 — revoke. Idempotent: no key is not an error. */
    deleteProviderKey: build.mutation<ProviderKeyResult, string>({
      query: (id) => ({ url: `agents/${id}/provider-key`, method: 'DELETE' }),
      invalidatesTags: (_r, _e, id) => ['Agent', { type: 'Agent', id }],
    }),

    /**
     * The handshake. A refused key answers 200 with `ok: false` — "the provider
     * said 401" is a successful test with a negative result — so a caller must
     * read `ok`, not the HTTP status. A 4xx/5xx means the test could not run at
     * all, which is why the mutation is not tagged for invalidation: it changes
     * nothing.
     */
    validateAgent: build.mutation<ValidateResult, string>({
      query: (id) => ({ url: `agents/${id}/validate`, method: 'POST' }),
    }),

    deleteAgent: build.mutation<void, string>({
      query: (id) => ({ url: `agents/${id}`, method: 'DELETE' }),
      invalidatesTags: (_r, _e, id) => [{ type: 'Agent' as const, id }, 'Agent'],
    }),

    /**
     * The assignment section of screen 28-agent-detail (US-AD73 AC1/AC2): the
     * tasks this agent holds, the picker its board's create-task modal would
     * offer, and how many archived agents that picker left out.
     *
     * One query, not three: the section states all three together, and a count of
     * hidden rows rendered beside a list fetched at another moment is exactly the
     * kind of claim this screen exists to make honestly.
     */
    getAgentTasks: build.query<AgentAssignment, string>({
      query: (id) => `agents/${id}/tasks`,
      providesTags: (_r, _e, id) => [{ type: 'Agent' as const, id: `${id}-TASKS` }],
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
  usePutProviderKeyMutation,
  useDeleteProviderKeyMutation,
  useValidateAgentMutation,
  useDeleteAgentMutation,
  useProbeProviderModelsMutation,
  useGetAgentTasksQuery,
} = agentsApi
