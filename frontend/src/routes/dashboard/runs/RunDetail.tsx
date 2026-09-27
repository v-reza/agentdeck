import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, CircleAlert, CircleCheck, LoaderCircle } from 'lucide-react'
import { useGetRunQuery, useGetRunSummaryQuery, useListRunStepsQuery, type Step } from '@/store/api/runs'
import { useRunLedgerQuery } from '@/store/api/finops'
import { useT } from '@/hooks/use-t'
import { formatDateTime } from '@/lib/format'
import { formatDuration } from '@/lib/formatters'
import { formatEstimatedMicroUSD, formatTokens, shortID } from '@/lib/formatters'
import { StepTimeline } from '@/components/terminal/StepTimeline'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Panel } from '@/components/ui/card'
import { SkeletonText } from '@/components/ui/skeleton'
import { cn } from '@/lib/cn'
import type { LedgerEntry } from '@/lib/domain'

/**
 * Screen: run detail — `design/stitch-output/v2/32-run-detail.html`, US-AD41.
 *
 * AC1 (status, outcome, duration, cost, tokens, attempt) is what the header
 * carries, because those six facts are what the design puts above the fold and
 * what decides whether anyone reads further.
 *
 * AC2 asks for five tabs — Steps, Logs, Approvals, Artifacts, Ledger. **Two are
 * built: Steps and Ledger.** The other three have no per-RUN endpoint: approvals
 * and artifacts are listed per TASK (`GET /tasks/{id}/approvals`,
 * `GET /tasks/{id}/artifacts`), and the run's own `events` table has no route at
 * all. Rendering them here would mean showing every approval and every artifact
 * of the task as if the run produced them, which for a retried task is a
 * different run's output under this run's name. The rows do carry the
 * discriminator (`artifacts.run_id`, `approvals.run_id`), so the honest version
 * is to filter by it — that belongs to the phase that owns the Ledger explorer,
 * and it is recorded in `docs/OPEN-ISSUES.md` rather than faked here.
 *
 * AC3/AC4: a run outside the caller's organisation answers 404 from the server,
 * and this screen renders that as a sentence. A missing run must not read as an
 * empty one.
 */
type RunTab = 'steps' | 'ledger'
const RUN_TABS: RunTab[] = ['steps', 'ledger']

export function RunDetail() {
  const { runID } = useParams<{ runID: string }>()
  const t = useT()
  const { data: run, isLoading, isError, error } = useGetRunQuery(runID as string)
  const { data: summary } = useGetRunSummaryQuery(runID as string, { skip: !runID })
  const [tab, setTab] = useState<RunTab>('steps')

  if (isLoading) return <SkeletonText lines={6} />

  // 404 is the server's answer for a run that does not exist OR belongs to
  // another organisation (AC3/AC4 — deliberately indistinguishable, so the
  // endpoint cannot be used to probe for other tenants' run ids). Both read as
  // one sentence here; a stand-in "empty run" would hide the difference between
  // "gone" and "never yours".
  const rt = error as { status?: unknown; originalStatus?: unknown } | undefined
  const code = Number(rt?.originalStatus ?? rt?.status)
  if (isError || !run) {
    return (
      <p className="p-4 text-[12px] text-[var(--color-danger)]">
        {code === 404 ? t['runDetail.notFound'] : t['runDetail.loadFailed']}
      </p>
    )
  }

  // `duration_seconds` comes from the summary because the run row does not hold
  // it: for a run still going, the server measures against now().
  const duration = summary ? summary.duration_seconds * 1000 : null

  return (
    <>
      <WorkspaceTopbar title={t['runDetail.title']} subtitle={shortID(run.id)} />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <Link
          to=".."
          relative="path"
          className="flex items-center gap-1.5 self-start font-mono text-[11px] text-[var(--color-tertiary)] transition-colors hover:text-[var(--color-accent)]"
        >
          <ArrowLeft size={12} strokeWidth={2} aria-hidden="true" />
          {t['runDetail.back']}
        </Link>

        <Panel className="p-3">
          <div className="flex items-center gap-2">
            <RunStatusIcon status={run.status} />
            <span className="font-mono text-[12px] text-[var(--color-primary)]">{run.status}</span>
            {run.outcome ? (
              <span className="font-mono text-[11px] text-[var(--color-tertiary)]">{run.outcome}</span>
            ) : null}
            <span className="ml-auto font-mono text-[10px] text-[var(--color-quaternary)]">
              {t['runDetail.attempt']} #{run.attempt}
            </span>
          </div>

          {/* `failure_kind` and `error` are the whole reason anyone opens this
              screen on a run that did not succeed, so they are never folded away. */}
          {run.failure_kind || run.error ? (
            <p className="mt-2 font-mono text-[11px] text-[var(--color-danger)]">
              {run.failure_kind ? `${run.failure_kind}: ` : ''}
              {run.error}
            </p>
          ) : null}
          {run.summary ? <p className="mt-2 text-[12px] text-[var(--color-secondary)]">{run.summary}</p> : null}
          {run.cancel_requested_at ? (
            <p className="mt-2 font-mono text-[11px] text-[var(--color-warning)]">{t['runDetail.cancelRequested']}</p>
          ) : null}

          <dl className="mt-3 grid grid-cols-3 gap-2 font-mono text-[11px]">
            <Metric label={t['runDetail.duration']} value={duration === null ? '—' : formatDuration(duration)} />
            <Metric label={t['runDetail.cost']} value={formatEstimatedMicroUSD(run.cost_micros)} />
            <Metric label={t['runDetail.tokens']} value={formatTokens(run.tokens_in + run.tokens_out)} />
            <Metric label={t['runDetail.tokensIn']} value={formatTokens(run.tokens_in)} />
            <Metric label={t['runDetail.tokensOut']} value={formatTokens(run.tokens_out)} />
            <Metric label={t['runDetail.startedAt']} value={formatDateTime(run.started_at, 'en')} />
          </dl>
        </Panel>

        <div
          role="tablist"
          aria-label={t['runDetail.title']}
          className="flex border-b border-[var(--color-border-subtle)]"
        >
          {RUN_TABS.map((entry) => (
            <button
              key={entry}
              type="button"
              role="tab"
              id={`run-tab-${entry}`}
              aria-selected={tab === entry}
              aria-controls={`run-panel-${entry}`}
              onClick={() => setTab(entry)}
              className={cn(
                'border-b-2 px-3 py-2 font-mono text-[11px] transition-colors',
                tab === entry
                  ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                  : 'border-transparent text-[var(--color-tertiary)] hover:text-[var(--color-secondary)]',
              )}
            >
              {t[entry === 'steps' ? 'runDetail.tab.steps' : 'runDetail.tab.ledger']}
            </button>
          ))}
        </div>

        {tab === 'steps' ? (
          <section role="tabpanel" id="run-panel-steps" aria-labelledby="run-tab-steps">
            <RunSteps runID={run.id} />
          </section>
        ) : (
          <section role="tabpanel" id="run-panel-ledger" aria-labelledby="run-tab-ledger">
            <RunLedger runID={run.id} />
          </section>
        )}
      </div>
    </>
  )
}

