import { useEffect } from 'react'
import { useListBoardsQuery, useListTasksQuery } from '@/store/api/boards'
import { useAppDispatch } from '@/store/hooks'
import { reportBoardCount, reportBoardTaskStats } from '@/store/slices/directorySlice'
import type { Board, Task } from '@/lib/domain'

/**
 * The project directory's data, one query per row.
 *
 * `useListBoardsQuery(projectID)` and `useListTasksQuery(boardID)` are called by
 * the row components themselves — never in a loop — because RTK Query ships no
 * `useQueries` (verified against @reduxjs/toolkit 2.12: the export does not
 * exist). A hook called inside `map()` would also break the rules of hooks.
 *
 * Each hook reports its answer into `directorySlice` so the summary row can show
 * workspace totals without a second, invented source of truth.
 */

/** A project's boards, with the answer reported for the summary row. */
export function useProjectBoards(projectID: string): {
  boards: Board[]
  isUnresolved: boolean
} {
  const dispatch = useAppDispatch()
  const enabled = projectID !== ''
  const { data, isUninitialized, isLoading } = useListBoardsQuery(projectID, { skip: !enabled })
  const boards = data ?? []
  const isUnresolved = enabled && (isUninitialized || isLoading)

  useEffect(() => {
    if (!enabled || isUnresolved) return
    dispatch(reportBoardCount({ projectID, count: boards.length }))
  }, [dispatch, projectID, enabled, boards.length, isUnresolved])

  return { boards, isUnresolved }
}

/**
 * A board's tasks, with the answer reported for the summary row and the rail.
 *
 * `skip` on an empty id is load-bearing, not defensive: the cost rail calls this
 * on every page, and without the guard the request becomes
 * `GET /api/v1/boards//tasks`, which the API answers with 500 "no rows in result
 * set" — a failed query and a danger toast on a page that never mentioned a
 * board. An empty id means "no board open", which is a normal state, not an
 * error.
 */
export function useBoardTasks(boardID: string): {
  tasks: Task[]
  running: number
  isUnresolved: boolean
} {
  const dispatch = useAppDispatch()
  const enabled = boardID !== ''
  const { data, isUninitialized, isLoading } = useListTasksQuery(boardID, { skip: !enabled })
  const tasks = data ?? []
  const isUnresolved = enabled && (isUninitialized || isLoading)
  const running = tasks.filter((task) => task.status === 'running').length

  useEffect(() => {
    if (!enabled || isUnresolved) return
    dispatch(reportBoardTaskStats({ boardID, tasks: tasks.length, running }))
  }, [dispatch, boardID, enabled, tasks.length, running, isUnresolved])

  return { tasks, running, isUnresolved }
}
