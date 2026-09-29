import { CircleAlert, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { COLUMN_LABELS, COLUMN_ORDER } from '@/lib/domain'
import { useT } from '@/hooks/use-t'

/**
 * US-AD63 / US-AD64 — the three states a board can be in that are not "here are
 * your tasks": loading, empty, and failed.
 *
 * They live together because they are the same decision seen three ways, and the
 * one thing that matters is that they are never confused with each other. A board
 * that failed to load must NOT render the empty state (US-AD64 AC3): "Belum ada
 * task" over a 500 is a lie, and it is the lie that makes an operator create a
 * duplicate of work that already exists.
 *
 * The skeleton is the board's own shape — five lanes at the real column width,
 * three cards each (US-AD63 AC1) — so the placeholder occupies the space the
 * board will. A generic row skeleton here would shift the whole layout when the
 * data lands, which is the one thing a placeholder exists to prevent.
 *
 * Only the first load gets a skeleton (US-AD63 AC2). That is the caller's
 * decision, not this component's: it is driven by RTK Query's `isLoading`, which
 * is true only while the first request is in flight, and false during a
 * background refetch (`isFetching`). The board reads `isLoading` for exactly this
 * reason.
 */

const SKELETON_CARDS = 3

export function BoardSkeleton() {
  return (
    <div data-testid="board-skeleton" className="flex min-h-0 flex-1 gap-2.5 overflow-x-auto p-4">
      {COLUMN_ORDER.map((columnKey) => (
        <section
          key={columnKey}
          className="flex w-[268px] min-w-[268px] flex-col rounded-[10px] bg-[var(--color-surface-sunken)]/60 p-2"
        >
          <header className="mb-2 flex items-center justify-between px-1">
            <h2 className="text-[11px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
              {COLUMN_LABELS[columnKey]}
            </h2>
            {/* No count: a number here would be invented, and 0 is a claim. */}
          </header>
          <div className="flex min-h-[80px] flex-col gap-2 p-1">
            {Array.from({ length: SKELETON_CARDS }, (_, index) => (
              <div
                key={index}
                data-testid="skeleton-card"
                className="rounded-[8px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-3"
              >
                <Skeleton pulse className="mb-2 w-full" />
                <Skeleton pulse className="w-2/3" />
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

/**
 * US-AD63 AC3 / US-AD64 AC3 — the failed load. Distinct from the empty state on
 * purpose, and the retry is the real refetch rather than a reload.
 */
export function BoardError({ onRetry, retrying }: { onRetry: () => void; retrying?: boolean }) {
  const t = useT()
  return (
    <div
      data-testid="board-error"
      role="alert"
      className="flex min-h-[240px] flex-1 flex-col items-center justify-center gap-3 p-8 text-center"
    >
      <CircleAlert size={22} className="text-[var(--color-danger)]" />
      <p className="text-[13px] font-medium text-[var(--color-primary)]">{t['state.error']}</p>
      <Button variant="secondary" onClick={onRetry} disabled={retrying} data-testid="board-retry">
        {t['action.retry']}
      </Button>
    </div>
  )
}

/**
 * US-AD64 — the empty board, and the different message when a filter is what
 * emptied it.
 *
 * The two are NOT the same screen (AC1): "create your first task" over a board
 * that has forty tasks and an active search is wrong advice, so the filtered case
 * says so and offers no CTA. The CTA creates into `backlog`, which is where a new
 * task starts (US-AD11 AC1) and the only status the API accepts at creation.
 */
export function BoardEmpty({ filtered, onCreate }: { filtered: boolean; onCreate: () => void }) {
  const t = useT()
  if (filtered) {
    return (
      <div
        data-testid="board-empty-filtered"
        className="flex min-h-[240px] flex-1 flex-col items-center justify-center gap-2 p-8 text-center"
      >
        <p className="text-[13px] font-medium text-[var(--color-primary)]">{t['boards.noTasksMatch']}</p>
        <p className="text-[12px] text-[var(--color-tertiary)]">{t['boards.noTasksMatchHint']}</p>
      </div>
    )
  }
  return (
    <div
      data-testid="board-empty"
      className="flex min-h-[240px] flex-1 flex-col items-center justify-center gap-3 p-8 text-center"
    >
      <p className="text-[13px] font-medium text-[var(--color-primary)]">{t['boards.emptyTasks']}</p>
      <p className="text-[12px] text-[var(--color-tertiary)]">{t['boards.emptyTasksHint']}</p>
      <Button onClick={onCreate} data-testid="board-create-first">
        <Plus size={14} />
        {t['boards.createFirstTask']}
      </Button>
    </div>
  )
}
