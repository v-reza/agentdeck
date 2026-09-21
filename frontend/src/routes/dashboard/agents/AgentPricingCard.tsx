import { Panel } from '@/components/ui/card'
import { useT } from '@/hooks/use-t'
import { interpolate } from '@/lib/format'
import { formatEstimatedMicroUSD } from '@/lib/formatters'
import type { CatalogModel } from '@/store/api/agents'

/**
 * Which of four situations the card is reporting. They used to collapse into one
 * sentence, which is why a missing rate read as a bug: an agent with no provider,
 * a model the table does not carry, and a catalog still in flight are three
 * unrelated causes with three different fixes.
 */
export type PricingState = 'loading' | 'noProvider' | 'unpriced' | 'priced'

/**
 * The estimated-rate card of screen 28-agent-detail, split out of
 * `AgentDetailParts.tsx` so that file stays a readable size.
 *
 * Every figure goes through `formatEstimatedMicroUSD`, which spells out
 * "(estimate)": US-AD108 AC1 requires every cost number in the UI to be marked as
 * an estimate rather than a bill, and a rate the table does not carry is reported
 * as unknown instead of `$0.00` — a zero there would read as free.
 *
 * The catalog resolves a model through four tiers. Only the exact tier is
 * reproduced here: re-implementing the pattern order in the client would create a
 * second copy of the price resolution that could drift from the table the ledger
 * actually bills with. A model that only a pattern covers is therefore reported as
 * having no exact entry, which is the honest answer at this level.
 *
 * The card frame is repeated rather than imported from `AgentDetailParts`: a
 * component module importing a constant back out of its own consumer is the kind
 * of cycle that reads as a bug, and the value is one string.
 */
export function PricingCard({
  state,
  model,
  entry,
  priceVersion,
}: {
  state: PricingState
  model: string
  entry: CatalogModel | undefined
  priceVersion: number
}) {
  const t = useT()
  const badge =
    state === 'priced'
      ? t['agents.detail.pricePriced']
      : state === 'loading'
        ? t['agents.detail.priceLoading']
        : t['agents.detail.priceUnpriced']

  return (
    <Panel
      className={
        'flex flex-col justify-between rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-[14px] shadow-xs'
      }
    >
      <div>
        <div className="mb-3 flex items-center justify-between border-b border-[var(--color-border-subtle)] pb-2">
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-[var(--color-accent)]" />
            <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['agents.detail.priceTitle']}</h2>
          </div>
          <span
            data-testid="pricing-badge"
            className={`rounded-[4px] px-2 py-0.5 font-mono text-[10px] font-semibold ${
              state === 'priced'
                ? 'bg-[var(--color-accent)]/10 text-[var(--color-accent)]'
                : 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'
            }`}
          >
            {badge}
          </span>
        </div>

        {state === 'priced' && entry ? (
          <div className="grid grid-cols-3 gap-3">
            <RateBox label={t['agents.detail.priceInput']} micros={entry.input.micros_per_1m} />
            <RateBox label={t['agents.detail.priceOutput']} micros={entry.output.micros_per_1m} />
            <RateBox label={t['agents.detail.priceCached']} micros={entry.cached.micros_per_1m} />
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-[var(--color-secondary)]" data-testid="pricing-reason">
            {reasonText(state, model, t)}
          </p>
        )}
      </div>

      <div className="mt-3 flex items-start justify-between gap-3 border-t border-[var(--color-border-subtle)] pt-2 font-mono text-[10.5px] leading-snug text-[var(--color-tertiary)]">
        <span>{state === 'priced' ? t['agents.detail.priceDisclaimer'] : ''}</span>
        <span className="shrink-0">{interpolate(t['agents.detail.priceVersion'], [String(priceVersion)])}</span>
      </div>
    </Panel>
  )
}

/**
 * One sentence per cause, each naming the thing that is actually missing. The
 * model name is interpolated, so a provider carrying a hundred models says which
 * one has no entry instead of leaving the operator to guess.
 */
function reasonText(state: PricingState, model: string, t: ReturnType<typeof useT>): string {
  if (state === 'loading') return t['agents.detail.priceLoadingBody']
  if (state === 'noProvider') return t['agents.detail.priceNoProvider']
  const named = interpolate(t['agents.detail.priceUnpricedBody'], [model || t['agents.detail.priceNoModel']])
  return `${named} ${t['agents.detail.providerHint']}`
}

function RateBox({ label, micros }: { label: string; micros: number }) {
  return (
    <div className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5">
      <div className="font-mono text-[10px] uppercase text-[var(--color-tertiary)]">{label}</div>
      <div className="mt-0.5 font-mono text-[13px] font-bold tabular-nums text-[var(--color-primary)]">
        {formatEstimatedMicroUSD(micros)}
      </div>
    </div>
  )
}
