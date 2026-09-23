import { Link, useParams } from 'react-router-dom'
import { useListProjectsQuery } from '@/store/api/boards'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState } from '@/components/ui/card'
import { CreateBoardForm } from './CreateBoardForm'
import { useProjectBoards, useBoardTasks } from '@/hooks/use-directory'
import { useBoardTaskStats, useDirectoryTotals } from '@/hooks/use-directory-totals'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'
import { setBoardSort, type BoardSortKey, type BoardTaskStats } from '@/store/slices/directorySlice'
import { plural } from '@/lib/formatters'
import type { Board } from '@/lib/domain'
import { SkeletonRows } from '@/components/ui/skeleton'

/**
 * Screen 10-board-list — every board in the workspace as one dense table
 * (US-AD91 AC1: name, parent project, task count, running count, cost today).
 *
 * The grouping the design shows is a project *column*, not nested sections, so
 * the table is flat and sortable — which is what AC2 asks for ("dapat diurutkan
 * berdasarkan biaya atau jumlah task"). Sorting by cost needs every row's cost,
 * and RTK Query ships no `useQueries`, so the per-row hooks report into
 * `directorySlice` and the sort reads that projection.
 *
 * Rows are grouped under their project for display, but the sort is applied
 * across the whole workspace, so "highest cost today" answers the question for
 * the workspace rather than per project.
 */
export function BoardList() {
  const { orgID } = useParams<{ orgID: string }>()
  const { data: projects, isLoading } = useListProjectsQuery()
  const t = useT()
  const list = projects ?? []

  if (isLoading) {
    return (
      <>
        <WorkspaceTopbar title="Boards" path="/boards" />
        <SkeletonRows rows={4} columns={4} />
      </>
    )
  }

  return (
    <>
      <WorkspaceTopbar
        title="Boards"
        path="/boards"
        right={
          <>
            <BoardCountChip count={list.length} />
            {/* US-AD09: the design's toolbar CTA. Rendered in the topbar's
                action slot so it sits where the design puts it. */}
            <CreateBoardForm projects={list.map((p) => ({ id: p.id, name: p.name }))} />
          </>
        }
      />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <IsolationBanner />

        {list.length === 0 ? (
          <EmptyState title={t['boards.noProjects']} hint={t['boards.noProjectsHint']} />
        ) : (
          <BoardTable projects={list.map((p) => ({ id: p.id, name: p.name, slug: p.slug }))} orgID={orgID ?? ''} />
        )}
      </div>
    </>
  )
}

function BoardCountChip({ count }: { count: number }) {
  return (
    <span className="rounded-[4px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-secondary)]">
      {plural(count, 'project')} in scope
    </span>
  )
}

/**
 * The design puts an explicit isolation notice above the table. It is not
 * decoration: it states which workspace the rows belong to, which is what makes
 * a cross-workspace leak visible to the operator (US-AD91 AC4).
 */
function IsolationBanner() {
  const workspace = useAppSelector((s) => s.session.workspaces.find((w) => w.id === s.session.activeOrgID))
  return (
    <div className="flex items-center justify-between gap-3 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-3 py-2 text-[11px] text-[var(--color-secondary)]">
      {/* `min-w-0` + `break-all`: a workspace slug is derived from the email
          (internal/auth workspaceSlugFrom), so it can be a 40-character token
          with no space in it. Without these a flex item refuses to shrink below
          its content width and the slug pushes the banner past its container. */}
      <span className="min-w-0">
        Showing boards scoped strictly to current workspace{' '}
        <strong className="font-mono break-all text-[var(--color-primary)]">{workspace?.slug ?? '—'}</strong>.
        Cross-workspace access restricted.
      </span>
      <SortChip />
    </div>
  )
}

/** Mirrors the banner's "Sort active: Cost today (desc)" readout. */
function SortChip() {
  const sort = useAppSelector((s) => s.directory.boardSort)
  const labels: Record<BoardSortKey, string> = { name: 'Board name', tasks: 'Tasks', cost: 'Cost today' }
  return (
    <span className="text-[var(--color-tertiary)]">
      Sort active: <span className="font-mono font-medium text-[var(--color-accent)]">{labels[sort.key]}</span>{' '}
      <span className="font-mono">({sort.direction})</span>
    </span>
  )
}

