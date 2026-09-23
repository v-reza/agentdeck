import { useGetTaskQuery, useListTaskLinksQuery, useUpdateTaskMutation } from '@/store/api/boards'
import { useBoardEventsQuery } from '@/store/api/stream'
import { useActionForm } from '@/hooks/use-action-form'
import { StepTimeline } from '@/components/terminal/StepTimeline'
import { Button } from '@/components/ui/button'
import { Field, Input, Textarea } from '@/components/ui/input'
import { useAppDispatch } from '@/store/hooks'
import { closeTask } from '@/store/slices/uiSlice'
import { formatEstimatedMicroUSD, formatTokens, shortID } from '@/lib/formatters'
import { SkeletonText } from '@/components/ui/skeleton'

/**
 * Screen 20-task-drawer — the task, its dependency edges, and its append-only
 * event timeline.
 *
 * The timeline is read from the same events table the API exposes and, while the
 * SSE stream is up, from the live tail the stream patches into cache. Edits go
 * through `useActionState`, so there is no local form state and no manual
 * refetch: the mutation invalidates the Task tag and the drawer re-renders from
 * cache.
 */
export function TaskDetailDrawer({ taskID }: { taskID: string }) {
  const dispatch = useAppDispatch()
  const { data: task, isError } = useGetTaskQuery(taskID)
  const { data: links } = useListTaskLinksQuery(taskID)
  const { data: events } = useBoardEventsQuery(task?.board_id ?? '', { skip: !task?.board_id })
  const [updateTask] = useUpdateTaskMutation()

  const [state, formAction, isPending] = useActionForm(updateTask, (form) => ({
    id: taskID,
    title: String(form.get('title') ?? ''),
    body: String(form.get('body') ?? ''),
    priority: Number(form.get('priority') ?? 0),
  }))

  return (
    <aside
      aria-label="Task Detail Drawer"
      className="z-[25] flex h-full w-[420px] min-w-[420px] shrink-0 flex-col border-l border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] shadow-[-4px_0_16px_rgba(0,0,0,0.05)]"
    >
      <header className="flex h-[var(--spacing-topbar)] min-h-[var(--spacing-topbar)] items-center justify-between border-b border-[var(--color-border-subtle)] px-4">
        <div className="min-w-0">
          <div className="truncate text-[13px] font-semibold text-[var(--color-primary)]">{task?.title ?? 'Task'}</div>
          <div className="font-mono text-[10px] text-[var(--color-tertiary)]">{shortID(taskID)}</div>
        </div>
        <Button size="sm" variant="ghost" onClick={() => dispatch(closeTask())}>
          Close
        </Button>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {task ? (
          <>
            <dl className="grid grid-cols-3 gap-2 font-mono text-[11px]">
              <Metric label="status" value={task.status} />
              <Metric label="cost" value={formatEstimatedMicroUSD(task.cost_micros)} />
              <Metric label="tokens" value={formatTokens(task.tokens_in + task.tokens_out)} />
            </dl>

            <form action={formAction} className="mt-4 flex flex-col gap-3">
              <Field label="Title">
                <Input name="title" defaultValue={task.title} required />
              </Field>
              <Field label="Description">
                <Textarea name="body" defaultValue={task.body} rows={4} />
              </Field>
              <Field label="Priority">
                <Input name="priority" type="number" defaultValue={task.priority} className="w-[80px]" />
              </Field>
              {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
              <Button type="submit" size="sm" disabled={isPending} className="self-start">
                {isPending ? 'Saving…' : 'Save'}
              </Button>
            </form>

            <section className="mt-5">
              <h3 className="mb-1.5 text-[10px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
                Dependencies
              </h3>
              {(links?.parents ?? []).length === 0 && (links?.children ?? []).length === 0 ? (
                <p className="text-[12px] text-[var(--color-tertiary)]">No dependency edges.</p>
              ) : (
                <ul className="font-mono text-[11px] text-[var(--color-secondary)]">
                  {(links?.parents ?? []).map((link) => (
                    <li key={`p-${link.ParentID}`}>waits on {shortID(link.ParentID)}</li>
                  ))}
                  {(links?.children ?? []).map((link) => (
                    <li key={`c-${link.ChildID}`}>blocks {shortID(link.ChildID)}</li>
                  ))}
                </ul>
              )}
            </section>

            <section className="mt-5">
              <h3 className="mb-1.5 text-[10px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
                Timeline
              </h3>
              <StepTimeline events={(events ?? []).filter((event) => event.task_id === taskID)} />
            </section>
          </>
        ) : isError ? (
          // A failed fetch must not masquerade as a slow one: the previous
          // fallback showed "Loading…" for an errored task, so a broken request
          // looked like a slow network forever.
          <p className="text-[12px] text-[var(--color-danger)]">
            This task could not be loaded. Close the drawer and try again.
          </p>
        ) : (
          <SkeletonText lines={4} />
        )}
      </div>
    </aside>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-[10px] uppercase text-[var(--color-tertiary)]">{label}</dt>
      <dd className="mt-0.5 font-semibold text-[var(--color-primary)] tabular-nums">{value}</dd>
    </div>
  )
}
