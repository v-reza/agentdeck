import { Component, type ErrorInfo, type ReactNode } from 'react'
import { translate } from '@/lib/i18n'
import { useAppSelector } from '@/store/hooks'
import { Button } from '@/components/ui/button'

/**
 * US-AD65 — the error boundary.
 *
 * React only surfaces render errors through a class component's
 * `getDerivedStateFromError`, so this is the one place in the app that is a class.
 * It catches a throwing child and renders a fallback instead of letting the whole
 * tree unmount to a white screen.
 *
 * What it deliberately does NOT do:
 *
 *  - It does not report anywhere. There is no client-error endpoint in the API,
 *    and inventing one would be a backend contract change.
 *  - It does not reset itself on navigation. `resetKey` is the caller's decision:
 *    the shell passes the current location, so a boundary that caught an error on
 *    one screen does not keep showing the fallback on the next one. Resetting on a
 *    timer or on every render would swallow a persistent error and turn a visible
 *    failure into a flaky one.
 *
 * The fallback carries its OWN strings rather than reusing `state.error` /
 * `action.retry`. Those two already exist and say "Ada yang gagal" / "Ulangi",
 * while US-AD65 AC1 names the copy exactly: "Terjadi kesalahan" + "Coba lagi".
 * Reusing the near-miss keys would have silently shipped wording the story does
 * not ask for; changing them would have altered unrelated screens.
 */

interface Props {
  children: ReactNode
  /** Changing this value clears the error and re-renders the children. */
  resetKey?: unknown
  /** Optional custom fallback; receives the error and a retry callback. */
  fallback?: (error: Error, retry: () => void) => ReactNode
}

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidUpdate(previous: Props) {
    if (this.state.error && previous.resetKey !== this.props.resetKey) {
      this.setState({ error: null })
    }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Kept as a console line on purpose: a boundary that silently swallows a
    // render error makes the next bug invisible in the browser console, which is
    // where anyone debugging this will look first.
    console.error('ErrorBoundary caught', error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    const retry = () => this.setState({ error: null })
    if (this.props.fallback) return this.props.fallback(error, retry)
    return <ErrorFallback error={error} onRetry={retry} />
  }
}

/**
 * The design's fallback: "Terjadi kesalahan" + "Coba lagi".
 *
 * The copy is read through `translate`, NOT through `useT`. `useT` unwraps the
 * dictionary with React 19's `use()`, which suspends on the first render — and a
 * fallback that suspends is a fallback that can fail for the same reason the
 * screen did: if the dictionary is what broke, the error surface deadlocks on the
 * Suspense boundary above it instead of reporting anything. It also cannot
 * resolve at all when it is the class boundary's own fallback, because the
 * suspended element never mounts for the boundary to catch. `translate` is the
 * synchronous lookup i18n already exposes for non-render code, and the language
 * still comes from the store, so a locale change re-renders this.
 */
export function ErrorFallback({ error, onRetry }: { error: Error; onRetry: () => void }) {
  const lang = useAppSelector((state) => state.lang.lang)
  const t = { title: translate(lang, 'error.boundary.title'), retry: translate(lang, 'error.boundary.retry') }
  return (
    <div
      data-testid="error-fallback"
      role="alert"
      className="flex min-h-[240px] flex-col items-center justify-center gap-3 p-8 text-center"
    >
      <p className="text-[14px] font-medium text-[var(--color-primary)]">{t.title}</p>
      {/* The message itself is shown, but never the stack: the operator gets the
          reason, not a trace they cannot act on. */}
      <p className="max-w-[420px] text-[12px] text-[var(--color-tertiary)]">{error.message}</p>
      <Button variant="secondary" onClick={onRetry} data-testid="error-retry">
        {t.retry}
      </Button>
    </div>
  )
}
