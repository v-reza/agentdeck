import { describe, expect, it } from 'vitest'
import reducer, { activeRole, clearSession, setActiveOrg, setSession } from './sessionSlice'

const WORKSPACES = [
  { id: 'org_1', name: 'Personal', slug: 'personal', role: 'owner' as const, kind: 'personal' },
  { id: 'org_2', name: 'Acme', slug: 'acme', role: 'viewer' as const, kind: 'manual' },
]

const initial = reducer(undefined, { type: '@@init' })

describe('sessionSlice', () => {
  it('starts unresolved so the router can wait instead of flashing /login', () => {
    expect(initial.resolved).toBe(false)
    expect(initial.activeOrgID).toBeNull()
  })

  it('selects the first workspace when the operator is not in one yet', () => {
    const state = reducer(initial, setSession({ id: 'u1', email: 'a@b.c', name: 'A', workspaces: WORKSPACES }))
    expect(state.activeOrgID).toBe('org_1')
    expect(state.resolved).toBe(true)
  })

  it('keeps the active workspace across a re-auth when membership survives', () => {
    const first = reducer(initial, setSession({ id: 'u1', email: 'a@b.c', name: 'A', workspaces: WORKSPACES }))
    const moved = reducer(first, setActiveOrg('org_2'))
    const again = reducer(moved, setSession({ id: 'u1', email: 'a@b.c', name: 'A', workspaces: WORKSPACES }))
    expect(again.activeOrgID).toBe('org_2')
  })

  it('falls back when the previously active workspace was revoked', () => {
    const first = reducer(initial, setSession({ id: 'u1', email: 'a@b.c', name: 'A', workspaces: WORKSPACES }))
    const moved = reducer(first, setActiveOrg('org_2'))
    const shrunk = reducer(moved, setSession({ id: 'u1', email: 'a@b.c', name: 'A', workspaces: [WORKSPACES[0]] }))
    // Staying on a workspace the user is no longer a member of would send every
    // subsequent request into a 403.
    expect(shrunk.activeOrgID).toBe('org_1')
  })

  it('reports the role of the active workspace only', () => {
    const state = reducer(initial, setSession({ id: 'u1', email: 'a@b.c', name: 'A', workspaces: WORKSPACES }))
    expect(activeRole(state)).toBe('owner')
    expect(activeRole(reducer(state, setActiveOrg('org_2')))).toBe('viewer')
  })

  it('clears everything on logout but stays resolved', () => {
    const state = reducer(initial, setSession({ id: 'u1', email: 'a@b.c', name: 'A', workspaces: WORKSPACES }))
    const out = reducer(state, clearSession())
    expect(out.userID).toBeNull()
    expect(out.workspaces).toEqual([])
    expect(out.resolved).toBe(true)
  })
})
