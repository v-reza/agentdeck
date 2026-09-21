import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'
import { cn } from '@/lib/cn'
import { isComboboxPopupOpen } from '@/components/ui/combobox'

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
 *
 * `placement` exists for the 420px credential panel (US-AD86), which the design
 * draws anchored to the right edge at full height rather than centred. The
 * alternative was a second dialog component with a copy of the focus trap, the
 * Escape handler, the scroll lock and the focus restore — four things that are
 * easy to get subtly wrong once, let alone twice. Geometry is a prop; the
 * accessibility contract stays in one place.
 *
 * One modal can open another (the register modal opens the credential panel),
 * so Escape and Tab are handled by the *topmost* dialog only. Without this,
 * one Escape closes both — the operator loses the half-filled form behind the
 * panel they meant to dismiss.
 */

/** Open modals, oldest first. The last entry is the one that owns the keyboard. */
const openDialogs: object[] = []

export function Modal({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  size = 'md',
  placement = 'center',
}: {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  size?: 'sm' | 'md' | 'lg' | 'panel'
  placement?: 'center' | 'right'
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
    const self = {}
    openDialogs.push(self)

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

    // Only the topmost modal locks scroll. A nested panel inherits the lock the
    // outer one already took, so it must not clear it on the way out.
    const innermost = openDialogs[openDialogs.length - 1] === self
    const previousOverflow = document.body.style.overflow
    if (innermost) document.body.style.overflow = 'hidden'

    function onKeyDown(event: KeyboardEvent) {
      // A modal that is not topmost is inert: the keyboard belongs to the dialog
      // above it, so Escape dismisses one layer, not the whole stack.
      if (openDialogs[openDialogs.length - 1] !== self) return

      if (event.key === 'Escape') {
        // An open combobox popup owns Escape: it is one layer above this dialog,
        // and this listener runs in the capture phase, so the popup never gets
        // the chance to claim the key itself. Closing the whole form here would
        // discard a half-filled registration.
        if (isComboboxPopupOpen()) return
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
      const index = openDialogs.indexOf(self)
      if (index >= 0) openDialogs.splice(index, 1)
      // Only restore what this modal changed. A nested panel must leave the
      // outer modal's lock in place, or the page behind both would scroll.
      if (innermost) document.body.style.overflow = previousOverflow
      trigger?.focus?.()
    }
  }, [open])

  if (!open) return null

  const width = size === 'sm' ? 'w-[360px]' : size === 'lg' ? 'w-[560px]' : size === 'panel' ? 'w-[420px]' : 'w-[440px]'
  const right = placement === 'right'

  return createPortal(
    <div
      className={
        right ? 'fixed inset-0 z-50 flex justify-end' : 'fixed inset-0 z-50 flex items-center justify-center p-4'
      }
    >
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
          'relative flex shrink-0 flex-col overflow-hidden',
          // `shrink-0` keeps the right panel at exactly 420px instead of letting
          // the flex row shave a pixel off it; `max-w-full` keeps every size
          // usable when the viewport is narrower than the panel.
          right ? 'h-full max-h-full shrink-0 rounded-none' : 'max-h-[85vh] rounded-[14px]',
          'max-w-full',
          'border border-[var(--color-border-standard)] bg-[var(--color-surface-elevated)] p-5 shadow-card',
          // The right-anchored panel fades only. `modal-in` carries a translateY,
          // and a transform on a full-height element leaves it briefly unstable —
          // a click that lands during the 140ms animation can miss its target.
          right ? 'motion-safe:animate-[fade-in_120ms_ease-out]' : 'motion-safe:animate-[modal-in_140ms_ease-out]',
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
