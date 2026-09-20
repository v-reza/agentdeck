import { useEffect, useMemo, useRef, useState } from 'react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical, Plus, Trash2, X } from 'lucide-react'
import { useGetBoardQuery, useListTasksQuery, useUpdateBoardColumnsMutation } from '@/store/api/boards'
import { useActionForm } from '@/hooks/use-action-form'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/cn'
import { columnForStatus } from '@/lib/domain'
import type { Column, Task } from '@/lib/domain'

/**
 * US-AD10 — the board column editor, cloned from `design/stitch-output/v2/22-column-editor.html`.
 *
 * What the design fixes and this follows: the 420px panel with its header, the
 * 11px mono uppercase section labels, a row per column at `p-2.5 rounded-[6px]`
 * with a `cursor-grab` handle on the left, the name as a borderless inline input,
 * a `N task` badge, and a trailing delete button that is dimmed and
 * `cursor-not-allowed` while the column still holds tasks. The add-column form is
 * the accent-tinted block at the bottom.
 *
 * What it does NOT follow: the mockup carries a banner that narrates the story's
 * own acceptance criteria ("US-AD10 AC1 - AC5 SPEC", "v2.4 Core"). That is spec
 * annotation burned into a static mock, not product chrome — shipping it would
 * put the ticket text in front of the operator. The behaviours it describes are
 * implemented here instead, and asserted in e2e/column-editor.spec.ts.
 *
 * The dots are `--color-status-*` rather than the mock's literal hexes: DESIGN.md
 * is the locked token source, and the kanban lane (TaskCard) already colours a
 * status that way, so a column dot and the task cards inside it now agree.
 */
