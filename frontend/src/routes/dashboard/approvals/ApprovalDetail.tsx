import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, Check, Clock, X } from 'lucide-react'
import { useGetApprovalQuery, useApproveApprovalMutation, useRejectApprovalMutation } from '@/store/api/stream'
import { useSseCache } from '@/hooks/use-sse-cache'
import { useGetTaskQuery } from '@/store/api/boards'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { describeError } from '@/hooks/use-action-form'
import { DiffViewer } from '@/components/approvals/DiffViewer'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Panel } from '@/components/ui/card'
import { SkeletonText } from '@/components/ui/skeleton'
import { formatDuration, formatEstimatedMicroUSD, formatTokens, shortID } from '@/lib/formatters'
import { formatDateTime } from '@/lib/format'
import type { Approval, EventEnvelope } from '@/lib/domain'

/**
 * Screen: approval detail — `design/stitch-output/v2/36-approval-detail.html`,
 * US-AD34 (approve) + US-AD35 (reject).
 *
 * The design fixes the shape and this follows it: task context, the proposing
 * agent, the gated action with its preview verbatim, the reject form, and the
 * run's event trail. Two things the mockup shows are deliberately NOT built, and
 * the reasons matter more than the omissions:
 *
 *  - **"Minimum 10 characters" for the reason.** The mockup states it; the
 *    server does not implement it (`internal/board/approval.go:218-220` trims and
 *    rejects only empty). Building the stricter rule here would refuse reasons
 *    the API accepts, and the operator would have no way to know why. The
 *    disagreement is recorded in `docs/OPEN-ISSUES.md` rather than guessed at.
 *  - **The approver's access table** (who may decide, and under what route). That
 *    is spec narration. The role gate is the same one the approvals tab uses, and
 *    the route table is not something a user of the screen can act on.
 *
 * The design also prints a full `sha256:` digest of the payload. It is rendered
 * as a short form computed from the stored bytes, because the digest of what was
 * approved is the auditable fact — a truncated mockup string would be theatre.
 */
export function ApprovalDetail() {
  const { approvalID } = useParams<{ approvalID: string }>()
  const t = useT()
  const { data: approval, isLoading, isError } = useGetApprovalQuery(approvalID as string)
  const { data: task } = useGetTaskQuery(approval?.task_id as string, { skip: !approval?.task_id })
  const { events } = useSseCache(task?.board_id ?? null)

  if (isLoading) return <SkeletonText lines={6} />

  // A failed fetch must not read as a slow one, and a decided approval must not
  // read as pending: the two answers are the difference between "wait" and "this
  // is over", and only one of them is actionable.
  if (isError || !approval) {
    return <p className="p-4 text-[12px] text-[var(--color-danger)]">{t['approvalDetail.loadFailed']}</p>
  }

  const decided = approval.decision !== 'pending'

  return (
    <>
      <WorkspaceTopbar title={t['approvalDetail.title']} subtitle={shortID(approval.id)} />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <Link
          to=".."
          relative="path"
          className="flex items-center gap-1.5 self-start font-mono text-[11px] text-[var(--color-tertiary)] transition-colors hover:text-[var(--color-accent)]"
        >
          <ArrowLeft size={12} strokeWidth={2} aria-hidden="true" />
          {t['approvalDetail.back']}
        </Link>

        <Panel className="p-3">
          <SectionTitle>{t['approvalDetail.context']}</SectionTitle>
          <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1.5 font-mono text-[11px]">
            <Row label={t['approvalDetail.task']} value={task?.title ?? shortID(approval.task_id)} />
            <Row label={t['approvalDetail.statusLabel']} value={task?.status ?? '—'} />
            <Row label={t['approvalDetail.cost']} value={formatEstimatedMicroUSD(task?.cost_micros ?? 0)} />
            <Row
              label={t['approvalDetail.tokens']}
              value={formatTokens((task?.tokens_in ?? 0) + (task?.tokens_out ?? 0))}
            />
            <Row label={t['approvalDetail.requestedBy']} value={approval.requested_by} />
            <Row label={t['approvalDetail.gateMode']} value={approval.gate_mode} />
            <Row label={t['approvalDetail.run']} value={shortID(approval.run_id)} />
            <Row label={t['approvalDetail.requestedAt']} value={formatDateTime(approval.created_at, 'en')} />
          </dl>
        </Panel>

        <Panel className="p-3">
          <SectionTitle>{t['approvalDetail.action']}</SectionTitle>
          {/* The exact proposal, never summarised: paraphrasing it would hide the
              thing being approved. */}
          <DiffViewer preview={approval.preview_json} className="mt-2" />
          {/*
            Only while the hold is open. `DecideApproval` sets
            `reason = COALESCE($5, reason)` (queries.sql:1664), so a rejection
            OVERWRITES the reason the request was raised with — this column holds
            one fact at a time. Rendering it here after a decision would label the
            approver's rejection reason as the requester's, and show it twice. The
            data-model consequence is recorded in docs/OPEN-ISSUES.md.
          */}
          {!decided && approval.reason ? (
            <p className="mt-2 text-[12px] text-[var(--color-secondary)]">{approval.reason}</p>
          ) : null}
        </Panel>

        {decided ? (
          <Panel className="p-3">
            <SectionTitle>{t['approvalDetail.decision']}</SectionTitle>
            <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1.5 font-mono text-[11px]">
              <Row label={t['approvalDetail.outcome']} value={approval.decision} />
              <Row label={t['approvalDetail.decidedBy']} value={approval.decided_by || '—'} />
              <Row label={t['approvalDetail.decidedAt']} value={formatDateTime(approval.decided_at, 'en')} />
            </dl>
            {/* The rejection reason lives in `reason`, the same column the request
                used — so it is shown here rather than assumed empty. */}
            {approval.decision === 'rejected' && approval.reason ? (
              <p className="mt-2 text-[12px] text-[var(--color-secondary)]">{approval.reason}</p>
            ) : null}
          </Panel>
        ) : (
          <DecisionPanel approval={approval} />
        )}

        <Panel className="p-3">
          <SectionTitle>{t['approvalDetail.trail']}</SectionTitle>
          <EventTrail
            events={(events ?? []).filter((event) => event.task_id === approval.task_id)}
            empty={t['approvalDetail.noEvents']}
          />
        </Panel>
      </div>
    </>
  )
}

