import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { dismissToast, type ToastTone } from '@/store/slices/toastSlice'

/**
 * Renders the toast queue. The queue is filled by `store/listeners/toast.ts`,
 * which turns rejected mutations and budget-threshold events into messages — no
 * component raises a toast directly, so a failure cannot go unreported because
 * one screen forgot to handle it.
 */
export function ToastHost() {
  const dispatch = useAppDispatch()
  const toasts = useAppSelector((state) => state.toast.items)

  if (toasts.length === 0) return null

  return (
    <div className="pointer-events-none fixed right-4 bottom-4 z-50 flex w-[320px] flex-col gap-2">
      {toasts.map((toast) => (
        <button
          key={toast.id}
          type="button"
          onClick={() => dispatch(dismissToast(toast.id))}
          className="pointer-events-auto rounded-[8px] border bg-[var(--color-surface-panel)] p-3 text-left shadow-lg"
          style={{ borderColor: borderFor(toast.tone) }}
        >
          <div className="text-[12px] font-semibold" style={{ color: borderFor(toast.tone) }}>
            {toast.title}
          </div>
          {toast.body ? <div className="mt-0.5 text-[11px] text-[var(--color-secondary)]">{toast.body}</div> : null}
        </button>
      ))}
    </div>
  )
}

/** The border/heading colour for a tone, resolved from the DESIGN.md tokens. */
function borderFor(tone: ToastTone): string {
  switch (tone) {
    case 'success':
      return 'var(--color-success)'
    case 'warning':
      return 'var(--color-warning)'
    case 'danger':
      return 'var(--color-danger)'
    case 'info':
      return 'var(--color-info)'
  }
}
