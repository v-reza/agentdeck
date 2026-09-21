import type { AgentCatalog, AgentSkill } from '@/store/api/agents'
import { Field, FieldError, Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { useT } from '@/hooks/use-t'
import { ProviderKeyFields } from '@/components/agents/ProviderKeyFields'
import { CREATE_PROVIDER_CHOICES, REASONING_EFFORTS, RETRY_POLICIES, TOOL_SET } from './AgentDetailOptions'
import { type Dictionary } from '@/lib/i18n'
import { type FieldErrors } from '@/lib/field-error'

/**
 * The body of the register-agent modal (screen 26-agent-form, US-AD96).
 *
 * Split out of `CreateAgentForm.tsx` so the host keeps the trigger, the two
 * writes and the pending state while this file owns the markup — the same
 * division `AgentDetailForm.tsx` uses on the detail screen.
 *
 * Provider and model are `Combobox`es. They were `<input list=…>` + `<datalist>`,
 * which is a browser-drawn popup: it cannot carry the design tokens, so the
 * field looked like a different product from the toolbar filter next to it, and
 * at 420px the two-column grid clipped the model name. The control is stacked
 * full width here, as the design draws it, and the popup is ours.
 *
 * `base_url` appears only for the BYO provider, because `agents_base_url_chk`
 * makes that an equivalence: a built-in provider with a base_url is exactly as
 * invalid as `openai_compatible` without one.
 */

/** The catalog tiers a user may pick by name. A pattern is a rule, not a model. */
export function exactModels(catalog: AgentCatalog | undefined): string[] {
  return (catalog?.models ?? [])
    .filter((model) => model.price_source === 'catalog' || model.price_source === 'manual')
    .map((model) => model.model)
}

/**
 * US-AD96 AC1: a combination outside the price table is refused before it is
 * sent. A BYO provider is exempt — its models come from the operator's own
 * `/models`, which this client cannot enumerate without a stored base URL.
 */
export function validateCatalogModel(
  provider: string,
  model: string,
  catalog: AgentCatalog | undefined,
): keyof Dictionary | null {
  if (provider === 'openai_compatible') return null
  if (!catalog) return 'agents.create.catalogLoading'
  if (exactModels(catalog).includes(model)) return null
  return 'agents.create.modelNotInCatalog'
}

export function IdentitySection({ defaultProject, errors }: { defaultProject: string; errors: FieldErrors }) {
  const t = useT()
  return (
    <section className="flex flex-col gap-3">
      <SectionLabel text={t['agents.create.section.identity']} hint={t['agents.create.required']} />
      <input type="hidden" name="projectID" value={defaultProject} />
      <Field label={t['field.name']}>
        <Input
          name="name"
          required
          placeholder="agent-backend"
          className="font-mono"
          aria-invalid={errors.byField['name'] ? true : undefined}
        />
        <FieldError>{errors.byField['name']}</FieldError>
      </Field>
    </section>
  )
}

export function ProviderSection({
  provider,
  onProviderChange,
  model,
  onModelChange,
  probedModels,
  onFetchModels,
  fetching,
  fetchNote,
  canFetch,
  errors,
}: {
  provider: string
  onProviderChange: (next: string) => void
  model: string
  onModelChange: (next: string) => void
  /** Models the operator pulled from their own endpoint (US-AD106 AC2). */
  probedModels: string[]
  onFetchModels: () => void
  fetching: boolean
  fetchNote: string
  canFetch: boolean
  errors: FieldErrors
}) {
  const t = useT()
  // BYO only, by decision (DECISIONS 6A.F). The operator brings their own
  // endpoint and credential, so the model list cannot come from our price
  // catalog — it comes from the probe below. The other provider names stay in
  // the *detail* screen so an agent already carrying one still displays it.
  const providerChoices = [...new Set([...CREATE_PROVIDER_CHOICES, provider])]
  // Whatever the operator pulled, plus the current value so a re-render cannot
  // silently blank a model they typed.
  const modelChoices = [...new Set([model, ...probedModels].filter(Boolean))]

  return (
    <section className="flex flex-col gap-3 rounded-[8px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] p-3">
      <SectionLabel text={t['agents.create.section.provider']} hint={t['agents.create.fromEndpoint']} />

      <Field label={t['agents.field.provider']}>
        <Combobox
          name="provider"
          label={t['agents.field.provider']}
          value={provider}
          onChange={onProviderChange}
          options={providerChoices.map((choice) => ({ value: choice, label: choice }))}
          invalid={Boolean(errors.byField['provider'])}
        />
        <FieldError>{errors.byField['provider']}</FieldError>
      </Field>

      <Field label={t['agents.detail.baseUrl']}>
        <Input
          name="baseUrl"
          type="url"
          required
          placeholder="https://api.example.com/v1"
          className="font-mono"
          aria-invalid={errors.byField['baseUrl'] ? true : undefined}
        />
        <FieldError>{errors.byField['baseUrl']}</FieldError>
      </Field>

      {/* Pulling the list needs the endpoint and the key, both of which live in
          this same form, so the button reads them at click time instead of the
          form mirroring every keystroke of a password field into state. */}
      {canFetch ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-[10.5px] leading-snug text-[var(--color-quaternary)]">{fetchNote}</span>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            className="shrink-0"
            disabled={fetching}
            onClick={onFetchModels}
          >
            {fetching ? t['agents.create.fetching'] : t['agents.create.fetchModels']}
          </Button>
        </div>
      ) : null}

      <Field label={t['agents.field.model']}>
        <Combobox
          name="model"
          label={t['agents.field.model']}
          value={model}
          onChange={onModelChange}
          placeholder={t['agents.create.modelPlaceholder']}
          options={modelChoices.map((name) => ({ value: name, label: name }))}
          allowCustom
          invalid={Boolean(errors.byField['model'])}
        />
        <FieldError>{errors.byField['model']}</FieldError>
      </Field>
    </section>
  )
}

