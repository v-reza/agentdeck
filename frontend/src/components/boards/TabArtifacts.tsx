import { useListTaskArtifactsQuery, artifactDownloadURL } from '@/store/api/artifacts'
import { useAppSelector } from '@/store/hooks'
import { useT } from '@/hooks/use-t'
import { formatDateTime, formatBytes } from '@/lib/format'
import { SkeletonText } from '@/components/ui/skeleton'
import { Download, Paperclip } from 'lucide-react'

/**
 * Tab Artifacts (US-AD48, screen 20-task-drawer).
 *
 * Shows what the task produced: filename, content type, size, upload time, and
 * a download link per row. The endpoints behind it have been live since F13 and
 * were proven end to end against a real S3-compatible store, so this tab is the
 * missing half of a finished feature.
 */
export function TabArtifacts({ taskID }: { taskID: string }) {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const { data: artifacts, isLoading, isError, error } = useListTaskArtifactsQuery(taskID)
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

  if (rows.length === 0) return <p className="text-[12px] text-[var(--color-tertiary)]">{t['artifacts.empty']}</p>

  return (
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
              {artifact.content_type} · {formatBytes(artifact.size, lang)} · {formatDateTime(artifact.created_at, lang)}
            </div>
          </div>
          {/*
            An anchor, not a click handler: the download endpoint mints a fresh
            presigned URL per request and answers 302, so the browser follows it
            and the signed link never lands in component state where it could
            outlive its expiry. That is what US-AD48 AC4 asks for.
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
  )
}
