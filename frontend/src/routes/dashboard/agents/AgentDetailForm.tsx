import { useState } from 'react'
import type { Agent } from '@/lib/domain'
import type { AgentCatalog, AgentSkill } from '@/store/api/agents'
import { ControlBox, runtimeLabel } from './AgentDetailParts'
import { PricingCard } from './AgentPricingCard'
import { interpolate } from '@/lib/format'
import { useT } from '@/hooks/use-t'
import { Input } from '@/components/ui/input'
import { Panel } from '@/components/ui/card'

/**
 * The editable half of screen 28-agent-detail: provider/model configuration,
 * runtime parameters with the tool allow-list and the org skill list.
 *
 * Both sections are one `<form>` submitted from the topbar (`form=` on the save
 * button), so the screen has exactly one write path and one pending state. The
 * save itself is a real `PATCH` — the catalog and the skill list are only
 * *choices*; the values are committed by the user.
 *
 * Choice lists:
 *  - models come from `GET /agent-catalog`, which is the pricing table's own
 *    surface. The label carries the estimated rate, marked as an estimate.
 *  - tools are the contract's closed set of nine primitives (DECISIONS 6A.H).
 *    A name outside it is refused with 400, so the field is a closed multi-select
 *    rather than free text.
 *  - skills are the org's library (`GET /agent-skills`), which the agent may read
 *    and never write.
 */

const PROVIDER_CHOICES = ['openai', 'anthropic', 'deepseek', 'openai_compatible']

/**
 * The nine tool primitives as a closed multi-select (DECISIONS 6A.H). `bash` is
 * marked because it always goes through the approval gate.
 */
const TOOL_SET = [
  { name: 'read_file', gated: false },
  { name: 'write_file', gated: false },
  { name: 'edit_file', gated: false },
  { name: 'list_dir', gated: false },
  { name: 'search_files', gated: false },
  { name: 'bash', gated: true },
  { name: 'sql_query', gated: false },
  { name: 'http_fetch', gated: false },
  { name: 'git', gated: false },
]

/** The daemon's own bounds for max_runtime_seconds, so the field is not free-form. */
const RUNTIME_PRESETS = [300, 900, 1800, 3600, 7200, 14400, 28800, 86400]

export interface AgentFormValues {
  provider: string
  model: string
  reasoning_effort: string
  base_url: string
  max_runtime_seconds: number
  max_attempts: number
  retry_policy: string
  tools: string[]
  skills: string[]
}

export function AgentConfigSection({
  agent,
  catalog,
  values,
}: {
  agent: Agent
  catalog: AgentCatalog | undefined
  values: AgentFormValues
}) {
  const t = useT()
  const [provider, setProvider] = useState(values.provider)
  // The choice list holds only the tiers the catalog resolves exactly. Listing
  // the pattern rows as if they were models would put a glob such as `gemini-*`
  // in a dropdown as a selectable name, and it is not one.
  const models = catalog?.models ?? []
  const exact = models.filter((model) => model.price_source === 'catalog' || model.price_source === 'manual')
  const entry = exact.find((model) => model.model === values.model)
  const needsBaseURL = provider === 'openai_compatible'

  return (
    <div key={agent.id} className="grid grid-cols-2 gap-4">
      <Panel className="flex flex-col justify-between p-[14px] shadow-xs">
        <div>
          <div className="mb-3 flex items-center justify-between border-b border-[var(--color-border-subtle)] pb-2">
            <div className="flex items-center gap-2">
              <span className="h-2 w-2 rounded-full bg-[var(--color-accent)]" />
              <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['agents.detail.providerTitle']}</h2>
            </div>
            {/* The design puts a story-id chip in this slot. Story ids are
                internal vocabulary, so the slot is left empty rather than
                filled with a second copy of the rate-availability wording
                that already sits beside the provider label below. */}
          </div>

          <div className="flex flex-col gap-3">
            <ControlBox
              label={t['agents.detail.provider']}
              htmlFor="agent-provider"
              hint={<VerifiedMark show={Boolean(entry)} />}
            >
              <select
                id="agent-provider"
                name="provider"
                form="agent-detail-form"
                value={provider}
                onChange={(event) => setProvider(event.target.value)}
                className="h-8 w-full rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] px-2.5 font-mono text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
              >
                {[...new Set([...PROVIDER_CHOICES, values.provider])].map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </select>
            </ControlBox>

            <ControlBox
              label={t['agents.detail.model']}
              htmlFor="agent-model"
              hint={<SnapshotHint catalog={catalog} />}
            >
              <select
                id="agent-model"
                name="model"
                form="agent-detail-form"
                defaultValue={values.model}
                className="h-8 w-full rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] px-2.5 font-mono text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
              >
                {/* The stored model stays selectable even when the catalog has
                    dropped it: it is what the row holds, and the save would
                    otherwise silently re-model the agent. */}
                {[...new Set([values.model, ...exact.map((model) => model.model)])].map((model) => (
                  <option key={model} value={model}>
                    {model}
                  </option>
                ))}
              </select>
            </ControlBox>

            {needsBaseURL ? (
              <ControlBox
                label={t['agents.detail.baseUrl']}
                htmlFor="agent-base-url"
                hint={<span className="font-mono text-[10px] text-[var(--color-tertiary)]">https</span>}
              >
                <Input
                  id="agent-base-url"
                  name="base_url"
                  type="url"
                  required
                  defaultValue={values.base_url}
                  placeholder="https://api.example.com/v1"
                  className="h-8 rounded-[6px] bg-[var(--color-surface-page)] font-mono text-[12px]"
                />
              </ControlBox>
            ) : null}
          </div>
        </div>

        <p className="mt-3 border-t border-[var(--color-border-subtle)] pt-2 text-[10.5px] leading-snug text-[var(--color-tertiary)]">
          {needsBaseURL ? t['agents.detail.baseUrlHint'] : t['agents.detail.modelHint']}
        </p>
      </Panel>

      <PricingCard entry={entry} priceVersion={catalog?.price_version ?? 0} />
    </div>
  )
}