export function ColumnEditor({ boardID, onClose }: { boardID: string; onClose: () => void }) {
  const { data: board } = useGetBoardQuery(boardID, { skip: !boardID })
  const { data: tasks } = useListTasksQuery(boardID, { skip: !boardID })
  const [updateColumns] = useUpdateBoardColumnsMutation()

  // The working copy. The layout is edited locally (rename, reorder, remove) and
  // committed as a whole, so a rejected write never leaves a half-applied board.
  const [draft, setDraft] = useState<Column[]>([])
  const [seeded, setSeeded] = useState(false)

  // Seed once from the server. Re-seeding on every refetch would discard an
  // in-flight edit the moment a background invalidation landed.
  useEffect(() => {
    if (!board || seeded) return
    setDraft(board.columns)
    setSeeded(true)
  }, [board, seeded])

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  const counts = useMemo(() => countTasksByColumn(tasks ?? []), [tasks])

  const dirty = useMemo(() => JSON.stringify(draft) !== JSON.stringify(board?.columns ?? []), [draft, board])

  const [state, formAction, isPending] = useActionForm(updateColumns, () => ({
    boardID,
    columns: draft,
  }))

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    setDraft((cols) => {
      const from = cols.findIndex((c) => c.key === active.id)
      const to = cols.findIndex((c) => c.key === over.id)
      if (from === -1 || to === -1) return cols
      return arrayMove(cols, from, to)
    })
  }

  function removeColumn(key: string) {
    setDraft((cols) => cols.filter((c) => c.key !== key))
  }

  function renameColumn(key: string, name: string) {
    setDraft((cols) => cols.map((c) => (c.key === key ? { ...c, name } : c)))
  }

  // AC1: a new column is appended, never inserted, so it lands in the last
  // position. The key is derived from the name and de-duplicated, because the
  // key is the identity the server stores and two columns cannot share one.
  function addColumn(name: string) {
    const trimmed = name.trim()
    if (!trimmed) return
    setDraft((cols) => [...cols, { key: uniqueKey(trimmed, cols), name: trimmed }])
  }

  return (
    <aside className="flex w-[420px] min-w-[420px] flex-col border-l border-[var(--color-border-standard)] bg-[var(--color-surface-panel)]">
      <header className="flex h-[46px] shrink-0 items-center justify-between border-b border-[var(--color-border-standard)] px-4">
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded-[6px] bg-[var(--color-accent-tint)] text-[var(--color-accent)]">
            <ColumnsGlyph />
          </span>
          <div>
            <h2 className="text-[13px] font-bold text-[var(--color-primary)]">Editor Kolom Board</h2>
            <div className="font-mono text-[10px] text-[var(--color-tertiary)]">{board?.slug ?? boardID}</div>
          </div>
        </div>
        <button
          type="button"
          onClick={onClose}
          title="Tutup panel"
          aria-label="Tutup panel"
          className="flex h-7 w-7 items-center justify-center rounded-[4px] text-[var(--color-tertiary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]"
        >
          <X size={18} />
        </button>
      </header>

      <div className="flex-1 space-y-4 overflow-y-auto p-4 text-[12px]">
        <section>
          <div className="mb-2 flex items-center justify-between">
            <span className="font-mono text-[11px] font-bold uppercase text-[var(--color-secondary)]">
              Struktur kolom aktif ({draft.length})
            </span>
            <span className="text-[10px] text-[var(--color-tertiary)]">Drag handle untuk urutan</span>
          </div>

          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={draft.map((c) => c.key)} strategy={verticalListSortingStrategy}>
              <div className="space-y-2">
                {draft.map((column) => (
                  <ColumnRow
                    key={column.key}
                    column={column}
                    taskCount={counts[column.key] ?? 0}
                    onRename={renameColumn}
                    onRemove={removeColumn}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        </section>

        <AddColumnForm onAdd={addColumn} />

        <form action={formAction} className="space-y-2">
          {/* AC2/AC3 arrive as a 409 and are surfaced here rather than as a toast:
              the failure belongs to the layout the operator is editing, and a
              toast would duplicate an inline surface (see store/listeners/toast). */}
          {state.error ? (
            <p role="alert" className="text-[11px] text-[var(--color-danger)]">
              {state.error}
            </p>
          ) : null}
          {state.done && !dirty ? <p className="text-[11px] text-[var(--color-success)]">Kolom tersimpan.</p> : null}
          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={isPending || !dirty}>
              {isPending ? 'Menyimpan…' : 'Simpan kolom'}
            </Button>
            <button
              type="button"
              onClick={() => setDraft(board?.columns ?? [])}
              disabled={!dirty}
              className={cn(
                'h-8 rounded-[6px] px-3 text-[11px] font-semibold transition-colors',
                dirty
                  ? 'text-[var(--color-secondary)] hover:bg-[var(--color-surface-hover)]'
                  : 'cursor-not-allowed text-[var(--color-quaternary)]',
              )}
            >
              Batalkan perubahan
            </button>
          </div>
        </form>
      </div>
    </aside>
  )
}

/** One sortable column row — the mock's `p-2.5 rounded-[6px]` card. */
function ColumnRow({
  column,
  taskCount,
  onRename,
  onRemove,
}: {
  column: Column
  taskCount: number
  onRename: (key: string, name: string) => void
  onRemove: (key: string) => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: column.key })
  // AC2: a column holding tasks cannot be removed. The server enforces this with
  // a 409 and this mirrors it — the button is disabled rather than hidden, so the
  // rule is visible instead of the control silently vanishing.
  const blocked = taskCount > 0

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(
        'flex items-center justify-between gap-2 rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] p-2.5',
        isDragging && 'z-10 opacity-80 shadow-md',
      )}
    >
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <button
          type="button"
          {...attributes}
          {...listeners}
          aria-label={`Ubah urutan kolom ${column.name}`}
          className="cursor-grab touch-none text-[var(--color-tertiary)] active:cursor-grabbing"
        >
          <GripVertical size={16} />
        </button>
        <span
          aria-hidden
          className="h-2.5 w-2.5 shrink-0 rounded-full"
          style={{ background: `var(--color-status-${statusTokenFor(column.key)})` }}
        />
        <input
          type="text"
          value={column.name}
          onChange={(event) => onRename(column.key, event.target.value)}
          aria-label={`Nama kolom ${column.key}`}
          className="w-full min-w-0 border-0 bg-transparent p-0 text-[12px] font-medium text-[var(--color-primary)] focus:border-b focus:border-[var(--color-accent)] focus:outline-none"
        />
      </div>

      <div className="flex shrink-0 items-center gap-2">
        <span
          data-testid={`task-count-${column.key}`}
          title={`${taskCount} task di dalam kolom`}
          className="rounded border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-secondary)]"
        >
          {taskCount} task
        </span>
        <button
          type="button"
          data-testid={`remove-column-${column.key}`}
          onClick={() => onRemove(column.key)}
          disabled={blocked}
          aria-label={`Hapus kolom ${column.name}`}
          title={
            blocked ? `Kolom berisi ${taskCount} task. Hapus menghasilkan 409 Conflict.` : `Hapus kolom ${column.name}`
          }
          className={cn(
            'rounded p-1 transition-colors',
            blocked
              ? 'cursor-not-allowed text-[var(--color-tertiary)] opacity-40'
              : 'text-[var(--color-tertiary)] hover:text-[var(--color-danger)]',
          )}
        >
          <Trash2 size={16} />
        </button>
      </div>
    </div>
  )
}

