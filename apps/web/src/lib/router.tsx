import { useEffect, useState, type ReactNode } from 'react'

/**
 * Dependency-free pathname router.
 *
 * Reads window.location.pathname on mount and after every `popstate` event,
 * so client-side navigation is a plain anchor navigation. Keep it that way.
 */

export function usePathname(): string {
  const [pathname, setPathname] = useState<string>(() => window.location.pathname)

  useEffect(() => {
    const sync = () => setPathname(window.location.pathname)
    window.addEventListener('popstate', sync)
    return () => window.removeEventListener('popstate', sync)
  }, [])

  return pathname
}

export type LinkProps = {
  href: string
  children: ReactNode
  className?: string
}

export function Link({ href, children, className = '' }: LinkProps) {
  return (
    <a href={href} className={className}>
      {children}
    </a>
  )
}
