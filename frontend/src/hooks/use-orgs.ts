import { useListOrgsQuery, useListMembersQuery } from '@/store/api/session'
import { useAppSelector } from '@/store/hooks'
import type { Role } from '@/lib/domain'

/**
 * The workspaces the operator belongs to, plus the roster of the active one.
 * Both are server state (RTK Query), so switching workspace only changes
 * `session.activeOrgID` and every scoped query refetches on the new header.
 */
export function useOrgs() {
  const { data, isLoading, error } = useListOrgsQuery()
  return { orgs: data ?? [], isLoading, error }
}

export function useMembers() {
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const { data, isLoading, isError } = useListMembersQuery(activeOrgID ?? '', { skip: !activeOrgID })
  return { members: data ?? [], isLoading, isError }
}

/**
 * Whether the operator may perform an action in the active workspace.
 * The backend enforces the same minimum with `requireRole`; this only hides
 * controls that would 403, and never grants anything on its own.
 */
export function useCanAct(minimum: Role): boolean {
  const role = useAppSelector((state) => state.session.workspaces.find((w) => w.id === state.session.activeOrgID)?.role)
  if (!role) return false
  const order: Role[] = ['viewer', 'member', 'admin', 'owner']
  return order.indexOf(role) >= order.indexOf(minimum)
}
