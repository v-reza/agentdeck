import { useDirectoryTotals } from '@/hooks/use-directory-totals'
import { useAppSelector } from '@/store/hooks'
import type { Project } from '@/lib/domain'

/**
 * The four summary cards (design source 11-project-list "Projects Summary Cards
 * / Overview Row"): Total Projects, Total Boards, Running Tasks, Combined Cost
 * Today — in that order, 4 columns, 10px padding, 10px radius.
 *
 * The first three are server values: projects from GET /projects, boards and
 * tasks from the directory rows' own answers, projected through
 * `directorySlice`. The fourth is a dash with its reason, because
 * `GET /orgs/{id}/cost-summary` is M2 and answers 404 — the design's `$9.250`
 * is sample data, and printing a stand-in would be inventing spend.
 */
export function ProjectSummary({ projects }: { projects: Project[] }) {
  const { boards, tasks, running } = useDirectoryTotals()
  const workspace = useAppSelector((state) => state.session.workspaces.find((w) => w.id === state.session.activeOrgID))

  return (
    <div className="grid grid-cols-4 gap-3">
      <SummaryCard
        label="Total Projects"
        value={String(projects.length)}
        hint={`All within ${workspace?.slug ?? '—'}`}
      />
      <SummaryCard
        label="Total Boards"
        value={String(boards)}
        hint={projects.length > 0 ? `Across ${projects.length} active projects` : 'No projects yet'}
      />
      <SummaryCard
        label="Running Tasks"
        value={String(running)}
        tone="warning"
        pulse={running > 0}
        hint={`${tasks} total tasks`}
      />
      <SummaryCard
        label="Combined Cost Today"
        value="—"
        tone="accent"
        hint="Rincian biaya per board menyusul di halaman biaya"
      />
    </div>
  )
}

function SummaryCard({
  label,
  value,
  hint,
  tone,
  pulse,
}: {
  label: string
  value: string
  hint: string
  tone?: 'accent' | 'warning'
  pulse?: boolean
}) {
  return (
    <div className="rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-[10px] shadow-sm">
      <div className="font-mono text-[10px] uppercase text-[var(--color-tertiary)]">{label}</div>
      <div
        className={[
          'mt-0.5 flex items-center gap-1.5 font-mono text-[18px] font-semibold',
          tone === 'accent'
            ? 'text-[var(--color-accent)]'
            : tone === 'warning'
              ? 'text-[var(--color-warning)]'
              : 'text-[var(--color-primary)]',
        ].join(' ')}
      >
        {pulse ? (
          <span className="h-2 w-2 animate-pulse rounded-full bg-[var(--color-warning)]" aria-hidden="true" />
        ) : null}
        {value}
      </div>
      <div className="mt-0.5 text-[10px] text-[var(--color-tertiary)]">{hint}</div>
    </div>
  )
}
