import { useEffect, useState } from 'react'
import { useAppSelector } from '@/store/hooks'
import { SSE_EVENTS_PATH } from '@/store/api/stream'

/**
 * The liveness of the event stream, for the rail's status dot.
 *
 * ARCHITECTURE 7.3: the browser opens one EventSource per workspace and
 * reconnects automatically with `Last-Event-ID`, so this hook only reports what
 * the connection is doing — it never retries by hand and never polls. When the
 * backend has not deployed the stream yet the connection simply stays down and
 * the dot stays grey, which is the truthful state.
 */
export interface BoardHealth {
  connected: boolean
  lastEventAt: string | null
}

export function useBoardHealth(): BoardHealth {
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const [connected, setConnected] = useState(false)
  const [lastEventAt, setLastEventAt] = useState<string | null>(null)

  useEffect(() => {
    if (!activeOrgID) return

    const source = new EventSource(SSE_EVENTS_PATH)
    const onOpen = () => setConnected(true)
    const onError = () => setConnected(false)
    const onMessage = () => setLastEventAt(new Date().toISOString())

    source.addEventListener('open', onOpen)
    source.addEventListener('error', onError)
    source.addEventListener('message', onMessage)

    return () => {
      source.removeEventListener('open', onOpen)
      source.removeEventListener('error', onError)
      source.removeEventListener('message', onMessage)
      source.close()
      setConnected(false)
    }
  }, [activeOrgID])

  return { connected, lastEventAt }
}
