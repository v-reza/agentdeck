import { useRef, useState } from 'react'
import { Plus } from 'lucide-react'
import { useCreateProjectMutation } from '@/store/api/boards'
import { useCanAct } from '@/hooks/use-orgs'
import { useActionForm } from '@/hooks/use-action-form'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'

/**
 * "New project" action on the project directory (US-AD08 AC4).
 *
 * The design (`11-project-list.html`) specifies the trigger exactly: a filled
 * accent button, `h-8`, radius 6, `text-[12px]`, a plus glyph before the label.
 * The `Button` primitive defaults to `variant="secondary"`, so the variant is
 * passed explicitly here — relying on a default is what shipped a white outline
 * button where the design has a teal one.
 *
 * The design has no dialog for this action, so the form's *home* is a UX
 * decision rather than a clone: an inline form beside the button pushed the
 * toolbar around, had no focus story, no Escape, and no room for validation. The
 * modal treatment comes from DESIGN.md's own `modal` token, so nothing new was
 * invented — only the placement was chosen.
 *
 * The slug is offered as a derived default but stays editable: the server treats
 * it as the project's stable identifier, so silently rewriting what the operator
 * typed would be worse than showing it. Derivation stops for good once the slug
 * is touched by hand.
 *
 * US-AD08 AC3 makes creating a project an owner/admin action, so the trigger is
 * hidden below `admin` rather than rendered and then rejected. `useCanAct` is
 * the same helper the members screen uses for its own admin-only controls; the
 * backend enforces the identical minimum, so hiding it is a UX decision and
 * never the security boundary.
 */
export function CreateProjectForm() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  // Once the operator edits the slug, the name stops driving it. Without this a
  // second keystroke in `name` would overwrite a deliberate slug.
  const slugEdited = useRef(false)
  const [createProject] = useCreateProjectMutation()
  const canCreate = useCanAct('admin')

  const [state, formAction, isPending] = useActionForm(
    createProject,
    (form) => ({
      name: String(form.get('name') ?? ''),
      slug: String(form.get('slug') ?? ''),
    }),
    () => {
      setOpen(false)
      setName('')
      setSlug('')
      slugEdited.current = false
    },
  )

  function close() {
    setOpen(false)
  }

  return (
    <>
      {/* Design: `h-8 px-3 rounded-[6px] text-[12px] gap-1.5 shadow-sm`. `md` is
          the h-8 size; its default px-3.5 is overridden to the design's px-3.
          AC3: absent below `admin`, so a member or viewer never sees a control
          that would answer 403. */}
      {canCreate ? (
        <Button variant="primary" size="md" className="gap-1.5 px-3 shadow-sm" onClick={() => setOpen(true)}>
          <Plus size={14} />
          New project
        </Button>
      ) : null}

      <Modal
        open={open}
        onClose={close}
        title="Project baru"
        description="Project dibuat di ruang kerja aktif — tidak perlu memilih organisasi."
      >
        <form action={formAction} id="create-project-form" className="flex flex-col gap-3.5">
          <Field label="Name">
            <Input
              name="name"
              required
              value={name}
              onChange={(event) => {
                const next = event.target.value
                setName(next)
                if (!slugEdited.current) setSlug(slugify(next))
              }}
              placeholder="Control plane"
              className="h-9 text-[13px]"
            />
          </Field>
          <Field label="Slug">
            <Input
              name="slug"
              required
              value={slug}
              onChange={(event) => {
                slugEdited.current = true
                setSlug(event.target.value)
              }}
              placeholder="control-plane"
              className="h-9 text-[13px]"
            />
          </Field>

          {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
        </form>

        <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
          <Button variant="ghost" size="sm" onClick={close}>
            Cancel
          </Button>
          <Button type="submit" form="create-project-form" variant="primary" size="sm" disabled={isPending}>
            {isPending ? 'Creating…' : 'Create'}
          </Button>
        </div>
      </Modal>
    </>
  )
}

/** Lowercase, hyphenated slug from a display name. */
export function slugify(value: string): string {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
}
