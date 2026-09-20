import { useDroppable } from '@dnd-kit/core'
import { cn } from '@/lib/cn'
import { TaskCard } from './TaskCard'
import { COLUMN_LABELS } from '@/lib/domain'
import type { ColumnKey, Task } from '@/lib/domain'

/**
 * One board column (ARCHITECTURE 18.2 `components/kanban/Column`). A column is a
 * view of status (DECISIONS 3), so it never invents a status of its own — the
 * droppable id IS the column key and the drop handler maps it back to a status.
 */
export function Column({
  columnKey,
  tasks,
  onOpenTask,
}: {
  columnKey: ColumnKey
  tasks: Task[]
  onOpenTask: (id: string) => void
}) {
  const { setNodeRef, isOver } = useDroppable({ id: columnKey })

  return (
    <section className="flex w-[268px] min-w-[268px] flex-col rounded-[10px] bg-[var(--color-surface-sunken)]/60 p-2">
      <header className="mb-2 flex items-center justify-between px-1">
        <h2 className="text-[11px] font-bold uppercase tracking-[0.06em] text-[var(--color-tertiary)]">
          {COLUMN_LABELS[columnKey]}
        </h2>
        <span className="font-mono text-[11px] text-[var(--color-tertiary)]">{tasks.length}</span>
      </header>

      <div
        ref={setNodeRef}
        className={cn(
          'flex min-h-[80px] flex-col gap-2 rounded-[8px] p-1 transition-colors',
          isOver && 'bg-[var(--color-accent-tint)]',
        )}
      >
        {tasks.map((task) => (
          <TaskCard key={task.id} task={task} onOpen={onOpenTask} />
        ))}
      </div>
    </section>
  )
}
