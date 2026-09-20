import { cn } from '@/lib/cn'
import type { AvatarUser } from '@/lib/domain'

/**
 * The user monogram (US-AD89 AC1).
 *
 * The values are the *server's*: `avatar_user` carries `initials`, `bg_color`,
 * and `size_px`, and this component renders them verbatim. The alternative —
 * deriving initials from the name on the client — was what the sidebar used to
 * do, and it disagreed with the API for any multi-word name ("Reza Probe" drew
 * "R" where the endpoint said "RP"). One source of truth, and it is the payload.
 *
 * `fallbackName` is used only while the session is still resolving, so the rail
 * does not flash an empty circle. Once `avatar` exists it always wins.
 */
export interface AvatarMonogramProps {
  avatar: AvatarUser | null
  fallbackName?: string | null
  /** The design's ring for the rail's active avatar; off everywhere else. */
  ring?: boolean
  /** Larger status dot for the rail footer. */
  className?: string
}

export function AvatarMonogram({ avatar, fallbackName, ring = false, className }: AvatarMonogramProps) {
  const size = avatar?.size_px ?? 26
  const label = avatar?.initials || firstLetter(fallbackName)

  if (avatar?.kind === 'image' && avatar.url) {
    return (
      <img
        src={avatar.url}
        alt={label}
        width={size}
        height={size}
        style={{ width: size, height: size }}
        className={cn('shrink-0 rounded-full border border-white object-cover', className)}
      />
    )
  }

  return (
    <span
      data-testid="profile-avatar-disc"
      aria-hidden={label === ''}
      style={{ width: size, height: size, backgroundColor: avatar?.bg_color ?? '#101014' }}
      className={cn(
        'flex shrink-0 items-center justify-center rounded-full border border-white text-[10px] font-bold text-white',
        ring ? 'shadow-sm ring-2 ring-[var(--color-accent)]' : null,
        className,
      )}
    >
      {label}
    </span>
  )
}

/** The first character of a display name, for the pre-resolution placeholder. */
function firstLetter(name?: string | null): string {
  const trimmed = (name ?? '').trim()
  return trimmed ? trimmed.slice(0, 1).toUpperCase() : '?'
}
