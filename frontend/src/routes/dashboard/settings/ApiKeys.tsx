import { useState } from 'react'
import { useAppSelector } from '@/store/hooks'
import { Check, Copy, KeyRound, Plus, ShieldOff, Trash2 } from 'lucide-react'
import {
  useCreateAPIKeyMutation,
  useDeleteAPIKeyMutation,
  useListAPIKeysQuery,
  useRevokeAPIKeyMutation,
  type CreatedAPIKey,
} from '@/store/api/api-keys'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState, Panel } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Field, FieldError, Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'
import { SkeletonRows } from '@/components/ui/skeleton'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { formatDateTime } from '@/lib/format'

/**
 * API keys — `design/stitch-output/v2/39-api-keys.html`, US-AD06.
 *
 * The screen this replaces said "API keys are not available yet" and claimed the
 * list/create endpoints would arrive with the agent credential work. They have
 * been live since F10; the stub was simply wrong. This is the real screen.
 *
 * Three columns in the mockup are deliberately absent, because the API has no
 * field for them:
 *
 *  - **HASH** (`sha256:7f4d...31e2`). `apiKeyResponse` carries no hash, and that
 *    is the point of storing one: the server keeps the digest so it never has to
 *    keep the token. There is nothing to display here and no endpoint that would
 *    hand it over.
 *  - **ROLE / SCOPE** (`admin:write`, `agent:exec`). Keys have no per-key scope
 *    in this schema — the response is id, name, prefix, last_used_at,
 *    revoked_at, created_at. Inventing a scope column would show permissions the
 *    server does not enforce.
 *  - **BIAYA HARI INI** (`$3.140`). Nothing ties a ledger entry to an API key:
 *    `ledger_entries` has no key column (checked in `internal/migrate`, all
 *    migrations) and no query joins one. A cost-per-key column would be invented.
 *
 * The scope that *does* exist is the owner. `APIKeys(ctx, orgID, userID)` lists
 * the caller's own keys, so the heading says "yours" rather than "the
 * workspace's" — a member who is not an admin sees only what they made, and
 * another member's key answers 404 rather than 403. Saying "workspace" there
 * would describe a set the endpoint cannot return.
 */
