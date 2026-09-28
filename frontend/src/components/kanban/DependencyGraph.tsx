import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link2, Lock, Plus } from 'lucide-react'
import { useParams } from 'react-router-dom'
import { useBoardDependenciesQuery, useGetBoardQuery, useListTasksQuery } from '@/store/api/boards'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { openCreateTask, openTask } from '@/store/slices/uiSlice'
import { BoardToolbar } from '@/components/kanban/BoardToolbar'
import { SkeletonRows } from '@/components/ui/skeleton'
import { statusColorVar } from '@/lib/domain'
import { groupByColumn } from '@/hooks/use-optimistic-card'
import type { BoardDependency, Task } from '@/lib/domain'
import { useT } from '@/hooks/use-t'
import { interpolate } from '@/lib/format'
import { cn } from '@/lib/cn'

/**
 * Screen 24-dependency-view — the board's dependency graph (US-AD19).
 *
 * The layout is the kanban's lanes, not a force-directed graph: the lanes already
 * mean status, and a DAG drawn on top of them reads as "this card is waiting on
 * that one, which is still in Backlog" without a legend. Edges are drawn as SVG
 * between measured card positions rather than through a layout engine, because
 * the cards are the real DOM the operator clicks.
 *
 * AC1 asks for two things on a card that has dependencies: a count badge and a
 * thin line to the parent. The count comes from the edge set, the line from the
 * same set — one request per board (`GET /boards/{id}/dependencies`), not one
 * per card.
 *
 * What this does NOT do, and why:
 *
 *  - It does not draw an edge to a parent that lives on ANOTHER board. The edge
 *    is real and the server reports it, but there is no card on screen to draw to,
 *    so it is listed under the child instead of invented as a floating node.
 *    Dropping it entirely would be worse: the card would read as unblocked while
 *    the dispatcher still refuses to promote it.
 *  - The locked marker reads `block_kind`, which is a field the task already
 *    carries. A card is never marked locked from the edge count alone — a task
 *    whose parents are all `done` has edges and is not blocked.
 */

interface Point {
  x: number
  y: number
}

export function DependencyGraph() {
  const { boardID } = useParams<{ boardID: string }>()
  const dispatch = useAppDispatch()
  const t = useT()
  const statusFilter = useAppSelector((state) => state.ui.statusFilter)
  const search = useAppSelector((state) => state.ui.search)

  const { data: board } = useGetBoardQuery(boardID ?? '', { skip: !boardID })
  const { data: taskData, isLoading } = useListTasksQuery(
    { boardID: boardID ?? '', statuses: statusFilter, search: search || undefined },
    { skip: !boardID },
  )
  const { data: depData } = useBoardDependenciesQuery(boardID ?? '', { skip: !boardID })

  const tasks = taskData ?? []
  const edges = depData?.edges ?? []

  // Card positions, measured. `useLayoutEffect` so the first paint already has
  // the lines; a `useEffect` here would show a frame with the cards and no edges.
  const canvasRef = useRef<HTMLDivElement>(null)
  const cardRefs = useRef(new Map<string, HTMLDivElement>())
  const [points, setPoints] = useState<Map<string, Point>>(new Map())
  const [canvasSize, setCanvasSize] = useState({ width: 0, height: 0 })

  const measure = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const base = canvas.getBoundingClientRect()
    const next = new Map<string, Point>()
    for (const [id, el] of cardRefs.current) {
      const rect = el.getBoundingClientRect()
      // Anchor points are the card's left and right mid-heights: an edge enters
      // the child on its left and leaves the parent on its right, so the two
      // ends never overlap the card text.
      next.set(id, {
        x: rect.left - base.left,
        y: rect.top - base.top + rect.height / 2,
        right: rect.right - base.left,
      } as Point & { right: number })
    }
    setPoints(next)
    setCanvasSize({ width: canvas.scrollWidth, height: canvas.scrollHeight })
  }, [])

  useLayoutEffect(measure, [measure, tasks, edges])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const observer = new ResizeObserver(measure)
    observer.observe(canvas)
    return () => observer.disconnect()
  }, [measure])

  const registerCard = useCallback((id: string, el: HTMLDivElement | null) => {
    if (el) cardRefs.current.set(id, el)
    else cardRefs.current.delete(id)
  }, [])

  const parentsOf = (taskID: string) => edges.filter((e) => e.child_id === taskID)
  // Grouping goes through `columnForStatus`, the same mapping the kanban uses —
  // NOT `task.status === column.key`. A column is a view of status (DECISIONS 3),
  // and `blocked`, `failed`, `cancelled` and `archived` have no column of their
  // own: they belong to `backlog` or `done`. Comparing status to key directly
  // silently DROPS those tasks from the screen — including every blocked one,
  // which is exactly the card a dependency view exists to show.
  const grouped = groupByColumn(tasks)
  const byColumn = (key: string) => grouped[key as keyof typeof grouped] ?? []
  const onBoard = new Set(tasks.map((task) => task.id))

  // Only edges whose BOTH ends are on screen get a line. The rest are listed on
  // the child (see the file header).
  const drawable = edges.filter((e) => onBoard.has(e.parent_id) && points.has(e.child_id) && points.has(e.parent_id))

  const columns = board?.columns ?? []

  return (
    <>
      <BoardToolbar boardName={board?.name} taskCount={tasks.length} />
      {isLoading ? (
        <div className="p-4">
          <SkeletonRows rows={5} columns={4} />
        </div>
      ) : (
        <div ref={canvasRef} className="relative flex min-h-0 flex-1 gap-2.5 overflow-x-auto p-4">
          {/* Edges sit under the cards: a line crossing a card would read as
              pointing at it. `pointer-events-none` keeps the cards clickable. */}
          <svg
            className="pointer-events-none absolute top-0 left-0 z-0"
            width={canvasSize.width}
            height={canvasSize.height}
            aria-hidden="true"
          >
            {drawable.map((edge) => (
              <Edge
                key={`${edge.parent_id}-${edge.child_id}`}
                edge={edge}
                from={points.get(edge.parent_id)!}
                to={points.get(edge.child_id)!}
              />
            ))}
          </svg>

          {columns.map((column) => {
            const lane = byColumn(column.key)
            return (
              <section key={column.key} className="relative z-10 flex w-[272px] min-w-[272px] flex-col gap-2">
                <header className="flex items-center gap-2 px-1">
                  <span
                    className="size-2 rounded-full"
                    style={{ background: statusColorVar(column.key as Task['status']) }}
                  />
                  <span className="font-mono text-[11px] font-bold uppercase text-[var(--color-secondary)]">
                    {column.name}
                  </span>
                  <span data-testid={`graph-count-${column.key}`} className="text-[11px] text-[var(--color-tertiary)]">
                    {lane.length}
                  </span>
                  <button
                    type="button"
                    onClick={() => dispatch(openCreateTask(column.key))}
                    title={t['graph.addTask']}
                    aria-label={t['graph.addTask']}
                    className="ml-auto flex size-5 items-center justify-center rounded-[4px] text-[var(--color-tertiary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]"
                  >
                    <Plus size={14} />
                  </button>
                </header>

                <div className="flex flex-col gap-2">
                  {lane.map((task) => {
                    const parents = parentsOf(task.id)
                    const offBoard = parents.filter((p) => !onBoard.has(p.parent_id))
                    return (
                      <div key={task.id} ref={(el) => registerCard(task.id, el)}>
                        <GraphCard
                          task={task}
                          parentCount={parents.length}
                          offBoard={offBoard}
                          onOpen={() => dispatch(openTask(task.id))}
                        />
                      </div>
                    )
                  })}
                </div>
              </section>
            )
          })}
        </div>
      )}
    </>
  )
}

