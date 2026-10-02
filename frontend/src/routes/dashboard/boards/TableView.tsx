import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { ArrowDown, ArrowUp, ChevronsUpDown, RotateCcw } from 'lucide-react'
import { useGetBoardQuery, useListTasksQuery } from '@/store/api/boards'
import { useListAgentsQuery } from '@/store/api/agents'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { openTask, toggleStatusFilter } from '@/store/slices/uiSlice'
import { EmptyState } from '@/components/ui/card'
import { BoardToolbar } from '@/components/kanban/BoardToolbar'
import { TASK_STATUSES, statusColorVar, type Task } from '@/lib/domain'
import { formatEstimatedMicroUSD, shortID } from '@/lib/formatters'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/cn'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useT } from '@/hooks/use-t'

/**
 * Screen 19-table-view — the same tasks as the kanban, dense (32px rows), with
 * the filter state living in `uiSlice` so it survives navigation between the two
 * views. Nothing is fetched twice: both views read the same RTK Query cache.
 *
 * US-AD54 AC1 fixes the columns at seven: ID, Title, Status, Priority, Agent,
 * Cost, Created. The old table carried a `tokens` column instead of ID/Agent/
 * Created — a column the story never asked for, standing in for three it did.
 * Tokens are still one click away in the drawer.
 *
 * US-AD54 AC2 is multi-sort: every header is a button and the sorts compose in
 * click order, which is what the design's "Active Multi-Sort" indicator shows.
 * It is client-side on purpose — `listTasks` has no sort parameter in the
 * contract (§6.2.16), and a sort chain is view state, so it stays in this
 * component rather than in `uiSlice` where it would outlive the board.
 */

/** The sortable columns. `key` is also the field a task is read for. */
type SortKey = 'id' | 'title' | 'status' | 'priority' | 'agent' | 'cost' | 'created'

interface SortCriterion {
  key: SortKey
  direction: 'asc' | 'desc'
}

/**
 * The value a column sorts by, always a primitive.
 *
 * `agent` resolves through the board's agent list: `assignee_agent_id` is a
 * ULID, and sorting ULIDs would order the column differently from the names the
 * operator can actually read. A task with no agent sorts as the empty string so
 * those rows group at one end instead of scattering through the list.
 */
function sortValue(task: Task, key: SortKey, agentNames: Map<string, string>): string | number {
  switch (key) {
    case 'id':
      return task.id
    case 'title':
      return task.title.toLowerCase()
    case 'status':
      return task.status
    case 'priority':
      return task.priority
    case 'agent':
      return (agentNames.get(task.assignee_agent_id) ?? '').toLowerCase()
    case 'cost':
      return task.cost_micros
    case 'created':
      return task.created_at
  }
}

