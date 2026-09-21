import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useListProvidersQuery, type Provider } from '@/store/api/providers'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { formatCount, interpolate } from '@/lib/format'
import { formatRelative, plural } from '@/lib/formatters'
import { useAppSelector } from '@/store/hooks'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Button } from '@/components/ui/button'
import { EmptyState, Panel } from '@/components/ui/card'
import { DeleteProviderDialog, ProviderFormDialog, ProviderRowActions, VerifiedBadge } from './providers-actions'

/**
 * Screen 47-providers — the per-workspace credential registry (US-AD109).
 *
 * Cloned from `47-providers.html`: the same 9 columns in the same order (name,
 * protocol, base URL, credential, models, verification, model sync, default,
 * actions), a 32px header row and 28px rows, the credential cell masked to
 * bullets and never a value (AC2), and the default badge on at most one row
 * (AC9).
 *
 * Four deliberate deviations, each a fact the design cannot know:
 *
 *  1. The design draws a 420px detail drawer beside the list. It is not cloned.
 *     Everything the drawer shows is already a column, and a second surface
 *     rendering the same fields is a second place for them to disagree. Edit is
 *     a modal instead, matching the members roster's decision.
 *  2. The design annotates the header with `AC1`/`AC3`/`AC7`/`AC9` and a
 *     "COMPLIANT" badge. Those are notes for whoever reviews the mock, not
 *     product copy, so they are not cloned — the repo forbids spec jargon in
 *     rendered UI.
 *  3. The design shows a fixed `2 jam lalu` / `26 jam lalu (kedaluwarsa)`. Here
 *     the column is computed from `models_fetched_at`, and the stale marker uses
 *     the same 24-hour window the backend's refresher uses (AC7) rather than a
 *     number that drifts out of date the moment it is rendered.
 *  4. The design's row actions are two bare icons. They carry `aria-label`s
 *     naming the provider: two unlabelled icon buttons per row is unusable by
 *     screen reader, and the design cannot express that.
 */
