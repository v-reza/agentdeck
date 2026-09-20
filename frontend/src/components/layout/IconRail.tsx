import { LayoutGrid, Folder, ListChecks, Bot, DollarSign, Settings, type LucideIcon } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { AccountMenu } from '@/components/layout/AccountMenu'
import { cn } from '@/lib/cn'
import { useAppSelector } from '@/store/hooks'

/**
 * The 44px icon rail (DESIGN.md `shell-rail`: surface-sunken, width 44px).
 *
 * It carries the product mark and the six top-level destinations, in the order
 * the design source shows. `AppIcon` from the earlier revision is gone: a
 * hand-rolled SVG registry duplicated what lucide-react already ships, and the
 * design source's icons are drawn on the same 16px grid.
 *
 * The daemon dot is the only live element: it mirrors the SSE connection so an
 * operator can tell "nothing is happening" from "the stream is down".
 */
export interface IconRailProps {
  daemonConnected: boolean
}

export function IconRail({ daemonConnected }: IconRailProps) {
  const orgID = useAppSelector((state) => state.session.activeOrgID)
  const base = orgID ? `/app/${orgID}` : ''

  return (
    <nav
      aria-label="Primary"
      className="z-30 flex h-screen w-[44px] min-w-[44px] flex-col items-center border-r border-[var(--color-border-subtle)] bg-[var(--color-surface-sunken)] py-2.5"
    >
      <div className="mb-5 flex h-7 w-7 items-center justify-center rounded-[6px] bg-[var(--color-accent)] font-mono text-[11px] font-bold tracking-[-0.05em] text-[var(--color-on-accent)]">
        AD
      </div>

      <div className="flex w-full flex-col items-center gap-3">
        <RailItem to={`${base}/boards`} icon={LayoutGrid} label="Boards" />
        <RailItem to={`${base}/projects`} icon={Folder} label="Projects" />
        <RailItem to={`${base}/approvals`} icon={ListChecks} label="Approvals" />
        <RailItem to={`${base}/agents`} icon={Bot} label="Agents" />
        <RailItem to={`${base}/cost`} icon={DollarSign} label="Cost & Usage" />
      </div>

      <div className="mt-auto flex flex-col items-center gap-3">
        <RailItem to={`${base}/settings`} icon={Settings} label="Settings" />
        {/* US-AD89 AC5: the profile opens from the avatar menu, for every role.
            The avatar itself is the server's `avatar_user` (AC1), so the rail and
            the profile screen can never draw two different monograms. */}
        <AccountMenu />
        <div
          aria-label={daemonConnected ? 'Event stream connected' : 'Event stream offline'}
          title={daemonConnected ? 'Event stream connected' : 'Event stream offline'}
          className={cn(
            'h-2 w-2 rounded-full',
            daemonConnected ? 'bg-[var(--color-status-done)]' : 'bg-[var(--color-border-strong)]',
          )}
        />
      </div>
    </nav>
  )
}

function RailItem({ to, icon: Icon, label }: { to: string; icon: LucideIcon; label: string }) {
  return (
    <NavLink
      to={to}
      title={label}
      aria-label={label}
      className={({ isActive }) =>
        cn(
          'flex h-8 w-8 items-center justify-center rounded-[6px] transition-colors',
          isActive
            ? 'bg-[var(--color-accent-tint)] text-[var(--color-accent)]'
            : 'text-[var(--color-tertiary)] hover:bg-[rgba(12,26,22,0.05)] hover:text-[var(--color-primary)]',
        )
      }
    >
      <Icon size={16} strokeWidth={1.75} />
    </NavLink>
  )
}
