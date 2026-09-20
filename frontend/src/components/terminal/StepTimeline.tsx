import { formatRelative } from '@/lib/formatters'
import type { EventEnvelope } from '@/lib/domain'

/**
 * The append-only event timeline for one task (ARCHITECTURE 18.2
 * `components/terminal/`, screen 20-task-drawer).
 *
 * It reads the stream's own envelope type: the SSE frame and the events table
 * carry the same fields (ARCHITECTURE 7.2), so one component serves both the
 * live tail and the replayed history without a mapping layer that could drift.
 *
 * Events render in the order the server sent them, which is the order they were
 * written — the table is append-only, so sorting here would be the only thing
 * able to lie.
 */
export function StepTimeline({ events }: { events: EventEnvelope[] }) {
  if (events.length === 0) {
    return <p className="px-1 py-2 text-[12px] text-[var(--color-tertiary)]">No events recorded for this task yet.</p>
  }

  return (
    <ol className="flex flex-col">
      {events.map((event) => (
        <li
          key={event.id}
          className="flex items-baseline gap-3 border-b border-[var(--color-border-subtle)] py-2 last:border-b-0"
        >
          <span className="w-[92px] shrink-0 font-mono text-[10px] text-[var(--color-tertiary)]">
            {formatRelative(event.created_at)}
          </span>
          <span className="font-mono text-[11px] font-medium text-[var(--color-primary)]">{event.kind}</span>
          <Payload event={event} />
        </li>
      ))}
    </ol>
  )
}

/** The raw payload, truncated by CSS rather than by slicing the JSON string. */
function Payload({ event }: { event: EventEnvelope }) {
  if (event.payload == null) return null
  const text = typeof event.payload === 'string' ? event.payload : JSON.stringify(event.payload)
  return <code className="min-w-0 flex-1 truncate font-mono text-[10px] text-[var(--color-secondary)]">{text}</code>
}
