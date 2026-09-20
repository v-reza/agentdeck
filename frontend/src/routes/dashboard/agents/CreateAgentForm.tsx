import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useCreateAgentMutation } from '@/store/api/agents'
import { useCanAct } from '@/hooks/use-orgs'
import { useActionForm } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'

/**
 * "Daftarkan Agent Baru" on the agent registry (US-AD20 AC1), cloned from the
 * design's own toolbar CTA in `25-agent-registry.html` and its form panel in
 * `26-agent-form.html`: filled accent, 30px tall, radius 6, `text-[12px]`,
 * plus glyph before the label.
 *
 * The design's form also asks for an API key. That field is deliberately absent:
 * `PUT /agents/{id}/provider-key` is US-AD86 (M2) and has no route yet, and
 * US-AD20 AC5 says an agent registered without a credential is valid — it is
 * simply not ready to run. Rendering the field would promise a write that can
 * only answer 404.
 *
 * AC1 makes registering a Member action, so the trigger hides below `member`.
 * The server enforces the same minimum through `requireRole`, so hiding it is a
 * UX decision and never the security boundary.
 */
export function CreateAgentForm({
  projects,
  activeProjectID,
}: {
  projects: { id: string; name: string }[]
  /** The project the registry is showing; preselected in the form. */
  activeProjectID: string
}) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [createAgent] = useCreateAgentMutation()
  const canCreate = useCanAct('member')

  const [state, formAction, isPending] = useActionForm(
    createAgent,
    (form) => ({
      projectID: String(form.get('projectID') ?? ''),
      name: String(form.get('name') ?? ''),
      provider: String(form.get('provider') ?? ''),
      model: String(form.get('model') ?? ''),
      reasoningEffort: String(form.get('reasoningEffort') ?? ''),
      maxRuntimeSeconds: Number(form.get('maxRuntimeSeconds') ?? 0),
      retryPolicy: String(form.get('retryPolicy') ?? ''),
      maxAttempts: Number(form.get('maxAttempts') ?? 0),
      // Free-text lists: the contract stores jsonb arrays, and the design's
      // tools field is a comma-separated input rather than a multi-select.
      tools: splitList(form.get('tools')),
      skills: splitList(form.get('skills')),
    }),
    () => setOpen(false),
  )

  if (!canCreate || projects.length === 0) return null

  return (
    <>
      <Button variant="primary" size="md" className="gap-1.5 px-3" onClick={() => setOpen(true)}>
        <Plus size={14} />
        {t['agents.new']}
      </Button>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={t['agents.create.title']}
        description={t['agents.create.description']}
      >
        <form action={formAction} id="create-agent-form" className="flex flex-col gap-3.5">
          <Field label={t['boards.create.project']}>
            <select
              name="projectID"
              required
              defaultValue={activeProjectID}
              className="h-8 w-full rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-2.5 text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
            >
              {projects.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
          </Field>

          <Field label={t['field.name']}>
            <Input name="name" required placeholder="agent-backend" className="h-9 text-[13px]" />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field label={t['agents.field.provider']}>
              <Input name="provider" required placeholder="openai" className="h-9 text-[13px]" />
            </Field>
            <Field label={t['agents.field.model']}>
              <Input name="model" required placeholder="gpt-4o" className="h-9 text-[13px]" />
            </Field>
          </div>
          <p className="text-[11px] text-[var(--color-tertiary)]">{t['agents.field.modelHint']}</p>

          <div className="grid grid-cols-3 gap-3">
            <Field label={t['agents.field.reasoning']}>
              <select
                name="reasoningEffort"
                defaultValue="medium"
                className="h-9 w-full rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-2.5 text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
              >
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </Field>
            <Field label={t['agents.field.runtime']}>
              <Input
                name="maxRuntimeSeconds"
                type="number"
                min={1}
                max={86400}
                defaultValue={14400}
                className="h-9 text-[13px]"
              />
            </Field>
            <Field label={t['agents.field.attempts']}>
              <Input name="maxAttempts" type="number" min={1} max={10} defaultValue={3} className="h-9 text-[13px]" />
            </Field>
          </div>

          <Field label={t['agents.field.retry']}>
            <select
              name="retryPolicy"
              defaultValue="transient_only"
              className="h-8 w-full rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-2.5 text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
            >
              <option value="never">never</option>
              <option value="transient_only">transient_only</option>
              <option value="always">always</option>
            </select>
          </Field>

          <Field label={t['agents.field.tools']}>
            <Input name="tools" placeholder="git, bash" className="h-9 text-[13px]" />
          </Field>
          <p className="text-[11px] text-[var(--color-tertiary)]">{t['agents.field.toolsHint']}</p>

          <Field label={t['agents.field.skills']}>
            <Input name="skills" placeholder="code-review" className="h-9 text-[13px]" />
          </Field>

          {state.error ? (
            <p className="text-[12px] text-[var(--color-danger)]">{`${t['agents.create.failed']}: ${state.error}`}</p>
          ) : null}
        </form>

        <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
          <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
            {t['action.cancel']}
          </Button>
          <Button type="submit" form="create-agent-form" variant="primary" size="sm" disabled={isPending}>
            {isPending ? t['agents.create.pending'] : t['agents.create.submit']}
          </Button>
        </div>
      </Modal>
    </>
  )
}

/** Splits a comma-separated input into trimmed, non-empty entries. */
function splitList(raw: FormDataEntryValue | null): string[] {
  return String(raw ?? '')
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part.length > 0)
}
