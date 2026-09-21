import { useState, type FormEvent } from 'react'
import { Plus } from 'lucide-react'
import { useCreateAgentMutation, useListAgentSkillsQuery } from '@/store/api/agents'
import { useListProvidersQuery } from '@/store/api/providers'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import {
  CredentialSection,
  IdentitySection,
  ProviderSection,
  RuntimeSection,
  type RuntimeValues,
} from './CreateAgentFields'
import { NO_FIELD_ERRORS, routeFieldError, type FieldErrors } from '@/lib/field-error'

const FORM_ID = 'create-agent-form'

/**
 * "Daftarkan Agent Baru" on the agent registry — screen 26-agent-form as a
 * modal (US-AD96), replacing the free-text form that used to sit on the page.
 *
 * The shell is the app's one `Modal` with `placement="right"` and
 * `size="panel"` (420px), which is the geometry the design draws for this
 * screen. That geometry is two props rather than a second dialog component:
 * the focus trap, the Escape handler, the scroll lock and the focus restore are
 * one contract and stay in one file.
 *
 * Four things are wired to the real API and none of them is hardcoded:
 *  - provider and model come from `GET /agent-catalog` (US-AD96 AC1), offered
 *    as datalists and refused by `validateCatalogModel` before the request;
 *  - `tools` is the closed set of nine primitives (AC7) and `skills` is the org
 *    library from `GET /agent-skills` (AC8);
 *  - the credential block is owner/admin only (AC4) and optional (AC3/AC5): an
 *    agent registered without a key is valid and simply not ready;
 *  - the handshake is a manual button (AC5), never fired on submit.
 *
 * Two writes, in this order and for a reason: `POST /projects/{id}/agents`
 * creates the row, and only then can the key be stored — the credential
 * endpoint resolves the agent's provider from the stored row, so it has nothing
 * to read before the create lands. A credential write that fails after a
 * successful create is reported inline and the agent stays registered without a
 * key, which is the state AC3 describes.
 *
 * AC1 makes registering a Member action, so the trigger hides below `member`.
 * The server enforces the same minimum through `requireRole`, so hiding it is a
 * UX decision and never the security boundary.
 */
