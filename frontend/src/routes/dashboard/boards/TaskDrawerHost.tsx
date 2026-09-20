import { useAppSelector } from '@/store/hooks'
import { TaskDetailDrawer } from './TaskDetailDrawer'

/**
 * Mounts the task drawer for whatever task the UI slice has open.
 *
 * The drawer is driven by `ui.openTaskID` rather than by a route, so opening a
 * card does not lose the board's scroll position or its drag state, and the same
 * drawer works from both the kanban and the table.
 */
export function TaskDrawerHost() {
  const openTaskID = useAppSelector((state) => state.ui.openTaskID)
  if (!openTaskID) return null
  return <TaskDetailDrawer taskID={openTaskID} />
}
