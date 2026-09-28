import { ArrowRight, CircleAlert, Clock, DollarSign, FolderKanban, Inbox } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel, StatTile } from '@/components/ui/card'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useApprovalInboxQuery, useSearchRunsQuery } from '@/store/api/dashboard'
import { useOrgCostSummaryQuery } from '@/store/api/finops'
import { useListProjectsQuery } from '@/store/api/boards'
import { useT } from '@/hooks/use-t'
import { useAppSelector } from '@/store/hooks'
import { formatMicroUSD } from '@/lib/formatters'
import { formatDateTime } from '@/lib/format'

/**
 * Screen: dashboard — `design/stitch-output/v2/42-dashboard.html`, US-AD76.
 *
 * AC1 asks for four metric cards and a 7-day cost chart. What the contract can
 * actually answer was measured against the running API, not assumed:
 *
 *  - **"Task per status" has no org-wide source.** `GET /search/tasks` requires a
 *    non-empty `q` (verified: `?q=` and `?limit=` alone both answer 400), and the
 *    only task list is `GET /boards/{id}/tasks`, which is per board.
 *  - **"Agent aktif" has no org-wide source either.** `GET /search/runs` has no
 *    `status` filter, and `runs.outcome` is NULL for the whole life of a running
 *    run — `runs_outcome_chk` does not even admit 'running', because `outcome` is
 *    written by `EndRun`. So `?outcome=running` answers `{"runs":[]}` forever.
 *    (The cost rail's "agents active" is the board's own task count, which is why
 *    it works there and not here.)
 *  - **The 7-day chart has no series.** `cost-summary` is a rolling 30-day total;
 *    the only per-day data is `boards/{id}/budget` (one board, today).
 *
 * Inventing an endpoint for any of those is a contract change, so this screen
 * answers what the API can answer and names what it cannot, in
 * `dashboard.noChartNote` and `dashboard.scopeNote`. The four cards are: failed
 * runs in 24h, rolling 30-day spend, pending approvals, and projects.
 *
 * AC3 is in `title` below: a user with exactly one workspace is addressed by
 * their own name, because for them the workspace name is the product's internal
 * label for their own account. AC4 holds by omission — this screen has no
 * members, invites, or org chrome to hide.
 */
