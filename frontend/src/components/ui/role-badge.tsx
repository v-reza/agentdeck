import { cn } from '@/lib/cn'
import type { Role } from '@/lib/domain'
import type { Dictionary } from '@/lib/i18n'
import { useT } from '@/hooks/use-t'

/**
 * The role chip from `38-members.html`.
 *
 * Owner/Admin/Member use the design's own classes: the accent-tinted chip for
 * owner, `blue-50/blue-200` for admin, `slate-100/slate-200` for member. The
 * design draws the owner border as raw `#7dd6ca`; that is the accent at half
 * opacity over white, so it is expressed as `accent/50` against the token rather
 * than a magic hex with no place in DESIGN.md.
 *
 * `viewer` has no chip in the design because the mock roster has no viewer. It is
 * the lowest rank, so it takes the most recessive treatment the system already
 * has — sunken surface, secondary text, standard border — instead of borrowing
 * another role's colour and implying a rank that is not there.
 */
const CHIPS: Record<Role, string> = {
  owner: 'bg-[var(--color-accent-tint)] text-[var(--color-accent)] border border-[var(--color-accent)]/50',
  admin: 'bg-blue-50 text-blue-700 border border-blue-200',
  member: 'bg-slate-100 text-slate-700 border border-slate-200',
  viewer: 'bg-[var(--color-surface-sunken)] text-[var(--color-secondary)] border border-[var(--color-border-standard)]',
}

/** `role.owner` → "Pemilik"/"Owner". Typed, so a missing label is a compile error. */
export const ROLE_LABEL: Record<Role, keyof Dictionary> = {
  owner: 'role.owner',
  admin: 'role.admin',
  member: 'role.member',
  viewer: 'role.viewer',
}

export function RoleBadge({ role, className }: { role: Role; className?: string }) {
  const t = useT()
  return (
    <span
      data-role={role}
      className={cn(
        'inline-flex items-center rounded-[6px] px-1.5 py-0.5 font-mono text-[10px] font-bold',
        CHIPS[role],
        className,
      )}
    >
      {t[ROLE_LABEL[role]]}
    </span>
  )
}
