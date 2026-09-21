import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * `createAgent` and `updateAgent` must carry the same mutable profile.
 *
 * They drifted: `updateAgent` spreads its whole args object, while `createAgent`
 * spells every key out (it has to — the server rejects unknown keys). So when
 * `base_url` was added to the update path, the create path was never told, and a
 * BYO registration sent `provider: 'openai_compatible'` with no endpoint. The
 * server answered 400 `base_url is required exactly when provider is
 * 'openai_compatible'` — which reads like the operator forgot the field, while
 * the form had rendered it and they had filled it in.
 *
 * The same drift appeared one layer down, in `CreateAgent`'s SQL, where it is
 * pinned by `internal/store/queries_columns_test.go`. This is the client half.
 *
 * It is a source-level check on purpose: the bug lives in the literal that
 * builds the request body, and no handler test can see a key that was never
 * sent.
 */

const here = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(resolve(here, 'agents.ts'), 'utf8')

/** The declared field names of one exported interface, in one spelling. */
function interfaceFields(name: string): string[] {
  const start = source.indexOf(`interface ${name} {`)
  if (start < 0) throw new Error(`interface ${name} not found in agents.ts`)
  const end = source.indexOf('\n}', start)
  // `CreateAgentArgs` is camelCase and `UpdateAgentArgs` is snake_case — they
  // are consumed differently (a typed object vs. a wire body), and that is
  // fine. What must not differ is the *set* of fields, so compare the names
  // with the convention removed.
  return [...source.slice(start, end).matchAll(/^\s{2}(\w+)\??:/gm)]
    .map((match) => match[1].toLowerCase().replace(/_/g, ''))
    .sort()
}

describe('the agent write paths carry the same profile', () => {
  it('CreateAgentArgs declares every field UpdateAgentArgs declares', () => {
    const created = interfaceFields('CreateAgentArgs')
    const updated = interfaceFields('UpdateAgentArgs')

    expect(created.length, 'the parser found CreateAgentArgs').toBeGreaterThan(5)
    expect(updated.length, 'the parser found UpdateAgentArgs').toBeGreaterThan(5)

    // Two kinds of field are legitimately absent on create.
    //
    // `id` and `projectID` are addressing, not profile: create is scoped by
    // project, update by the agent's own id.
    //
    // `provider` and `base_url` are *derived* since US-AD109 phase 5. The
    // registry owns both, so create sends `provider_id` and the server fills
    // them in (AC6). Update still carries them because the detail screen is
    // what edits an agent that has no provider — the US-AD86 case — and
    // `applyProvider` overwrites them from the registry whenever one is set.
    // Excluding them here is a statement about the contract, not a loosening:
    // a create that sent them would be a second writer for a fact the provider
    // owns, and the test below pins that it does not.
    const notOnCreate = new Set(['id', 'projectID', 'provider', 'baseurl'])
    const missing = updated.filter((field) => !notOnCreate.has(field) && !created.includes(field))

    expect(missing, 'UpdateAgentArgs fields with no create-side equivalent').toEqual([])
  })

  it('the create body sends provider_id and never a hand-typed endpoint', () => {
    // Declaring the field is not enough — it has to reach the request, which is
    // exactly what was broken. The direction has flipped: it is now the
    // omission of `provider`/`base_url` that is load-bearing, because sending
    // them would let the agent carry a copy of what the provider owns (AC6).
    expect(source).toMatch(/provider_id: body\.providerID/)
    expect(source).not.toMatch(/base_url: body\./)
    expect(source).not.toMatch(/^\s+provider: body\./m)
  })
})
