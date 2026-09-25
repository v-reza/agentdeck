import { useState } from 'react'
import { Archive, CornerDownRight, Shield, Trash2, X } from 'lucide-react'
import {
  useDeleteTaskMutation,
  useGetTaskQuery,
  useListTaskLinksQuery,
  useMoveTaskMutation,
  useUpdateTaskMutation,
} from '@/store/api/boards'
import { useBoardEventsQuery } from '@/store/api/stream'
import { useActionForm, describeError } from '@/hooks/use-action-form'
import { useCanAct } from '@/hooks/use-orgs'
import { StepTimeline } from '@/components/terminal/StepTimeline'
import { Button } from '@/components/ui/button'
import { Field, Input, Textarea } from '@/components/ui/input'
import { useAppDispatch } from '@/store/hooks'
import type { TaskStatus } from '@/lib/domain'
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
 *
 * The design's own icons live here: a close `X` in the header, `CornerDownRight`
 * marking each dependency edge, `Archive`, `Trash2` and `Shield` in the footer.
 * The dependency arrow matters more than it looks — an edge rendered as plain
 * text reads as a label, and the direction ("this task waits on X") is the whole
 * content of the row.
 *
 * Archiving is the one action here a member cannot take (US-AD59 AC4). That is
 * enforced on the server in `moveTask`, and both destructive controls are hidden
 * when the session's role cannot use them, so the drawer does not offer a button
 * that would 403. A refusal that still gets through — a 409 because the task is
 * not finished — is rendered in the server's own words.
 *
 * Not built, and named rather than faked: the design's Logs tab, Artifacts tab,
 * Approvals tab, assignee picker, and code-font markdown body all need endpoints
 * that do not exist (`runs`, `artifacts`, `approvals`). Rendering dead tabs would
 * be the same lie as a screen reading a column nobody writes.
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
        <button
          type="button"
          onClick={() => dispatch(closeTask())}
          title="Tutup"
          aria-label="Tutup"
          className="flex h-7 w-7 items-center justify-center rounded-[6px] text-[var(--color-secondary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]"
        >
          <X size={15} strokeWidth={2} aria-hidden="true" />
        </button>
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
              <Field label="Task Body">
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
                {isPending ? 'Menyimpan…' : 'Simpan Perubahan'}
              </Button>
            </form>

            <section className="mt-5">
              <SectionTitle>Dependencies</SectionTitle>
              {(links?.parents ?? []).length === 0 && (links?.children ?? []).length === 0 ? (
                <p className="text-[12px] text-[var(--color-tertiary)]">No dependency edges.</p>
              ) : (
                <ul className="flex flex-col gap-1 font-mono text-[11px] text-[var(--color-secondary)]">
                  {(links?.parents ?? []).map((link) => (
                    <DependencyRow key={`p-${link.ParentID}`} text={`menunggu ${shortID(link.ParentID)}`} />
                  ))}
                  {(links?.children ?? []).map((link) => (
                    <DependencyRow key={`c-${link.ChildID}`} text={`memblokir ${shortID(link.ChildID)}`} />
                  ))}
                </ul>
              )}
            </section>

            <section className="mt-5">
              <SectionTitle>Timeline</SectionTitle>
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

      {task ? <DrawerFooter taskID={taskID} status={task.status} /> : null}
    </aside>
  )
}

/**
 * The design's footer: archive on the left, delete on the right, the permission
 * note underneath.
 *
 * Two pieces of local state rather than one: the two writes fail for different
 * reasons (archive can be a 409 because the task is still moving; delete can be
 * a 409 because tasks depend on it), and a single shared error slot would report
 * one button's refusal under the other.
 */
export function DrawerFooter({ taskID, status }: { taskID: string; status: TaskStatus }) {
  const canAdminister = useCanAct('admin')
  const archived = status === 'archived'

  const [archiveMove] = useMoveTaskMutation()
  const [removeTask] = useDeleteTaskMutation()
  const [archivePending, setArchivePending] = useState(false)
  const [removePending, setRemovePending] = useState(false)
  const [archiveError, setArchiveError] = useState<string | null>(null)
  const [removeError, setRemoveError] = useState<string | null>(null)

  async function archive() {
    setArchivePending(true)
    setArchiveError(null)
    try {
      // The move carries `from` as the caller sees it. The server reads the
      // stored status itself for the archive guard, so a stale `from` here is
      // not what decides whether the transition is legal (US-AD59 AC2/AC3).
      await archiveMove({ id: taskID, from: status, to: 'archived' }).unwrap()
    } catch (rejection) {
      setArchiveError(describeError(rejection))
    } finally {
      setArchivePending(false)
    }
  }

  async function remove() {
    setRemovePending(true)
    setRemoveError(null)
    try {
      await removeTask(taskID).unwrap()
    } catch (rejection) {
      setRemoveError(describeError(rejection))
    } finally {
      setRemovePending(false)
    }
  }

  return (
    <div className="border-t border-[var(--color-border-subtle)] px-4 py-3">
      <div className="flex items-center justify-between gap-2">
        {canAdminister ? (
          <Button size="sm" variant="ghost" disabled={archivePending || archived} onClick={archive}>
            <Archive size={13} strokeWidth={2} aria-hidden="true" />
            {archivePending ? 'Mengarsipkan…' : 'Arsipkan Task'}
          </Button>
        ) : (
          <span className="text-[11px] text-[var(--color-tertiary)]">Arsip butuh owner atau admin.</span>
        )}

        {canAdminister ? (
          <Button size="sm" variant="danger" disabled={removePending} onClick={remove}>
            <Trash2 size={13} strokeWidth={2} aria-hidden="true" />
            {removePending ? 'Menghapus…' : 'Hapus'}
          </Button>
        ) : null}
      </div>

      {archiveError ? <p className="mt-2 text-[11px] text-[var(--color-danger)]">{archiveError}</p> : null}
      {removeError ? <p className="mt-2 text-[11px] text-[var(--color-danger)]">{removeError}</p> : null}

      <p className="mt-2 flex items-center gap-1.5 text-[11px] text-[var(--color-tertiary)]">
        <Shield size={12} strokeWidth={2} aria-hidden="true" />
        {canAdminister
          ? 'Arsip dan hapus tersedia untuk peran Anda.'
          : 'Hanya owner/admin yang bisa mengarsipkan atau menghapus task.'}
      </p>
    </div>
  )
}

/** One dependency edge, with the design's direction arrow. */
function DependencyRow({ text }: { text: string }) {
  return (
    <li className="flex items-center gap-1.5">
      <CornerDownRight size={12} strokeWidth={2} aria-hidden="true" className="shrink-0 text-[var(--color-tertiary)]" />
      <span>{text}</span>
    </li>
  )
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h3 className="mb-1.5 text-[10px] font-bold tracking-[0.06em] text-[var(--color-tertiary)] uppercase">
      {children}
    </h3>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-[10px] text-[var(--color-tertiary)] uppercase">{label}</dt>
      <dd className="mt-0.5 font-semibold text-[var(--color-primary)] tabular-nums">{value}</dd>
    </div>
  )
}
