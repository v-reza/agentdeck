import { startTransition, useCallback, useOptimistic } from 'react'
import type { Task, TaskStatus, ColumnKey } from '@/lib/domain'
import { columnForStatus } from '@/lib/domain'

/**
 * ARCHITECTURE 18.2: "Optimistic drag kartu | RTK Query `optimisticUpdate` +
 * `useOptimistic` (React 19)".
 *
 * `moveCard` applies the new status through `useOptimistic` inside a transition
 * and only then awaits the mutation. If the mutation rejects, the transition
 * settles and React discards the optimistic value, so the card returns to its
 * server status with no compensating code — the runtime is the rollback.
 */
export interface PendingMove {
  taskID: string
  from: TaskStatus
  to: TaskStatus
}

export function useOptimisticCards(tasks: Task[]): {
  tasks: Task[]
  moveCard: (move: PendingMove, commit: () => Promise<unknown>) => void
} {
  const [optimisticTasks, applyMove] = useOptimistic(tasks, (current: Task[], move: PendingMove) =>
    current.map((task) => (task.id === move.taskID ? { ...task, status: move.to } : task)),
  )

  const moveCard = useCallback(
    (move: PendingMove, commit: () => Promise<unknown>) => {
      startTransition(async () => {
        applyMove(move)
        try {
          await commit()
        } catch {
          // Swallowed on purpose: RTK Query records the rejection and the
          // toast listener reports it. Rethrowing here would surface an
          // unhandled rejection from a transition.
        }
      })
    },
    [applyMove],
  )

  return { tasks: optimisticTasks, moveCard }
}

/** Groups tasks into the five board columns. Shared by Kanban and Table views. */
export function groupByColumn(tasks: Task[]): Record<ColumnKey, Task[]> {
  const grouped: Record<ColumnKey, Task[]> = {
    backlog: [],
    ready: [],
    running: [],
    review: [],
    done: [],
  }
  for (const task of tasks) {
    grouped[columnForStatus(task.status)].push(task)
  }
  return grouped
}
