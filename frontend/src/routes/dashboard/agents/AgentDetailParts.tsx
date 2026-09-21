import type { ReactNode } from 'react'
import { Archive, ArchiveRestore, Check, CheckCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Panel } from '@/components/ui/card'
import { useT } from '@/hooks/use-t'
import { formatRelative } from '@/lib/formatters'
import type { Agent } from '@/lib/domain'

/**
 * The presentational half of screen 28-agent-detail: the topbar controls, the
 * summary banner, the lifecycle/protection card, and the estimated-rate card.
 *
 * Everything is cloned class-for-class from
 * `design/stitch-output/v2/28-agent-detail.html`, with the mock's raw hex
 * replaced by the design tokens (`#ffffff` -> `--color-surface-panel`,
 * `#f6f7f6` -> `--color-surface-page`, `#0d7a70` -> `--color-accent`,
 * `#16a34a` -> `--color-status-done`, `#d97706` -> `--color-warning`,
 * `#dc2626` -> `--color-danger`), and the mock's meta vocabulary (story ids,
 * acceptance-criterion numbers, HTTP status codes, `POST /agents/validate`)
 * dropped: an operator reads what the agent does, not which story shipped the
 * screen.
 *
 * The mock's sample figures are replaced by fields the contract actually
 * returns. Where the mock showed data no endpoint can answer — a per-agent run
 * counter and a spend total — the banner counts the two lists the agent row
 * really carries (tools, skills) instead of printing a confident zero.
 */

const CARD =
  'rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-[14px] shadow-xs'

export type AgentState = 'ready' | 'needsKey' | 'archived'

/** Archived wins over credential state: a retired agent is not "ready". */
export function agentState(agent: Agent): AgentState {
  if (agent.archived_at) return 'archived'
  return agent.has_provider_key ? 'ready' : 'needsKey'
}

const STATE_TONE: Record<AgentState, string> = {
  ready: 'text-[var(--color-status-done)]',
  needsKey: 'text-[var(--color-warning)]',
  archived: 'text-[var(--color-tertiary)]',
}

const STATE_DOT: Record<AgentState, string> = {
  ready: 'bg-[var(--color-status-done)]',
  needsKey: 'bg-[var(--color-warning)]',
  archived: 'bg-[var(--color-status-archived)]',
}

const STATE_TINT: Record<AgentState, string> = {
  ready: 'bg-[var(--color-status-done)]/10 border-[var(--color-status-done)]/20 text-[var(--color-status-done)]',
  needsKey: 'bg-[var(--color-warning)]/10 border-[var(--color-warning)]/20 text-[var(--color-warning)]',
  archived: 'bg-[var(--color-surface-sunken)] border-[var(--color-border-subtle)] text-[var(--color-secondary)]',
}

/**
 * STATUS pill in the topbar (design: `STATUS: ACTIVE (ASSIGN READY)`).
 *
 * The archived state short-circuits the credential state: a retired agent is
 * still not "ready to assign", and saying otherwise would contradict the button
 * sitting next to it.
 */
export function StatusPill({ agent }: { agent: Agent }) {
  const t = useT()
  const state = agentState(agent)
  const label =
    state === 'archived'
      ? t['agents.detail.statusArchived']
      : state === 'needsKey'
        ? t['agents.detail.statusNeedsKey']
        : t['agents.detail.statusActive']

  return (
    <div
      data-testid="agent-status-pill"
      className={`flex items-center gap-1.5 rounded-[6px] border px-2.5 py-1 font-mono text-[11px] font-medium ${STATE_TINT[state]}`}
    >
      <span className={`h-2 w-2 rounded-full ${STATE_DOT[state]} ${state === 'ready' ? 'animate-pulse' : ''}`} />
      <span>{label}</span>
    </div>
  )
}

/**
 * The archive / unarchive control, the one control that changes the archive
 * state. A refusal is announced by the lifecycle card's `role="alert"`; the
 * `aria-describedby` on this button is the pointer to it, which is also what
 * tells a test which failure the control belongs to.
 */
