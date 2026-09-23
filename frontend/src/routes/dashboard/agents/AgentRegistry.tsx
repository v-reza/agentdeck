import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Search } from 'lucide-react'
import { useListProjectsQuery } from '@/store/api/boards'
import { useDeleteAgentMutation, useListAgentsQuery } from '@/store/api/agents'
import { useListProvidersQuery } from '@/store/api/providers'
import { useCanAct } from '@/hooks/use-orgs'
import { describeError } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { EmptyState, Panel } from '@/components/ui/card'
import { Modal } from '@/components/ui/modal'
import { cn } from '@/lib/cn'
import { formatRelative, shortID } from '@/lib/formatters'
import { interpolate } from '@/lib/format'
import { CreateAgentForm } from './CreateAgentForm'
import { AgentSpecCards } from './AgentSpecCards'
import { StatusFilter, type StatusFilterValue } from './StatusFilter'
import type { Agent } from '@/lib/domain'
import { SkeletonRows } from '@/components/ui/skeleton'

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
 *
 * The toolbar (220px search, status filter), the table footer counts and the two
 * guidance cards are cloned from the same design file. The search and the filter
 * narrow the rows the server already sent, and the footer's "ready" count is
 * `has_provider_key` rather than the mock's hardcoded 3. The archived count is 0
 * because `archived_at` does not exist in the schema yet (US-AD73 is M1 and the
 * column is landing separately) — rendering the mock's 1 would be an invented
 * metric, and the card must not call an endpoint that answers 404.
 */
