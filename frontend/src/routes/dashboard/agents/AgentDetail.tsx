import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  useArchiveAgentMutation,
  useGetAgentCatalogQuery,
  useGetAgentQuery,
  useListAgentSkillsQuery,
  useUpdateAgentMutation,
  type ArchiveAgentArgs,
} from '@/store/api/agents'
import { useListProvidersQuery } from '@/store/api/providers'
import { useCanAct } from '@/hooks/use-orgs'
import { describeError, useActionForm } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import type { Dictionary } from '@/lib/i18n'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { Button } from '@/components/ui/button'
import { EmptyState, Panel } from '@/components/ui/card'
import {
  AgentConfigSection,
  RuntimeSection,
  formList,
  formNumber,
  formValue,
  type AgentFormValues,
} from './AgentDetailForm'
import { AgentHero, ArchiveButton, LifecycleCard, SaveButton, StatusPill, agentState } from './AgentDetailParts'
import type { AgentState } from './AgentDetailParts'
import { AgentProviderKeyPanel } from '@/components/agents/AgentProviderKeyPanel'

const FORM_ID = 'agent-detail-form'
const ARCHIVE_ERROR_ID = 'agent-archive-error'

/**
 * The three lifecycle states, in the words the dictionary uses. `agentState`
 * returns the enum; this is the only place it becomes text, so no screen can
 * print the raw `needsKey` literal again. The e2e jargon guard greps for
 * camelCase identifiers in the body, which is what catches a regression here.
 */
const STATE_LABEL: Record<AgentState, keyof Dictionary> = {
  ready: 'agents.status.ready',
  needsKey: 'agents.status.needsKey',
  archived: 'agents.status.archived',
}

/**
 * Screen 28-agent-detail — one agent's configuration, in the shape
 * `design/stitch-output/v2/28-agent-detail.html` draws it.
 *
 * This file owns four things and delegates the rest: the topbar controls, the
 * loaded row, the single form that writes it, and the archive toggle. The cards
 * live in `AgentDetailParts.tsx` (presentation) and `AgentDetailForm.tsx` (the
 * editable sections), which is what keeps this file to the data and the wiring.
 *
 * Both writes are real:
 *  - `PATCH /agents/{id}` with the full profile, submitted from the topbar button
 *    through `useActionForm` (React 19 `useActionState`).
 *  - `PATCH /agents/{id}` with `{archived: true|false}` — the spelling the handler
 *    branches on before it looks at any other field. The mutation invalidates the
 *    `Agent` tag, so the registry list and this page refetch rather than showing a
 *    stale badge.
 *
 * What the screen deliberately does not show: a credential control. Storing a
 * provider key is a different story reserved for owner/admin, and a field that
 * cannot be written is worse than no field. The lifecycle card states the
 * credential state as fact instead.
 */
