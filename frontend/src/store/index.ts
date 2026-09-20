import { combineReducers, configureStore } from '@reduxjs/toolkit'
import { createListenerMiddleware, type TypedStartListening } from '@reduxjs/toolkit'
import { setupListeners } from '@reduxjs/toolkit/query'

import { baseApi } from './api/base'
import { releasesApi } from './api/releases'
// Side-effect imports: each module injects its endpoints into baseApi. Order is
// irrelevant but the imports are load-bearing — removing one silently drops its
// endpoints from the API.
import './api/session'
import './api/boards'
import './api/agents'
import './api/finops'
import './api/stream'

import sessionReducer from './slices/sessionSlice'
import uiReducer from './slices/uiSlice'
import langReducer from './slices/langSlice'
import toastReducer from './slices/toastSlice'
import directoryReducer from './slices/directorySlice'
import { registerListeners } from './listeners'

/**
 * The single Redux store (ARCHITECTURE 18.2 `store/index.ts`).
 *
 * Redux Toolkit is the only state manager in this app. Server state lives in the
 * RTK Query cache; client state lives in the slices; side effects live in
 * listener middleware. There is no Context for application data and no second
 * data-fetching library.
 */
const rootReducer = combineReducers({
  [baseApi.reducerPath]: baseApi.reducer,
  [releasesApi.reducerPath]: releasesApi.reducer,
  session: sessionReducer,
  ui: uiReducer,
  lang: langReducer,
  toast: toastReducer,
  directory: directoryReducer,
})

export type RootState = ReturnType<typeof rootReducer>
export type AppDispatch = typeof store.dispatch

export const listenerMiddleware = createListenerMiddleware()
export type AppListenerMiddleware = typeof listenerMiddleware

/**
 * The start-listening function every listener module receives, with the store's
 * state and dispatch already bound. Declaring it once means the listener modules
 * never cast and never widen their own action or state types.
 */
export type AppStartListening = TypedStartListening<RootState, AppDispatch>

export const store = configureStore({
  reducer: rootReducer,
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware().prepend(listenerMiddleware.middleware).concat(baseApi.middleware, releasesApi.middleware),
})

registerListeners(listenerMiddleware)

// Enables refetchOnFocus / refetchOnReconnect for RTK Query.
setupListeners(store.dispatch)
