import { useState, type ChangeEvent } from 'react'
import {
  useListTaskArtifactsQuery,
  useArtifactUploadURLMutation,
  useRegisterArtifactMutation,
  artifactDownloadURL,
  sha256Hex,
} from '@/store/api/artifacts'
import { useListTaskRunsQuery } from '@/store/api/runs'
import { useAppSelector } from '@/store/hooks'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { formatDateTime, formatBytes } from '@/lib/format'
import { describeError } from '@/hooks/use-action-form'
import { SkeletonText } from '@/components/ui/skeleton'
import { Download, Paperclip, Upload } from 'lucide-react'

/**
 * Tab Artifacts (US-AD48, screen 20-task-drawer).
 *
 * Shows what the task produced: filename, content type, size, upload time, and
 * a download link per row. The endpoints behind it have been live since F13 and
 * were proven end to end against a real S3-compatible store, so this tab is the
 * missing half of a finished feature.
 *
 * Upload runs in three calls, and the split is what keeps a 25 MB file off the
 * API's heap: `upload-url` mints a presigned PUT, the browser PUTs the bytes
 * straight to object storage, and `register` tells the API what landed. The API
 * then re-hashes the object it finds and refuses the row when the digest
 * disagrees — so the SHA-256 computed here is not bookkeeping, it is the claim
 * the server checks.
 */
