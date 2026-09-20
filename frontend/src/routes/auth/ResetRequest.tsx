import { Link } from 'react-router-dom'
import { useRequestPasswordResetMutation } from '@/store/api/session'
import { useActionForm } from '@/hooks/use-action-form'
import { Button } from '@/components/ui/button'
import { AuthField, AuthInput, AuthNotice, AuthShell, useAuthCopy } from './AuthShell'

/**
 * Screen 03-reset-request (design/stitch-output/v2/03-reset-request.html) —
 * US-AD88 AC1/AC5.
 *
 * The confirmation is deliberately identical for a registered and an
 * unregistered address: the server answers 202 either way so the endpoint
 * cannot be used to discover which accounts exist, and a screen that said
 * "email not found" would undo that from the client side. So the success state
 * is the neutral "if that email is registered…", never "we sent it".
 *
 * The "Ingat password Anda?" row sits inside the form in the design, so it
 * carries the form's 16px gap above it (`cardFooter: mt-4 pt-1`).
 */
export function ResetRequest() {
  const [requestReset] = useRequestPasswordResetMutation()
  const t = useAuthCopy()

  const [state, formAction, isPending] = useActionForm(requestReset, (form) => ({
    email: String(form.get('email') ?? ''),
  }))

  return (
    <AuthShell
      variant="resetRequest"
      route="/reset"
      title={t['auth.resetRequest.title']}
      subtitle={t['auth.resetRequest.subtitle']}
      footer={
        <>
          {t['auth.rememberedPassword']}{' '}
          <Link to="/login" className="text-[12px] font-semibold text-[var(--color-accent)] hover:underline">
            {t['auth.signIn']}
          </Link>
        </>
      }
    >
      <form action={formAction} className="flex flex-col gap-4">
        <AuthField variant="resetRequest" id="email" label={t['field.email']}>
          <AuthInput
            variant="resetRequest"
            id="email"
            name="email"
            type="email"
            placeholder="nama@perusahaan.com"
            required
            autoComplete="email"
          />
        </AuthField>

        <AuthNotice variant="resetRequest" title={t['auth.securityNoticeTitle']}>
          {t['auth.resetRequest.notice']}
        </AuthNotice>

        {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
        {state.done ? <p className="text-[12px] text-[var(--color-accent)]">{t['auth.resetRequest.sent']}</p> : null}

        <div className="pt-1">
          <Button
            type="submit"
            variant="primary"
            disabled={isPending}
            className="h-9 w-full text-[12px] font-semibold shadow-sm"
          >
            {isPending ? t['auth.resetRequest.pending'] : t['auth.resetRequest.submit']}
          </Button>
        </div>
      </form>
    </AuthShell>
  )
}
