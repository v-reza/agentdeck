import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { BrowserRouter } from 'react-router-dom'
import { store } from './store'
import App from './App'
import './index.css'
// Public marketing/docs surfaces: the component layer restored from the
// pre-refactor stylesheet. The dashboard is Tailwind-only; this file exists
// because ~30 public components address these class names.
import './public.css'

/**
 * Entry point (ARCHITECTURE 18.2 `src/main.tsx` — "createRoot + <Provider
 * store>"). The store is a module singleton so RTK Query's cache is shared by
 * every route; there is no second store and no context provider for app data.
 */
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Provider store={store}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </Provider>
  </StrictMode>,
)
