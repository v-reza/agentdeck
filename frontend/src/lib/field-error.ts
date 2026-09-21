import { describeError } from '@/hooks/use-action-form'

/**
 * Routes a rejection to the field that caused it.
 *
 * A form that prints one message under the whole thing is unusable: the
 * operator has to work out which of a dozen inputs the server disliked. The
 * backend answers with a message, not a field name, so the mapping lives here
 * and is keyed on the exact strings the API emits — a message that does not
 * match still surfaces at form level rather than being dropped.
 */
export interface FieldErrors {
  /** Keyed by the form field name (`name`, `model`, `baseUrl`, `apiKey`). */
  byField: Record<string, string>
  /** A message that belongs to no single field. */
  form: string | null
}

export const NO_FIELD_ERRORS: FieldErrors = { byField: {}, form: null }

/**
 * `message → field`. Ordered: the first pattern that matches wins, so the more
 * specific ones come first.
 *
 * Every pattern below is copied from a sentinel in `internal/board/service.go`,
 * not invented. A new backend message simply falls through to the form level,
 * which is the safe direction — mis-attributing a message to the wrong field
 * would point the operator at a field they already filled in correctly.
 */
const ROUTES: [RegExp, string][] = [
  [/^an agent with this name already exists/i, 'name'],
  [/^base_url is required/i, 'baseUrl'],
  [/^base_url is not a reachable/i, 'baseUrl'],
  [/^unknown provider$/i, 'provider'],
  [/^unknown model$/i, 'model'],
  [/^agent has no stored provider credential/i, 'apiKey'],
  [/^provider handshake failed/i, 'apiKey'],
  [/^provider credential encryption is not configured/i, 'apiKey'],
  [/^invalid input$/i, 'name'],
]

export function routeFieldError(error: unknown, prefix?: string): FieldErrors {
  const message = describeError(error)
  const label = (text: string) => (prefix ? `${prefix}: ${text}` : text)
  for (const [pattern, field] of ROUTES) {
    if (pattern.test(message)) return { byField: { [field]: label(message) }, form: null }
  }
  return { byField: {}, form: label(message) }
}
