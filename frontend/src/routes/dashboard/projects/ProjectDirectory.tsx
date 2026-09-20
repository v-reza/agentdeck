import { ChevronDown, Search, SlidersHorizontal } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'
import { useListProjectsQuery } from '@/store/api/boards'
import { useProjectBoards, useBoardTasks } from '@/hooks/use-directory'
import { useBoardTaskStats } from '@/hooks/use-directory-totals'
import type { BoardTaskStats } from '@/store/slices/directorySlice'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import {
  setDirectorySearch,
  setDirectorySort,
  toggleDirectoryCollapsed,
  type DirectorySortKey,
} from '@/store/slices/uiSlice'
import { EmptyState } from '@/components/ui/card'
import { formatMicroUSD, formatRelative, plural } from '@/lib/formatters'
import { cn } from '@/lib/cn'
import type { Board, Project } from '@/lib/domain'

/**
 * The grouped project directory (design source 11-project-list "MAIN PROJECTS &
 * BOARDS TABLE"): a 32px header, a group row per project, and a 28px row per
 * board, with the density contract the source states in the toolbar.
 *
 * Structure follows the source exactly — checkbox column, Project/Board Name,
 * Parent Project, Tasks, Running Tasks, Cost Today, Action — because the table
 * *is* the screen. Sortable columns are wired to `uiSlice` (US-AD91 AC2), and
 * the search box narrows the rows client-side on data the server already sent.
 *
 * Cost and task counts come from endpoints that exist; `GET /boards/{id}/budget`
 * is M2 and answers 404, so the Cost column renders the empty marker rather than
 * a zero that would read as "this board is free".
 */
