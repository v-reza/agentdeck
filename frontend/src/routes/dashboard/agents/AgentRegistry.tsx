import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useListProjectsQuery } from '@/store/api/boards'
import { useDeleteAgentMutation, useListAgentsQuery } from '@/store/api/agents'
import { useCanAct } from '@/hooks/use-orgs'
import { describeError } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Button } from '@/components/ui/button'
import { EmptyState, Panel } from '@/components/ui/card'
import { Modal } from '@/components/ui/modal'
import { formatRelative, shortID } from '@/lib/formatters'
import { CreateAgentForm } from './CreateAgentForm'
import type { Agent } from '@/lib/domain'

/**
 * Screen 25-agent-registry — the agents configured for a project.
 *
 * Agents are project-scoped in the contract (GET /projects/{id}/agents), so the
 * registry picks a project instead of pretending there is a workspace-wide
 * roster. The project picker only appears for a workspace that has more than
 * one, matching the single-workspace rule the other screens follow.
 *
 * The design's per-row "Status Fleet" and "Reasoning" columns come straight from
 * the row: a credential is M2 scope, so every agent registered today reports
 * `has_provider_key: false` and is rendered as awaiting a credential rather than
 * as active. Nothing here invents a count the API did not return.
 */
export function AgentRegistry() {
  const t = useT()
  const { data: projects } = useListProjectsQuery()
  const [projectID, setProjectID] = useState<string | null>(null)
  const activeProject = projectID ?? projects?.[0]?.id ?? null
  const { data: agents, isLoading } = useListAgentsQuery(activeProject ?? '', {
    skip: !activeProject,
  })
  const canDelete = useCanAct('admin')
  const [deleteAgent] = useDeleteAgentMutation()
  const [pendingDelete, setPendingDelete] = useState<Agent | null>(null)
  // US-AD20 AC4 answers 409 when the agent still holds a running task. Swallowing
  // that would leave the row on screen with no explanation, which reads as a
  // broken button rather than a rule.
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const list = agents ?? []

  return (
    <>
      <WorkspaceTopbar
        title={t['agents.title']}
        path="/agents"
        subtitle={agents ? `${list.length} ${t['agents.count']}` : undefined}
        right={
          <div className="flex items-center gap-2.5">
            {(projects ?? []).length > 1 ? (
              <select
                value={activeProject ?? ''}
                onChange={(event) => setProjectID(event.target.value)}
                aria-label={t['boards.create.project']}
                className="h-[30px] rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 text-[12px] text-[var(--color-primary)]"
              >
                {(projects ?? []).map((project) => (
                  <option key={project.id} value={project.id}>
                    {project.name}
                  </option>
                ))}
              </select>
            ) : null}
            <CreateAgentForm projects={projects ?? []} activeProjectID={activeProject ?? ''} />
          </div>
        }
      />

      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        {deleteError ? (
          <p className="text-[12px] text-[var(--color-danger)]">
            {t['agents.delete.failed']}: {deleteError}
          </p>
        ) : null}
        {isLoading ? (
          <p className="font-mono text-[12px] text-[var(--color-tertiary)]">Loading…</p>
        ) : (projects ?? []).length === 0 ? (
          <EmptyState title={t['agents.noProjects']} hint={t['agents.noProjectsHint']} />
        ) : list.length === 0 ? (
          <EmptyState title={t['agents.empty']} hint={t['agents.emptyHint']} />
        ) : (
          <Panel className="overflow-x-auto p-0">
            <table className="w-full min-w-[900px] border-collapse text-left">
              <thead>
                <tr className="h-[32px] border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-sunken)] text-[11px] font-semibold uppercase tracking-wider text-[var(--color-tertiary)]">
                  <th className="min-w-[190px] px-3">{t['agents.col.agent']}</th>
                  <th className="w-[110px] px-3">{t['agents.col.provider']}</th>
                  <th className="w-[150px] px-3">{t['agents.col.model']}</th>
                  <th className="w-[120px] px-3">{t['agents.col.reasoning']}</th>
                  <th className="w-[110px] px-3">{t['agents.col.status']}</th>
                  <th className="w-[120px] px-3">{t['agents.col.runtime']}</th>
                  <th className="w-[130px] px-3">{t['agents.col.tools']}</th>
                  {canDelete ? <th className="w-[100px] px-3 text-right">{t['agents.col.actions']}</th> : null}
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--color-border-subtle)] text-[12px]">
                {list.map((agent) => (
                  <AgentRow
                    key={agent.id}
                    agent={agent}
                    canDelete={canDelete}
                    onDelete={() => setPendingDelete(agent)}
                  />
                ))}
              </tbody>
            </table>
          </Panel>
        )}
      </div>

      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t['agents.delete.title']}
        description={pendingDelete?.name}
        size="sm"
      >
        <p className="text-[12px] text-[var(--color-secondary)]">{t['agents.delete.body']}</p>
        <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
          <Button variant="ghost" size="sm" onClick={() => setPendingDelete(null)}>
            {t['action.cancel']}
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={async () => {
              if (!pendingDelete) return
              const target = pendingDelete
              setPendingDelete(null)
              setDeleteError(null)
              const result = await deleteAgent(target.id)
              if ('error' in result) {
                setDeleteError(describeError(result.error))
              }
            }}
          >
            {t['agents.delete.confirm']}
          </Button>
        </div>
      </Modal>
    </>
  )
}

