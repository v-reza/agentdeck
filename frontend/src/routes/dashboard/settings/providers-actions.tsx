import { useState } from 'react'
import { CheckCircle2, RefreshCw, Zap } from 'lucide-react'
import {
  useCreateProviderMutation,
  useDeleteProviderMutation,
  useRefreshProviderModelsMutation,
  useUpdateProviderMutation,
  useVerifyProviderMutation,
  type CreateProviderArgs,
  type Provider,
  type ProviderInUse,
  type UpdateProviderArgs,
} from '@/store/api/providers'
import { useActionForm, describeError, type MutationTrigger } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { useAppDispatch } from '@/store/hooks'
import { pushToast } from '@/store/slices/toastSlice'
import { interpolate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'

/**
 * US-AD109's three write paths, all as modals.
 *
 * The design (`47-providers.html`) draws the list and the 420px detail drawer
 * but no dialog for create, edit or delete — so, as with the members roster, the
 * placement is a UX decision built from the design system's own parts rather
 * than invented chrome.
 *
 * Two shape decisions worth stating:
 *
 *  1. **The credential field is never pre-filled.** AC2 makes it write-only; the
 *     server cannot return it, so an edit dialog that showed a value would be
 *     showing a lie. It starts empty and empty means "keep the stored key" —
 *     which is why the create and edit cases carry different hints rather than
 *     sharing one placeholder.
 *  2. **409 names the blocking agents (AC5).** The refusal is not a generic
 *     failure toast: the operator needs to know *which* agents to repoint, and
 *     that list is in the response body. It is rendered in the dialog, next to
 *     the control that caused it, per the `use-action-form` rule.
 */
const PROTOCOLS = ['openai_compatible', 'anthropic', 'google']

/** Create carries no `id`; edit requires one. Discriminating on it keeps the
 *  two mutations behind a single trigger, so the dialog has one form and one
 *  pending state rather than two of each. */
type ProviderFormArgs = CreateProviderArgs | UpdateProviderArgs

/** The create/edit dialog. `provider` absent means create. */
export function ProviderFormDialog({
  open,
  onClose,
  provider,
  canManage,
  onRequestDelete,
}: {
  open: boolean
  onClose: () => void
  provider?: Provider
  canManage: boolean
  /** Present only on edit. Delete is a dialog action, not a stray page link. */
  onRequestDelete?: () => void
}) {
  const t = useT()
  const [createProvider] = useCreateProviderMutation()
  const [updateProvider] = useUpdateProviderMutation()
  const isEdit = Boolean(provider)

  const [protocol, setProtocol] = useState(provider?.protocol ?? PROTOCOLS[0])
  const [isDefault, setIsDefault] = useState(provider?.is_default ?? false)

  // Both mutations share one form and one pending state. The trigger is
  // narrowed here rather than duplicated: `useActionForm` only needs
  // `(arg) => { unwrap() }`, and the two endpoints differ solely in whether
  // `id` is present, which the builder below decides.
  const submit = (isEdit ? updateProvider : createProvider) as MutationTrigger<ProviderFormArgs>

  const [state, formAction, isPending] = useActionForm(
    submit,
    (form): ProviderFormArgs => {
      const apiKey = String(form.get('api_key') ?? '').trim()
      const base = {
        name: String(form.get('name') ?? '').trim(),
        protocol,
        base_url: String(form.get('base_url') ?? '').trim(),
        isDefault,
      }
      // An empty field means "leave the stored credential alone", so it is
      // omitted rather than sent as "": the server would treat "" as a real
      // value and overwrite the key with nothing.
      if (isEdit && provider) {
        return apiKey ? { id: provider.id, ...base, apiKey } : { id: provider.id, ...base }
      }
      return apiKey ? { ...base, apiKey } : base
    },
    onClose,
  )

  if (!canManage) return null

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={isEdit ? t['providers.form.edit'] : t['providers.form.create']}
      description={isEdit ? provider?.name : undefined}
    >
      <form action={formAction} id="provider-form" className="flex flex-col gap-3.5">
        <Field label={t['providers.form.name']}>
          <Input name="name" required defaultValue={provider?.name} className="h-9 text-[13px]" />
        </Field>

        <Field label={t['providers.form.protocol']}>
          <Combobox
            name="protocol"
            label={t['providers.form.protocol']}
            value={protocol}
            onChange={setProtocol}
            options={PROTOCOLS.map((p) => ({ value: p, label: p }))}
            searchThreshold={99}
          />
        </Field>

        <Field label={t['providers.form.baseUrl']}>
          <Input
            name="base_url"
            required
            defaultValue={provider?.base_url}
            placeholder="https://api.example.com/v1"
            className="h-9 font-mono text-[12px]"
          />
        </Field>

        <Field
          label={t['providers.form.apiKey']}
          hint={isEdit ? t['providers.form.keepKey'] : t['providers.form.apiKeyOptional']}
        >
          <Input name="api_key" type="password" autoComplete="off" className="h-9 font-mono text-[12px]" />
        </Field>

        <label className="flex items-center gap-2 text-[12px] text-[var(--color-secondary)]">
          <input
            type="checkbox"
            checked={isDefault}
            onChange={(event) => setIsDefault(event.target.checked)}
            className="h-3.5 w-3.5 accent-[var(--color-accent)]"
          />
          {t['providers.form.isDefault']}
        </label>

        {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
      </form>

      <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
        {onRequestDelete ? (
          <Button variant="ghost" size="sm" className="mr-auto text-[var(--color-danger)]" onClick={onRequestDelete}>
            {t['providers.delete.title']}
          </Button>
        ) : null}
        <Button variant="ghost" size="sm" onClick={onClose}>
          {t['action.cancel']}
        </Button>
        <Button type="submit" form="provider-form" variant="primary" size="sm" disabled={isPending}>
          {isPending ? t['providers.form.pending'] : t['providers.form.submit']}
        </Button>
      </div>
    </Modal>
  )
}

/**
 * The row's destructive action.
 *
 * AC5's 409 is not treated as a dead end: the body carries the agents pinning
 * the provider, so the dialog shows them and the operator knows what to repoint.
 * A bare "delete failed" would leave them to discover it by trial.
 *
 * This dialog reads the mutation result directly instead of going through
 * `useActionForm`. That hook flattens a rejection to a string (`describeError`),
 * which is right for a form that renders one sentence — but the 409 body is
 * structured, and stringifying it discards the agent list, which is the only
 * reason AC5's refusal is actionable. `FetchBaseQueryError.data` still holds the
 * parsed object, so that is what is read here.
 */
export function DeleteProviderDialog({
  open,
  onClose,
  provider,
}: {
  open: boolean
  onClose: () => void
  provider: Provider
}) {
  const t = useT()
  const [deleteProvider, deleteState] = useDeleteProviderMutation()

  const blocked = readInUse(deleteState.error)

  async function submit() {
    try {
      await deleteProvider(provider.id).unwrap()
      onClose()
    } catch {
      // The rejection is rendered below from `deleteState.error`; there is
      // nothing to do here but let the dialog stay open.
    }
  }

  return (
    <Modal open={open} onClose={onClose} size="sm" title={t['providers.delete.title']} description={provider.name}>
      <div className="flex flex-col gap-3">
        <p className="text-[12px] text-[var(--color-secondary)]">{t['providers.delete.body']}</p>
        {blocked ? (
          <div className="rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-well)] p-2.5">
            <p className="text-[12px] font-medium text-[var(--color-primary)]">{t['providers.inUse.title']}</p>
            <p className="mt-1 text-[11px] text-[var(--color-secondary)]">
              {interpolate(t['providers.inUse.body'], [
                String(blocked.total),
                blocked.agents.map((agent) => agent.name).join(', '),
              ])}
            </p>
          </div>
        ) : deleteState.isError ? (
          <p className="text-[12px] text-[var(--color-danger)]">{describeError(deleteState.error)}</p>
        ) : null}
      </div>

      <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
        <Button variant="ghost" size="sm" onClick={onClose}>
          {t['action.cancel']}
        </Button>
        <Button variant="danger" size="sm" disabled={deleteState.isLoading || Boolean(blocked)} onClick={submit}>
          {t['providers.delete.submit']}
        </Button>
      </div>
    </Modal>
  )
}

/**
 * Reads AC5's 409 body off a rejection.
 *
 * The body is `{error, agents, total}`; `data` is that parsed object, and a
 * plain-text or network failure leaves it a string, which is not this shape.
 */
function readInUse(error: unknown): ProviderInUse | null {
  const data = (error as { data?: unknown } | null | undefined)?.data
  if (!data || typeof data !== 'object') return null
  const candidate = data as Partial<ProviderInUse>
  return Array.isArray(candidate.agents) ? (candidate as ProviderInUse) : null
}

/**
 * The two per-row operations.
 *
 * Both report through a toast rather than an inline message: neither has a form
 * to render into, and `store/listeners/toast.ts` deliberately does not toast
 * mutations (a write reports itself where it happened). A row button has no
 * "where", so this is the case the listener's own doc comment points at — the
 * mutation result is read directly.
 */
export function ProviderRowActions({
  provider,
  canManage,
  onEdit,
}: {
  provider: Provider
  canManage: boolean
  onEdit: () => void
}) {
  const t = useT()
  const dispatch = useAppDispatch()
  const [verify, verifyState] = useVerifyProviderMutation()
  const [refresh, refreshState] = useRefreshProviderModelsMutation()

  if (!canManage) return null

  async function run(kind: 'verify' | 'refresh') {
    try {
      if (kind === 'verify') await verify(provider.id).unwrap()
      else await refresh(provider.id).unwrap()
      dispatch(
        pushToast({
          tone: 'success',
          title: kind === 'verify' ? t['providers.verifyOk'] : t['providers.modelsOk'],
          body: provider.name,
        }),
      )
    } catch (error) {
      dispatch(
        pushToast({
          tone: 'danger',
          title: kind === 'verify' ? t['providers.verifyFailed'] : t['providers.modelsFailed'],
          body: `${provider.name}: ${describeError(error)}`,
        }),
      )
    }
  }

  const busy = verifyState.isLoading || refreshState.isLoading

  return (
    <div className="flex items-center justify-end gap-1 whitespace-nowrap">
      <button
        type="button"
        title={t['providers.test']}
        aria-label={`${t['providers.test']} — ${provider.name}`}
        disabled={busy}
        onClick={() => run('verify')}
        className="rounded p-1 text-[var(--color-tertiary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)] disabled:opacity-40"
      >
        <Zap size={13} />
      </button>
      <button
        type="button"
        title={t['providers.refresh']}
        aria-label={`${t['providers.refresh']} — ${provider.name}`}
        disabled={busy}
        onClick={() => run('refresh')}
        className="rounded p-1 text-[var(--color-tertiary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)] disabled:opacity-40"
      >
        <RefreshCw size={13} />
      </button>
      <button
        type="button"
        onClick={onEdit}
        className="text-[11px] font-medium text-[var(--color-tertiary)] transition-colors hover:text-[var(--color-primary)]"
      >
        {t['providers.form.edit']}
      </button>
    </div>
  )
}

/** The verified/unverified badge (AC3). */
export function VerifiedBadge({ provider }: { provider: Provider }) {
  const t = useT()
  if (provider.last_verified_at) {
    return (
      <span className="inline-flex items-center gap-1 whitespace-nowrap rounded-[4px] bg-[var(--color-accent-tint)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-accent)]">
        <CheckCircle2 size={11} />
        {t['providers.verified']}
      </span>
    )
  }
  return (
    <span className="whitespace-nowrap rounded-[4px] bg-[var(--color-surface-sunken)] px-1.5 py-0.5 text-[10px] text-[var(--color-secondary)]">
      {t['providers.unverified']}
    </span>
  )
}
