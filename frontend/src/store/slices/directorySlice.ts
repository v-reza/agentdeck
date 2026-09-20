import { createSelector, createSlice, type PayloadAction } from '@reduxjs/toolkit'

/**
 * Per-row server answers, projected so the summary cards can total them.
 *
 * RTK Query has no `useQueries`, so a project's boards and a board's tasks are
 * each fetched by the row that renders them (one stable cache key per row, no
 * hook loop). That leaves the *totals* — "6 boards", "103 tasks" — with no owner,
 * because a parent cannot fan out over an unknown number of children.
 *
 * Rows therefore report what the server told them here, and the summary reads
 * the projection. Nothing is estimated: a row that has not answered yet is
 * absent from the map, and the total simply excludes it rather than counting a
 * zero it never received.
 */
export interface BoardTaskStats {
  tasks: number
  running: number
}

export type BoardSortKey = 'name' | 'tasks' | 'cost'

export interface BoardSort {
  key: BoardSortKey
  direction: 'asc' | 'desc'
}

export interface DirectoryState {
  boardCounts: Record<string, number>
  taskStats: Record<string, BoardTaskStats>
  /** Board-list sort (US-AD91 AC2: sortable by cost or task count). */
  boardSort: BoardSort
}

const initialState: DirectoryState = {
  boardCounts: {},
  taskStats: {},
  // The design source shows Cost today active, descending.
  boardSort: { key: 'cost', direction: 'desc' },
}

const directorySlice = createSlice({
  name: 'directory',
  initialState,
  reducers: {
    reportBoardCount(state, action: PayloadAction<{ projectID: string; count: number }>) {
      state.boardCounts[action.payload.projectID] = action.payload.count
    },
    reportBoardTaskStats(state, action: PayloadAction<{ boardID: string } & BoardTaskStats>) {
      state.taskStats[action.payload.boardID] = { tasks: action.payload.tasks, running: action.payload.running }
    },
    /**
     * Clicking a sortable header. Selecting the column already active flips the
     * direction; selecting a new one starts descending for cost and task count
     * (the interesting end first) and ascending for a name.
     */
    setBoardSort(state, action: PayloadAction<BoardSortKey>) {
      if (state.boardSort.key === action.payload) {
        state.boardSort.direction = state.boardSort.direction === 'asc' ? 'desc' : 'asc'
        return
      }
      state.boardSort = { key: action.payload, direction: action.payload === 'name' ? 'asc' : 'desc' }
    },
  },
})

export const { reportBoardCount, reportBoardTaskStats, setBoardSort } = directorySlice.actions
export default directorySlice.reducer

/**
 * Totals over everything reported so far, for the summary row.
 *
 * Memoized with `createSelector`: this returns a fresh object, and a plain
 * selector handing back a new reference on every call makes React-Redux warn
 * and re-render every subscriber on each store change. The cache key is the two
 * maps, so the total is recomputed only when a row actually reports something.
 */
export const directoryTotals = createSelector(
  [(state: DirectoryState) => state.boardCounts, (state: DirectoryState) => state.taskStats],
  (boardCounts, taskStats) => {
    let boards = 0
    for (const count of Object.values(boardCounts)) boards += count

    let tasks = 0
    let running = 0
    for (const stats of Object.values(taskStats)) {
      tasks += stats.tasks
      running += stats.running
    }

    return { boards, tasks, running }
  },
)

/** A single board's stats, or undefined while its row has not answered. */
export function boardStats(state: DirectoryState, boardID: string): BoardTaskStats | undefined {
  return state.taskStats[boardID]
}
