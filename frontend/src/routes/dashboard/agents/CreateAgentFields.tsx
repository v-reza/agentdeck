import type { AgentCatalog, AgentSkill } from '@/store/api/agents'
import type { Provider } from '@/store/api/providers'
import { Link, useParams } from 'react-router-dom'
import { Field, FieldError, Input } from '@/components/ui/input'
import { Combobox } from '@/components/ui/combobox'
import { useT } from '@/hooks/use-t'
import { REASONING_EFFORTS, RETRY_POLICIES, TOOL_SET } from './AgentDetailOptions'
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
 * The provider dropdown lists the workspace registry (US-AD109) and the model
 * list is the one that provider fetched (AC7). The endpoint is rendered as text
 * under the provider rather than as a field: the provider owns it, and a second
 * place to edit it would be a second source of truth for it.
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
  providerID,
  onProviderChange,
  providers,
  model,
  onModelChange,
  errors,
}: {
  /** The workspace provider this agent runs on. Empty means none. */
  providerID: string
  onProviderChange: (next: string) => void
  /** The workspace registry (US-AD109). Drives both dropdowns. */
  providers: Provider[]
  model: string
  onModelChange: (next: string) => void
  errors: FieldErrors
}) {
  const t = useT()
  const { orgID = '' } = useParams<{ orgID: string }>()
  // US-AD109 AC6/AC10: the provider owns the endpoint and the credential, and
  // the model list comes from the provider rather than from a probe the operator
  // has to run by hand. The endpoint field is gone on purpose — an agent that
  // carried its own copy of the base URL is what the registry exists to remove.
  const selected = providers.find((entry) => entry.id === providerID)
  const providerChoices = providers.map((entry) => ({ value: entry.id, label: entry.name }))
  // The model list is whatever the provider last fetched (AC7), plus the current
  // value so a re-render cannot blank a model already chosen.
  const modelChoices = [...new Set([model, ...(selected?.models ?? [])].filter(Boolean))]

  // An empty registry is not a form with an empty dropdown: `provider` and
  // `base_url` are both derived from the provider now, so there is nothing the
  // operator could type here that the server would accept. Measured against a
  // fresh workspace: POST with no provider and no base_url is a 400, and a
  // workspace that has never created a provider has nothing to pick. The
  // section therefore points at the page that fixes it instead of offering two
  // fields whose every value is refused.
  if (providers.length === 0) {
    return (
      <section className="flex flex-col gap-3 rounded-[8px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] p-3">
        <SectionLabel text={t['agents.create.section.provider']} hint={t['agents.create.fromRegistry']} />
        <p className="text-[11px] leading-snug text-[var(--color-tertiary)]">{t['agents.create.noProvider']}</p>
        <Link
          to={`/app/${orgID}/settings/providers`}
          className="self-start font-mono text-[11px] text-[var(--color-accent)] underline-offset-2 hover:underline"
        >
          {t['agents.create.noProviderCta']}
        </Link>
      </section>
    )
  }

  return (
    <section className="flex flex-col gap-3 rounded-[8px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] p-3">
      <SectionLabel text={t['agents.create.section.provider']} hint={t['agents.create.fromRegistry']} />

      <Field label={t['agents.field.provider']}>
        <Combobox
          name="providerID"
          label={t['agents.field.provider']}
          value={providerID}
          onChange={onProviderChange}
          options={providerChoices}
          invalid={Boolean(errors.byField['providerID'])}
        />
        <FieldError>{errors.byField['providerID']}</FieldError>
      </Field>

      {/* The endpoint is shown, never typed: it belongs to the provider, and a
          second place to edit it would be a second source of truth for it. */}
      {selected ? (
        <p className="truncate font-mono text-[10.5px] text-[var(--color-tertiary)]" data-testid="agent-form-base-url">
          {selected.base_url}
        </p>
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

/**
 * The credential block, in its registry form: a statement, not an input.
 *
 * US-AD109 AC6 moves the credential onto the provider, and the register form
 * always has one — a workspace with no provider cannot register an agent at all
 * (see `ProviderSection`). So the per-agent key field US-AD86 used to draw here
 * is unreachable by construction, and keeping it would be a field whose value
 * the server ignores. The key is entered once, on the Provider page.
 */
export function CredentialSection() {
  const t = useT()
  return (
    <section className="flex flex-col gap-3 rounded-[8px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] p-3">
      <SectionLabel text={t['agents.create.section.credential']} hint={t['agents.create.credentialFromProvider']} />
      <p className="text-[11px] leading-snug text-[var(--color-tertiary)]">
        {t['agents.create.credentialViaProvider']}
      </p>
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
