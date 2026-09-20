import { useEffect, useRef, useState } from 'react'
import { ChevronDown, Filter } from 'lucide-react'
import { cn } from '@/lib/cn'

/** Which bucket the registry table is narrowed to. `ready` mirrors
 * `has_provider_key`; `archived` is its own bucket, never mixed into the other
 * two — a retired agent is not "ready to take a task" whatever its credentials
 * say. */
export type StatusFilterValue = 'all' | 'ready' | 'needsKey' | 'archived'

/**
 * The toolbar's status filter.
 *
 * A real popover, not a `<select>`. A native select cannot render a trigger that
 * matches the design's control: `appearance-none` removes the OS arrow, and the
 * OS arrow is the only thing that made it read as a dropdown. It also sizes to
 * its longest option — measured, the longest label needs 120px while the control
 * gave the text 92px, so it clipped the word. A button + listbox keeps the
 * design's look, removes the clipping, and stays keyboard reachable: Escape
 * closes and returns focus, matching `AccountMenu`.
 */
export function StatusFilter({
  value,
  onChange,
  label,
  options,
}: {
  value: StatusFilterValue
  onChange: (next: StatusFilterValue) => void
  label: string
  options: { value: StatusFilterValue; label: string }[]
}) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const active = options.find((option) => option.value === value) ?? options[0]

  useEffect(() => {
    if (!open) return
    function onPointerDown(event: MouseEvent) {
      if (containerRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      setOpen(false)
      triggerRef.current?.focus()
    }
    document.addEventListener('mousedown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  return (
    <div ref={containerRef} className="relative">
      <button
        ref={triggerRef}
        type="button"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="flex h-[30px] items-center gap-1.5 rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-panel)] px-2.5 text-[12px] font-medium whitespace-nowrap text-[var(--color-secondary)] hover:bg-[var(--color-surface-page)]"
      >
        <Filter size={13} className="shrink-0 text-[var(--color-tertiary)]" />
        <span>{active.label}</span>
        <ChevronDown size={13} className="shrink-0 text-[var(--color-tertiary)]" />
      </button>

      {open ? (
        <ul
          role="listbox"
          aria-label={label}
          className="absolute right-0 z-20 mt-1 min-w-full rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-panel)] py-1 shadow-md"
        >
          {options.map((option) => (
            <li key={option.value}>
              <button
                type="button"
                role="option"
                aria-selected={option.value === value}
                onClick={() => {
                  onChange(option.value)
                  setOpen(false)
                  triggerRef.current?.focus()
                }}
                className={cn(
                  'block w-full px-2.5 py-1.5 text-left text-[12px] whitespace-nowrap',
                  option.value === value
                    ? 'bg-[var(--color-accent-tint)] font-semibold text-[var(--color-accent)]'
                    : 'text-[var(--color-secondary)] hover:bg-[var(--color-surface-page)]',
                )}
              >
                {option.label}
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
