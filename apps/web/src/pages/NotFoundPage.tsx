// Fallback for unpublished routes.
import { Link } from '../lib/router'
import { PublicShell } from '../components/PublicShell'

function NotFoundPage() {
  return (
    <PublicShell>
      <div className="public-page centered-page">
        <div className="page-kicker">404</div>
        <h1>That page is not published yet.</h1>
        <p className="page-lead">The public docs are intentionally explicit instead of showing a blank screen.</p>
        <Link href="/docs/quickstart" className="btn-primary">
          Go to Quickstart
        </Link>
      </div>
    </PublicShell>
  )
}

export { NotFoundPage }
