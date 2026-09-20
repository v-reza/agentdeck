import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'

/**
 * The public GitHub Releases feed for the repo itself (changelog + GitHub pages).
 *
 * It is a second `createApi` rather than an endpoint on `baseApi` because it
 * talks to a different origin with different auth: `baseApi` targets our own
 * `/api/v1` and attaches the session cookie and X-Org-ID, neither of which
 * belongs on a request to api.github.com.
 *
 * It is still RTK Query, not a `fetch` in an effect, so the release list has one
 * cache, one loading state, and no second source of truth (ARCHITECTURE 18.2).
 */
export interface GitHubRelease {
  tag_name: string
  name: string
  html_url: string
  published_at: string | null
  body: string | null
  prerelease: boolean
  draft: boolean
}

export const releasesApi = createApi({
  reducerPath: 'releases',
  baseQuery: fetchBaseQuery({
    baseUrl: 'https://api.github.com/repos/v-reza/agentdeck',
  }),
  endpoints: (build) => ({
    releases: build.query<GitHubRelease[], number>({
      query: (perPage) => ({
        url: 'releases',
        params: { per_page: perPage },
        headers: { Accept: 'application/vnd.github+json' },
      }),
    }),
  }),
})

export const { useReleasesQuery } = releasesApi
