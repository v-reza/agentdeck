import { Link, useLocation, useParams } from 'react-router-dom'
import { Filter, Plus, Search, SquareKanban, Table2 } from 'lucide-react'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { openCreateTask, setSearch, toggleStatusFilter } from '@/store/slices/uiSlice'
import { TASK_STATUSES, statusColorVar } from '@/lib/domain'
import { interpolate } from '@/lib/format'
import { cn } from '@/lib/cn'
import { useT } from '@/hooks/use-t'

/**
 * Screens 18-kanban / 19-table-view — the board toolbar.
 *
 * One component for both views. The design draws this bar once, and the two
 * views differ only in which entry is current: duplicating it would have been
 * two places to keep in step, and the search box was already drifting — the
 * table view had one and the kanban had none, so the same board was searchable
 * or not depending on which URL you were on.
 *
 * The view switcher is links, not tabs, because each view is a real route
 * (`boards/:boardID` and `boards/:boardID/table`). That is what makes the two
 * reachable by URL.
 *
 * The controls write filter state to `uiSlice`; the views pass it to
 * `useListTasksQuery` as query parameters, so the filtering itself happens in
 * SQL. The chips therefore describe what was asked for rather than what happens
 * to be in the cache.
 */
export function BoardToolbar({ boardName, taskCount }: { boardName?: string; taskCount: number }) {
  const { boardID, orgID } = useParams<{ boardID: string; orgID?: string }>()
  const location = useLocation()
  // The dashboard routes are mounted under both `/app` and `/app/:orgID`, so the
  // prefix is carried through rather than hardcoded: switching views on an
  // org-scoped URL must not silently drop the org segment.
  const base = orgID ? `/app/${orgID}/boards/${boardID}` : `/app/boards/${boardID}`
  const dispatch = useAppDispatch()
  const statusFilter = useAppSelector((state) => state.ui.statusFilter)
  const search = useAppSelector((state) => state.ui.search)
  const t = useT()

  const view = location.pathname.endsWith('/table') ? 'table' : 'board'

  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border-subtle)] px-4 py-2">
      <span className="text-[13px] font-semibold text-[var(--color-primary)]">{boardName ?? 'Board'}</span>
      <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
        {interpolate(t['boards.taskCount'], [String(taskCount)])}
      </span>

      <div className="flex items-center gap-0.5 rounded-[6px] border border-[var(--color-border-subtle)] p-0.5">
        <ViewLink to={base} active={view === 'board'} icon={SquareKanban} label={t['boards.viewBoard']} />
        <ViewLink to={`${base}/table`} active={view === 'table'} icon={Table2} label={t['boards.viewTable']} />
      </div>

      <div className="relative ml-auto">
        <Search
          size={13}
          className="pointer-events-none absolute top-1/2 left-2 -translate-y-1/2 text-[var(--color-quaternary)]"
          aria-hidden="true"
        />
        <input
          value={search}
          onChange={(event) => dispatch(setSearch(event.target.value))}
          placeholder={t['boards.search']}
          aria-label={t['boards.search']}
          className="h-8 w-[190px] rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] pr-2.5 pl-7 text-[12px] text-[var(--color-primary)] outline-none placeholder:text-[var(--color-quaternary)] focus:border-[var(--color-accent)]"
        />
      </div>

      <FilterMenu />

      <button
        type="button"
        onClick={() => dispatch(openCreateTask(boardID ?? ''))}
        className="flex h-8 items-center gap-1.5 rounded-[6px] bg-[var(--color-accent)] px-2.5 text-[12px] font-semibold text-[var(--color-on-accent)]"
      >
        <Plus size={14} strokeWidth={2} aria-hidden="true" />
        {t['boards.newTask']}
      </button>

      {statusFilter.length > 0 ? (
        <div className="flex w-full items-center gap-1.5 text-[10px] text-[var(--color-tertiary)]">
          <Filter size={11} aria-hidden="true" />
          {t['boards.filterActive']}
          {statusFilter.map((status) => (
            <button
              key={status}
              type="button"
              onClick={() => dispatch(toggleStatusFilter(status))}
              className="flex items-center gap-1 rounded-[4px] px-1.5 py-0.5 font-mono text-[var(--color-on-accent)]"
              style={{ background: statusColorVar(status) }}
              title={t['boards.clearFilter']}
            >
              {status}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function ViewLink({
  to,
  active,
  icon: Icon,
  label,
}: {
  to: string
  active: boolean
  icon: typeof Table2
  label: string
}) {
  return (
    <Link
      to={to}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex items-center gap-1.5 rounded-[4px] px-2 py-1 text-[11px] transition-colors',
        active
          ? 'bg-[var(--color-surface-hover)] text-[var(--color-primary)]'
          : 'text-[var(--color-tertiary)] hover:text-[var(--color-secondary)]',
      )}
    >
      <Icon size={13} strokeWidth={2} aria-hidden="true" />
      {label}
    </Link>
  )
}

/**
 * Status chips live in a disclosure rather than the bar itself: the design shows
 * one "Filter" affordance, and ten statuses inline pushed the search box off a
 * 1024px window.
 *
 * `<details>` and not a popover component, because this is the one menu in the
 * app that needs no positioning logic — the summary is its own anchor, the
 * browser handles Escape, and no outside-click listener can leak.
 */
function FilterMenu() {
  const dispatch = useAppDispatch()
  const statusFilter = useAppSelector((state) => state.ui.statusFilter)
  const t = useT()

  return (
    <details className="relative">
      <summary className="flex h-8 cursor-pointer list-none items-center gap-1.5 rounded-[6px] border border-[var(--color-border-subtle)] px-2.5 text-[12px] text-[var(--color-secondary)] marker:content-none">
        <Filter size={13} strokeWidth={2} aria-hidden="true" />
        {t['boards.filter']}
        {statusFilter.length > 0 ? (
          <span className="rounded-[3px] bg-[var(--color-accent)] px-1 font-mono text-[10px] text-[var(--color-on-accent)]">
            {statusFilter.length}
          </span>
        ) : null}
      </summary>
      <div className="absolute right-0 z-20 mt-1 flex w-[190px] flex-col gap-0.5 rounded-[8px] border border-[var(--color-border-standard)] bg-[var(--color-surface-elevated)] p-1.5 shadow-card">
        {TASK_STATUSES.map((status) => {
          const active = statusFilter.includes(status)
          return (
            <button
              key={status}
              type="button"
              onClick={() => dispatch(toggleStatusFilter(status))}
              aria-pressed={active}
              className={cn(
                'flex items-center gap-2 rounded-[4px] px-2 py-1 text-left text-[11px]',
                active ? 'text-[var(--color-primary)]' : 'text-[var(--color-tertiary)]',
              )}
            >
              <span
                className="h-2 w-2 shrink-0 rounded-full"
                style={{ background: statusColorVar(status) }}
                aria-hidden="true"
              />
              {status}
            </button>
          )
        })}
      </div>
    </details>
  )
}