function AgentRow({ agent, canDelete, onDelete }: { agent: Agent; canDelete: boolean; onDelete: () => void }) {
  const t = useT()
  const tools = agent.tools ?? []

  return (
    <tr className="h-[28px] transition-colors hover:bg-[var(--color-surface-sunken)]">
      <td className="px-3 py-2">
        <div className="flex items-center gap-2">
          <div className="flex h-[20px] w-[20px] shrink-0 items-center justify-center rounded-full border border-[var(--color-border-subtle)] bg-[var(--color-surface-sunken)] font-mono text-[10px] font-bold text-[var(--color-accent)]">
            {agent.name.slice(0, 2).toLowerCase()}
          </div>
          <div>
            <Link
              to={agent.id}
              className="text-[13px] font-semibold leading-none text-[var(--color-primary)] hover:text-[var(--color-accent)]"
            >
              {agent.name}
            </Link>
            <div className="mt-1 font-mono text-[10px] text-[var(--color-quaternary)]">
              {shortID(agent.id)} · {formatRelative(agent.created_at)}
            </div>
          </div>
        </div>
      </td>
      <td className="px-3 py-2 font-mono text-[11px] text-[var(--color-secondary)]">{agent.provider}</td>
      <td className="px-3 py-2 font-mono text-[11px] font-medium text-[var(--color-primary)]">{agent.model}</td>
      <td className="px-3 py-2 font-mono text-[11px] text-[var(--color-secondary)]">
        <span className="rounded bg-[var(--color-surface-sunken)] px-1.5 py-0.5 text-[10px]">
          {agent.reasoning_effort}
        </span>
      </td>
      <td className="px-3 py-2">
        {agent.has_provider_key ? (
          <span className="inline-flex items-center gap-1 rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-status-done)]">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-status-done)]" />
            {t['agents.status.ready']}
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-tertiary)]">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-border-strong)]" />
            {t['agents.status.needsKey']}
          </span>
        )}
      </td>
      <td className="px-3 py-2 font-mono text-[10px] text-[var(--color-secondary)]">
        <div>{agent.max_runtime_seconds.toLocaleString()}s max</div>
        <div className="text-[var(--color-quaternary)]">
          {agent.retry_policy} (x{agent.max_attempts})
        </div>
      </td>
      <td className="px-3 py-2 font-mono text-[10px] text-[var(--color-secondary)]">
        {tools.length === 0 ? (
          <span className="text-[var(--color-quaternary)]">—</span>
        ) : (
          <div className="flex items-center gap-1">
            {tools.slice(0, 2).map((tool) => (
              <span key={tool} className="rounded bg-[var(--color-surface-sunken)] px-1">
                {tool}
              </span>
            ))}
            {tools.length > 2 ? <span className="text-[var(--color-quaternary)]">+{tools.length - 2}</span> : null}
          </div>
        )}
      </td>
      {canDelete ? (
        <td className="px-3 py-2 text-right">
          <Button variant="ghost" size="sm" onClick={onDelete} className="text-[var(--color-danger)]">
            {t['agents.delete']}
          </Button>
        </td>
      ) : null}
    </tr>
  )
}
