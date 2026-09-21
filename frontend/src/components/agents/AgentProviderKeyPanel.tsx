import { useState, type FormEvent } from 'react'
import { useDeleteProviderKeyMutation, usePutProviderKeyMutation } from '@/store/api/agents'
import { describeError } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import { ProviderKeyFields } from './ProviderKeyFields'

const FORM_ID = 'provider-key-form'

/** What the panel writes, and everything it knows about the target. */
export interface ProviderKeyTarget {
  id: string
  name: string
  provider: string
  model: string
  has_provider_key: boolean
}

/**
 * Screen 27-agent-provider-key — the 420px credential panel (US-AD86), used
 * from two places: the registry's register modal, and the agent detail page
 * (rotation / revocation).
 *
 * It is a `Modal` with `placement="right"` and `size="panel"` rather than a new
 * dialog: the focus trap, the Escape handler, the scroll lock and the focus
 * restore are one contract, and a second implementation of them is a second
 * chance to get them wrong. What the design adds is geometry — a full-height
 * panel pinned to the right edge, 420px wide — which is what those two props
 * carry.
 *
 * The key is read from the form on submit, never mirrored into React state.
 * `ProviderKeyFields` renders an uncontrolled input, so a state copy would stay
 * empty however much the operator typed and gate the save button on a value
 * that could never arrive.
 *
 * Reading a credential is impossible by design: no endpoint returns one. So the
 * panel reports `has_provider_key` plus the mask this session's own PUT
 * produced, and nothing else. It never invents a mask for a key it did not see.
 *
 * Only owner/admin get here — `PUT`/`DELETE /agents/{id}/provider-key` are
 * gated at Admin by the route table. The hidden trigger is a courtesy; the
 * server is the boundary.
 */
export function AgentProviderKeyPanel({
  open,
  onClose,
  agent,
  onSaved,
}: {
  open: boolean
  onClose: () => void
  /** Absent while the agent is unsaved: there is no id to write against yet. */
  agent?: ProviderKeyTarget
  /** Reports the new `has_provider_key` so the host can update its own badge. */
  onSaved?: (hasKey: boolean) => void
}) {
  const t = useT()
  const [putKey, { isLoading: saving }] = usePutProviderKeyMutation()
  const [deleteKey, { isLoading: revoking }] = useDeleteProviderKeyMutation()
  const [masked, setMasked] = useState<string | undefined>(undefined)
  const [hasKey, setHasKey] = useState(Boolean(agent?.has_provider_key))
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!agent) return
    // Captured before the first `await`: React clears `currentTarget` once the
    // handler's synchronous phase is over, so reading it later would be null.
    const form = event.currentTarget
    const apiKey = String(new FormData(form).get('panel-api-key') ?? '').trim()
    setError(null)
    setSaved(false)
    if (apiKey === '') {
      setError(t['agents.key.empty'])
      return
    }
    try {
      const answer = await putKey({ id: agent.id, apiKey }).unwrap()
      setHasKey(answer.has_provider_key)
      setMasked(answer.masked_key)
      // The plaintext leaves the form the moment the server has answered.
      // Holding it longer is the only way it could end up somewhere it must not.
      form.reset()
      setSaved(true)
      onSaved?.(answer.has_provider_key)
    } catch (rejection) {
      setError(describeError(rejection))
    }
  }

  async function revoke() {
    if (!agent) return
    setError(null)
    setSaved(false)
    try {
      const answer = await deleteKey(agent.id).unwrap()
      setHasKey(answer.has_provider_key)
      setMasked(undefined)
      setSaved(true)
      onSaved?.(answer.has_provider_key)
    } catch (rejection) {
      setError(describeError(rejection))
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t['agents.key.title']}
      description={agent ? `${agent.name} · ${agent.provider} / ${agent.model}` : t['agents.key.newAgent']}
      size="panel"
      placement="right"
      footer={
        <div className="flex w-full items-center justify-between gap-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            {t['action.cancel']}
          </Button>
          <div className="flex items-center gap-2">
            {agent && hasKey ? (
              <Button variant="danger" size="sm" disabled={revoking} onClick={revoke}>
                {revoking ? t['agents.key.revoking'] : t['agents.key.revoke']}
              </Button>
            ) : null}
            <Button type="submit" form={FORM_ID} variant="primary" size="sm" disabled={!agent || saving}>
              {saving ? t['agents.key.saving'] : t['agents.key.save']}
            </Button>
          </div>
        </div>
      }
    >
      <form id={FORM_ID} onSubmit={save} className="flex flex-col gap-3">
        <dl className="grid grid-cols-2 gap-2 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5 font-mono text-[11px]">
          <div className="min-w-0">
            <dt className="text-[10px] uppercase text-[var(--color-tertiary)]">{t['agents.key.target']}</dt>
            <dd className="truncate text-[var(--color-primary)]">{agent?.name ?? t['agents.key.unsaved']}</dd>
          </div>
          <div className="min-w-0">
            <dt className="text-[10px] uppercase text-[var(--color-tertiary)]">{t['agents.key.provider']}</dt>
            <dd className="truncate text-[var(--color-primary)]">{agent?.provider ?? '—'}</dd>
          </div>
        </dl>

        <ProviderKeyFields inputName="panel-api-key" hasKey={hasKey} masked={masked} agentID={agent?.id} />

        {/* One inline error, no toast: the panel is the surface that caused the
            failure, and the toast listener deliberately refuses to duplicate a
            write's message in a second place. */}
        {error ? (
          <p
            id="provider-key-error"
            role="alert"
            className="rounded-[6px] border border-[var(--color-danger)]/30 p-2.5 text-[11px] leading-snug text-[var(--color-danger)]"
          >
            {`${t['agents.key.failed']}: ${error}`}
          </p>
        ) : saved ? (
          <p className="rounded-[6px] border border-[var(--color-status-done)]/30 p-2.5 text-[11px] leading-snug text-[var(--color-status-done)]">
            {t['agents.key.saved']}
          </p>
        ) : null}

        {!agent ? (
          <p className="text-[11px] leading-relaxed text-[var(--color-tertiary)]">{t['agents.key.newAgentHint']}</p>
        ) : null}

        <p className="text-[10.5px] leading-snug text-[var(--color-quaternary)]">{t['agents.key.encryption']}</p>
      </form>
    </Modal>
  )
}
