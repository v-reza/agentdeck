import { useDraggable } from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
import { cn } from '@/lib/cn'
import { formatMicroUSD, shortID } from '@/lib/formatters'
import type { Task } from '@/lib/domain'

/**
 * One draggable task card (ARCHITECTURE 18.2 `components/kanban/`).
 *
 * The card shows what the design source shows: status stripe, title, id, cost.
 * Cost is the task's own `cost_micros` (integer micro-USD) — never an estimate.
 */
export function TaskCard({ task, onOpen }: { task: Task; onOpen: (id: string) => void }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: task.id })

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform) }}
      className={cn(
        'group cursor-grab rounded-[8px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-3 shadow-[0_1px_2px_rgba(12,26,22,0.04)] transition-shadow',
        isDragging && 'opacity-50',
      )}
      {...attributes}
      {...listeners}
      onClick={() => onOpen(task.id)}
    >
      <div className="flex items-start gap-2">
        <span
          className="mt-[3px] h-3 w-[3px] shrink-0 rounded-full"
          style={{ background: `var(--color-status-${task.status.replace(/_/g, '-')})` }}
        />
        <div className="min-w-0 flex-1">
          <p className="truncate text-[12px] font-medium leading-snug text-[var(--color-primary)]">{task.title}</p>
          <div className="mt-1.5 flex items-center gap-2 font-mono text-[10px] text-[var(--color-tertiary)]">
            <span>{shortID(task.id)}</span>
            {task.cost_micros > 0 ? <span>{formatMicroUSD(task.cost_micros)}</span> : null}
            {task.consecutive_failures > 0 ? (
              <span className="text-[var(--color-danger)]">{task.consecutive_failures}× failed</span>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  )
}
