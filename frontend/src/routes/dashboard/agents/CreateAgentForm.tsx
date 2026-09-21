import { useState, type FormEvent } from 'react'
import { Plus } from 'lucide-react'
import {
  useCreateAgentMutation,
  useListAgentSkillsQuery,
  usePutProviderKeyMutation,
  useProbeProviderModelsMutation,
} from '@/store/api/agents'
import { useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import { AgentProviderKeyPanel } from '@/components/agents/AgentProviderKeyPanel'
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
  const [created, setCreated] = useState<{ id: string; name: string } | null>(null)
  const [hasKey, setHasKey] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>(NO_FIELD_ERRORS)
  const [probedModels, setProbedModels] = useState<string[]>([])
  const [probeNote, setProbeNote] = useState('')
  const [busy, setBusy] = useState(false)

  const [createAgent] = useCreateAgentMutation()
  const [putProviderKey] = usePutProviderKeyMutation()
  const [probeProviderModels, { isLoading: probing }] = useProbeProviderModelsMutation()
  // The skill list is only needed while the modal is open, so a closed form
  // costs no request on a registry page the operator may never register from.
  // The price catalog is no longer fetched: the register form is BYO-only, so
  // its model list comes from the probe, not from our table (DECISIONS 6A.F).
  const { data: skills } = useListAgentSkillsQuery(undefined, { skip: !open })
  const canCreate = useCanAct('member')
  const canManageKey = useCanAct('admin')

  const [runtime, setRuntime] = useState<RuntimeValues>({
    reasoningEffort: 'medium',
    maxRuntimeSeconds: 14400,
    maxAttempts: 3,
    retryPolicy: 'transient_only',
  })
  // Defaults to the only provider this form offers. It used to default to
  // `openai`, which then leaked into the choice list as a second entry and let
  // the operator pick a provider whose model list this form cannot populate.
  const [provider, setProvider] = useState('openai_compatible')
  const [model, setModel] = useState('')

  if (!canCreate || projects.length === 0) return null

  /**
   * One submit path, driven from React rather than a `form action`.
   *
   * US-AD96 AC1 refuses an off-catalog combination *before* the request, and a
   * `form action` cannot do that. React 19 runs the action on submit even when
   * `onSubmit` calls `preventDefault` — calling it is the only way to stop the
   * action, so there is no second handler left to refuse from. Here refusing is
   * an early `return`, and the request happens only once the combination is one
   * the catalog prices.
   */
  /**
   * Pulls the model list from the operator's own endpoint (US-AD106 AC2).
   *
   * The endpoint and the key are read out of the form at click time rather than
   * mirrored into state: the key field is a password, and copying it on every
   * keystroke would leave a second live copy of the credential in React state
   * for no gain. The probe stores nothing server-side either.
   */
  async function fetchModels() {
    const form = document.getElementById(FORM_ID) as HTMLFormElement | null
    if (!form) return
    const data = new FormData(form)
    const baseUrl = String(data.get('baseUrl') ?? '').trim()
    const apiKey = String(data.get('apiKey') ?? '').trim()
    setProbeNote('')
    if (!baseUrl || !apiKey) {
      setProbeNote(t['agents.create.fetchNeedsBase'])
      return
    }
    try {
      const answer = await probeProviderModels({ baseUrl, apiKey }).unwrap()
      setProbedModels(answer.models)
      setProbeNote(`${answer.models.length} ${t['agents.create.fetchOk']}`)
    } catch (rejection) {
      // A probe failure is not a form failure: the agent can still be registered
      // and the model typed by hand, so this never blocks the submit.
      setProbeNote(routeFieldError(rejection).form ?? '')
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const chosenProvider = String(form.get('provider') ?? '')
    const chosenModel = String(form.get('model') ?? '')
    setError(null)
    setFieldErrors(NO_FIELD_ERRORS)

    // US-AD96 AC1's catalog refusal is deliberately gone from this form: the
    // register flow is BYO-only (DECISIONS 6A.F), so the model list comes from
    // the operator's own endpoint and there is no catalog of ours to check
    // against. `validateCatalogModel` stays exported for the detail screen.
    // The server still gates provider and model (US-AD67), so this is a UX
    // change, not a hole.

    setBusy(true)
    try {
      const row = (await createAgent({
        projectID: String(form.get('projectID') ?? ''),
        name: String(form.get('name') ?? ''),
        provider: chosenProvider,
        model: chosenModel,
        // Only meaningful for the BYO provider, and the server refuses a
        // base_url on any other one. The field is not even rendered then, so
        // this reads empty and is dropped by the slice.
        baseURL: String(form.get('baseUrl') ?? '').trim() || undefined,
        reasoningEffort: runtime.reasoningEffort,
        maxRuntimeSeconds: runtime.maxRuntimeSeconds,
        retryPolicy: runtime.retryPolicy,
        maxAttempts: runtime.maxAttempts,
        tools: form.getAll('tools').map(String),
        skills: form.getAll('skills').map(String),
      }).unwrap()) as { id?: string; name?: string }

      if (!row?.id) return
      setCreated({ id: row.id, name: row.name ?? '' })
      setProvider(chosenProvider)
      setModel(chosenModel)

      const key = String(form.get('apiKey') ?? '').trim()
      if (!key) return
      // The key is stored in the same submit because the credential endpoint
      // needs the row to exist first. A failure is reported without undoing the
      // create: the agent is registered, just not ready (AC3).
      try {
        const answer = await putProviderKey({ id: row.id, apiKey: key }).unwrap()
        setHasKey(answer.has_provider_key)
      } catch (rejection) {
        // The agent is already registered, so this failure belongs to the key
        // field alone — not to the form, and never to the create.
        setFieldErrors(routeFieldError(rejection, t['agents.key.failed']))
      }
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
    setCreated(null)
    setHasKey(false)
    setPanelOpen(false)
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
            provider={provider}
            onProviderChange={(next) => {
              setProvider(next)
              setModel('')
              setError(null)
              setFieldErrors(NO_FIELD_ERRORS)
              setProbedModels([])
            }}
            model={model}
            onModelChange={(next) => {
              setModel(next)
              setError(null)
              setFieldErrors(NO_FIELD_ERRORS)
            }}
            probedModels={probedModels}
            onFetchModels={fetchModels}
            fetching={probing}
            fetchNote={probeNote}
            canFetch={canManageKey}
            errors={fieldErrors}
          />

          <CredentialSection allowed={canManageKey} agentID={created?.id} errors={fieldErrors} />

          <RuntimeSection
            skills={skills}
            runtime={runtime}
            onRuntimeChange={(next) => setRuntime((current) => ({ ...current, ...next }))}
          />

          {/* US-AD96 AC1 refusal, and the create / credential failures. One
              inline surface: the modal is what caused them, and the toast
              listener deliberately refuses to duplicate a write's message. */}
          {error ? (
            <p role="alert" className="text-[11px] leading-snug text-[var(--color-danger)]">
              {error}
            </p>
          ) : null}

          {/* Once the row exists the full 420px panel is available — the same
              component the detail page uses to rotate and revoke. */}
          {created && canManageKey ? (
            <button
              type="button"
              data-testid="open-provider-key-panel"
              onClick={() => setPanelOpen(true)}
              className="self-start font-mono text-[11px] text-[var(--color-accent)] underline-offset-2 hover:underline"
            >
              {t['agents.key.open']}
            </button>
          ) : null}
        </form>
      </Modal>

      <AgentProviderKeyPanel
        open={panelOpen}
        onClose={() => setPanelOpen(false)}
        agent={
          created
            ? {
                id: created.id,
                name: created.name,
                provider,
                model,
                has_provider_key: hasKey,
              }
            : undefined
        }
        onSaved={(next) => {
          setHasKey(next)
          setError(null)
        }}
      />
    </>
  )
}
