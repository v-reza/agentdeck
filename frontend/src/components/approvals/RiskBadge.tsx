import { cn } from '@/lib/cn'
import type { ApprovalGateMode } from '@/lib/domain'

/**
 * The gate mode is the risk signal (ARCHITECTURE 8, DECISIONS 4): `require`
 * means a human is the only thing between the agent and the action, `deny` means
 * the action was refused by policy, `auto` means it already ran. Colour follows
 * DESIGN.md's semantic tokens — the single accent is never used for risk.
 */
const TONE: Record<ApprovalGateMode, { label: string; className: string }> = {
  require: {
    label: 'Requires approval',
    className: 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]',
  },
  deny: {
    label: 'Denied by policy',
    className: 'bg-[var(--color-danger)]/10 text-[var(--color-danger)]',
  },
  auto: {
    label: 'Auto',
    className: 'bg-[var(--color-success)]/10 text-[var(--color-success)]',
  },
}

export function RiskBadge({ mode }: { mode: ApprovalGateMode }) {
  const tone = TONE[mode] ?? TONE.require
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-[4px] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.06em]',
        tone.className,
      )}
    >
      {tone.label}
    </span>
  )
}
