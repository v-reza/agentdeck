import { useState } from 'react'
import { Laptop, LogOut, Shield } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EmptyState } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { useT } from '@/hooks/use-t'
import { useAppSelector } from '@/store/hooks'
import { formatDateTime } from '@/lib/format'
import {
  useListSessionsQuery,
  useRevokeSessionMutation,
  useChangePasswordMutation,
  type SessionInfo,
} from '@/store/api/session'

/**
 * Screen 16-security — password and active sessions (US-AD90).
 *
 * Two panels on one screen because they are one decision: changing the password
 * revokes every other session (AC1), so the session list is where that promise
 * becomes visible. Splitting them would hide the consequence of the action on the
 * other panel.
 *
 * WHAT THE CONTRACT DOES NOT ALLOW, and is therefore not built:
 *   - No "Cabut Semua Sesi Lainnya" button as the design draws it. `DELETE
 *     /auth/sessions/{id}` takes ONE id, and the list carries no other session of
 *     the caller. A bulk revoke would be N sequential requests the client
 *     orchestrates — and if one fails halfway the operator cannot tell which
 *     sessions survived. Changing the password already does the bulk revoke,
 *     atomically and server-side, and the panel says so.
 *   - No location ("Jakarta, ID"), no client type, no protocol, and no session id
 *     hash: the API returns `user_agent`, `ip`, `last_seen_at`, `created_at`, and
 *     `current`, and nothing else. The device label is derived from the user agent
 *     rather than invented.
 *   - Revoking someone else's session is possible server-side for an owner/admin
 *     (US-AD05 AC2) but is NOT reachable from this screen: `GET /auth/sessions`
 *     only ever returns the caller's own rows, so there is no id to act on. That
 *     path is not reachable from the UI at all, which is a fact about the API, not
 *     a gap in this screen.
 */
export function Security() {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const { data, isLoading } = useListSessionsQuery()
  const [revokeSession, revokeState] = useRevokeSessionMutation()

  const sessions = data ?? []

  return (
    <>
      <WorkspaceTopbar
        title={t['security.title']}
        path="/settings/security"
        subtitle={sessions.length > 0 ? t['security.count'].replace('{count}', String(sessions.length)) : undefined}
      />

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        <div className="flex max-w-[860px] flex-col gap-4">
          <SessionsPanel
            sessions={sessions}
            isLoading={isLoading}
            lang={lang}
            revoking={revokeState.isLoading}
            onRevoke={(id) => void revokeSession(id)}
          />
          <PasswordPanel />
        </div>
      </div>
    </>
  )
}