export function Providers() {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const orgs = useAppSelector((state) => state.session.workspaces)
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const org = orgs.find((workspace) => workspace.id === activeOrgID)

  const { data, isLoading, isError } = useListProvidersQuery()
  const providers = data ?? []
  const canManage = useCanAct('admin')

  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Provider | null>(null)
  const [deleting, setDeleting] = useState<Provider | null>(null)

  return (
    <>
      <WorkspaceTopbar
        title={t['providers.title']}
        path="/settings/providers"
        subtitle={org ? org.name : t['providers.subtitle']}
      />

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
        <Panel className="overflow-hidden">
          <div className="flex items-center justify-between gap-4 border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-well)] px-3.5 py-2.5">
            <div className="flex min-w-0 items-center gap-2">
              <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['providers.title']}</h2>
              <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
                {interpolate(t['providers.count'], [formatCount(providers.length, lang)])}
              </span>
            </div>
            {canManage ? (
              <Button
                variant="primary"
                size="md"
                className="shrink-0 gap-1.5 px-3 shadow-sm"
                onClick={() => setCreating(true)}
              >
                <Plus size={14} />
                {t['providers.add']}
              </Button>
            ) : null}
          </div>

          {isLoading ? (
            <p className="px-3.5 py-6 font-mono text-[12px] text-[var(--color-tertiary)]">{t['state.loading']}</p>
          ) : isError ? (
            <div className="p-3.5">
              <EmptyState title={t['providers.loadFailed']} />
            </div>
          ) : providers.length === 0 ? (
            <div className="p-3.5">
              <EmptyState title={t['providers.empty']} hint={t['providers.emptyHint']} />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-left">
                <thead>
                  <tr className="h-8 border-b border-[var(--color-border-subtle)] font-mono text-[10px] uppercase tracking-[0.04em] text-[var(--color-tertiary)]">
                    <th className="px-3 font-medium">{t['providers.colName']}</th>
                    <th className="px-3 font-medium">{t['providers.colProtocol']}</th>
                    <th className="px-3 font-medium">{t['providers.colBaseUrl']}</th>
                    <th className="px-3 font-medium">{t['providers.colCredential']}</th>
                    <th className="px-3 font-medium">{t['providers.colModels']}</th>
                    <th className="px-3 font-medium">{t['providers.colVerified']}</th>
                    <th className="px-3 font-medium">{t['providers.colSync']}</th>
                    <th className="px-3 font-medium">{t['providers.colDefault']}</th>
                    <th className="px-3 text-right font-medium">{t['providers.colActions']}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--color-border-subtle)] text-[12px]">
                  {providers.map((provider) => (
                    <tr
                      key={provider.id}
                      // The design pins a fixed 28px row, which only holds if the
                      // cells stay on one line: at the default wrapping the
                      // protocol badge and the sync label break to a second line
                      // and push the row to 37px. `align-middle` removes the
                      // baseline inflation from mixing mono and sans cells, and
                      // `whitespace-nowrap` keeps every cell single-line — the
                      // panel's `overflow-x-auto` takes the overflow.
                      className="h-7 hover:bg-[var(--color-surface-hover)] [&>td]:align-middle [&>td]:whitespace-nowrap"
                    >
                      <td className="px-3 py-0 font-medium text-[var(--color-primary)]">{provider.name}</td>
                      <td className="px-3 py-0">
                        <span className="rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-secondary)]">
                          {provider.protocol}
                        </span>
                      </td>
                      <td className="max-w-[220px] truncate px-3 py-0 font-mono text-[11px] text-[var(--color-tertiary)]">
                        {provider.base_url}
                      </td>
                      <td className="px-3 py-0">
                        {/* AC2: masked, and only ever masked. The design's cell is
                          the contract here — bullets plus a label. */}
                        <span className="font-mono text-[11px] text-[var(--color-quaternary)]">••••••••••••</span>{' '}
                        <span className="text-[10px] text-[var(--color-tertiary)]">
                          {provider.has_key ? t['providers.encrypted'] : '—'}
                        </span>
                      </td>
                      <td className="px-3 py-0 font-mono text-[11px] text-[var(--color-secondary)]">
                        {plural(provider.models.length, 'model')}
                      </td>
                      <td className="px-3 py-0">
                        <VerifiedBadge provider={provider} />
                      </td>
                      <td className="px-3 py-0 text-[11px]">
                        <SyncCell
                          provider={provider}
                          staleLabel={t['providers.stale']}
                          neverLabel={t['providers.neverFetched']}
                        />
                      </td>
                      <td className="px-3 py-0">
                        {provider.is_default ? (
                          <span className="whitespace-nowrap rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-primary)]">
                            {t['providers.default']}
                          </span>
                        ) : (
                          <span className="text-[var(--color-quaternary)]">—</span>
                        )}
                      </td>
                      <td className="px-3 py-0">
                        <ProviderRowActions
                          provider={provider}
                          canManage={canManage}
                          onEdit={() => setEditing(provider)}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>
      </div>

      {creating ? <ProviderFormDialog open onClose={() => setCreating(false)} canManage={canManage} /> : null}
      {editing ? (
        <ProviderFormDialog
          open
          onClose={() => setEditing(null)}
          provider={editing}
          canManage={canManage}
          onRequestDelete={() => {
            setDeleting(editing)
            setEditing(null)
          }}
        />
      ) : null}
      {deleting ? <DeleteProviderDialog open onClose={() => setDeleting(null)} provider={deleting} /> : null}
    </>
  )
}

/** AC7's freshness column. Stale is computed, never stored. */
function SyncCell({
  provider,
  staleLabel,
  neverLabel,
}: {
  provider: Provider
  staleLabel: string
  neverLabel: string
}) {
  if (!provider.models_fetched_at) {
    return <span className="text-[var(--color-warning)]">{neverLabel}</span>
  }
  const fetched = new Date(provider.models_fetched_at).getTime()
  // Same 24-hour window as the backend's `ModelsStaleAfter`, so the marker and
  // the refresher cannot disagree about what "stale" means.
  const stale = Date.now() - fetched > 24 * 60 * 60 * 1000
  const relative = formatRelative(provider.models_fetched_at)
  if (stale) {
    return <span className="text-[var(--color-warning)]">{interpolate(staleLabel, [relative])}</span>
  }
  return <span className="text-[var(--color-tertiary)]">{relative}</span>
}
