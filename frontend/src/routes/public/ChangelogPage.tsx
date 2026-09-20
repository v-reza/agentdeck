// Full release history plus the roadmap pointer.
import { Link } from 'react-router-dom'
import { PublicShell } from '../../components/PublicShell'
import { ReleaseEntry } from '../../components/ReleaseEntry'
import { useGitHubReleases } from '../../lib/useGitHubReleases'

function ChangelogPage() {
  const { status, releases } = useGitHubReleases(20)
  const published = releases.filter((item) => !item.draft)
  const state = status === 'ready' && !published.length ? 'empty' : status

  return (
    <PublicShell>
      <div className="public-page narrow-page">
        <div className="docs-breadcrumb">
          Documentation <span>/</span> <b>Changelog</b>
        </div>
        <h1>Changelog</h1>
        <p className="page-lead">
          Release notes for the AgentDeck single binary, pulled from the public GitHub Releases feed.
        </p>
        {state === 'loading' && (
          <div className="changelog-state">
            <span className="release-loading-dot" />
            Loading release history…
          </div>
        )}
        {state === 'ready' && (
          <div className="changelog-list">
            {releases.map((release) => (
              <ReleaseEntry release={release} key={release.tag_name} />
            ))}
          </div>
        )}
        {state === 'empty' && (
          <div className="changelog-state">
            <h2>No public releases yet.</h2>
            <p>Release notes will appear here after the first GitHub Release is published.</p>
            <a
              href="https://github.com/v-reza/agentdeck/releases"
              target="_blank"
              rel="noreferrer"
              className="text-link"
            >
              Open GitHub Releases ↗
            </a>
          </div>
        )}
        {state === 'error' && (
          <div className="changelog-state">
            <h2>Release history unavailable.</h2>
            <p>GitHub API could not be reached right now. Open the repository for the canonical release history.</p>
            <a
              href="https://github.com/v-reza/agentdeck/releases"
              target="_blank"
              rel="noreferrer"
              className="text-link"
            >
              Open GitHub Releases ↗
            </a>
          </div>
        )}
        <div className="changelog-roadmap">
          <div className="page-kicker">WHAT'S NEXT</div>
          <h2>Build in public, one operational loop at a time.</h2>
          <p>
            Follow the roadmap from identity and workspace foundation through board operations, cost controls, approval
            gates, reliability, and governance.
          </p>
          <Link to="/github" className="text-link">
            See the M0–M6 roadmap →
          </Link>
        </div>
      </div>
    </PublicShell>
  )
}

export { ChangelogPage }
