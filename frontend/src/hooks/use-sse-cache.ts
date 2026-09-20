import { useBoardEventsQuery } from '@/store/api/stream'
import type { EventEnvelope } from '@/lib/domain'

/**
 * ARCHITECTURE 18.2: "Realtime SSE | RTK Query `onCacheEntryAdded` → stream
 * menambal cache langsung". The subscription itself lives in
 * `store/api/stream.ts`; this hook is the component-facing side, so a view can
 * read the live tail without owning an EventSource.
 *
 * `boardID` empty means "not subscribed" — the query is skipped rather than
 * pointed at a placeholder id, which would open a stream for a board that does
 * not exist.
 */
export function useSseCache(boardID: string | null): { events: EventEnvelope[]; live: boolean } {
  const { data, isSuccess } = useBoardEventsQuery(boardID ?? '', { skip: !boardID })
  return { events: data ?? [], live: Boolean(boardID) && isSuccess }
}