export function AgentDetail() {
  const t = useT()
  const { orgID, agentID } = useParams<{ orgID: string; agentID: string }>()
  const id = agentID ?? ''

  const { data: agent, isError } = useGetAgentQuery(id, { skip: !id })
  const { data: catalog, isLoading: catalogLoading } = useGetAgentCatalogQuery()
  const { data: skills } = useListAgentSkillsQuery()
  const { data: providers = [] } = useListProvidersQuery()
  // Same resolution as the registry: `agent.provider` is the protocol, the
  // column and the hero ask which provider the operator registered.
  const providerName = new Map(providers.map((entry) => [entry.id, entry.name]))
  const [updateAgent] = useUpdateAgentMutation()
  const [archiveAgent] = useArchiveAgentMutation()
  const canArchive = useCanAct('admin')
  const canManageKey = useCanAct('admin')
  const [keyPanelOpen, setKeyPanelOpen] = useState(false)

  // US-AD109 AC10: changing the provider empties the model choice, because the
  // old provider's models need not exist on the new one. The field has to be
  // controlled for that to be expressible at all, which is why both live here
  // rather than inside the section.
  const [chosenProviderID, setChosenProviderID] = useState<string | null>(null)
  const [chosenModel, setChosenModel] = useState<string | null>(null)
  const providerID = chosenProviderID ?? agent?.provider_id ?? ''
  const model = chosenModel ?? agent?.model ?? ''

  function changeProvider(next: string) {
    setChosenProviderID(next)
    setChosenModel('')
  }

  const [saveState, saveAction, isSaving] = useActionForm(updateAgent, (form) => ({
    id,
    name: formValue(form, 'name') || (agent?.name ?? ''),
    provider_id: formValue(form, 'provider_id') || (agent?.provider_id ?? ''),
    model: formValue(form, 'model') || (agent?.model ?? ''),
    reasoning_effort: formValue(form, 'reasoning_effort') || (agent?.reasoning_effort ?? 'medium'),
    skills: formList(form, 'skills'),
    tools: formList(form, 'tools'),
    max_runtime_seconds: formNumber(form, 'max_runtime_seconds', agent?.max_runtime_seconds ?? 14400),
    retry_policy: formValue(form, 'retry_policy') || (agent?.retry_policy ?? 'transient_only'),
    max_attempts: formNumber(form, 'max_attempts', agent?.max_attempts ?? 3),
  }))

  const archive = useArchiveToggle(id, archiveAgent)
  const archived = Boolean(agent?.archived_at)

  const values: AgentFormValues = {
    model,
    reasoning_effort: agent?.reasoning_effort ?? 'medium',
    max_runtime_seconds: agent?.max_runtime_seconds ?? 14400,
    max_attempts: agent?.max_attempts ?? 3,
    retry_policy: agent?.retry_policy ?? 'transient_only',
    tools: agent?.tools ?? [],
    skills: agent?.skills ?? [],
  }

  return (
    <>
      <WorkspaceTopbar
        title={agent?.name ?? t['agents.detail.title']}
        path="/agents"
        subtitle={agent ? agent.model : undefined}
        right={
          agent ? (
            <div className="flex items-center gap-2.5">
              <StatusPill agent={agent} />
              {/* US-AD86: rotating or revoking the credential happens here.
                  Owner/admin only — the same floor the PUT/DELETE routes carry. */}
              {canManageKey ? (
                <Button
                  variant="secondary"
                  size="md"
                  data-testid="agent-key-panel-trigger"
                  onClick={() => setKeyPanelOpen(true)}
                >
                  {t['agents.key.title']}
                </Button>
              ) : null}
              <ArchiveButton
                archived={archived}
                allowed={canArchive}
                isArchiving={archive.pending}
                describedBy={ARCHIVE_ERROR_ID}
                onClick={() => archive.run(!archived)}
              />
              <SaveButton formID={FORM_ID} isPending={isSaving} />
            </div>
          ) : null
        }
      />

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto bg-[var(--color-surface-page)] p-5">
        <div className="mx-auto flex w-full max-w-[940px] flex-col gap-4 pb-8">
          <div className="flex items-center gap-2 text-[11px]">
            <Link
              to={orgID ? `/app/${orgID}/agents` : '/agents'}
              className="font-medium text-[var(--color-tertiary)] hover:text-[var(--color-primary)]"
            >
              {t['agents.detail.back']}
            </Link>
            <span className="text-[var(--color-border-strong)]">/</span>
            <span className="font-mono text-[10px] text-[var(--color-quaternary)]">
              {t['agents.detail.id']} {agentID}
            </span>
          </div>

          {isError ? (
            <EmptyState title={t['agents.detail.notFound']} hint={t['agents.detail.tasksEmpty']} />
          ) : !agent ? (
            <Panel className="p-3">
              <p className="font-mono text-[12px] text-[var(--color-tertiary)]">{t['agents.detail.loading']}</p>
            </Panel>
          ) : (
            <>
              <AgentHero agent={agent} providerName={providerName.get(agent.provider_id ?? '')} />

              <form id={FORM_ID} action={saveAction} className="flex flex-col gap-4">
                <AgentConfigSection
                  agent={agent}
                  catalog={catalog}
                  catalogLoading={catalogLoading}
                  providers={providers}
                  providerID={providerID}
                  onProviderChange={changeProvider}
                  model={model}
                  onModelChange={setChosenModel}
                />

                <div className="grid grid-cols-2 gap-4">
                  <LifecycleCard
                    agent={agent}
                    canArchive={canArchive}
                    error={archive.error}
                    errorID={ARCHIVE_ERROR_ID}
                  />
                  <SaveStateCard
                    state={STATE_LABEL[agentState(agent)]}
                    saved={saveState.done}
                    error={saveState.error}
                    pending={isSaving}
                  />
                </div>

                <RuntimeSection agent={agent} skills={skills} values={values} />
              </form>
            </>
          )}
        </div>
      </div>

      {agent ? (
        <AgentProviderKeyPanel
          open={keyPanelOpen}
          onClose={() => setKeyPanelOpen(false)}
          agent={{
            id: agent.id,
            name: agent.name,
            provider: agent.provider,
            model: agent.model,
            has_provider_key: agent.has_provider_key,
          }}
        />
      ) : null}
    </>
  )
}

/**
 * The archive toggle's own state: one in-flight request, one inline error, and
 * the error cleared on the next attempt. It is separate from the form's
 * `useActionState` because it is not a form — but a refused PATCH is read through
 * the same `describeError`, so both write paths report a 403 or a 409 in the same
 * words the server used.
 */
function useArchiveToggle(id: string, trigger: (args: ArchiveAgentArgs) => { unwrap: () => Promise<unknown> }) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function run(archived: boolean) {
    setPending(true)
    setError(null)
    try {
      await trigger({ id, archived }).unwrap()
    } catch (rejection) {
      setError(describeError(rejection))
    } finally {
      setPending(false)
    }
  }

  return { run, pending, error }
}

/**
 * The second card of the design's two-column row, reporting what the save did.
 * The mock drew a mock success strip with an invented latency; this one only
 * changes once the server has answered, and on a rejection it renders the
 * server's own words next to the fields that caused them.
 */
function SaveStateCard({
  state,
  saved,
  error,
  pending,
}: {
  state: keyof Dictionary
  saved: boolean
  error: string | null
  pending: boolean
}) {
  const t = useT()
  const message = error
    ? `${t['agents.detail.saveFailed']}: ${error}`
    : pending
      ? t['agents.detail.saving']
      : saved
        ? t['agents.detail.saved']
        : t['agents.detail.providerHint']

  return (
    <Panel className="flex flex-col justify-between p-[14px] shadow-xs">
      <div>
        <div className="mb-3 flex items-center justify-between border-b border-[var(--color-border-subtle)] pb-2">
          <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['agents.detail.save']}</h2>
          <span className="rounded-[4px] bg-[var(--color-surface-page)] px-2 py-0.5 font-mono text-[10px] font-semibold text-[var(--color-secondary)]">
            {t[state]}
          </span>
        </div>
        <p className="text-[11px] leading-relaxed text-[var(--color-secondary)]">{t['agents.detail.modelHint']}</p>
      </div>

      <div
        role={error ? 'alert' : undefined}
        className="mt-3 border-t border-[var(--color-border-subtle)] pt-2 font-mono text-[10.5px] leading-snug text-[var(--color-tertiary)]"
      >
        {message}
      </div>
    </Panel>
  )
}