function SessionsPanel({
  sessions,
  isLoading,
  lang,
  revoking,
  onRevoke,
}: {
  sessions: SessionInfo[]
  isLoading: boolean
  lang: 'en' | 'id'
  revoking: boolean
  onRevoke: (id: string) => void
}) {
  const t = useT()
  return (
    <section className="rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)]">
      <header className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] px-4 py-3">
        <Laptop size={14} className="text-[var(--color-tertiary)]" />
        <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['security.sessionsTitle']}</h2>
      </header>

      {isLoading ? (
        <div className="flex flex-col gap-2 p-4">
          {[0, 1].map((row) => (
            <Skeleton key={row} className="w-full" />
          ))}
        </div>
      ) : sessions.length === 0 ? (
        <div className="p-4">
          <EmptyState title={t['security.noSessions']} hint={t['security.noSessionsHint']} />
        </div>
      ) : (
        <ul className="flex flex-col" data-testid="session-list">
          {sessions.map((session) => (
            <li
              key={session.id}
              data-testid="session-row"
              data-current={session.current ? 'true' : 'false'}
              className={
                'flex items-start justify-between gap-3 border-b border-[var(--color-border-subtle)] px-4 py-3 last:border-b-0 ' +
                (session.current ? 'bg-[var(--color-accent-tint)]' : '')
              }
            >
              <div className="flex min-w-0 flex-col gap-0.5">
                <span className="flex items-center gap-2">
                  <span className="truncate text-[12px] font-semibold text-[var(--color-primary)]">
                    {deviceLabel(session.user_agent, t['security.unknownDevice'])}
                  </span>
                  {session.current ? (
                    <span
                      data-testid="session-current"
                      className="rounded-[4px] bg-[var(--color-accent)]/12 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-accent)]"
                    >
                      {t['security.thisDevice']}
                    </span>
                  ) : null}
                </span>
                {/* The full user agent, because the label above is a guess at a
                    human name and the raw string is what identifies the client. */}
                <span className="truncate font-mono text-[10px] text-[var(--color-tertiary)]">
                  {session.user_agent}
                </span>
                <span className="font-mono text-[10px] text-[var(--color-tertiary)]">
                  {t['security.ip']}: {session.ip || '—'}
                </span>
                <span className="text-[10px] text-[var(--color-tertiary)]">
                  {t['security.lastSeen']}: {formatDateTime(session.last_seen_at, lang)}
                </span>
              </div>

              {/* The current session keeps no revoke button: signing yourself out
                  from the session list is what the logout control is for, and a
                  button that ends the session rendering it is a trap. */}
              {!session.current ? (
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={revoking}
                  onClick={() => onRevoke(session.id)}
                  data-testid="session-revoke"
                >
                  <LogOut size={12} />
                  {t['security.revoke']}
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

/**
 * A short human label from the user agent. Deliberately crude and deliberately
 * labelled as such in the UI (the raw string is printed underneath): a real
 * parser is a dependency, and a wrong-but-confident device name is worse than an
 * honest "Perangkat tidak dikenal".
 */
function deviceLabel(userAgent: string, fallback: string): string {
  if (!userAgent) return fallback
  const os = /Windows/i.test(userAgent)
    ? 'Windows'
    : /Macintosh|Mac OS X/i.test(userAgent)
      ? 'macOS'
      : /Android/i.test(userAgent)
        ? 'Android'
        : /iPhone|iPad|iOS/i.test(userAgent)
          ? 'iOS'
          : /Linux/i.test(userAgent)
            ? 'Linux'
            : ''
  const browser = /Edg\//i.test(userAgent)
    ? 'Edge'
    : /Chrome\//i.test(userAgent)
      ? 'Chrome'
      : /Safari\//i.test(userAgent)
        ? 'Safari'
        : /Firefox\//i.test(userAgent)
          ? 'Firefox'
          : ''
  const label = [browser, os].filter(Boolean).join(' · ')
  return label || fallback
}

function PasswordPanel() {
  const t = useT()
  const [changePassword] = useChangePasswordMutation()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)

  // The server's floor is 8 (minPasswordLength). Checked here so the operator is
  // told before the request, not after it.
  const tooShort = newPassword.length > 0 && newPassword.length < 8
  const mismatch = confirm.length > 0 && confirm !== newPassword
  const canSave = oldPassword.length > 0 && newPassword.length >= 8 && confirm === newPassword && !busy

  return (
    <section className="rounded-[10px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)]">
      <header className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] px-4 py-3">
        <Shield size={14} className="text-[var(--color-tertiary)]" />
        <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['security.passwordTitle']}</h2>
      </header>

      <div className="flex flex-col gap-3 p-4">
        {/* AC1's consequence, stated where the action is rather than in a doc. */}
        <p className="text-[11px] text-[var(--color-tertiary)]">{t['security.passwordHint']}</p>

        <label className="flex flex-col gap-1">
          <span className="text-[11px] text-[var(--color-secondary)]">{t['security.oldPassword']}</span>
          <Input
            type="password"
            value={oldPassword}
            onChange={(event) => setOldPassword(event.target.value)}
            autoComplete="current-password"
            data-testid="security-old-password"
          />
        </label>

        <label className="flex flex-col gap-1">
          <span className="text-[11px] text-[var(--color-secondary)]">{t['security.newPassword']}</span>
          <Input
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            autoComplete="new-password"
            data-testid="security-new-password"
          />
          {tooShort ? <span className="text-[10px] text-[var(--color-danger)]">{t['security.tooShort']}</span> : null}
        </label>

        <label className="flex flex-col gap-1">
          <span className="text-[11px] text-[var(--color-secondary)]">{t['security.confirmPassword']}</span>
          <Input
            type="password"
            value={confirm}
            onChange={(event) => setConfirm(event.target.value)}
            autoComplete="new-password"
            data-testid="security-confirm-password"
          />
          {mismatch ? <span className="text-[10px] text-[var(--color-danger)]">{t['security.mismatch']}</span> : null}
        </label>

        {error ? (
          <p role="alert" data-testid="security-error" className="text-[11px] text-[var(--color-danger)]">
            {error}
          </p>
        ) : null}
        {done ? (
          <p role="status" data-testid="security-done" className="text-[11px] text-[var(--color-accent)]">
            {t['security.passwordChanged']}
          </p>
        ) : null}

        <div>
          <Button
            size="sm"
            disabled={!canSave}
            data-testid="security-save"
            onClick={async () => {
              setBusy(true)
              setError('')
              setDone(false)
              try {
                await changePassword({ old_password: oldPassword, new_password: newPassword }).unwrap()
                setDone(true)
                setOldPassword('')
                setNewPassword('')
                setConfirm('')
              } catch (cause) {
                // AC3: the server answers 401 for a wrong old password and changes
                // nothing. Shown against the field it belongs to.
                //
                // The code arrives as `originalStatus`, not `status`, and that is
                // not a detail to paper over: this API writes every error with
                // `http.Error`, which is `text/plain`, while RTK Query's base query
                // parses responses as JSON. The parse throws, so RTK reports
                // `status: 'PARSING_ERROR'` and keeps the real code in
                // `originalStatus`. Branching on `status === 401` compiles and
                // never matches — it was the first version of this code, and the
                // e2e caught it by showing the generic message instead.
                const rt = cause as { status?: unknown; originalStatus?: unknown } | undefined
                const status = Number(rt?.originalStatus ?? rt?.status)
                setError(status === 401 ? t['security.wrongOldPassword'] : t['state.error'])
              } finally {
                setBusy(false)
              }
            }}
          >
            {t['action.save']}
          </Button>
        </div>
      </div>
    </section>
  )
}