/**
 * Order two boards for the active sort column.
 *
 * A value the server has not answered yet is `undefined`, and undefined always
 * sorts last regardless of direction — a board whose tasks have not been read is
 * not a board with zero tasks, so it must never win "fewest tasks".
 *
 * Cost today comes from `GET /boards/{id}/budget`, which lands in M2, so every
 * cost is currently unknown. Rather than present a cost sort that silently does
 * nothing, the comparator falls back to the board name, which keeps the table
 * deterministic and the "Sort active" readout honest.
 */
function compareBoards(
  a: Board,
  b: Board,
  sort: { key: BoardSortKey; direction: 'asc' | 'desc' },
  stats: Record<string, BoardTaskStats>,
): number {
  const dir = sort.direction === 'asc' ? 1 : -1

  if (sort.key === 'cost') return a.name.localeCompare(b.name) * dir
  if (sort.key === 'name') return a.name.localeCompare(b.name) * dir

  const av = stats[a.id]?.tasks
  const bv = stats[b.id]?.tasks
  if (av === undefined && bv === undefined) return a.name.localeCompare(b.name) * dir
  if (av === undefined) return 1
  if (bv === undefined) return -1
  return (av - bv) * dir
}

interface ProjectRef {
  id: string
  name: string
  slug: string
}

function BoardTable({ projects, orgID }: { projects: ProjectRef[]; orgID: string }) {
  // One hook call per project, at a fixed position — the row components each
  // fetch their own boards and report counts into the projection.
  return (
    <div className="overflow-hidden rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)]">
      <table className="w-full border-collapse text-left">
        <thead>
          <tr className="h-8 border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] text-[11px] font-semibold text-[var(--color-tertiary)] select-none">
            <Th className="w-8 text-center font-normal">
              <input type="checkbox" aria-label="Select all boards" className="h-3.5 w-3.5 rounded-[3px]" />
            </Th>
            <SortHeader label="Board Name" sortKey="name" />
            <Th>Project</Th>
            <SortHeader label="Tasks" sortKey="tasks" align="right" />
            <Th className="pl-6">Running Tasks</Th>
            <SortHeader label="Cost Today" sortKey="cost" align="right" />
            <Th className="w-20 text-right">Action</Th>
          </tr>
        </thead>
        <tbody className="text-[12px]">
          {projects.map((project) => (
            <ProjectRows key={project.id} project={project} orgID={orgID} />
          ))}
        </tbody>
      </table>
      <TableFooter />
    </div>
  )
}

/**
 * A project's rows. The project's boards are fetched here (one stable cache key
 * per project, never a hook inside `map`) and every board's tasks are read by
 * `BoardRow`, which reports the counts the header sorts on.
 */
function ProjectRows({ project, orgID }: { project: ProjectRef; orgID: string }) {
  const { boards, isUnresolved } = useProjectBoards(project.id)
  const sort = useAppSelector((s) => s.directory.boardSort)
  const stats = useBoardTaskStats()
  const t = useT()

  if (isUnresolved) {
    return (
      <tr className="h-8 border-b border-[var(--color-border-subtle)]">
        <td colSpan={7} className="px-3 font-mono text-[11px] text-[var(--color-tertiary)]">
          Loading boards of {project.name}…
        </td>
      </tr>
    )
  }

  if (boards.length === 0) {
    return (
      <tr className="h-8 border-b border-[var(--color-border-subtle)]">
        <td colSpan={7} className="px-3">
          <div className="flex items-center justify-between gap-3">
            <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
              {t['boards.empty']} — {project.name}
            </span>
            {/* US-AD91 AC3: a workspace with no boards gets a CTA, not a bare
                table. Scoped to this project so the form opens with the right
                parent preselected. */}
            <CreateBoardForm projects={[{ id: project.id, name: project.name }]} label={t['boards.createFirst']} />
          </div>
        </td>
      </tr>
    )
  }

  const ordered = [...boards].sort((a, b) => compareBoards(a, b, sort, stats))

  return (
    <>
      {ordered.map((board) => (
        <BoardRow key={board.id} board={board} project={project} orgID={orgID} />
      ))}
    </>
  )
}