export function AgentRegistry() {
  const t = useT()
  const { data: projects } = useListProjectsQuery()
  const { data: providers = [] } = useListProvidersQuery()
  const [projectID, setProjectID] = useState<string | null>(null)
  const activeProject = projectID ?? projects?.[0]?.id ?? null
  const { data: agents, isLoading } = useListAgentsQuery(activeProject ?? '', {
    skip: !activeProject,
  })
  // `agents.provider` is the *protocol* the row runs on, derived server-side
  // from the provider it points at (US-AD109 AC6). The column asks which
  // provider the operator registered, so it is resolved from the registry the
  // page already has in cache — not from a second copy of the name on every
  // agent row, which is the denormalisation US-AD109 removed.
  const providerName = new Map(providers.map((entry) => [entry.id, entry.name]))
  const canDelete = useCanAct('admin')
  const [deleteAgent] = useDeleteAgentMutation()
  const [pendingDelete, setPendingDelete] = useState<Agent | null>(null)
  // US-AD20 AC4 answers 409 when the agent still holds a running task. Swallowing
  // that would leave the row on screen with no explanation, which reads as a
  // broken button rather than a rule.
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState<StatusFilterValue>('all')

  const list = agents ?? []
  // US-AD73 AC2: the registry lists archived agents too, so the user can find
  // one and unarchive it. They are excluded from the assignable count, and the
  // filter below can surface them on their own.
  const active = list.filter((agent) => !agent.archived_at)
  const archivedCount = list.length - active.length
  const visible = filterAgents(list, search, status)
  const readyCount = active.filter((agent) => agent.has_provider_key).length

  return (
    <>
      <WorkspaceTopbar
        title={t['agents.title']}
        path="/agents"
        subtitle={agents ? `${list.length} ${t['agents.count']}` : undefined}
        right={
          <div className="flex items-center gap-2.5">
            <div className="relative">
              <Search
                size={14}
                className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-[var(--color-tertiary)]"
              />
              <input
                type="search"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t['agents.search']}
                aria-label={t['agents.search']}
                className="h-[30px] w-[220px] rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] pr-3 pl-8 text-[12px] text-[var(--color-primary)] placeholder:text-[var(--color-tertiary)] focus:border-[var(--color-accent)] focus:outline-none"
              />
            </div>

            <StatusFilter
              value={status}
              onChange={setStatus}
              label={t['agents.filter.label']}
              options={[
                { value: 'all', label: t['agents.filter.all'] },
                { value: 'ready', label: t['agents.status.ready'] },
                { value: 'needsKey', label: t['agents.status.needsKey'] },
                { value: 'archived', label: t['agents.status.archived'] },
              ]}
            />

            {(projects ?? []).length > 1 ? (
              <Combobox
                value={activeProject ?? ''}
                onChange={setProjectID}
                label={t['boards.create.project']}
                options={(projects ?? []).map((project) => ({ value: project.id, label: project.name }))}
                className="w-[180px]"
              />
            ) : null}
            <CreateAgentForm projects={projects ?? []} activeProjectID={activeProject ?? ''} />
          </div>
        }
      />

      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        <AgentStatusCard
          total={list.length}
          readyCount={readyCount}
          needsKeyCount={active.length - readyCount}
          archivedCount={archivedCount}
        />
        {deleteError ? (
          <p className="text-[12px] text-[var(--color-danger)]">
            {t['agents.delete.failed']}: {deleteError}
          </p>
        ) : null}
        {isLoading ? (
          <SkeletonRows rows={6} columns={6} />
        ) : (projects ?? []).length === 0 ? (
          <EmptyState title={t['agents.noProjects']} hint={t['agents.noProjectsHint']} />
        ) : list.length === 0 ? (
          <EmptyState title={t['agents.empty']} hint={t['agents.emptyHint']} />
        ) : visible.length === 0 ? (
          <EmptyState title={t['agents.filter.empty']} hint={t['agents.filter.emptyHint']} />
        ) : (
          <Panel className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[858px] table-fixed border-collapse text-left">
                {/* Explicit widths, sized from the longest value each column has
                    to hold rather than from the design's spacing. Auto layout
                    ignores a `width` hint once the sum exceeds the table and
                    redistributes by content instead — which left the agent
                    column at 100px while a trailing spacer took 199. The total
                    stays under the panel's ~874px so no horizontal scrollbar
                    appears at the design's own breakpoint. `px-2` not `px-3`:
                    eight columns of 24px padding is 64px this table cannot
                    spare, and every value here is short. */}
                <colgroup>
                  <col className="w-[196px]" />
                  <col className="w-[126px]" />
                  <col className="w-[118px]" />
                  <col className="w-[86px]" />
                  <col className="w-[104px]" />
                  <col className="w-[110px]" />
                  <col className="w-[54px]" />
                  {canDelete ? <col className="w-[66px]" /> : null}
                </colgroup>
                <thead>
                  <tr className="h-[32px] border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-sunken)] text-[11px] font-semibold uppercase tracking-wider text-[var(--color-tertiary)]">
                    <th className="px-2">{t['agents.col.agent']}</th>
                    <th className="px-2">{t['agents.col.provider']}</th>
                    <th className="px-2">{t['agents.col.model']}</th>
                    <th className="px-2">{t['agents.col.reasoning']}</th>
                    <th className="px-2">{t['agents.col.status']}</th>
                    <th className="px-2">{t['agents.col.runtime']}</th>
                    <th className="px-2">{t['agents.col.tools']}</th>
                    {canDelete ? <th className="px-2 text-right">{t['agents.col.actions']}</th> : null}
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--color-border-subtle)] text-[12px]">
                  {visible.map((agent) => (
                    <AgentRow
                      key={agent.id}
                      agent={agent}
                      providerName={providerName.get(agent.provider_id ?? '')}
                      canDelete={canDelete}
                      onDelete={() => setPendingDelete(agent)}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            <div
              data-testid="agents-footer"
              className="flex h-[32px] items-center justify-between border-t border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-4 text-[11px] text-[var(--color-tertiary)]"
            >
              <div>{interpolate(t['agents.footer.showing'], [String(visible.length), String(list.length)])}</div>
              <div className="flex items-center gap-3">
                <span className="flex items-center gap-1 font-mono">
                  <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-status-done)]" />
                  {interpolate(t['agents.footer.ready'], [String(readyCount)])}
                </span>
                <span>•</span>
                <span className="flex items-center gap-1 font-mono">
                  <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-status-archived)]" />
                  {interpolate(t['agents.footer.archived'], [String(archivedCount)])}
                </span>
              </div>
            </div>
          </Panel>
        )}

        <AgentSpecCards readyCount={readyCount} archivedCount={archivedCount} />
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

/** The status strip above the table.
 *
 * The design's own banner is a spec explainer — it prints "US-AD20 / US-AD73
 * Scoped", the acceptance-criterion numbers, and a "HTTP 201 Created" badge.
 * That vocabulary belongs in the PRD, not in the product: an operator should not
 * have to know which story shipped a screen. What the strip says instead is the
 * one thing an operator acts on before assigning work — how many agents can
 * actually take a task right now, and why the rest cannot.
 */
