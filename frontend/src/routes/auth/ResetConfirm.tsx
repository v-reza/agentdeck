import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useResetPasswordMutation } from '@/store/api/session'
import { useActionForm } from '@/hooks/use-action-form'
import { Button } from '@/components/ui/button'
import { AuthField, AuthInput, AuthNotice, AuthShell, useAuthCopy } from './AuthShell'

/**
 * Screen 04-reset-confirm (design/stitch-output/v2/04-reset-confirm.html) —
 * US-AD88 AC2/AC3.
 *
 * The token comes from the URL, which is what the emailed link carries. An
 * expired, already-used, or unknown token all come back as 410 with the same
 * body, so the screen shows the server's message rather than guessing which of
 * the three it was.
 *
 * The two password fields are compared before the request goes out: a mismatch
 * is a typo, not a server error, and burning a single-use token on one would
 * force the operator to start over.
 */
export function ResetConfirm() {
  const { token = '' } = useParams<{ token: string }>()
  const navigate = useNavigate()
  const [resetPassword] = useResetPasswordMutation()
  const t = useAuthCopy()
  const [mismatch, setMismatch] = useState<string | null>(null)

  const [state, formAction, isPending] = useActionForm(
    resetPassword,
    (form) => ({
      token,
      password: String(form.get('password') ?? ''),
    }),
    () => navigate('/login', { replace: true }),
  )

  return (
    <AuthShell
      variant="resetConfirm"
      route="/reset/:token"
      title={t['auth.resetConfirm.title']}
      subtitle={t['auth.resetConfirm.subtitle']}
      footer={
        <>
          {t['auth.rememberedPassword']}{' '}
          <Link to="/login" className="font-semibold text-[var(--color-accent)] hover:underline">
            {t['auth.signIn']}
          </Link>
        </>
      }
    >
      <form
        action={formAction}
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          // Compare before the action runs. `useActionForm` is driven by the
          // form action, so a mismatch has to stop the submit here.
          const data = new FormData(event.currentTarget)
          if (data.get('password') !== data.get('confirm_password')) {
            event.preventDefault()
            setMismatch(t['auth.resetConfirm.mismatch'])
            return
          }
          setMismatch(null)
        }}
      >
        <AuthField
          variant="resetConfirm"
          id="password"
          label={t['field.newPassword']}
          hint={
            <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
              {t['auth.register.passwordHint']}
            </span>
          }
        >
          <AuthInput
            variant="resetConfirm"
            id="password"
            name="password"
            type="password"
            placeholder={t['auth.register.passwordHint']}
            required
            minLength={8}
            autoComplete="new-password"
          />
        </AuthField>

        <AuthField variant="resetConfirm" id="confirm_password" label={t['field.confirmPassword']}>
          <AuthInput
            variant="resetConfirm"
            id="confirm_password"
            name="confirm_password"
            type="password"
            placeholder={t['auth.register.passwordHint']}
            required
            minLength={8}
            autoComplete="new-password"
          />
        </AuthField>

        <AuthNotice variant="resetConfirm" title={t['auth.securityNoticeTitle']}>
          {t['auth.resetConfirm.notice']}
        </AuthNotice>

        {mismatch ? <p className="text-[12px] text-[var(--color-danger)]">{mismatch}</p> : null}
        {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}

        <div className="pt-1">
          <Button
            type="submit"
            variant="primary"
            disabled={isPending}
            className="h-[38px] w-full text-[13px] font-semibold shadow-sm"
          >
            {isPending ? t['auth.resetConfirm.pending'] : t['auth.resetConfirm.submit']}
          </Button>
        </div>
      </form>
    </AuthShell>
  )
}
