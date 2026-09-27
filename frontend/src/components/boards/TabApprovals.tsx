import { useEffect, useState } from 'react'
import { Check, Clock, X } from 'lucide-react'
import { useListApprovalsQuery, useApproveApprovalMutation, useRejectApprovalMutation } from '@/store/api/stream'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { describeError } from '@/hooks/use-action-form'
import { DiffViewer } from '@/components/approvals/DiffViewer'
import { SkeletonText } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatDuration } from '@/lib/formatters'
import type { Approval } from '@/lib/domain'

/**
 * Tab Approvals (US-AD34, US-AD35, US-AD33; screen 20-task-drawer).
 *
 * The gate, read from inside the task it is holding. The design draws this tab
 * with a count on it and nothing about the panel body, so the body follows the
 * approvals inbox — one card per waiting decision, the proposed payload
 * verbatim, the two decision buttons — and then differs where the context
 * differs:
 *
 *  - **Filtered to this task, client-side.** `GET /api/v1/approvals` takes no
 *    `task_id`; it returns the organisation's queue. Filtering here is not a
 *    shortcut around a missing parameter, it is the only correct read of an API
 *    that does not take one: the repository has `ListTaskApprovals` (queries.sql
 *    `ListTaskApprovals`, service `TaskApprovals`), but **no route exposes it**.
 *    The inbox needs the whole queue anyway, so this keeps one cache entry
 *    instead of inventing a second query shape the server does not serve.
 *    A real `GET /tasks/{id}/approvals` is recorded in `docs/OPEN-ISSUES.md` —
 *    it would also carry decided history, which this tab deliberately omits.
 *  - **No `decision` filter here.** The endpoint already answers pending rows
 *    whose deadline has not passed (`ListPendingApprovals`), so re-filtering
 *    would be a duplicate of the server's rule sitting in the client where it
 *    can drift. The expiry edge belongs to the server, and the screen shows what
 *    it is given.
 *  - **The role gate reads `admin`.** The canonical story (US-AD35 AC4) wants
 *    reject at admin, and approve at member. The server implements both at
 *    admin. Splitting the two here would mean rendering a button that 403s for
 *    a member who was told — by this screen — that they could press it. So the
 *    screen follows the server, and the disagreement is recorded in
 *    `docs/OPEN-ISSUES.md` rather than papered over.
 */
export function TabApprovals({ taskID }: { taskID: string }) {
  const t = useT()
  const { data, isLoading } = useListApprovalsQuery()
  const mine = (data ?? []).filter((approval) => approval.task_id === taskID)

  if (isLoading) return <SkeletonText lines={3} />
  if (mine.length === 0) return <p className="text-[12px] text-[var(--color-tertiary)]">{t['approvals.none']}</p>

  return (
    <ul className="flex flex-col gap-3">
      {mine.map((approval) => (
        <ApprovalCard key={approval.id} approval={approval} />
      ))}
    </ul>
  )
}

/** One waiting decision: what is proposed, how long is left, what to do about it. */
function ApprovalCard({ approval }: { approval: Approval }) {
  const t = useT()
  const canDecide = useCanAct('admin')
  const [approve] = useApproveApprovalMutation()
  const [reject] = useRejectApprovalMutation()
  const [state, setState] = useState<{ pending: boolean; error: string | null }>({ pending: false, error: null })
  const [reason, setReason] = useState('')

  async function onApprove() {
    setState({ pending: true, error: null })
    try {
      await approve(approval.id).unwrap()
      setState({ pending: false, error: null })
    } catch (error) {
      setState({ pending: false, error: describeError(error) })
    }
  }

  async function onReject() {
    setState({ pending: true, error: null })
    try {
      await reject({ id: approval.id, reason }).unwrap()
      setState({ pending: false, error: null })
    } catch (error) {
      setState({ pending: false, error: describeError(error) })
    }
  }

  return (
    <li
      data-approval-id={approval.id}
      className="flex flex-col gap-2 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface)] p-3"
    >
      {/* The exact proposal, not a summary of it: paraphrasing it would hide
          the thing being approved. */}
      <DiffViewer preview={approval.preview_json} />
      <Countdown
        expiresAt={approval.expires_at}
        label={t['approvals.expiresIn']}
        expiredLabel={t['approvals.expired']}
      />

      {canDecide ? (
        <>
          <Input
            aria-label={t['approvals.reason']}
            placeholder={t['approvals.reason']}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            disabled={state.pending}
          />
          <div className="flex items-center gap-2">
            <Button type="button" size="sm" variant="primary" disabled={state.pending} onClick={onApprove}>
              <Check size={13} strokeWidth={2} aria-hidden="true" />
              {t['approvals.approve']}
            </Button>
            {/*
              Reject needs a reason and the API enforces it, so the button stays
              disabled until one is typed. An enabled button that always fails
              with a validation error teaches the operator nothing.
            */}
            <Button
              type="button"
              size="sm"
              variant="danger"
              disabled={state.pending || reason.trim() === ''}
              onClick={onReject}
            >
              <X size={13} strokeWidth={2} aria-hidden="true" />
              {t['approvals.reject']}
            </Button>
          </div>
        </>
      ) : (
        <p className="text-[11px] text-[var(--color-tertiary)]">{t['approvals.needsAdmin']}</p>
      )}

      {state.error ? <p className="text-[11px] text-[var(--color-danger)]">{state.error}</p> : null}
    </li>
  )
}

/**
 * The remaining time, recomputed from `expires_at` on every tick.
 *
 * Decrementing a counter instead would drift: a suspended tab, a throttled
 * interval, or a missed tick all make the screen disagree with the server's own
 * deadline, and the deadline is the part that matters. An unset `expires_at`
 * renders the neutral label rather than a zero.
 */
function Countdown({ expiresAt, label, expiredLabel }: { expiresAt: string; label: string; expiredLabel: string }) {
  const [, setTick] = useState(0)

  useEffect(() => {
    const timer = setInterval(() => setTick((n) => n + 1), 1000)
    return () => clearInterval(timer)
  }, [])

  const left = new Date(expiresAt).getTime() - Date.now()
  const expired = !expiresAt || Number.isNaN(left) || left <= 0

  return (
    <p className={cnCountdown(expired)} data-expired={expired ? 'true' : 'false'} title={expiresAt || undefined}>
      <Clock size={12} strokeWidth={2} aria-hidden="true" />
      {expired ? expiredLabel : `${label} ${formatDuration(left)}`}
    </p>
  )
}

function cnCountdown(expired: boolean): string {
  const base = 'flex items-center gap-1.5 font-mono text-[10px]'
  return expired ? `${base} text-[var(--color-danger)]` : `${base} text-[var(--color-tertiary)]`
}
