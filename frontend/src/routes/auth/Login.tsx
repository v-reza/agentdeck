import { Link, useNavigate } from 'react-router-dom'
import { useLoginMutation } from '@/store/api/session'
import { useActionForm } from '@/hooks/use-action-form'
import { Button } from '@/components/ui/button'
import { AuthField, AuthInput, AuthShell, useAuthCopy } from './AuthShell'

/**
 * Screen 01-login (design/stitch-output/v2/01-login.html).
 *
 * Cloned from the design: brand mark and wordmark above the card, a `/login`
 * route chip with the tagline, a 400px card, and the `agentdeck v2.0 •
 * self-hosted • isolated` footer. The copy comes from the dictionary because
 * US-AD02 AC5 names the button "Log in" and the link "Lupa password?" — those
 * strings are the acceptance criterion, not taste.
 *
 * The design has no card footer: login is reached from the landing page and the
 * public nav, so there is no "Belum punya akun?" row to render here. The
 * /register route stays reachable from those same places.
 *
 * Login answers 200 with an empty body and sets the session cookie, so success
 * is a redirect, not a payload read. The session slice is seeded by the
 * `/auth/me` query the dashboard layout runs, which is why this form does not
 * fetch the profile itself.
 */
export function Login() {
  const navigate = useNavigate()
  const [login] = useLoginMutation()
  const t = useAuthCopy()

  const [state, formAction, isPending] = useActionForm(
    login,
    (form) => ({
      email: String(form.get('email') ?? ''),
      password: String(form.get('password') ?? ''),
    }),
    () => navigate('/app', { replace: true }),
  )

  return (
    <AuthShell
      variant="login"
      route="/login"
      title={t['auth.login.title']}
      subtitle={t['auth.login.subtitle']}
      // The design's card ends at the button, so the register link is rendered
      // below the system footer rather than as a card footer row.
      outsideFooter={
        <>
          {t['auth.noAccount']}{' '}
          <Link to="/register" className="font-medium text-[var(--color-accent)] hover:underline">
            {t['auth.register.submit']}
          </Link>
        </>
      }
    >
      <form action={formAction} className="flex flex-col gap-4">
        <AuthField variant="login" id="email" label={t['field.email']}>
          <AuthInput
            variant="login"
            id="email"
            name="email"
            type="email"
            placeholder="nama@perusahaan.com"
            required
            autoComplete="email"
          />
        </AuthField>

        <AuthField
          variant="login"
          id="password"
          label={t['field.password']}
          hint={
            // US-AD02 AC5: the "Lupa password?" link lives on the password
            // label row, exactly where the design puts it.
            <Link
              to="/reset"
              className="text-[12px] font-medium text-[var(--color-accent)] hover:underline focus:outline-none"
            >
              {t['auth.login.forgot']}
            </Link>
          }
        >
          <AuthInput
            variant="login"
            id="password"
            name="password"
            type="password"
            placeholder="••••••••"
            required
            autoComplete="current-password"
          />
        </AuthField>

        {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}

        <div className="pt-2">
          <Button type="submit" variant="primary" disabled={isPending} className="h-9 w-full text-[13px] font-medium">
            {isPending ? t['auth.login.pending'] : t['auth.login.submit']}
          </Button>
        </div>
      </form>
    </AuthShell>
  )
}
