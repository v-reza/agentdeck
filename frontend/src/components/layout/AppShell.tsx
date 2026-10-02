import { useEffect, type ReactNode } from 'react'
import { cn } from '@/lib/cn'
import { useLocation } from 'react-router-dom'
import { IconRail } from './IconRail'
import { WorkspaceSidebar } from './WorkspaceSidebar'
import { CostRail } from './CostRail'
import { MobileTabBar } from './MobileTabBar'
import { useIsMobile } from '@/hooks/use-media-query'
import { TaskDrawerHost } from '@/routes/dashboard/boards/TaskDrawerHost'
import { TaskCreateHost } from '@/routes/dashboard/boards/TaskCreateForm'
import { CommandPalette } from './CommandPalette'
import { ErrorBoundary } from '@/components/ui/error-boundary'
import { useBoardHealth } from '@/hooks/use-board-health'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { closeTask } from '@/store/slices/uiSlice'

/**
 * The four-zone application shell: 44px rail, 224px sidebar, the content pane,
 * and the 264px cost rail (DESIGN.md `shell-*` components; ARCHITECTURE 18.2
 * `components/layout/`).
 *
 * The shell owns the frame and nothing else. Each page supplies its own topbar
 * and body, so a new route reuses the rail, the sidebar tree, and the cost rail
 * instead of re-implementing them — the god-component page this replaces had all
 * four zones inlined in one file.
 *
 * `withCostRail` is per route: the list and finops pages already spend their
 * width on numbers, so the rail is hidden there and shown where it is the only
 * place cost is visible.
 */
export interface AppShellProps {
  children: ReactNode
  withCostRail?: boolean
}

export function AppShell({ children, withCostRail = true }: AppShellProps) {
  const { connected } = useBoardHealth()
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const openTaskID = useAppSelector((state) => state.ui.openTaskID)
  const dispatch = useAppDispatch()
  const { pathname } = useLocation()
  // US-AD60: the desktop shell is 44 + 224 + 264 = 532px of chrome. At a phone
  // width the content pane is narrower than zero and the cost rail paints over the
  // board, so the side chrome is not merely cramped — it makes the board
  // unreachable. Below the breakpoint the rail, the sidebar and the cost rail come
  // off the screen and navigation moves to the bottom bar, which is what the
  // mobile design source shows.
  const isMobile = useIsMobile()

  // The drawer now lives in the shell, so it outlives the board route. Without
  // this it would stay open showing a task from a board the operator left.
  useEffect(() => {
    if (openTaskID) dispatch(closeTask())
    // eslint-disable-next-line react-hooks/exhaustive-deps -- path change is the trigger
  }, [pathname])

  return (
    <div className="relative flex h-screen w-screen overflow-hidden bg-[var(--color-surface-page)]">
      {isMobile ? null : <IconRail daemonConnected={connected} />}
      {isMobile ? null : <WorkspaceSidebar />}

      <div className={cn('flex min-w-0 flex-1 flex-col', isMobile && 'pb-[var(--spacing-tabbar)]')}>
        {activeOrgID ? (
          // US-AD65: the boundary wraps the CONTENT, not the shell. A screen that
          // throws loses its own pane and keeps the rail, the sidebar and the cost
          // rail — the operator stays oriented and can navigate away, instead of
          // the whole app unmounting to a white screen. `resetKey` is the path, so
          // leaving the broken screen clears the error rather than pinning the
          // fallback in place for the rest of the session.
          <ErrorBoundary resetKey={pathname}>{children}</ErrorBoundary>
        ) : (
          <NoWorkspace />
        )}
      </div>

      <TaskDrawerHost />
      <TaskCreateHost />
      {/* US-AD55. Mounted once, in the shell, so the Cmd+K listener exists on every
          dashboard screen without each route remembering to add it. */}
      <CommandPalette />

      {withCostRail && !isMobile ? <CostRail /> : null}

      {isMobile ? (
        <div className="absolute inset-x-0 bottom-0">
          <MobileTabBar />
        </div>
      ) : null}
    </div>
  )
}

function NoWorkspace() {
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="max-w-md rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-5 text-center">
        <h2 className="text-[14px] font-semibold text-[var(--color-primary)]">No workspace selected</h2>
        <p className="mt-1 text-[12px] text-[var(--color-secondary)]">
          Registration creates a personal workspace automatically. Seeing this means the session has no membership —
          sign in again.
        </p>
      </div>
    </div>
  )
}
