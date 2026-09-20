import type { AppListenerMiddleware, AppStartListening, RootState, AppDispatch } from '../index'
import { registerBudgetAlertListener } from './budgetAlert'
import { registerToastListener } from './toast'

/**
 * Listener registration (ARCHITECTURE 18.2 `store/listeners/`).
 *
 * Listeners exist so cross-cutting reactions — a toast on every rejected
 * mutation, the N18 budget warning — live in exactly one place instead of being
 * repeated in each component that happens to trigger a mutation.
 */
export function registerListeners(middleware: AppListenerMiddleware) {
  const startListening: AppStartListening = middleware.startListening.withTypes<RootState, AppDispatch>()
  registerToastListener(startListening)
  registerBudgetAlertListener(startListening)
}
