import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState } from '@/components/ui/card'

/**
 * API keys tab (ARCHITECTURE 6.2.3). The list endpoint is not part of M1, so this
 * screen states that plainly. It is deliberately not a form: creating a key
 * whose plaintext the UI cannot show once would be worse than not offering it.
 */
export function ApiKeys() {
  return (
    <>
      <WorkspaceTopbar title="API keys" subtitle="programmatic access" />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <EmptyState
          title="API keys are not available yet"
          hint="The list/create endpoints land with the agent credential work (adk_ keys)."
        />
      </div>
    </>
  )
}
