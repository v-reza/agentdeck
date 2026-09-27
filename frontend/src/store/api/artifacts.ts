import { baseApi } from './base'

/**
 * Artifacts per task (US-AD48, ARCHITECTURE 6.2.16).
 *
 * The five endpoints behind this have been live since F13, proven end to end
 * against a real S3-compatible store. The list, the register call and the
 * download link are wrapped here.
 *
 * Why a third endpoint (upload-url) exists at all: the API never receives the
 * bytes. It hands back a presigned URL, the browser PUTs the file straight to
 * object storage, and the register call then tells the API what landed. That is
 * what keeps a 25 MB artifact off the API's heap and out of its request body.
 *
 * `upload-url` answers with a `storage_key` and an `artifact_id`. The register
 * call sends the key back and the API checks it is under THIS org AND THIS task
 * before accepting it — a key is not a credential here, it is a claim the server
 * verifies.
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

    /** `POST /tasks/{id}/artifacts/upload-url` (Member). */
    artifactUploadURL: build.mutation<UploadTicket, UploadURLArgs>({
      query: ({ taskID, ...body }) => ({
        url: `tasks/${taskID}/artifacts/upload-url`,
        method: 'POST',
        body,
      }),
    }),

    /**
     * `POST /tasks/{id}/artifacts` (Member).
     *
     * Invalidates the task's artifact list, which is what makes the new row
     * appear: the list is the one query the tab renders from, so there is no
     * second copy to patch.
     */
    registerArtifact: build.mutation<Artifact, RegisterArgs>({
      query: ({ taskID, ...body }) => ({ url: `tasks/${taskID}/artifacts`, method: 'POST', body }),
      invalidatesTags: (_r, _e, { taskID }) => [{ type: 'Artifact', id: `TASK-${taskID}` }],
    }),
  }),
})

export const { useListTaskArtifactsQuery, useArtifactUploadURLMutation, useRegisterArtifactMutation } = artifactsApi

export interface UploadTicket {
  upload_url: string
  storage_key: string
  artifact_id: string
  expires_at: string
  max_file_size: number
}

export interface UploadURLArgs {
  taskID: string
  run_id: string
  filename: string
  content_type: string
  size: number
}

export interface RegisterArgs {
  taskID: string
  run_id: string
  filename: string
  content_type: string
  size: number
  sha256: string
  storage_key: string
}

/**
 * SHA-256 as lowercase hex, computed in the browser.
 *
 * The digest is not bookkeeping: the API re-hashes the object it finds under
 * `storage_key` and refuses the registration when the two differ, so this value
 * is what makes "the file the operator picked" and "the file that landed in
 * storage" the same statement. `crypto.subtle` is the only digest available in
 * a browser without shipping a hash implementation.
 */
export async function sha256Hex(file: File): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', await file.arrayBuffer())
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

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
