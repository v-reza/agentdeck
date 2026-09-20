import { AppRoutes } from './app/router'
import { ToastHost } from './components/ui/toast'

/**
 * Root component (ARCHITECTURE 18.2 `src/App.tsx` — "routing, layout, lang
 * provider"). The store lives in main.tsx so this stays a pure composition of
 * routing and app-wide overlays.
 */
export default function App() {
  return (
    <>
      <AppRoutes />
      <ToastHost />
    </>
  )
}
