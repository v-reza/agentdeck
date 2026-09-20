import { useListProjectsQuery } from '@/store/api/boards'
import { useDirectoryTotals } from '@/hooks/use-directory-totals'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { ProjectDirectory } from './ProjectDirectory'
import { ProjectSummary } from './ProjectSummary'
import { CreateProjectForm } from './CreateProjectForm'

/**
 * Screen 11-project-list. Composition only: topbar, summary row, the grouped
 * directory table, and the totals footer. Every piece is a shared component,
 * which is the point of the §18.2 structure — the revision this replaces had the
 * whole shell inlined in one file.
 */
export function ProjectList() {
  const { data } = useListProjectsQuery()
  const projects = data ?? []
  const { boards, tasks, running } = useDirectoryTotals()

  return (
    <>
      <WorkspaceTopbar
        title="Projects"
        path="/projects"
        subtitle={`${projects.length} in this workspace`}
        right={<CreateProjectForm />}
      />

      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <div className="rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-3 py-2 font-mono text-[11px] text-[var(--color-secondary)]">
          Scope: this view lists every project and its boards inside{' '}
          <span className="font-semibold text-[var(--color-primary)]">
            {projects.length === 0 ? 'this workspace' : `${projects.length} projects`}
          </span>{' '}
          — nothing crosses the tenant boundary (US-AD91 AC4).
        </div>

        <ProjectSummary projects={projects} />
        <ProjectDirectory />

        <div className="flex h-9 items-center justify-between rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-4 font-mono text-[11px] text-[var(--color-secondary)]">
          <div className="flex items-center gap-3">
            <span>{projects.length} projects total</span>
            <span>•</span>
            <span>{boards} boards total</span>
            <span>•</span>
            <span>{tasks} tasks</span>
          </div>
          <div className="flex items-center gap-4">
            <span>
              Total running: <strong className="font-semibold text-[var(--color-warning)]">{running} tasks</strong>
            </span>
            <span>
              Total today: <strong className="font-bold text-[var(--color-accent)]">—</strong>
            </span>
          </div>
        </div>
      </div>
    </>
  )
}
