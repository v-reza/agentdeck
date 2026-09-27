import { Link, useParams } from 'react-router-dom'
import { Bell } from 'lucide-react'
import { useListNotificationsQuery } from '@/store/api/notifications'
import { useT } from '@/hooks/use-t'

/**
 * The bell in the topbar's right slot (US-AD61 AC1: "bell icon pojok kanan
 * atas, badge jumlah unread").
 *
 * The count is the server's `unread_count`, not the length of the rendered
 * list: the list is capped, the count is not, so a badge derived from the list
 * would silently stop at the cap and under-report.
 *
 * A zero count renders no badge rather than a badge reading "0" — the design
 * shows a count, and "0" is noise in a 52px bar.
 */
export function NotificationBell() {
  const t = useT()
  const { orgID } = useParams<{ orgID: string }>()
  const { data } = useListNotificationsQuery()
  const unread = data?.unread_count ?? 0
  const base = orgID ? `/app/${orgID}` : '/app'

  return (
    <Link
      to={`${base}/notifications`}
      aria-label={unread > 0 ? `${t['nav.notifications']} (${unread})` : t['nav.notifications']}
      className="relative flex h-8 w-8 items-center justify-center rounded-[6px] text-[var(--color-secondary)] transition-colors hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]"
    >
      <Bell size={15} strokeWidth={2} aria-hidden="true" />
      {unread > 0 ? (
        <span
          data-testid="notification-badge"
          className="absolute -right-0.5 -top-0.5 flex h-[15px] min-w-[15px] items-center justify-center rounded-full bg-[var(--color-danger)] px-1 font-mono text-[9px] font-semibold text-[var(--color-on-danger)]"
        >
          {unread > 99 ? '99+' : unread}
        </span>
      ) : null}
    </Link>
  )
}
