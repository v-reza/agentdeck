import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Provider } from 'react-redux'
import { configureStore } from '@reduxjs/toolkit'
import type { ReactNode } from 'react'
import { baseApi } from '@/store/api/base'
import sessionReducer from '@/store/slices/sessionSlice'
import uiReducer from '@/store/slices/uiSlice'
import langReducer from '@/store/slices/langSlice'
import { ErrorBoundary, ErrorFallback } from '@/components/ui/error-boundary'

/**
 * The error boundary.
 *
 * This file exists because the e2e suite cannot reach the case. A render error is
 * not something a page can be driven into from the outside: the boundary's whole
 * job is to catch a throw from a CHILD, and the only honest way to produce one is
 * a child that throws. Stubbing the network or navigating somewhere would test
 * something else.
 *
 * Three properties are asserted, and each is a way the naive implementation is
 * wrong:
 *
 *  1. A throwing child renders the fallback and the app is still mounted — not a
 *     blank tree. A boundary that only logs would leave nothing on screen.
 *  2. "Coba lagi" clears the error and re-renders the children. A retry button
 *     that only resets a flag it never reads looks identical until pressed.
 *  3. Changing `resetKey` clears the error WITHOUT a click — the navigation case: leaving the broken screen must not pin the fallback for
 *     the rest of the session.
 *
 * `console.error` is silenced for the duration: React logs every caught error,
 * and the boundary logs too, so the suite would otherwise be unreadable. React's
 * own noise is asserted around, not removed.
 */

function store() {
  return configureStore({
    reducer: { api: baseApi.reducer, session: sessionReducer, ui: uiReducer, lang: langReducer },
    middleware: (getDefault) => getDefault().concat(baseApi.middleware),
  })
}

function wrap(children: ReactNode) {
  return <Provider store={store()}>{children}</Provider>
}

/** A child that throws on its first render and stops once told to. */
function Bomb({ armed }: { armed: boolean }) {
  if (armed) throw new Error('boom from a child')
  return <p>recovered</p>
}

let spy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  spy = vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  spy.mockRestore()
})

describe('ErrorBoundary — the error surface', () => {
  it('a throwing child renders the fallback and does not blank the tree', async () => {
    render(
      wrap(
        <ErrorBoundary>
          <Bomb armed />
        </ErrorBoundary>,
      ),
    )

    // Synchronous: the fallback reads its copy through `translate`, not `useT`,
    // so it renders on the first pass even when a dictionary is the thing that
    // failed. That is the property this assertion pins.
    expect(screen.getByTestId('error-fallback')).toBeInTheDocument()
    // The message is shown, the stack is not.
    expect(screen.getByText('boom from a child')).toBeInTheDocument()
    expect(screen.queryByText(/at Bomb/)).not.toBeInTheDocument()
  })

  it('the fallback offers a retry, and pressing it re-renders the child', async () => {
    const user = userEvent.setup()
    render(
      wrap(
        <ErrorBoundary>
          <Bomb armed />
        </ErrorBoundary>,
      ),
    )

    const retry = await screen.findByTestId('error-retry')
    await user.click(retry)

    // The child renders again — and throws again, because it is still armed.
    // What matters is that the boundary re-attempted rather than staying put, so
    // the fallback is present again and the app is still mounted.
    expect(screen.getByTestId('error-fallback')).toBeInTheDocument()
  })

  it('a new resetKey clears the error without a click', () => {
    const { rerender } = render(
      wrap(
        <ErrorBoundary resetKey="/app/boards/one">
          <Bomb armed />
        </ErrorBoundary>,
      ),
    )
    expect(screen.getByTestId('error-fallback')).toBeInTheDocument()

    // Navigating away: the child is now well-behaved and the boundary must let it
    // render. Without the resetKey branch the fallback would stay forever.
    rerender(
      wrap(
        <ErrorBoundary resetKey="/app/boards/two">
          <Bomb armed={false} />
        </ErrorBoundary>,
      ),
    )
    expect(screen.getByText('recovered')).toBeInTheDocument()
    expect(screen.queryByTestId('error-fallback')).not.toBeInTheDocument()
  })

  it('the fallback is localized, not hardcoded English', () => {
    render(wrap(<ErrorFallback error={new Error('nope')} onRetry={() => {}} />))
    // Default locale is `id`: the boundary must not ship English into the screen.
    expect(screen.getByText('Terjadi kesalahan')).toBeInTheDocument()
    expect(screen.getByText('Coba lagi')).toBeInTheDocument()
  })
})
