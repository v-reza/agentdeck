import type { ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { setActiveOrg } from '@/store/slices/sessionSlice'
import { baseApi } from '@/store/api/base'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'
import { LangToggle } from './LangToggle'

/**
 * The 52px topbar (DESIGN.md `shell-topbar`, design source 11-project-list
 * "3. TOPBAR"): breadcrumb path on the left, page actions on the right.
 *
 * The design puts a breadcrumb chip (`/projects`) beside the title, so the path
 * is passed in rather than derived from `window.location` — ARCHITECTURE 18.2
 * forbids reading the location directly, and the router already knows the path.
 * `right` stays a slot: the kanban's filter chips and the project page's "New
 * project" button are different controls and neither belongs in the shell.
 */
export interface WorkspaceTopbarProps {
  title: string
  /** Breadcrumb path as the design renders it, e.g. `/projects`. */
  path?: string
  /** Small monospace line under the title — count, route, or board slug. */
  subtitle?: string
  right?: ReactNode
}

export function WorkspaceTopbar({ title, path, subtitle, right }: WorkspaceTopbarProps) {
  const t = useT()
  const location = useLocation()
  const navigate = useNavigate()
  const dispatch = useAppDispatch()
  const workspaces = useAppSelector((state) => state.session.workspaces)
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const canSwitch = workspaces.length > 1

  function switchWorkspace(nextOrgID: string) {
    if (nextOrgID === activeOrgID) return
    dispatch(setActiveOrg(nextOrgID))
    dispatch(baseApi.util.resetApiState())
    const rest = location.pathname.replace(/^\/app(?:\/[^/]+)?/, '') || '/projects'
    navigate(`/app/${nextOrgID}${rest}`, { replace: true })
  }

  return (
    <header className="flex h-[52px] min-h-[52px] items-center justify-between border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-4">
      <div className="flex min-w-0 items-center gap-3">
        {path ? (
          <span className="rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-secondary)]">
            {path}
          </span>
        ) : null}
        <div className="flex min-w-0 items-baseline gap-2">
          <h1 className="truncate text-[15px] font-semibold tracking-tight text-[var(--color-primary)]">{title}</h1>
          {subtitle ? (
            <span className="truncate font-mono text-[11px] text-[var(--color-tertiary)]">{subtitle}</span>
          ) : null}
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-2">
        {right}
        {canSwitch ? (
          <label className="flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-wide text-[var(--color-tertiary)]">
            <span className="sr-only">{t['workspace.switcher']}</span>
            <select
              aria-label={t['workspace.switcher']}
              className="max-w-[180px] rounded-[4px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-1.5 py-1 font-mono text-[10px] normal-case tracking-normal text-[var(--color-secondary)] outline-none focus:border-[var(--color-accent)]"
              value={activeOrgID ?? ''}
              onChange={(event) => switchWorkspace(event.target.value)}
            >
              {workspaces.map((workspace) => (
                <option key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <LangToggle />
      </div>
    </header>
  )
}
