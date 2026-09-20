import { Link, useNavigate } from 'react-router-dom'
import { useRegisterMutation } from '@/store/api/session'
import { useActionForm } from '@/hooks/use-action-form'
import { Button } from '@/components/ui/button'
import { AuthField, AuthInput, AuthNotice, AuthShell, useAuthCopy } from './AuthShell'

/**
 * Screen 02-register (design/stitch-output/v2/02-register.html).
 *
 * Two fields, not four. The design asks only for email and password because
 * registration creates the personal workspace itself (US-AD01 AC5) and falls
 * back to the email local part when no name is sent (AC6) — the backend accepts
 * `name` and `org_name` but neither is required, and asking for them would be
 * a step the design deliberately removed. The design says so in its own banner:
 * "Workspace personal langsung dibuat tanpa konfigurasi organisasi."
 *
 * The form carries no flex gap: the design spaces its fields with margins
 * (`form-group { gap: 6px; margin-bottom: 14px }`), which `AuthField` applies.
 */
export function Register() {
  const navigate = useNavigate()
  const [register] = useRegisterMutation()
  const t = useAuthCopy()

  const [state, formAction, isPending] = useActionForm(
    register,
    (form) => ({
      email: String(form.get('email') ?? ''),
      password: String(form.get('password') ?? ''),
      name: '',
      org_name: '',
    }),
    () => navigate('/app', { replace: true }),
  )

  return (
    <AuthShell
      variant="register"
      route="/register"
      title={t['auth.register.title']}
      subtitle={t['auth.register.subtitle']}
      footer={
        <>
          {t['auth.haveAccount']}{' '}
          <Link to="/login" className="font-semibold text-[var(--color-accent)] hover:underline">
            {t['auth.signIn']}
          </Link>
        </>
      }
    >
      <form action={formAction} className="flex flex-col">
        <AuthField variant="register" id="email" label={t['field.email']}>
          <AuthInput
            variant="register"
            id="email"
            name="email"
            type="email"
            placeholder="nama@perusahaan.com"
            required
            autoComplete="email"
          />
        </AuthField>

        <AuthField
          variant="register"
          id="password"
          label={t['field.password']}
          hint={
            <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
              {t['auth.register.passwordHint']}
            </span>
          }
        >
          <AuthInput
            variant="register"
            id="password"
            name="password"
            type="password"
            placeholder="Minimal 8 karakter"
            required
            minLength={8}
            autoComplete="new-password"
          />
        </AuthField>

        <AuthNotice variant="register" title={t['auth.register.workspaceBadgeTitle']}>
          {t['auth.register.workspaceBadgeText']}
        </AuthNotice>

        {state.error ? <p className="mb-3 text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}

        <Button type="submit" variant="primary" disabled={isPending} className="h-9 w-full text-[13px] font-semibold">
          {isPending ? t['auth.register.pending'] : t['auth.register.submit']}
        </Button>
      </form>
    </AuthShell>
  )
}
