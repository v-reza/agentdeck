import type { ReactNode } from 'react'
import { Panel } from '@/components/ui/card'
import { interpolate } from '@/lib/format'
import { useT } from '@/hooks/use-t'

/**
 * The two reference cards under the registry table, cloned from
 * `design/stitch-output/v2/25-agent-registry.html` (the `grid grid-cols-2 gap-4`
 * block after the table's footer).
 *
 * The design's copy is a spec explainer: it names the stories ("US-AD20",
 * "US-AD73"), the acceptance-criterion numbers, the HTTP status, and an internal
 * roadmap phase. That vocabulary is for the PRD, not the product — an operator
 * does not need to know which story shipped the screen. So the shape, the grid,
 * the chips and the tokens are the design's, while the words say what the reader
 * can act on: what a valid agent needs, and what archiving one does.
 *
 * The right card's "assign-task dropdown simulation" was a static picture of a
 * `<select>`; it is replaced by the count that control would have shown, which is
 * real data rather than a drawing of a control that does not exist yet.
 *
 * Colours are the design system's tokens, not the mock's raw hex: the mock's
 * `#16a34a`/`#94a3b8` are `--color-status-done`/`--color-status-archived`, and
 * its amber note is `--color-warning` on the page surface.
 */
export function AgentSpecCards({ readyCount, archivedCount }: { readyCount: number; archivedCount: number }) {
  const t = useT()

  return (
    <div data-testid="agents-spec-cards" className="grid grid-cols-2 gap-4">
      <Panel className="p-3.5 shadow-xs">
        <CardHeader
          dotClassName="bg-[var(--color-accent)]"
          title={t['agents.spec.title']}
          badge={t['agents.spec.badge']}
          badgeClassName="bg-[var(--color-accent-tint)] text-[var(--color-accent)]"
        />

        <div className="space-y-2 text-[11px]">
          <p className="text-[var(--color-secondary)]">{t['agents.spec.intro']}</p>

          <div className="grid grid-cols-2 gap-1.5 font-mono text-[10px]">
            {FIELDS.map(([label, value]) => (
              <div
                key={label}
                className="rounded border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-1.5"
              >
                <span className="text-[var(--color-tertiary)]">{label}</span>{' '}
                <span className="font-semibold break-words text-[var(--color-primary)]">{value}</span>
              </div>
            ))}
          </div>

          <div className="mt-2 rounded-[6px] border border-[var(--color-warning)]/30 bg-[var(--color-surface-page)] p-2 text-[var(--color-warning)]">
            <strong className="font-semibold">{t['agents.spec.credentialLabel']}</strong>{' '}
            {t['agents.spec.credentialBody']}
          </div>
        </div>
      </Panel>

      <Panel className="flex flex-col justify-between p-3.5 shadow-xs">
        <div>
          <CardHeader
            dotClassName="bg-[var(--color-status-archived)]"
            title={t['agents.archive.title']}
            badge={t['agents.archive.badge']}
            badgeClassName="bg-[var(--color-surface-page)] text-[var(--color-secondary)]"
          />

          <div className="space-y-2.5 text-[11px]">
            <Note
              label={t['agents.archive.runningLabel']}
              body={withCode(t['agents.archive.runningBody'], ['agent-legacy-qa', 'running'])}
            />
            <Note
              label={t['agents.archive.assignLabel']}
              body={withCode(t['agents.archive.assignBody'], ['agent-legacy-qa'])}
            />

            <div className="flex items-center justify-between rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] px-2 py-1.5 text-[10px]">
              <span className="text-[var(--color-secondary)]">
                {interpolate(t['agents.archive.assignable'], [String(readyCount)])}
              </span>
              <span className="font-mono text-[var(--color-tertiary)]">
                {interpolate(t['agents.archive.hidden'], [String(archivedCount)])}
              </span>
            </div>
          </div>
        </div>

        <div className="mt-2 flex items-center gap-1.5 border-t border-[var(--color-border-subtle)] pt-2 text-[10px] text-[var(--color-tertiary)]">
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-status-done)]" />
          <span>{t['agents.archive.note']}</span>
        </div>
      </Panel>
    </div>
  )
}

/** The card's dot + title + right-hand chip, the same shape on both cards. */
function CardHeader({
  dotClassName,
  title,
  badge,
  badgeClassName,
}: {
  dotClassName: string
  title: string
  badge: string
  badgeClassName: string
}) {
  return (
    <div className="mb-2.5 flex items-center justify-between gap-2 border-b border-[var(--color-border-subtle)] pb-2">
      <div className="flex min-w-0 items-center gap-2">
        <span className={`h-2 w-2 shrink-0 rounded-full ${dotClassName}`} />
        <span className="text-[12px] font-bold text-[var(--color-primary)]">{title}</span>
      </div>
      <span className={`shrink-0 rounded px-1.5 py-0.5 font-mono text-[10px] whitespace-nowrap ${badgeClassName}`}>
        {badge}
      </span>
    </div>
  )
}

/** One rule: a tick, a bold label, and a paragraph. */
function Note({ label, body }: { label: string; body: ReactNode }) {
  return (
    <div className="flex items-start gap-2">
      <div className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[var(--color-status-done)]/10 text-[9px] font-bold text-[var(--color-status-done)]">
        ✓
      </div>
      <div>
        <strong className="text-[var(--color-primary)]">{label}</strong>
        <p className="mt-0.5 text-[var(--color-secondary)]">{body}</p>
      </div>
    </div>
  )
}

/**
 * Renders `{0}`-style placeholders as inline `<code>` spans, so the sentence
 * stays one translatable string instead of three fragments.
 */
function withCode(text: string, codes: string[]): ReactNode[] {
  return text.split(/\{(\d+)\}/).map((part, index) =>
    index % 2 === 0 ? (
      part
    ) : (
      <code key={index} className="rounded bg-[var(--color-surface-page)] px-1 font-mono">
        {codes[Number(part)]}
      </code>
    ),
  )
}

/** What a valid agent needs, in the design's own chip grid. */
const FIELDS: [string, string][] = [
  ['name:', 'string (slug)'],
  ['provider:', 'openai | anthropic | custom'],
  ['model:', 'gpt-4o | claude-sonnet-4-6'],
  ['reasoning_effort:', 'low | medium | high'],
  ['skills:', 'daftar skill org'],
  ['tools:', 'allowlist tool'],
  ['max_runtime_seconds:', '1800'],
  ['retry_policy:', 'transient_only, 3x'],
]