/** AC1 — the accent-tinted add form at the bottom of the panel. */
function AddColumnForm({ onAdd }: { onAdd: (name: string) => void }) {
  const inputRef = useRef<HTMLInputElement>(null)

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        const value = inputRef.current?.value ?? ''
        onAdd(value)
        if (inputRef.current) inputRef.current.value = ''
      }}
      className="space-y-2.5 rounded-[6px] border border-[var(--color-accent)]/30 bg-[var(--color-accent-tint)]/70 p-3"
    >
      <span className="flex items-center gap-1 font-mono text-[11px] font-bold uppercase text-[var(--color-accent)]">
        <Plus size={15} />
        Tambah kolom baru
      </span>
      <p className="text-[11px] leading-snug text-[var(--color-secondary)]">
        Kolom baru muncul di <strong className="text-[var(--color-primary)]">urutan terakhir</strong> board.
      </p>
      <label className="block">
        <span className="mb-1 block font-mono text-[10px] font-semibold uppercase text-[var(--color-secondary)]">
          Nama kolom baru
        </span>
        <div className="flex items-center gap-2">
          <input
            ref={inputRef}
            type="text"
            placeholder="misal: Blocked, Staging, QA…"
            aria-label="Nama kolom baru"
            className="h-8 flex-1 rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-panel)] px-2.5 text-[12px] text-[var(--color-primary)] placeholder:text-[var(--color-tertiary)] focus:border-[var(--color-accent)] focus:outline-none"
          />
          <Button type="submit" size="sm" className="shrink-0">
            Tambah
          </Button>
        </div>
      </label>
    </form>
  )
}

/**
 * How many tasks a column renders. Uses `columnForStatus`, the same mapping the
 * kanban lanes use, so the badge cannot disagree with what the board shows — a
 * plain `status === key` comparison would report `awaiting_approval` as missing
 * from `running` and let AC2's guard be bypassed from the UI side.
 */
function countTasksByColumn(tasks: Task[]): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const task of tasks) {
    const key = columnForStatus(task.status)
    counts[key] = (counts[key] ?? 0) + 1
  }
  return counts
}

/** Maps a column key onto the status token that colours it, matching the lane. */
function statusTokenFor(key: string): string {
  switch (key) {
    case 'backlog':
      return 'backlog'
    case 'ready':
      return 'ready'
    case 'running':
      return 'running'
    case 'review':
      return 'review'
    case 'done':
      return 'done'
    default:
      // A custom column has no status of its own (DECISIONS 3), so it takes the
      // neutral archived tone rather than borrowing a status colour it does not
      // represent.
      return 'archived'
  }
}

/** A key derived from the name, de-duplicated against the columns already present. */
function uniqueKey(name: string, existing: Column[]): string {
  const base =
    name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'kolom'
  const taken = new Set(existing.map((c) => c.key))
  if (!taken.has(base)) return base
  let n = 2
  while (taken.has(`${base}-${n}`)) n += 1
  return `${base}-${n}`
}

/** The panel's own column glyph, so the header does not depend on an icon font. */
function ColumnsGlyph() {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden>
      <rect x="1.5" y="2.5" width="4" height="11" rx="1" stroke="currentColor" />
      <rect x="6.5" y="2.5" width="4" height="11" rx="1" stroke="currentColor" />
      <rect x="11.5" y="2.5" width="3" height="11" rx="1" stroke="currentColor" />
    </svg>
  )
}
