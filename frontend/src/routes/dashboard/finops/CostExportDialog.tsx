import { useState } from 'react'
import { Download } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import { useOrgLedgerQuery, type LedgerQueryArgs } from '@/store/api/finops'
import type { LedgerEntry } from '@/lib/domain'
import { useT } from '@/hooks/use-t'

/**
 * Screen 31-cost-export — the cost CSV export dialog (US-AD56).
 *
 * AC1 lists the columns: Task ID, Run ID, Agent, Model, Tokens In, Tokens Out,
 * Cost. That list is what the file carries, in that order. The design's own
 * "Format Kolom" line omits Run ID and is therefore not the contract — the AC is.
 *
 * AC2 is the date range, and it is the only filter the export offers: the
 * endpoint takes `from`/`to` as `YYYY-MM-DD` (the screen's inputs are dates, so
 * they are passed through unchanged) plus agent/model, and narrowing by agent is
 * the explorer's job, not the export's.
 *
 * THE HONEST LIMIT, stated on screen and not just in this comment: there is no
 * export endpoint and no streaming. The file is assembled from the API's own
 * pages, and the export stops at MAX_ROWS rows so it can never be mistaken for
 * "everything". The row count shown is the API's `total_rows` for the chosen
 * range, so when the range is larger than the cap the screen says so BEFORE the
 * download rather than after.
 */

/** The server clamps `limit` to 500 (`internal/board/runtime.go`). */
const PAGE_SIZE = 500
/** The most rows this export will assemble, so the cap is visible and fixed. */
const MAX_ROWS = 500

const COLUMNS = ['time', 'task_id', 'run_id', 'agent', 'model', 'tokens_in', 'tokens_out', 'cost_micros'] as const

/** Quote always: a model name can contain a comma and one stray comma shifts every later column. */
function csvCell(value: unknown): string {
  return `"${String(value ?? '').replace(/"/g, '""')}"`
}

function toRow(entry: LedgerEntry): string {
  return [
    entry.created_at,
    entry.task_id,
    entry.run_id,
    entry.agent_name,
    entry.model,
    entry.tokens_in,
    entry.tokens_out,
    entry.cost_micros,
  ]
    .map(csvCell)
    .join(',')
}

