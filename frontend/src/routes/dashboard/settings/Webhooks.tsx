import { useState } from 'react'
import { Plus, RotateCw, Trash2 } from 'lucide-react'
import {
  useCreateWebhookMutation,
  useDeleteWebhookMutation,
  useListWebhookDeliveriesQuery,
  useListWebhooksQuery,
  usePatchWebhookMutation,
  useRetryWebhookDeliveryMutation,
  type Webhook,
} from '@/store/api/webhooks'
import { useListProjectsQuery, useListBoardsQuery } from '@/store/api/boards'

import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { Combobox } from '@/components/ui/combobox'
import { Modal } from '@/components/ui/modal'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { formatDateTime } from '@/lib/format'

/**
 * Screen: webhooks — `design/stitch-output/v2/40-webhooks.html`, US-AD52 +
 * US-AD53.
 *
 * The seven endpoints have been live since the webhook worker landed, and this
 * screen was a stub saying "not available yet" — a claim that had stopped being
 * true. What it renders now is what the API actually serves.
 *
 * Three things the design draws that the API cannot answer, so none is faked:
 *
 *  - **A masked secret column** (`whsec_••••9a1f`). `CreateInput` takes a
 *    secret; `webhookResponse` has no field for it and there is no reveal
 *    endpoint. The secret is write-only — typed once at creation, then gone.
 *    Rendering a mask would be inventing the four characters it shows.
 *  - **A "Semua Board (Global)" scope.** `webhooks.board_id` is NOT NULL and
 *    `ListMatchingWebhooks` matches on `board_id`. A webhook always belongs to
 *    one board, so the scope picker has no "global" option — a webhook that
 *    fires for every board is not representable.
 *  - **A last-delivery cell per row.** The webhook row carries no delivery
 *    data, and filling one cell per row would cost one request per row.
 *    Deliveries are one panel, fetched when a webhook is selected.
 *
 * "Auto-retry: aktif (backoff)" is also absent: the attempt ceiling belongs to
 * the worker, and this screen shows the attempts that actually happened rather
 * than restating the policy.
 */

/**
 * The canonical event kinds, from DECISIONS §4.
 *
 * A fixed list rather than free text: `normalizeEvents` accepts any non-empty
 * string, so a typo produces a webhook that never matches and never says why.
 */
const EVENT_KINDS = [
  'task.created',
  'task.status_changed',
  'task.assigned',
  'run.claimed',
  'run.heartbeat',
  'run.finished',
  'run.reclaimed',
  'step.started',
  'step.finished',
  'step.failed',
  'approval.requested',
  'approval.decided',
  'approval.expired',
  'artifact.created',
  'ledger.entry',
  'comment.created',
  'budget.threshold_crossed',
] as const