/** The design's green tick strip beside the provider field. */
function VerifiedMark({ show }: { show: boolean }) {
  const t = useT()
  if (!show) return null
  return (
    <span className="flex items-center gap-1 font-mono text-[10px] font-normal text-[var(--color-status-done)]">
      <svg className="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
        <polyline points="20 6 9 17 4 12" />
      </svg>
      {t['agents.detail.providerVerified']}
    </span>
  )
}

function SnapshotHint({ catalog }: { catalog: AgentCatalog | undefined }) {
  const t = useT()
  if (!catalog) return null
  return (
    <span className="font-mono text-[10px] text-[var(--color-tertiary)]">
      {interpolate(t['agents.detail.priceVersion'], [String(catalog.price_version)])}
    </span>
  )
}

export function RuntimeSection({
  agent,
  skills,
  values,
}: {
  agent: Agent
  skills: AgentSkill[] | undefined
  values: AgentFormValues
}) {
  const t = useT()
  const reasoning = [...new Set(['low', 'medium', 'high', values.reasoning_effort])]
  const policies = [...new Set(['never', 'transient_only', 'always', values.retry_policy])]
  const library = skills ?? []
  const known = new Set(library.map((skill) => skill.slug))
  // A slug the agent carries but the library no longer has is still shown: it is
  // what the row stores, and hiding it would make the screen disagree with the
  // agent it describes.
  const skillChoices = [...library.map((skill) => skill.slug), ...values.skills.filter((slug) => !known.has(slug))]

  return (
    <Panel className="p-[14px] shadow-xs">
      <div className="mb-3 flex items-center justify-between border-b border-[var(--color-border-subtle)] pb-2">
        <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['agents.detail.runtimeTitle']}</h2>
        <span className="font-mono text-[10px] text-[var(--color-tertiary)]">{t['agents.detail.runtimeSubtitle']}</span>
      </div>

      <div key={agent.id} className="grid grid-cols-3 gap-3">
        <ControlBox label={t['agents.detail.runtime']} htmlFor="agent-runtime">
          <div className="flex items-center gap-1.5">
            <select
              id="agent-runtime"
              name="max_runtime_seconds"
              form="agent-detail-form"
              defaultValue={values.max_runtime_seconds}
              className="h-8 w-full rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] px-2.5 font-mono text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
            >
              {[...new Set([...RUNTIME_PRESETS, values.max_runtime_seconds])]
                .sort((a, b) => a - b)
                .map((seconds) => (
                  <option key={seconds} value={seconds}>
                    {runtimeLabel(seconds)}{' '}
                    {interpolate(t['agents.detail.minutes'], [String(Math.round(seconds / 60))])}
                  </option>
                ))}
            </select>
          </div>
        </ControlBox>

        <ControlBox label={t['agents.detail.attempts']} htmlFor="agent-attempts">
          <Input
            id="agent-attempts"
            name="max_attempts"
            form="agent-detail-form"
            type="number"
            min={1}
            max={10}
            required
            defaultValue={values.max_attempts}
            className="h-8 rounded-[6px] bg-[var(--color-surface-page)] font-mono text-[12px]"
          />
        </ControlBox>

        <ControlBox label={t['agents.detail.reasoning']} htmlFor="agent-reasoning">
          <select
            id="agent-reasoning"
            name="reasoning_effort"
            form="agent-detail-form"
            defaultValue={values.reasoning_effort}
            className="h-8 w-full rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] px-2.5 font-mono text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
          >
            {reasoning.map((effort) => (
              <option key={effort} value={effort}>
                {effort}
              </option>
            ))}
          </select>
        </ControlBox>
      </div>

      <div className="mt-3 grid grid-cols-4 gap-3">
        <ControlBox label={t['agents.detail.retry']} htmlFor="agent-retry">
          <select
            id="agent-retry"
            name="retry_policy"
            form="agent-detail-form"
            defaultValue={values.retry_policy}
            className="h-8 w-full rounded-[6px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] px-2.5 font-mono text-[12px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none"
          >
            {policies.map((policy) => (
              <option key={policy} value={policy}>
                {policy}
              </option>
            ))}
          </select>
        </ControlBox>

        <ControlBox label={t['agents.detail.gatePolicy']} htmlFor="agent-gate">
          <div className="flex h-8 items-center gap-1.5">
            <span
              className={`h-2 w-2 rounded-full ${
                values.tools.includes('bash') ? 'bg-[var(--color-warning)]' : 'bg-[var(--color-border-strong)]'
              }`}
            />
            <span className="font-mono text-[11px] font-bold text-[var(--color-primary)]">
              {values.tools.includes('bash') ? t['agents.detail.gateGated'] : t['agents.detail.gateNone']}
            </span>
          </div>
        </ControlBox>

        <div className="col-span-2 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5">
          <div className="font-mono text-[10px] uppercase text-[var(--color-tertiary)]">
            {t['agents.detail.toolsGranted']}
          </div>
          <div className="mt-1.5">
            <ToolChecks selected={values.tools} />
          </div>
          <p className="mt-1.5 text-[10px] text-[var(--color-quaternary)]">{t['agents.detail.toolsHint']}</p>
        </div>
      </div>

      <div className="mt-3 rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5">
        <div className="flex items-center justify-between">
          <span className="font-mono text-[10px] uppercase text-[var(--color-tertiary)]">
            {t['agents.detail.skillsInUse']}
          </span>
          <span className="text-[10px] text-[var(--color-quaternary)]">{t['agents.detail.skillsHint']}</span>
        </div>
        <div className="mt-1.5">
          <SkillChecks choices={skillChoices} selected={values.skills} />
        </div>
      </div>
    </Panel>
  )
}

