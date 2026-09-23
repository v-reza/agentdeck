import { useRef, useState } from 'react'
import { Plus } from 'lucide-react'
import { useCreateBoardMutation } from '@/store/api/boards'
import { useCanAct } from '@/hooks/use-orgs'
import { useActionForm } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'
import { Combobox } from '@/components/ui/combobox'
import { slugify } from '@/routes/dashboard/projects/CreateProjectForm'

/**
 * "New board" action on the board list (US-AD09), cloned from the design's own
 * toolbar CTA in `10-board-list.html`: filled accent, `h-8`, radius 6,
 * `text-[12px]`, plus glyph before the label. `Button` defaults to
 * `variant="secondary"`, so the variant is passed explicitly — that default is
 * what once shipped a white outline button where the design has a teal one.
 *
 * The design renders a second control next to it, "From template", which is
 * US-AD72 and belongs to M6. It is deliberately absent rather than stubbed: a
 * button that opens nothing is worse than no button.
 *
 * US-AD09 AC4 ("tidak memerlukan pemilihan organisasi maupun project baru bila
 * pengguna belum punya project") is satisfied by the server, not by this form:
 * registration seeds a starter project, so the project list is never empty for
 * a workspace that has just been created. This form therefore only ever *picks*
 * an existing project and never offers to create one — the org picker and the
 * project-creation step AC4 forbids simply do not exist here.
 *
 * US-AD09 AC3 makes board creation an owner/admin action, so the trigger is
 * hidden below `admin` rather than rendered and then rejected with a 403. The
 * backend enforces the identical minimum through `requireRole`, so hiding it is
 * a UX decision and never the security boundary.
 */
export function CreateBoardForm({
  projects,
  label,
}: {
  projects: { id: string; name: string }[]
  /** Overrides the trigger copy. The empty state asks for the first board. */
  label?: string
}) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  // Controlled picker: `Combobox` holds its value in state, so the project
  // default that used to be a native `defaultValue` lives here instead. The
  // reset below keeps it pointing at the first project after a successful create.
  const [projectID, setProjectID] = useState(projects[0]?.id ?? '')
  // Once the operator edits the slug, the name stops driving it. Without this a
  // second keystroke in `name` would overwrite a deliberate slug.
  const slugEdited = useRef(false)
  const [createBoard] = useCreateBoardMutation()
  const canCreate = useCanAct('admin')

  const [state, formAction, isPending] = useActionForm(
    createBoard,
    (form) => ({
      projectID: String(form.get('projectID') ?? ''),
      name: String(form.get('name') ?? ''),
      slug: String(form.get('slug') ?? ''),
    }),
    () => {
      setOpen(false)
      setName('')
      setSlug('')
      setProjectID(projects[0]?.id ?? '')
      slugEdited.current = false
    },
  )

  function close() {
    setOpen(false)
  }

  // No project means no board can exist: `boards_project_fk` keys on
  // project_id. Rendering the CTA anyway would open a form whose submit is
  // guaranteed to fail, so the trigger stays hidden and the caller's empty
  // state explains the project step instead.
  if (!canCreate || projects.length === 0) return null

  return (
    <>
      <Button variant="primary" size="md" className="gap-1.5 px-3" onClick={() => setOpen(true)}>
        <Plus size={14} />
        {label ?? t['boards.new']}
      </Button>

      <Modal open={open} onClose={close} title={t['boards.create.title']} description={t['boards.create.description']}>
        <form action={formAction} id="create-board-form" className="flex flex-col gap-3.5">
          <Field label={t['boards.create.project']}>
            <Combobox
              name="projectID"
              label={t['boards.create.project']}
              value={projectID}
              onChange={setProjectID}
              options={projects.map((project) => ({ value: project.id, label: project.name }))}
            />
          </Field>

          <Field label={t['field.name']}>
            <Input
              name="name"
              required
              value={name}
              onChange={(event) => {
                const next = event.target.value
                setName(next)
                if (!slugEdited.current) setSlug(slugify(next))
              }}
              placeholder="Sprint 24"
              className="h-9 text-[13px]"
            />
          </Field>

          <Field label={t['field.slug']}>
            <Input
              name="slug"
              required
              value={slug}
              onChange={(event) => {
                slugEdited.current = true
                setSlug(event.target.value)
              }}
              placeholder="sprint-24"
              className="h-9 text-[13px]"
            />
          </Field>

          {/* The server assigns the default columns when `columns` is absent
              (board.Service.CreateBoard → DefaultColumns). Stating them here is
              a description of what will exist, not a claim about what the form
              sends. */}
          <p className="text-[11px] text-[var(--color-tertiary)]">{t['boards.create.columnsHint']}</p>

          {state.error ? (
            <p className="text-[12px] text-[var(--color-danger)]">{`${t['boards.create.failed']}: ${state.error}`}</p>
          ) : null}
        </form>

        <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
          <Button variant="ghost" size="sm" onClick={close}>
            {t['action.cancel']}
          </Button>
          <Button type="submit" form="create-board-form" variant="primary" size="sm" disabled={isPending}>
            {isPending ? t['boards.create.pending'] : t['boards.create.submit']}
          </Button>
        </div>
      </Modal>
    </>
  )
}
