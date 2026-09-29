import { useMemo, useState } from 'react'
import { Eye, FileCode2, Pencil, Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'
import { EmptyState } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { renderMarkdown } from '@/lib/markdown'
import { formatDateTime } from '@/lib/format'
import { useAppSelector } from '@/store/hooks'
import {
  useListAgentSkillsQuery,
  useCreateAgentSkillMutation,
  useUpdateAgentSkillMutation,
  useDeleteAgentSkillMutation,
  useGetSkillAgentsQuery,
  type AgentSkill,
} from '@/store/api/agents'

/**
 * Screen 26b-agent-skills — the workspace's skill library (US-AD107).
 *
 * The skill body is markdown and is PREVIEWED as rendered markdown (AC2), not
 * shown as source. The rendering goes through `lib/markdown.ts`, which escapes
 * every piece of text before applying any structure, so AC3 holds by
 * construction rather than by a sanitizer allow-list. That is the one decision in
 * this screen worth stating: the preview uses `dangerouslySetInnerHTML` because
 * that is how React takes HTML, and the only value it is ever given is the output
 * of a function whose contract is "nothing from the input reaches the output as
 * markup".
 *
 * WHO CAN WRITE (AC4): the create/edit/delete controls are gated on `admin`, and
 * the server gates the same routes independently — `POST`/`PATCH`/`DELETE
 * /agent-skills` are `auth.Admin`. A viewer sees the same library and no controls.
 *
 * WHAT THE CONTRACT DOES NOT ALLOW, and is therefore not built:
 *   - No "duplicate skill" action: the slug is unique per org and is what agents
 *     store, so a copy would have to invent a slug.
 *   - No version history: `version` rises on every edit (AC6) but the API keeps
 *     no previous bodies, so a history panel would have nothing to list.
 *   - No slug editing: `PATCH` deliberately omits it. The form shows the slug as
 *     read-only after creation and says why.
 *   - A system skill cannot be deleted (409). The delete control is disabled with
 *     that reason rather than failing on click.
 */
export function SkillLibrary() {
  const t = useT()
  const canAdmin = useCanAct('admin')
  const { data, isLoading } = useListAgentSkillsQuery()
  const [createSkill] = useCreateAgentSkillMutation()
  const [updateSkill] = useUpdateAgentSkillMutation()
  const [deleteSkill] = useDeleteAgentSkillMutation()

  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState(false)

  const skills = useMemo(() => data ?? [], [data])
  const selected = skills.find((skill) => skill.id === selectedID) ?? skills[0]

  return (
    <>
      <WorkspaceTopbar
        title={t['skills.title']}
        path="/skills"
        subtitle={skills.length > 0 ? t['skills.count'].replace('{count}', String(skills.length)) : undefined}
        right={
          canAdmin ? (
            <Button size="sm" onClick={() => setCreating(true)} data-testid="skill-create">
              <Plus size={12} />
              {t['skills.add']}
            </Button>
          ) : null
        }
      />

      <div className="flex min-h-0 flex-1">
        <div className="flex w-[300px] min-w-[300px] flex-col overflow-y-auto border-r border-[var(--color-border-subtle)]">
          {isLoading ? (
            <div className="flex flex-col gap-2 p-3">
              {[0, 1, 2, 3].map((row) => (
                <Skeleton key={row} className="w-full" />
              ))}
            </div>
          ) : skills.length === 0 ? (
            <div className="p-3">
              <EmptyState title={t['skills.empty']} hint={t['skills.emptyHint']} />
            </div>
          ) : (
            <ul className="flex flex-col">
              {skills.map((skill) => (
                <li key={skill.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedID(skill.id)}
                    data-testid={`skill-row-${skill.slug}`}
                    data-system={skill.is_system ? 'true' : 'false'}
                    className={
                      'flex w-full flex-col items-start gap-0.5 border-b border-[var(--color-border-subtle)] px-3 py-2.5 text-left transition-colors ' +
                      (skill.id === selected?.id
                        ? 'bg-[var(--color-accent-tint)]'
                        : 'hover:bg-[var(--color-surface-hover)]')
                    }
                  >
                    <span className="flex w-full items-center justify-between gap-2">
                      <span className="truncate text-[12px] font-semibold text-[var(--color-primary)]">
                        {skill.name}
                      </span>
                      <span className="shrink-0 font-mono text-[10px] text-[var(--color-tertiary)]">
                        v{skill.version}
                      </span>
                    </span>
                    <span className="truncate font-mono text-[10px] text-[var(--color-tertiary)]">{skill.slug}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="min-w-0 flex-1 overflow-y-auto p-4">
          {selected ? (
            <SkillDetail
              skill={selected}
              canAdmin={canAdmin}
              onEdit={() => setEditing(true)}
              onDelete={async () => {
                await deleteSkill(selected.id).unwrap()
                setSelectedID(null)
              }}
            />
          ) : (
            !isLoading && <EmptyState title={t['skills.selectHint']} />
          )}
        </div>
      </div>

      {creating ? (
        <SkillForm
          onClose={() => setCreating(false)}
          onSubmit={async (values) => {
            const created = await createSkill(values).unwrap()
            setSelectedID(created.id)
            setCreating(false)
          }}
        />
      ) : null}

      {editing && selected ? (
        <SkillForm
          skill={selected}
          onClose={() => setEditing(false)}
          onSubmit={async (values) => {
            await updateSkill({ id: selected.id, name: values.name, body_md: values.body_md }).unwrap()
            setEditing(false)
          }}
        />
      ) : null}
    </>
  )
}

/**
 * The detail pane: metadata, the "used by" list, and the rendered preview.
 *
 * `used_by` on the row is a COUNT; the named list is a separate query keyed by
 * id. They are not interchangeable — a skill with a stale count would still list
 * the right agents, and the count is what the row shows without asking.
 */
function SkillDetail({
  skill,
  canAdmin,
  onEdit,
  onDelete,
}: {
  skill: AgentSkill
  canAdmin: boolean
  onEdit: () => void
  onDelete: () => void | Promise<void>
}) {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const { data: agents } = useGetSkillAgentsQuery(skill.id)
  const [confirming, setConfirming] = useState(false)

  return (
    <div className="flex max-w-[760px] flex-col gap-4">
      <header className="flex flex-col gap-2 border-b border-[var(--color-border-subtle)] pb-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-[15px] font-semibold text-[var(--color-primary)]">{skill.name}</h2>
            <p className="mt-0.5 flex items-center gap-2 font-mono text-[11px] text-[var(--color-tertiary)]">
              <span>{skill.slug}</span>
              <span>v{skill.version}</span>
              {skill.is_system ? (
                <span
                  data-testid="skill-system-badge"
                  className="rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 text-[10px] uppercase tracking-wide"
                >
                  {t['skills.system']}
                </span>
              ) : null}
            </p>
          </div>
          {canAdmin ? (
            <div className="flex shrink-0 items-center gap-2">
              <Button size="sm" variant="secondary" onClick={onEdit} data-testid="skill-edit">
                <Pencil size={12} />
                {t['skills.edit']}
              </Button>
              {/* A system skill cannot be deleted (the API answers 409), so the
                  control is disabled and says why instead of failing on click. */}
              <Button
                size="sm"
                variant="secondary"
                disabled={skill.is_system}
                title={skill.is_system ? t['skills.systemNoDelete'] : undefined}
                onClick={() => setConfirming(true)}
                data-testid="skill-delete"
              >
                <Trash2 size={12} />
                {t['skills.delete']}
              </Button>
            </div>
          ) : null}
        </div>

        <dl className="flex flex-wrap gap-x-6 gap-y-1 text-[11px] text-[var(--color-secondary)]">
          <div className="flex gap-1.5">
            <dt className="text-[var(--color-tertiary)]">{t['skills.updated']}</dt>
            <dd>{formatDateTime(skill.updated_at, lang)}</dd>
          </div>
          <div className="flex gap-1.5">
            <dt className="text-[var(--color-tertiary)]">{t['skills.usedBy']}</dt>
            <dd data-testid="skill-used-by">{skill.used_by}</dd>
          </div>
        </dl>
      </header>

      {agents && agents.length > 0 ? (
        <section className="flex flex-col gap-1.5">
          <h3 className="font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
            {t['skills.usedByAgents']}
          </h3>
          <ul className="flex flex-wrap gap-1.5">
            {agents.map((agent) => (
              <li
                key={agent.id}
                className="rounded-[4px] border border-[var(--color-border-subtle)] px-1.5 py-0.5 text-[11px] text-[var(--color-secondary)]"
              >
                {agent.name}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="flex flex-col gap-1.5">
        <h3 className="font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
          {t['skills.preview']}
        </h3>
        {/* The one `dangerouslySetInnerHTML` in this screen, fed only by
            `renderMarkdown`, which escapes all input text before adding markup. */}
        <div
          data-testid="skill-preview"
          className="skill-markdown rounded-[8px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-3 text-[12px] leading-relaxed text-[var(--color-primary)]"
          dangerouslySetInnerHTML={{ __html: renderMarkdown(skill.body_md) }}
        />
      </section>

      {confirming ? (
        <Modal
          open
          title={t['skills.deleteTitle']}
          onClose={() => setConfirming(false)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setConfirming(false)}>
                {t['action.cancel']}
              </Button>
              <Button
                size="sm"
                data-testid="skill-delete-confirm"
                onClick={() => {
                  setConfirming(false)
                  void onDelete()
                }}
              >
                {t['skills.delete']}
              </Button>
            </>
          }
        >
          <p className="text-[12px] text-[var(--color-secondary)]">
            {t['skills.deleteBody'].replace('{name}', skill.name)}
          </p>
        </Modal>
      ) : null}
    </div>
  )
}

/**
 * The create/edit form. The same component for both, because the fields are the
 * same and the only difference is whether the slug is editable: on create it is
 * typed, on edit it is shown read-only with the reason (agents store the slug, so
 * changing it would detach them).
 */
function SkillForm({
  skill,
  onClose,
  onSubmit,
}: {
  skill?: AgentSkill
  onClose: () => void
  onSubmit: (values: { slug: string; name: string; body_md: string }) => void | Promise<void>
}) {
  const t = useT()
  const editing = Boolean(skill)
  const [slug, setSlug] = useState(skill?.slug ?? '')
  const [name, setName] = useState(skill?.name ?? '')
  const [body, setBody] = useState(skill?.body_md ?? '')
  const [preview, setPreview] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // Mirrors the DDL check and the server's `slugPattern`, so the operator is told
  // before the request instead of by a 400.
  const slugValid = editing || /^[a-z0-9_]{1,64}$/.test(slug)
  const canSave = name.trim().length > 0 && slugValid && body.length > 0 && !busy

  return (
    <Modal
      open
      title={editing ? t['skills.editTitle'] : t['skills.createTitle']}
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose}>
            {t['action.cancel']}
          </Button>
          <Button
            size="sm"
            disabled={!canSave}
            data-testid="skill-save"
            onClick={async () => {
              setBusy(true)
              setError('')
              try {
                await onSubmit({ slug, name: name.trim(), body_md: body })
              } catch (cause) {
                // The server's refusals are the ones the client cannot predict:
                // a slug already taken in this org, or a body that is not UTF-8.
                setError(cause instanceof Error ? cause.message : t['state.error'])
                setBusy(false)
              }
            }}
          >
            {t['action.save']}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <label className="flex flex-col gap-1">
          <span className="text-[11px] text-[var(--color-secondary)]">{t['skills.fieldSlug']}</span>
          <Input
            value={slug}
            onChange={(event) => setSlug(event.target.value)}
            disabled={editing}
            placeholder="code_review"
            data-testid="skill-slug"
            className="font-mono"
          />
          <span className="text-[10px] text-[var(--color-tertiary)]">
            {editing ? t['skills.slugLocked'] : t['skills.slugHint']}
          </span>
        </label>

        <label className="flex flex-col gap-1">
          <span className="text-[11px] text-[var(--color-secondary)]">{t['skills.fieldName']}</span>
          <Input value={name} onChange={(event) => setName(event.target.value)} data-testid="skill-name" />
        </label>

        <div className="flex flex-col gap-1">
          <div className="flex items-center justify-between">
            <span className="text-[11px] text-[var(--color-secondary)]">{t['skills.fieldBody']}</span>
            <button
              type="button"
              onClick={() => setPreview((on) => !on)}
              data-testid="skill-form-preview-toggle"
              className="flex items-center gap-1 text-[11px] text-[var(--color-accent)]"
            >
              {preview ? <FileCode2 size={11} /> : <Eye size={11} />}
              {preview ? t['skills.showSource'] : t['skills.showPreview']}
            </button>
          </div>
          {preview ? (
            <div
              data-testid="skill-form-preview"
              className="skill-markdown min-h-[180px] rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-2.5 text-[12px] leading-relaxed text-[var(--color-primary)]"
              dangerouslySetInnerHTML={{ __html: renderMarkdown(body) }}
            />
          ) : (
            <textarea
              value={body}
              onChange={(event) => setBody(event.target.value)}
              rows={10}
              data-testid="skill-body"
              placeholder={t['skills.bodyHint']}
              className="w-full rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5 font-mono text-[11px] text-[var(--color-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          )}
        </div>

        {error ? (
          <p role="alert" data-testid="skill-error" className="text-[11px] text-[var(--color-danger)]">
            {error}
          </p>
        ) : null}
      </div>
    </Modal>
  )
}
