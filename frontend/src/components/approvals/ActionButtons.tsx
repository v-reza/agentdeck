import { useFormStatus } from 'react-dom'
import { Button } from '@/components/ui/button'

/**
 * Approve / reject buttons (ARCHITECTURE 18.2 lists `useFormStatus` for exactly
 * these). Reading pending state from the parent form means the buttons never
 * need a local `isSubmitting` flag, so they cannot drift out of sync with the
 * form that actually owns the request.
 */
export function ApproveButton({ label = 'Approve' }: { label?: string }) {
  const { pending } = useFormStatus()
  return (
    <Button type="submit" name="decision" value="approved" disabled={pending}>
      {pending ? 'Sending…' : label}
    </Button>
  )
}

export function RejectButton({ label = 'Reject' }: { label?: string }) {
  const { pending } = useFormStatus()
  return (
    <Button type="submit" name="decision" value="rejected" variant="secondary" disabled={pending}>
      {pending ? 'Sending…' : label}
    </Button>
  )
}