export function ArchiveButton({
  archived,
  allowed,
  isArchiving,
  describedBy,
  onClick,
}: {
  archived: boolean
  allowed: boolean
  isArchiving: boolean
  describedBy: string
  onClick: () => void
}) {
  const t = useT()
  const label = archived ? t['agents.detail.unarchiveAction'] : t['agents.detail.archiveAction']

  return (
    <Button
      variant="secondary"
      size="sm"
      onClick={onClick}
      disabled={!allowed || isArchiving}
      aria-describedby={describedBy}
      title={allowed ? label : t['agents.archive.forbidden']}
      className="h-[30px] gap-1.5 px-3 text-[12px] hover:border-[var(--color-danger)]/40 hover:bg-[var(--color-danger)]/5 hover:text-[var(--color-danger)]"
    >
      {archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}
      <span>{label}</span>
    </Button>
  )
}

/**
 * The save trigger, cloned from the design's filled topbar button. It submits the
 * one form on this screen by id, so the control can live in the topbar while the
 * fields live in the content pane.
 */
export function SaveButton({ formID, isPending }: { formID: string; isPending: boolean }) {
  const t = useT()
  return (
    <Button
      type="submit"
      form={formID}
      variant="primary"
      size="sm"
      disabled={isPending}
      className="h-[30px] gap-1.5 px-3 text-[12px] shadow-xs"
    >
      <Check size={14} />
      <span>{isPending ? t['agents.detail.saving'] : t['agents.detail.save']}</span>
    </Button>
  )
}

/** HEADER BANNER: monogram, name, status badge, and the two real counters. */
export function AgentHero({ agent }: { agent: Agent }) {
  const t = useT()
  const state = agentState(agent)
  const badge =
    state === 'archived'
      ? t['agents.status.archived']
      : state === 'needsKey'
        ? t['agents.status.needsKey']
        : t['agents.status.ready']

  return (
    <Panel className="flex items-start justify-between p-[14px] shadow-xs">
      <div className="flex min-w-0 items-start gap-3.5">
        <div className="flex h-[42px] w-[42px] shrink-0 items-center justify-center rounded-[10px] border border-[var(--color-accent)]/20 bg-[var(--color-accent)]/10 font-mono text-[16px] font-bold text-[var(--color-accent)]">
          {agent.name.slice(0, 2).toLowerCase()}
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h1 className="truncate text-[17px] font-bold tracking-tight text-[var(--color-primary)]">{agent.name}</h1>
            <span
              className={`shrink-0 rounded-[4px] px-2 py-0.5 font-mono text-[10px] font-semibold tracking-wide ${
                state === 'archived'
                  ? 'bg-[var(--color-surface-sunken)] text-[var(--color-secondary)]'
                  : state === 'needsKey'
                    ? 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'
                    : 'bg-[var(--color-status-done)]/10 text-[var(--color-status-done)]'
              }`}
            >
              {badge}
            </span>
          </div>
          <p className="mt-0.5 truncate font-mono text-[12px] text-[var(--color-secondary)]">
            {agent.provider} · {agent.model}
          </p>
          {agent.base_url ? (
            <p className="mt-0.5 truncate font-mono text-[10.5px] text-[var(--color-tertiary)]">{agent.base_url}</p>
          ) : null}
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-4 border-l border-[var(--color-border-subtle)] pl-4 text-right">
        <Counter label={t['agents.detail.tools']} value={String((agent.tools ?? []).length)} />
        <Counter label={t['agents.detail.skills']} value={String((agent.skills ?? []).length)} accent />
      </div>
    </Panel>
  )
}

function Counter({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div>
      <div className="font-mono text-[10px] uppercase text-[var(--color-tertiary)]">{label}</div>
      <div
        className={`mt-0.5 font-mono text-[15px] font-bold tabular-nums ${
          accent ? 'text-[var(--color-accent)]' : 'text-[var(--color-primary)]'
        }`}
      >
        {value}
      </div>
    </div>
  )
}

/**
 * The lifecycle and protection card: what the current assignment state is, the
 * two guarantees that make archiving safe, and — when the toggle above is
 * refused — the server's own reason, in an element a screen reader announces.
 *
 * The mock labels these rules with criterion numbers; the rules themselves are
 * the product, so the titles state the guarantee and the bodies explain it.
 */
