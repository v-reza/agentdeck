import { useDispatch, useSelector } from 'react-redux'
import type { RootState, AppDispatch } from './index'

/**
 * Typed hooks (ARCHITECTURE 18.2 `store/hooks.ts`). Components import these two
 * instead of the raw react-redux hooks so state and dispatch are typed without
 * a cast at every call site.
 */
export const useAppDispatch = useDispatch.withTypes<AppDispatch>()
export const useAppSelector = useSelector.withTypes<RootState>()