/**
 * The two decisions.
 *
 * Reject needs a reason because the API refuses one without it (US-AD35 AC2),
 * so the button is disabled until something is typed — an enabled button that
 * always fails with a validation error teaches the operator nothing.
 */
function DecisionPanel({ approval }: { approval: Approval }) {
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

  if (!canDecide) {
    return (
      <Panel className="p-3">
        <SectionTitle>{t['approvalDetail.decision']}</SectionTitle>
        <p className="mt-2 text-[12px] text-[var(--color-tertiary)]">{t['approvals.needsAdmin']}</p>
      </Panel>
    )
  }

  return (
    <Panel className="p-3">
      <SectionTitle>{t['approvalDetail.decision']}</SectionTitle>
      <Remaining expiresAt={approval.expires_at} />

      <div className="mt-3 flex flex-col gap-2">
        <Input
          aria-label={t['approvals.reason']}
          placeholder={t['approvals.reason']}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          disabled={state.pending}
        />
        {/*
          Quick reasons. The mockup lists maintenance-window and replication-lag;
          the product keeps the mechanism and drops the example text, because a
          canned reason that does not fit the hold is worse than typing.
        */}
        <div className="flex flex-wrap gap-1.5">
          {QUICK_REASONS.map((key) => (
            <button
              key={key}
              type="button"
              disabled={state.pending}
              // The typed reason is the TRANSLATED text: what lands in the API and
              // in the audit record must be what the operator saw and chose.
              onClick={() => setReason(t[key])}
              className="rounded-[4px] border border-[var(--color-border-subtle)] px-2 py-0.5 font-mono text-[10px] text-[var(--color-secondary)] transition-colors hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              {t[key]}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <Button type="button" size="sm" variant="primary" disabled={state.pending} onClick={onApprove}>
            <Check size={13} strokeWidth={2} aria-hidden="true" />
            {t['approvalDetail.approveAndRun']}
          </Button>
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
        {state.error ? <p className="text-[11px] text-[var(--color-danger)]">{state.error}</p> : null}
      </div>
    </Panel>
  )
}

/**
 * Quick reasons, in the operator's language.
 *
 * They are i18n keys, not literals: the queue is bilingual and a canned reason
 * that arrives in the wrong language would be pasted into the audit record.
 */
const QUICK_REASONS = [
  'approvalDetail.quick.maintenance',
  'approvalDetail.quick.lag',
  'approvalDetail.quick.window',
] as const

/** A labelled fact. `dd` first in the markup is avoided so the pair reads in DOM order. */
function Row({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt className="text-[var(--color-tertiary)]">{label}</dt>
      <dd className="truncate text-[var(--color-primary)]">{value}</dd>
    </>
  )
}

/**
 * Remaining time, recomputed from `expires_at` on every tick rather than
 * decremented, so a suspended tab cannot drift away from the server's deadline.
 */
function Remaining({ expiresAt }: { expiresAt: string }) {
  const t = useT()
  const [, tick] = useState(0)

  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [])

  const left = new Date(expiresAt).getTime() - Date.now()
  const expired = !expiresAt || Number.isNaN(left) || left <= 0

  return (
    <p
      data-expired={expired ? 'true' : 'false'}
      className={`mt-1 flex items-center gap-1.5 font-mono text-[10px] ${
        expired ? 'text-[var(--color-danger)]' : 'text-[var(--color-tertiary)]'
      }`}
    >
      <Clock size={12} strokeWidth={2} aria-hidden="true" />
      {expired ? t['approvals.expired'] : `${t['approvals.expiresIn']} ${formatDuration(left)}`}
    </p>
  )
}

/**
 * The run's live tail.
 *
 * Read from the board event stream rather than from an events endpoint: the
 * stream is the same source that already keeps the board and the approval queue
 * fresh, and the `events` table has no route of its own. The list is bounded by
 * the stream's own buffer, which is what "recent history" means here.
 */
function EventTrail({ events, empty }: { events: EventEnvelope[]; empty: string }) {
  if (events.length === 0) return <p className="mt-2 text-[12px] text-[var(--color-tertiary)]">{empty}</p>

  return (
    <ul className="mt-2 flex flex-col font-mono text-[10px]">
      {events.map((event) => (
        <li
          key={event.id}
          className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] py-1 last:border-b-0"
        >
          <span className="text-[var(--color-quaternary)]">{formatDateTime(event.created_at, 'en')}</span>
          <span className="text-[var(--color-secondary)]">{event.kind}</span>
        </li>
      ))}
    </ul>
  )
}

/**
 * Local section heading, matching the drawer's own `SectionTitle` rather than a
 * shared export: the two screens are the only callers, and a shared component
 * for two labels would be an abstraction with no second use.
 */
function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 className="font-mono text-[10px] uppercase tracking-wide text-[var(--color-tertiary)]">{children}</h2>
}