function downloadCSV(filename: string, header: readonly string[], rows: string[]): void {
  // A BOM, so Excel reads the file as UTF-8 instead of the local codepage. The
  // agent names are the reason: an accented name is mojibake without it.
  const body = '\uFEFF' + [header.map(csvCell).join(','), ...rows].join('\r\n') + '\r\n'
  const url = URL.createObjectURL(new Blob([body], { type: 'text/csv;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

/** `YYYY-MM-DD` for a day offset back from today, in the operator's own clock. */
function dayOffset(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() - days)
  return d.toISOString().slice(0, 10)
}

type Range = 'today' | '7d' | '30d' | 'custom'

interface Props {
  open: boolean
  onClose: () => void
  orgID: string
}

export function CostExportDialog({ open, onClose, orgID }: Props) {
  const t = useT()
  const [range, setRange] = useState<Range>('7d')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [busy, setBusy] = useState(false)

  const effectiveFrom =
    range === 'custom' ? from : range === 'today' ? dayOffset(0) : dayOffset(range === '7d' ? 6 : 29)
  const effectiveTo = range === 'custom' ? to : dayOffset(0)

  // `limit: 1` because only `total_rows` is wanted here — the server computes it
  // with a window function before the limit, so it is the count of the whole
  // filtered range and not of the page.
  const countArgs: LedgerQueryArgs = { orgID, from: effectiveFrom, to: effectiveTo, limit: 1 }
  const { data, isFetching } = useOrgLedgerQuery(countArgs, { skip: !open || !orgID })

  const total = data?.total_rows ?? 0
  const capped = total > MAX_ROWS

  async function exportNow() {
    setBusy(true)
    try {
      const collected: LedgerEntry[] = []
      for (let offset = 0; offset < MAX_ROWS; offset += PAGE_SIZE) {
        const page = await fetchPage(orgID, effectiveFrom, effectiveTo, offset)
        collected.push(...page)
        if (page.length < PAGE_SIZE) break
      }
      const rows = collected.slice(0, MAX_ROWS).map(toRow)
      downloadCSV(`agentdeck-cost-${effectiveFrom}_${effectiveTo}.csv`, COLUMNS, rows)
      onClose()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="md"
      title={t['costExport.title']}
      description={t['costExport.description']}
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose} data-testid="cost-export-cancel">
            {t['action.cancel']}
          </Button>
          <Button
            size="sm"
            disabled={busy || isFetching || total === 0}
            onClick={() => void exportNow()}
            data-testid="cost-export-download"
          >
            <Download size={13} aria-hidden="true" />
            {t['costExport.download']}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1">
          <span className="text-[12px] text-[var(--color-secondary)]">{t['costExport.range']}</span>
          <div className="flex flex-wrap gap-1">
            {(['today', '7d', '30d', 'custom'] as const).map((option) => (
              <Button
                key={option}
                size="sm"
                variant={range === option ? 'primary' : 'secondary'}
                onClick={() => setRange(option)}
                data-testid={`cost-export-range-${option}`}
              >
                {t[`costExport.range.${option}`]}
              </Button>
            ))}
          </div>
        </div>

        {range === 'custom' ? (
          <div className="flex gap-3">
            <label className="flex flex-col gap-1 text-[12px] text-[var(--color-secondary)]">
              {t['costExport.from']}
              <input
                type="date"
                value={from}
                onChange={(event) => setFrom(event.target.value)}
                data-testid="cost-export-from"
                className="h-8 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] text-[var(--color-secondary)]">
              {t['costExport.to']}
              <input
                type="date"
                value={to}
                onChange={(event) => setTo(event.target.value)}
                data-testid="cost-export-to"
                className="h-8 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
              />
            </label>
          </div>
        ) : null}

        <dl className="grid grid-cols-[180px_1fr] gap-y-1 text-[12px]">
          <dt className="text-[var(--color-tertiary)]">{t['costExport.selected']}</dt>
          <dd className="font-mono text-[var(--color-primary)]" data-testid="cost-export-range-label">
            {effectiveFrom} → {effectiveTo}
          </dd>
          <dt className="text-[var(--color-tertiary)]">{t['costExport.rows']}</dt>
          <dd className="font-mono text-[var(--color-primary)]" data-testid="cost-export-count">
            {total}
          </dd>
          <dt className="text-[var(--color-tertiary)]">{t['costExport.columns']}</dt>
          <dd className="font-mono text-[11px] text-[var(--color-secondary)]">{COLUMNS.join(', ')}</dd>
        </dl>

        {/* The cap is stated before the download, never discovered after it. */}
        {capped ? (
          <p className="text-[12px] text-[var(--color-danger)]" data-testid="cost-export-cap">
            {t['costExport.cap'].replace('{max}', String(MAX_ROWS))}
          </p>
        ) : null}
      </div>
    </Modal>
  )
}

/**
 * One page of the org ledger.
 *
 * Deliberately a `fetch` and not the RTK Query hook: the export walks pages in a
 * loop, and a hook per page would put every page in the cache for a result that
 * is written to a file and then never read again.
 */
async function fetchPage(orgID: string, from: string, to: string, offset: number): Promise<LedgerEntry[]> {
  const params = new URLSearchParams({ from, to, offset: String(offset), limit: String(PAGE_SIZE) })
  const response = await fetch(`/api/v1/orgs/${orgID}/ledger?${params.toString()}`, { credentials: 'include' })
  if (!response.ok) throw new Error(`ledger export page failed: ${response.status}`)
  const body = (await response.json()) as { entries: LedgerEntry[] }
  return body.entries
}
