import { cn } from '@/lib/cn'

/**
 * The loading placeholder from `43-state-loading.html`, expressed with
 * DESIGN.md's `skeleton` token instead of the mock's raw hex.
 *
 * The design draws a skeleton as a grey block: `bg-[#e8ecea] rounded-[4px]`,
 * at the height of the text it stands in for. `#e8ecea` is `surface-sunken` in
 * the token suite and 4px is `rounded.xs`, so nothing here is a magic value.
 *
 * The mock does NOT pulse, and neither does this. That is deliberate and worth
 * naming, because `animate-pulse` is the reflexive choice: a shimmering block
 * reads as "this is a different kind of thing", while a static one at the
 * content's own height reads as the content not having arrived yet. The design
 * picked the second. Motion is also the one property the repo cannot test —
 * `prefers-reduced-motion` would have to disable it anyway.
 *
 * `aria-hidden` is on every block: a screen reader announcing a dozen empty
 * boxes is worse than announcing nothing. The live region belongs to the
 * caller, which knows whether the thing is a table, a panel or a list.
 */
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cn('h-3 rounded-[4px] bg-[var(--color-surface-sunken)]', className)} />
}

/**
 * A block of placeholder lines.
 *
 * The last line is short on purpose: a paragraph of full-width bars reads as a
 * table, and real prose ends mid-line. `lines` is capped by the caller's shape,
 * not by a default, because a skeleton that guesses the wrong height is the
 * layout shift the placeholder was supposed to prevent.
 */
export function SkeletonText({ lines = 2, className }: { lines?: number; className?: string }) {
  return (
    <div aria-hidden className={cn('flex flex-col gap-1.5', className)}>
      {Array.from({ length: lines }, (_, index) => (
        <Skeleton key={index} className={index === lines - 1 && lines > 1 ? 'w-3/4' : 'w-full'} />
      ))}
    </div>
  )
}

/**
 * The row-shaped skeleton, for a table that has not loaded yet.
 *
 * `columns` is the real column count so the placeholder occupies the same grid
 * the rows will: a 3-column skeleton above a 5-column table is a visible jump.
 */
export function SkeletonRows({ rows = 5, columns = 4 }: { rows?: number; columns?: number }) {
  return (
    <div aria-hidden className="flex flex-col gap-2 p-3">
      {Array.from({ length: rows }, (_, row) => (
        <div key={row} className="flex items-center gap-3">
          {Array.from({ length: columns }, (_, column) => (
            <Skeleton key={column} className={column === 0 ? 'w-16' : 'flex-1'} />
          ))}
        </div>
      ))}
    </div>
  )
}
