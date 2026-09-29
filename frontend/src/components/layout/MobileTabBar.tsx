import { LayoutGrid, Folder, ListChecks, DollarSign, Settings, type LucideIcon } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { cn } from '@/lib/cn'
import { useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'

/**
 * The bottom tab bar for widths below 768px (design source: 46-mobile-board).
 *
 * WHY THIS HAS TO EXIST FOR US-AD60 TO BE TRUE: the desktop shell is 44px of rail
 * plus 224px of sidebar plus a 264px cost rail — 532px of chrome before any
 * content. At a 390px viewport the content pane is narrower than zero and the cost
 * rail paints ON TOP of the board, intercepting clicks. "No horizontal scroll"
 * (AC1) is not reachable while that shell is on screen, and the design agrees:
 * the mobile mockup shows no rail, no sidebar and no cost rail, only a topbar and
 * a bottom bar.
 *
 * DELIBERATE DEVIATION FROM THE DESIGN: the mockup's five tabs are board-scoped
 * (Board / Table / Graf / Biaya / Setelan) because the mockup IS the board screen.
 * These five are app destinations instead. The board's own view switcher already
 * lives in `BoardToolbar` and is reachable on mobile, and putting a second copy of
 * it here would mean two controls for one piece of state — the toolbar's would
 * still be on screen right above this bar. The structural claim the design makes
 * (no side chrome, navigation moves to the bottom, five slots) is what is kept.
 */
export function MobileTabBar() {
  const t = useT()
  const orgID = useAppSelector((state) => state.session.activeOrgID)
  if (!orgID) return null
  const base = `/app/${orgID}`

  const items: { to: string; icon: LucideIcon; label: string }[] = [
    { to: `${base}/boards`, icon: LayoutGrid, label: t['nav.boards'] },
    { to: `${base}/projects`, icon: Folder, label: t['nav.projects'] },
    { to: `${base}/approvals`, icon: ListChecks, label: t['nav.approvals'] },
    { to: `${base}/cost`, icon: DollarSign, label: t['nav.finops'] },
    { to: `${base}/settings`, icon: Settings, label: t['nav.settings'] },
  ]

  return (
    <nav
      aria-label={t['nav.primary']}
      data-testid="mobile-tab-bar"
      className="z-30 flex h-[var(--spacing-tabbar)] min-h-[var(--spacing-tabbar)] items-stretch border-t border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] pb-[env(safe-area-inset-bottom)]"
    >
      {items.map(({ to, icon: Icon, label }) => (
        <NavLink
          key={to}
          to={to}
          aria-label={label}
          className={({ isActive }) =>
            cn(
              'flex flex-1 flex-col items-center justify-center gap-0.5 transition-colors',
              isActive ? 'text-[var(--color-accent)]' : 'text-[var(--color-tertiary)]',
            )
          }
        >
          <Icon size={18} strokeWidth={1.75} aria-hidden="true" />
          <span className="text-[10px] font-medium">{label}</span>
        </NavLink>
      ))}
    </nav>
  )
}
