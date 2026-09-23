import { Check } from 'lucide-react'
import { useT } from '@/hooks/use-t'
import { interpolate } from '@/lib/format'
import { formatEstimatedMicroUSD, plural, shortID } from '@/lib/formatters'
import { statusColorVar, type TaskStatus } from '@/lib/domain'
import type { AgentAssignment } from '@/store/api/agents'

/**
 * The assignment section of screen 28-agent-detail, which the screen was missing
 * entirely: the mock's third card had no implementation, so the two rules it
 * exists to demonstrate — a running task survives archiving (US-AD73 AC1) and the
 * assign picker offers no archived agent (AC2) — were asserted in prose on the
 * lifecycle card and never shown against real rows.
 *
 * Both halves come from one call (`GET /agents/{id}/tasks`): a count of hidden
 * agents rendered beside a picker fetched at another moment would be exactly the
 * unverifiable claim this section was built to stop making.
 *
 * What the mock draws is kept; what it says to a reviewer is not. Its copy is a
 * spec explainer ("AC2 Verification", "Enforce Rule: AC1 + AC2") and that
 * vocabulary may not reach a rendered screen, so the sentences here are the
 * product's own wording. Its static `<select>` and hardcoded rows become the real
 * list, and its "1 agent diarsip disembunyikan" line becomes the count the server
 * reported. Its status colours (`#d97706`, `#16a34a`) come from the frozen palette
 * in DESIGN.md instead, where running is `warning` — the mock had drifted from the
 * token suite, and the suite is the contract.
 */
