import { useState } from 'react'
import { BadgeCheck, Check, Info, Pencil, ShieldCheck, Smile } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'
import { Panel } from '@/components/ui/card'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { useT } from '@/hooks/use-t'
import { useActionForm } from '@/hooks/use-action-form'
import { useAppSelector } from '@/store/hooks'
import { useUpdateOrgMutation } from '@/store/api/session'
import { SkeletonRows } from '@/components/ui/skeleton'

/**
 * Workspace settings (37-workspace-settings, US-AD03/US-AD77).
 *
 * The design is a summary screen. Values that do not have a server endpoint yet
 * are explicitly represented as unavailable; fabricated resource counts and
 * costs would turn a visual mock into false operational data.
 */
export function WorkspaceSettings() {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const workspaces = useAppSelector((state) => state.session.workspaces)
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const workspace = workspaces.find((candidate) => candidate.id === activeOrgID)
  const org = workspace
  const canRename = org?.role === 'owner'
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(org?.name ?? '')
  const [updateOrg] = useUpdateOrgMutation()

  const [state, formAction, isPending] = useActionForm(
    updateOrg,
    (form) => ({ orgID: activeOrgID ?? '', name: String(form.get('name') ?? '') }),
    () => setOpen(false),
  )

  if (!org) {
    return (
      <>
        <WorkspaceTopbar title={t['workspace.title']} path="/settings/workspace" />
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
          <SkeletonRows rows={3} columns={2} />
        </div>
      </>
    )
  }

  function openRename() {
    if (!org) return
    setName(org.name)
    setOpen(true)
  }

  return (
    <>
      <WorkspaceTopbar
        title={t['workspace.title']}
        path="/settings/workspace"
        subtitle={org.name}
        right={
          canRename ? (
            <Button size="sm" variant="secondary" onClick={openRename}>
              <Pencil size={13} />
              {t['workspace.editName']}
            </Button>
          ) : null
        }
      />
      <main className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-5">
        <Panel className="p-2.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="inline-flex items-center gap-1 rounded-[4px] border border-[var(--color-accent)]/30 bg-[var(--color-accent-tint)] px-2 py-0.5 font-mono text-[11px] font-bold text-[var(--color-accent)]">
              <BadgeCheck size={14} /> {t['workspace.badge']}
            </span>
            <span className="rounded-[4px] bg-[var(--color-surface-sunken)] px-2 py-0.5 font-mono text-[11px] text-[var(--color-tertiary)]">
              {t['workspace.subtitle']}
            </span>
          </div>
          <h1 className="mt-1.5 text-[16px] font-bold tracking-tight text-[var(--color-primary)]">
            {t['workspace.title']}
          </h1>
          <p className="mt-1 text-[12px] leading-relaxed text-[var(--color-secondary)]">{t['workspace.description']}</p>
        </Panel>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <SummaryCard
            icon={<Smile size={16} />}
            title={t['workspace.soloTitle']}
            text={t['workspace.soloText']}
            status={t['workspace.soloStatus']}
          />
          <SummaryCard
            icon={<Info size={16} />}
            title={t['workspace.cleanTitle']}
            text={t['workspace.cleanText']}
            status={t['workspace.cleanStatus']}
          />
        </div>

        <Panel className="space-y-3 p-2.5">
          <div className="flex items-center justify-between border-b border-[var(--color-border-subtle)] pb-2">
            <div className="flex items-center gap-2">
              <Info size={17} className="text-[var(--color-accent)]" />
              <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['workspace.summaryTitle']}</h2>
            </div>
            {canRename ? (
              <Button size="sm" variant="ghost" onClick={openRename}>
                <Pencil size={13} />
                {t['workspace.editName']}
              </Button>
            ) : null}
          </div>
          <p className="text-[11px] text-[var(--color-tertiary)]">{t['workspace.summaryHint']}</p>
          <div className="grid grid-cols-2 gap-2 text-[12px] md:grid-cols-5">
            <SummaryValue label={t['workspace.id']} value={org.id} mono />
            <SummaryValue label={t['workspace.displayName']} value={org.name} />
            <SummaryValue label={t['workspace.slug']} value={org.slug} mono />
            <SummaryValue label={t['workspace.kind']} value={kindLabel(org.kind, lang)} />
            <SummaryValue label={t['workspace.role']} value={org.role} mono />
          </div>

          <div className="pt-2">
            <h3 className="mb-2 text-[11px] font-bold uppercase tracking-[0.08em] text-[var(--color-tertiary)]">
              {t['workspace.resourceTitle']}
            </h3>
            <div className="rounded-[6px] border border-dashed border-[var(--color-border-standard)] p-3">
              <p className="font-mono text-[11px] text-[var(--color-tertiary)]">{t['workspace.resourceUnavailable']}</p>
            </div>
          </div>
        </Panel>

        <Panel className="flex items-start gap-3 p-2.5">
          <ShieldCheck size={20} className="mt-0.5 shrink-0 text-[var(--color-accent)]" />
          <div>
            <h2 className="text-[12px] font-bold text-[var(--color-primary)]">{t['workspace.securityTitle']}</h2>
            <p className="mt-0.5 text-[11px] leading-relaxed text-[var(--color-secondary)]">
              {t['workspace.securityText']}
            </p>
          </div>
        </Panel>
      </main>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={t['workspace.editTitle']}
        description={t['workspace.editDescription']}
      >
        <form action={formAction} id="workspace-name-form" className="flex flex-col gap-3.5">
          <Field label={t['workspace.displayName']}>
            <Input
              name="name"
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="organization"
            />
          </Field>
          {state.error ? (
            <p className="text-[12px] text-[var(--color-danger)]">{state.error || t['workspace.updateFailed']}</p>
          ) : null}
        </form>
        <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
          <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
            {t['action.cancel']}
          </Button>
          <Button type="submit" form="workspace-name-form" variant="primary" size="sm" disabled={isPending}>
            {isPending ? t['workspace.savePending'] : t['workspace.save']}
          </Button>
        </div>
      </Modal>
    </>
  )
}

function SummaryCard({
  icon,
  title,
  text,
  status,
}: {
  icon: React.ReactNode
  title: string
  text: string
  status: string
}) {
  return (
    <Panel className="flex flex-col justify-between p-2.5">
      <div>
        <div className="mb-1 flex items-center justify-between text-[var(--color-accent)]">{icon}</div>
        <h2 className="mt-1 text-[13px] font-bold text-[var(--color-primary)]">{title}</h2>
        <p className="mt-1 text-[12px] leading-relaxed text-[var(--color-secondary)]">{text}</p>
      </div>
      <div className="mt-3 flex items-center gap-1.5 border-t border-[var(--color-border-subtle)] pt-2 font-mono text-[11px] text-[var(--color-accent)]">
        <Check size={14} /> {status}
      </div>
    </Panel>
  )
}

function SummaryValue({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-well)] p-2.5">
      <span className="block truncate font-mono text-[10px] uppercase text-[var(--color-tertiary)]">{label}</span>
      <span
        className={`mt-0.5 block truncate text-[12px] font-medium text-[var(--color-primary)] ${mono ? 'font-mono' : ''}`}
      >
        {value}
      </span>
    </div>
  )
}

function kindLabel(kind: string, lang: 'en' | 'id'): string {
  const labels: Record<string, Record<'en' | 'id', string>> = {
    personal: { en: 'Personal / Solo', id: 'Personal / Solo' },
    manual: { en: 'Organization', id: 'Organisasi' },
  }
  return labels[kind]?.[lang] ?? kind
}