export function Webhooks() {
  const t = useT()
  const canAdmin = useCanAct('admin')
  // Project -> board, two single hooks. A workspace-wide board list would need
  // one query per project inside a loop, and this repo's rule (use-directory.ts)
  // is that a hook is never called inside `map()` — RTK Query has no
  // `useQueries`, and a hook loop breaks the rules of hooks.
  const { data: projects } = useListProjectsQuery()
  const [projectID, setProjectID] = useState('')
  const activeProject = projectID || projects?.[0]?.id || ''
  const { data: boards } = useListBoardsQuery(activeProject, { skip: !activeProject })
  const [boardID, setBoardID] = useState('')
  const [creating, setCreating] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)

  const activeBoard = boardID || boards?.[0]?.id || ''
  const { data, isLoading } = useListWebhooksQuery(activeBoard, { skip: !activeBoard })
  const [patchWebhook] = usePatchWebhookMutation()
  const [deleteWebhook] = useDeleteWebhookMutation()

  const rows = data ?? []

  return (
    <>
      <WorkspaceTopbar
        title={t['webhooks.title']}
        path="/settings/webhooks"
        subtitle={rows.length > 0 ? t['webhooks.count'].replace('{count}', String(rows.length)) : undefined}
        right={
          canAdmin && activeBoard ? (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus size={12} />
              {t['webhooks.add']}
            </Button>
          ) : null
        }
      />

      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-4">
        {!canAdmin ? (
          <div
            role="status"
            className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-3.5 py-2 text-[11px] text-[var(--color-secondary)]"
          >
            {t['webhooks.needsAdmin']}
          </div>
        ) : null}

        {(boards ?? []).length === 0 ? (
          <EmptyState title={t['webhooks.noBoards']} hint={t['webhooks.noBoardsHint']} />
        ) : (
          <>
            <div className="flex flex-wrap items-end gap-2">
              <div className="w-[220px]">
                <Combobox
                  label={t['webhooks.project']}
                  value={activeProject}
                  onChange={(next) => {
                    setProjectID(next)
                    // The previous board belongs to the previous project; keeping
                    // it would list webhooks for a board the picker no longer shows.
                    setBoardID('')
                    setSelected(null)
                  }}
                  options={(projects ?? []).map((project) => ({ value: project.id, label: project.name }))}
                />
              </div>
              <div className="w-[220px]">
                <Combobox
                  label={t['webhooks.board']}
                  value={activeBoard}
                  onChange={(next) => {
                    setBoardID(next)
                    setSelected(null)
                  }}
                  options={(boards ?? []).map((board) => ({ value: board.id, label: board.name }))}
                />
              </div>
            </div>

            {isLoading ? (
              <SkeletonRows rows={3} columns={4} />
            ) : rows.length === 0 ? (
              <div data-testid="webhooks-empty">
                <EmptyState title={t['webhooks.none']} hint={t['webhooks.noneHint']} />
              </div>
            ) : (
              <Panel className="overflow-hidden">
                <table className="w-full border-collapse text-left">
                  <thead>
                    <tr className="border-b border-[var(--color-border-subtle)] font-mono text-[10px] text-[var(--color-tertiary)]">
                      <th className="px-3 py-2 font-medium">{t['webhooks.colUrl']}</th>
                      <th className="px-3 py-2 font-medium">{t['webhooks.colEvents']}</th>
                      <th className="px-3 py-2 font-medium">{t['webhooks.colStatus']}</th>
                      <th className="px-3 py-2 font-medium">{t['webhooks.colActions']}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((webhook) => (
                      <WebhookRow
                        key={webhook.id}
                        webhook={webhook}
                        canAdmin={canAdmin}
                        selected={selected === webhook.id}
                        onSelect={() => setSelected(selected === webhook.id ? null : webhook.id)}
                        onToggle={(active) => patchWebhook({ id: webhook.id, active })}
                        onDelete={() => deleteWebhook(webhook.id)}
                      />
                    ))}
                  </tbody>
                </table>
              </Panel>
            )}

            {selected ? <Deliveries webhookID={selected} /> : null}
          </>
        )}
      </div>

      {creating && activeBoard ? <CreateWebhookModal boardID={activeBoard} onClose={() => setCreating(false)} /> : null}
    </>
  )
}

function WebhookRow({
  webhook,
  canAdmin,
  selected,
  onSelect,
  onToggle,
  onDelete,
}: {
  webhook: Webhook
  canAdmin: boolean
  selected: boolean
  onSelect: () => void
  onToggle: (active: boolean) => void
  onDelete: () => void
}) {
  const t = useT()
  return (
    <tr
      data-testid={`webhook-${webhook.id}`}
      className="border-b border-[var(--color-border-subtle)] text-[12px] last:border-b-0"
    >
      <td className="px-3 py-2">
        <button
          type="button"
          onClick={onSelect}
          aria-expanded={selected}
          className="max-w-[320px] truncate font-mono text-[11px] text-[var(--color-primary)] hover:underline"
        >
          {webhook.url}
        </button>
      </td>
      <td
        className="px-3 py-2 font-mono text-[10px] text-[var(--color-secondary)]"
        data-testid="webhook-events"
        data-event-count={webhook.events_json.length}
      >
        {webhook.events_json.length === 0 ? (
          // Empty is "every event", not "none" — `ListMatchingWebhooks` reads a
          // zero-length array as unfiltered. Saying "none" here would be the
          // opposite of what the server does.
          <span
            data-testid="webhook-all-events"
            className="rounded-[4px] bg-[var(--color-accent-tint)] px-1.5 py-0.5 text-[var(--color-accent)]"
          >
            {t['webhooks.allEvents']}
          </span>
        ) : (
          `${webhook.events_json.length} ${t['webhooks.events']}`
        )}
      </td>
      <td className="px-3 py-2">
        <span
          data-status={webhook.active ? 'active' : 'paused'}
          className={`font-mono text-[10px] ${webhook.active ? 'text-[var(--color-accent)]' : 'text-[var(--color-tertiary)]'}`}
        >
          {webhook.active ? t['webhooks.active'] : t['webhooks.paused']}
        </span>
      </td>
      <td className="px-3 py-2">
        {canAdmin ? (
          <div className="flex items-center gap-1">
            <Button size="sm" variant="ghost" onClick={() => onToggle(!webhook.active)}>
              {webhook.active ? t['webhooks.pause'] : t['webhooks.resume']}
            </Button>
            <Button size="sm" variant="ghost" className="text-[var(--color-danger)]" onClick={onDelete}>
              <Trash2 size={12} />
            </Button>
          </div>
        ) : null}
      </td>
    </tr>
  )
}

