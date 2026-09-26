import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useListBoardsQuery, useListProjectsQuery } from '@/store/api/boards'
import { useBoardBudgetQuery, useBoardLedgerQuery } from '@/store/api/finops'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel, StatTile } from '@/components/ui/card'
import { budgetPercent, formatEstimatedMicroUSD, formatTokens, formatRelative } from '@/lib/formatters'
import type { Board } from '@/lib/domain'
import { SkeletonRows } from '@/components/ui/skeleton'

/**
 * Screens 33/34 — cost overview and ledger explorer.
 *
 * Cost is scoped to a board by the contract: the only aggregate endpoint is
 * `/boards/{id}/budget` and `/boards/{id}/ledger` (ARCHITECTURE 6.2.14). So this
 * screen picks a board and reports that board's real numbers, rather than
 * summing across boards on the client and calling it a workspace total.
 */
export function CostOverview() {
  const { orgID } = useParams<{ orgID: string }>()
  const { data: projects } = useListProjectsQuery()
  const [boardID, setBoardID] = useState<string | null>(null)

  return (
    <>
      <WorkspaceTopbar title="Cost & Usage" subtitle="per board" />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {(projects ?? []).length === 0 ? (
          <EmptyState title="No projects yet" hint="Cost is reported per board, so a board has to exist first." />
        ) : (
          <BoardPicker projects={projects ?? []} boardID={boardID} onPick={setBoardID} orgID={orgID ?? ''} />
        )}
      </div>
    </>
  )
}

function BoardPicker({
  projects,
  boardID,
  onPick,
  orgID,
}: {
  projects: { id: string; name: string }[]
  boardID: string | null
  onPick: (id: string) => void
  orgID: string
}) {
  return (
    <div className="flex flex-col gap-3">
      {projects.map((project) => (
        <ProjectBoardChoices
          key={project.id}
          projectID={project.id}
          projectName={project.name}
          boardID={boardID}
          onPick={onPick}
          orgID={orgID}
        />
      ))}
    </div>
  )
}

function ProjectBoardChoices({
  projectID,
  projectName,
  boardID,
  onPick,
  orgID,
}: {
  projectID: string
  projectName: string
  boardID: string | null
  onPick: (id: string) => void
  orgID: string
}) {
  const { data: boards } = useListBoardsQuery(projectID)
  if ((boards ?? []).length === 0) return null

  return (
    <section className="flex flex-col gap-1.5">
      <h2 className="text-[11px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">{projectName}</h2>
      <div className="flex flex-wrap gap-1.5">
        {(boards ?? []).map((board: Board) => (
          <button
            key={board.id}
            type="button"
            onClick={() => onPick(board.id)}
            className={
              board.id === boardID
                ? 'rounded-[6px] bg-[var(--color-accent)] px-2.5 py-1 text-[12px] font-medium text-[var(--color-on-accent)]'
                : 'rounded-[6px] border border-[var(--color-border-subtle)] px-2.5 py-1 text-[12px] text-[var(--color-secondary)] hover:text-[var(--color-primary)]'
            }
          >
            {board.name}
          </button>
        ))}
      </div>
      {boardID ? <BoardCost boardID={boardID} orgID={orgID} /> : null}
    </section>
  )
}

function BoardCost({ boardID }: { boardID: string; orgID: string }) {
  const { data: budget } = useBoardBudgetQuery(boardID)
  const { data: ledger, isLoading } = useBoardLedgerQuery(boardID)

  if (!budget) {
    return <p className="text-[12px] text-[var(--color-tertiary)]">No usage recorded for this board yet.</p>
  }

  const percent = budgetPercent(budget.spent_micros, budget.budget_daily_micros)

  return (
    <>
      <div className="grid grid-cols-4 gap-2.5">
        <StatTile label="Spent today" value={formatEstimatedMicroUSD(budget.spent_micros)} accent />
        <StatTile label="Daily cap" value={formatEstimatedMicroUSD(budget.budget_daily_micros)} />
        <StatTile
          label="Used"
          value={`${percent.toFixed(1)}%`}
          hint={budget.threshold_crossed ? 'over the 80% alert (N18)' : undefined}
        />
        <StatTile label="Runs" value={String(budget.run_count)} />
      </div>

      <Panel className="mt-2.5">
        <div className="border-b border-[var(--color-border-subtle)] px-3 py-2 text-[11px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
          Ledger
        </div>
        {isLoading ? (
          <SkeletonRows rows={4} columns={4} />
        ) : (ledger?.entries ?? []).length === 0 ? (
          <p className="p-3 text-[12px] text-[var(--color-tertiary)]">No ledger entries yet.</p>
        ) : (
          <table className="w-full border-collapse">
            <tbody>
              {(ledger?.entries ?? []).map((entry) => (
                <tr key={entry.id} className="h-8 border-b border-[var(--color-border-subtle)] last:border-b-0">
                  <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">
                    {entry.provider}/{entry.model}
                  </td>
                  <td className="px-3 font-mono text-[11px] text-[var(--color-tertiary)]">
                    {formatTokens(entry.tokens_in)} in / {formatTokens(entry.tokens_out)} out
                  </td>
                  <td className="px-3 text-right font-mono text-[11px] text-[var(--color-primary)]">
                    {formatEstimatedMicroUSD(entry.cost_micros)}
                  </td>
                  <td className="px-3 text-right font-mono text-[10px] text-[var(--color-quaternary)]">
                    {formatRelative(entry.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </>
  )
}