function AgentStatusCard({
  total,
  readyCount,
  needsKeyCount,
  archivedCount,
}: {
  total: number
  readyCount: number
  needsKeyCount: number
  archivedCount: number
}) {
  const t = useT()

  return (
    <Panel className="flex items-center justify-between gap-4 p-3 shadow-xs">
      <div className="flex items-center gap-3">
        <span
          className={cn(
            'h-2 w-2 shrink-0 rounded-full',
            readyCount > 0 ? 'bg-[var(--color-status-done)]' : 'bg-[var(--color-status-archived)]',
          )}
        />
        <div>
          <div className="text-[12px] font-semibold text-[var(--color-primary)]">
            {interpolate(t['agents.statusCard.title'], [String(total)])}
          </div>
          <div className="mt-0.5 text-[11px] text-[var(--color-secondary)]">
            {interpolate(t['agents.statusCard.hint'], [String(readyCount), String(needsKeyCount)])}
          </div>
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-2">
        <span className="rounded-[6px] bg-[var(--color-status-done)]/10 px-2 py-1 font-mono text-[10px] whitespace-nowrap text-[var(--color-status-done)]">
          {interpolate(t['agents.statusCard.ready'], [String(readyCount)])}
        </span>
        <span className="rounded-[6px] bg-[var(--color-warning)]/10 px-2 py-1 font-mono text-[10px] whitespace-nowrap text-[var(--color-warning)]">
          {interpolate(t['agents.statusCard.needsKey'], [String(needsKeyCount)])}
        </span>
        <span className="rounded-[6px] bg-[var(--color-surface-page)] px-2 py-1 font-mono text-[10px] whitespace-nowrap text-[var(--color-tertiary)]">
          {interpolate(t['agents.statusCard.archived'], [String(archivedCount)])}
        </span>
      </div>
    </Panel>
  )
}

/**
 * Narrows the rows the server already returned. The design's placeholder names
 * the three things it searches — agent, model, skill — so those are the three
 * fields; provider and tools are not in the design's copy and are not invented.
 */
function filterAgents(list: Agent[], search: string, status: StatusFilterValue): Agent[] {
  const needle = search.trim().toLowerCase()
  return list.filter((agent) => {
    // Archived is its own bucket, never mixed into ready/needsKey: an archived
    // agent is not "ready to take a task" whatever its credential state is.
    if (status === 'archived' && !agent.archived_at) return false
    if (status === 'ready' && (agent.archived_at || !agent.has_provider_key)) return false
    if (status === 'needsKey' && (agent.archived_at || agent.has_provider_key)) return false
    if (needle === '') return true
    return [agent.name, agent.model, ...(agent.skills ?? [])].some((field) =>
      String(field).toLowerCase().includes(needle),
    )
  })
}

function AgentRow({
  agent,
  providerName,
  canDelete,
  onDelete,
}: {
  agent: Agent
  /** Resolved by the list, which already holds the registry in cache. */
  providerName?: string
  canDelete: boolean
  onDelete: () => void
}) {
  const t = useT()
  const tools = agent.tools ?? []

  return (
    <tr className="h-[28px] transition-colors hover:bg-[var(--color-surface-sunken)]">
      <td className="px-2 py-2">
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
      <td className="px-2 py-2 font-mono text-[11px] text-[var(--color-secondary)]" data-testid="agent-provider">
        {providerName ?? (agent.provider_id ? '—' : agent.provider)}
      </td>
      <td className="px-2 py-2 font-mono text-[11px] font-medium text-[var(--color-primary)]">{agent.model}</td>
      <td className="px-2 py-2 font-mono text-[11px] text-[var(--color-secondary)]">
        <span className="rounded bg-[var(--color-surface-sunken)] px-1.5 py-0.5 text-[10px]">
          {agent.reasoning_effort}
        </span>
      </td>
      <td className="px-2 py-2">
        {agent.archived_at ? (
          <span className="inline-flex items-center gap-1 rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-tertiary)]">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-status-archived)]" />
            {t['agents.status.archived']}
          </span>
        ) : agent.has_provider_key ? (
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
      <td className="px-2 py-2 font-mono text-[10px] text-[var(--color-secondary)]">
        <div>{agent.max_runtime_seconds.toLocaleString()}s max</div>
        <div className="text-[var(--color-quaternary)]">
          {agent.retry_policy} (x{agent.max_attempts})
        </div>
      </td>
      <td className="px-2 py-2 font-mono text-[10px] text-[var(--color-secondary)]">
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
        <td className="px-2 py-2 text-right">
          <Button variant="ghost" size="sm" onClick={onDelete} className="text-[var(--color-danger)]">
            {t['agents.delete']}
          </Button>
        </td>
      ) : null}
    </tr>
  )
}
