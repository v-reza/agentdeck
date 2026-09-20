import { useState } from 'react'
import { UserPlus } from 'lucide-react'
import { useAddMemberMutation, useUpdateMemberRoleMutation, useListMembersQuery } from '@/store/api/session'
import { useActionForm } from '@/hooks/use-action-form'
import { useT } from '@/hooks/use-t'
import { useAppSelector } from '@/store/hooks'
import type { Role } from '@/lib/domain'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'

type Member = NonNullable<ReturnType<typeof useListMembersQuery>['data']>[number]

/**
 * US-AD04's two write paths, both as modals.
 *
 * The design has no dialog for either: `38-members.html` mocks an unconverted
 * toolbar button and a bare "Ubah Role" text link per row. Placement is
 * therefore a UX decision made with the design system's own parts — a real
 * `<select>` for the role, which is accessible by default, and a field stack
 * matching the create dialog on the project directory.
 *
 * The role choices stop at `admin`. `owner` is never assignable: the server
 * rejects it on both POST and PATCH because ownership is granted with the
 * workspace, so offering it would only produce a guaranteed error. The
 * dictionary carries the reason (`members.role.ownerHint`) so the operator is
 * told why rather than left guessing.
 */
const ASSIGNABLE: Role[] = ['admin', 'member', 'viewer']

const SELECT_CLASS =
  'h-9 w-full rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-2.5 text-[13px] text-[var(--color-primary)] focus:border-[var(--color-accent)] focus:outline-none'

/** The invite dialog, triggered from the roster header. Renders nothing below admin (AC2). */
export function MembersToolbar({ orgName, canManage }: { orgName?: string; canManage: boolean }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [addMember] = useAddMemberMutation()
  // The tenant comes from the session slice, never a route param: the server
  // resolves the org from X-Org-ID (ARCHITECTURE 11.1 step 5).
  const orgID = useAppSelector((state) => state.session.activeOrgID) ?? ''

  const [state, formAction, isPending] = useActionForm(
    addMember,
    (form) => ({
      orgID,
      email: String(form.get('email') ?? '').trim(),
      role: String(form.get('role') ?? 'member') as Role,
    }),
    () => setOpen(false),
  )

  if (!canManage) {
    // Admin and above only (AC2). Absent rather than disabled: a greyed-out
    // button invites the operator to hunt for a way to turn it on.
    return null
  }

  return (
    <>
      <Button variant="primary" size="md" className="shrink-0 gap-1.5 px-3 shadow-sm" onClick={() => setOpen(true)}>
        <UserPlus size={14} />
        {t['members.invite.cta']}
      </Button>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={t['members.invite.title']}
        description={orgName ? `${t['members.invite.description']} (${orgName})` : t['members.invite.description']}
      >
        <form action={formAction} id="invite-member-form" className="flex flex-col gap-3.5">
          <Field label={t['members.colEmail']}>
            <Input name="email" type="email" required placeholder="nama@perusahaan.com" className="h-9 text-[13px]" />
          </Field>
          <Field label={t['members.colRole']} hint={t['members.role.ownerHint']}>
            <select name="role" defaultValue="member" className={SELECT_CLASS}>
              {ASSIGNABLE.map((role) => (
                <option key={role} value={role}>
                  {role}
                </option>
              ))}
            </select>
          </Field>

          {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
        </form>

        <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
          <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
            {t['action.cancel']}
          </Button>
          <Button type="submit" form="invite-member-form" variant="primary" size="sm" disabled={isPending}>
            {isPending ? t['members.invite.pending'] : t['members.invite.submit']}
          </Button>
        </div>
      </Modal>
    </>
  )
}

/**
 * The row's role editor.
 *
 * Renders nothing for a viewer (AC2). For the last-owner case the server would
 * answer 403 (AC3); rather than offering a control that cannot succeed, the row
 * shows the design's own "Full owner" label — the same editorial choice the mock
 * makes for Reza's row.
 */
export function MemberRowActions({
  orgID,
  member,
  canManage,
  isSelf,
}: {
  orgID: string
  member: Member
  canManage: boolean
  isSelf: boolean
}) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [updateRole] = useUpdateMemberRoleMutation()

  const [state, formAction, isPending] = useActionForm(
    updateRole,
    (form) => ({
      orgID,
      userID: String(form.get('user_id') ?? ''),
      role: String(form.get('role') ?? member.role) as Role,
    }),
    () => setOpen(false),
  )

  const lockedByOwnership = member.role === 'owner'

  return (
    // `whitespace-nowrap` is required, not cosmetic: the action column is the
    // first to be squeezed by the table's auto layout, and "Pemilik penuh"
    // wrapping to a second line pushed the design's 28px row to 33.5px. The
    // design's rows are single-line throughout.
    <div className="flex items-center justify-end gap-2 whitespace-nowrap">
      {isSelf ? (
        <span className="rounded bg-[var(--color-surface-sunken)] px-1 font-mono text-[9px] text-[var(--color-secondary)]">
          {t['members.you']}
        </span>
      ) : null}

      {lockedByOwnership ? (
        <span className="font-mono text-[10px] text-[var(--color-tertiary)] italic">{t['members.ownerLocked']}</span>
      ) : canManage ? (
        <button
          type="button"
          onClick={() => setOpen(true)}
          className="text-[11px] font-medium text-[var(--color-tertiary)] transition-colors hover:text-[var(--color-primary)]"
        >
          {t['members.changeRole']}
        </button>
      ) : null}

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        size="sm"
        title={t['members.role.title']}
        description={member.email}
      >
        <form action={formAction} id={`role-form-${member.user_id}`} className="flex flex-col gap-3.5">
          <input type="hidden" name="user_id" value={member.user_id} />
          <Field label={t['members.colRole']} hint={t['members.role.ownerHint']}>
            <select name="role" defaultValue={member.role} className={SELECT_CLASS}>
              {ASSIGNABLE.map((role) => (
                <option key={role} value={role}>
                  {role}
                </option>
              ))}
            </select>
          </Field>

          {state.error ? <p className="text-[12px] text-[var(--color-danger)]">{state.error}</p> : null}
        </form>

        <div className="mt-5 flex items-center justify-end gap-2 border-t border-[var(--color-border-subtle)] pt-4">
          <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
            {t['action.cancel']}
          </Button>
          <Button type="submit" form={`role-form-${member.user_id}`} variant="primary" size="sm" disabled={isPending}>
            {isPending ? t['members.invite.pending'] : t['members.role.submit']}
          </Button>
        </div>
      </Modal>
    </div>
  )
}
