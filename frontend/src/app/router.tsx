import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { DashboardLayout } from '@/routes/dashboard/Layout'
import { LandingPage } from '@/routes/public/LandingPage'

/**
 * The route table (ARCHITECTURE 18.2).
 *
 * The dashboard is one nested tree under `DashboardLayout`, which resolves the
 * session and renders the shell exactly once. The workspace id is an *optional*
 * segment: `/app` and `/app/:orgID` are the same routes, so there is no second
 * copy of the tree to keep in sync, and a bookmarked `/app` resolves the
 * operator's first membership instead of dead-ending.
 *
 * Everything except the landing page is lazy. N2 caps first paint at 400 ms and
 * the dashboard carries Redux, dnd-kit, and the board screens; shipping those to
 * a visitor who only wants the pricing page is what pushes a static SPA past the
 * budget. The landing page stays eager because it *is* the first paint.
 */
const FeaturesPage = lazy(() => import('@/routes/public/FeaturesPage').then((m) => ({ default: m.FeaturesPage })))
const PricingPage = lazy(() => import('@/routes/public/PricingPage').then((m) => ({ default: m.PricingPage })))
const DocsPage = lazy(() => import('@/routes/public/DocsPage').then((m) => ({ default: m.DocsPage })))
const GitHubPage = lazy(() => import('@/routes/public/GitHubPage').then((m) => ({ default: m.GitHubPage })))
const ChangelogPage = lazy(() => import('@/routes/public/ChangelogPage').then((m) => ({ default: m.ChangelogPage })))
const CommunityPage = lazy(() => import('@/routes/public/CommunityPage').then((m) => ({ default: m.CommunityPage })))
const SupportPage = lazy(() => import('@/routes/public/SupportPage').then((m) => ({ default: m.SupportPage })))
const NotFoundPage = lazy(() => import('@/routes/public/NotFoundPage').then((m) => ({ default: m.NotFoundPage })))
const Login = lazy(() => import('@/routes/auth/Login').then((m) => ({ default: m.Login })))
const Register = lazy(() => import('@/routes/auth/Register').then((m) => ({ default: m.Register })))
const ResetRequest = lazy(() => import('@/routes/auth/ResetRequest').then((m) => ({ default: m.ResetRequest })))
const ResetConfirm = lazy(() => import('@/routes/auth/ResetConfirm').then((m) => ({ default: m.ResetConfirm })))

const ProjectList = lazy(() =>
  import('@/routes/dashboard/projects/ProjectList').then((m) => ({ default: m.ProjectList })),
)
const ProjectDetail = lazy(() =>
  import('@/routes/dashboard/projects/ProjectDetail').then((m) => ({ default: m.ProjectDetail })),
)
const BoardList = lazy(() => import('@/routes/dashboard/boards/BoardList').then((m) => ({ default: m.BoardList })))
const KanbanBoard = lazy(() =>
  import('@/routes/dashboard/boards/KanbanBoard').then((m) => ({ default: m.KanbanBoard })),
)
const TableView = lazy(() => import('@/routes/dashboard/boards/TableView').then((m) => ({ default: m.TableView })))
const BoardSettings = lazy(() =>
  import('@/routes/dashboard/boards/BoardSettings').then((m) => ({ default: m.BoardSettings })),
)
const AgentRegistry = lazy(() =>
  import('@/routes/dashboard/agents/AgentRegistry').then((m) => ({ default: m.AgentRegistry })),
)
const AgentDetail = lazy(() =>
  import('@/routes/dashboard/agents/AgentDetail').then((m) => ({ default: m.AgentDetail })),
)
const ApprovalInbox = lazy(() =>
  import('@/routes/dashboard/approvals/ApprovalInbox').then((m) => ({ default: m.ApprovalInbox })),
)
const CostOverview = lazy(() =>
  import('@/routes/dashboard/finops/CostOverview').then((m) => ({ default: m.CostOverview })),
)
const Members = lazy(() => import('@/routes/dashboard/settings/Members').then((m) => ({ default: m.Members })))
const Profile = lazy(() => import('@/routes/dashboard/settings/Profile').then((m) => ({ default: m.Profile })))
const WorkspaceSettings = lazy(() =>
  import('@/routes/dashboard/settings/WorkspaceSettings').then((m) => ({ default: m.WorkspaceSettings })),
)
const ApiKeys = lazy(() => import('@/routes/dashboard/settings/ApiKeys').then((m) => ({ default: m.ApiKeys })))
const Webhooks = lazy(() => import('@/routes/dashboard/settings/Webhooks').then((m) => ({ default: m.Webhooks })))
const Providers = lazy(() => import('@/routes/dashboard/settings/Providers').then((m) => ({ default: m.Providers })))

/** The dashboard route set, mounted under both `/app` and `/app/:orgID`. */
function dashboardRoutes() {
  return (
    <>
      <Route index element={<Navigate to="projects" replace />} />
      <Route path="projects" element={<ProjectList />} />
      <Route path="projects/:projectID" element={<ProjectDetail />} />
      <Route path="boards" element={<BoardList />} />
      <Route path="boards/:boardID" element={<KanbanBoard />} />
      <Route path="boards/:boardID/table" element={<TableView />} />
      <Route path="boards/:boardID/settings" element={<BoardSettings />} />
      <Route path="agents" element={<AgentRegistry />} />
      <Route path="agents/:agentID" element={<AgentDetail />} />
      <Route path="approvals" element={<ApprovalInbox />} />
      <Route path="cost" element={<CostOverview />} />
      <Route path="settings" element={<Navigate to="workspace" replace />} />
      <Route path="settings/workspace" element={<WorkspaceSettings />} />
      {/* US-AD89: reachable from the rail's account menu, viewer-and-above. */}
      <Route path="settings/profile" element={<Profile />} />
      <Route path="settings/members" element={<Members />} />
      <Route path="settings/api-keys" element={<ApiKeys />} />
      <Route path="settings/webhooks" element={<Webhooks />} />
      {/* US-AD109: the credential registry. Settings-scoped, not a rail slot —
          the rail's six slots are pinned by the design's shell geometry. */}
      <Route path="settings/providers" element={<Providers />} />
    </>
  )
}

export function AppRoutes() {
  return (
    <Suspense fallback={null}>
      <Routes>
        <Route path="/" element={<LandingPage />} />
        <Route path="/features" element={<FeaturesPage />} />
        {/* /product is a legacy alias for the features page. */}
        <Route path="/product" element={<FeaturesPage />} />
        <Route path="/pricing" element={<PricingPage />} />
        <Route path="/github" element={<GitHubPage />} />
        <Route path="/community" element={<CommunityPage />} />
        <Route path="/changelog" element={<ChangelogPage />} />
        <Route path="/about" element={<SupportPage kind="about" />} />
        <Route path="/contact" element={<SupportPage kind="contact" />} />
        <Route path="/download" element={<SupportPage kind="download" />} />
        <Route path="/docs" element={<DocsPage />} />
        <Route path="/docs/*" element={<DocsPage />} />

        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        {/* US-AD88: the emailed link points at /reset/:token. */}
        <Route path="/reset" element={<ResetRequest />} />
        <Route path="/reset/:token" element={<ResetConfirm />} />

        {/* One tree, two mount points: /app and /app/:orgID. */}
        <Route path="/app" element={<DashboardLayout />}>
          {dashboardRoutes()}
        </Route>
        <Route path="/app/:orgID" element={<DashboardLayout />}>
          {dashboardRoutes()}
        </Route>

        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Suspense>
  )
}