export function TabArtifacts({ taskID }: { taskID: string }) {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const { data: artifacts, isLoading, isError, error } = useListTaskArtifactsQuery(taskID)
  const { data: runs } = useListTaskRunsQuery(taskID)
  const rows = artifacts ?? []

  if (isLoading) return <SkeletonText lines={3} />

  // 503 is the API saying object storage is not configured, which is a different
  // answer from "this task has no artifacts". Collapsing the two would tell an
  // operator their artifacts are gone when the feature is simply switched off,
  // and would send them looking for a bug that is a deployment setting.
  //
  // The status arrives as `originalStatus`, not `status`, and that is not a
  // detail to paper over: every error this API writes with `http.Error` is
  // `text/plain`, while RTK Query's base query parses responses as JSON. The
  // parse throws, so RTK reports `status: 'PARSING_ERROR'` and keeps the real
  // code in `originalStatus`. Branching on `status === 503` compiles and never
  // matches — it was the first version of this code, and the test above caught
  // it. `Number(status)` normalises the numeric case RTK uses for a parseable
  // error body.
  const rt = error as { status?: unknown; originalStatus?: unknown } | undefined
  if (isError && Number(rt?.originalStatus ?? rt?.status) === 503) {
    return <p className="font-mono text-[11px] text-[var(--color-tertiary)]">{t['artifacts.notConfigured']}</p>
  }

  // A failed fetch must not read as an empty task: an operator who sees "no
  // artifacts" stops looking for the ones that exist.
  if (isError) return <p className="text-[12px] text-[var(--color-danger)]">{t['artifacts.loadFailed']}</p>

  return (
    <div className="flex flex-col gap-2">
      <UploadControl taskID={taskID} latestRunID={runs?.at(-1)?.id} />
      {rows.length === 0 ? (
        <p className="text-[12px] text-[var(--color-tertiary)]">{t['artifacts.empty']}</p>
      ) : (
        <ul className="flex flex-col">
          {rows.map((artifact) => (
            <li
              key={artifact.id}
              className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] py-2 last:border-b-0"
            >
              <Paperclip size={13} aria-hidden="true" className="shrink-0 text-[var(--color-tertiary)]" />
              <div className="min-w-0 flex-1">
                <div className="truncate font-mono text-[11px] text-[var(--color-primary)]">{artifact.filename}</div>
                <div className="truncate font-mono text-[10px] text-[var(--color-tertiary)]">
                  {artifact.content_type} · {formatBytes(artifact.size, lang)} ·{' '}
                  {formatDateTime(artifact.created_at, lang)}
                </div>
              </div>
              {/*
                An anchor, not a click handler: the download endpoint mints a
                fresh presigned URL per request and answers 302, so the browser
                follows it and the signed link never lands in component state
                where it could outlive its expiry. That is what US-AD48 AC4 asks
                for.
              */}
              <a
                href={artifactDownloadURL(artifact.id)}
                download={artifact.filename}
                aria-label={`${t['artifacts.download']} ${artifact.filename}`}
                className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[6px] text-[var(--color-secondary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-accent)]"
              >
                <Download size={14} strokeWidth={2} aria-hidden="true" />
              </a>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/**
 * The upload control, or the reason it cannot be offered.
 *
 * Three refusals are deliberate, and each one replaces a silent failure:
 *
 *  - **No run yet.** `run_id` is mandatory server-side (`artifacts_run_fk`), so a
 *    task that was never claimed cannot take an artifact. The control says so
 *    instead of letting the operator pick a file and collect a 400 for a reason
 *    the form never mentioned.
 *  - **Not a Member.** Upload routes are Member; a Viewer sees the list and
 *    nothing else, which is the honest form of that gate.
 *  - **No crypto.subtle.** A browser without WebCrypto cannot produce the digest
 *    the API verifies against, so it cannot upload at all. Checked rather than
 *    assumed, because the failure is otherwise a confusing rejection.
 */
function UploadControl({ taskID, latestRunID }: { taskID: string; latestRunID?: string }) {
  const t = useT()
  const canUpload = useCanAct('member')
  const [uploadURL] = useArtifactUploadURLMutation()
  const [register] = useRegisterArtifactMutation()
  const [state, setState] = useState<{ pending: boolean; error: string | null }>({ pending: false, error: null })

  if (!canUpload) return null
  if (!latestRunID) {
    return <p className="font-mono text-[10px] text-[var(--color-quaternary)]">{t['artifacts.noRun']}</p>
  }
  if (typeof crypto === 'undefined' || !crypto.subtle) {
    return <p className="font-mono text-[10px] text-[var(--color-quaternary)]">{t['artifacts.noCrypto']}</p>
  }

  const runID = latestRunID

  async function onPick(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    // Clear the input straight away so picking the same file again still fires.
    event.target.value = ''
    if (!file) return

    setState({ pending: true, error: null })
    try {
      const digest = await sha256Hex(file)
      const ticket = await uploadURL({
        taskID,
        run_id: runID,
        filename: file.name,
        content_type: file.type || 'application/octet-stream',
        size: file.size,
      }).unwrap()

      // The bytes go straight to storage. No Authorization header: the URL is
      // already signed, and adding a credential here would hand it to a third
      // party for nothing.
      const put = await fetch(ticket.upload_url, { method: 'PUT', body: file })
      if (!put.ok) {
        throw new Error(`${t['artifacts.uploadFailed']} (${put.status})`)
      }

      await register({
        taskID,
        run_id: runID,
        filename: file.name,
        content_type: file.type || 'application/octet-stream',
        size: file.size,
        sha256: digest,
        storage_key: ticket.storage_key,
      }).unwrap()

      setState({ pending: false, error: null })
    } catch (error) {
      // `describeError` unwraps the API's JSON body the way every other form in
      // the app does, so a rejection reads the same here as it does elsewhere.
      setState({ pending: false, error: describeError(error) })
    }
  }

  return (
    <div className="flex flex-col gap-1">
      <label className="flex cursor-pointer items-center gap-2 self-start font-mono text-[11px] text-[var(--color-accent)] transition-colors hover:text-[var(--color-accent-hover)]">
        <Upload size={13} strokeWidth={2} aria-hidden="true" />
        {state.pending ? t['artifacts.uploading'] : t['artifacts.upload']}
        <input
          type="file"
          className="sr-only"
          aria-label={t['artifacts.upload']}
          disabled={state.pending}
          onChange={onPick}
        />
      </label>
      {state.error ? <p className="text-[11px] text-[var(--color-danger)]">{state.error}</p> : null}
    </div>
  )
}
