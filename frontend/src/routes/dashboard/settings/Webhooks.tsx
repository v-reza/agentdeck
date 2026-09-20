import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { EmptyState } from '@/components/ui/card'

/**
 * Webhooks tab (ARCHITECTURE 13). Same rule as the API keys tab: the endpoint is
 * not implemented, so the screen says so instead of offering a control that
 * would silently fail.
 */
export function Webhooks() {
  return (
    <>
      <WorkspaceTopbar title="Webhooks" subtitle="outbound event delivery" />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <EmptyState
          title="Webhooks are not available yet"
          hint="Signed delivery with retry lands with the webhook worker."
        />
      </div>
    </>
  )
}