/**
 * US-AD53: what was actually delivered, and the manual retry.
 *
 * `attempts` and `status` are the server's record — this panel does not restate
 * a retry policy, it shows the attempts that happened and lets an operator
 * re-send one that failed.
 */
function Deliveries({ webhookID }: { webhookID: string }) {
  const t = useT()
  const { data, isLoading } = useListWebhookDeliveriesQuery(webhookID)
  const [retry] = useRetryWebhookDeliveryMutation()
  const rows = data ?? []

  return (
    <Panel className="p-3">
      <div className="font-mono text-[10px] uppercase tracking-[0.08em] text-[var(--color-tertiary)]">
        {t['webhooks.deliveries']}
      </div>
      {isLoading ? (
        <SkeletonRows rows={2} columns={3} />
      ) : rows.length === 0 ? (
        <p className="mt-2 text-[12px] text-[var(--color-tertiary)]">{t['webhooks.noDeliveries']}</p>
      ) : (
        <ul className="mt-2 flex flex-col gap-1">
          {rows.map((delivery) => (
            <li
              key={delivery.id}
              data-testid={`delivery-${delivery.id}`}
              className="flex items-center gap-3 border-b border-[var(--color-border-subtle)] py-1.5 text-[11px] last:border-b-0"
            >
              <span data-status={delivery.status} className="font-mono text-[10px] text-[var(--color-secondary)]">
                {delivery.status}
              </span>
              <span
                data-testid="delivery-attempts"
                data-attempts={delivery.attempts}
                className="font-mono text-[10px] tabular-nums text-[var(--color-tertiary)]"
              >
                {t['webhooks.attempts']} {delivery.attempts}
              </span>
              {delivery.response_code !== null ? (
                <span className="font-mono text-[10px] tabular-nums text-[var(--color-tertiary)]">
                  {delivery.response_code}
                </span>
              ) : null}
              {delivery.last_error ? (
                <span data-testid="delivery-error" className="min-w-0 flex-1 truncate text-[var(--color-danger)]">
                  {delivery.last_error}
                </span>
              ) : null}
              <span className="ml-auto font-mono text-[10px] text-[var(--color-tertiary)]">
                {formatDateTime(delivery.created_at, 'en')}
              </span>
              <Button size="sm" variant="ghost" onClick={() => retry({ webhookID, deliveryID: delivery.id })}>
                <RotateCw size={12} />
                {t['webhooks.retry']}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

/**
 * US-AD52 AC1: url, secret, events.
 *
 * The URL rule is mirrored here so a bad scheme is refused before the request:
 * `ValidateURL` accepts https anywhere and http only on loopback. The server
 * stays the authority — this only saves a round trip on the common mistake.
 */
function CreateWebhookModal({ boardID, onClose }: { boardID: string; onClose: () => void }) {
  const t = useT()
  const [createWebhook, { isLoading }] = useCreateWebhookMutation()
  const [url, setUrl] = useState('')
  const [secret, setSecret] = useState('')
  const [events, setEvents] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    setError(null)
    try {
      await createWebhook({ boardID, url, secret, events }).unwrap()
      onClose()
    } catch {
      setError(t['webhooks.createFailed'])
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t['webhooks.add']}
      description={t['webhooks.modalHint']}
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose}>
            {t['webhooks.cancel']}
          </Button>
          <Button size="sm" onClick={submit} disabled={isLoading || !url || !secret}>
            {t['webhooks.save']}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <Field label={t['webhooks.colUrl']} required>
          <Input
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            placeholder="https://example.com/hooks/agentdeck"
          />
        </Field>
        <Field label={t['webhooks.secret']} required hint={t['webhooks.secretHint']}>
          <Input value={secret} onChange={(event) => setSecret(event.target.value)} />
        </Field>
        <Field label={t['webhooks.colEvents']} hint={t['webhooks.eventsHint']}>
          <div className="flex flex-wrap gap-1.5">
            {EVENT_KINDS.map((kind) => {
              const on = events.includes(kind)
              return (
                <button
                  key={kind}
                  type="button"
                  aria-pressed={on}
                  onClick={() => setEvents(on ? events.filter((e) => e !== kind) : [...events, kind])}
                  className={`rounded-[6px] border px-2 py-1 font-mono text-[10px] transition-colors ${
                    on
                      ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                      : 'border-[var(--color-border-subtle)] text-[var(--color-tertiary)] hover:text-[var(--color-secondary)]'
                  }`}
                >
                  {kind}
                </button>
              )
            })}
          </div>
        </Field>
        {error ? <p className="text-[12px] text-[var(--color-danger)]">{error}</p> : null}
      </div>
    </Modal>
  )
}