function ToolChecks({ selected }: { selected: string[] }) {
  const t = useT()
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
      {TOOL_SET.map((tool) => (
        <label
          key={tool.name}
          className="flex items-center gap-1.5 font-mono text-[10.5px] text-[var(--color-primary)]"
        >
          <input
            type="checkbox"
            name="tools"
            form="agent-detail-form"
            value={tool.name}
            defaultChecked={selected.includes(tool.name)}
            className="h-3.5 w-3.5 rounded-[3px] border border-[var(--color-border-standard)] accent-[var(--color-accent)]"
          />
          <span>{tool.name}</span>
          {tool.gated ? <span className="text-[var(--color-warning)]">({t['agents.detail.gatePolicy']})</span> : null}
        </label>
      ))}
    </div>
  )
}

function SkillChecks({ choices, selected }: { choices: string[]; selected: string[] }) {
  const t = useT()
  if (choices.length === 0) {
    return (
      <span className="font-mono text-[10.5px] text-[var(--color-quaternary)]">{t['agents.detail.tasksEmpty']}</span>
    )
  }
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
      {choices.map((slug) => (
        <label key={slug} className="flex items-center gap-1.5 font-mono text-[10.5px] text-[var(--color-primary)]">
          <input
            type="checkbox"
            name="skills"
            form="agent-detail-form"
            value={slug}
            defaultChecked={selected.includes(slug)}
            className="h-3.5 w-3.5 rounded-[3px] border border-[var(--color-border-standard)] accent-[var(--color-accent)]"
          />
          <span>{slug}</span>
        </label>
      ))}
    </div>
  )
}

/** Reads a form value without `String()`-ing `null` into "null". */
export function formValue(form: FormData, key: string): string {
  const raw = form.get(key)
  return typeof raw === 'string' ? raw.trim() : ''
}

export function formNumber(form: FormData, key: string, fallback: number): number {
  const parsed = Number(formValue(form, key))
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

export function formList(form: FormData, key: string): string[] {
  return form.getAll(key).map((value) => String(value))
}
