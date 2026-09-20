import { useAppSelector } from '@/store/hooks'
import { directoryTotals, type BoardTaskStats } from '@/store/slices/directorySlice'

/**
 * Workspace totals as reported by the directory rows.
 *
 * Reads the projection in `directorySlice` rather than re-fetching: the rows
 * already hold every answer, and a second source of truth for the same number is
 * how the header and the table start disagreeing.
 */
export function useDirectoryTotals(): { boards: number; tasks: number; running: number } {
  return useAppSelector((state) => directoryTotals(state.directory))
}

/**
 * The whole per-board stats map.
 *
 * This exists so a group row can total its own children without calling a hook
 * per child — `useAppSelector` once, then plain arithmetic. A hook inside
 * `boards.map()` would break the rules of hooks the moment a project has a
 * different number of boards than the previous render.
 */
export function useBoardTaskStats(): Record<string, BoardTaskStats> {
  return useAppSelector((state) => state.directory.taskStats)
}
