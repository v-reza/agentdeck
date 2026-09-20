import { baseApi } from './base'
import type { LedgerEntry, BoardBudget, CostSummary } from '@/lib/domain'

/**
 * Finops server state (ARCHITECTURE 18.2: `store/api/finops.ts`, tags Ledger,
 * CostSummary).
 *
 * Every money field is an integer count of micro-USD (DECISIONS 6: 1 USD =
 * 1_000_000, never a float). The API never returns a pre-formatted string, so
 * rounding happens once, in lib/formatters.ts.
 *
 * These endpoints are defined by ARCHITECTURE 6.2 but land in M2. Until the
 * backend serves them the queries answer 404, and every consumer renders the
 * empty state — never a stand-in number.
 */

export const finopsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    boardBudget: build.query<BoardBudget, string>({
      query: (boardID) => `boards/${boardID}/budget`,
      providesTags: (_r, _e, boardID) => [{ type: 'CostSummary', id: `BUDGET-${boardID}` }],
    }),

    updateBoardBudget: build.mutation<BoardBudget, { boardID: string; budgetDailyMicros: number }>({
      query: ({ boardID, ...body }) => ({
        url: `boards/${boardID}/budget`,
        method: 'PATCH',
        body,
      }),
      invalidatesTags: (_r, _e, { boardID }) => [
        { type: 'CostSummary', id: `BUDGET-${boardID}` },
        { type: 'Board', id: boardID },
      ],
    }),

    boardLedger: build.query<LedgerEntry[], string>({
      query: (boardID) => `boards/${boardID}/ledger`,
      providesTags: (_r, _e, boardID) => [{ type: 'Ledger', id: `BOARD-${boardID}` }],
    }),

    runLedger: build.query<LedgerEntry[], string>({
      query: (runID) => `runs/${runID}/ledger`,
      providesTags: (_r, _e, runID) => [{ type: 'Ledger', id: `RUN-${runID}` }],
    }),

    orgCostSummary: build.query<CostSummary, string>({
      query: (orgID) => `orgs/${orgID}/cost-summary`,
      providesTags: [{ type: 'CostSummary', id: 'SUMMARY' }],
    }),
  }),
})

export const {
  useBoardBudgetQuery,
  useUpdateBoardBudgetMutation,
  useBoardLedgerQuery,
  useRunLedgerQuery,
  useOrgCostSummaryQuery,
} = finopsApi
