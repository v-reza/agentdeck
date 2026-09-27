import { baseApi } from './base'

/**
 * In-app notifications (US-AD61, ARCHITECTURE 6.2.19).
 *
 * Both endpoints have been live since the notification store landed; nothing in
 * the app called them, so there was no bell, no badge, and no screen.
 *
 * `unread_count` is its own field rather than `notifications.filter(unread)`:
 * the server counts it over every unread row while the list is capped by
 * `limit`, so deriving the badge from the list would under-report the moment
 * the cap is reached.
 *
 * The store is `notifications`, scoped to `user_id AND org_id`. Nothing here
 * needs to filter by tenant — the server already does, and re-filtering client
 * side would only hide a server that stopped.
 */

export interface Notification {
  id: string
  kind: string
  title: string
  body?: string
  /** `task` | `run` | `board` — absent means the notification goes nowhere. */
  target_type?: string
  target_id?: string
  /** Empty while unread; that is the badge's whole input. */
  read_at?: string
  created_at: string
}

export interface NotificationList {
  notifications: Notification[]
  unread_count: number
}

export const notificationsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listNotifications: build.query<NotificationList, number | void>({
      // A cap is sent because the server has one anyway (`ClampAuditLimit`);
      // asking for the default keeps the request honest about what it renders.
      query: (limit) => (limit ? `notifications?limit=${limit}` : 'notifications'),
      providesTags: (result) => [
        ...(result?.notifications ?? []).map((n) => ({ type: 'Notification' as const, id: n.id })),
        { type: 'Notification' as const, id: 'LIST' },
      ],
    }),

    markNotificationsRead: build.mutation<{ marked: number }, { ids?: string[]; all?: boolean }>({
      query: (body) => ({ url: 'notifications/read', method: 'POST', body }),
      invalidatesTags: [{ type: 'Notification', id: 'LIST' }],
    }),
  }),
})

export const { useListNotificationsQuery, useMarkNotificationsReadMutation } = notificationsApi
