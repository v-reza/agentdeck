import { useState } from 'react'
import { BadgeCheck, Info, KeyRound, Lock, Save, ShieldCheck } from 'lucide-react'
import { AvatarMonogram } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Panel } from '@/components/ui/card'
import { Field, Input } from '@/components/ui/input'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { useT } from '@/hooks/use-t'
import { useActionForm } from '@/hooks/use-action-form'
import { interpolate } from '@/lib/format'
import { useMeQuery, useUpdateMeMutation, type ProfilePatchBody } from '@/store/api/session'
import { useAppSelector } from '@/store/hooks'
import type { Dictionary } from '@/lib/i18n'
import type { Lang } from '@/lib/domain'
import { SkeletonText } from '@/components/ui/skeleton'

/**
 * Screen 15-profile — the operator's own account (US-AD89).
 *
 * Cloned from `design/stitch-output/v2/15-profile.html`, which is Blueprint C:
 * the 44px rail and the 224px sidebar are the shell's, and this route owns the
 * 420px work panel plus the 264px cost rail that `AppShell` already draws.
 *
 * Four deliberate deviations from the mock, each because the mock is annotated
 * rather than literal:
 *
 *  1. The mock's header carries three review annotations, not product copy:
 *     "State: default", "Blueprint C (Drawer 420px Aktif)", and a "US-AD89
 *     (AC1, AC2, AC5) COMPLIANT" badge. All three are dropped — the AC list is
 *     in the PRD, and a screen that claims its own compliance is not evidence of
 *     it. The badge and the chip are KEPT as shapes, because the design's own
 *     header has two, and the e2e asserts their accent tint and radius; only the
 *     text changes, from spec codes to what the pair actually tells the reader:
 *     the account is verified and self-managed.
 *  2. The mock's `<h1>` is the screen title ("Profil Akun Mandiri"), but
 *     `WorkspaceTopbar` already renders that title for every settings screen, so
 *     the panel's `<h1>` carries the section name instead. Rendering both would
 *     put the same string on screen twice.
 *  3. The mock marks **email** read-only ("Terkunci"), but ARCHITECTURE 6.2.2
 *     specifies `PATCH {name, email, avatar}` and AC3 is explicitly the *failure
 *     path* for a taken address. A locked field would make AC3 unreachable, so
 *     the field is editable and the 409 renders inline.
 *  4. The mock's "PATCH → 200 OK (Latency: 14ms)" success strip is a mock
 *     number. The real strip appears only after a successful save and states
 *     what actually happened, with no invented latency.
 *
 * `avatar_user` is a read-only display: the payload carries it and the endpoint
 * accepts an `avatar` URL, but there is no upload control in the design and none
 * is invented here.
 *
 * The form goes through `useActionForm` (`useActionState`), so there is no local
 * pending/error state. Only changed fields are sent: PATCH is partial, and
 * sending an unchanged name would be a write that stores the value already there.
 */
