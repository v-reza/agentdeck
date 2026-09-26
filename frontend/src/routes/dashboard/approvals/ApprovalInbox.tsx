import { useEffect, useState } from 'react'
import { Check, Clock, X } from 'lucide-react'
import { useListApprovalsQuery, useApproveApprovalMutation, useRejectApprovalMutation } from '@/store/api/stream'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { DiffViewer } from '@/components/approvals/DiffViewer'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useCanAct } from '@/hooks/use-orgs'
import { formatDuration, shortID } from '@/lib/formatters'
import type { Approval } from '@/lib/domain'

/**
 * Screen: approvals inbox — `design/stitch-output/v2/35-approval-inbox.html`,
 * US-AD37.
 *
 * What the design fixes and this follows: one card per waiting decision, each
 * carrying the agent, the task, the board, the remaining time, the proposed
 * payload verbatim, and the two decision buttons. The preview is never
 * summarised — it is the exact payload the agent proposed, and paraphrasing it
 * would hide the thing the operator is being asked to approve.
 *
 * AC2 asks for "waktu tersisa", and that is what is rendered: `expires_at` is
 * the gate's own deadline, so the countdown is computed from it. The screen
 * previously showed `created_at` as a relative timestamp, which reads as
 * progress but is not a deadline — an approval could sit at "2h ago" past its
 * own expiry with nothing on screen saying so. The clock icon comes with the
 * countdown; neither existed before.
 *
 * The countdown ticks on a one-second interval that re-derives from
 * `expires_at` every time rather than decrementing a counter, so a suspended
 * tab or a missed tick cannot make the screen drift away from the server's
 * deadline. `expires_at` unset renders the neutral state, not a zero.
 *
 * AC4 is enforced by hiding the decision buttons below admin. The backend
 * requires the same role, so this hides a control that would 403 — it does not
 * grant anything.
 */
export function ApprovalInbox() {
  const { data, isLoading } = useListApprovalsQuery()
  const [approve] = useApproveApprovalMutation()
  const [reject] = useRejectApprovalMutation()
  const canDecide = useCanAct('admin')

  const pending = (data ?? []).filter((approval) => approval.decision === 'pending')

  return (
    <>
      <WorkspaceTopbar
        title="Approvals"
        subtitle={pending.length > 0 ? `${pending.length} awaiting decision` : undefined}
      />
      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        {isLoading ? (
          <SkeletonRows rows={4} columns={3} />
        ) : pending.length === 0 ? (
          <EmptyState title="Nothing waiting on you" hint="Approvals appear here when a run pauses for a decision." />
        ) : (
          <>
            {!canDecide ? (
              <div
                role="status"
                className="flex items-center gap-2 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-3.5 py-2 text-[11px] text-[var(--color-secondary)]"
              >
                <span>You can monitor this queue. Approving and rejecting needs the owner or admin role.</span>
              </div>
            ) : null}
            {pending.map((approval) => (
              <ApprovalRow
                key={approval.id}
                approval={approval}
                canDecide={canDecide}
                onApprove={() => approve(approval.id)}
                onReject={() => reject({ id: approval.id, reason: 'rejected from inbox' })}
              />
            ))}
          </>
        )}
      </div>
    </>
  )
}

function ApprovalRow({
  approval,
  canDecide,
  onApprove,
  onReject,
}: {
  approval: Approval
  canDecide: boolean
  onApprove: () => void
  onReject: () => void
}) {
  const remaining = useRemaining(approval.expires_at)

  return (
    <Panel className="p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="rounded-[4px] bg-[var(--color-accent-tint)] px-1.5 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-accent)]">
            {approval.gate_mode}
          </span>
          <span className="font-mono text-[11px] text-[var(--color-secondary)]">task {shortID(approval.task_id)}</span>
        </div>
        <RemainingChip remaining={remaining} />
      </div>

      {approval.reason ? <p className="mt-2 text-[12px] text-[var(--color-secondary)]">{approval.reason}</p> : null}

      {/* The design's "Preview Aksi" card. The payload used to be a bare <pre>
          here, which dropped the card's header — the label naming what kind of
          action is gated — and the border that separates the proposal from the
          decision buttons under it. */}
      {approval.preview_json ? <DiffViewer preview={approval.preview_json} className="mt-2" /> : null}

      {canDecide ? (
        <div className="mt-3 flex gap-2">
          <Button size="sm" onClick={onApprove}>
            <Check size={12} />
            Approve &amp; Run
          </Button>
          <Button size="sm" variant="secondary" className="text-[var(--color-danger)]" onClick={onReject}>
            <X size={12} />
            Reject
          </Button>
        </div>
      ) : null}
    </Panel>
  )
}

/**
 * The deadline chip from the design: a clock, the remaining time, and the tone
 * that carries urgency (red under five minutes, amber under half an hour,
 * accent while there is room). The design also prints `TTL: 30m` next to it;
 * that is derived from `expires_at - created_at` and only restates what the
 * countdown already says, so it is left out rather than shown twice.
 */
function RemainingChip({ remaining }: { remaining: number | null }) {
  if (remaining === null) {
    return <span className="font-mono text-[10px] text-[var(--color-quaternary)]">No deadline</span>
  }

  const expired = remaining <= 0
  const tone = expired
    ? 'var(--color-danger)'
    : remaining < 5 * 60_000
      ? 'var(--color-danger)'
      : remaining < 30 * 60_000
        ? 'var(--color-warning)'
        : 'var(--color-accent)'

  return (
    <span
      data-testid="approval-remaining"
      className="flex items-center gap-1 rounded px-2 py-0.5 font-mono text-[11px] font-semibold"
      style={{ color: tone, backgroundColor: `color-mix(in srgb, ${tone} 10%, transparent)` }}
    >
      <Clock size={12} />
      <span>{expired ? 'Expired' : `Remaining: ${formatDuration(remaining)}`}</span>
    </span>
  )
}

/**
 * Milliseconds until `expires_at`, re-derived on a one-second tick so the value
 * cannot drift from the server's deadline. Null when there is no deadline.
 */
function useRemaining(expiresAt: string): number | null {
  const target = new Date(expiresAt).getTime()
  const valid = Boolean(expiresAt) && !Number.isNaN(target)
  const [, tick] = useState(0)

  useEffect(() => {
    if (!valid) return
    const id = setInterval(() => tick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [valid])

  return valid ? target - Date.now() : null
}
