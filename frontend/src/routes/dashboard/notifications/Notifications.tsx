import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { CircleAlert, CircleCheck, Info, TriangleAlert } from 'lucide-react'
import { useListNotificationsQuery, useMarkNotificationsReadMutation } from '@/store/api/notifications'
import { useGetTaskQuery } from '@/store/api/boards'
import { useAppDispatch } from '@/store/hooks'
import { openTask } from '@/store/slices/uiSlice'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useT } from '@/hooks/use-t'
import { formatRelative } from '@/lib/formatters'

/**
 * Screen: notification centre — `design/stitch-output/v2/13-notifications.html`,
 * US-AD61.
 *
 * What the design fixes and this follows: one row per notification carrying the
 * kind's own mark, the relative time, the title, the body, and a per-row "mark
 * read". The header carries the unread count and the mark-all action.
 *
 * What the design asks for and the API does not have — deliberately not faked:
 *
 *  - **A "success" severity.** `notifications_kind_chk` (migration 0018) allows
 *    exactly four kinds: `approval.requested`, `budget.warning`, `run.failed`,
 *    `credential.invalid`. None of them is a success. A "Success" filter would
 *    be a chip that can never match a row, so the filters here are the four
 *    kinds the store can actually hold.
 *  - **A per-row dismiss.** There is no delete endpoint; `POST
 *    /notifications/read` is the only write. "Mark read" is real, "close" is
 *    not, so only the former is rendered.
 *
 * AC3 (a broken channel must not lose the notification) is why this screen
 * reads from the query cache and never from a stream: the rows are in Postgres,
 * and the list is refetched when the tab comes back. There is nothing to lose.
 */
const KIND_META: Record<string, { icon: typeof Info; tone: string }> = {
  'approval.requested': { icon: CircleAlert, tone: 'var(--color-warning)' },
  'budget.warning': { icon: TriangleAlert, tone: 'var(--color-warning)' },
  'run.failed': { icon: CircleAlert, tone: 'var(--color-danger)' },
  'credential.invalid': { icon: CircleAlert, tone: 'var(--color-danger)' },
}

/** Fallback for a kind the server adds before this screen knows about it. */
const DEFAULT_META = { icon: Info, tone: 'var(--color-tertiary)' }

export function Notifications() {
  const t = useT()
  const { orgID } = useParams<{ orgID: string }>()
  const { data, isLoading } = useListNotificationsQuery()
  const [markRead] = useMarkNotificationsReadMutation()
  const [filter, setFilter] = useState<string>('')

  const rows = data?.notifications ?? []
  const unread = data?.unread_count ?? 0
  const shown = filter ? rows.filter((n) => n.kind === filter) : rows

  return (
    <>
      <WorkspaceTopbar
        title={t['notifications.title']}
        path="/notifications"
        subtitle={unread > 0 ? t['notifications.unread'].replace('{count}', String(unread)) : undefined}
        right={
          unread > 0 ? (
            <Button size="sm" onClick={() => markRead({ all: true })}>
              <CircleCheck size={12} />
              {t['notifications.markAll']}
            </Button>
          ) : null
        }
      />

      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        {isLoading ? (
          <SkeletonRows rows={4} columns={3} />
        ) : rows.length === 0 ? (
          <EmptyState title={t['notifications.none']} hint={t['notifications.noneHint']} />
        ) : (
          <>
            <div role="group" aria-label={t['notifications.filter']} className="flex flex-wrap gap-1.5">
              <FilterChip active={filter === ''} onClick={() => setFilter('')} label={t['notifications.all']} />
              {Object.keys(KIND_META).map((kind) => (
                <FilterChip
                  key={kind}
                  active={filter === kind}
                  onClick={() => setFilter(kind)}
                  label={t[`notifications.kind.${kind}` as 'notifications.kind.run.failed']}
                />
              ))}
            </div>

            {shown.map((n) => {
              const meta = KIND_META[n.kind] ?? DEFAULT_META
              const Icon = meta.icon
              const unreadRow = !n.read_at
              return (
                <Panel key={n.id} className={`p-3 ${unreadRow ? 'border-l-2' : ''}`}>
                  <div className="flex items-start gap-2.5">
                    <Icon
                      size={15}
                      strokeWidth={2}
                      style={{ color: meta.tone }}
                      aria-hidden="true"
                      className="mt-0.5 shrink-0"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline gap-2">
                        <span className="font-mono text-[10px] text-[var(--color-tertiary)]">
                          {formatRelative(n.created_at)}
                        </span>
                        {unreadRow ? (
                          <span className="rounded-[4px] bg-[var(--color-accent-tint)] px-1.5 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-accent)]">
                            {t['notifications.unreadChip']}
                          </span>
                        ) : null}
                      </div>
                      <Target title={n.title} targetType={n.target_type} targetID={n.target_id} orgID={orgID} />
                      {n.body ? <p className="mt-1 text-[12px] text-[var(--color-secondary)]">{n.body}</p> : null}
                    </div>
                    {unreadRow ? (
                      <Button size="sm" variant="ghost" onClick={() => markRead({ ids: [n.id] })}>
                        {t['notifications.markRead']}
                      </Button>
                    ) : null}
                  </div>
                </Panel>
              )
            })}
          </>
        )}
      </div>
    </>
  )
}

/**
 * AC2: clicking a notification goes to the task it is about.
 *
 * The server sends `target_type` (`task` | `run` | `board`, migration 0018's
 * check) and the id. A `task` target opens the drawer from the board it lives
 * on, because the drawer needs a board to render inside — the notification does
 * not carry `board_id`, so the task is fetched and the board read off it. A
 * target with no known route stays plain text rather than becoming a link to a
 * 404.
 */
function Target({
  title,
  targetType,
  targetID,
  orgID,
}: {
  title: string
  targetType?: string
  targetID?: string
  orgID?: string
}) {
  const base = orgID ? `/app/${orgID}` : '/app'
  const dispatch = useAppDispatch()
  // The drawer is driven by `ui.openTaskID` and renders inside a board, so a
  // `task` target needs the board it lives on. The notification carries the
  // task id and nothing else, so the task is fetched — this is the one target
  // whose destination is not in the payload.
  const isTask = targetType === 'task' && Boolean(targetID)
  const { data: task } = useGetTaskQuery(targetID ?? '', { skip: !isTask })

  if (targetType === 'run' && targetID) {
    return (
      <Link
        to={`${base}/runs/${targetID}`}
        className="text-[13px] font-semibold text-[var(--color-primary)] hover:underline"
      >
        {title}
      </Link>
    )
  }
  if (targetType === 'board' && targetID) {
    return (
      <Link
        to={`${base}/boards/${targetID}`}
        className="text-[13px] font-semibold text-[var(--color-primary)] hover:underline"
      >
        {title}
      </Link>
    )
  }
  if (isTask && task) {
    return (
      <Link
        to={`${base}/boards/${task.board_id}`}
        onClick={() => dispatch(openTask(task.id))}
        className="text-[13px] font-semibold text-[var(--color-primary)] hover:underline"
      >
        {title}
      </Link>
    )
  }
  // A target with no reachable destination stays plain text: a link that leads
  // to a 404 is worse than no link at all.
  return <span className="text-[13px] font-semibold text-[var(--color-primary)]">{title}</span>
}

function FilterChip({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={`rounded-[6px] border px-2 py-1 font-mono text-[10px] transition-colors ${
        active
          ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
          : 'border-[var(--color-border-subtle)] text-[var(--color-tertiary)] hover:text-[var(--color-secondary)]'
      }`}
    >
      {label}
    </button>
  )
}
