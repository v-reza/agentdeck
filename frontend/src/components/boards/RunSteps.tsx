import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ChevronDown, ChevronRight, CircleAlert, CircleCheck, LoaderCircle } from 'lucide-react'
import { useListRunStepsQuery, useListTaskRunsQuery, type Step } from '@/store/api/runs'
import { useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'
import { formatMicroUSD, formatTokens, formatDuration } from '@/lib/formatters'
import { formatDateTime } from '@/lib/format'
import { SkeletonText } from '@/components/ui/skeleton'
import { cn } from '@/lib/cn'

/**
 * Tab Logs (US-AD26, US-AD94, screen 20-task-drawer).
 *
 * Every run of the task, each with its steps. A run is the unit that retries, so
 * a task that failed twice before succeeding has three runs and one of them is
 * the one that matters; rendering a flat step list would merge them and hide
 * which attempt each step belonged to.
 *
 * The ACs this does NOT fully answer, and why (both are recorded in
 * `docs/OPEN-ISSUES.md`):
 *
 *  - US-AD94 AC1 asks for cache read/write per step. The `steps` table has no
 *    such columns — they live on `ledger_entries`. Rendering 0 would be an
 *    invented number, so this panel shows the cost and tokens the step actually
 *    reports and stops there.
 *  - US-AD94 AC5 asks for the raw payload to be redacted from a viewer. The app
 *    has no viewer-scoped masking surface anywhere; the role gate lives on the
 *    endpoint. Claiming it here would be a rule enforced nowhere.
 */

/** The status colours AC26 AC1 asks for, mapped once. */
const STATUS_STYLE: Record<Step['status'], { icon: typeof CircleCheck; className: string }> = {
  succeeded: { icon: CircleCheck, className: 'text-[var(--color-success)]' },
  failed: { icon: CircleAlert, className: 'text-[var(--color-danger)]' },
  running: { icon: LoaderCircle, className: 'text-[var(--color-tertiary)]' },
}

export function RunSteps({ taskID }: { taskID: string }) {
  const t = useT()
  const { orgID } = useParams<{ orgID: string }>()
  const lang = useAppSelector((state) => state.lang.lang)
  const { data: runs, isLoading, isError } = useListTaskRunsQuery(taskID)
  const list = runs ?? []

  if (isLoading) return <SkeletonText lines={4} />

  // A failed fetch must not read as "this task never ran".
  if (isError) return <p className="text-[12px] text-[var(--color-danger)]">{t['logs.loadFailed']}</p>

  if (list.length === 0) {
    return <p className="text-[12px] text-[var(--color-tertiary)]">{t['logs.noRuns']}</p>
  }

  return (
    <div className="flex flex-col gap-3">
      {list.map((run) => (
        <section key={run.id} className="flex flex-col">
          {/*
            The header is the way into the full run screen (US-AD41). A run
            screen with no door is a route nobody can reach, and this row is
            already the thing an operator clicks when one attempt of three looks
            wrong.
          */}
          <header className="flex items-baseline gap-2 border-b border-[var(--color-border-subtle)] pb-1">
            <Link
              to={orgID ? `/app/${orgID}/runs/${run.id}` : `/runs/${run.id}`}
              aria-label={`${t['logs.attempt']} ${run.attempt} ${run.id}`}
              className="flex items-baseline gap-2 rounded-[4px] transition-opacity hover:opacity-80"
            >
              <span className="font-mono text-[11px] font-semibold text-[var(--color-primary)]">
                {t['logs.attempt']} {run.attempt}
              </span>
              <span className="font-mono text-[10px] text-[var(--color-tertiary)]">{run.status}</span>
            </Link>
            <span className="ml-auto font-mono text-[10px] tabular-nums text-[var(--color-tertiary)]">
              {formatMicroUSD(run.cost_micros)} · {formatTokens(run.tokens_in + run.tokens_out)}
            </span>
          </header>
          {run.failure_kind ? (
            <p className="pt-1 font-mono text-[11px] text-[var(--color-danger)]">{run.failure_kind}</p>
          ) : null}
          <StepList runID={run.id} lang={lang} />
        </section>
      ))}
    </div>
  )
}

function StepList({ runID, lang }: { runID: string; lang: Parameters<typeof formatDateTime>[1] }) {
  const t = useT()
  const { data: steps, isLoading, isError } = useListRunStepsQuery(runID)
  const list = steps ?? []

  if (isLoading) return <SkeletonText lines={2} />
  if (isError) return <p className="pt-1 text-[11px] text-[var(--color-danger)]">{t['logs.stepsFailed']}</p>
  // A run with no steps yet is normal at the start of an attempt, not an error.
  if (list.length === 0) return <p className="pt-1 text-[11px] text-[var(--color-tertiary)]">{t['logs.noSteps']}</p>

  return (
    <ol className="flex flex-col">
      {list.map((step) => (
        <StepRow key={step.seq} step={step} lang={lang} />
      ))}
    </ol>
  )
}

function StepRow({ step, lang }: { step: Step; lang: Parameters<typeof formatDateTime>[1] }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const { icon: Icon, className } = STATUS_STYLE[step.status] ?? STATUS_STYLE.running

  // AC94 AC3: a step that has not finished must not report 0ms or $0.00, which
  // would read as "this step was free and instant" instead of "still going".
  const ended = step.ended_at !== ''
  const duration = ended
    ? formatDuration(new Date(step.ended_at).getTime() - new Date(step.started_at).getTime())
    : null

  return (
    <li className="border-b border-[var(--color-border-subtle)] last:border-b-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 py-1.5 text-left"
      >
        {open ? (
          <ChevronDown size={12} aria-hidden="true" className="shrink-0 text-[var(--color-tertiary)]" />
        ) : (
          <ChevronRight size={12} aria-hidden="true" className="shrink-0 text-[var(--color-tertiary)]" />
        )}
        {/*
          `data-status` carries the same fact the colour does. It exists so the
          status colour can be asserted without addressing a Tailwind class:
          `svg.text-[var(--color-success)]` is not a valid CSS selector, and
          writing the assertion against a resolved colour would pin a hex value
          that DESIGN.md owns, not this component.
        */}
        <Icon size={12} aria-hidden="true" data-status={step.status} className={cn('shrink-0', className)} />
        <span className="font-mono text-[10px] text-[var(--color-quaternary)] tabular-nums">
          {String(step.seq).padStart(3, '0')}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--color-primary)]">
          {step.name || step.kind}
        </span>
        {/* Tabular numerals: these three columns are read as a table, and
            proportional digits make two rows of the same width look ragged. */}
        <span className="shrink-0 font-mono text-[10px] tabular-nums text-[var(--color-tertiary)]">
          {duration ?? t['logs.running']}
        </span>
        <span className="w-[72px] shrink-0 text-right font-mono text-[10px] tabular-nums text-[var(--color-secondary)]">
          {formatTokens(step.tokens_in + step.tokens_out)}
        </span>
        <span className="w-[64px] shrink-0 text-right font-mono text-[10px] tabular-nums text-[var(--color-secondary)]">
          {formatMicroUSD(step.cost_micros)}
        </span>
      </button>

      {open ? (
        <div className="flex flex-col gap-2 pb-2 pl-6">
          <dl className="grid grid-cols-2 gap-x-3 gap-y-1 font-mono text-[10px] text-[var(--color-tertiary)]">
            <div className="flex justify-between gap-2">
              <dt>{t['logs.kind']}</dt>
              <dd className="text-[var(--color-secondary)]">{step.kind}</dd>
            </div>
            <div className="flex justify-between gap-2">
              <dt>{t['logs.started']}</dt>
              <dd className="text-[var(--color-secondary)]">{formatDateTime(step.started_at, lang)}</dd>
            </div>
            <div className="flex justify-between gap-2">
              <dt>{t['logs.tokensIn']}</dt>
              <dd className="tabular-nums text-[var(--color-secondary)]">{step.tokens_in}</dd>
            </div>
            <div className="flex justify-between gap-2">
              <dt>{t['logs.tokensOut']}</dt>
              <dd className="tabular-nums text-[var(--color-secondary)]">{step.tokens_out}</dd>
            </div>
          </dl>
          <Payload payload={step.payload_json} />
        </div>
      ) : null}
    </li>
  )
}

/**
 * AC26 AC2/AC3. A missing or unparseable payload is a labelled empty panel, not
 * an error: nothing is broken when a step stored no payload.
 *
 * The JSON is pretty-printed, and the block scrolls rather than growing without
 * bound — AC94 AC2 asks for truncation, and a 200-line completion on the drawer
 * would push everything else off screen.
 */
function Payload({ payload }: { payload: string | null }) {
  const t = useT()

  if (payload == null || payload === '') {
    return <p className="font-mono text-[10px] text-[var(--color-quaternary)]">{t['logs.noPayload']}</p>
  }

  let pretty = payload
  try {
    pretty = JSON.stringify(JSON.parse(payload), null, 2)
  } catch {
    // Stored payloads are text; the only requirement is that they were valid
    // JSON when written. If it is not parseable now, showing it raw is still
    // more useful than an error for something nobody can fix from this screen.
  }

  return (
    <pre className="max-h-[220px] overflow-auto rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-sunken)] p-2 font-mono text-[10px] whitespace-pre-wrap text-[var(--color-secondary)]">
      {pretty}
    </pre>
  )
}
