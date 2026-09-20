import { isRejectedWithValue } from '@reduxjs/toolkit'
import type { AppStartListening } from '../index'
import { pushToast } from '../slices/toastSlice'
import { describeError } from '@/hooks/use-action-form'

/**
 * Reports a rejected RTK Query *read* as a toast (ARCHITECTURE 18.2
 * `store/listeners/toast.ts`).
 *
 * It listens for `isRejectedWithValue`, not `isRejected`: a rejection without a
 * value is an aborted or unstarted request (a component unmounted, a query was
 * skipped), and toasting those would blame the operator for nothing.
 *
 * Writes are deliberately not toasted. Every mutating screen renders the
 * failure next to the control that caused it — that is the rule
 * `use-action-form` documents ("the operator needs the message next to the
 * field that caused it") — so a toast repeats the same sentence in a second
 * place. The reset-confirm screen showed exactly that: one inline error and one
 * toast reading `reset token invalid`, which is also how this was found.
 * A write that has no form to render into reads its own `isError` from the
 * mutation result instead.
 */
export function registerToastListener(startListening: AppStartListening) {
  startListening({
    matcher: isRejectedWithValue,
    effect: (action, api) => {
      const payload = (action as { payload?: unknown }).payload
      const request = (action as { meta?: { arg?: { type?: string } } }).meta?.arg
      const kind = request?.type === 'mutation' ? 'mutation' : 'query'

      if (!shouldToastRejection(kind, payload)) return

      api.dispatch(
        pushToast({
          tone: 'danger',
          title: 'Request failed',
          body: describeError(payload),
        }),
      )
    },
  })
}

/**
 * Whether a rejection deserves the operator's attention.
 *
 * A read that answers 404 is an endpoint the fleet has not deployed yet — the
 * cost rail's budget and ledger are M2 (ARCHITECTURE 6.2.14) and answer 404
 * today, and each of them raised a toast on every board visit. Those reads
 * already render their own empty state, so the toast only repeats a fact the
 * page states calmly, and it blames the operator for a server-side gap.
 *
 * The status must be read from `originalStatus`, not `status`: the Go server
 * answers a missing route with a plain-text `404 page not found`, so
 * `fetchBaseQuery` cannot parse it as JSON and reports `status: 'PARSING_ERROR'`
 * with the HTTP code in `originalStatus`. Checking `status` alone therefore
 * never matched and the toasts kept firing (verified by reading the payload the
 * listener actually receives in the browser).
 */
export function shouldToastRejection(kind: 'query' | 'mutation', payload: unknown): boolean {
  // A write reports itself where it happened; only reads are toasted globally.
  if (kind === 'mutation') return false
  const error = payload as { status?: unknown; originalStatus?: unknown } | null
  if (error?.status === 404 || error?.originalStatus === 404) return false
  return true
}