export function Profile() {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const { data, isLoading, isError } = useMeQuery()
  const [updateMe] = useUpdateMeMutation()

  const [saved, setSaved] = useState(false)
  const [state, formAction, isPending] = useActionForm(
    updateMe,
    (form) => buildPatch(form, { email: data?.email ?? '', name: data?.name ?? '' }),
    () => setSaved(true),
  )

  const avatar = data?.avatar_user ?? null
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const memberships = data?.workspaces ?? []

  return (
    <>
      <WorkspaceTopbar
        title={t['profile.title']}
        path="/settings/profile"
        subtitle={data ? interpolate(t['profile.accountActive'], [data.name]) : undefined}
      />

      <main className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-5">
        <Panel className="p-2.5">
          <div
            data-testid="profile-spec-strip"
            className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border-subtle)] pb-2"
          >
            <span
              data-testid="profile-story-badge"
              className="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-accent)]/30 bg-[var(--color-accent-tint)] px-2.5 py-0.5 font-mono text-[11px] font-bold text-[var(--color-accent)]"
            >
              <BadgeCheck size={14} />
              {t['profile.verified']}
            </span>
            <span
              data-testid="profile-milestone-chip"
              className="rounded-[4px] bg-[var(--color-surface-page)] px-2 py-0.5 font-mono text-[11px] text-[var(--color-tertiary)]"
            >
              {t['profile.selfManaged']}
            </span>
          </div>
          <h1
            data-testid="profile-panel-title"
            className="mt-2 text-[16px] font-bold tracking-tight text-[var(--color-primary)]"
          >
            {t['profile.subtitle']}
          </h1>
          <p className="mt-0.5 text-[12px] leading-relaxed text-[var(--color-secondary)]">{t['profile.description']}</p>
        </Panel>

        {isLoading ? (
          <Panel className="p-4">
            <SkeletonText lines={3} />
          </Panel>
        ) : isError || !data ? (
          <Panel className="p-4 font-mono text-[12px] text-[var(--color-danger)]">{t['state.error']}</Panel>
        ) : (
          <>
            <Panel className="space-y-2.5 p-2.5">
              <div className="flex items-center justify-between gap-2 border-b border-[var(--color-border-subtle)] pb-2">
                <div className="flex items-center gap-2">
                  <KeyRound size={17} className="text-[var(--color-accent)]" />
                  <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['profile.subtitle']}</h2>
                </div>
                <span className="rounded-[4px] border border-[var(--color-border-standard)] bg-[var(--color-surface-page)] px-2 py-0.5 font-mono text-[10px] text-[var(--color-secondary)]">
                  GET /api/v1/auth/me
                </span>
              </div>

              <div className="grid grid-cols-1 gap-2.5 md:grid-cols-2">
                <Attribute
                  testID="profile-tile-id"
                  label={t['profile.idField']}
                  value={data.id}
                  tag={t['profile.readOnly']}
                  mono
                  hint={t['profile.idHint']}
                />
                <Attribute
                  testID="profile-tile-email"
                  label={t['profile.emailField']}
                  value={data.email}
                  tag={t['profile.editable']}
                  mono
                  hint={t['profile.emailHint']}
                  tone="accent"
                />
                <Attribute
                  testID="profile-tile-name"
                  label={t['profile.nameField']}
                  value={data.name}
                  tag={t['profile.editable']}
                  hint={t['profile.nameHint']}
                  tone="accent"
                />
                <div
                  data-testid="profile-tile-avatar"
                  className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5"
                >
                  <div className="flex items-center justify-between font-mono text-[10px] font-bold uppercase text-[var(--color-tertiary)]">
                    <span>{t['profile.avatarField']}</span>
                    <span className="rounded bg-[var(--color-accent-tint)] px-1 text-[9px] text-[var(--color-accent)]">
                      {t['profile.avatarAuto']}
                    </span>
                  </div>
                  <div className="mt-1 flex items-center gap-2">
                    <AvatarMonogram avatar={avatar} fallbackName={data.name} />
                    <span className="truncate font-mono text-[11px] text-[var(--color-primary)]">
                      {avatar?.kind === 'image' && avatar.url
                        ? avatar.url
                        : `initials: "${avatar?.initials ?? ''}" (${avatar?.bg_color ?? '#101014'})`}
                    </span>
                  </div>
                  <div className="mt-0.5 font-mono text-[10px] text-[var(--color-tertiary)]">
                    {t['profile.avatarHint']}
                  </div>
                </div>
              </div>
            </Panel>

            <Panel className="space-y-2 p-2.5">
              <div className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] pb-2">
                <ShieldCheck size={17} className="text-[var(--color-accent)]" />
                <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['profile.workspacesTitle']}</h2>
              </div>
              <p className="text-[11px] text-[var(--color-tertiary)]">{t['profile.workspacesHint']}</p>

              {memberships.length === 0 ? (
                <p className="font-mono text-[11px] text-[var(--color-tertiary)]">{t['profile.empty']}</p>
              ) : (
                <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                  {memberships.map((membership) => (
                    <div
                      key={membership.id}
                      data-active={membership.id === activeOrgID ? 'true' : 'false'}
                      className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5"
                    >
                      <div className="truncate text-[12px] font-semibold text-[var(--color-primary)]">
                        {membership.name}
                      </div>
                      <div className="mt-0.5 flex items-center gap-2 font-mono text-[10px] text-[var(--color-tertiary)]">
                        <span className="truncate">{membership.slug}</span>
                        <span className="rounded bg-[var(--color-surface-sunken)] px-1.5 py-0.5 text-[9px] font-semibold uppercase text-[var(--color-secondary)]">
                          {membership.role}
                        </span>
                      </div>
                      <div className="mt-0.5 font-mono text-[10px] text-[var(--color-tertiary)]">
                        {t['profile.workspaceKind']}: {kindLabel(membership.kind, lang)}
                      </div>
                    </div>
                  ))}
                </div>
              )}

              <p className="flex items-center gap-1.5 border-t border-[var(--color-border-subtle)] pt-2 text-[11px] text-[var(--color-secondary)]">
                <Lock size={12} className="shrink-0 text-[var(--color-accent)]" />
                {t['profile.roleNote']}
              </p>
            </Panel>

            <Panel className="p-2.5">
              <form action={formAction} className="space-y-3.5">
                <div className="flex items-center justify-between border-b border-[var(--color-border-subtle)] pb-2">
                  <div className="flex items-center gap-2">
                    <Info size={17} className="text-[var(--color-accent)]" />
                    <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['profile.nameField']}</h2>
                  </div>
                  <span className="rounded-[4px] bg-[var(--color-accent-tint)] px-1.5 py-0.5 font-mono text-[9px] font-bold text-[var(--color-accent)]">
                    PATCH /api/v1/auth/me
                  </span>
                </div>

                <div>
                  <span className="block text-[11px] font-bold uppercase tracking-wider text-[var(--color-secondary)]">
                    {t['profile.idField']}
                  </span>
                  <Input
                    readOnly
                    name="id"
                    aria-label={t['profile.idField']}
                    value={data.id}
                    className="mt-1 cursor-not-allowed bg-[var(--color-surface-page)] font-mono text-[var(--color-tertiary)]"
                  />
                  <span className="mt-0.5 block font-mono text-[10px] text-[var(--color-tertiary)]">
                    {t['profile.idHint']}
                  </span>
                </div>

                <Field label={t['profile.emailField']} hint={t['profile.emailHint']}>
                  <Input
                    name="email"
                    type="email"
                    required
                    defaultValue={data.email}
                    className="font-mono"
                    onChange={() => setSaved(false)}
                  />
                </Field>

                <Field label={`${t['profile.nameField']} *`} hint={t['profile.nameHint']}>
                  <Input
                    name="name"
                    required
                    defaultValue={data.name}
                    className="border-[var(--color-accent)]"
                    onChange={() => setSaved(false)}
                  />
                </Field>

                {state.error ? (
                  <div
                    data-testid="profile-form-error"
                    className="rounded-[6px] border border-[var(--color-danger)]/40 bg-[var(--color-danger)]/5 p-2.5 font-mono text-[11px] text-[var(--color-danger)]"
                  >
                    {state.error}
                  </div>
                ) : null}

                {saved && state.done ? (
                  <div className="flex items-center gap-2 rounded-[6px] border border-[var(--color-accent)]/30 bg-[var(--color-accent-tint)] p-2.5 font-mono text-[11px] text-[var(--color-accent)]">
                    <BadgeCheck size={14} />
                    {t['profile.saved']}
                  </div>
                ) : null}

                <div className="flex items-center gap-2 pt-2">
                  <Button type="submit" variant="primary" disabled={isPending} className="flex-1">
                    <Save size={14} />
                    {isPending ? t['profile.savePending'] : t['profile.save']}
                  </Button>
                  <Button type="reset" variant="secondary" onClick={() => setSaved(false)}>
                    {t['profile.reset']}
                  </Button>
                </div>
              </form>
            </Panel>
          </>
        )}
      </main>
    </>
  )
}

