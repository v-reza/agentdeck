import type { AppStartListening } from '../index'
import { pushToast } from '../slices/toastSlice'
import { finopsApi } from '../api/finops'

/**
 * N18: warn once when a board crosses 80% of its daily cap.
 *
 * The server decides the threshold (`BoardBudget.threshold_crossed`); this
 * listener only reports it. Recomputing 80% on the client would drift from the
 * contract the moment the threshold changes, and would double-fire for every
 * subscriber of the same board — so the toast is keyed by board id.
 */
export function registerBudgetAlertListener(startListening: AppStartListening) {
  const announced = new Set<string>()

  startListening({
    matcher: finopsApi.endpoints.boardBudget.matchFulfilled,
    effect: (action, api) => {
      const budget = action.payload
      if (!budget.threshold_crossed) return
      if (announced.has(budget.board_id)) return
      announced.add(budget.board_id)

      api.dispatch(
        pushToast({
          tone: 'warning',
          title: 'Budget threshold reached',
          body: `This board has used 80% of its daily cap (N18).`,
        }),
      )
    },
  })
}
