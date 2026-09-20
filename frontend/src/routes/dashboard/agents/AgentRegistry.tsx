import { useState } from 'react'
import { useListProjectsQuery } from '@/store/api/boards'
import { useListAgentsQuery } from '@/store/api/agents'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel } from '@/components/ui/card'
import { formatRelative, shortID } from '@/lib/formatters'
import type { Agent } from '@/lib/domain'

/**
 * Screen 25-agent-registry — the agents configured for a project.
 *
 * Agents are project-scoped in the contract (GET /projects/{id}/agents), so the
 * registry picks a project first instead of pretending there is a workspace-wide
 * roster. `has_provider_key` is rendered as a state, never the key itself: the
 * server never returns a provider key, so the UI cannot leak one.
 */
export function AgentRegistry() {
  const { data: projects } = useListProjectsQuery()
  const [projectID, setProjectID] = useState<string | null>(null)
  const activeProject = projectID ?? projects?.[0]?.id ?? null
  const { data: agents, isLoading } = useListAgentsQuery(activeProject ?? '', {
    skip: !activeProject,
  })

  return (
    <>
      <WorkspaceTopbar
        title="Agent Registry"
        subtitle={agents ? `${agents.length} agents` : undefined}
        right={
          (projects ?? []).length > 1 ? (
            <select
              value={activeProject ?? ''}
              onChange={(event) => setProjectID(event.target.value)}
              className="h-8 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 text-[12px] text-[var(--color-primary)]"
            >
              {(projects ?? []).map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
          ) : null
        }
      />

      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        {isLoading ? (
          <p className="font-mono text-[12px] text-[var(--color-tertiary)]">Loading…</p>
        ) : (agents ?? []).length === 0 ? (
          <EmptyState title="No agents in this project" hint="Register one to give tasks a runner." />
        ) : (
          <div className="grid grid-cols-2 gap-2.5">
            {(agents ?? []).map((agent) => (
              <AgentCard key={agent.id} agent={agent} />
            ))}
          </div>
        )}
      </div>
    </>
  )
}

function AgentCard({ agent }: { agent: Agent }) {
  return (
    <Panel className="p-3">
      <div className="flex items-center justify-between">
        <span className="text-[13px] font-semibold text-[var(--color-primary)]">{agent.name}</span>
        <span className="font-mono text-[10px] text-[var(--color-quaternary)]">{shortID(agent.id)}</span>
      </div>
      <div className="mt-1 flex flex-wrap items-center gap-2 font-mono text-[11px] text-[var(--color-tertiary)]">
        <span>
          {agent.provider}/{agent.model}
        </span>
        <span>retry {agent.retry_policy}</span>
        <span>max {agent.max_attempts}</span>
        <span>{agent.max_runtime_seconds}s</span>
      </div>
      <div className="mt-2 flex items-center gap-2">
        <span
          className={
            agent.has_provider_key
              ? 'h-1.5 w-1.5 rounded-full bg-[var(--color-status-done)]'
              : 'h-1.5 w-1.5 rounded-full bg-[var(--color-border-strong)]'
          }
        />
        <span className="text-[11px] text-[var(--color-secondary)]">
          {agent.has_provider_key ? 'provider key set' : 'no provider key'}
        </span>
        <span className="ml-auto font-mono text-[10px] text-[var(--color-quaternary)]">
          {formatRelative(agent.created_at)}
        </span>
      </div>
    </Panel>
  )
}
