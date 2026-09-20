import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { Lang } from '@/lib/domain'

/**
 * Language selection (ARCHITECTURE 18.2 `store/slices/langSlice.ts`). EN is the
 * default and ID is the fallback-safe second locale: an unknown key resolves to
 * the EN string rather than rendering the key itself (see lib/i18n.ts).
 */
export interface LangState {
  lang: Lang
}

const initialState: LangState = { lang: 'id' }

const langSlice = createSlice({
  name: 'lang',
  initialState,
  reducers: {
    setLang(state, action: PayloadAction<Lang>) {
      state.lang = action.payload
    },
    toggleLang(state) {
      state.lang = state.lang === 'en' ? 'id' : 'en'
    },
  },
})

export const { setLang, toggleLang } = langSlice.actions
export default langSlice.reducer
