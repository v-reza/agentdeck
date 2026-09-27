import type { Lang } from './domain'

/**
 * Date and number formatting. The wire format is RFC 3339
 * (`time.RFC3339Nano`, see `cmd/api/boards.go` `toProjectResponse`), so every
 * timestamp on screen goes through here rather than being sliced with
 * `String.prototype.slice` — a raw `2026-09-19T04:31:17.123456Z` on a table row
 * is unreadable, and a hand-rolled slice silently renders UTC as if it were local.
 *
 * `id-ID` is the product's default locale (Annex A / i18n), so the Indonesian
 * month names are the expected output, not a fallback.
 */

const LOCALES: Record<Lang, string> = { en: 'en-GB', id: 'id-ID' }

function parse(iso: string): Date | null {
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? null : date
}

/** `19 Sep 2026` — a table cell. */
export function formatDate(iso: string, lang: Lang): string {
  const date = parse(iso)
  if (!date) return '—'
  return new Intl.DateTimeFormat(LOCALES[lang], {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }).format(date)
}

/** `19 Sep 2026, 11.31` — a detail line. */
export function formatDateTime(iso: string, lang: Lang): string {
  const date = parse(iso)
  if (!date) return '—'
  return new Intl.DateTimeFormat(LOCALES[lang], {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

/** `"3"` / `"12"` — counts. Grouping matters once a roster or a run list is long. */
export function formatCount(value: number, lang: Lang): string {
  return new Intl.NumberFormat(LOCALES[lang]).format(value)
}

/**
 * `{0}`-style substitution for dictionary strings.
 *
 * Deliberately not `Intl.MessageFormat`: the dictionaries are flat key → string
 * maps and every interpolation here is positional. Values are inserted as
 * literal strings, never re-scanned, so a name containing `{1}` cannot pull a
 * neighbouring argument.
 */
export function interpolate(template: string, values: string[]): string {
  return template.replace(/\{(\d+)\}/g, (whole, index: string) => values[Number(index)] ?? whole)
}

/**
 * `1.5 MB` — a file size (US-AD48 AC1).
 *
 * Binary units (1024), matching what object storage and the API's own 100 MB
 * quota mean by "MB". `Intl.NumberFormat` gives the locale's decimal separator
 * for free, so `1,5 MB` in Indonesian and `1.5 MB` in English come from the
 * same line.
 */
export function formatBytes(bytes: number, lang: Lang): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  const digits = unit === 0 ? 0 : value < 10 ? 1 : 0
  return `${new Intl.NumberFormat(LOCALES[lang], {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value)} ${units[unit]}`
}