export function ApiKeys() {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const canAct = useCanAct('member')
  const { data, isLoading } = useListAPIKeysQuery()
  const [revoke, revokeState] = useRevokeAPIKeyMutation()
  const [remove, removeState] = useDeleteAPIKeyMutation()
  const [creating, setCreating] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  const rows = data ?? []
  const active = rows.filter((k) => k.revoked_at === null).length

  return (
    <>
      <WorkspaceTopbar
        title={t['apiKeys.title']}
        path="/settings/api-keys"
        subtitle={rows.length > 0 ? t['apiKeys.count'].replace('{count}', String(rows.length)) : undefined}
        right={
          canAct ? (
            <Button size="sm" data-testid="api-key-create" onClick={() => setCreating(true)}>
              <Plus size={14} />
              {t['apiKeys.create']}
            </Button>
          ) : null
        }
      />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {!canAct ? (
          <div
            role="status"
            data-testid="api-keys-viewer-note"
            className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-3.5 py-2 text-[11px] text-[var(--color-secondary)]"
          >
            {t['apiKeys.needsMember']}
          </div>
        ) : null}

        {/* The one rule a user cannot learn by looking at the list: the token is
            gone after this moment. It sits above the table, not in a tooltip. */}
        <Panel>
          <div className="flex items-start gap-2.5 p-3">
            <KeyRound size={15} className="mt-0.5 shrink-0 text-[var(--color-tertiary)]" />
            <div className="flex flex-col gap-0.5">
              <p className="text-[12px] font-semibold text-[var(--color-primary)]">{t['apiKeys.onceTitle']}</p>
              <p className="text-[12px] text-[var(--color-secondary)]">{t['apiKeys.onceBody']}</p>
              <p className="text-[11px] text-[var(--color-tertiary)]">{t['apiKeys.scopeNote']}</p>
            </div>
          </div>
        </Panel>

        {isLoading ? (
          <SkeletonRows rows={3} columns={4} />
        ) : rows.length === 0 ? (
          <div data-testid="api-keys-empty" className="flex flex-col items-center gap-3">
            <EmptyState title={t['apiKeys.emptyTitle']} hint={t['apiKeys.emptyHint']} />
            {canAct ? (
              <Button
                size="sm"
                variant="secondary"
                data-testid="api-key-create-empty"
                onClick={() => setCreating(true)}
              >
                {t['apiKeys.create']}
              </Button>
            ) : null}
          </div>
        ) : (
          <Panel>
            <div className="flex items-center justify-between border-b border-[var(--color-border-subtle)] px-3 py-2">
              <span className="text-[11px] font-semibold tracking-wide text-[var(--color-secondary)] uppercase">
                {t['apiKeys.tableTitle']}
              </span>
              <span className="font-mono text-[11px] text-[var(--color-tertiary)]" data-testid="api-keys-active">
                {t['apiKeys.activeCount'].replace('{count}', String(active))}
              </span>
            </div>
            <table className="w-full text-left text-[12px]">
              <thead>
                <tr className="border-b border-[var(--color-border-subtle)] text-[10px] tracking-wider text-[var(--color-tertiary)] uppercase">
                  <th className="px-3 py-2 font-semibold">{t['apiKeys.colName']}</th>
                  <th className="px-3 py-2 font-semibold">{t['apiKeys.colPrefix']}</th>
                  <th className="px-3 py-2 font-semibold">{t['apiKeys.colLastUsed']}</th>
                  <th className="px-3 py-2 font-semibold">{t['apiKeys.colStatus']}</th>
                  <th className="px-3 py-2 font-semibold">{t['apiKeys.colActions']}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((key) => {
                  const revoked = key.revoked_at !== null
                  return (
                    <tr
                      key={key.id}
                      data-testid="api-key-row"
                      data-key-id={key.id}
                      data-key-prefix={key.prefix}
                      data-revoked={revoked ? 'true' : 'false'}
                      className="border-b border-[var(--color-border-subtle)] last:border-0"
                    >
                      <td className="px-3 py-2 font-medium text-[var(--color-primary)]">{key.name}</td>
                      <td className="px-3 py-2 font-mono text-[var(--color-secondary)]">{key.prefix}</td>
                      <td className="px-3 py-2 text-[var(--color-secondary)]">
                        {key.last_used_at ? formatDateTime(key.last_used_at, lang) : t['apiKeys.neverUsed']}
                      </td>
                      <td className="px-3 py-2">
                        <span
                          data-testid="api-key-status"
                          data-status={revoked ? 'revoked' : 'active'}
                          className={
                            revoked ? 'text-[var(--color-tertiary)] line-through' : 'text-[var(--color-primary)]'
                          }
                        >
                          {revoked ? t['apiKeys.statusRevoked'] : t['apiKeys.statusActive']}
                        </span>
                      </td>
                      <td className="px-3 py-2">
                        <div className="flex items-center gap-1">
                          {!revoked ? (
                            <Button
                              size="sm"
                              variant="ghost"
                              data-testid="api-key-revoke"
                              disabled={revokeState.isLoading}
                              onClick={() => void revoke(key.id)}
                            >
                              <ShieldOff size={13} />
                              {t['apiKeys.revoke']}
                            </Button>
                          ) : null}
                          <Button
                            size="sm"
                            variant="ghost"
                            data-testid="api-key-delete"
                            disabled={removeState.isLoading}
                            onClick={() => setConfirmDelete(key.id)}
                          >
                            <Trash2 size={13} />
                            {t['apiKeys.delete']}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </Panel>
        )}
      </div>

      <CreateKeyDialog open={creating} onClose={() => setCreating(false)} />
      <ConfirmDeleteDialog
        id={confirmDelete}
        onClose={() => setConfirmDelete(null)}
        onConfirm={(id) => {
          void remove(id)
          setConfirmDelete(null)
        }}
      />
    </>
  )
}

/**
 * Create dialog, then the single sighting of the token.
 *
 * The token lives in this component's state and nowhere else — not in the RTK
 * Query cache (the create response is not kept), not in localStorage. Closing the
 * dialog drops it, which is the same promise the server makes.
 */
function CreateKeyDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT()
  const [name, setName] = useState('')
  const [created, setCreated] = useState<CreatedAPIKey | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [create, { isLoading }] = useCreateAPIKeyMutation()

  const close = () => {
    setName('')
    setCreated(null)
    setError(null)
    setCopied(false)
    onClose()
  }

  const submit = async () => {
    setError(null)
    // Mirrors `maxAPIKeyNameLen` (internal/auth/apikey.go): 1-64 after trimming.
    const trimmed = name.trim()
    if (trimmed.length === 0 || trimmed.length > 64) {
      setError(t['apiKeys.nameInvalid'])
      return
    }
    try {
      const result = await create({ name: trimmed }).unwrap()
      setCreated(result)
    } catch {
      setError(t['apiKeys.createFailed'])
    }
  }

  const copy = async () => {
    if (!created) return
    try {
      await navigator.clipboard.writeText(created.key)
      setCopied(true)
    } catch {
      // Clipboard can be denied by the browser. The token is still on screen to
      // select by hand, so this is not worth an error state.
      setCopied(false)
    }
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title={created ? t['apiKeys.createdTitle'] : t['apiKeys.create']}
      description={created ? undefined : t['apiKeys.createHint']}
      size="md"
      footer={
        created ? (
          <Button size="sm" data-testid="api-key-done" onClick={close}>
            {t['apiKeys.done']}
          </Button>
        ) : (
          <div className="flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={close}>
              {t['action.cancel']}
            </Button>
            <Button size="sm" data-testid="api-key-submit" disabled={isLoading} onClick={() => void submit()}>
              {t['apiKeys.create']}
            </Button>
          </div>
        )
      }
    >
      {created ? (
        <div className="flex flex-col gap-3">
          <p className="text-[12px] text-[var(--color-secondary)]">{t['apiKeys.createdBody']}</p>
          <div className="flex items-center gap-2">
            <code
              data-testid="api-key-token"
              className="min-w-0 flex-1 overflow-x-auto rounded border border-[var(--color-border-standard)] bg-[var(--color-surface-raised)] px-2.5 py-2 font-mono text-[12px] break-all"
            >
              {created.key}
            </code>
            <Button size="sm" variant="secondary" onClick={() => void copy()}>
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {copied ? t['apiKeys.copied'] : t['apiKeys.copy']}
            </Button>
          </div>
          <p className="text-[11px] text-[var(--color-tertiary)]">{t['apiKeys.createdWarning']}</p>
        </div>
      ) : (
        <Field label={t['apiKeys.colName']} required>
          <Input
            data-testid="api-key-name"
            value={name}
            autoFocus
            maxLength={64}
            placeholder={t['apiKeys.namePlaceholder']}
            onChange={(event) => setName(event.target.value)}
          />
          <FieldError>{error}</FieldError>
        </Field>
      )}
    </Modal>
  )
}

function ConfirmDeleteDialog({
  id,
  onClose,
  onConfirm,
}: {
  id: string | null
  onClose: () => void
  onConfirm: (id: string) => void
}) {
  const t = useT()
  return (
    <Modal
      open={id !== null}
      onClose={onClose}
      title={t['apiKeys.deleteTitle']}
      size="sm"
      footer={
        <div className="flex items-center gap-2">
          <Button size="sm" variant="secondary" onClick={onClose}>
            {t['action.cancel']}
          </Button>
          <Button size="sm" variant="danger" data-testid="api-key-delete-confirm" onClick={() => id && onConfirm(id)}>
            {t['apiKeys.delete']}
          </Button>
        </div>
      }
    >
      <p className="text-[12px] text-[var(--color-secondary)]">{t['apiKeys.deleteBody']}</p>
    </Modal>
  )
}
