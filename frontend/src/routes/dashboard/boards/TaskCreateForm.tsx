import { useState } from 'react'
import { useCreateTaskMutation, useListAssignableAgentsQuery } from '@/store/api/boards'
import { useActionForm } from '@/hooks/use-action-form'
import { Modal } from '@/components/ui/modal'
import { Button } from '@/components/ui/button'
import { Field, Input, Textarea } from '@/components/ui/input'
import { Combobox } from '@/components/ui/combobox'
import { useT } from '@/hooks/use-t'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { closeCreateTask } from '@/store/slices/uiSlice'

/**
 * Screen 21-task-create — "Buat Task Baru", a modal on the board.
 *
 * The design draws a full modal (title, description, priority, assignee) and the
 * implementation had a three-field inline form with no description field at all,
 * so `body` — which the API has always accepted and US-AD11 AC1 names — could
 * not be entered from anywhere. The inline form was also unreachable: nothing
 * imported it, which is why the board had no way to create a task.
 *
 * `assignee_agent_id` is optional on purpose (AC5): a task with no agent is
 * valid and lands in `backlog`. The picker is the board's own — the same query
 * the agent detail screen reads — so an archived agent cannot be picked here.
 *
 * The status is fixed to `backlog` rather than exposed: `ready` is what the
 * dispatcher claims, and a form that could create a task already `ready` would
 * be a way to hand work to a runner that never saw it (DECISIONS 3).
 */
export function TaskCreateForm({ boardID, open, onClose }: { boardID: string; open: boolean; onClose: () => void }) {
  const [createTask] = useCreateTaskMutation()
  const t = useT()

  const [state, formAction, isPending] = useActionForm(
    createTask,
    (form) => ({
      boardID,
      title: String(form.get('title') ?? ''),
      body: String(form.get('body') ?? ''),
      priority: Number(form.get('priority') ?? 0),
      assignee_agent_id: String(form.get('assignee') ?? ''),
      status: 'backlog' as const,
    }),
    onClose,
  )

  return (
    <Modal open={open} onClose={onClose} title={t['boards.newTask']} description={t['task.create.subtitle']}>
      <form action={formAction} className="flex flex-col gap-3">
        <Field label={t['task.create.fieldTitle']}>
          <Input name="title" required autoFocus placeholder={t['task.create.titlePlaceholder']} />
        </Field>

        <Field label={t['task.create.description']} hint={t['task.create.markdownHint']}>
          <Textarea name="body" rows={5} placeholder={t['task.create.bodyPlaceholder']} />
        </Field>

        <div className="flex gap-3">
          <div className="w-[110px]">
            <Field label={t['field.priority']}>
              <Input name="priority" type="number" min={-10} max={10} defaultValue={0} />
            </Field>
          </div>
          <div className="flex-1">
            <AssigneePicker boardID={boardID} />
          </div>
        </div>

        {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}

        <div className="flex justify-end gap-2 pt-1">
          <Button type="button" variant="ghost" size="sm" onClick={onClose}>
            {t['action.cancel']}
          </Button>
          <Button type="submit" size="sm" disabled={isPending}>
            {isPending ? t['task.create.pending'] : t['task.create.submit']}
          </Button>
        </div>
      </form>
    </Modal>
  )
}

/**
 * The board's assignable agents, wrapped in the shared `Combobox` (the only
 * select in the app — a native `<select>` was removed from every screen).
 *
 * The first option is the empty one: the design's "— Tanpa agent —" row. Without
 * it there would be no way back to an unassigned task once an agent was chosen.
 */
function AssigneePicker({ boardID }: { boardID: string }) {
  const { data: picker } = useListAssignableAgentsQuery(boardID, { skip: !boardID })
  const [value, setValue] = useState('')
  const t = useT()

  const options = [
    { value: '', label: t['task.create.assigneeNone'] },
    ...(picker ?? []).map((agent) => ({ value: agent.id, label: agent.name })),
  ]

  // `name="assignee"` is what puts the value into FormData: the form is submitted
  // by the platform (`action=`), which reads the form element rather than React
  // state.
  return (
    <Combobox name="assignee" label={t['task.create.assignee']} options={options} value={value} onChange={setValue} />
  )
}

/**
 * Mounted once for the whole shell (like `TaskDrawerHost`), so the modal is not
 * remounted by a board view switch and the focus trap keeps its target.
 *
 * The board comes from the slice rather than the route: the toolbar that opens
 * this lives on two routes (`/boards/:boardID` and `.../table`), and reading
 * `useParams` here would break the moment those two diverge.
 */
export function TaskCreateHost() {
  const boardID = useAppSelector((state) => state.ui.createTaskBoardID)
  const dispatch = useAppDispatch()
  if (!boardID) return null
  return <TaskCreateForm boardID={boardID} open onClose={() => dispatch(closeCreateTask())} />
}