export function ProjectDirectory() {
  const { orgID } = useParams<{ orgID: string }>()
  const dispatch = useAppDispatch()
  const search = useAppSelector((state) => state.ui.directorySearch)
  const sort = useAppSelector((state) => state.ui.directorySort)
  const collapsed = useAppSelector((state) => state.ui.directoryCollapsed)

  const { data, isLoading, error } = useListProjectsQuery()

  if (isLoading) return <DirectoryNote text="Loading projects…" />
  if (error) return <EmptyState title="Could not load projects" hint="The API rejected the request." />

  const projects = data ?? []
  const visible = sortProjects(filterProjects(projects, search), sort)

  return (
    <div className="overflow-hidden rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] shadow-sm">
      <div className="flex h-10 items-center justify-between border-b border-[var(--color-border-subtle)] px-4">
        <div className="flex items-center gap-3">
          <span className="text-[12px] font-semibold text-[var(--color-primary)]">
            Project Directory &amp; Child Boards
          </span>
          <span className="rounded-[4px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 py-0.5 font-mono text-[11px] text-[var(--color-tertiary)]">
            {visible.length} projects
          </span>
        </div>

        <div className="flex items-center gap-3 font-mono text-[11px] text-[var(--color-tertiary)]">
          <span className="flex items-center gap-1">
            <span className="h-2 w-2 rounded-full bg-[var(--color-status-done)]" />
            <span>Workspace Auto-Bind: Active</span>
          </span>
          <span className="text-[var(--color-border-standard)]">|</span>
          <span className="font-medium text-[var(--color-accent)]">Density: 28px row / 32px header</span>
        </div>
      </div>

      <div className="flex h-10 items-center justify-between gap-3 border-b border-[var(--color-border-subtle)] px-4">
        <div className="flex h-7 w-[240px] items-center gap-1.5 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2">
          <Search size={12} className="shrink-0 text-[var(--color-tertiary)]" />
          <input
            type="search"
            value={search}
            onChange={(event) => dispatch(setDirectorySearch(event.target.value))}
            placeholder="Filter projects and boards…"
            aria-label="Filter projects and boards"
            className="h-full w-full bg-transparent font-mono text-[11px] text-[var(--color-primary)] outline-none placeholder:text-[var(--color-tertiary)]"
          />
        </div>

        <button
          type="button"
          onClick={() => dispatch(setDirectorySort('tasks'))}
          className="inline-flex h-7 items-center gap-1.5 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-2.5 font-mono text-[11px] text-[var(--color-secondary)] transition-colors hover:text-[var(--color-primary)]"
        >
          <SlidersHorizontal size={12} />
          <span>Sort &amp; Filter</span>
        </button>
      </div>

      {visible.length === 0 ? (
        <div className="p-4">
          <EmptyState
            title={projects.length === 0 ? 'No projects yet' : 'Nothing matches that filter'}
            hint={
              projects.length === 0 ? 'Create the first one with New project.' : 'Clear the search to see them again.'
            }
          />
        </div>
      ) : (
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="h-[32px] border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] font-mono text-[11px] uppercase tracking-wider text-[var(--color-tertiary)]">
              <th className="w-7 px-3 text-center">
                <span className="sr-only">Select</span>
              </th>
              <th className="px-3 font-semibold text-[var(--color-primary)]">
                <SortHeader label="Project / Board Name" sortKey="name" />
              </th>
              <th className="px-3 font-semibold">Parent Project</th>
              <th className="px-3 text-right font-semibold">
                <SortHeader label="Tasks" sortKey="tasks" align="right" />
              </th>
              <th className="px-3 font-semibold">Running Tasks</th>
              <th className="px-3 text-right font-semibold">
                <SortHeader label="Cost Today" sortKey="cost" align="right" />
              </th>
              <th className="px-3 text-right font-semibold">Action</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[rgba(12,26,22,0.05)] text-[12px]">
            {visible.map((project) => (
              <ProjectGroup
                key={project.id}
                project={project}
                orgID={orgID ?? ''}
                collapsed={collapsed.includes(project.id)}
                sort={sort}
              />
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function SortHeader({ label, sortKey, align }: { label: string; sortKey: DirectorySortKey; align?: 'right' }) {
  const dispatch = useAppDispatch()
  const sort = useAppSelector((state) => state.ui.directorySort)
  const active = sort.key === sortKey

  return (
    <button
      type="button"
      onClick={() => dispatch(setDirectorySort(sortKey))}
      title={`Sort by ${label} (US-AD91 AC2)`}
      className={cn(
        'inline-flex items-center gap-1 uppercase transition-colors hover:text-[var(--color-primary)]',
        align === 'right' && 'flex-row-reverse',
        active && 'font-bold text-[var(--color-accent)]',
      )}
    >
      <span>{label}</span>
      <ChevronDown size={12} className={cn(active && sort.direction === 'asc' && 'rotate-180')} />
    </button>
  )
}

function ProjectGroup({
  project,
  orgID,
  collapsed,
  sort,
}: {
  project: Project
  orgID: string
  collapsed: boolean
  sort: { key: DirectorySortKey; direction: 'asc' | 'desc' }
}) {
  const dispatch = useAppDispatch()
  const { boards, isUnresolved } = useProjectBoards(project.id)
  const stats = useBoardTaskStats()

  return (
    <>
      <tr className="border-y border-[rgba(12,26,22,0.08)] bg-[rgba(13,122,112,0.03)] font-medium">
        <td className="px-3 py-1.5 text-center">
          <button
            type="button"
            onClick={() => dispatch(toggleDirectoryCollapsed(project.id))}
            aria-expanded={!collapsed}
            aria-label={collapsed ? `Expand ${project.name}` : `Collapse ${project.name}`}
            className="text-[var(--color-accent)]"
          >
            <ChevronDown size={14} className={cn('transition-transform', collapsed && '-rotate-90')} />
          </button>
        </td>
        <td className="px-3 py-1.5" colSpan={2}>
          <div className="flex items-center gap-2">
            <Link
              to={`/app/${orgID}/projects/${project.id}`}
              className="font-mono text-[13px] font-bold text-[var(--color-primary)] hover:text-[var(--color-accent)]"
            >
              {project.name}
            </Link>
            <span className="rounded-[4px] border border-[rgba(12,26,22,0.08)] bg-[var(--color-surface-panel)] px-1.5 font-mono text-[10px] text-[var(--color-secondary)]">
              {isUnresolved ? '–' : plural(boards.length, 'board')}
            </span>
            <span className="truncate font-mono text-[11px] font-normal text-[var(--color-tertiary)]">
              — {project.slug}
            </span>
          </div>
        </td>
        <td className="px-3 py-1.5 text-right font-mono font-semibold text-[var(--color-primary)] tabular-nums">
          {isUnresolved ? '–' : <ProjectTaskTotal boards={boards} />}
        </td>
        <td className="px-3 py-1.5">
          <ProjectRunningTotal boards={boards} />
        </td>
        <td className="px-3 py-1.5 text-right font-mono font-bold text-[var(--color-accent)] tabular-nums">—</td>
        <td className="px-3 py-1.5 text-right">
          <Link
            to={`/app/${orgID}/projects/${project.id}`}
            className="font-mono text-[11px] font-medium text-[var(--color-accent)] hover:underline"
          >
            View boards →
          </Link>
        </td>
      </tr>

      {collapsed ? null : boards.length === 0 ? (
        <tr className="h-[28px]">
          <td />
          <td className="px-3 pl-8 font-mono text-[11px] text-[var(--color-tertiary)]" colSpan={6}>
            {isUnresolved ? 'Loading boards…' : 'No boards in this project yet.'}
          </td>
        </tr>
      ) : (
        sortBoards(boards, sort, stats).map((board) => (
          <BoardRow key={board.id} board={board} project={project} orgID={orgID} />
        ))
      )}
    </>
  )
}

/**
 * Sum of what the board rows already reported, read in ONE selector.
 *
 * The map is read once and then reduced with plain arithmetic. Calling a hook
 * per board inside `boards.map()` would break the rules of hooks as soon as a
 * project's board count changed between renders.
 *
 * A board whose row has not answered is absent from the map, so it is left out
 * of the sum rather than counted as zero — the total is never larger than what
 * the server actually said.
 */
function ReportedTotals({ boards, field }: { boards: Board[]; field: 'tasks' | 'running' }) {
  const stats = useBoardTaskStats()
  const reported = boards.map((board) => stats[board.id]).filter((entry) => entry !== undefined)

  if (field === 'tasks') {
    if (reported.length === 0) return <>–</>
    return <>{reported.reduce((sum, entry) => sum + entry.tasks, 0)}</>
  }

  if (reported.length === 0) return <span className="font-mono text-[10px] text-[var(--color-tertiary)]">—</span>
  const running = reported.reduce((sum, entry) => sum + entry.running, 0)
  if (running === 0) return <span className="font-mono text-[10px] text-[var(--color-tertiary)]">0 running</span>
  return (
    <span className="inline-flex items-center gap-1 rounded-[4px] bg-[var(--color-warning)]/10 px-1.5 py-0.5 font-mono text-[11px] font-medium text-[var(--color-warning)]">
      <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-warning)]" />
      {running} running
    </span>
  )
}

/** Group-row task cell: a real 0 when the project has no boards, else the sum. */
function ProjectTaskTotal({ boards }: { boards: Board[] }) {
  if (boards.length === 0) return <>0</>
  return <ReportedTotals boards={boards} field="tasks" />
}

/** Group-row running cell. */
function ProjectRunningTotal({ boards }: { boards: Board[] }) {
  if (boards.length === 0) return <span className="font-mono text-[10px] text-[var(--color-tertiary)]">0 running</span>
  return <ReportedTotals boards={boards} field="running" />
}

function BoardRow({ board, project, orgID }: { board: Board; project: Project; orgID: string }) {
  const { tasks, running, isUnresolved } = useBoardTasks(board.id)

  return (
    <tr className="h-[28px] transition-colors hover:bg-[var(--color-surface-page)]">
      <td className="px-3 text-center">
        <input
          type="checkbox"
          aria-label={`Select ${board.name}`}
          className="rounded-[3px] border-[var(--color-border-strong)] accent-[var(--color-accent)]"
        />
      </td>
      <td className="px-3 pl-8">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              'h-2 w-2 shrink-0 rounded-full',
              running > 0 ? 'bg-[var(--color-warning)]' : 'bg-[var(--color-quaternary)]',
            )}
          />
          <Link
            to={`/app/${orgID}/boards/${board.id}`}
            className="truncate font-medium text-[var(--color-primary)] transition-colors hover:text-[var(--color-accent)]"
          >
            {board.name}
          </Link>
        </div>
      </td>
      <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">{project.name}</td>
      <td className="px-3 text-right font-mono text-[11px] text-[var(--color-secondary)] tabular-nums">
        {isUnresolved ? '–' : tasks.length}
      </td>
      <td className="px-3">
        {isUnresolved ? (
          <span className="font-mono text-[10px] text-[var(--color-tertiary)]">–</span>
        ) : running === 0 ? (
          <span className="font-mono text-[10px] text-[var(--color-tertiary)]">0 running</span>
        ) : (
          <span className="inline-flex items-center gap-1 rounded-[4px] bg-[var(--color-warning)]/10 px-1.5 py-0.2 font-mono text-[10px] font-medium text-[var(--color-warning)]">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-warning)]" />
            {running} running
          </span>
        )}
      </td>
      <td className="px-3 text-right font-mono text-[11px] tabular-nums">
        {/* GET /boards/{id}/budget is M2; until it exists the cell says so
            instead of printing $0.00, which would read as "free". */}
        <span className="text-[var(--color-tertiary)]" title="Per-board spend arrives with the M2 cost endpoints">
          —
        </span>
      </td>
      <td className="px-3 text-right">
        <Link
          to={`/app/${orgID}/boards/${board.id}`}
          className="font-mono text-[11px] text-[var(--color-accent)] hover:underline"
        >
          Open →
        </Link>
      </td>
    </tr>
  )
}

function DirectoryNote({ text }: { text: string }) {
  return <p className="p-4 font-mono text-[12px] text-[var(--color-tertiary)]">{text}</p>
}

/** Case-insensitive match on project name/slug; a matching project keeps all its boards. */
function filterProjects(projects: Project[], search: string): Project[] {
  const needle = search.trim().toLowerCase()
  if (!needle) return projects
  return projects.filter(
    (project) => project.name.toLowerCase().includes(needle) || project.slug.toLowerCase().includes(needle),
  )
}

/** Projects sort by name; US-AD91 AC2. */
function sortProjects(projects: Project[], sort: DirectorySort): Project[] {
  const factor = sort.direction === 'asc' ? 1 : -1
  return [...projects].sort((a, b) => factor * a.name.localeCompare(b.name))
}

/**
 * US-AD91 AC2. Boards are sorted *inside* their group, so grouping survives the
 * sort.
 *
 * `tasks` ranks by the count the board's own row reported; a board that has not
 * answered yet has no stats entry and ranks last rather than being treated as a
 * zero, which would silently sort "unknown" alongside "empty". `cost` has no
 * server value until M2, so it falls back to name — the column is still
 * clickable, it just cannot rank by a number that does not exist yet.
 */
function sortBoards(boards: Board[], sort: DirectorySort, stats: Record<string, BoardTaskStats>): Board[] {
  const factor = sort.direction === 'asc' ? 1 : -1
  const byName = (a: Board, b: Board) => factor * a.name.localeCompare(b.name)

  if (sort.key !== 'tasks') return [...boards].sort(byName)

  return [...boards].sort((a, b) => {
    const left = stats[a.id]
    const right = stats[b.id]
    if (!left && !right) return byName(a, b)
    if (!left) return 1
    if (!right) return -1
    if (left.tasks === right.tasks) return byName(a, b)
    return factor * (left.tasks - right.tasks)
  })
}

interface DirectorySort {
  key: DirectorySortKey
  direction: 'asc' | 'desc'
}

export { formatMicroUSD, formatRelative }