export function LifecycleCard({
  agent,
  canArchive,
  error,
  errorID,
}: {
  agent: Agent
  canArchive: boolean
  error: string | null
  errorID: string
}) {
  const t = useT()
  const state = agentState(agent)
  const archived = state === 'archived'
  const stateLabel = archived
    ? t['agents.detail.statusArchived']
    : state === 'needsKey'
      ? t['agents.detail.statusNeedsKey']
      : t['agents.detail.statusActive']

  return (
    <Panel className={`${CARD} flex flex-col justify-between`}>
      <div>
        <div className="mb-3 flex items-center justify-between border-b border-[var(--color-border-subtle)] pb-2">
          <div className="flex items-center gap-2">
            <span className={`h-2 w-2 rounded-full ${STATE_DOT[state]}`} />
            <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['agents.detail.lifecycleTitle']}</h2>
          </div>
          <span className="rounded-[4px] bg-[var(--color-warning)]/15 px-2 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-warning)]">
            {t['agents.archive.badge']}
          </span>
        </div>

        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5">
            <div>
              <div className="font-mono text-[10px] uppercase text-[var(--color-tertiary)]">
                {t['agents.detail.assignmentState']}
              </div>
              <div className={`mt-0.5 flex items-center gap-1.5 text-[13px] font-bold ${STATE_TONE[state]}`}>
                <span className={`h-2 w-2 rounded-full ${STATE_DOT[state]}`} />
                <span>{stateLabel}</span>
              </div>
            </div>
            <span className="shrink-0 rounded border border-[var(--color-border-standard)] bg-[var(--color-surface-panel)] px-2 py-1 font-mono text-[10px] text-[var(--color-secondary)]">
              {t['agents.detail.created']} {formatRelative(agent.created_at)}
            </span>
          </div>

          {error ? (
            <p
              id={errorID}
              role="alert"
              className="rounded-[6px] border border-[var(--color-danger)]/30 bg-[var(--color-surface-page)] p-2.5 text-[11px] leading-snug text-[var(--color-danger)]"
            >
              {error}
            </p>
          ) : null}

          <div className="flex flex-col gap-2">
            <RuleNote title={t['agents.detail.rule1Title']} body={t['agents.detail.rule1Body']} />
            <RuleNote title={t['agents.detail.rule2Title']} body={t['agents.detail.rule2Body']} />
          </div>

          {canArchive ? null : (
            <p className="text-[11px] text-[var(--color-tertiary)]">{t['agents.archive.forbidden']}</p>
          )}
        </div>
      </div>

      <div className="mt-3 flex items-start justify-between gap-3 border-t border-[var(--color-border-subtle)] pt-2 text-[10.5px] leading-snug text-[var(--color-tertiary)]">
        <span className="font-mono">
          {archived ? t['agents.detail.unarchiveHint'] : t['agents.detail.archiveHint']}
        </span>
        <span className="shrink-0 font-mono">{t['agents.detail.archiveAuthority']}</span>
      </div>
    </Panel>
  )
}

/** One rule: a tick disc, a bold line, and a paragraph, as the design draws it. */
function RuleNote({ title, body }: { title: string; body: string }) {
  return (
    <div className="flex items-start gap-2.5 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-2.5">
      <div className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[var(--color-accent)]/10 text-[10px] font-bold text-[var(--color-accent)]">
        <CheckCheck size={10} strokeWidth={3} />
      </div>
      <div>
        <div className="text-[11.5px] font-semibold text-[var(--color-primary)]">{title}</div>
        <p className="mt-0.5 text-[11px] leading-relaxed text-[var(--color-secondary)]">{body}</p>
      </div>
    </div>
  )
}

/**
 * One labelled field in the metric-box shape the design uses for the runtime
 * parameters. The label is bound to the control by id, so `getByLabel` and a
 * screen reader both find the field.
 */
export function ControlBox({
  label,
  hint,
  htmlFor,
  children,
}: {
  label: string
  hint?: ReactNode
  htmlFor: string
  children: ReactNode
}) {
  return (
    <div className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5">
      <div className="flex items-center justify-between gap-2">
        <label htmlFor={htmlFor} className="font-mono text-[10px] uppercase text-[var(--color-tertiary)]">
          {label}
        </label>
        {hint}
      </div>
      <div className="mt-1.5">{children}</div>
    </div>
  )
}

/** `1,800s` — the design prints the unit beside the value, never a bare number. */
export function runtimeLabel(seconds: number): string {
  return `${seconds.toLocaleString()}s`
}
