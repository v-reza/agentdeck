import { useListApprovalsQuery, useApproveApprovalMutation, useRejectApprovalMutation } from '@/store/api/stream'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { formatRelative, shortID } from '@/lib/formatters'
import type { Approval } from '@/lib/domain'
import { SkeletonRows } from '@/components/ui/skeleton'

/**
 * Screen: approvals inbox (ARCHITECTURE 8, US-AD gate decisions).
 *
 * The preview is rendered verbatim — it is the exact payload the agent proposed,
 * and summarising it would hide the thing the operator is being asked to
 * approve. Decisions are single clicks that invalidate the Approval tag, so the
 * row leaves the pending list on its own.
 */
export function ApprovalInbox() {
  const { data, isLoading } = useListApprovalsQuery()
  const [approve] = useApproveApprovalMutation()
  const [reject] = useRejectApprovalMutation()

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
          pending.map((approval) => (
            <ApprovalRow
              key={approval.id}
              approval={approval}
              onApprove={() => approve(approval.id)}
              onReject={() => reject({ id: approval.id, reason: 'rejected from inbox' })}
            />
          ))
        )}
      </div>
    </>
  )
}

function ApprovalRow({
  approval,
  onApprove,
  onReject,
}: {
  approval: Approval
  onApprove: () => void
  onReject: () => void
}) {
  return (
    <Panel className="p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="rounded-[4px] bg-[var(--color-accent-tint)] px-1.5 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-accent)]">
            {approval.gate_mode}
          </span>
          <span className="font-mono text-[11px] text-[var(--color-secondary)]">task {shortID(approval.task_id)}</span>
        </div>
        <span className="font-mono text-[10px] text-[var(--color-quaternary)]">
          {formatRelative(approval.created_at)}
        </span>
      </div>

      {approval.reason ? <p className="mt-2 text-[12px] text-[var(--color-secondary)]">{approval.reason}</p> : null}

      {approval.preview_json ? (
        <pre className="mt-2 max-h-[160px] overflow-auto rounded-[6px] bg-[var(--color-surface-sunken)] p-2 font-mono text-[10px] leading-relaxed text-[var(--color-primary)]">
          {approval.preview_json}
        </pre>
      ) : null}

      <div className="mt-3 flex gap-2">
        <Button size="sm" onClick={onApprove}>
          Approve
        </Button>
        <Button size="sm" variant="danger" onClick={onReject}>
          Reject
        </Button>
      </div>
    </Panel>
  )
}
