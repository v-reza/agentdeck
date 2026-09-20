import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'
import { cn } from '@/lib/cn'

/**
 * The app's one modal.
 *
 * DESIGN.md has a `modal` token, so this is not invented: panel is
 * `surface-elevated`, radius `rounded.lg` (14px), padding 20px, and the backdrop
 * is `colors.overlay` = rgba(12,18,16,0.44). Reused everywhere instead of
 * `window.confirm()`/`prompt()`, which the contract forbids — they block the
 * page, cannot be styled, and look nothing like the product.
 *
 * Rendered through a portal so an ancestor with `overflow: hidden` (the board's
 * scroll panes) cannot clip it.
 *
 * Accessibility contract, all of it required:
 *   - `role="dialog"` + `aria-modal` + `aria-labelledby` pointing at the title
 *   - focus moves into the panel on open, and returns to the trigger on close
 *   - `Escape` closes, backdrop click closes, and there is a visible close button
 *   - Tab is trapped inside the panel while open
 *   - background scroll is locked
 *   - the open/close transition honours `prefers-reduced-motion`
 */
export function Modal({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  size = 'md',
}: {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  size?: 'sm' | 'md' | 'lg'
}) {
  const panelRef = useRef<HTMLDivElement>(null)
  const titleID = useRef(`modal-title-${Math.random().toString(36).slice(2, 9)}`).current
  // `onClose` is almost always an inline arrow, so it changes identity on every
  // parent render. Keeping it in a ref lets the effect below depend on `open`
  // alone — otherwise the effect re-runs mid-life, restores focus to whatever is
  // focused *inside* the panel, and records that as the trigger to return to.
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  // Return focus to whatever opened the modal. Captured on the open transition,
  // not on mount, so a modal that starts closed still restores the right element.
  useEffect(() => {
    if (!open) return
    const trigger = document.activeElement as HTMLElement | null

    // Move focus into the panel. Form fields win over the close button: focus
    // landing on "Tutup" would make the first Tab go nowhere useful, and the
    // panel's own close control is earlier in DOM order so a plain
    // `querySelector` would pick it. Fall back to any focusable, then the panel.
    const panel = panelRef.current
    const first =
      panel?.querySelector<HTMLElement>('input:not([type="hidden"]), select, textarea') ??
      panel?.querySelector<HTMLElement>('button, [href], [tabindex]:not([tabindex="-1"])') ??
      panel
    first?.focus()

    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        event.stopPropagation()
        onCloseRef.current()
        return
      }
      if (event.key !== 'Tab' || !panel) return

      // Trap Tab. The focusable list is recomputed per keypress so a field that
      // becomes disabled (a pending submit) drops out of the cycle.
      const focusable = Array.from(
        panel.querySelectorAll<HTMLElement>(
          'input:not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => el.offsetParent !== null || el === document.activeElement)

      if (focusable.length === 0) return
      const firstEl = focusable[0]
      const lastEl = focusable[focusable.length - 1]

      if (event.shiftKey && document.activeElement === firstEl) {
        event.preventDefault()
        lastEl.focus()
      } else if (!event.shiftKey && document.activeElement === lastEl) {
        event.preventDefault()
        firstEl.focus()
      }
    }

    document.addEventListener('keydown', onKeyDown, true)
    return () => {
      document.removeEventListener('keydown', onKeyDown, true)
      document.body.style.overflow = previousOverflow
      trigger?.focus?.()
    }
  }, [open])

  if (!open) return null

  const width = size === 'sm' ? 'w-[360px]' : size === 'lg' ? 'w-[560px]' : 'w-[440px]'

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Backdrop. `motion-safe` keeps the fade for everyone except operators who
          asked the OS for reduced motion. `data-testid` because `[aria-hidden]`
          alone is ambiguous — the app has many. */}
      <div
        data-testid="modal-backdrop"
        className="absolute inset-0 bg-[var(--color-overlay)] motion-safe:animate-[fade-in_120ms_ease-out]"
        onClick={onClose}
        aria-hidden="true"
      />

      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        tabIndex={-1}
        className={cn(
          'relative flex max-h-[85vh] flex-col overflow-hidden',
          'rounded-[14px] border border-[var(--color-border-standard)] bg-[var(--color-surface-elevated)] p-5 shadow-card',
          'motion-safe:animate-[modal-in_140ms_ease-out]',
          width,
        )}
      >
        <div className="mb-4 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h2 id={titleID} className="text-[14px] font-semibold tracking-tight text-[var(--color-primary)]">
              {title}
            </h2>
            {description ? <p className="mt-1 text-[12px] text-[var(--color-tertiary)]">{description}</p> : null}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Tutup"
            className="-mr-1 -mt-1 shrink-0 rounded-[6px] p-1.5 text-[var(--color-tertiary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]"
          >
            <X size={14} />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>

        {footer ? (
          <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
            {footer}
          </div>
        ) : null}
      </div>
    </div>,
    document.body,
  )
}
