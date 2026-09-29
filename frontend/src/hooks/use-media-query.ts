import { useEffect, useState } from 'react'

/**
 * A media query as React state (US-AD60 AC3).
 *
 * The board needs to choose between two LAYOUTS, not two styles. A Tailwind
 * `md:` class can hide one and show the other, but both trees would still mount,
 * both would run their hooks, and the accordion's "which column is open" state
 * would exist on desktop where nothing renders it. The breakpoint is a decision
 * the component has to make, so it is read as a value.
 *
 * `matchMedia` rather than a resize listener: it fires only when the query's
 * result actually changes, so a desktop window being dragged does not re-render
 * the board on every pixel. It also reports the initial value synchronously,
 * which a resize listener cannot.
 *
 * SSR-safe by construction (`typeof window` guard) even though this app is
 * client-only: the guard is one line and the alternative is a crash the first
 * time anything renders this on the server.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() =>
    typeof window === 'undefined' ? false : window.matchMedia(query).matches,
  )

  useEffect(() => {
    if (typeof window === 'undefined') return
    const list = window.matchMedia(query)
    const onChange = (event: MediaQueryListEvent) => setMatches(event.matches)
    // Re-read on subscribe: the query can have changed between the initial render
    // and this effect (a rotation during hydration), and the state would be stale.
    setMatches(list.matches)
    list.addEventListener('change', onChange)
    return () => list.removeEventListener('change', onChange)
  }, [query])

  return matches
}

/** US-AD60's breakpoint, named once so the two layouts cannot disagree. */
export const MOBILE_QUERY = '(max-width: 767px)'

export function useIsMobile(): boolean {
  return useMediaQuery(MOBILE_QUERY)
}
