import { useParams } from 'react-router-dom'
import { useGetAgentQuery } from '@/store/api/agents'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Panel } from '@/components/ui/card'
import { formatRelative, shortID } from '@/lib/formatters'

/**
 * Screen 28-agent-detail — one agent's configuration.
 *
 * Read-only on purpose: `PATCH /agents/{id}` is US-AD67 AC4 (M2) and is gated
 * owner/admin, and the provider credential is US-AD86. Until those routes exist
 * this screen shows what the registry knows and offers no control that could
 * only answer 404.
 */
export function AgentDetail() {
  const { agentID } = useParams<{ agentID: string }>()
  const { data: agent } = useGetAgentQuery(agentID ?? '', { skip: !agentID })

  return (
    <>
      <WorkspaceTopbar title={agent?.name ?? 'Agent'} subtitle={agent?.provider} />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {agent ? (
          <Panel className="p-3">
            <dl className="grid grid-cols-4 gap-2 font-mono text-[11px]">
              <Metric label="model" value={agent.model} />
              <Metric label="retry" value={agent.retry_policy} />
              <Metric label="attempts" value={String(agent.max_attempts)} />
              <Metric label="runtime" value={`${agent.max_runtime_seconds}s`} />
            </dl>
            <div className="mt-2 font-mono text-[10px] text-[var(--color-quaternary)]">
              {shortID(agent.id)} · created {formatRelative(agent.created_at)}
            </div>
          </Panel>
        ) : (
          <p className="font-mono text-[12px] text-[var(--color-tertiary)]">Loading…</p>
        )}
      </div>
    </>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-[10px] uppercase text-[var(--color-tertiary)]">{label}</dt>
      <dd className="mt-0.5 font-semibold text-[var(--color-primary)]">{value}</dd>
    </div>
  )
}
