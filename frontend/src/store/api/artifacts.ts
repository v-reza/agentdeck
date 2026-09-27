import { baseApi } from './base'

/**
 * Artifacts per task (US-AD48, ARCHITECTURE 6.2.16).
 *
 * The five endpoints behind this have been live since F13, proven end to end
 * against a real S3-compatible store. Only the list and the download link are
 * wrapped here: those are what US-AD48 specifies. Registration is deliberately
 * absent — it needs a run picker, and the client has no `runs` endpoint yet.
 */
export interface Artifact {
  id: string
  task_id: string
  run_id: string
  filename: string
  content_type: string
  size: number
  sha256: string
  created_at: string
}

export const artifactsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    /**
     * `GET /tasks/{id}/artifacts` (Viewer).
     *
     * ponytail: no cursor. US-AD48 AC2 asks for cursor pagination past 50
     * artifacts, but the server has no cursor — `ListTaskArtifacts` is
     * `WHERE task_id = $1 ORDER BY created_at DESC, id DESC` with no LIMIT and
     * no keyset, so every page is the whole list. A client cannot invent
     * pagination an API does not offer, and slicing "everything" into pages
     * would only hide the work from the database. Upgrade path: add
     * `after`/`limit` to the query and return `next_cursor`, then slice in the
     * component, which renders whatever it is given. Recorded in
     * `docs/OPEN-ISSUES.md`.
     *
     * The tag is keyed per TASK, matching how a mutation would invalidate. A
     * single `LIST` tag would be the same mistake as a global board tag: one
     * task's upload would refetch every task's artifacts.
     */
    listTaskArtifacts: build.query<Artifact[], string>({
      query: (taskID) => `tasks/${taskID}/artifacts`,
      providesTags: (_r, _e, taskID) => [{ type: 'Artifact', id: `TASK-${taskID}` }],
    }),
  }),
})

export const { useListTaskArtifactsQuery } = artifactsApi

/**
 * Where an artifact's bytes are fetched from (US-AD48 AC4).
 *
 * This returns a plain URL rather than an endpoint that asks for a signed link
 * first. The download endpoint IS the signed link: each request makes the API
 * mint a fresh presigned URL and answer 302 to it, so signing happens
 * per-request on the server. A client-held signed URL would be a storage
 * credential living in component state with a lifetime to expire while the
 * drawer sits open, and it would need a query per row. An anchor tag gets all
 * of that for free.
 */
export function artifactDownloadURL(artifactID: string): string {
  return `/api/v1/artifacts/${artifactID}/download`
}
