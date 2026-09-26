import { useState } from 'react'
import { Check, Plus, UserPlus } from 'lucide-react'
import { useCreateTaskMutation, useGetBoardQuery, useListAssignableAgentsQuery } from '@/store/api/boards'
import { useActionForm } from '@/hooks/use-action-form'
import { Modal } from '@/components/ui/modal'
import { Button } from '@/components/ui/button'
import { Field, Input, Textarea } from '@/components/ui/input'
import { Combobox } from '@/components/ui/combobox'
import { COLUMN_LABELS } from '@/lib/domain'
import { useT } from '@/hooks/use-t'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { closeCreateTask } from '@/store/slices/uiSlice'

/** Lets the modal's footer submit the form its children render. */
const FORM_ID = 'task-create-form'

/**
 * Screen 21-task-create — "Buat Task Baru", a modal on the board.
 *
 * The design draws a full modal (title, description, priority, assignee) and the
 * implementation had a three-field inline form with no description field at all,
 * so `body` — which the API has always accepted and US-AD11 AC1 names — could
 * not be entered from anywhere. The inline form was also unreachable: nothing
 * imported it, which is why the board had no way to create a task.
 *
 * Priority is the design's four labelled levels (P0 Blocker … P3 Low), not a
 * free number. The API takes an integer in −10..10, but the screen the operator
 * actually uses shows four choices: a spinner over that range offers values that
 * mean nothing to anyone reading the board, and the row renders the raw number.
 * The four levels map to 0..3, the same values `P0`…`P3` already carry.
 *
 * `assignee_agent_id` is optional on purpose (AC5): a task with no agent is
 * valid and lands in `backlog`. The picker is the board's own — the same query
 * the agent detail screen reads — so an archived agent cannot be picked here.
 *
 * The design's "Status Awal" list is not rendered, and the field states the
 * fixed value instead: `ready` is what the dispatcher claims (DECISIONS 3), and
 * US-AD11 AC1/AC5 both say a created task starts in `backlog`. Offering `ready`
 * here would hand work to a runner that never saw the task; the move to `ready`
 * is a board move, which records the transition.
 *
 * The design's "AC5 …/AC1 …" helper card is not reproduced: it is spec jargon
 * (`US-AD`, `AC`) in rendered UI, which `.hermes.md` forbids. Its two facts — no
 * assignee is fine, and a chosen agent receives the task — are on screen in the
 * operator's own words instead.
 */
export function TaskCreateForm({ boardID, open, onClose }: { boardID: string; open: boolean; onClose: () => void }) {
  const [createTask] = useCreateTaskMutation()
  const { data: board } = useGetBoardQuery(boardID, { skip: !boardID })
  const t = useT()

  // Held in state rather than read by `build`: the design's levels are not the
  // integers the API stores, so the mapping has to happen where the choice is.
  const [priority, setPriority] = useState('2')

  const [state, formAction, isPending] = useActionForm(
    createTask,
    (form) => {
      // The helper text is stated on screen, so it is sent rather than dropped:
      // a modal that describes what the agent should do and then discards the
      // text would leave the agent reading an empty instruction.
      return {
        boardID,
        title: String(form.get('title') ?? ''),
        body: String(form.get('instruction') ?? ''),
        priority: Number(priority),
        // US-AD14 AC1: a chosen agent must be sent, an empty choice means
        // unassigned (AC5).
        assignee_agent_id: String(form.get('assignee') ?? ''),
        status: 'backlog' as const,
      }
    },
    onClose,
  )

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="lg"
      title={
        <span className="flex items-center gap-2.5">
          <span className="flex h-6 w-6 items-center justify-center rounded-[6px] bg-[var(--color-accent-tint)] text-[var(--color-accent)]">
            <Plus size={14} strokeWidth={2.2} aria-hidden="true" />
          </span>
          {t['boards.newTask']}
        </span>
      }
      description={t['task.create.subtitle']}
      footer={
        <div className="flex w-full items-center justify-between gap-2">
          <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
            {t['task.create.target']}{' '}
            <span className="font-semibold text-[var(--color-primary)]">{board?.name ?? '—'}</span>
          </span>
          <div className="flex items-center gap-2">
            <Button type="button" variant="secondary" size="md" onClick={onClose}>
              {t['action.cancel']}
            </Button>
            <Button type="submit" form={FORM_ID} variant="primary" size="md" disabled={isPending}>
              <Check size={13} strokeWidth={2} aria-hidden="true" />
              {isPending ? t['task.create.pending'] : t['task.create.submit']}
            </Button>
          </div>
        </div>
      }
    >
      <form id={FORM_ID} action={formAction} className="flex flex-col gap-4">
        <Field label={t['task.create.fieldTitle']} required hint={t['task.create.fieldTitleHint']}>
          <Input name="title" required autoFocus placeholder={t['task.create.titlePlaceholder']} />
        </Field>

        <Field label={t['task.create.description']} hint={t['task.create.markdownHint']}>
          <Textarea name="instruction" rows={3} placeholder={t['task.create.bodyPlaceholder']} />
        </Field>

        <div className="flex flex-col gap-3 rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-3">
          <AssigneePicker boardID={boardID} />

          <div className="flex items-start gap-2 border-t border-[var(--color-border-subtle)] pt-3">
            <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-accent)]" />
            <p className="text-[11px] leading-relaxed text-[var(--color-secondary)]">{t['task.create.assigneeHint']}</p>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <Field label={t['field.priority']}>
            <Combobox
              name="priority"
              label={t['field.priority']}
              value={priority}
              onChange={setPriority}
              options={[
                { value: '0', label: t['task.create.priorityP0'] },
                { value: '1', label: t['task.create.priorityP1'] },
                { value: '2', label: t['task.create.priorityP2'] },
                { value: '3', label: t['task.create.priorityP3'] },
              ]}
            />
          </Field>

          <Field label={t['task.create.initialStatus']} hint={t['task.create.initialStatusHint']}>
            <span className="flex h-8 items-center gap-2 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-hover)] px-2.5 text-[12px] text-[var(--color-secondary)]">
              <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-status-backlog)]" />
              {COLUMN_LABELS.backlog}
            </span>
          </Field>
        </div>

        {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
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
    <Combobox
      name="assignee"
      label={t['task.create.assignee']}
      icon={<UserPlus size={13} strokeWidth={2} aria-hidden="true" />}
      options={options}
      value={value}
      onChange={setValue}
    />
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
