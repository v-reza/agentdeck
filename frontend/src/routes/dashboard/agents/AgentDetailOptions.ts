/**
 * The closed choice lists of screen 28-agent-detail, split out of
 * `AgentDetailForm.tsx` so the section components stay readable.
 *
 * Every list below mirrors a server-side rule. None of them is a suggestion:
 * `validateAgent` refuses what is not in these sets, so the UI offers exactly
 * what the API accepts.
 *
 * The provider lists are gone with US-AD109: the provider is no longer a closed
 * set of four names typed by the operator, it is a row in the workspace registry
 * and the dropdown is fed from `GET /providers`.
 */

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
