import { useMembers, useCanAct } from '@/hooks/use-orgs'
import { useT } from '@/hooks/use-t'
import { useAppSelector } from '@/store/hooks'
import { formatCount, formatDate, interpolate } from '@/lib/format'
import { WorkspaceTopbar } from '@/components/layout/WorkspaceTopbar'
import { MembersToolbar, MemberRowActions } from './members-actions'
import { RoleBadge } from '@/components/ui/role-badge'
import { EmptyState, Panel } from '@/components/ui/card'

/**
 * Screen 38-members — the workspace roster (US-AD04).
 *
 * Cloned from `38-members.html`: a `rounded-[10px]` panel with a `px-3.5 py-2.5`
 * header, a 12px table whose header row is 32px (`h-8`) and whose rows are 28px
 * (`h-7`), 20px initial avatars, and `px-3` cells at 11-12px. The design's row
 * height is why the cells carry no vertical padding — `h-7` on the row sets the
 * density and `py-0` stops the content from adding to it.
 *
 * Four deviations, each deliberate:
 *
 *  1. "Status" is not rendered. The design mocks a green "Aktif" dot on every
 *     row, but nothing in the schema records a pending or inactive membership —
 *     a dot that is always green is decoration pretending to be data.
 *  2. "Akses Terakhir" (last access) is not rendered either: `sessions` records
 *     logins, not membership facts, and the roster endpoint does not return it.
 *     The "Joined" column shows the real membership timestamp instead.
 *  3. The design's header banner ("US-AD04 COMPLIANT (M0)", "Density: 28px baris
 *     / 32px header") is an annotation for whoever reviews the mock, not product
 *     copy, so it is not cloned.
 *  4. The design's toolbar button says "Buat Tautan Undangan" (create an invite
 *     link) but AC1 is a direct invite — `POST /orgs/{id}/members {email, role}`
 *     writes the membership and emails a notice. There is no token to create, so
 *     the label follows what the endpoint does.
 */
export function Members() {
  const t = useT()
  const lang = useAppSelector((state) => state.lang.lang)
  const orgs = useAppSelector((state) => state.session.workspaces)
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  // Hoisted: reading this inside the row map would call a hook in a loop.
  const selfID = useAppSelector((state) => state.session.userID)
  const org = orgs.find((workspace) => workspace.id === activeOrgID)

  const { members, isLoading, isError } = useMembers()
  const canManage = useCanAct('admin')

  return (
    <>
      <WorkspaceTopbar
        title={t['members.title']}
        path="/settings/members"
        subtitle={org ? org.name : t['members.subtitle']}
      />

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
        <Panel className="overflow-hidden">
          <div className="flex items-center justify-between gap-4 border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-well)] px-3.5 py-2.5">
            <div className="flex min-w-0 items-center gap-2">
              <h2 className="text-[13px] font-bold text-[var(--color-primary)]">{t['members.subtitle']}</h2>
              <span className="font-mono text-[11px] text-[var(--color-tertiary)]">
                {interpolate(t['members.count'], [formatCount(members.length, lang)])}
              </span>
            </div>
            <MembersToolbar orgName={org?.name} canManage={canManage} />
          </div>

          {isLoading ? (
            <p className="px-3.5 py-6 font-mono text-[12px] text-[var(--color-tertiary)]">{t['state.loading']}</p>
          ) : isError ? (
            <div className="p-3.5">
              <EmptyState title={t['members.loadFailed']} />
            </div>
          ) : members.length === 0 ? (
            <div className="p-3.5">
              <EmptyState title={t['members.empty']} hint={t['members.emptyHint']} />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-left text-[12px]">
                <thead>
                  <tr className="h-8 border-b border-[var(--color-border-subtle)] bg-[var(--color-surface-well)] font-mono text-[11px] text-[var(--color-tertiary)] uppercase">
                    <Th>{t['members.colUser']}</Th>
                    <Th>{t['members.colEmail']}</Th>
                    <Th>{t['members.colRole']}</Th>
                    <Th>{t['members.colJoined']}</Th>
                    <Th className="text-right">{t['members.colAction']}</Th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--color-border-subtle)]">
                  {members.map((member) => (
                    <tr
                      key={member.user_id}
                      // The design pins a fixed 28px row, which only holds if the
                      // cells stay on one line: with the default wrapping, a long
                      // email or a "19 Sep 2026" joined date breaks to a second
                      // line and pushes the row to 34px. `align-middle` removes
                      // the baseline inflation from mixing mono and sans cells,
                      // and `whitespace-nowrap` keeps every cell single-line.
                      // The panel's `overflow-x-auto` takes the overflow.
                      className="h-7 hover:bg-[var(--color-surface-hover)] [&>td]:align-middle [&>td]:whitespace-nowrap"
                    >
                      <td className="px-3 py-0">
                        <div className="flex items-center gap-2">
                          <span
                            aria-hidden="true"
                            className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-primary)] text-[9px] font-bold text-white"
                          >
                            {initials(member.name || member.email)}
                          </span>
                          <span className="truncate font-semibold text-[var(--color-primary)]">
                            {member.name || member.email}
                          </span>
                        </div>
                      </td>
                      <td className="px-3 py-0 font-mono text-[11px] text-[var(--color-secondary)]">{member.email}</td>
                      <td className="px-3 py-0">
                        <RoleBadge role={member.role} />
                      </td>
                      <td className="px-3 py-0 font-mono text-[10px] text-[var(--color-tertiary)]">
                        {formatDate(member.created_at, lang)}
                      </td>
                      <td className="px-3 py-0 text-right">
                        <MemberRowActions
                          orgID={activeOrgID ?? ''}
                          member={member}
                          canManage={canManage}
                          isSelf={member.user_id === selfID}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>
      </div>
    </>
  )
}

function Th({ children, className }: { children: React.ReactNode; className?: string }) {
  return <th className={['px-3 py-0 font-semibold', className ?? ''].join(' ')}>{children}</th>
}

/** Up to two initials for the avatar, from the name field when present. */
function initials(value: string): string {
  const parts = value.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}