/**
 * The PATCH body from the submitted form. Unchanged fields are dropped, so a
 * save that only renames sends `{name}` and cannot collide with an email that
 * belongs to somebody else. An empty patch is `{}`, which the server answers
 * with the current identity (PATCH is idempotent — ARCHITECTURE 6.2.2).
 */
export function buildPatch(form: FormData, current: { email: string; name: string }): ProfilePatchBody {
  const patch: ProfilePatchBody = {}
  const name = String(form.get('name') ?? '').trim()
  const email = String(form.get('email') ?? '').trim()
  if (name && name !== current.name) patch.name = name
  if (email && email !== current.email) patch.email = email
  return patch
}

/** One read-only attribute tile of the AC1 grid. */
function Attribute({
  label,
  value,
  tag,
  hint,
  mono,
  tone,
  testID,
}: {
  label: string
  value: string
  tag: string
  hint: string
  mono?: boolean
  tone?: 'accent'
  testID?: string
}) {
  return (
    <div
      data-testid={testID}
      className="rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-2.5"
    >
      <div className="flex items-center justify-between font-mono text-[10px] font-bold uppercase text-[var(--color-tertiary)]">
        <span>{label}</span>
        <span
          className={
            tone === 'accent'
              ? 'rounded border border-[var(--color-accent)]/30 bg-[var(--color-accent-tint)] px-1 text-[9px] text-[var(--color-accent)]'
              : 'rounded bg-[var(--color-accent-tint)] px-1 text-[9px] text-[var(--color-accent)]'
          }
        >
          {tag}
        </span>
      </div>
      <div
        className={
          mono
            ? 'mt-1 truncate font-mono text-[12px] font-semibold text-[var(--color-primary)]'
            : 'mt-1 truncate text-[12px] font-semibold text-[var(--color-primary)]'
        }
      >
        {value}
      </div>
      <div className="mt-0.5 font-mono text-[10px] text-[var(--color-tertiary)]">{hint}</div>
    </div>
  )
}

/** `kind` → the label the workspace screen also uses, kept in one place here. */
function kindLabel(kind: string, lang: Lang): string {
  const labels: Record<string, string> = {
    personal: lang === 'id' ? 'Personal / Solo' : 'Personal / Solo',
    manual: lang === 'id' ? 'Organisasi' : 'Organization',
  }
  return labels[kind] ?? kind
}

/** Exported so the form's type is asserted in the unit test without a DOM. */
export type ProfileDictionary = Dictionary
