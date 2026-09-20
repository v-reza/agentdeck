import { createSlice, type PayloadAction } from '@reduxjs/toolkit'

/**
 * Transient toast queue. Toasts are client state produced by listener effects
 * (ARCHITECTURE 18.2 `store/listeners/toast.ts`), never by components directly,
 * so every mutation error surfaces through one code path.
 *
 * Tones are the DESIGN.md semantic names (`success`/`warning`/`danger`/`info`),
 * not ad-hoc strings, so a toast colour is always a token that exists.
 */
export type ToastTone = 'success' | 'warning' | 'danger' | 'info'

export interface Toast {
  id: string
  tone: ToastTone
  title: string
  body?: string
}

export interface ToastState {
  items: Toast[]
}

const initialState: ToastState = { items: [] }

const toastSlice = createSlice({
  name: 'toast',
  initialState,
  reducers: {
    pushToast(state, action: PayloadAction<Omit<Toast, 'id'> & { id?: string }>) {
      const { id, ...rest } = action.payload
      state.items.push({ id: id ?? crypto.randomUUID(), ...rest })
    },
    dismissToast(state, action: PayloadAction<string>) {
      state.items = state.items.filter((toast) => toast.id !== action.payload)
    },
  },
})

export const { pushToast, dismissToast } = toastSlice.actions
export default toastSlice.reducer