export function AssignmentSection({
  agentID,
  data,
  loading,
}: {
  agentID: string
  data: AgentAssignment | undefined
  loading: boolean
}) {
  const t = useT()
  const tasks = data?.tasks ?? []
  const picker = data?.picker ?? []
  const running = data?.running_count ?? 0

  return (
    <section
      data-testid="agent-assignment"
      className="rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-[14px] shadow-xs"
    >
      <div className="mb-3 flex items-start justify-between gap-3 border-b border-[var(--color-border-subtle)] pb-2">
        <div>
          <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['agents.detail.assignmentTitle']}</h2>
          <div className="mt-0.5 text-[11px] text-[var(--color-tertiary)]">
            {data?.board_id
              ? interpolate(t['agents.detail.assignmentBoard'], [shortID(data.board_id)])
              : t['agents.detail.assignmentLead']}
          </div>
        </div>
      </div>

      <div className="grid grid-cols-12 items-start gap-4">
        <div className="col-span-5 rounded-[8px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-3">
          <div className="mb-2 text-[10.5px] leading-tight text-[var(--color-tertiary)]">
            {t['agents.detail.pickerHint']}
          </div>

          {loading && !data ? (
            <p className="font-mono text-[11px] text-[var(--color-tertiary)]">{t['agents.detail.loading']}</p>
          ) : picker.length === 0 ? (
            <p className="text-[11px] leading-snug text-[var(--color-tertiary)]">{t['agents.detail.pickerEmpty']}</p>
          ) : (
            <div className="rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-panel)] p-1.5">
              <div className="mb-1 flex items-center justify-between border-b border-[var(--color-border-subtle)] px-1 pb-1 font-mono text-[11px] text-[var(--color-tertiary)]">
                <span>{t['agents.detail.pickerFilter']}</span>
                <span className="text-[9.5px] font-semibold text-[var(--color-accent)]">
                  {t['agents.detail.pickerFilterActive']}
                </span>
              </div>
              <ul data-testid="assignment-picker" className="flex flex-col">
                {picker.map((option) => {
                  // The agent whose page this is: the mock marks its own row as
                  // chosen, which is how a reader sees this picker is the board's.
                  const chosen = option.id === agentID
                  return (
                    <li
                      key={option.id}
                      className={`my-0.5 flex items-center justify-between rounded px-2 py-1 text-[11.5px] font-medium ${
                        chosen
                          ? 'bg-[var(--color-accent)]/10 text-[var(--color-accent)]'
                          : 'text-[var(--color-secondary)]'
                      }`}
                    >
                      <span className="flex min-w-0 items-center gap-1.5">
                        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-success)]" />
                        <span className="truncate font-mono">{option.name}</span>
                      </span>
                      <span
                        className={`shrink-0 rounded px-1.5 font-mono text-[10px] ${
                          chosen
                            ? 'bg-[var(--color-surface-panel)] text-[var(--color-accent)]'
                            : 'text-[var(--color-tertiary)]'
                        }`}
                      >
                        {option.has_provider_key ? t['agents.detail.pickerReady'] : t['agents.detail.pickerNeedsKey']}
                      </span>
                    </li>
                  )
                })}
              </ul>
            </div>
          )}

          {/* AC2's other half: what the picker left out, stated rather than
              dropped. A picker that silently omits rows is how the rule would
              regress unseen. */}
          {data && data.hidden_agents > 0 ? (
            <div
              data-testid="assignment-hidden"
              className="mt-2 flex items-center gap-1.5 border-t border-dashed border-[var(--color-border-standard)] px-1 pt-1.5 text-[10px] text-[var(--color-tertiary)]"
            >
              <span aria-hidden className="font-mono text-[9px] text-[var(--color-danger)]">
                ✕
              </span>
              <span>
                {interpolate(t['agents.detail.pickerHidden'], [plural(data.hidden_agents, t['agents.detail.agent'])])}
              </span>
            </div>
          ) : null}
        </div>

        <div className="col-span-7">
          <div className="mb-1 flex items-center justify-between text-[11px] font-semibold text-[var(--color-primary)]">
            <span>{t['agents.detail.assignmentTasksTitle']}</span>
            {running > 0 ? (
              <span
                data-testid="assignment-running"
                className="rounded bg-[var(--color-warning)]/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-[var(--color-warning)]"
              >
                {interpolate(t['agents.detail.assignmentRunning'], [String(running)])}
              </span>
            ) : null}
          </div>
          <div className="mb-2 text-[10.5px] leading-tight text-[var(--color-tertiary)]">
            {t['agents.detail.assignmentTasksHint']}
          </div>

          {tasks.length === 0 ? (
            <p className="text-[11px] leading-snug text-[var(--color-tertiary)]" data-testid="assignment-empty">
              {t['agents.detail.assignmentEmpty']}
            </p>
          ) : (
            <div className="overflow-hidden rounded-[6px] border border-[var(--color-border-subtle)]">
              <table className="w-full table-fixed border-collapse text-left">
                <colgroup>
                  <col className="w-[88px]" />
                  <col />
                  <col className="w-[132px]" />
                  <col className="w-[84px]" />
                </colgroup>
                <thead>
                  <tr className="h-[32px] border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] font-mono text-[10.5px] uppercase text-[var(--color-tertiary)]">
                    <th className="px-2.5 font-medium">{t['agents.detail.taskColId']}</th>
                    <th className="px-2.5 font-medium">{t['agents.detail.taskColTitle']}</th>
                    <th className="px-2.5 font-medium">{t['agents.detail.taskColStatus']}</th>
                    <th className="px-2.5 text-right font-medium">{t['agents.detail.taskColCost']}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--color-border-subtle)] text-[12px]">
                  {tasks.map((task) => (
                    <tr key={task.id} className="h-[32px]" data-testid="assignment-row">
                      <td className="truncate px-2.5 font-mono text-[10.5px] text-[var(--color-tertiary)]">
                        {shortID(task.id)}
                      </td>
                      <td className="truncate px-2.5 text-[var(--color-primary)]">{task.title}</td>
                      <td className="px-2.5">
                        <TaskStatusChip status={task.status} />
                      </td>
                      <td className="px-2.5 text-right font-mono text-[10.5px] tabular-nums text-[var(--color-secondary)]">
                        {formatEstimatedMicroUSD(task.cost_micros)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      <div className="mt-3 flex items-start gap-1.5 border-t border-[var(--color-border-subtle)] pt-2 text-[10.5px] leading-snug text-[var(--color-tertiary)]">
        <Check size={12} className="mt-0.5 shrink-0 text-[var(--color-accent)]" aria-hidden />
        <span>{t['agents.detail.assignmentFooter']}</span>
      </div>
    </section>
  )
}

/**
 * One row's status. It reuses `statusColorVar`, the same function the board's
 * table view paints its status chips with, so a status can never read one colour
 * here and another on the board. The label is the raw enum, exactly as the board
 * renders it.
 */
function TaskStatusChip({ status }: { status: string }) {
  const color = statusColorVar(status as TaskStatus)
  return (
    <span className="inline-flex items-center gap-1.5 font-mono text-[10px] font-medium" style={{ color }}>
      <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: color }} />
      {status}
    </span>
  )
}
