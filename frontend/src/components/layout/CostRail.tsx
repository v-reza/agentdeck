import { useCostRail } from '@/hooks/use-cost-rail'
import { budgetOver, formatEstimatedMicroUSD, formatPercent } from '@/lib/formatters'

/**
 * The 264px cost rail (DESIGN.md `shell-costrail`, design source 11-project-list
 * "4. RIGHT COST RAIL"): TODAY with the 6px budget meter, the 7-DAY sparkline,
 * TOP SPENDERS (three rows), and RUNNING NOW pinned to the bottom.
 *
 * The design source is explicit — "isi TETAP ... JANGAN tambah isi lain" — so
 * this file adds nothing beyond those four sections. Every figure is a server
 * value or an explicit empty state; when a section has no data it says so rather
 * than drawing a plausible-looking bar.
 */
export function CostRail() {
  const { today, capMicros, ratio, thresholdCrossed, runningNow, week, topSpenders, emptyReason } = useCostRail()

  // The meter width is clamped, so an overspend has to be signalled by tone and
  // by the N18 threshold flag rather than by a width past 100%.
  const overBudget = budgetOver(today, capMicros) || thresholdCrossed

  return (
    <aside
      aria-label="Cost and usage"
      className="z-20 flex w-[264px] min-w-[264px] flex-col justify-between border-l border-[var(--color-border-subtle)] bg-[var(--color-surface-well)] p-4"
    >
      <div className="flex flex-col gap-6">
        <section>
          <div className="flex items-center justify-between font-mono text-[11px] uppercase text-[var(--color-tertiary)]">
            <span>Today</span>
            <span>{capMicros > 0 ? `cap ${formatEstimatedMicroUSD(capMicros)}` : 'no cap'}</span>
          </div>

          <div className="mt-1 flex items-baseline gap-2">
            <div className="font-mono text-[24px] font-bold tracking-tight text-[var(--color-primary)] tabular-nums">
              {formatEstimatedMicroUSD(today)}
            </div>
            {capMicros > 0 ? (
              <div className="font-mono text-[12px] text-[var(--color-tertiary)]">
                / {formatEstimatedMicroUSD(capMicros)}
              </div>
            ) : null}
          </div>

          {capMicros > 0 ? (
            <div className="mt-2.5 h-[6px] w-full overflow-hidden rounded-full bg-[var(--color-border-standard)]">
              <div
                className={
                  overBudget
                    ? 'h-full rounded-full bg-[var(--color-cost-over)]'
                    : 'h-full rounded-full bg-[var(--color-accent)]'
                }
                style={{ width: formatPercent(ratio) }}
              />
            </div>
          ) : null}

          {emptyReason ? (
            <p className="mt-2 text-[11px] leading-snug text-[var(--color-tertiary)]">{emptyReason}</p>
          ) : null}
        </section>

        <section>
          <div className="flex items-center justify-between font-mono text-[11px] uppercase text-[var(--color-tertiary)]">
            <span>7-day</span>
            {week.length > 0 ? (
              <span className="text-[10px] normal-case">
                {week[0].day} – {week[week.length - 1].day}
              </span>
            ) : null}
          </div>

          {week.length === 0 ? (
            <p className="mt-3 font-mono text-[10px] leading-snug text-[var(--color-tertiary)]">
              No ledger yet — per-day spend appears once runs are recorded.
            </p>
          ) : (
            <div className="mt-3 flex h-[42px] items-end justify-between gap-1.5 px-0.5">
              {week.map((bar, index) => (
                <div
                  key={bar.day}
                  title={`${bar.day}: ${formatEstimatedMicroUSD(bar.total_micros)}`}
                  style={{ height: `${bar.height}%` }}
                  className={
                    index === week.length - 1
                      ? 'flex-1 rounded-[2px] bg-[var(--color-accent)]'
                      : 'flex-1 rounded-[2px] bg-[var(--color-accent)]/70'
                  }
                />
              ))}
            </div>
          )}
        </section>

        <section>
          <div className="mb-2 flex items-center justify-between font-mono text-[11px] uppercase text-[var(--color-tertiary)]">
            <span>Top spenders</span>
            <span>Today</span>
          </div>

          {topSpenders.length === 0 ? (
            <p className="font-mono text-[10px] leading-snug text-[var(--color-tertiary)]">
              No spend recorded on this board yet.
            </p>
          ) : (
            <div className="flex flex-col gap-2.5">
              {topSpenders.map((row) => (
                <div key={row.name}>
                  <div className="mb-1 flex items-center justify-between font-mono text-[11px]">
                    <span className="truncate font-medium text-[var(--color-primary)]">{row.name}</span>
                    <span className="font-semibold text-[var(--color-accent)] tabular-nums">
                      {formatEstimatedMicroUSD(row.cost_micros)}
                    </span>
                  </div>
                  <div className="h-[3px] w-full overflow-hidden rounded-full bg-[var(--color-border-subtle)]">
                    <div className="h-full rounded-full bg-[var(--color-accent)]" style={{ width: `${row.share}%` }} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

      <section className="border-t border-[var(--color-border-subtle)] pt-3">
        <div className="mb-1 font-mono text-[11px] uppercase text-[var(--color-tertiary)]">Running now</div>
        <div className="flex items-center gap-2">
          <span
            className={
              runningNow > 0
                ? 'h-2 w-2 animate-pulse rounded-full bg-[var(--color-status-running)]'
                : 'h-2 w-2 rounded-full bg-[var(--color-border-strong)]'
            }
          />
          <span className="font-mono text-[12px] font-semibold text-[var(--color-primary)] tabular-nums">
            {runningNow}
          </span>
          <span className="text-[12px] text-[var(--color-secondary)]">agents active</span>
        </div>
      </section>
    </aside>
  )
}
