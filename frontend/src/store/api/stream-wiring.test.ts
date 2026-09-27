import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { boardTaskTag } from './base'

/**
 * US-AD39 (Must, M3) — the event stream must refresh the board's task list.
 *
 * This is the failure that has no symptom. `EventSource` connects, frames
 * arrive, the tail fills — and the board never re-renders, because the
 * invalidation never reaches the store. Nothing throws, nothing logs. The
 * operator sees a board that never updates and concludes realtime is broken,
 * while every piece involved reports success.
 *
 * Two ways to write that bug, both of which looked correct and neither of which
 * any other test could see:
 *
 *  1. Name a different tag on each end (`BOARD-x` here, `BOARD:x` there).
 *  2. Declare `invalidatesTags` on this endpoint at all. It has `queryFn`, not
 *     `query`, and the `queryFn` variant of an endpoint definition has no
 *     `invalidatesTags` — the type is `never` and the value is ignored. It
 *     compiles, it reads correctly, and it does nothing. This was the first
 *     version of this code, and only `tsc` caught it.
 *
 * So this is a source-level check on purpose, the same way `agents.test.ts` pins
 * the create/update field drift: no handler test can see an invalidation that
 * was never dispatched.
 */

const here = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(resolve(here, f), 'utf8')

/**
 * One endpoint's body, whitespace-normalised.
 *
 * Slicing to the end of the file is not enough: a teammate endpoint that also
 * mentions `boardTaskTag` would satisfy the assertion for the endpoint under
 * test. (That exact hole was found by mutation — replacing `listTasks`'s call
 * with a literal still passed.) The block ends at the next sibling definition.
 */
function endpoint(source: string, name: string): string {
  const start = source.indexOf(`${name}: build.`)
  if (start < 0) throw new Error(`endpoint not found in source: ${name}`)
  const rest = source.slice(start + 1)
  const nextSibling = rest.search(/\n {4}\w+: build\./)
  const body = nextSibling < 0 ? rest : rest.slice(0, nextSibling)
  return body.replace(/\s+/g, ' ')
}

const boards = read('boards.ts')
const stream = read('stream.ts')
const base = read('base.ts')

const boardEvents = endpoint(stream, 'boardEvents')
const listTasks = endpoint(boards, 'listTasks')
const createTask = endpoint(boards, 'createTask')

describe('the event stream refreshes the tag the task list provides', () => {
  it('the task list provides the shared board tag', () => {
    expect(listTasks).toMatch(/providesTags:[^;]*boardTaskTag\(boardID\)/)
  })

  it('the stream dispatches an invalidation of that same tag', () => {
    expect(boardEvents).toMatch(/invalidateTags\(\[boardTaskTag\(boardID\)\]\)/)
  })

  it('the invalidation runs from the event handler, so it fires per event', () => {
    // A dispatched invalidation that is defined but never called is the same bug
    // wearing a different hat: the helper exists, the call does not happen.
    expect(boardEvents).toMatch(/refreshBoard\(\)/)
    expect(boardEvents).toMatch(/append = \(event: MessageEvent<string>\) => \{/)
  })

  it('NEVER declares invalidatesTags on the stream endpoint', () => {
    // `queryFn` endpoints ignore it — the type is `never`. Declaring it is the
    // bug that shipped first: correct-looking, type-checked, and inert.
    expect(boardEvents).not.toMatch(/invalidatesTags:/)
    expect(stream).toMatch(/queryFn:/)
  })

  it('createTask invalidates through the helper too, not a second literal', () => {
    // This tag used to be spelled out in three places and only two agreed.
    expect(createTask).toMatch(/invalidatesTags:[^;]*boardTaskTag\(boardID\)/)
  })

  it('no caller spells the Task tag inline', () => {
    // `base.ts` is the one place allowed to name the prefix.
    //
    // Only the Task tag is pinned. `finops.ts` and the `Ledger` tag legitimately
    // build their own `BOARD-x` ids under a DIFFERENT tag type, which cannot
    // collide with this one — banning the string outright would be asserting a
    // style rule instead of the wiring bug.
    for (const source of [boards, stream]) {
      expect(source).not.toMatch(/type: 'Task',? id: `BOARD-\$\{/)
    }
    expect(base).toMatch(/type: 'Task' as const, id: `BOARD-\$\{boardID\}`/)
  })

  it('both ends import the helper from base', () => {
    for (const source of [boards, stream]) {
      expect(source).toMatch(/import \{[^}]*boardTaskTag[^}]*\} from '\.\/base'/)
    }
  })

  it('an unresolved board subscribes to nothing', () => {
    // Empty board id means "not subscribed" — `useSseCache` skips the query, and
    // the lifecycle returns before opening an EventSource for board ''.
    expect(boardEvents).toMatch(/if \(!boardID\) return/)
  })

  it('a pending refresh is cancelled when the board goes away', () => {
    // Otherwise a coalesced refetch fires for a cache entry nobody subscribes to.
    // The position is pinned: `clearTimeout(refresh)` also appears inside
    // `refreshBoard` (where it cancels the previous timer), so asserting its
    // presence alone passed even with the cleanup deleted.
    expect(boardEvents).toMatch(/source\.close\(\).{0,240}if \(refresh\) clearTimeout\(refresh\)/)
  })

  it('no endpoint invalidates an Event tag, because nothing provides one', () => {
    // `Event` is a declared tag type with zero providers, so invalidating it
    // refreshed nothing while reading like it refreshed the timeline.
    expect(boards).not.toMatch(/type: 'Event'/)
    expect(stream).not.toMatch(/type: 'Event'/)
  })
})

describe('the board tag is scoped, never global', () => {
  it('differs per board', () => {
    expect(boardTaskTag('a')).not.toEqual(boardTaskTag('b'))
  })

  it('carries its board id so RTK can match one board', () => {
    expect(boardTaskTag('01ABC')).toEqual({ type: 'Task', id: 'BOARD-01ABC' })
  })
})
