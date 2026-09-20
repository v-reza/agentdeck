import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { AvatarUser, Role } from '@/lib/domain'

/**
 * The authenticated identity and the active tenant. This is *client* state in
 * the ARCHITECTURE 18.2 sense: the server state (boards, tasks, runs) lives in
 * RTK Query cache, and this slice only holds what the client itself decided —
 * which workspace the operator is looking at.
 *
 * `activeOrgID` is mirrored into the X-Org-ID header by store/api/base.ts, so
 * switching workspace is a single dispatch and every cached query refetches
 * against the new tenant.
 */
export interface WorkspaceSummary {
  id: string
  name: string
  slug: string
  role: Role
  kind: string
}

export interface SessionState {
  userID: string | null
  email: string | null
  name: string | null
  /**
   * The `avatar_user` object GET /auth/me returned (US-AD89 AC1). The rail and
   * the top bar draw this value rather than deriving their own monogram, so an
   * uploaded image and the initials fallback are decided in exactly one place.
   */
  avatar: AvatarUser | null
  workspaces: WorkspaceSummary[]
  activeOrgID: string | null
  /** False until /auth/me has answered once; the router waits on this. */
  resolved: boolean
}

const initialState: SessionState = {
  userID: null,
  email: null,
  name: null,
  avatar: null,
  workspaces: [],
  activeOrgID: null,
  resolved: false,
}

const sessionSlice = createSlice({
  name: 'session',
  initialState,
  reducers: {
    setSession(
      state,
      action: PayloadAction<{
        id: string
        email: string
        name: string
        avatar_user?: AvatarUser
        workspaces: WorkspaceSummary[]
      }>,
    ) {
      state.userID = action.payload.id
      state.email = action.payload.email
      state.name = action.payload.name
      state.avatar = action.payload.avatar_user ?? state.avatar
      state.workspaces = action.payload.workspaces
      state.resolved = true
      // Keep the operator on the workspace they are already in when it is
      // still a valid membership; otherwise fall back to the first one.
      const stillMember = action.payload.workspaces.some((w) => w.id === state.activeOrgID)
      if (!stillMember) {
        state.activeOrgID = action.payload.workspaces[0]?.id ?? null
      }
    },
    setActiveOrg(state, action: PayloadAction<string>) {
      state.activeOrgID = action.payload
    },
    clearSession(state) {
      state.userID = null
      state.email = null
      state.name = null
      state.avatar = null
      state.workspaces = []
      state.activeOrgID = null
      state.resolved = true
    },
  },
})

export const { setSession, setActiveOrg, clearSession } = sessionSlice.actions
export default sessionSlice.reducer

/** Role of the operator in the active workspace, or null when unresolved. */
export function activeRole(state: SessionState): Role | null {
  return state.workspaces.find((w) => w.id === state.activeOrgID)?.role ?? null
}
