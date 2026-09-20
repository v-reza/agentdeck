import { Lock } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { cn } from '@/lib/cn'
import { useProjects } from '@/hooks/use-projects'
import { useDirectoryTotals } from '@/hooks/use-directory-totals'
import { useProjectBoards } from '@/hooks/use-directory'
import { useAppSelector } from '@/store/hooks'
import { plural } from '@/lib/formatters'
import { activeRole } from '@/store/slices/sessionSlice'
import type { Project } from '@/lib/domain'

/**
 * The 224px workspace sidebar (DESIGN.md `shell-sidebar`, design source
 * 11-project-list "2. SIDEBAR"): workspace block, fixed navigation with counts,
 * the projects in scope with per-project board counts, and the user footer.
 *
 * It is `justify-between`, so the footer is pinned to the bottom of the column
 * and the scrollable middle is the project tree — the shell never scrolls as a
 * whole.
 *
 * It is a `<nav>`, which is what DESIGN.md `shell-sidebar` specifies: the element
 * carries the landmark role, so a 224px element that only *looks* like a sidebar
 * but announces itself as a plain group is a real accessibility regression.
 *
 * Counts are server values: projects from GET /projects, board totals from the
 * per-project board lists the directory rows fetch. A count that has not arrived
 * renders a dash, never a confident zero.
 */
export function WorkspaceSidebar() {
  const { projects } = useProjects()
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const workspace = useAppSelector((state) => state.session.workspaces.find((w) => w.id === state.session.activeOrgID))
  const name = useAppSelector((state) => state.session.name)
  const role = useAppSelector((state) => activeRole(state.session))
  const { boards: boardTotal } = useDirectoryTotals()

  if (!activeOrgID) return null

  return (
    <nav
      aria-label="Workspace"
      className="z-20 flex h-screen w-[224px] min-w-[224px] flex-col justify-between border-r border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-3.5"
    >
      <div className="flex min-h-0 flex-col gap-5">
        <div>
          <div className="mb-1 flex items-center justify-between">
            <span className="font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
              Workspace
            </span>
            <span className="inline-flex min-w-0 max-w-[108px] items-center gap-1 truncate rounded-[4px] bg-[var(--color-accent)]/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-[var(--color-accent)]">
              <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-accent)]" />
              <span className="truncate">{workspace?.slug ?? 'workspace'}</span>
            </span>
          </div>
          <div className="truncate font-mono text-[13px] font-semibold text-[var(--color-primary)]">
            {workspace?.name ?? 'Workspace'}
          </div>
          <div className="mt-0.5 flex items-center gap-1 text-[11px] text-[var(--color-tertiary)]">
            <Lock size={12} className="shrink-0" />
            <span className="font-mono text-[10px]">auto-bound</span>
          </div>
        </div>

        <nav className="flex flex-col gap-1">
          <div className="mb-1 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
            Navigation
          </div>
          <SidebarLink to={`/app/${activeOrgID}/boards`} label="All Boards" count={boardTotal} />
          <SidebarLink to={`/app/${activeOrgID}/projects`} label="All Projects" count={projects.length} />
          <SidebarLink to={`/app/${activeOrgID}/approvals`} label="Approvals" />
        </nav>

        <div className="flex min-h-0 flex-col gap-1">
          <div className="mb-1 flex items-center justify-between">
            <span className="font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
              Projects in scope
            </span>
            <span className="font-mono text-[10px] text-[var(--color-tertiary)]">Scope</span>
          </div>
          <div className="flex min-h-0 flex-col gap-0.5 overflow-y-auto">
            {projects.length === 0 ? (
              <p className="px-2.5 text-[11px] text-[var(--color-tertiary)]">No projects yet</p>
            ) : (
              projects.map((project) => <ProjectTreeRow key={project.id} project={project} orgID={activeOrgID} />)
            )}
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between border-t border-[var(--color-border-subtle)] pt-3">
        <div className="flex min-w-0 items-center gap-2">
          <div className="flex h-[26px] w-[26px] shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-sunken)] font-mono text-[11px] font-semibold text-[var(--color-secondary)]">
            {(name ?? '?').slice(0, 1).toUpperCase()}
          </div>
          <div className="flex min-w-0 flex-col">
            <span className="truncate text-[12px] font-medium leading-none text-[var(--color-primary)]">
              {name ?? 'Signed in'}
            </span>
            <span className="truncate font-mono text-[10px] leading-tight text-[var(--color-tertiary)]">
              {workspace?.name ?? '—'}
            </span>
          </div>
        </div>
        {role ? (
          <span className="rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase text-[var(--color-secondary)]">
            {role}
          </span>
        ) : null}
      </div>
    </nav>
  )
}

function ProjectTreeRow({ project, orgID }: { project: Project; orgID: string }) {
  // Same cache key the directory row uses, so this costs no extra request.
  const { boards, isUnresolved } = useProjectBoards(project.id)

  return (
    <NavLink
      to={`/app/${orgID}/projects/${project.id}`}
      className={({ isActive }) =>
        cn(
          'group flex items-center justify-between rounded-[6px] px-2.5 py-1.5 text-[12px] transition-colors',
          isActive
            ? 'bg-[var(--color-accent-tint)] font-semibold text-[var(--color-accent)]'
            : 'text-[var(--color-primary)] hover:bg-[rgba(12,26,22,0.04)]',
        )
      }
    >
      <span className="truncate font-mono text-[11px]">{project.name}</span>
      <span className="font-mono text-[10px] text-[var(--color-tertiary)] group-hover:text-[var(--color-primary)]">
        {isUnresolved ? '–' : plural(boards.length, 'board')}
      </span>
    </NavLink>
  )
}

function SidebarLink({ to, label, count }: { to: string; label: string; count?: number }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cn(
          'flex items-center justify-between rounded-[6px] px-2.5 py-1.5 text-[12px] transition-colors',
          isActive
            ? 'bg-[var(--color-accent-tint)] font-semibold text-[var(--color-accent)]'
            : 'font-medium text-[var(--color-secondary)] hover:bg-[rgba(12,26,22,0.04)] hover:text-[var(--color-primary)]',
        )
      }
    >
      <span>{label}</span>
      {count === undefined ? null : <span className="font-mono text-[11px] tabular-nums">{count}</span>}
    </NavLink>
  )
}
