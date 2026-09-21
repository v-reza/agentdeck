import { useState } from 'react'
import { useValidateAgentMutation } from '@/store/api/agents'
import { describeError } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { Button } from '@/components/ui/button'
import { Field, FieldError, Input } from '@/components/ui/input'

/**
 * The credential block shared by the register modal (26-agent-form) and the
 * 420px panel (27-agent-provider-key), factored out because the plan asks for
 * one panel used in two places and both hosts must agree on what "write-only"
 * means.
 *
 * It is the *fields*, not the dialog: the modal embeds it in its own form, the
 * panel wraps it in right-anchored chrome with its own save and revoke.
 *
 * The key is write-only (US-AD86 AC2). Nothing here can read a stored value
 * back, because no endpoint returns one. `masked` is only what the PUT answer
 * reported for the value this session sent, so it is shown when we have it and
 * the field stays a plain replace-me input when we do not — deriving a mask from
 * `has_provider_key` would be inventing a secret shape.
 *
 * The handshake is a manual button (US-AD96 AC5), never fired on submit, and its
 * result is inline. `POST /agents/{id}/validate` reads the stored endpoint and
 * credential, so it needs a saved agent: on an unsaved one the button is
 * disabled and says why instead of promising a request that cannot be made.
 */
export function ProviderKeyFields({
  inputName,
  hasKey,
  masked,
  agentID,
  error,
}: {
  /** Form field name. The host form reads the value under this key. */
  inputName: string
  /** The generated column `agents.has_provider_key` (DECISIONS 6A.I). */
  hasKey: boolean
  /** The masked shape the last PUT answered with, if this session sent one. */
  masked?: string
  /** Absent while the agent is unsaved — the handshake has nothing to read yet. */
  agentID?: string
  /** A rejection the host form routed to this field. */
  error?: string
}) {
  const t = useT()
  const [validate, { isLoading }] = useValidateAgentMutation()
  const [result, setResult] = useState<{ ok: boolean; detail: string } | { error: string } | null>(null)

  async function runTest() {
    if (!agentID) return
    setResult(null)
    try {
      // `ok: false` is a successful test with a negative result, so the HTTP
      // status is not the thing to read here.
      setResult(await validate(agentID).unwrap())
    } catch (rejection) {
      setResult({ error: describeError(rejection) })
    }
  }

  const refused = result !== null && ('error' in result || !result.ok)

  return (
    <div className="flex flex-col gap-2.5">
      <Field label={t['agents.cred.field']}>
        <Input
          name={inputName}
          type="password"
          autoComplete="off"
          placeholder="sk-..."
          className="font-mono"
          aria-invalid={error ? true : undefined}
        />
        <FieldError>{error}</FieldError>
      </Field>

      <div className="flex items-center justify-between gap-3">
        <span
          data-testid="provider-key-status"
          className={`rounded-[4px] px-1.5 py-0.5 font-mono text-[10px] font-semibold ${
            hasKey
              ? 'bg-[var(--color-accent)]/10 text-[var(--color-accent)]'
              : 'bg-[var(--color-warning)]/15 text-[var(--color-warning)]'
          }`}
        >
          {hasKey ? t['agents.cred.stored'] : t['agents.cred.missing']}
        </span>
        {masked ? (
          <span className="flex items-center gap-1.5 font-mono text-[10.5px] text-[var(--color-tertiary)]">
            <span data-testid="provider-key-masked">{masked}</span>
            <span className="rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5">
              {t['agents.cred.masked']}
            </span>
          </span>
        ) : null}
      </div>

      <p className="text-[10.5px] leading-snug text-[var(--color-tertiary)]">{t['agents.cred.fieldHint']}</p>

      <div className="flex items-center justify-between gap-3 border-t border-[var(--color-border-subtle)] pt-2.5">
        <span className="text-[10.5px] leading-snug text-[var(--color-quaternary)]">
          {agentID ? t['agents.cred.optional'] : t['agents.cred.testNeedsSave']}
        </span>
        <Button size="sm" variant="secondary" className="shrink-0" disabled={!agentID || isLoading} onClick={runTest}>
          {isLoading ? t['agents.cred.testing'] : t['agents.cred.test']}
        </Button>
      </div>

      {result ? (
        <p
          role="alert"
          data-testid="provider-key-test-result"
          className={`rounded-[6px] border p-2 text-[11px] leading-snug ${
            refused
              ? 'border-[var(--color-danger)]/30 text-[var(--color-danger)]'
              : 'border-[var(--color-status-done)]/30 text-[var(--color-status-done)]'
          }`}
        >
          {'error' in result
            ? `${t['agents.cred.testFailed']}: ${result.error}`
            : `${result.ok ? t['agents.cred.testOk'] : t['agents.cred.testRefused']} ${result.detail}`}
        </p>
      ) : null}
    </div>
  )
}
