import { describe, expect, it } from 'vitest'
import reducer, { setDirectorySort, setDirectorySearch, toggleDirectoryCollapsed } from './uiSlice'

/**
 * US-AD91 AC2: the directory is sortable by task count and by cost. These pin the
 * toggle behaviour, because "click the same header twice" is the part a user
 * notices when it is wrong and a unit test is the cheapest place to catch it.
 */
describe('directory sort', () => {
  it('defaults to ascending by name', () => {
    const state = reducer(undefined, { type: '@@init' })
    expect(state.directorySort).toEqual({ key: 'name', direction: 'asc' })
  })

  it('switches column and starts descending for numeric columns', () => {
    // "Sort by cost" is almost always asked as "what costs most".
    const state = reducer(undefined, setDirectorySort('cost'))
    expect(state.directorySort).toEqual({ key: 'cost', direction: 'desc' })
  })

  it('flips direction when the active column is clicked again', () => {
    let state = reducer(undefined, setDirectorySort('tasks'))
    expect(state.directorySort).toEqual({ key: 'tasks', direction: 'desc' })

    state = reducer(state, setDirectorySort('tasks'))
    expect(state.directorySort).toEqual({ key: 'tasks', direction: 'asc' })
  })

  it('returns to the name column ascending after leaving it', () => {
    // 'name' is the default key, so switching *back* to it from another column
    // must reset the direction to ascending rather than inherit the previous
    // column's direction. (Clicking it while already active is the flip case
    // covered above.)
    let state = reducer(undefined, setDirectorySort('tasks'))
    state = reducer(state, setDirectorySort('name'))
    expect(state.directorySort).toEqual({ key: 'name', direction: 'asc' })
  })
})

describe('directory search', () => {
  it('holds the filter text', () => {
    const state = reducer(undefined, setDirectorySearch('cinder'))
    expect(state.directorySearch).toBe('cinder')
  })
})

describe('collapsed groups', () => {
  it('toggles a project without touching the others', () => {
    let state = reducer(undefined, toggleDirectoryCollapsed('p1'))
    state = reducer(state, toggleDirectoryCollapsed('p2'))
    expect(state.directoryCollapsed).toEqual(['p1', 'p2'])

    state = reducer(state, toggleDirectoryCollapsed('p1'))
    expect(state.directoryCollapsed).toEqual(['p2'])
  })
})
