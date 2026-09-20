import { describe, expect, it } from 'vitest'
import reducer, {
  reportBoardCount,
  reportBoardTaskStats,
  directoryTotals,
  boardStats,
  setBoardSort,
} from './directorySlice'

/**
 * The projection exists so the summary row can total per-row answers without
 * inventing a second source of truth. The rule under test is that a row which
 * has not answered is *absent*, never counted as a zero — otherwise the totals
 * would claim knowledge the server never sent.
 */
describe('directoryTotals', () => {
  it('sums only what was reported', () => {
    let state = reducer(undefined, reportBoardCount({ projectID: 'p1', count: 3 }))
    state = reducer(state, reportBoardCount({ projectID: 'p2', count: 2 }))
    state = reducer(state, reportBoardTaskStats({ boardID: 'b1', tasks: 10, running: 2 }))

    expect(directoryTotals(state)).toEqual({ boards: 5, tasks: 10, running: 2 })
  })

  it('reports zero totals when nothing has answered yet', () => {
    expect(directoryTotals(reducer(undefined, { type: '@@init' }))).toEqual({ boards: 0, tasks: 0, running: 0 })
  })

  it('lets a later answer replace the earlier one for the same key', () => {
    let state = reducer(undefined, reportBoardTaskStats({ boardID: 'b1', tasks: 4, running: 0 }))
    state = reducer(state, reportBoardTaskStats({ boardID: 'b1', tasks: 7, running: 3 }))
    expect(boardStats(state, 'b1')).toEqual({ tasks: 7, running: 3 })
    expect(directoryTotals(state)).toEqual({ boards: 0, tasks: 7, running: 3 })
  })

  it('distinguishes "no answer yet" from "answered zero"', () => {
    const state = reducer(undefined, reportBoardTaskStats({ boardID: 'b1', tasks: 0, running: 0 }))
    expect(boardStats(state, 'b1')).toEqual({ tasks: 0, running: 0 })
    expect(boardStats(state, 'never-asked')).toBeUndefined()
  })
})

/**
 * US-AD91 AC2 makes the board table sortable by cost or task count. The slice
 * owns which column is active, so the rules that matter are: a click on the
 * active column flips direction, and a click on a new column starts at the
 * useful end (largest cost / most tasks first, but A→Z for a name).
 */
describe('setBoardSort — US-AD91 AC2', () => {
  it('defaults to cost descending, which is the state the design renders', () => {
    // The design source shows "Cost today (desc)" already active on load, so the
    // initial state is the assertion here — clicking cost would only flip it.
    expect(reducer(undefined, { type: '@@init' }).boardSort).toEqual({ key: 'cost', direction: 'desc' })
  })

  it('starts descending for cost when arriving from another column', () => {
    let state = reducer(undefined, setBoardSort('name'))
    state = reducer(state, setBoardSort('cost'))
    expect(state.boardSort).toEqual({ key: 'cost', direction: 'desc' })
  })

  it('starts descending for task count, so the busiest board leads', () => {
    const state = reducer(undefined, setBoardSort('tasks'))
    expect(state.boardSort).toEqual({ key: 'tasks', direction: 'desc' })
  })

  it('starts ascending for a name, because A comes before Z', () => {
    const state = reducer(undefined, setBoardSort('name'))
    expect(state.boardSort).toEqual({ key: 'name', direction: 'asc' })
  })

  it('flips direction when the active column is clicked again', () => {
    let state = reducer(undefined, setBoardSort('tasks'))
    state = reducer(state, setBoardSort('tasks'))
    expect(state.boardSort).toEqual({ key: 'tasks', direction: 'asc' })

    state = reducer(state, setBoardSort('tasks'))
    expect(state.boardSort).toEqual({ key: 'tasks', direction: 'desc' })
  })

  it('resets to the new column default instead of carrying the old direction', () => {
    let state = reducer(undefined, setBoardSort('tasks'))
    state = reducer(state, setBoardSort('tasks')) // now ascending
    state = reducer(state, setBoardSort('cost'))
    expect(state.boardSort).toEqual({ key: 'cost', direction: 'desc' })
  })
})
