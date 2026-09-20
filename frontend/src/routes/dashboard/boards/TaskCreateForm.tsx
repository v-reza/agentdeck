import { useState } from 'react'
import { useCreateTaskMutation } from '@/store/api/boards'
import { useActionForm } from '@/hooks/use-action-form'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'

/**
 * Screen 21-task-create — "new task" on a board.
 *
 * The task is created in `backlog` unless the operator names another status: a
 * task must not start `ready` by accident, because `ready` is what the
 * dispatcher claims (DECISIONS 3).
 */
export function CreateTaskForm({ boardID }: { boardID: string }) {
  const [open, setOpen] = useState(false)
  const [createTask] = useCreateTaskMutation()

  const [state, formAction, isPending] = useActionForm(
    createTask,
    (form) => ({
      boardID,
      title: String(form.get('title') ?? ''),
      body: String(form.get('body') ?? ''),
      priority: Number(form.get('priority') ?? 0),
      status: 'backlog' as const,
    }),
    () => setOpen(false),
  )

  if (!open) {
    return (
      <Button size="sm" onClick={() => setOpen(true)}>
        New task
      </Button>
    )
  }

  return (
    <form action={formAction} className="flex items-end gap-2">
      <Field label="Title">
        <Input name="title" required autoFocus placeholder="Fix flaky reclaim" className="w-[240px]" />
      </Field>
      <Field label="Priority">
        <Input name="priority" type="number" defaultValue={0} className="w-[70px]" />
      </Field>
      <Button type="submit" size="sm" disabled={isPending}>
        {isPending ? 'Adding…' : 'Add'}
      </Button>
      <Button type="button" size="sm" variant="ghost" onClick={() => setOpen(false)}>
        Cancel
      </Button>
      {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
    </form>
  )
}