export function Dashboard() {
  const t = useT()
  const { orgID } = useParams<{ orgID: string }>()
  const userName = useAppSelector((state) => state.session.name)
  const workspaces = useAppSelector((state) => state.session.workspaces)

  const failed = useSearchRunsQuery({ outcome: 'failed', limit: 100 })
  const cost = useOrgCostSummaryQuery(orgID ?? '', { skip: !orgID })
  const approvals = useApprovalInboxQuery()
  const projects = useListProjectsQuery(undefined, { skip: !orgID })

  // AC3: one workspace means the workspace is this person, so the heading is
  // their name. More than one and the workspace name is the meaningful one.
  const soloWorkspace = workspaces.length === 1
  const activeWorkspace = workspaces.find((w) => w.id === orgID)
  const title = soloWorkspace && userName ? userName : (activeWorkspace?.name ?? t['dashboard.title'])

  const failedRecent = (failed.data ?? []).filter((run) => withinHours(run.ended_at || run.started_at, 24))
  const spend = cost.data

  return (
    <>
      <WorkspaceTopbar title={title} path="/dashboard" subtitle={t['dashboard.title']} />

      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {/* Satu baris yang menyebut apa yang tidak bisa dijawab kontrak, supaya
            kartu yang tidak ada tidak terbaca sebagai "nol". */}
        <p data-testid="dashboard-scope-note" className="text-[11px] text-[var(--color-tertiary)]">
          {t['dashboard.scopeNote']}
        </p>

        <div className="grid grid-cols-4 gap-3">
          <div data-testid="metric-failed">
            <StatTile
              label={t['dashboard.failed']}
              value={failed.isLoading ? '—' : String(failedRecent.length)}
              hint={t['dashboard.failedHint']}
            />
          </div>
          <div data-testid="metric-projects">
            <StatTile
              label={t['dashboard.projects']}
              value={projects.isLoading ? '—' : String(projects.data?.length ?? 0)}
              hint={t['dashboard.projectsHint']}
            />
          </div>
          <div data-testid="metric-cost">
            <StatTile
              label={t['dashboard.cost']}
              value={cost.isLoading ? '—' : formatMicroUSD(spend?.total_micros ?? 0)}
              hint={t['dashboard.costHint']}
            />
          </div>
          <div data-testid="metric-approvals">
            <StatTile
              label={t['dashboard.approvals']}
              value={approvals.isLoading ? '—' : String(approvals.data?.length ?? 0)}
              hint={t['dashboard.approvalsHint']}
              accent={(approvals.data?.length ?? 0) > 0}
            />
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <Panel>
            <SectionHeader
              icon={<CircleAlert size={14} />}
              title={t['dashboard.failedRuns']}
              to={`/app/${orgID}/boards`}
              linkLabel={t['dashboard.openBoards']}
            />
            {failed.isLoading ? (
              <SkeletonRows rows={3} columns={2} />
            ) : failedRecent.length === 0 ? (
              <div data-testid="dashboard-runs-empty" className="p-3">
                <EmptyState title={t['dashboard.noRuns']} hint={t['dashboard.noRunsHint']} />
              </div>
            ) : (
              <ul className="divide-y divide-[var(--color-border-subtle)]">
                {failedRecent.slice(0, 6).map((run) => (
                  <li key={run.id}>
                    <Link
                      to={`/app/${orgID}/runs/${run.id}`}
                      data-testid="dashboard-run-row"
                      data-outcome={run.outcome}
                      className="flex items-center gap-2 px-3 py-2 hover:bg-[var(--color-surface-hover)]"
                    >
                      <CircleAlert size={13} className="shrink-0 text-[var(--color-danger)]" />
                      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--color-secondary)]">
                        {run.id}
                      </span>
                      <span className="shrink-0 font-mono text-[11px] text-[var(--color-tertiary)]">
                        {formatMicroUSD(run.cost_micros)}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel>
            <SectionHeader
              icon={<Inbox size={14} />}
              title={t['dashboard.pendingApprovals']}
              to={`/app/${orgID}/approvals`}
              linkLabel={t['dashboard.openInbox']}
            />
            {(approvals.data ?? []).length === 0 ? (
              <div data-testid="dashboard-approvals-empty" className="p-3">
                <EmptyState title={t['dashboard.noApprovals']} hint={t['dashboard.noApprovalsHint']} />
              </div>
            ) : (
              <ul className="divide-y divide-[var(--color-border-subtle)]">
                {(approvals.data ?? []).slice(0, 6).map((approval) => (
                  <li key={approval.id}>
                    <Link
                      to={`/app/${orgID}/approvals/${approval.id}`}
                      className="flex items-center gap-2 px-3 py-2 hover:bg-[var(--color-surface-hover)]"
                    >
                      <Clock size={13} className="shrink-0 text-[var(--color-tertiary)]" />
                      <span className="min-w-0 flex-1 truncate text-[12px] text-[var(--color-secondary)]">
                        {approval.reason || approval.id}
                      </span>
                      <span className="shrink-0 font-mono text-[10px] text-[var(--color-tertiary)]">
                        {formatDateTime(approval.expires_at, 'en')}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>

        {/* Replaces the design's 7-day chart, which the contract cannot feed. */}
        <Panel>
          <div className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] px-3 py-2">
            <DollarSign size={14} className="text-[var(--color-tertiary)]" />
            <span className="text-[11px] font-semibold tracking-wide text-[var(--color-secondary)] uppercase">
              {t['dashboard.spendByBoard']}
            </span>
            <FolderKanban size={12} className="text-[var(--color-quaternary)]" />
            <span className="ml-auto text-[11px] text-[var(--color-tertiary)]">{t['dashboard.spendWindow']}</span>
          </div>
          {(spend?.by_board ?? []).length === 0 ? (
            <div data-testid="dashboard-spend-empty" className="p-3">
              <EmptyState title={t['dashboard.noSpend']} hint={t['dashboard.noSpendHint']} />
            </div>
          ) : (
            <ul className="divide-y divide-[var(--color-border-subtle)]">
              {(spend?.by_board ?? []).map((row) => (
                <li
                  key={row.board_id || row.name}
                  data-testid="spend-row"
                  className="flex items-center gap-3 px-3 py-2 text-[12px]"
                >
                  {/* A board that has been deleted keeps its cost but loses its
                      name — the API says so, so the screen says so too. */}
                  <span className="min-w-0 flex-1 truncate text-[var(--color-secondary)]">
                    {row.name || t['dashboard.deletedBoard']}
                  </span>
                  <span className="shrink-0 font-mono text-[11px] text-[var(--color-tertiary)]">
                    {t['dashboard.runsCount'].replace('{count}', String(row.runs))}
                  </span>
                  <span className="w-20 shrink-0 text-right font-mono tabular-nums text-[var(--color-primary)]">
                    {formatMicroUSD(row.cost_micros)}
                  </span>
                </li>
              ))}
            </ul>
          )}
          <p className="border-t border-[var(--color-border-subtle)] px-3 py-2 text-[11px] text-[var(--color-tertiary)]">
            {t['dashboard.noChartNote']}
          </p>
        </Panel>
      </div>
    </>
  )
}

/** Panel header with a link to the screen that owns the full list. */
function SectionHeader({
  icon,
  title,
  to,
  linkLabel,
}: {
  icon: React.ReactNode
  title: string
  to: string
  linkLabel: string
}) {
  return (
    <div className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] px-3 py-2">
      <span className="text-[var(--color-tertiary)]">{icon}</span>
      <span className="text-[11px] font-semibold tracking-wide text-[var(--color-secondary)] uppercase">{title}</span>
      <Link to={to} className="ml-auto flex items-center gap-1 text-[11px] text-[var(--color-accent)] hover:underline">
        {linkLabel}
        <ArrowRight size={11} />
      </Link>
    </div>
  )
}

/** True when `iso` is within the last `hours`. An unparseable stamp is not recent. */
function withinHours(iso: string, hours: number): boolean {
  const at = Date.parse(iso)
  if (Number.isNaN(at)) return false
  return Date.now() - at <= hours * 60 * 60 * 1000
}
