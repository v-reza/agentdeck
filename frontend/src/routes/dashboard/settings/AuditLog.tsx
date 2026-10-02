import { useMemo, useState } from 'react'
import { Download, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EmptyState, Panel } from '@/components/ui/card'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useListAuditLogQuery, type AuditEntry } from '@/store/api/audit'
import { useCanAct } from '@/hooks/use-orgs'
import { useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'
import { formatDateTime } from '@/lib/format'

/**
 * Screen 41-audit-log — who changed what, when (US-AD95).
 *
 * ROLE GATE IS ON THE PAGE, NOT ONLY ON THE ENDPOINT. AC4 is explicit that
 * `member` and `viewer` get a refusal here rather than a screen that renders an
 * empty table because every request 403s. The endpoint still enforces it; this
 * is the second, visible half.
 *
 * WHAT IS NOT BUILT, AND WHY:
 *
 * - **Actor names.** The API returns `actor_user_id` / `actor_agent_id` and
 *   nothing else — there is no endpoint that resolves an id to a display name
 *   for an arbitrary actor, and inventing one from the membership list would be
 *   wrong the moment the actor is an agent or a former member. The column shows
 *   the id's tail with the full id on `title` and on `data-actor`, which is
 *   unambiguous even if it is not pretty.
 * - **Server-side export.** There is no export endpoint and no `text/csv`
 *   anywhere in the app. CSV is written client-side from the rows the API
 *   actually returned, so the file can never contain a figure the API did not
 *   send. Consequence, stated plainly: the export covers the loaded pages, not
 *   the whole table.
 * - **Action catalogue.** `action` is filtered by the exact string the row
 *   carries (`org.rename`), and there is no endpoint listing every action name.
 *   That is why actor and action are TEXT inputs rather than dropdowns: a
 *   dropdown built from the loaded page can only ever re-select what is already
 *   on screen, so it cannot be used to FIND a row — which is the entire job of
 *   the filter. The value the API wants is a string, and the screen asks for the
 *   string.
 */

const PAGE_LIMIT = 50

/** `2026-09-19` → the RFC3339 instant the server accepts. `end` is exclusive. */
function dayToRFC3339(day: string, end: boolean): string {
  if (!day) return ''
  // A date input gives a bare day; the server wants an instant. Start of day for
  // `from`, end of day for `to`, so a single-day range includes that whole day.
  const suffix = end ? 'T23:59:59Z' : 'T00:00:00Z'
  return `${day}${suffix}`
}

/** The tail of a ULID is enough to tell two actors apart on screen. */
function shortID(id: string): string {
  return id.length > 8 ? `…${id.slice(-8)}` : id
}

function diffSummary(entry: AuditEntry): string {
  const parts: string[] = []
  if (entry.before_json !== undefined) parts.push(JSON.stringify(entry.before_json))
  if (entry.after_json !== undefined) parts.push(JSON.stringify(entry.after_json))
  return parts.join(' → ')
}

function csvCell(value: unknown): string {
  const text = typeof value === 'string' ? value : JSON.stringify(value ?? '')
  // Quote always: a JSON diff contains commas, quotes, and newlines, and one
  // unquoted comma silently shifts every later column.
  return `"${String(text ?? '').replace(/"/g, '""')}"`
}

export function AuditLog() {
  const t = useT()
  const orgID = useAppSelector((state) => state.session.activeOrgID)
  // The dictionary carries copy, not the locale tag; `formatDateTime` needs the
  // tag, so it is read from the slice the same way `useT` reads it.
  const lang = useAppSelector((state) => state.lang.lang)
  const canRead = useCanAct('admin')

  const [actor, setActor] = useState('')
  const [action, setAction] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [cursor, setCursor] = useState<number | undefined>(undefined)
  const [pages, setPages] = useState<AuditEntry[][]>([])

  const { data, isLoading, isFetching, refetch } = useListAuditLogQuery(
    {
      actor,
      action,
      from: dayToRFC3339(from, false),
      to: dayToRFC3339(to, true),
      cursor,
      limit: PAGE_LIMIT,
    },
    { skip: !orgID || !canRead },
  )

  // Pages accumulate so "next" walks backwards through history. A cursor query
  // replaces `data`, so without this the table would show one page at a time and
  // an operator could never read a range longer than 50 rows.
  const entries = useMemo(() => [...pages.flat(), ...(data?.entries ?? [])], [pages, data])

  const filtered = Boolean(actor || action || from || to)
  const lastID = entries.length > 0 ? entries[entries.length - 1].id : undefined
  const canPage = (data?.entries?.length ?? 0) === PAGE_LIMIT

  function resetFilters() {
    setPages([])
    setCursor(undefined)
  }

  function exportCsv() {
    const header = [
      t['audit.csvTime'],
      t['audit.csvActor'],
      t['audit.csvAction'],
      t['audit.csvTarget'],
      t['audit.csvBefore'],
      t['audit.csvAfter'],
    ]
    const rows = entries.map((e) => [
      e.created_at,
      e.actor_user_id || e.actor_agent_id || '',
      e.action,
      `${e.target_type}:${e.target_id}`,
      e.before_json === undefined ? '' : JSON.stringify(e.before_json),
      e.after_json === undefined ? '' : JSON.stringify(e.after_json),
    ])
    const csv = [header, ...rows].map((r) => r.map(csvCell).join(',')).join('\r\n')
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `audit-log-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  if (!canRead) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center p-6">
        <div data-testid="audit-forbidden">
          <EmptyState title={t['audit.forbiddenTitle']} hint={t['audit.forbiddenHint']} />
        </div>
      </div>
    )
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border-subtle)] px-4 py-2">
        <h1 className="text-[13px] font-semibold text-[var(--color-primary)]">{t['audit.title']}</h1>
        {/* The design carries a role badge beside the title. Its wording is spec
            jargon and is not copied; the fact it states — who may read this — is. */}
        <span
          className="rounded-full border border-[var(--color-border-subtle)] px-2 py-[2px] font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]"
          data-testid="audit-role-badge"
        >
          {t['audit.roleBadge']}
        </span>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-1 text-[11px] text-[var(--color-tertiary)]">
            {t['audit.from']}
            <input
              type="date"
              value={from}
              onChange={(event) => {
                setFrom(event.target.value)
                resetFilters()
              }}
              data-testid="audit-from"
              className="h-8 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          </label>
          <label className="flex items-center gap-1 text-[11px] text-[var(--color-tertiary)]">
            {t['audit.to']}
            <input
              type="date"
              value={to}
              onChange={(event) => {
                setTo(event.target.value)
                resetFilters()
              }}
              data-testid="audit-to"
              className="h-8 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          </label>
          <label className="flex items-center gap-1 text-[11px] text-[var(--color-tertiary)]">
            {t['audit.actor']}
            <input
              type="text"
              value={actor}
              onChange={(event) => {
                setActor(event.target.value)
                resetFilters()
              }}
              placeholder={t['audit.actorPlaceholder']}
              data-testid="audit-actor-input"
              className="h-8 w-[200px] rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          </label>
          <label className="flex items-center gap-1 text-[11px] text-[var(--color-tertiary)]">
            {t['audit.action']}
            <input
              type="text"
              value={action}
              onChange={(event) => {
                setAction(event.target.value)
                resetFilters()
              }}
              placeholder={t['audit.actionPlaceholder']}
              data-testid="audit-action-input"
              className="h-8 w-[190px] rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          </label>
          <Button variant="secondary" size="sm" onClick={() => void refetch()} disabled={isFetching}>
            <RefreshCw size={13} aria-hidden="true" />
            {t['audit.refresh']}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={exportCsv}
            disabled={entries.length === 0}
            data-testid="audit-export"
          >
            <Download size={13} aria-hidden="true" />
            {t['audit.export']}
          </Button>
        </div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        <Panel className="overflow-hidden">
          {/* The design states the active filter and offers a reset. Only shown
              when a filter IS active, so the row is not dead chrome. */}
          {filtered ? (
            <div
              className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border-subtle)] px-3 py-2 text-[11px] text-[var(--color-tertiary)]"
              data-testid="audit-active-filters"
            >
              <span>{t['audit.activeFilters']}</span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setActor('')
                  setAction('')
                  setFrom('')
                  setTo('')
                  resetFilters()
                }}
                data-testid="audit-reset"
              >
                {t['audit.reset']}
              </Button>
            </div>
          ) : null}
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-[12px]" data-testid="audit-table">
              <thead>
                <tr className="border-b border-[var(--color-border-subtle)] text-left">
                  <th
                    className="px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]"
                    data-testid="audit-col-time"
                  >
                    {t['audit.colTime']}
                  </th>
                  <th
                    className="px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]"
                    data-testid="audit-col-actor"
                  >
                    {t['audit.colActor']}
                  </th>
                  <th
                    className="px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]"
                    data-testid="audit-col-action"
                  >
                    {t['audit.colAction']}
                  </th>
                  <th
                    className="px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]"
                    data-testid="audit-col-target"
                  >
                    {t['audit.colTarget']}
                  </th>
                  <th
                    className="px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]"
                    data-testid="audit-col-diff"
                  >
                    {t['audit.colDiff']}
                  </th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr
                    key={entry.id}
                    className="border-b border-[var(--color-border-subtle)] last:border-b-0"
                    data-testid="audit-row"
                  >
                    <td className="whitespace-nowrap px-3 py-2 font-mono text-[11px] text-[var(--color-secondary)]">
                      {formatDateTime(entry.created_at, lang)}
                    </td>
                    <td
                      className="px-3 py-2 text-[var(--color-primary)]"
                      data-testid="audit-actor"
                      data-actor={entry.actor_user_id || entry.actor_agent_id || ''}
                      title={entry.actor_user_id || entry.actor_agent_id || ''}
                    >
                      {entry.actor_user_id ? (
                        <span className="font-mono text-[11px]">{shortID(entry.actor_user_id)}</span>
                      ) : entry.actor_agent_id ? (
                        <span className="text-[var(--color-tertiary)]">
                          {t['audit.actorAgent']} <span className="font-mono">{entry.actor_agent_id}</span>
                        </span>
                      ) : (
                        <span className="text-[var(--color-tertiary)]">{t['audit.actorUnknown']}</span>
                      )}
                    </td>
                    <td
                      className="px-3 py-2 font-mono text-[11px] text-[var(--color-primary)]"
                      data-testid="audit-action"
                    >
                      {entry.action}
                    </td>
                    <td className="px-3 py-2 font-mono text-[11px] text-[var(--color-secondary)]">
                      {entry.target_type}
                      <span className="text-[var(--color-tertiary)]"> {shortID(entry.target_id)}</span>
                    </td>
                    <td className="max-w-[380px] px-3 py-2 font-mono text-[10px] text-[var(--color-secondary)]">
                      <span className="block truncate" title={diffSummary(entry)}>
                        {diffSummary(entry) || '—'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {isLoading ? <SkeletonRows rows={6} /> : null}

          {!isLoading && entries.length === 0 ? (
            <div className="p-4" data-testid="audit-empty">
              <EmptyState
                title={filtered ? t['audit.emptyFiltered'] : t['audit.emptyAll']}
                hint={t['audit.emptyHint']}
              />
            </div>
          ) : null}
        </Panel>

        {canPage ? (
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              data-testid="audit-more"
              disabled={isFetching}
              onClick={() => {
                if (lastID === undefined) return
                setPages((prev) => [...prev, data?.entries ?? []])
                setCursor(lastID)
              }}
            >
              {t['audit.loadMore']}
            </Button>
            <span className="text-[11px] text-[var(--color-tertiary)]" data-testid="audit-count">
              {t['audit.loaded']} {entries.length}
            </span>
          </div>
        ) : null}
      </div>
    </>
  )
}
