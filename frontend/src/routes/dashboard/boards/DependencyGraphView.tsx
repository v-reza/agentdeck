import { DependencyGraph } from '@/components/kanban/DependencyGraph'

/**
 * Screen 24-dependency-view — the route shell for the board's dependency graph
 * (US-AD19).
 *
 * A one-line shell on purpose: `BoardToolbar` owns the view switcher and the SSE
 * subscription, and it is rendered by `DependencyGraph` itself, so mounting it
 * here as well would open a second stream for the same board.
 */
export function DependencyGraphView() {
  return <DependencyGraph />
}
