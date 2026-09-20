import type { ReactNode } from 'react'
import { cn } from '@/lib/cn'

/** Panel primitive. Radius 10px, padding 12px, hairline border (DESIGN.md). */
export function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        'rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)]',
        className,
      )}
    >
      {children}
    </div>
  )
}

/** Small stat tile used by the summary rows in the design source. */
export function StatTile({
  label,
  value,
  hint,
  accent,
}: {
  label: string
  value: string
  hint?: string
  accent?: boolean
}) {
  return (
    <Panel className="p-2.5">
      <div className="font-mono text-[10px] uppercase tracking-[0.04em] text-[var(--color-tertiary)]">{label}</div>
      <div
        className={cn(
          'mt-1 font-mono text-[20px] font-bold tabular-nums',
          accent ? 'text-[var(--color-accent)]' : 'text-[var(--color-primary)]',
        )}
      >
        {value}
      </div>
      {hint ? <div className="mt-0.5 text-[11px] text-[var(--color-tertiary)]">{hint}</div> : null}
    </Panel>
  )
}

/** Empty and error states, so every list renders the same words. */
export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="rounded-[10px] border border-dashed border-[var(--color-border-standard)] p-6 text-center">
      <p className="text-[13px] font-medium text-[var(--color-secondary)]">{title}</p>
      {hint ? <p className="mt-1 text-[12px] text-[var(--color-tertiary)]">{hint}</p> : null}
    </div>
  )
}
