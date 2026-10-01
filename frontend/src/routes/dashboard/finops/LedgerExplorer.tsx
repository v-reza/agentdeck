import { useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { EmptyState, Panel, StatTile } from '@/components/ui/card'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useOrgLedgerQuery } from '@/store/api/finops'
import { formatMicroUSD, formatTokens } from '@/lib/formatters'
import { useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'

/**
 * Screen 30-ledger-explorer — the cost ledger, workspace-wide (US-AD27 AC4, AC5).
 *
 * This is NOT the panel on the cost overview screen. That one shows one board's
 * last few priced calls beside its budget; this one is the ledger as a queryable
 * table across every board in the workspace, which is what "explorer" means.
 *
 * THE FOUR CARDS DESCRIBE THE FILTER, NOT THE PAGE. The page is capped at 100 rows
 * and the server sends the totals of the whole filtered set alongside it
 * (`total_rows`, `total_micros`). Reading them off `entries` instead would quietly
 * understate a workspace's spend the moment it has more than one page of history,
 * and it would do so on the screen whose entire job is being right about money.
 *
 * WHAT IS NOT BUILT, AND WHY:
 *
 * - CSV export. The design has the button; there is no export endpoint and
 *   building it client-side would mean fetching every page in a loop to write a
 *   file the server could stream. A download that silently truncates at the page
 *   boundary is worse than no button.
 * - "Sync: Ingested (0 drift)" and "Precision: 1 µUSD" badges. The first is a
 *   reconciliation figure no endpoint produces; the second is a claim about the
 *   schema (`cost_micros` is BIGINT micro-USD) that belongs in the docs, not
 *   repeated as chrome.
 * - Cache read/write as their own columns. The API returns
 *   `cache_read_tokens`/`cache_write_tokens` and they are shown, but folded into
 *   one "Cache" cell (`read / write`) because two more numeric columns pushed the
 *   row past the width of the pane and made the table scroll sideways.
 */
/**
 * Rows per page. Deliberately small enough that a busy workspace pages, because
 * the whole point of the summary is to describe the FILTER and not the page —
 * a limit nobody reaches would let a "count the visible rows" bug look correct.
 */
const PAGE_LIMIT = 25

export function LedgerExplorer() {
  const t = useT()
  const orgID = useAppSelector((state) => state.session.activeOrgID)

  const [agentID, setAgentID] = useState('')
  const [model, setModel] = useState('')
  const [offset, setOffset] = useState(0)
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  const { data, isLoading, isFetching, refetch } = useOrgLedgerQuery(
    { orgID: orgID ?? '', agentID, model, from, to, offset, limit: PAGE_LIMIT },
    { skip: !orgID },
  )
  // BOTH option lists come from the rows in hand, not from a catalogue.
  //
  // Agents are per-PROJECT (`GET /projects/{id}/agents`) while the ledger is
  // per-WORKSPACE, so there is no single call that lists the agents a ledger page
  // could contain — fetching them would mean one request per project to build a
  // dropdown. The filter's job is to narrow what is on screen, and an option the
  // workspace has never spent against could only ever produce an empty table.
  //
  // Consequence, stated plainly: the dropdown offers the agents present in the
  // CURRENT page, so an agent whose only entries are older than the page cannot be
  // selected from here. That is a real limit of this screen, not an oversight.
  const agentOptions = useMemo(() => {
    const seen = new Set<string>()
    for (const entry of data?.entries ?? []) if (entry.agent_id) seen.add(entry.agent_id)
    return [{ value: '', label: t['ledger.allAgents'] }, ...[...seen].sort().map((a) => ({ value: a, label: a }))]
  }, [data, t])

  const modelOptions = useMemo(() => {
    const seen = new Set<string>()
    for (const entry of data?.entries ?? []) seen.add(entry.model)
    return [{ value: '', label: t['ledger.allModels'] }, ...[...seen].sort().map((m) => ({ value: m, label: m }))]
  }, [data, t])

  const filtered = Boolean(agentID || model || from || to)
  const entries = data?.entries ?? []
  const totalRows = data?.total_rows ?? 0

  return (
    <>
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border-subtle)] px-4 py-2">
        <h1 className="text-[13px] font-semibold text-[var(--color-primary)]">{t['ledger.title']}</h1>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-1 text-[11px] text-[var(--color-tertiary)]">
            {t['ledger.from']}
            <input
              type="date"
              value={from}
              onChange={(event) => {
                setFrom(event.target.value)
                setOffset(0)
              }}
              data-testid="ledger-from"
              className="h-8 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          </label>
          <label className="flex items-center gap-1 text-[11px] text-[var(--color-tertiary)]">
            {t['ledger.to']}
            <input
              type="date"
              value={to}
              onChange={(event) => {
                setTo(event.target.value)
                setOffset(0)
              }}
              data-testid="ledger-to"
              className="h-8 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          </label>
          <div className="w-[168px]">
            <Combobox
              label={t['ledger.agent']}
              value={agentID}
              onChange={(next) => {
                setAgentID(next)
                setOffset(0)
              }}
              options={agentOptions}
              placeholder={t['ledger.allAgents']}
            />
          </div>
          <div className="w-[168px]">
            <Combobox
              label={t['ledger.model']}
              value={model}
              onChange={(next) => {
                setModel(next)
                setOffset(0)
              }}
              options={modelOptions}
              placeholder={t['ledger.allModels']}
            />
          </div>
          <Button variant="secondary" size="sm" onClick={() => void refetch()} disabled={isFetching}>
            <RefreshCw size={13} aria-hidden="true" />
            {t['ledger.refresh']}
          </Button>
        </div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        <div className="grid grid-cols-2 gap-2.5 lg:grid-cols-4">
          {/* The card wraps in a testid because its label is translated while the
              number is not: a test that keyed off the wording would break the
              moment the locale changed, which this repo has been bitten by. */}
          <div data-testid="ledger-card-entries">
            <StatTile label={t['ledger.totalEntries']} value={String(data?.total_rows ?? 0)} />
          </div>
          {/* From the server's window totals, not summed from `entries`: on a
              workspace with more rows than a page, summing the page would make
              these two cards disagree with the entry count next to them. */}
          <div data-testid="ledger-card-tokens-in">
            <StatTile label={t['ledger.tokensIn']} value={formatTokens(data?.total_tokens_in ?? 0)} />
          </div>
          <StatTile label={t['ledger.tokensOut']} value={formatTokens(data?.total_tokens_out ?? 0)} />
          <StatTile label={t['ledger.totalCost']} value={formatMicroUSD(data?.total_micros ?? 0)} accent />
        </div>

        <Panel>
          <div className="flex items-center justify-between border-b border-[var(--color-border-subtle)] px-3 py-2">
            <span className="text-[11px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
              {t['ledger.table']}
            </span>
            <div className="flex items-center gap-2">
              <span data-testid="ledger-showing" className="font-mono text-[11px] text-[var(--color-quaternary)]">
                {isLoading
                  ? t['ledger.loading']
                  : t['ledger.showing']
                      .replace('{shown}', String(entries.length))
                      .replace('{total}', String(data?.total_rows ?? 0))}
              </span>
              {/* Paging exists because the totals describe the filter, not the
                  page: without a way past the first page, a workspace with more
                  rows than PAGE_LIMIT could never reach the rest of them. */}
              {totalRows > PAGE_LIMIT ? (
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    data-testid="ledger-prev"
                    disabled={offset === 0}
                    onClick={() => setOffset(Math.max(0, offset - PAGE_LIMIT))}
                    className="rounded-[4px] border border-[var(--color-border-subtle)] px-1.5 text-[11px] text-[var(--color-secondary)] disabled:opacity-40"
                  >
                    ‹
                  </button>
                  <button
                    type="button"
                    data-testid="ledger-next"
                    disabled={offset + PAGE_LIMIT >= totalRows}
                    onClick={() => setOffset(offset + PAGE_LIMIT)}
                    className="rounded-[4px] border border-[var(--color-border-subtle)] px-1.5 text-[11px] text-[var(--color-secondary)] disabled:opacity-40"
                  >
                    ›
                  </button>
                </div>
              ) : null}
            </div>
            {/* The qualifier lives here, once, instead of on every row: the
                ledger holds exact micro-USD and `formatMicroUSD` renders it at
                full precision, but the number is still a projection from the
                internal price table rather than a provider invoice. */}
            <span className="text-[10px] text-[var(--color-quaternary)]">{t['ledger.estimateNote']}</span>
          </div>

          {isLoading ? (
            <SkeletonRows rows={8} columns={7} />
          ) : entries.length === 0 ? (
            // AC5: an empty range gets a prompt to change the range, not a bare
            // table. The wording differs when a filter is set, because "no rows in
            // this range" and "this workspace has never spent anything" are
            // different facts and only one of them is actionable.
            <div data-testid="ledger-empty" className="p-4">
              <EmptyState
                title={filtered ? t['ledger.emptyFiltered'] : t['ledger.emptyAll']}
                hint={filtered ? t['ledger.emptyFilteredHint'] : t['ledger.emptyAllHint']}
              />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table data-testid="ledger-table" className="w-full border-collapse">
                <thead>
                  <tr className="border-b border-[var(--color-border-subtle)]">
                    {(
                      [
                        ['time', t['ledger.colTime']],
                        ['agent', t['ledger.colAgent']],
                        ['model', t['ledger.colModel']],
                        ['tokens-in', t['ledger.colTokensIn']],
                        ['tokens-out', t['ledger.colTokensOut']],
                        ['cache', t['ledger.colCache']],
                        ['cost', t['ledger.colCost']],
                        ['price-version', t['ledger.colPriceVersion']],
                      ] as const
                    ).map(([column, label], index) => (
                      <th
                        key={column}
                        data-testid={`ledger-col-${column}`}
                        className={`px-3 py-1.5 text-[10px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)] ${
                          index >= 3 ? 'text-right' : 'text-left'
                        }`}
                      >
                        {label}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {entries.map((entry) => (
                    <tr
                      key={entry.id}
                      data-testid="ledger-row"
                      className="h-8 border-b border-[var(--color-border-subtle)] last:border-b-0"
                    >
                      {/* tabular-nums on every numeric cell: AC4 asks for monospace
                          with tabular figures, and without it the columns jitter
                          row to row. */}
                      <td className="px-3 font-mono text-[11px] tabular-nums text-[var(--color-secondary)]">
                        {entry.created_at.replace('T', ' ').slice(0, 19)}
                      </td>
                      <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">
                        {entry.agent_name || entry.agent_id || '—'}
                      </td>
                      <td className="px-3 font-mono text-[11px] text-[var(--color-secondary)]">{entry.model}</td>
                      <td className="px-3 text-right font-mono text-[11px] tabular-nums text-[var(--color-primary)]">
                        {entry.tokens_in}
                      </td>
                      <td className="px-3 text-right font-mono text-[11px] tabular-nums text-[var(--color-primary)]">
                        {entry.tokens_out}
                      </td>
                      <td className="px-3 text-right font-mono text-[11px] tabular-nums text-[var(--color-tertiary)]">
                        {entry.cache_read_tokens} / {entry.cache_write_tokens}
                      </td>
                      <td className="px-3 text-right font-mono text-[11px] tabular-nums text-[var(--color-primary)]">
                        {formatMicroUSD(entry.cost_micros)}
                      </td>
                      <td className="px-3 text-right font-mono text-[11px] text-[var(--color-quaternary)]">
                        v{entry.price_version}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>
      </div>
    </>
  )
}
