import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react'
import { Check, ChevronDown, Search } from 'lucide-react'
import { cn } from '@/lib/cn'

/**
 * The app's one dropdown.
 *
 * Every list in the shell used to be a native `<select>`, and a native control
 * cannot be themed: the popup is drawn by the OS, so the rows ignore
 * `--color-surface-panel`, the highlight ignores `--color-accent-tint`, and the
 * arrow is the browser's own glyph. That is the "not standard" look — a filter
 * built as a button + listbox next to a form built from `<select>` reads as two
 * different products.
 *
 * So this is the one control both use. It is a button + `role="listbox"`, the
 * same shape `StatusFilter` proved out, plus the two things a form needs and a
 * four-item filter did not:
 *
 *  - **search**, on by default once the list is longer than `searchThreshold`.
 *    A catalog of 220 models is not navigable by arrow keys.
 *  - **a hidden input carrying `name`**, so a plain `<form>` submit (or
 *    `new FormData(form)`) still sees the value. Every form in the app reads
 *    `FormData` rather than component state, and a listbox that only lived in
 *    React state would silently drop the field.
 *
 * Keyboard contract matches `AccountMenu` and `StatusFilter`: Escape closes and
 * restores focus to the trigger, ArrowUp/ArrowDown move the active row, Enter
 * selects, and clicking outside closes.
 */
export interface ComboboxOption {
  value: string
  label: string
  /** Optional right-aligned monospace note, e.g. a price or a snapshot id. */
  note?: string
}

/**
 * How many combobox popups are open, across the whole document.
 *
 * The modal listens for Escape on `document` in the capture phase, so its
 * handler runs *before* the popup's own React handler can stop the event. Left
 * alone, Escape aimed at an open dropdown closes the entire form and the
 * operator loses everything they had typed. The modal asks this instead of
 * guessing from focus, which would break the moment a field is empty.
 */
let openPopups = 0

/** True while any combobox list is showing. */
export function isComboboxPopupOpen() {
  return openPopups > 0
}

