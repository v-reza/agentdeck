import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { ColumnKey, TaskStatus } from '@/lib/domain'

/**
 * Client-only view state: what the operator has open and how they have narrowed
 * the board. None of this is server state, so none of it belongs in RTK Query
 * (ARCHITECTURE 18.2). It survives navigation between boards, which is exactly
 * why it is Redux and not component state.
 */
export interface UiState {
  /** Board view mode; the Kanban and the dense Table read the same tasks. */
  boardView: 'kanban' | 'table'
  /** Filter chip selection; empty means "no status filter". */
  statusFilter: TaskStatus[]
  assigneeFilter: string | null
  /** Case-insensitive substring match on task title. */
  search: string
  /** Task detail drawer: the open task id, or null when closed. */
  openTaskID: string | null
  /** Column the drag started from, for the optimistic overlay. */
  draggingFrom: ColumnKey | null
  /** Command palette (screen 14) visibility. */
  commandPaletteOpen: boolean
  /** Project directory search box (design source: topbar search, 240px). */
  directorySearch: string
  /** US-AD91 AC2: the directory is sortable by task count or by cost. */
  directorySort: DirectorySort
  /** Collapsed project groups in the directory, by project id. */
  directoryCollapsed: string[]
}

/** US-AD91 AC2 — the sortable columns, and the direction they are sorted in. */
export type DirectorySortKey = 'name' | 'tasks' | 'cost'
export interface DirectorySort {
  key: DirectorySortKey
  direction: 'asc' | 'desc'
}

const initialState: UiState = {
  boardView: 'kanban',
  statusFilter: [],
  assigneeFilter: null,
  search: '',
  openTaskID: null,
  draggingFrom: null,
  commandPaletteOpen: false,
  directorySearch: '',
  directorySort: { key: 'name', direction: 'asc' },
  directoryCollapsed: [],
}

const uiSlice = createSlice({
  name: 'ui',
  initialState,
  reducers: {
    setBoardView(state, action: PayloadAction<'kanban' | 'table'>) {
      state.boardView = action.payload
    },
    toggleStatusFilter(state, action: PayloadAction<TaskStatus>) {
      const index = state.statusFilter.indexOf(action.payload)
      if (index === -1) state.statusFilter.push(action.payload)
      else state.statusFilter.splice(index, 1)
    },
    setAssigneeFilter(state, action: PayloadAction<string | null>) {
      state.assigneeFilter = action.payload
    },
    setSearch(state, action: PayloadAction<string>) {
      state.search = action.payload
    },
    openTask(state, action: PayloadAction<string>) {
      state.openTaskID = action.payload
    },
    closeTask(state) {
      state.openTaskID = null
    },
    setDraggingFrom(state, action: PayloadAction<ColumnKey | null>) {
      state.draggingFrom = action.payload
    },
    toggleCommandPalette(state) {
      state.commandPaletteOpen = !state.commandPaletteOpen
    },
    setDirectorySearch(state, action: PayloadAction<string>) {
      state.directorySearch = action.payload
    },
    /**
     * US-AD91 AC2. Clicking the active column flips the direction; clicking a
     * different column switches to it descending, because "sort by cost" is
     * almost always asked as "which costs most".
     */
    setDirectorySort(state, action: PayloadAction<DirectorySortKey>) {
      if (state.directorySort.key === action.payload) {
        state.directorySort.direction = state.directorySort.direction === 'asc' ? 'desc' : 'asc'
        return
      }
      state.directorySort = { key: action.payload, direction: action.payload === 'name' ? 'asc' : 'desc' }
    },
    toggleDirectoryCollapsed(state, action: PayloadAction<string>) {
      const index = state.directoryCollapsed.indexOf(action.payload)
      if (index === -1) state.directoryCollapsed.push(action.payload)
      else state.directoryCollapsed.splice(index, 1)
    },
  },
})

export const {
  setBoardView,
  toggleStatusFilter,
  setAssigneeFilter,
  setSearch,
  openTask,
  closeTask,
  setDraggingFrom,
  toggleCommandPalette,
  setDirectorySearch,
  setDirectorySort,
  toggleDirectoryCollapsed,
} = uiSlice.actions
export default uiSlice.reducer