function BoardRow({ board, project, orgID }: { board: Board; project: ProjectRef; orgID: string }) {
  // Each row reads its own board's tasks and reports the counts upward, so the
  // table can sort and total without a second, invented source of truth.
  const { running, isUnresolved } = useBoardTasks(board.id)
  const stats = useBoardTaskStats()
  const mine = stats[board.id]

  return (
    <tr className="h-8 border-b border-[var(--color-border-subtle)] last:border-b-0 hover:bg-[var(--color-surface-page)]">
      <td className="px-3 text-center">
        <input type="checkbox" aria-label={`Select ${board.name}`} className="h-3.5 w-3.5 rounded-[3px]" />
      </td>
      <td className="px-3 font-medium text-[var(--color-primary)]">
        <div className="flex items-center gap-2">
          <span
            className={
              running > 0
                ? 'h-2 w-2 rounded-full bg-[var(--color-status-running)]'
                : 'h-2 w-2 rounded-full bg-[var(--color-tertiary)]'
            }
          />
          <Link to={`/app/${orgID}/boards/${board.id}`} className="font-semibold hover:underline">
            {board.name}
          </Link>
        </div>
      </td>
      <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">{project.slug}</td>
      <td className="px-3 text-right font-mono font-medium tabular-nums text-[var(--color-primary)]">
        {mine === undefined ? '–' : mine.tasks}
      </td>
      <td className="px-3 pl-6">
        {isUnresolved ? (
          <span className="font-mono text-[11px] text-[var(--color-tertiary)]">–</span>
        ) : running > 0 ? (
          <span className="inline-flex items-center gap-1 rounded-[4px] bg-[var(--color-status-running)]/10 px-1.5 py-0.5 font-mono text-[11px] font-medium text-[var(--color-status-running)]">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-[var(--color-status-running)]" />
            {running} running
          </span>
        ) : (
          <span className="rounded bg-[var(--color-surface-page)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-tertiary)]">
            0 running
          </span>
        )}
      </td>
      <td className="px-3 text-right font-mono font-semibold tabular-nums text-[var(--color-accent)]">
        {/* Cost today comes from GET /boards/{id}/budget, which is M2. Until then
            the cell shows a dash rather than a fabricated $0.00. */}
        –
      </td>
      <td className="px-3 text-right">
        <Link
          to={`/app/${orgID}/boards/${board.id}`}
          className="font-mono text-[11px] font-medium text-[var(--color-accent)] hover:underline"
        >
          Open →
        </Link>
      </td>
    </tr>
  )
}

function TableFooter() {
  const { boards, tasks, running } = useDirectoryTotals()
  return (
    <div className="flex h-8 items-center justify-between border-t border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-4 font-mono text-[11px] text-[var(--color-tertiary)]">
      <div className="flex items-center gap-3">
        <span>{plural(boards, 'board')} total</span>
        <span>•</span>
        <span>{plural(tasks, 'task')}</span>
      </div>
      <div className="flex items-center gap-4">
        <span>
          Total running:{' '}
          <strong className="font-semibold text-[var(--color-status-running)]">{plural(running, 'task')}</strong>
        </span>
        <span>
          Total today: <strong className="font-semibold text-[var(--color-accent)]">–</strong>
        </span>
      </div>
    </div>
  )
}

function SortHeader({ label, sortKey, align }: { label: string; sortKey: BoardSortKey; align?: 'right' }) {
  const dispatch = useAppDispatch()
  const sort = useAppSelector((s) => s.directory.boardSort)
  const active = sort.key === sortKey
  const glyph = !active ? '↕' : sort.direction === 'asc' ? '↑' : '↓'

  return (
    <th className={['px-3 py-0 font-medium', align === 'right' ? 'text-right' : ''].join(' ')}>
      <button
        type="button"
        onClick={() => dispatch(setBoardSort(sortKey))}
        aria-label={`Sort by ${label}`}
        aria-sort={active ? (sort.direction === 'asc' ? 'ascending' : 'descending') : 'none'}
        className={[
          'inline-flex items-center gap-1 hover:text-[var(--color-primary)]',
          align === 'right' ? 'justify-end' : '',
          active ? 'text-[var(--color-accent)]' : '',
        ].join(' ')}
      >
        <span>{label}</span>
        <span aria-hidden="true" className="text-[10px]">
          {glyph}
        </span>
      </button>
    </th>
  )
}

function Th({ children, className }: { children?: React.ReactNode; className?: string }) {
  return <th className={['px-3 py-0 font-medium', className ?? ''].join(' ')}>{children}</th>
}
