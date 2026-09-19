// One release row in the changelog.
import type { GitHubRelease } from '../lib/useGitHubReleases'

function ReleaseEntry({ release }: { release: GitHubRelease }) {
  const date = release.published_at
    ? new Intl.DateTimeFormat('en-US', { year: 'numeric', month: 'long', day: 'numeric' }).format(
        new Date(release.published_at),
      )
    : 'Unpublished'
  const notes =
    release.body
      ?.split('\n')
      .map((line) => line.replace(/^[-*#\s]+/, '').trim())
      .filter(Boolean)
      .slice(0, 8) ?? []
  return (
    <article className="changelog-entry">
      <div className="release-top">
        <b>{release.tag_name}</b>
        <span>{release.prerelease ? 'Pre-release' : 'Stable'}</span>
        <time>{date}</time>
      </div>
      <h2>{release.name || release.tag_name}</h2>
      {notes.length ? (
        <ul>
          {notes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      ) : (
        <p>No release notes were added.</p>
      )}
      <a href={release.html_url} target="_blank" rel="noreferrer" className="text-link">
        View release on GitHub ↗
      </a>
    </article>
  )
}

export { ReleaseEntry }
