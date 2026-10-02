import { useState } from 'react'
import { AlertTriangle, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EmptyState, Panel } from '@/components/ui/card'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useCloseAccountMutation, useMeQuery } from '@/store/api/session'
import { useT } from '@/hooks/use-t'

/**
 * Screen 17-close-account — closing your own account (US-AD98).
 *
 * AC1 is the whole interaction: the email has to be TYPED and has to match
 * exactly, so the destructive button stays disabled until it does. A confirm
 * dialog that only asks "are you sure" would satisfy the design and not the AC.
 *
 * AC3 is the refusal worth rendering: the last owner of a workspace that still
 * has other members gets a 409, and that is the one outcome the operator can do
 * something about (hand over ownership first). It is shown inline, against the
 * form, not as a toast.
 *
 * WHAT IS NOT HERE:
 * - **A reason field.** The design draws an optional "reason for closing"
 *   dropdown, and `DELETE /auth/me` accepts exactly one field,
 *   `confirm_email`. A reason with nowhere to go is a field that lies.
 * - **A soft-delete countdown.** AC5 makes the closure reversible for 30 days,
 *   and the design's "permanent and irreversible" copy would contradict it. The
 *   screen says the closure is soft, because that is what the server does.
 * - **Workspace deletion.** The server decides which workspaces go with the
 *   account (AC2: only ones where this user is the sole member). The screen
 *   lists what the caller owns so the consequence is visible, and does not
 *   pretend to choose.
 */

export function CloseAccount() {
  const t = useT()
  const { data, isLoading } = useMeQuery()
  const [closeAccount, { isLoading: busy }] = useCloseAccountMutation()

  const [typed, setTyped] = useState('')
  const [acknowledged, setAcknowledged] = useState(false)
  const [error, setError] = useState('')

  const email = data?.email ?? ''
  // AC1: exact match, and the checkbox is the second gate. Two independent
  // conditions, so neither alone can fire the request.
  const matches = typed.trim().length > 0 && typed.trim() === email
  const canSubmit = matches && acknowledged && !busy

  async function submit() {
    setError('')
    try {
      await closeAccount({ confirm_email: typed.trim() }).unwrap()
      // On success the mutation clears the session slice and the router sends
      // the operator to /login. Nothing is rendered from here on purpose: the
      // account this screen belongs to no longer exists.
    } catch (err) {
      // The code arrives as `originalStatus`, not `status`: this API writes every
      // error with `http.Error` (`text/plain`), RTK Query parses responses as
      // JSON, the parse throws, and RTK reports `status: 'PARSING_ERROR'` with
      // the real code kept aside. Branching on `status === 409` compiles and
      // never matches — `Security.tsx` records the same trap, and the e2e caught
      // this copy of it by showing the generic message.
      const rt = err as { status?: unknown; originalStatus?: unknown } | undefined
      const status = Number(rt?.originalStatus ?? rt?.status)
      setError(status === 409 ? t['closeAccount.errorLastOwner'] : t['closeAccount.errorGeneric'])
    }
  }

  if (isLoading) {
    return (
      <div className="p-4">
        <SkeletonRows rows={4} />
      </div>
    )
  }

  if (!data) {
    return (
      <div className="p-4" data-testid="close-account-unavailable">
        <EmptyState title={t['closeAccount.unavailable']} hint={t['closeAccount.unavailableHint']} />
      </div>
    )
  }

  const soloOwned = data.workspaces.filter((w) => w.role === 'owner')

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
      <Panel className="p-4">
        <h1 className="text-[13px] font-semibold text-[var(--color-primary)]">{t['closeAccount.title']}</h1>
        <p className="mt-1 text-[12px] text-[var(--color-secondary)]">{t['closeAccount.intro']}</p>
      </Panel>

      <Panel className="p-4" data-testid="close-account-summary">
        <div className="mb-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
          {t['closeAccount.summary']}
        </div>
        <dl className="grid grid-cols-[160px_1fr] gap-y-2 text-[12px]">
          <dt className="text-[var(--color-tertiary)]">{t['closeAccount.email']}</dt>
          <dd className="font-mono text-[var(--color-primary)]" data-testid="close-account-email">
            {data.email}
          </dd>
          <dt className="text-[var(--color-tertiary)]">{t['closeAccount.name']}</dt>
          <dd className="text-[var(--color-primary)]">{data.name}</dd>
          <dt className="text-[var(--color-tertiary)]">{t['closeAccount.workspaces']}</dt>
          <dd className="text-[var(--color-primary)]" data-testid="close-account-workspaces">
            {soloOwned.length > 0
              ? soloOwned.map((w) => `${w.name} (${w.slug})`).join(', ')
              : t['closeAccount.noSoloWorkspace']}
          </dd>
        </dl>
      </Panel>

      <Panel className="p-4">
        <div className="mb-2 flex items-center gap-2">
          <AlertTriangle size={14} className="text-[var(--color-danger)]" aria-hidden="true" />
          <span className="font-mono text-[10px] uppercase tracking-wider text-[var(--color-danger)]">
            {t['closeAccount.impact']}
          </span>
        </div>
        <ul className="flex list-disc flex-col gap-1 pl-5 text-[12px] text-[var(--color-secondary)]">
          <li>{t['closeAccount.impactSessions']}</li>
          <li>{t['closeAccount.impactLogin']}</li>
          <li>{t['closeAccount.impactSoft']}</li>
        </ul>
      </Panel>

      <Panel className="p-4">
        <div className="mb-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
          {t['closeAccount.confirmTitle']}
        </div>
        <label className="flex flex-col gap-1 text-[12px] text-[var(--color-secondary)]">
          {t['closeAccount.confirmLabel']}
          <input
            type="email"
            value={typed}
            onChange={(event) => setTyped(event.target.value)}
            placeholder={email}
            autoComplete="off"
            data-testid="close-account-confirm-email"
            className="h-8 w-[320px] rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[12px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
          />
        </label>

        <label className="mt-3 flex items-start gap-2 text-[12px] text-[var(--color-secondary)]">
          <input
            type="checkbox"
            checked={acknowledged}
            onChange={(event) => setAcknowledged(event.target.checked)}
            data-testid="close-account-ack"
            className="mt-[3px]"
          />
          {t['closeAccount.ackLabel']}
        </label>

        {error ? (
          <p role="alert" data-testid="close-account-error" className="mt-3 text-[12px] text-[var(--color-danger)]">
            {error}
          </p>
        ) : null}

        <div className="mt-3 flex items-center gap-2">
          <Button
            variant="danger"
            size="sm"
            disabled={!canSubmit}
            onClick={() => void submit()}
            data-testid="close-account-submit"
          >
            <Trash2 size={13} aria-hidden="true" />
            {t['closeAccount.submit']}
          </Button>
          <span className="text-[11px] text-[var(--color-tertiary)]" data-testid="close-account-gate">
            {matches ? t['closeAccount.gateReady'] : t['closeAccount.gateBlocked']}
          </span>
        </div>
      </Panel>
    </div>
  )
}
