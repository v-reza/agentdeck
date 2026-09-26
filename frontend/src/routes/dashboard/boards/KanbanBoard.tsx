import { DndContext, type DragEndEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { useParams } from 'react-router-dom'
import { useGetBoardQuery, useListTasksQuery, useMoveTaskMutation } from '@/store/api/boards'
import { useOptimisticCards, groupByColumn } from '@/hooks/use-optimistic-card'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { openTask } from '@/store/slices/uiSlice'
import { Column } from '@/components/kanban/Column'
import { BoardToolbar } from '@/components/kanban/BoardToolbar'
import { COLUMN_ORDER, isTaskStatus } from '@/lib/domain'
import { SkeletonRows } from '@/components/ui/skeleton'

/**
 * Screen 18-kanban — drag a card between columns.
 *
 * The drop target is a column key, and the status written back is the column's
 * canonical status. DECISIONS 3 makes a column a view of status, so the mapping
 * goes through the contract's own status list rather than a lookup table here.
 * The move is optimistic and rolls back through React if the API rejects it.
 */
export function KanbanBoard() {
  const { boardID } = useParams<{ boardID: string }>()
  const dispatch = useAppDispatch()
  const { data: board } = useGetBoardQuery(boardID ?? '', { skip: !boardID })
  // The filter is part of the query, not a `.filter()` over the response: the
  // server applies it in SQL (§6.2.16), so the two views and `curl` agree.
  const statusFilter = useAppSelector((state) => state.ui.statusFilter)
  const search = useAppSelector((state) => state.ui.search)
  const { data, isLoading } = useListTasksQuery(
    { boardID: boardID ?? '', statuses: statusFilter, search: search || undefined },
    { skip: !boardID },
  )
  const [moveTask] = useMoveTaskMutation()
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

  return (
    <>
      <BoardToolbar boardName={board?.name} taskCount={tasks.length} />
      <div className="flex min-h-0 flex-1 gap-2.5 overflow-x-auto p-4">
        {isLoading ? (
          <SkeletonRows rows={5} columns={4} />
        ) : (
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
        )}
      </div>
    </>
  )
}
