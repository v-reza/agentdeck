import { describe, expect, it } from 'vitest'
import { streamShouldGiveUp } from './stream'

/**
 * US-AD13 (Must, M1) — the task drawer's timeline reads the board event stream.
 *
 * `EventSource` reconnects on its own forever, so an endpoint that is not
 * deployed yet (ARCHITECTURE 6.2.13 SSE is M2 and answers 404) becomes an
 * unbounded request loop in the operator's console. These assertions pin the
 * bound: a few consecutive failures close the stream instead of retrying for
 * the lifetime of the tab.
 */
describe('board event stream retry bound', () => {
  it('keeps retrying while the connection may still recover', () => {
    expect(streamShouldGiveUp(0)).toBe(false)
    expect(streamShouldGiveUp(1)).toBe(false)
  })

  it('gives up once consecutive failures pass the bound', () => {
    expect(streamShouldGiveUp(3)).toBe(true)
    expect(streamShouldGiveUp(9)).toBe(true)
  })
})