export function Combobox({
  name,
  value,
  onChange,
  options,
  label,
  placeholder,
  searchThreshold = 8,
  allowCustom = false,
  icon,
  className,
  id,
  form,
  disabled,
  invalid = false,
}: {
  /** Rendered as a hidden input so `FormData` carries the value. */
  name?: string
  value: string
  onChange: (next: string) => void
  options: ComboboxOption[]
  /** Accessible name. Required: an unlabelled listbox is unusable by screen reader. */
  label: string
  placeholder?: string
  /** Show the search field once the list reaches this many rows. */
  searchThreshold?: number
  /**
   * Accept a value the list does not contain.
   *
   * Two fields need this and both are real: the model of a BYO provider comes
   * from the operator's own endpoint (US-AD106 AC2), so it is in no catalog of
   * ours; and US-AD96 AC1 wants a model *outside* the catalog to be refused
   * inline, which is only a rule if the operator can get to type it. Off, the
   * list is closed and the field is a picker.
   */
  allowCustom?: boolean
  /** Leading glyph inside the control, for a toolbar where the icon carries the meaning. */
  icon?: ReactNode
  className?: string
  id?: string
  /** Ties the hidden input to a `<form>` outside this subtree, as the topbar save button does. */
  form?: string
  disabled?: boolean
  /** Draws the danger border. Set when the host routed a rejection to this field. */
  invalid?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const listboxID = useId()

  const selected = options.find((option) => option.value === value)
  // Search is on whenever the list is long, or whenever the operator may type a
  // value that is not in it — the field is a text box then, not a picker.
  const searchable = options.length >= searchThreshold || allowCustom
  const filtered = useMemo(() => {
    if (query.trim() === '') return options
    const needle = query.trim().toLowerCase()
    return options.filter(
      (option) => option.value.toLowerCase().includes(needle) || option.label.toLowerCase().includes(needle),
    )
  }, [options, query])

  // Reset the cursor to the selected row each time the list opens, so Enter
  // repeats the current value instead of jumping to the first option.
  useEffect(() => {
    if (!open) return
    setQuery(allowCustom ? value : '')
    const index = options.findIndex((option) => option.value === value)
    setActiveIndex(index < 0 ? 0 : index)
    if (searchable) searchRef.current?.focus()
  }, [open, options, value, searchable, allowCustom])

  useEffect(() => {
    if (!open) return
    function onPointerDown(event: MouseEvent) {
      if (containerRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }
    document.addEventListener('mousedown', onPointerDown)
    return () => document.removeEventListener('mousedown', onPointerDown)
  }, [open])

  // Keep the document-wide count honest: the popup can be closed by a click
  // outside, by choosing a row, or by the field unmounting mid-flight.
  useEffect(() => {
    if (!open) return
    openPopups += 1
    return () => {
      openPopups -= 1
    }
  }, [open])

  function close(restoreFocus: boolean) {
    setOpen(false)
    if (restoreFocus) triggerRef.current?.focus()
  }

  function choose(option: ComboboxOption) {
    onChange(option.value)
    close(true)
  }

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === 'Escape') {
      event.stopPropagation()
      close(true)
      return
    }
    if (!open && (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault()
      setOpen(true)
      return
    }
    if (!open) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((index) => Math.min(index + 1, filtered.length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((index) => Math.max(index - 1, 0))
    } else if (event.key === 'Enter') {
      event.preventDefault()
      const option = filtered[activeIndex]
      if (option) choose(option)
    }
  }

  return (
    <div ref={containerRef} className={cn('relative', className)}>
      {/* The value a form submit reads. When the field is a text box the visible
          input IS the form field, so a second one here would submit the value
          twice. */}
      {name && !allowCustom ? <input type="hidden" name={name} id={id} form={form} value={value} /> : null}

      {allowCustom ? (
        <div className="relative">
          <input
            type="text"
            name={name}
            id={id}
            form={form}
            role="combobox"
            aria-label={label}
            aria-expanded={open}
            aria-controls={open ? listboxID : undefined}
            aria-autocomplete="list"
            disabled={disabled}
            value={value}
            placeholder={placeholder}
            autoComplete="off"
            onChange={(event) => {
              onChange(event.target.value)
              if (!open) setOpen(true)
            }}
            onFocus={() => setOpen(true)}
            onKeyDown={onKeyDown}
            className={cn(
              'h-8 w-full rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-panel)]',
              invalid && 'border-[var(--color-danger)]',
              'pr-7 pl-2.5 font-mono text-[12px] text-[var(--color-primary)]',
              'placeholder:font-sans placeholder:text-[var(--color-tertiary)]',
              'focus:border-[var(--color-accent)] focus:outline-none disabled:cursor-not-allowed disabled:opacity-50',
            )}
          />
          <ChevronDown
            size={13}
            aria-hidden
            className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-[var(--color-tertiary)]"
          />
        </div>
      ) : (
        <button
          ref={triggerRef}
          type="button"
          role="combobox"
          aria-label={label}
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-controls={open ? listboxID : undefined}
          disabled={disabled}
          onClick={() => setOpen((current) => !current)}
          onKeyDown={onKeyDown}
          className={cn(
            'flex h-8 w-full items-center gap-2 rounded-[6px] border border-[var(--color-border-standard)]',
            invalid && 'border-[var(--color-danger)]',
            'bg-[var(--color-surface-panel)] px-2.5 text-left text-[12px] text-[var(--color-primary)]',
            'focus:border-[var(--color-accent)] focus:outline-none',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          {icon ? <span className="shrink-0 text-[var(--color-tertiary)]">{icon}</span> : null}
          <span className={cn('min-w-0 flex-1 truncate font-mono', !selected && 'text-[var(--color-tertiary)]')}>
            {selected?.label ?? placeholder ?? ''}
          </span>
          <ChevronDown size={13} className="shrink-0 text-[var(--color-tertiary)]" />
        </button>
      )}

      {open ? (
        <div className="absolute left-0 z-30 mt-1 w-full min-w-max rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-panel)] shadow-md">
          {searchable ? (
            <div className="flex items-center gap-1.5 border-b border-[var(--color-border-subtle)] px-2.5 py-1.5">
              <Search size={12} className="shrink-0 text-[var(--color-tertiary)]" />
              <input
                ref={searchRef}
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value)
                  setActiveIndex(0)
                }}
                onKeyDown={onKeyDown}
                placeholder={label}
                aria-label={label}
                className="w-full bg-transparent text-[12px] text-[var(--color-primary)] placeholder:text-[var(--color-tertiary)] focus:outline-none"
              />
            </div>
          ) : null}

          <ul id={listboxID} role="listbox" aria-label={label} className="max-h-[240px] overflow-y-auto py-1">
            {filtered.length === 0 ? (
              <li className="px-2.5 py-1.5 text-[12px] text-[var(--color-tertiary)]">—</li>
            ) : (
              filtered.map((option, index) => {
                const isSelected = option.value === value
                return (
                  <li key={option.value}>
                    <button
                      type="button"
                      role="option"
                      aria-selected={isSelected}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={() => choose(option)}
                      className={cn(
                        'flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-[12px] whitespace-nowrap',
                        index === activeIndex
                          ? 'bg-[var(--color-accent-tint)] text-[var(--color-accent)]'
                          : 'text-[var(--color-secondary)]',
                        isSelected && 'font-semibold',
                      )}
                    >
                      <Check
                        size={12}
                        className={cn('shrink-0', isSelected ? 'opacity-100' : 'opacity-0')}
                        aria-hidden
                      />
                      <span className="min-w-0 flex-1 truncate font-mono">{option.label}</span>
                      {option.note ? (
                        <span className="shrink-0 font-mono text-[10px] text-[var(--color-tertiary)]">
                          {option.note}
                        </span>
                      ) : null}
                    </button>
                  </li>
                )
              })
            )}
          </ul>
        </div>
      ) : null}
    </div>
  )
}
