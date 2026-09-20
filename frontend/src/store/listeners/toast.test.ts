import { describe, expect, it } from 'vitest'
import { shouldToastRejection } from './toast'

/**
 * US-AD32 AC3 — the cost rail reports a board budget, and ARCHITECTURE 6.2.14
 * keeps that endpoint in M2, so it answers 404 until the runtime ships.
 *
 * A 404 on a read is "this is not deployed yet", not "your request was wrong":
 * toasting it blames the operator for a server the fleet has not shipped. This
 * pins the rule so the toast listener keeps reporting real failures (a 500, a
 * 403, a rejected write) while staying quiet about an absent read endpoint.
 */
describe('toast listener rejection filter', () => {
  it('stays quiet for an absent read endpoint (M2 404)', () => {
    expect(shouldToastRejection('query', { status: 404 })).toBe(false)
  })

  it('stays quiet when the 404 body is not JSON', () => {
    // The real wire shape: Go answers an unknown route with the plain-text body
    // "404 page not found", so fetchBaseQuery reports PARSING_ERROR and carries
    // the HTTP code in originalStatus. This is the case that shipped broken.
    expect(shouldToastRejection('query', { status: 'PARSING_ERROR', originalStatus: 404 })).toBe(false)
  })

  it('still reports a non-404 parse error', () => {
    expect(shouldToastRejection('query', { status: 'PARSING_ERROR', originalStatus: 502 })).toBe(true)
  })

  it('leaves a failed write to the screen that made it', () => {
    // A write renders its own error next to the control that caused it, so a
    // toast would state the same failure twice. This shipped broken on the
    // reset-confirm screen: one inline error plus one toast, both reading
    // "reset token invalid".
    expect(shouldToastRejection('mutation', { status: 404 })).toBe(false)
    expect(shouldToastRejection('mutation', { status: 500 })).toBe(false)
  })

  it('still reports server errors and authorisation failures on reads', () => {
    expect(shouldToastRejection('query', { status: 500 })).toBe(true)
    expect(shouldToastRejection('query', { status: 403 })).toBe(true)
    expect(shouldToastRejection('query', { status: 401 })).toBe(true)
  })
})
