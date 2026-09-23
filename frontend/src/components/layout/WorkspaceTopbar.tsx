import type { ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { setActiveOrg } from '@/store/slices/sessionSlice'
import { baseApi } from '@/store/api/base'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'
import { LangToggle } from './LangToggle'
import { Combobox } from '@/components/ui/combobox'

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
    <header className="flex h-[var(--spacing-topbar)] min-h-[var(--spacing-topbar)] items-center justify-between border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-4">
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
          // The repo has one dropdown component, and this is it. It used to be a
          // native `<select>` with its own border, radius and 10px type — the
          // only control in the shell that did not come from the design system,
          // sitting in the same bar as buttons that do.
          //
          // The width cap moves to the root so the trigger does not stretch to
          // the bar's full width; the popup is `w-full min-w-max`, so it can
          // still be wider than the trigger when a workspace name is long.
          <Combobox
            className="w-[168px]"
            label={t['workspace.switcher']}
            value={activeOrgID ?? ''}
            onChange={switchWorkspace}
            options={workspaces.map((workspace) => ({ value: workspace.id, label: workspace.name }))}
          />
        ) : null}
        <LangToggle />
      </div>
    </header>
  )
}
