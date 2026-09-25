import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Provider } from 'react-redux'
import { configureStore } from '@reduxjs/toolkit'
import type { ReactNode } from 'react'
import { baseApi } from '@/store/api/base'
import sessionReducer, { setActiveOrg, setSession } from '@/store/slices/sessionSlice'
import { useCanAct } from '@/hooks/use-orgs'
import { DrawerFooter } from '@/routes/dashboard/boards/TaskDetailDrawer'

/**
 * US-AD59 AC4 — the drawer hides the archive and delete controls from a member.
 *
 * This file exists because the e2e suite cannot prove it. `dashboard.spec.ts`
 * registers a fresh account, which is the workspace *owner*, so the role gate is
 * always on the permissive side there: mutating `useCanAct('admin')` to `true`
 * left every Playwright assertion green. The gate is only observable from a
 * member's session, and a unit test can build one in a line.
 *
 * What is asserted here is the hook the drawer reads its permission from, plus
 * the footer as the drawer actually applies it. The hook cases pin the role
 * comparison; the footer cases pin the consequence of it (a mutation that
 * replaced `useCanAct('admin')` with `true` inside `DrawerFooter` left every e2e
 * assertion green and the hook cases untouched). The footer is rendered with
 * mutations mounted but never dispatched — the buttons are only inspected, not
 * pressed, so nothing here reaches the network.
 */

/**
 * A store whose active workspace carries `role`.
 *
 * `baseApi` is mounted without its middleware: the footer's mutations are never
 * dispatched in these tests (the buttons are only rendered, not pressed), and a
 * store with no API middleware cannot accidentally reach the network.
 */
function storeWithRole(role: 'owner' | 'admin' | 'member' | 'viewer') {
  const store = configureStore({
    reducer: { session: sessionReducer, api: baseApi.reducer },
  })
  // `kind` is part of the workspace summary (personal vs organisation); any
  // value will do here — nothing under test reads it.
  store.dispatch(
    setSession({
      id: 'user-1',
      email: 'member@x.test',
      name: 'Member',
      workspaces: [{ id: 'org-1', name: 'Acme', slug: 'acme', role, kind: 'organisation' }],
    }),
  )
  store.dispatch(setActiveOrg('org-1'))
  return store
}

function Gate({ minimum, children }: { minimum: 'admin' | 'member'; children: (allowed: boolean) => ReactNode }) {
  return <>{children(useCanAct(minimum))}</>
}

describe('useCanAct — the gate the task drawer reads', () => {
  const cases: { role: 'owner' | 'admin' | 'member' | 'viewer'; canAdminister: boolean }[] = [
    { role: 'owner', canAdminister: true },
    { role: 'admin', canAdminister: true },
    { role: 'member', canAdminister: false },
    { role: 'viewer', canAdminister: false },
  ]

  for (const { role, canAdminister } of cases) {
    it(`says ${canAdminister} for a ${role} at the admin floor`, () => {
      render(
        <Provider store={storeWithRole(role)}>
          <Gate minimum="admin">
            {(allowed) => <span data-testid="verdict">{allowed ? 'allowed' : 'denied'}</span>}
          </Gate>
        </Provider>,
      )
      expect(screen.getByTestId('verdict').textContent).toBe(canAdminister ? 'allowed' : 'denied')
    })
  }

  it('keeps ordinary column moves available to a member', () => {
    // The board's drag-and-drop runs at Member level, so raising the archive
    // floor must not have raised this one (US-AD59 AC4 is about archiving only).
    render(
      <Provider store={storeWithRole('member')}>
        <Gate minimum="member">{(allowed) => <span data-testid="verdict">{allowed ? 'allowed' : 'denied'}</span>}</Gate>
      </Provider>,
    )
    expect(screen.getByTestId('verdict').textContent).toBe('allowed')
  })

  it('denies a viewer at the member floor', () => {
    render(
      <Provider store={storeWithRole('viewer')}>
        <Gate minimum="member">{(allowed) => <span data-testid="verdict">{allowed ? 'allowed' : 'denied'}</span>}</Gate>
      </Provider>,
    )
    expect(screen.getByTestId('verdict').textContent).toBe('denied')
  })
})

/**
 * The gate as the drawer actually applies it.
 *
 * The hook-level cases above pin the comparison; these pin the consequence. A
 * mutation that replaced `useCanAct('admin')` with `true` inside `DrawerFooter`
 * left every e2e assertion green — because the e2e session is always the owner —
 * and passed the hook cases too, since the hook was untouched. Only rendering
 * the footer from a member's session catches it.
 */
describe('Task drawer footer — what a member sees', () => {
  it('offers no archive or delete control to a member', () => {
    render(
      <Provider store={storeWithRole('member')}>
        <DrawerFooter taskID="task-1" status="done" />
      </Provider>,
    )
    expect(screen.queryByRole('button', { name: /Arsipkan Task/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Hapus/ })).toBeNull()
    expect(screen.getByText(/Arsip butuh owner atau admin/)).toBeTruthy()
  })

  it('offers no archive or delete control to a viewer', () => {
    render(
      <Provider store={storeWithRole('viewer')}>
        <DrawerFooter taskID="task-1" status="done" />
      </Provider>,
    )
    expect(screen.queryByRole('button', { name: /Arsipkan Task/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Hapus/ })).toBeNull()
  })

  it('offers both controls to an admin', () => {
    render(
      <Provider store={storeWithRole('admin')}>
        <DrawerFooter taskID="task-1" status="done" />
      </Provider>,
    )
    expect(screen.getByRole('button', { name: /Arsipkan Task/ })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Hapus/ })).toBeTruthy()
  })

  it('disables archive for a task that is already archived', () => {
    render(
      <Provider store={storeWithRole('owner')}>
        <DrawerFooter taskID="task-1" status="archived" />
      </Provider>,
    )
    expect(screen.getByRole('button', { name: /Arsipkan Task/ }).hasAttribute('disabled')).toBe(true)
  })
})
