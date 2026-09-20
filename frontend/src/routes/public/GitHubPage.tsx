// Repository and release metadata, fetched live from the GitHub API.
import { PublicShell } from '../../components/PublicShell'
import { Roadmap } from '../../components/landing/Roadmap'
import { useGitHubReleases } from '../../lib/useGitHubReleases'

function GitHubPage() {
  const { status, releases } = useGitHubReleases(5)
  const release = releases.find((item) => !item.draft && !item.prerelease) ?? null
  const releaseState = status === 'ready' && !release ? 'empty' : status

  const releaseDate = release?.published_at
    ? new Intl.DateTimeFormat('en-US', { year: 'numeric', month: 'long', day: 'numeric' }).format(
        new Date(release.published_at),
      )
    : null
  const releaseNotes =
    release?.body
      ?.split('\n')
      .map((line) => line.replace(/^[-*]\s*/, '').trim())
      .filter(Boolean)
      .slice(0, 4) ?? []

  return (
    <PublicShell active="github">
      <div className="public-page">
        <div className="docs-breadcrumb">
          AgentDeck <span>/</span> <b>Repository & releases</b>
        </div>
        <h1>AgentDeck on GitHub</h1>
        <p className="page-lead">Open source Go orchestration, cost ledger, and approval gates for AI agent fleets.</p>
        <div className="resource-grid">
          <div className="resource-card">
            <div className="resource-icon">GH</div>
            <h2>v-reza/agentdeck</h2>
            <p>Single-binary Go orchestrator and telemetry ledger for autonomous agent fleets.</p>
            <a className="btn-primary full" href="https://github.com/v-reza/agentdeck" target="_blank" rel="noreferrer">
              Open repository ↗
            </a>
            <dl>
              <dt>License</dt>
              <dd>Apache-2.0</dd>
              <dt>Version</dt>
              <dd>{release?.tag_name ?? 'v0.1 preview'}</dd>
              <dt>Release</dt>
              <dd>{releaseDate ?? 'Not published yet'}</dd>
            </dl>
          </div>
          <div>
            <h2 className="subheading">Latest release</h2>
            {releaseState === 'loading' && (
              <div className="release-card release-loading">
                <span className="release-loading-dot" />
                Loading release metadata…
              </div>
            )}
            {releaseState === 'ready' && release && (
              <div className="release-card">
                <div className="release-top">
                  <b>{release.tag_name}</b>
                  <span>{release.prerelease ? 'Pre-release' : 'Latest stable'}</span>
                  <time>{releaseDate}</time>
                </div>
                <ul>
                  {(releaseNotes.length ? releaseNotes : ['Published release metadata is available on GitHub.']).map(
                    (note) => (
                      <li key={note}>{note}</li>
                    ),
                  )}
                </ul>
                <a href={release.html_url} target="_blank" rel="noreferrer" className="text-link">
                  View release on GitHub ↗
                </a>
              </div>
            )}
            {releaseState === 'empty' && (
              <div className="release-card release-empty">
                <div className="release-top">
                  <b>v0.1</b>
                  <span>Preview</span>
                </div>
                <h3>No public release yet.</h3>
                <p>
                  The repository is live, but no GitHub Release has been published. Create the first release to show
                  version, date, and release notes here.
                </p>
                <a
                  href="https://github.com/v-reza/agentdeck/releases/new"
                  target="_blank"
                  rel="noreferrer"
                  className="btn-outline"
                >
                  Create v0.1.0 release ↗
                </a>
              </div>
            )}
            {releaseState === 'error' && (
              <div className="fallback-card">
                <b>Release metadata unavailable.</b>
                <p>
                  GitHub API could not be reached right now. The source and release history remain available directly.
                </p>
                <a
                  href="https://github.com/v-reza/agentdeck/releases"
                  target="_blank"
                  rel="noreferrer"
                  className="text-link"
                >
                  Open releases ↗
                </a>
              </div>
            )}
            <a
              href="https://github.com/v-reza/agentdeck/releases"
              target="_blank"
              rel="noreferrer"
              className="text-link release-index-link"
            >
              View all releases ↗
            </a>
          </div>
        </div>
        <Roadmap />
      </div>
    </PublicShell>
  )
}

export { GitHubPage }
