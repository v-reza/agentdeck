import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/cn'
import { TaskCard } from './TaskCard'
import { COLUMN_LABELS, COLUMN_ORDER, columnForStatus } from '@/lib/domain'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { setMobileColumn } from '@/store/slices/uiSlice'
import { useT } from '@/hooks/use-t'
import type { Task } from '@/lib/domain'

/**
 * Screen 46-mobile-board — the board below 768px (US-AD60 AC1).
 *
 * The desktop board is five 268px lanes side by side; that is 1340px of content,
 * which on a phone is a horizontal scroll with four fifths of the board off
 * screen. Here the same five columns become a vertical accordion and exactly one
 * is open, so the board is usable without scrolling sideways.
 *
 * TWO DECISIONS WORTH STATING:
 *
 * 1. WHICH COLUMN IS OPEN lives in the redux store, not in local state and not in
 *    the URL. AC2 asks the selection to survive an orientation change, and a
 *    rotation is a resize, not a navigation — React keeps the component mounted
 *    through it, so the value is simply still there. The design note suggests the
 *    URL hash; that would also survive, but it would put a transient view
 *    preference into the history stack (a back button that closes an accordion)
 *    and it contradicts the repo's rule that Redux Toolkit is the only state
 *    management. It also survives a trip to a desktop width and back, which is
 *    what AC3's "desktop keeps the columns" really tests.
 *
 * 2. NO COST PER COLUMN. The design prints "$0.000" per lane and "$0.420" on the
 *    open one. The API has no per-column cost: `cost-summary` aggregates by MODEL
 *    and by BOARD. A number here would be invented, and an invented cost on a
 *    budget screen is worse than no cost, so the lane header carries the task
 *    count only.
 *
 * Drag-and-drop is deliberately absent: `dnd-kit`'s pointer sensor needs a
 * pointer, and a board that looks draggable but is not is worse than one that
 * offers the status change through the task drawer instead.
 *
 * Columns are derived with `columnForStatus`, the same mapping the desktop lanes
 * use — NOT `task.status === key`. `blocked`, `failed`, and `archived` have no
 * lane of their own and are shown in the lane of the status they map to; matching
 * on equality would drop exactly the cards an operator most needs to see, which is
 * the bug the dependency graph hit in US-AD19.
 */
export function MobileBoard({ tasks, onOpenTask }: { tasks: Task[]; onOpenTask: (id: string) => void }) {
  const t = useT()
  const dispatch = useAppDispatch()
  const open = useAppSelector((state) => state.ui.mobileColumn)

  // Default to the first column with work in it, else the first column. An
  // accordion with everything closed shows no tasks at all, which reads as an
  // empty board.
  const initial =
    COLUMN_ORDER.find((key) => tasks.some((task) => columnForStatus(task.status) === key)) ?? COLUMN_ORDER[0]
  const openKey = open && COLUMN_ORDER.includes(open) ? open : initial

  return (
    <div data-testid="mobile-board" className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-3">
      {COLUMN_ORDER.map((key) => {
        const rows = tasks.filter((task) => columnForStatus(task.status) === key)
        const expanded = key === openKey
        return (
          <section
            key={key}
            data-testid={`mobile-column-${key}`}
            data-expanded={expanded ? 'true' : 'false'}
            className="rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)]"
          >
            <h2>
              <button
                type="button"
                // A collapsed section with `aria-expanded` is the accessible shape
                // of an accordion: the button is the control, and the region below
                // is its panel.
                aria-expanded={expanded}
                aria-controls={`mobile-panel-${key}`}
                onClick={() => dispatch(setMobileColumn(key))}
                data-testid={`mobile-toggle-${key}`}
                className="flex w-full items-center justify-between gap-2 px-3 py-2.5 text-left"
              >
                <span className="flex items-center gap-2">
                  <span className="text-[12px] font-bold uppercase tracking-[0.06em] text-[var(--color-secondary)]">
                    {COLUMN_LABELS[key]}
                  </span>
                  <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
                    {t['mobile.taskCount'].replace('{count}', String(rows.length))}
                  </span>
                </span>
                <ChevronDown
                  size={16}
                  aria-hidden="true"
                  className={cn(
                    'shrink-0 text-[var(--color-tertiary)] transition-transform motion-reduce:transition-none',
                    expanded && 'rotate-180',
                  )}
                />
              </button>
            </h2>

            <div id={`mobile-panel-${key}`} hidden={!expanded} className="flex flex-col gap-2 px-2 pb-2">
              {rows.length === 0 ? (
                <p className="px-1 py-2 text-[11px] text-[var(--color-tertiary)]">{t['mobile.columnEmpty']}</p>
              ) : (
                rows.map((task) => <TaskCard key={task.id} task={task} onOpen={onOpenTask} />)
              )}
            </div>
          </section>
        )
      })}
    </div>
  )
}
