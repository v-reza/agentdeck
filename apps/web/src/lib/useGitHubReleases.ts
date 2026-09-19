import { useEffect, useState } from 'react'

export type GitHubRelease = {
  tag_name: string
  name: string
  html_url: string
  published_at: string | null
  body: string | null
  prerelease: boolean
  draft: boolean
}

export type ReleaseStatus = 'loading' | 'ready' | 'empty' | 'error'

const RELEASES_URL = 'https://api.github.com/repos/v-reza/agentdeck/releases'
const HEADERS = { Accept: 'application/vnd.github+json' } as const

/**
 * Fetches the GitHub Releases feed for v-reza/agentdeck.
 *
 * Pass a `perPage` of 5 to resolve the latest published release (GitHubPage),
 * or 20 to resolve the full history (ChangelogPage). Requests are cancelled on
 * unmount so a slow response can never write state into an unmounted page.
 */
export function useGitHubReleases(perPage = 20) {
  const [status, setStatus] = useState<ReleaseStatus>('loading')
  const [releases, setReleases] = useState<GitHubRelease[]>([])

  useEffect(() => {
    let cancelled = false

    fetch(`${RELEASES_URL}?per_page=${perPage}`, { headers: HEADERS })
      .then((response) => {
        if (!response.ok) throw new Error(`GitHub releases returned ${response.status}`)
        return response.json() as Promise<GitHubRelease[]>
      })
      .then((items) => {
        if (cancelled) return
        setReleases(items)
        setStatus(items.length ? 'ready' : 'empty')
      })
      .catch(() => {
        if (!cancelled) setStatus('error')
      })

    return () => {
      cancelled = true
    }
  }, [perPage])

  return { status, releases }
}
