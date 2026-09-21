import type { Dictionary } from '@/lib/i18n'

/**
 * The closed choice lists of screen 28-agent-detail, split out of
 * `AgentDetailForm.tsx` so the section components stay readable.
 *
 * Every list below mirrors a server-side rule. None of them is a suggestion:
 * `validateAgent` refuses what is not in these sets, so the UI offers exactly
 * what the API accepts.
 */

/** The four provider values the agents table accepts. */
export const PROVIDER_CHOICES = ['openai', 'anthropic', 'deepseek', 'openai_compatible']

/**
 * What the register form offers (DECISIONS 6A.F).
 *
 * BYO only, by the operator's own decision: they bring an endpoint and a
 * credential, so the model list is pulled from their server rather than from
 * our price catalog. `PROVIDER_CHOICES` stays for the detail screen, where an
 * agent that already carries `openai` must still render its own value — the
 * narrower list here is a choice about *creating*, not about displaying.
 */
export const CREATE_PROVIDER_CHOICES = ['openai_compatible']

const PROVIDER_LABEL: Record<string, keyof Dictionary> = {
  openai: 'agents.detail.providerOpenAI',
  anthropic: 'agents.detail.providerAnthropic',
  deepseek: 'agents.detail.providerDeepseek',
  openai_compatible: 'agents.detail.providerCustom',
}

/**
 * The display name of a provider value.
 *
 * The stored value is a machine token and is shown as the label only when it is
 * not one of the four known ones — an unknown provider is real data, so it is
 * printed rather than hidden behind a blank option.
 */
export function providerLabel(t: Dictionary, value: string): string {
  const key = PROVIDER_LABEL[value]
  return key ? t[key] : value
}

/**
 * The nine tool primitives as a closed multi-select (DECISIONS 6A.H). `bash` is
 * marked because it always goes through the approval gate.
 */
export const TOOL_SET: { name: string; gated: boolean }[] = [
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
export const RUNTIME_PRESETS = [300, 900, 1800, 3600, 7200, 14400, 28800, 86400]

/**
 * The reasoning levels the provider adapter passes through. A closed list, not a
 * free-form field: the value goes into the request body verbatim, so anything
 * else is a runtime error the form could have caught.
 */
export const REASONING_EFFORTS = ['low', 'medium', 'high']

/** The retry policies `agents_retry_policy_chk` accepts, in escalation order. */
export const RETRY_POLICIES = ['never', 'transient_only', 'always']

/**
 * The tool choices for one agent: the closed set, plus any name the row already
 * carries that the set does not know.
 *
 * The second half is not decoration. The handler stores `tools` as a list and
 * does not validate its members, so a row can hold a name from an older
 * contract. Rendering only the closed set would leave that name unchecked, and
 * the next save would silently drop it — the screen would quietly rewrite the
 * agent it is describing.
 */
export function toolChoices(selected: string[]): { name: string; gated: boolean }[] {
  const known = new Set(TOOL_SET.map((tool) => tool.name))
  const extras = selected.filter((name) => !known.has(name)).map((name) => ({ name, gated: false }))
  return [...TOOL_SET, ...extras]
}