export function CredentialSection({
  allowed,
  agentID,
  errors,
}: {
  allowed: boolean
  agentID?: string
  errors: FieldErrors
}) {
  const t = useT()
  return (
    <section className="flex flex-col gap-3 rounded-[8px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] p-3">
      <SectionLabel
        text={t['agents.create.section.credential']}
        hint={allowed ? t['agents.create.adminOnly'] : t['agents.create.memberHidden']}
      />
      {allowed ? (
        <ProviderKeyFields inputName="apiKey" hasKey={false} agentID={agentID} error={errors.byField['apiKey']} />
      ) : (
        <p className="text-[11px] leading-snug text-[var(--color-tertiary)]">
          {t['agents.create.credentialRestricted']}
        </p>
      )}
    </section>
  )
}

export function RuntimeSection({
  skills,
  runtime,
  onRuntimeChange,
}: {
  skills: AgentSkill[] | undefined
  runtime: RuntimeValues
  onRuntimeChange: (next: Partial<RuntimeValues>) => void
}) {
  const t = useT()
  const library = skills ?? []

  return (
    <section className="flex flex-col gap-3">
      <SectionLabel text={t['agents.create.section.runtime']} hint={t['agents.create.optional']} />

      <div className="grid grid-cols-2 gap-3">
        <Field label={t['agents.field.reasoning']}>
          <Combobox
            name="reasoningEffort"
            label={t['agents.field.reasoning']}
            value={runtime.reasoningEffort}
            onChange={(value) => onRuntimeChange({ reasoningEffort: value })}
            options={REASONING_EFFORTS.map((option) => ({ value: option, label: option }))}
          />
        </Field>
        <Field label={t['agents.field.retry']}>
          <Combobox
            name="retryPolicy"
            label={t['agents.field.retry']}
            value={runtime.retryPolicy}
            onChange={(value) => onRuntimeChange({ retryPolicy: value })}
            options={RETRY_POLICIES.map((option) => ({ value: option, label: option }))}
          />
        </Field>
        <Field label={t['agents.field.runtime']}>
          <Input
            name="maxRuntimeSeconds"
            type="number"
            min={1}
            max={86400}
            required
            value={runtime.maxRuntimeSeconds}
            onChange={(event) => onRuntimeChange({ maxRuntimeSeconds: Number(event.target.value) })}
            className="font-mono"
          />
        </Field>
        <Field label={t['agents.field.attempts']}>
          <Input
            name="maxAttempts"
            type="number"
            min={1}
            max={10}
            required
            value={runtime.maxAttempts}
            onChange={(event) => onRuntimeChange({ maxAttempts: Number(event.target.value) })}
            className="font-mono"
          />
        </Field>
      </div>

      <ChoiceBox label={t['agents.field.tools']} hint={t['agents.field.toolsHint']}>
        {TOOL_SET.map((tool) => (
          <Check key={tool.name} name="tools" value={tool.name} label={tool.name} />
        ))}
      </ChoiceBox>

      <ChoiceBox label={t['agents.field.skills']} hint={t['agents.field.skillsHint']}>
        {library.length === 0 ? (
          <span className="font-mono text-[10.5px] text-[var(--color-quaternary)]">
            {t['agents.detail.skillsEmpty']}
          </span>
        ) : (
          library.map((skill) => <Check key={skill.id} name="skills" value={skill.slug} label={skill.slug} />)
        )}
      </ChoiceBox>
    </section>
  )
}

export interface RuntimeValues {
  reasoningEffort: string
  maxRuntimeSeconds: number
  maxAttempts: number
  retryPolicy: string
}

/** A section heading with a right-aligned monospace note, as the design draws it. */
function SectionLabel({ text, hint }: { text: string; hint: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-[10px] font-bold tracking-wider text-[var(--color-tertiary)] uppercase">{text}</span>
      <span className="shrink-0 font-mono text-[9.5px] text-[var(--color-quaternary)]">{hint}</span>
    </div>
  )
}

function ChoiceBox({ label, hint, children }: { label: string; hint: string; children: React.ReactNode }) {
  return (
    <div className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-2.5">
      <div className="flex items-center justify-between gap-3">
        <span className="text-[10px] font-semibold tracking-[0.06em] text-[var(--color-tertiary)] uppercase">
          {label}
        </span>
        <span className="shrink-0 text-[10px] text-[var(--color-quaternary)]">{hint}</span>
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1.5">{children}</div>
    </div>
  )
}

function Check({ name, value, label }: { name: string; value: string; label: string }) {
  return (
    <label className="flex items-center gap-1.5 font-mono text-[10.5px] text-[var(--color-primary)]">
      <input
        type="checkbox"
        name={name}
        value={value}
        className="h-3.5 w-3.5 rounded-[3px] border border-[var(--color-border-standard)] accent-[var(--color-accent)]"
      />
      <span>{label}</span>
    </label>
  )
}
