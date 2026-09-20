import { cn } from '@/lib/cn'

/**
 * Renders the exact payload an agent proposes to apply (ARCHITECTURE 8: the
 * approval gate shows the operator what will happen, not a summary of it).
 *
 * The preview is whatever the server stored — usually a JSON object. It is shown
 * verbatim rather than parsed into fields: re-formatting it would hide the
 * difference between what the agent sent and what the operator approves, which
 * is the one thing this component exists to expose.
 */
export function DiffViewer({ preview, className }: { preview: string | null; className?: string }) {
  if (!preview) {
    return (
      <p className={cn('font-mono text-[11px] text-[var(--color-tertiary)]', className)}>
        No preview payload was recorded for this request.
      </p>
    )
  }

  let pretty = preview
  try {
    pretty = JSON.stringify(JSON.parse(preview), null, 2)
  } catch {
    // A non-JSON payload is still shown as-is; it is the real request body.
  }

  return (
    <pre
      className={cn(
        'max-h-64 overflow-auto rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-well)] p-2.5',
        'font-mono text-[11px] leading-[1.5] text-[var(--color-secondary)]',
        className,
      )}
    >
      {pretty}
    </pre>
  )
}
