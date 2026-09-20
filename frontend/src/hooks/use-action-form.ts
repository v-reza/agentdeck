import { useActionState } from 'react'
import type { FetchBaseQueryError } from '@reduxjs/toolkit/query'

/**
 * ARCHITECTURE 18.2: "Form mutasi | `useActionState` (React 19) men-dispatch
 * thunk". Every mutating form goes through this hook, so no form keeps its own
 * `useState` for pending or error and every submit handler has the same shape.
 *
 * The mutation trigger itself dispatches, so the action awaits `.unwrap()` for
 * the real outcome instead of guessing from cache state.
 */
export interface FormState {
  error: string | null
  /**
   * True once the action resolved. Screens whose success is a redirect ignore
   * it; screens whose success is a message on the same page (the reset request)
   * need it, because "submitted" and "not yet submitted" must look different
   * without inventing a second piece of state.
   */
  done: boolean
}

const EMPTY: FormState = { error: null, done: false }

/** The shape every RTK Query mutation trigger already has. */
export interface MutationTrigger<TArg> {
  (arg: TArg): { unwrap: () => Promise<unknown> }
}

export function useActionForm<TArg>(
  trigger: MutationTrigger<TArg>,
  build: (form: FormData) => TArg,
  onSuccess?: (result: unknown) => void,
): [FormState, (form: FormData) => void, boolean] {
  const [state, formAction, isPending] = useActionState(
    async (_prev: FormState, form: FormData): Promise<FormState> => {
      try {
        const result = await trigger(build(form)).unwrap()
        onSuccess?.(result)
        return { error: null, done: true }
      } catch (error) {
        return { error: describeError(error), done: false }
      }
    },
    EMPTY,
  )

  return [state, formAction, isPending]
}

/** Renders an RTK Query rejection as something a human can act on. */
export function describeError(error: unknown): string {
  if (typeof error === 'string') return error
  if (error && typeof error === 'object') {
    const candidate = error as Partial<FetchBaseQueryError> & { data?: unknown }
    if (typeof candidate.data === 'string' && candidate.data.trim()) return candidate.data.trim()
    if (candidate.status) return `Request failed (${String(candidate.status)})`
  }
  return 'Request failed'
}
