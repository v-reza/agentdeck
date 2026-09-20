import { Link, useParams } from 'react-router-dom'
import { useGetProjectQuery, useListBoardsQuery } from '@/store/api/boards'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel } from '@/components/ui/card'
import { formatMicroUSD, formatRelative } from '@/lib/formatters'

/**
 * One project and its boards (screen 10-board-list). Boards are the unit that
 * owns a budget cap, so the board's cap is shown here — it is the only cost
 * figure the contract can answer for a project without inventing an aggregate.
 */
export function ProjectDetail() {
  const { orgID, projectID } = useParams<{ orgID: string; projectID: string }>()
  const { data: project } = useGetProjectQuery(projectID ?? '', { skip: !projectID })
  const { data: boards, isLoading } = useListBoardsQuery(projectID ?? '', { skip: !projectID })

  return (
    <>
      <WorkspaceTopbar title={project?.name ?? 'Project'} subtitle={project?.slug} />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {isLoading ? (
          <p className="font-mono text-[12px] text-[var(--color-tertiary)]">Loading…</p>
        ) : (boards ?? []).length === 0 ? (
          <EmptyState title="No boards in this project" hint="A board holds the tasks and the daily budget cap." />
        ) : (
          <div className="grid grid-cols-2 gap-2.5">
            {(boards ?? []).map((board) => (
              <Panel key={board.id} className="p-3">
                <Link
                  to={`/app/${orgID}/boards/${board.id}`}
                  className="text-[13px] font-semibold text-[var(--color-primary)] hover:text-[var(--color-accent)]"
                >
                  {board.name}
                </Link>
                <div className="mt-1 flex items-center gap-3 font-mono text-[11px] text-[var(--color-tertiary)]">
                  <span>{board.slug}</span>
                  <span>cap {formatMicroUSD(board.budget_daily_micros)}</span>
                  <span>{formatRelative(board.created_at)}</span>
                </div>
                <div className="mt-2 flex gap-1">
                  {board.columns.map((column) => (
                    <span
                      key={column.key}
                      className="rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-secondary)]"
                    >
                      {column.name}
                    </span>
                  ))}
                </div>
              </Panel>
            ))}
          </div>
        )}
      </div>
    </>
  )
}