/** One edge: a thin line from the parent's right edge to the child's left. */
function Edge({ edge, from, to }: { edge: BoardDependency; from: Point; to: Point }) {
  const start = from as Point & { right?: number }
  const x1 = start.right ?? start.x
  const y1 = start.y
  const x2 = to.x
  const y2 = to.y
  // A cubic with horizontal control points: the line leaves the parent and
  // enters the child level, which is what makes the direction readable.
  const dx = Math.max(24, Math.abs(x2 - x1) / 2)
  return (
    <path
      data-testid="graph-edge"
      data-parent={edge.parent_id}
      data-child={edge.child_id}
      d={`M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`}
      fill="none"
      stroke="var(--color-border-strong, rgba(12,26,22,0.28))"
      strokeWidth={1}
    />
  )
}

function GraphCard({
  task,
  parentCount,
  offBoard,
  onOpen,
}: {
  task: Task
  parentCount: number
  offBoard: BoardDependency[]
  onOpen: () => void
}) {
  const t = useT()
  // `block_kind` is the task's own field; the marker is never derived from the
  // edge count (a task whose parents are all done has edges and is not blocked).
  const locked = Boolean(task.block_kind)

  return (
    <div
      data-testid="graph-card"
      data-task-id={task.id}
      data-parent-count={parentCount}
      className={cn(
        'cursor-pointer rounded-[8px] border bg-[var(--color-surface-panel)] p-3 shadow-[0_1px_2px_rgba(12,26,22,0.04)]',
        locked
          ? 'border-[var(--color-warning,#d97706)]/40 bg-[var(--color-warning,#d97706)]/[0.04]'
          : 'border-[var(--color-border-subtle)]',
      )}
      onClick={onOpen}
    >
      <div className="flex items-start gap-2">
        <span
          className="mt-[3px] h-3 w-[3px] shrink-0 rounded-full"
          style={{ background: statusColorVar(task.status) }}
        />
        <p className="min-w-0 flex-1 text-[12px] leading-snug text-[var(--color-primary)]">{task.title}</p>
      </div>

      {parentCount > 0 || locked ? (
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {parentCount > 0 ? (
            <span
              data-testid="dep-badge"
              className="flex items-center gap-1 rounded-[4px] bg-[var(--color-surface-hover)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-secondary)]"
            >
              <Link2 size={11} />
              {interpolate(t['graph.nDeps'], [String(parentCount)])}
            </span>
          ) : null}
          {locked ? (
            <span
              data-testid="dep-locked"
              className="flex items-center gap-1 rounded-[4px] bg-[var(--color-warning,#d97706)]/10 px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-warning,#d97706)]"
            >
              <Lock size={11} />
              {t['graph.locked']}
            </span>
          ) : null}
        </div>
      ) : null}

      {offBoard.length > 0 ? (
        <ul data-testid="dep-offboard" className="mt-2 space-y-0.5 border-t border-[var(--color-border-subtle)] pt-2">
          {offBoard.map((edge) => (
            <li key={edge.parent_id} className="truncate text-[10px] text-[var(--color-tertiary)]">
              {t['graph.otherBoard']}: {edge.parent_title}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
