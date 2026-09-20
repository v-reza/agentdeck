import { useMemo } from 'react'
import { useMatch } from 'react-router-dom'
import { useBoardBudgetQuery, useBoardLedgerQuery } from '@/store/api/finops'
import { useBoardTasks } from '@/hooks/use-directory'
import { useAppSelector } from '@/store/hooks'
import { formatMicroUSD } from '@/lib/formatters'

/**
 * Everything the cost rail renders, and where each number comes from.
 *
 * The design source fixes the rail's contents — TODAY (with the 6px budget
 * meter), 7-DAY sparkline, TOP SPENDERS (three rows), RUNNING NOW — and says
 * explicitly not to add anything else. An earlier revision of this rail grew a
 * USAGE block with run and token counters; that was removed because the design
 * forbids it and because the same numbers already live on the board page.
 *
 * ARCHITECTURE 6.2.14 exposes cost per *board*: `GET /boards/{id}/budget` for
 * realtime spend against the cap and `GET /boards/{id}/ledger` for line items.
 * Both are M2 and answer 404 today (verified against the running API), so:
 *
 *  - TODAY renders 0 with the explicit "open a board" hint when no board is in
 *    scope, and the real figures as soon as the endpoint exists.
 *  - 7-DAY and TOP SPENDERS render their empty state, because a bar chart of
 *    invented heights is worse than an empty chart — it looks like data.
 *
 * Nothing here is estimated, extrapolated, or carried over between boards.
 */
export interface WeekBar {
  day: string
  total_micros: number
  /** Bar height as a percentage of the busiest day, floored at 2% so a zero day still renders. */
  height: number
}

export interface SpenderRow {
  name: string
  cost_micros: number
  /** Bar width as a percentage of the top spender. */
  share: number
}

export interface CostRailData {
  boardID: string | null
  today: number
  capMicros: number
  /** Ratio of today against the cap, clamped to 1. Drives the meter width. */
  ratio: number
  /** N18: the server decides whether the 80% threshold was crossed. */
  thresholdCrossed: boolean
  runningNow: number
  week: WeekBar[]
  topSpenders: SpenderRow[]
  /** Human-readable reason the figures are zero, or null when a board is active. */
  emptyReason: string | null
}

/** Seven days ending today, oldest first, as `YYYY-MM-DD` keys. */
function lastSevenDays(): string[] {
  const days: string[] = []
  const now = new Date()
  for (let offset = 6; offset >= 0; offset -= 1) {
    const day = new Date(now)
    day.setDate(now.getDate() - offset)
    days.push(day.toISOString().slice(0, 10))
  }
  return days
}

export function useCostRail(): CostRailData {
  const boardMatch = useMatch('/app/:orgID/boards/:boardID/*')
  const boardID = boardMatch?.params.boardID ?? null
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)

  const { data: budget } = useBoardBudgetQuery(boardID ?? '', { skip: !boardID || !activeOrgID })
  const { data: ledger } = useBoardLedgerQuery(boardID ?? '', { skip: !boardID || !activeOrgID })

  // RUNNING NOW is the count of tasks the board itself reports as running —
  // the same query the directory rows use, so it costs no extra request.
  const { running } = useBoardTasks(boardID ?? '')

  const capMicros = budget?.budget_daily_micros ?? 0
  const today = budget?.spent_micros ?? 0
  const ratio = capMicros > 0 ? Math.min(today / capMicros, 1) : 0

  const week = useMemo<WeekBar[]>(() => {
    const entries = ledger ?? []
    if (entries.length === 0) return []

    const perDay = new Map<string, number>()
    for (const entry of entries) {
      const day = entry.created_at.slice(0, 10)
      perDay.set(day, (perDay.get(day) ?? 0) + entry.cost_micros)
    }

    const days = lastSevenDays()
    const totals = days.map((day) => perDay.get(day) ?? 0)
    const busiest = Math.max(...totals, 0)

    return days.map((day, index) => ({
      day,
      total_micros: totals[index],
      height: busiest > 0 ? Math.max(2, Math.round((totals[index] / busiest) * 100)) : 2,
    }))
  }, [ledger])

  const topSpenders = useMemo<SpenderRow[]>(() => {
    const entries = ledger ?? []
    if (entries.length === 0) return []

    const perModel = new Map<string, number>()
    for (const entry of entries) {
      perModel.set(entry.model, (perModel.get(entry.model) ?? 0) + entry.cost_micros)
    }

    const ranked = [...perModel.entries()].sort((a, b) => b[1] - a[1]).slice(0, 3)
    const top = ranked[0]?.[1] ?? 0

    return ranked.map(([name, cost_micros]) => ({
      name,
      cost_micros,
      share: top > 0 ? Math.round((cost_micros / top) * 100) : 0,
    }))
  }, [ledger])

  return {
    boardID,
    today,
    capMicros,
    ratio,
    thresholdCrossed: budget?.threshold_crossed ?? false,
    runningNow: boardID ? running : 0,
    week,
    topSpenders,
    emptyReason: boardID ? null : 'Open a board to see its daily budget, spend, and running agents.',
  }
}

/** The budget cap rendered for the rail header, e.g. "cap $20.00". */
export function capLabel(capMicros: number): string {
  return capMicros > 0 ? `cap ${formatMicroUSD(capMicros)}` : 'no cap set'
}
