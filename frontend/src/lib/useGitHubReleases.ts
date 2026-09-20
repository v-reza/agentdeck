import { useReleasesQuery, type GitHubRelease } from '@/store/api/releases'

/**
 * The changelog/GitHub pages want a status string, not the raw RTK Query result.
 * Mapping it here keeps those pages free of cache internals while the data still
 * comes from the shared RTK Query cache (ARCHITECTURE 18.2: server state lives in
 * Redux, never in a component effect).
 */
export type ReleaseStatus = 'loading' | 'ready' | 'empty' | 'error'
export type { GitHubRelease }

export function useGitHubReleases(perPage: number) {
  const { data, isLoading, isError } = useReleasesQuery(perPage)

  let status: ReleaseStatus = 'loading'
  if (isError) status = 'error'
  else if (isLoading) status = 'loading'
  else if (!data || data.length === 0) status = 'empty'
  else status = 'ready'

  return { status, releases: data ?? [] }
}
