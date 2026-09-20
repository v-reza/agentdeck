import { useEffect } from 'react'
import { Navigate, Outlet, useLocation, useNavigate, useParams } from 'react-router-dom'
import { AppShell } from '@/components/layout/AppShell'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { useMeQuery } from '@/store/api/session'
import { setActiveOrg, setSession } from '@/store/slices/sessionSlice'

/**
 * ARCHITECTURE 18.2: `routes/dashboard/Layout.tsx` — "sidebar, org picker,
 * breadcrumb". This is the one place that resolves the session and the active
 * tenant; every dashboard route renders inside it and inherits the shell.
 *
 * The layout is deliberately thin: it owns authentication, the workspace
 * selection, and the frame. It renders no page content, so adding a screen means
 * adding a route, never editing the layout.
 */
export function DashboardLayout() {
  const { orgID } = useParams<{ orgID: string }>()
  const location = useLocation()
  const navigate = useNavigate()
  const dispatch = useAppDispatch()
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const { data, isLoading, error } = useMeQuery()

  // Seed the session slice from the server's answer. `activeOrgID` follows the
  // URL so a deep link into a workspace lands on that workspace, and a bare
  // /app lands on the first membership.
  useEffect(() => {
    if (!data) return
    dispatch(setSession(data))
    const fromURL = orgID && data.workspaces.some((w) => w.id === orgID) ? orgID : undefined
    if (fromURL && fromURL !== activeOrgID) dispatch(setActiveOrg(fromURL))
  }, [data, dispatch, orgID, activeOrgID])

  // Canonicalise the URL so a workspace has exactly one address.
  //
  // ARCHITECTURE 11.1 resolves the tenant from the session and X-Org-ID, never
  // from the path — so the org id in the URL is a *label*, and the page must
  // still work without it (a bare /app after login). But leaving both /app/x and
  // /app/<org>/x live means the same screen has two addresses, and a shared link
  // then depends on which one the sender happened to be on. Rewriting to the
  // workspace-scoped form keeps deep links meaningful without making the path
  // load-bearing for authorisation.
  useEffect(() => {
    if (!data || !activeOrgID) return
    const isMember = orgID ? data.workspaces.some((w) => w.id === orgID) : false
    if (isMember) return
    const rest = location.pathname.replace(/^\/app(\/[^/]+)?/, '')
    navigate(`/app/${activeOrgID}${rest || '/projects'}`, { replace: true })
  }, [data, orgID, activeOrgID, location.pathname, navigate])

  if (isLoading) return <CenteredNote text="Loading workspace…" />
  if (error) return <Navigate to="/login" replace />

  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}

function CenteredNote({ text }: { text: string }) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-[var(--color-surface-page)]">
      <p className="font-mono text-[12px] text-[var(--color-tertiary)]">{text}</p>
    </main>
  )
}
