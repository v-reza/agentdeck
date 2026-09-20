/**
 * Money and duration formatting. DECISIONS 6: `cost_micros` is BIGINT micro-USD
 * (1 USD = 1_000_000) and must never be a float. Every money value crossing this
 * boundary arrives as an integer number of micros; formatting is the only place
 * a decimal point appears.
 *
 * The rendering rules come from the PRD and the design source: a cost badge uses
 * two decimals (US-AD32 AC1 pins `$0.04` and `$1.23`), and a sub-cent amount must
 * still show a digit rather than round to a clean zero. One fixed precision can
 * only ever satisfy one of those two: whole cents get two decimals, and a
 * sub-cent amount is shown at the ledger's own micro-USD precision.
 */

const MICROS_PER_USD = 1_000_000

/**
 * Costs are rendered with two decimals, matching the two literals the PRD pins
 * for a cost badge (US-AD32 AC1: `$0.04`, `$1.23`).
 *
 * Two decimals alone would round a sub-cent run down to a clean `$0.00` — hiding
 * real spend, the one thing a cost ledger must never do. So a non-zero amount
 * that would vanish at two decimals is rendered at full micro-USD precision
 * instead: exactly the integer the ledger holds, never a rounded stand-in.
 * `$0.000` would be as much of a lie as `$0.00`, and so would `$0.00032` for a
 * cost of 318 micros — the formatter must not invent precision either way.
 */
const ONE_CENT_MICROS = 10_000
/** Micro-USD is the ledger's own unit, so 6 decimals is exact, not approximate. */
const MICRO_DIGITS = 6

function dollars(micros: number, digits: number): string {
  return (micros / MICROS_PER_USD).toFixed(digits)
}

/**
 * formatMicroUSD(4_820_000) === "$4.82"
 * formatMicroUSD(40_000)    === "$0.04"
 * formatMicroUSD(318)       === "$0.000318"
 */
export function formatMicroUSD(micros: number): string {
  if (micros === 0) return '$0.00'
  if (Math.abs(micros) >= ONE_CENT_MICROS) return `$${dollars(micros, 2)}`
  // Trailing zeros carry no information, so they are dropped — but only past the
  // decimal point, or stripping them from 0 would leave a bare "$0.".
  const exact = dollars(micros, MICRO_DIGITS).replace(/(\.\d*?)0+$/, '$1')
  return `$${exact}`
}

/**
 * Cost copy for product surfaces. Ledger values are projections from the
 * internal pricing table, not provider invoices, so every visible amount uses
 * this helper rather than the unqualified currency formatter.
 */
export function formatEstimatedMicroUSD(micros: number): string {
  return `${formatMicroUSD(micros)} (estimate)`
}

/**
 * formatMicroUSDPadded keeps a column of costs visually aligned: always three
 * decimals, right-aligned by padding to a fixed width. Used inside tables where
 * a shifting decimal point would make the column unreadable.
 */
export function formatMicroUSDPadded(micros: number): string {
  return `$${dollars(micros, 3)}`.padStart(9, ' ')
}

/** Bare decimal without the currency sign, for axis labels and tooltips. */
export function formatUSD(micros: number): string {
  return dollars(micros, 2)
}

/**
 * Compact token counts: 1200 -> "1.2k", 3400000 -> "3.4M". Lowercase `k` matches
 * the design source (`128k`, `7.2k`); the `M` suffix is uppercase there too.
 */
export function formatTokens(tokens: number): string {
  if (tokens < 1000) return String(tokens)
  if (tokens < 1_000_000) return `${(tokens / 1000).toFixed(1)}k`
  return `${(tokens / 1_000_000).toFixed(1)}M`
}

/**
 * Budget meter width as a percentage. The value is clamped to 100 because it is
 * a CSS width: an overspend must not overflow the meter. Overspend is signalled
 * separately by `budgetOver` / the N18 threshold flag, not by a >100 width.
 */
export function budgetPercent(usedMicros: number, capMicros: number): number {
  if (capMicros <= 0) return 0
  return Math.min(100, (usedMicros / capMicros) * 100)
}

/**
 * True when spend has reached or passed the cap. Drives the danger tone on the
 * meter.
 *
 * US-AD30 AC1 rejects a claim when spend is `>= N16`, so "at the cap" is already
 * over: a strict `>` would leave a board sitting at exactly 100% rendered as
 * healthy, which is the state an operator most needs to notice.
 */
export function budgetOver(usedMicros: number, capMicros: number): boolean {
  return capMicros > 0 && usedMicros >= capMicros
}

/** formatDuration(90_000) === "1m 30s". Used by run timelines. */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  const totalSeconds = Math.floor(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

/** Relative time for board rows: "just now", "4m ago", "3d ago". */
export function formatRelative(iso: string | null | undefined): string {
  if (!iso) return '—'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '—'
  const deltaSeconds = Math.max(0, Math.floor((Date.now() - then) / 1000))
  if (deltaSeconds < 45) return 'just now'
  if (deltaSeconds < 3600) return `${Math.floor(deltaSeconds / 60)}m ago`
  if (deltaSeconds < 86_400) return `${Math.floor(deltaSeconds / 3600)}h ago`
  return `${Math.floor(deltaSeconds / 86_400)}d ago`
}

/**
 * Short ULID rendering for dense table cells. A ULID is time-ordered, so its
 * first characters are shared by every id created in the same window; the tail
 * is what distinguishes two rows. Keeping the tail (and marking the elision) is
 * therefore the only version that is safe to show in a table.
 */
export function shortID(id: string): string {
  if (id.length <= 8) return id
  return `…${id.slice(-8)}`
}

/** A 0..1 ratio as a whole-percent string. Used by the budget bar and tiles. */
export function formatPercent(ratio: number): string {
  if (!Number.isFinite(ratio)) return '0%'
  return `${Math.round(ratio * 100)}%`
}

/**
 * `3 boards` / `1 board` / `0 boards`. The design source always shows a plural
 * because its sample data happens to be plural; a real workspace with one board
 * must not read "1 boards".
 */
export function plural(count: number, singular: string, pluralForm = `${singular}s`): string {
  return `${count} ${count === 1 ? singular : pluralForm}`
}