export function CreateAgentForm({
  projects,
  activeProjectID,
}: {
  projects: { id: string; name: string }[]
  /** The project the registry is showing; preselected in the form. */
  activeProjectID: string
}) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>(NO_FIELD_ERRORS)
  const [busy, setBusy] = useState(false)

  const [createAgent] = useCreateAgentMutation()
  // The skill list is only needed while the modal is open, so a closed form
  // costs no request on a registry page the operator may never register from.
  const { data: skills } = useListAgentSkillsQuery(undefined, { skip: !open })
  // US-AD109 AC6: the endpoint and the credential belong to the provider now, so
  // the form offers the registry instead of a base URL field. Fetched lazily for
  // the same reason the skill list is.
  const { data: providers } = useListProvidersQuery(undefined, { skip: !open })
  const canCreate = useCanAct('member')

  const [runtime, setRuntime] = useState<RuntimeValues>({
    reasoningEffort: 'medium',
    maxRuntimeSeconds: 14400,
    maxAttempts: 3,
    retryPolicy: 'transient_only',
  })
  // Defaults to the workspace default provider when there is one (AC9), and to
  // none otherwise. It used to default to the literal 'openai_compatible',
  // which is a protocol rather than a provider and stopped being selectable
  // once the registry became the source of the list.
  const [providerID, setProviderID] = useState('')
  const [model, setModel] = useState('')

  if (!canCreate || projects.length === 0) return null

  const registry = providers ?? []
  // AC9: a workspace default provider is preselected; without one the operator
  // picks from the list.
  const chosenProviderID = providerID || registry.find((entry) => entry.is_default)?.id || ''

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const chosenModel = String(form.get('model') ?? '')
    setError(null)
    setFieldErrors(NO_FIELD_ERRORS)

    const pickedProviderID = String(form.get('providerID') ?? '').trim()

    // A workspace with no provider cannot register an agent: `provider` and
    // `base_url` are derived from the provider (AC6), and the server refuses a
    // request that carries neither (measured: 400 "invalid input"). Failing here
    // keeps the refusal on the section that can fix it, instead of letting the
    // server's generic message land on the name field.
    if (!pickedProviderID && registry.length === 0) {
      setError(t['agents.create.noProvider'])
      return
    }

    setBusy(true)
    try {
      const row = (await createAgent({
        projectID: String(form.get('projectID') ?? ''),
        name: String(form.get('name') ?? ''),
        // The server derives `provider` and `base_url` from this (AC6), which is
        // why neither is sent: the agent must not carry its own copy of the
        // endpoint. An empty id is a real choice — the deployment default.
        providerID: pickedProviderID || undefined,
        model: chosenModel,
        reasoningEffort: runtime.reasoningEffort,
        maxRuntimeSeconds: runtime.maxRuntimeSeconds,
        retryPolicy: runtime.retryPolicy,
        maxAttempts: runtime.maxAttempts,
        tools: form.getAll('tools').map(String),
        skills: form.getAll('skills').map(String),
      }).unwrap()) as { id?: string; name?: string }

      // US-AD109 AC6: the provider owns the endpoint and the credential, so the
      // create is the whole flow — one write and the form is done. The agent
      // must have a provider to get here, which the guard above enforces.
      void row
      close()
    } catch (rejection) {
      // A message the server did not tie to a field stays at form level rather
      // than being dropped or guessed onto the wrong input.
      const routed = routeFieldError(rejection, t['agents.create.failed'])
      setFieldErrors(routed)
      setError(routed.form)
    } finally {
      setBusy(false)
    }
  }

  function close() {
    setOpen(false)
    setError(null)
    setFieldErrors(NO_FIELD_ERRORS)
    setModel('')
  }

  return (
    <>
      <Button variant="primary" size="md" className="gap-1.5 px-3" onClick={() => setOpen(true)}>
        <Plus size={14} />
        {t['agents.new']}
      </Button>

      <Modal
        open={open}
        onClose={close}
        title={t['agents.create.title']}
        description={t['agents.create.description']}
        /* The design draws this form as a 420px aside, and at that width the
           model name is clipped inside its own field — the operator cannot read
           back what they are about to register. Widened to the `lg` step rather
           than inventing a size: the credential panel keeps its measured 420px
           (27-agent-provider-key), this one only needs room to read. */
        size="lg"
        placement="right"
        footer={
          <div className="flex w-full items-center justify-between gap-2">
            <Button variant="ghost" size="sm" onClick={close}>
              {t['action.cancel']}
            </Button>
            <Button type="submit" form={FORM_ID} variant="primary" size="sm" disabled={busy}>
              {busy ? t['agents.create.pending'] : t['agents.create.submit']}
            </Button>
          </div>
        }
      >
        <form id={FORM_ID} onSubmit={submit} className="flex flex-col gap-4">
          <IdentitySection defaultProject={activeProjectID} errors={fieldErrors} />

          <ProviderSection
            providerID={chosenProviderID}
            onProviderChange={(next) => {
              setProviderID(next)
              // US-AD109 AC10: the previous provider's model need not exist on
              // the new one, so the model is cleared rather than carried over.
              setModel('')
              setError(null)
              setFieldErrors(NO_FIELD_ERRORS)
            }}
            providers={registry}
            model={model}
            onModelChange={(next) => {
              setModel(next)
              setError(null)
              setFieldErrors(NO_FIELD_ERRORS)
            }}
            errors={fieldErrors}
          />

          <CredentialSection />

          <RuntimeSection
            skills={skills}
            runtime={runtime}
            onRuntimeChange={(next) => setRuntime((current) => ({ ...current, ...next }))}
          />

          {/* US-AD96 AC1 refusal and the create failure. One inline surface: the
              modal is what caused them, and the toast listener deliberately
              refuses to duplicate a write's message. */}
          {error ? (
            <p role="alert" className="text-[11px] leading-snug text-[var(--color-danger)]">
              {error}
            </p>
          ) : null}
        </form>
      </Modal>
    </>
  )
}
