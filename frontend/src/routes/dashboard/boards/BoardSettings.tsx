import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useGetBoardQuery, useUpdateBoardMutation } from '@/store/api/boards'
import { useActionForm } from '@/hooks/use-action-form'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { useCanAct } from '@/hooks/use-orgs'
import { ColumnEditor } from '@/components/boards/ColumnEditor'
import { formatUSD } from '@/lib/formatters'

/**
 * Screen 23-board-settings — the board's daily cap (N16).
 *
 * The budget field is entered in whole dollars and converted to micro-USD on
 * submit, because DECISIONS 6 requires integer micro-USD on the wire and a float
 * dollar amount would introduce exactly the rounding the contract forbids.
 *
 * The column layout is NOT edited here: US-AD10 gives it its own panel
 * (design 22-column-editor, opened from the board topbar) because editing it is
 * an Admin action while this screen's fields are Member-level. Folding the layout
 * into this form would have put an Admin-only write behind a Member-gated route.
 */
export function BoardSettings() {
  const { boardID } = useParams<{ boardID: string }>()
  const { data: board } = useGetBoardQuery(boardID ?? '', { skip: !boardID })
  const [updateBoard] = useUpdateBoardMutation()
  const [editingColumns, setEditingColumns] = useState(false)
  // US-AD10 AC4: editing the layout is owner/admin. Hiding the control is UX,
  // not the boundary — the route is Admin-gated server-side and the e2e suite
  // asserts the 403 for a member, so a hidden button over a 201 is impossible.
  const canEditColumns = useCanAct('admin')

  const [state, formAction, isPending] = useActionForm(updateBoard, (form) => ({
    id: boardID ?? '',
    name: String(form.get('name') ?? ''),
    budgetDailyMicros: Math.round(Number(form.get('budget_dollars') ?? 0) * 1_000_000),
  }))

  return (
    <>
      <WorkspaceTopbar title="Board settings" subtitle={board?.slug} />
      <div className="flex min-h-0 flex-1">
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
          <form action={formAction} className="flex max-w-[420px] flex-col gap-3">
            <Field label="Nama Board">
              <Input name="name" defaultValue={board?.name ?? ''} required />
            </Field>
            <Field label="Daily budget cap (USD)">
              <Input
                name="budget_dollars"
                type="number"
                step="0.01"
                min="0"
                defaultValue={board ? formatUSD(board.budget_daily_micros) : '0.00'}
              />
            </Field>
            <p className="text-[11px] text-[var(--color-tertiary)]">
              N16 default is $20 per day per board. The value is stored as integer micro-USD.
            </p>
            {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
            <Button type="submit" size="sm" disabled={isPending} className="self-start">
              {isPending ? 'Saving…' : 'Save'}
            </Button>
          </form>

          <section>
            <div className="mb-1.5 flex items-center justify-between">
              <h3 className="text-[10px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
                Columns
              </h3>
              {canEditColumns ? (
                <button
                  type="button"
                  onClick={() => setEditingColumns(true)}
                  className="text-[11px] font-semibold text-[var(--color-accent)] hover:underline"
                >
                  Edit kolom
                </button>
              ) : null}
            </div>
            <ul className="flex flex-wrap gap-1.5">
              {(board?.columns ?? []).map((column) => (
                <li
                  key={column.key}
                  className="rounded-[6px] border border-[var(--color-border-subtle)] px-2 py-1 font-mono text-[11px] text-[var(--color-secondary)]"
                >
                  {column.name}
                </li>
              ))}
            </ul>
            <p className="mt-2 text-[11px] text-[var(--color-tertiary)]">
              A column is a view of status, never a status of its own (DECISIONS 3).
            </p>
          </section>
        </div>

        {editingColumns && boardID ? <ColumnEditor boardID={boardID} onClose={() => setEditingColumns(false)} /> : null}
      </div>
    </>
  )
}
