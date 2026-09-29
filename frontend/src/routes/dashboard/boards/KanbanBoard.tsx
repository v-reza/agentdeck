import { DndContext, type DragEndEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { useParams } from 'react-router-dom'
import { useGetBoardQuery, useListTasksQuery, useMoveTaskMutation } from '@/store/api/boards'
import { useOptimisticCards, groupByColumn } from '@/hooks/use-optimistic-card'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { openCreateTask, openTask } from '@/store/slices/uiSlice'
import { Column } from '@/components/kanban/Column'
import { BoardToolbar } from '@/components/kanban/BoardToolbar'
import { BoardEmpty, BoardError, BoardSkeleton } from '@/components/kanban/BoardStates'
import { MobileBoard } from '@/components/kanban/MobileBoard'
import { useIsMobile } from '@/hooks/use-media-query'
import { COLUMN_ORDER, isTaskStatus } from '@/lib/domain'

/**
 * Screen 18-kanban — drag a card between columns.
 *
 * The drop target is a column key, and the status written back is the column's
 * canonical status. DECISIONS 3 makes a column a view of status, so the mapping
 * goes through the contract's own status list rather than a lookup table here.
 * The move is optimistic and rolls back through React if the API rejects it.
 *
 * US-AD63/64: the four states below are ordered deliberately. Failed load first,
 * then first load, then empty — because an empty list over a failed request is
 * the one combination that must never render the empty state's "create your first
 * task" (US-AD64 AC3), and that is a real risk here: the task query answers with
 * an empty array when it errors, so checking `tasks.length === 0` alone would
 * show the lie.
 *
 * `isLoading` rather than `isFetching` is load-bearing (US-AD63 AC2): RTK Query
 * sets `isLoading` only while the FIRST request is in flight and leaves it false
 * during a background refetch, which is exactly the rule the story asks for. The
 * SSE-driven invalidation of this same query therefore never re-shows a skeleton.
 */
export function KanbanBoard() {
  const { boardID } = useParams<{ boardID: string }>()
  const dispatch = useAppDispatch()
  const { data: board } = useGetBoardQuery(boardID ?? '', { skip: !boardID })
  // The filter is part of the query, not a `.filter()` over the response: the
  // server applies it in SQL (§6.2.16), so the two views and `curl` agree.
  const statusFilter = useAppSelector((state) => state.ui.statusFilter)
  const search = useAppSelector((state) => state.ui.search)
  const { data, isLoading, isError, refetch, isFetching } = useListTasksQuery(
    { boardID: boardID ?? '', statuses: statusFilter, search: search || undefined },
    { skip: !boardID },
  )
  const [moveTask] = useMoveTaskMutation()
  // US-AD60 AC3: below 768px the lanes become an accordion. Read as a value, not
  // as a `md:` class, because the two are different LAYOUTS — hiding one with CSS
  // would still mount both trees and run both sets of hooks.
  const isMobile = useIsMobile()
  const { tasks, moveCard } = useOptimisticCards(data ?? [])

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  function onDragEnd(event: DragEndEvent) {
    const taskID = String(event.active.id)
    const target = event.over?.id ? String(event.over.id) : null
    if (!target) return

    const task = tasks.find((t) => t.id === taskID)
    if (!task) return

    // A column key is a status name for the five default columns, so a drop is
    // only honoured when the target really is a status (DECISIONS 3).
    if (!isTaskStatus(target) || target === task.status) return
    const to = target

    moveCard({ taskID, from: task.status, to }, () => moveTask({ id: taskID, from: task.status, to }).unwrap())
  }

  const grouped = groupByColumn(tasks)
  const filtered = statusFilter.length > 0 || search.trim().length > 0

  return (
    <>
      <BoardToolbar boardName={board?.name} taskCount={tasks.length} />
      {isError ? (
        <BoardError onRetry={() => void refetch()} retrying={isFetching} />
      ) : isLoading ? (
        <BoardSkeleton />
      ) : tasks.length === 0 ? (
        <BoardEmpty filtered={filtered} onCreate={() => dispatch(openCreateTask(boardID ?? ''))} />
      ) : isMobile ? (
        <MobileBoard tasks={tasks} onOpenTask={(id) => dispatch(openTask(id))} />
      ) : (
        <div className="flex min-h-0 flex-1 gap-2.5 overflow-x-auto p-4">
          <DndContext sensors={sensors} onDragEnd={onDragEnd}>
            {COLUMN_ORDER.map((columnKey) => (
              <Column
                key={columnKey}
                columnKey={columnKey}
                tasks={grouped[columnKey]}
                onOpenTask={(id) => dispatch(openTask(id))}
              />
            ))}
          </DndContext>
        </div>
      )}
    </>
  )
}
