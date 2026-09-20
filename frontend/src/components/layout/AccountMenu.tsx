import { useEffect, useRef, useState } from 'react'
import { LogOut, Settings, UserRound } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { AvatarMonogram } from '@/components/ui/avatar'
import { useT } from '@/hooks/use-t'
import { useAppSelector } from '@/store/hooks'
import { activeRole } from '@/store/slices/sessionSlice'
import { useLogoutMutation } from '@/store/api/session'

/**
 * The account menu hanging off the rail's avatar (US-AD89 AC5).
 *
 * AC5 is about *reachability*: the profile must open from the avatar menu and
 * must not require `owner`/`admin`. So the menu is the only navigation that
 * leads to `/settings/profile`, and it renders for every signed-in role — the
 * role is displayed, never used as a gate. The RBAC minimum for the page itself
 * is `viewer` (ARCHITECTURE 6.2.2), which every member holds.
 *
 * It is a real popover rather than a `<select>` or a `window.confirm`: the
 * contract bans the blocking dialogs, and a menu that lists identity, the two
 * settings screens, and sign-out is what the design's "Buka menu profil" title
 * promises. Escape closes it and returns focus to the trigger, and a click
 * outside closes it, so it never traps the operator.
 */
export function AccountMenu() {
  const t = useT()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)

  const name = useAppSelector((state) => state.session.name)
  const email = useAppSelector((state) => state.session.email)
  const avatar = useAppSelector((state) => state.session.avatar)
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const workspace = useAppSelector((state) =>
    state.session.workspaces.find((candidate) => candidate.id === state.session.activeOrgID),
  )
  const role = useAppSelector((state) => activeRole(state.session))
  const [logout] = useLogoutMutation()

  useEffect(() => {
    if (!open) return

    function onPointerDown(event: MouseEvent) {
      if (containerRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      setOpen(false)
      triggerRef.current?.focus()
    }

    document.addEventListener('mousedown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  async function signOut() {
    await logout()
      .unwrap()
      .catch(() => undefined)
    setOpen(false)
    navigate('/login', { replace: true })
  }

  return (
    <div ref={containerRef} className="relative">
      <button
        ref={triggerRef}
        type="button"
        title={t['profile.menu']}
        aria-label={t['profile.menu']}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex items-center justify-center rounded-full"
      >
        <AvatarMonogram avatar={avatar} fallbackName={name} ring />
      </button>

      {open ? (
        <div
          role="menu"
          aria-label={t['profile.menuOpen']}
          className="absolute bottom-0 left-[calc(100%+10px)] z-40 w-[232px] rounded-[10px] border border-[var(--color-border-standard)] bg-[var(--color-surface-elevated)] p-2 shadow-[0_10px_24px_-6px_rgba(12,26,22,0.18)]"
        >
          <div className="border-b border-[var(--color-border-subtle)] px-2 pb-2">
            <div className="truncate text-[12px] font-semibold text-[var(--color-primary)]">{name ?? '—'}</div>
            <div className="truncate font-mono text-[10px] text-[var(--color-tertiary)]">{email ?? '—'}</div>
            {workspace ? (
              <div className="mt-1 flex items-center gap-1.5">
                <span className="truncate font-mono text-[10px] text-[var(--color-accent)]">{workspace.slug}</span>
                {role ? (
                  <span className="rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase text-[var(--color-secondary)]">
                    {role}
                  </span>
                ) : null}
              </div>
            ) : null}
          </div>

          <MenuLink
            to={activeOrgID ? `/app/${activeOrgID}/settings/profile` : '/settings/profile'}
            icon={<UserRound size={14} />}
            label={t['profile.title']}
            onNavigate={() => setOpen(false)}
          />
          <MenuLink
            to={activeOrgID ? `/app/${activeOrgID}/settings/workspace` : '/settings/workspace'}
            icon={<Settings size={14} />}
            label={t['workspace.title']}
            onNavigate={() => setOpen(false)}
          />

          <button
            type="button"
            role="menuitem"
            onClick={signOut}
            className="mt-1 flex w-full items-center gap-2 rounded-[6px] border-t border-[var(--color-border-subtle)] px-2 py-1.5 pt-2 text-left text-[12px] text-[var(--color-secondary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]"
          >
            <LogOut size={14} />
            {t['auth.logout']}
          </button>
        </div>
      ) : null}
    </div>
  )
}

function MenuLink({
  to,
  icon,
  label,
  onNavigate,
}: {
  to: string
  icon: React.ReactNode
  label: string
  onNavigate: () => void
}) {
  return (
    <Link
      to={to}
      role="menuitem"
      onClick={onNavigate}
      className="flex items-center gap-2 rounded-[6px] px-2 py-1.5 text-[12px] text-[var(--color-secondary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]"
    >
      {icon}
      {label}
    </Link>
  )
}