export function TableView() {
  const { boardID } = useParams<{ boardID: string }>()
  const dispatch = useAppDispatch()
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const { data: board } = useGetBoardQuery(boardID ?? '', { skip: !boardID })
  const statusFilter = useAppSelector((state) => state.ui.statusFilter)
  const search = useAppSelector((state) => state.ui.search)
  // The filters are the server's (§6.2.16), not a pass over the response: this
  // view used to hide rows the API still counted, so the two disagreed.
  const { data, isLoading } = useListTasksQuery(
    { boardID: boardID ?? '', statuses: statusFilter, search: search || undefined },
    { skip: !boardID },
  )
  const tasks = useMemo(() => data ?? [], [data])

  // Agents come from the board's project so the Agent column can show a name.
  // `board` is undefined on the first paint, so the query is skipped until it
  // arrives rather than fired against an empty project id.
  const { data: agents } = useListAgentsQuery(board?.project_id ?? '', { skip: !board?.project_id })
  const agentNames = useMemo(() => new Map((agents ?? []).map((a) => [a.id, a.name])), [agents])

  const [sorts, setSorts] = useState<SortCriterion[]>([])

  /**
   * Clicking a header appends it to the sort chain, or flips its direction when
   * it is already in the chain. Appending rather than replacing is the point of
   * AC2 — the design's indicator shows two active sorts at once.
   */
  function toggleSort(key: SortKey) {
    setSorts((current) => {
      const at = current.findIndex((c) => c.key === key)
      if (at === -1) return [...current, { key, direction: 'asc' }]
      const next = [...current]
      next[at] = { key, direction: current[at].direction === 'asc' ? 'desc' : 'asc' }
      return next
    })
  }

  const sorted = useMemo(() => {
    if (sorts.length === 0) return tasks
    return [...tasks].sort((a, b) => {
      for (const { key, direction } of sorts) {
        const av = sortValue(a, key, agentNames)
        const bv = sortValue(b, key, agentNames)
        if (av === bv) continue
        const cmp = av < bv ? -1 : 1
        return direction === 'asc' ? cmp : -cmp
      }
      return 0
    })
  }, [tasks, sorts, agentNames])

  const columns: { key: SortKey; label: string; className?: string }[] = [
    { key: 'id', label: t['table.colID'], className: 'w-[90px]' },
    { key: 'title', label: t['table.colTitle'] },
    { key: 'status', label: t['table.colStatus'], className: 'w-[110px]' },
    { key: 'priority', label: t['table.colPriority'], className: 'w-[130px]' },
    { key: 'agent', label: t['table.colAgent'], className: 'w-[170px]' },
    { key: 'cost', label: t['table.colCost'], className: 'w-[120px]' },
    { key: 'created', label: t['table.colCreated'], className: 'w-[160px]' },
  ]

  const priorityLabels = [t['table.priorityP0'], t['table.priorityP1'], t['table.priorityP2'], t['table.priorityP3']]

  return (
    <>
      <BoardToolbar boardName={board?.name} taskCount={tasks.length} />

      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-4">
        <div className="flex flex-wrap gap-1">
          {TASK_STATUSES.map((status) => {
            const active = statusFilter.includes(status)
            return (
              <button
                key={status}
                type="button"
                onClick={() => dispatch(toggleStatusFilter(status))}
                className={cn(
                  'rounded-[4px] border px-2 py-0.5 font-mono text-[10px] transition-colors',
                  active
                    ? 'border-transparent text-[var(--color-on-accent)]'
                    : 'border-[var(--color-border-subtle)] text-[var(--color-secondary)] hover:text-[var(--color-primary)]',
                )}
                style={active ? { background: statusColorVar(status) } : undefined}
              >
                {status}
              </button>
            )
          })}
        </div>

        {/* The design's "Active Multi-Sort" bar: the chain in click order, each
            chip numbered, with the reset the mockup also draws. It appears only
            when there is a sort, so an unsorted table is not decorated with a
            control that does nothing. */}
        {sorts.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2 font-mono text-[11px]">
            <span className="text-[var(--color-tertiary)]">{t['table.activeSort']}</span>
            {sorts.map((criterion, index) => (
              <span
                key={criterion.key}
                data-testid="table-sort-chip"
                data-sort-key={criterion.key}
                data-sort-direction={criterion.direction}
                data-sort-order={index + 1}
                className={cn(
                  'inline-flex items-center gap-1 rounded-[4px] border px-2 py-0.5',
                  index === 0
                    ? 'border-[var(--color-accent)]/40 bg-[var(--color-accent-light)] font-semibold text-[var(--color-accent)]'
                    : 'border-[var(--color-border-subtle)] bg-[var(--color-surface-hover)] text-[var(--color-secondary)]',
                )}
              >
                <span>{columns.find((c) => c.key === criterion.key)?.label}</span>
                {criterion.direction === 'asc' ? (
                  <ArrowUp size={13} aria-hidden />
                ) : (
                  <ArrowDown size={13} aria-hidden />
                )}
                <span className="rounded-[2px] bg-[var(--color-border-subtle)] px-1 text-[9px]">{index + 1}</span>
              </span>
            ))}
            <button
              type="button"
              data-testid="table-sort-reset"
              onClick={() => setSorts([])}
              className="ml-1 inline-flex items-center gap-1 text-[10px] text-[var(--color-tertiary)] underline hover:text-[var(--color-primary)]"
            >
              <RotateCcw size={11} aria-hidden />
              {t['table.resetSort']}
            </button>
          </div>
        ) : null}

        {isLoading ? (
          <SkeletonRows rows={6} columns={7} />
        ) : tasks.length === 0 ? (
          <EmptyState title={t['table.empty']} hint={t['table.emptyHint']} />
        ) : (
          // The design wraps the table in `overflow-x-auto` for a reason: seven
          // fixed-width columns total more than the content pane at a 1280px
          // viewport (44 rail + 224 sidebar + 264 cost rail leaves ~716px).
          // Without it the Title column is squeezed to nothing and every cell
          // wraps to four lines — a 55px row where AC1 asks for 32px. The table
          // scrolls instead of wrapping.
          <div className="overflow-x-auto">
            <table className="w-full min-w-[960px] border-collapse text-[12px]">
              <thead>
                <tr className="border-b border-[var(--color-border-subtle)] text-left">
                  {columns.map((column) => {
                    const at = sorts.findIndex((c) => c.key === column.key)
                    const criterion = at === -1 ? null : sorts[at]
                    return (
                      <th
                        key={column.key}
                        className={cn(
                          'px-3 py-1.5 text-[10px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]',
                          column.className,
                        )}
                        // The position in the chain, not just the direction, so a
                        // screen reader hears "ascending, 2nd" instead of a
                        // direction that does not say where it sits.
                        aria-sort={
                          criterion === null ? 'none' : criterion.direction === 'asc' ? 'ascending' : 'descending'
                        }
                      >
                        <button
                          type="button"
                          data-testid={`table-sort-${column.key}`}
                          data-sort-order={at === -1 ? undefined : at + 1}
                          onClick={() => toggleSort(column.key)}
                          className="inline-flex items-center gap-1 uppercase tracking-[0.06em] transition-colors hover:text-[var(--color-primary)]"
                        >
                          {column.label}
                          {criterion === null ? (
                            <ChevronsUpDown size={12} className="opacity-40" aria-hidden />
                          ) : criterion.direction === 'asc' ? (
                            <ArrowUp size={12} aria-hidden />
                          ) : (
                            <ArrowDown size={12} aria-hidden />
                          )}
                        </button>
                      </th>
                    )
                  })}
                </tr>
              </thead>
              <tbody>
                {sorted.map((task) => {
                  const agentName = agentNames.get(task.assignee_agent_id)
                  return (
                    <tr
                      key={task.id}
                      onClick={() => dispatch(openTask(task.id))}
                      className="h-8 cursor-pointer border-b border-[var(--color-border-subtle)] hover:bg-[var(--color-surface-hover)]"
                    >
                      <td className="truncate px-3 font-mono text-[10px] text-[var(--color-quaternary)]">
                        {shortID(task.id)}
                      </td>
                      <td className="truncate px-3 text-[var(--color-primary)]">{task.title}</td>
                      <td className="px-3">
                        <span
                          className="rounded-[4px] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-on-accent)]"
                          style={{ background: statusColorVar(task.status) }}
                        >
                          {task.status}
                        </span>
                      </td>
                      <td className="truncate px-3 font-mono text-[11px] text-[var(--color-secondary)]">
                        {priorityLabels[task.priority] ?? String(task.priority)}
                      </td>
                      <td className="px-3">
                        {agentName ? (
                          <span className="inline-flex items-center gap-1.5">
                            <span className="inline-flex h-4 w-4 items-center justify-center rounded-[4px] bg-[var(--color-surface-hover)] font-mono text-[9px] text-[var(--color-secondary)]">
                              {agentName.slice(0, 2).toLowerCase()}
                            </span>
                            <span className="text-[11px] text-[var(--color-secondary)]">{agentName}</span>
                          </span>
                        ) : (
                          <span className="text-[11px] text-[var(--color-quaternary)]">{t['table.noAgent']}</span>
                        )}
                      </td>
                      <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">
                        {formatEstimatedMicroUSD(task.cost_micros)}
                      </td>
                      <td className="whitespace-nowrap px-3 font-mono text-[11px] text-[var(--color-secondary)]">
                        {formatDateTime(task.created_at, lang)}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  )
}
