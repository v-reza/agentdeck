import { useParams } from 'react-router-dom'
import { useGetAgentQuery, useSetProviderKeyMutation } from '@/store/api/agents'
import { useActionForm } from '@/hooks/use-action-form'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { Panel } from '@/components/ui/card'
import { formatRelative, shortID } from '@/lib/formatters'

/**
 * Screen 26-agent-form — one agent's configuration, plus its provider key.
 *
 * The key input is write-only by construction: PUT replaces whatever was stored
 * and the server never echoes it back, so the field is always empty and the
 * "set / not set" state comes from `has_provider_key`.
 */
export function AgentDetail() {
  const { agentID } = useParams<{ agentID: string }>()
  const { data: agent } = useGetAgentQuery(agentID ?? '', { skip: !agentID })
  const [setProviderKey] = useSetProviderKeyMutation()

  const [keyState, keyAction, keyPending] = useActionForm(setProviderKey, (form) => ({
    id: agentID ?? '',
    apiKey: String(form.get('api_key') ?? ''),
  }))

  return (
    <>
      <WorkspaceTopbar title={agent?.name ?? 'Agent'} subtitle={agent?.provider} />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {agent ? (
          <>
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

            <Panel className="max-w-[420px] p-3">
              <h3 className="mb-2 text-[10px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
                Provider key
              </h3>
              <p className="mb-3 text-[11px] text-[var(--color-secondary)]">
                {agent.has_provider_key
                  ? 'A key is stored. Saving a new one replaces it.'
                  : 'No key stored; the agent falls back to the environment default.'}
              </p>
              <form action={keyAction} className="flex flex-col gap-3">
                <Field label="API key">
                  <Input name="api_key" type="password" placeholder="sk-…" required autoComplete="off" />
                </Field>
                {keyState.error ? <p className="text-[12px] text-[var(--color-danger)]">{keyState.error}</p> : null}
                <Button type="submit" size="sm" disabled={keyPending} className="self-start">
                  {keyPending ? 'Saving…' : 'Save key'}
                </Button>
              </form>
            </Panel>
          </>
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
