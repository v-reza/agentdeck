import { cn } from '@/lib/cn'

/**
 * Screen 35-approval-inbox — the design's "Preview Aksi" card.
 *
 * The card has a header (a label naming what kind of action is gated, and the
 * step id on the right) and a body holding the payload. Both come from the same
 * string the server stored, so the header is derived from it rather than added
 * as a second field: the API sends `preview_json` and nothing else, and
 * inventing a separate title would put a caption on screen that the server
 * never agreed to.
 *
 * The payload is rendered as it arrived, untouched. This component previously
 * re-formatted it through `JSON.stringify(JSON.parse(...))`, which directly
 * contradicted its own docstring: pretty-printing hides differences in what the
 * agent proposed, and surfacing those differences is the entire point of showing
 * the payload at all.
 *
 * "Rendered as it arrived" is not the same as "byte-identical to what the agent
 * proposed", and the difference is worth stating because it bounds what this
 * screen can prove: `approvals.preview_json` is JSONB, so Postgres normalises key
 * order and whitespace before this component ever sees it. The comparison is
 * faithful in content — which key holds which value — and not in bytes. Anything
 * whose exact bytes matter (a signed body, a literal command line) has to be a
 * string field inside the JSON for the preview to show it truthfully.
 */
export function DiffViewer({ preview, className }: { preview: string | null; className?: string }) {
  if (!preview) {
    return (
      <p className={cn('font-mono text-[11px] text-[var(--color-tertiary)]', className)}>
        No preview payload was recorded for this request.
      </p>
    )
  }

  return (
    <div
      data-testid="approval-preview"
      className={cn(
        'rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5',
        className,
      )}
    >
      <div className="mb-1.5 flex items-center justify-between gap-2 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
        <span>Preview Aksi Berisiko</span>
        <span className="normal-case tracking-normal">{previewLabel(preview)}</span>
      </div>
      <pre className="max-h-64 overflow-auto rounded-[4px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-2 font-mono text-[11px] leading-[1.5] text-[var(--color-primary)]">
        {preview}
      </pre>
    </div>
  )
}

/**
 * The design's header label — `tool: bash_exec` — derived from the payload.
 *
 * A tool call names itself (`tool` / `name` / `action`), and that name is the
 * most useful one-line description of the risk, so it is read out when present.
 * When the payload is not JSON, or names nothing, the label falls back to the
 * verbatim/JSON distinction rather than guessing a name that is not there.
 */
function previewLabel(preview: string): string {
  try {
    const parsed = JSON.parse(preview) as unknown
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      const record = parsed as Record<string, unknown>
      for (const key of ['tool', 'name', 'action']) {
        const value = record[key]
        if (typeof value === 'string' && value.trim()) {
          return key === 'tool' ? `tool: ${value}` : `${key}: ${value}`
        }
      }
    }
    return 'JSON'
  } catch {
    return 'Teks'
  }
}