/**
 * The run's steps, rendered through the same `StepTimeline` the drawer's Logs tab
 * uses.
 *
 * It is handed one synthetic envelope per step rather than a second timeline
 * renderer: the shape it reads is `{kind, payload, created_at}`, and building a
 * parallel component for the same rows would mean two places to fix when the
 * design changes. The synthesis happens here, at the boundary, so `StepTimeline`
 * stays unaware of runs.
 */
function RunSteps({ runID }: { runID: string }) {
  const t = useT()
  const { data, isLoading, isError } = useListRunStepsQuery(runID)

  if (isLoading) return <SkeletonText lines={4} />
  if (isError) return <p className="text-[12px] text-[var(--color-danger)]">{t['runDetail.stepsFailed']}</p>

  const steps = data ?? []
  if (steps.length === 0) return <p className="text-[12px] text-[var(--color-tertiary)]">{t['runDetail.noSteps']}</p>

  return (
    <StepTimeline
      events={steps.map((step: Step) => ({
        id: `step-${step.seq}`,
        kind: step.kind,
        board_id: '',
        task_id: '',
        run_id: runID,
        created_at: step.started_at,
        payload: {
          name: step.name,
          status: step.status,
          tokens_in: step.tokens_in,
          tokens_out: step.tokens_out,
          cost_micros: step.cost_micros,
          payload_json: step.payload_json,
        },
      }))}
    />
  )
}

/** The run's own ledger rows — this is the per-run money view the design draws. */
function RunLedger({ runID }: { runID: string }) {
  const t = useT()
  const { data, isLoading, isError } = useRunLedgerQuery(runID)

  if (isLoading) return <SkeletonText lines={3} />
  if (isError) return <p className="text-[12px] text-[var(--color-danger)]">{t['runDetail.ledgerFailed']}</p>

  const rows = (data ?? []) as LedgerEntry[]
  if (rows.length === 0) return <p className="text-[12px] text-[var(--color-tertiary)]">{t['runDetail.noLedger']}</p>

  return (
    <ul className="flex flex-col font-mono text-[11px]">
      {rows.map((entry) => (
        <li
          key={entry.id}
          className="flex items-center gap-3 border-b border-[var(--color-border-subtle)] py-1.5 last:border-b-0"
        >
          <span className="text-[var(--color-secondary)]">{entry.kind}</span>
          <span className="ml-auto tabular-nums text-[var(--color-primary)]">
            {formatEstimatedMicroUSD(entry.cost_micros)}
          </span>
          <span className="w-[76px] text-right tabular-nums text-[var(--color-tertiary)]">
            {formatTokens(entry.tokens_in + entry.tokens_out)}
          </span>
        </li>
      ))}
    </ul>
  )
}

function RunStatusIcon({ status }: { status: string }) {
  const shared = { size: 14, strokeWidth: 2, 'aria-hidden': true } as const
  if (status === 'running') return <LoaderCircle {...shared} className="shrink-0 text-[var(--color-warning)]" />
  if (status === 'succeeded') return <CircleCheck {...shared} className="shrink-0 text-[var(--color-success)]" />
  if (status === 'failed' || status === 'cancelled') {
    return <CircleAlert {...shared} className="shrink-0 text-[var(--color-danger)]" />
  }
  return <CircleAlert {...shared} className="shrink-0 text-[var(--color-tertiary)]" />
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col">
      <dt className="text-[10px] uppercase tracking-wide text-[var(--color-quaternary)]">{label}</dt>
      <dd className="truncate tabular-nums text-[var(--color-primary)]">{value}</dd>
    </div>
  )
}
