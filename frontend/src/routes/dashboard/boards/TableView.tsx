import { useParams } from 'react-router-dom'
import { useGetBoardQuery, useListTasksQuery } from '@/store/api/boards'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { openTask, setSearch, toggleStatusFilter } from '@/store/slices/uiSlice'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState } from '@/components/ui/card'
import { TASK_STATUSES, statusColorVar } from '@/lib/domain'
import { formatEstimatedMicroUSD, formatTokens, shortID } from '@/lib/formatters'
import { cn } from '@/lib/cn'

/**
 * Screen 19-table-view — the same tasks as the kanban, dense (32px rows), with
 * the filter state living in `uiSlice` so it survives navigation between the two
 * views. Nothing is fetched twice: both views read the same RTK Query cache.
 */
export function TableView() {
  const { boardID } = useParams<{ boardID: string }>()
  const dispatch = useAppDispatch()
  const { data: board } = useGetBoardQuery(boardID ?? '', { skip: !boardID })
  const { data, isLoading } = useListTasksQuery(boardID ?? '', { skip: !boardID })
  const statusFilter = useAppSelector((state) => state.ui.statusFilter)
  const search = useAppSelector((state) => state.ui.search)

  const tasks = (data ?? []).filter((task) => {
    if (statusFilter.length > 0 && !statusFilter.includes(task.status)) return false
    if (search && !task.title.toLowerCase().includes(search.toLowerCase())) return false
    return true
  })

  return (
    <>
      <WorkspaceTopbar
        title={board?.name ?? 'Board'}
        subtitle={`${tasks.length} tasks`}
        right={
          <input
            value={search}
            onChange={(event) => dispatch(setSearch(event.target.value))}
            placeholder="Search tasks"
            className="h-8 w-[180px] rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2.5 text-[12px] text-[var(--color-primary)] outline-none placeholder:text-[var(--color-quaternary)] focus:border-[var(--color-accent)]"
          />
        }
      />

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

        {isLoading ? (
          <p className="font-mono text-[12px] text-[var(--color-tertiary)]">Loading…</p>
        ) : tasks.length === 0 ? (
          <EmptyState title="No tasks match" hint="Clear a filter or add a task to this board." />
        ) : (
          <table className="w-full border-collapse">
            <thead>
              <tr className="border-b border-[var(--color-border-subtle)] text-left">
                <Th>Task</Th>
                <Th className="w-[120px]">Status</Th>
                <Th className="w-[80px]">Priority</Th>
                <Th className="w-[110px]">Tokens</Th>
                <Th className="w-[100px]">Cost</Th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((task) => (
                <tr
                  key={task.id}
                  onClick={() => dispatch(openTask(task.id))}
                  className="h-8 cursor-pointer border-b border-[var(--color-border-subtle)] last:border-b-0 hover:bg-[var(--color-surface-hover)]"
                >
                  <td className="px-3">
                    <span className="text-[12px] text-[var(--color-primary)]">{task.title}</span>
                    <span className="ml-2 font-mono text-[10px] text-[var(--color-quaternary)]">
                      {shortID(task.id)}
                    </span>
                  </td>
                  <td className="px-3">
                    <span
                      className="rounded-[4px] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-on-accent)]"
                      style={{ background: statusColorVar(task.status) }}
                    >
                      {task.status}
                    </span>
                  </td>
                  <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">{task.priority}</td>
                  <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">
                    {formatTokens(task.tokens_in + task.tokens_out)}
                  </td>
                  <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">
                    {formatEstimatedMicroUSD(task.cost_micros)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  )
}

function Th({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <th
      className={[
        'px-3 py-1.5 text-[10px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]',
        className ?? '',
      ].join(' ')}
    >
      {children}
    </th>
  )
}
