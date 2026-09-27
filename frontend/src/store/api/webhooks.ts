import { baseApi } from './base'

/**
 * Webhooks (US-AD52, US-AD53, ARCHITECTURE 6.2.18 / 3.22, 3.23).
 *
 * Seven endpoints, all Admin-gated. Three hang off a board
 * (`/boards/{board_id}/webhooks`), four off the webhook itself
 * (`/webhooks/{id}`) — the latter scope the tenant from the stored `org_id`
 * because there is no board in their path.
 *
 * Two things the design draws and the API does not carry, so neither is faked:
 *
 *  - **The secret.** `CreateInput` accepts it; `webhookResponse` has no field
 *    for it and there is no reveal endpoint. A masked `whsec_••••` column would
 *    be an invented value. The secret is write-only: typed once at creation,
 *    then gone.
 *  - **Last delivery per row.** `webhookResponse` has no delivery field, and
 *    fetching deliveries per row to fill one table cell costs a request per
 *    row. Deliveries live in their own panel, fetched when it is opened.
 *
 * `events_json` empty is not "no events": `ListMatchingWebhooks` reads an empty
 * array as every event. The UI says so rather than rendering an empty chip.
 */

/** `webhookResponse`. `events_json` is `[]` for "every event". */
export interface Webhook {
  id: string
  org_id: string
  board_id: string
  url: string
  events_json: string[]
  active: boolean
  created_at: string
}

export interface WebhookDelivery {
  id: number
  webhook_id: string
  event_id: number
  /** `pending` | `delivered` | `failed` | `dead` (migration 0021's CHECK). */
  status: string
  attempts: number
  response_code: number | null
  last_error: string
  created_at: string
}

export interface CreateWebhookArgs {
  boardID: string
  url: string
  secret: string
  events: string[]
}

export const webhooksApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listWebhooks: build.query<Webhook[], string>({
      query: (boardID) => `boards/${boardID}/webhooks`,
      providesTags: (result) => [
        ...(result ?? []).map((w) => ({ type: 'Webhook' as const, id: w.id })),
        { type: 'Webhook' as const, id: 'LIST' },
      ],
    }),

    createWebhook: build.mutation<Webhook, CreateWebhookArgs>({
      query: ({ boardID, url, secret, events }) => ({
        url: `boards/${boardID}/webhooks`,
        method: 'POST',
        // `events_json` is the request field name too — `CreateInput` tags it
        // that way, and the handler decodes into that struct directly.
        body: { url, secret, events_json: events },
      }),
      invalidatesTags: [{ type: 'Webhook', id: 'LIST' }],
    }),

    /**
     * Only `url` and `active` are updatable — `UpdateInput` has no field for
     * events or secret, so this cannot change what a webhook watches or how it
     * is signed. Replacing those means deleting and re-creating.
     */
    patchWebhook: build.mutation<Webhook, { id: string; url?: string; active?: boolean }>({
      query: ({ id, ...body }) => ({ url: `webhooks/${id}`, method: 'PATCH', body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'Webhook', id },
        { type: 'Webhook', id: 'LIST' },
      ],
    }),

    deleteWebhook: build.mutation<void, string>({
      query: (id) => ({ url: `webhooks/${id}`, method: 'DELETE' }),
      invalidatesTags: [{ type: 'Webhook', id: 'LIST' }],
    }),

    listWebhookDeliveries: build.query<WebhookDelivery[], string>({
      query: (id) => `webhooks/${id}/deliveries`,
      providesTags: (_r, _e, id) => [{ type: 'Webhook', id: `DELIVERIES-${id}` }],
    }),

    retryWebhookDelivery: build.mutation<WebhookDelivery, { webhookID: string; deliveryID: number }>({
      query: ({ webhookID, deliveryID }) => ({
        url: `webhooks/${webhookID}/deliveries/${deliveryID}/retry`,
        method: 'POST',
      }),
      invalidatesTags: (_r, _e, { webhookID }) => [{ type: 'Webhook', id: `DELIVERIES-${webhookID}` }],
    }),
  }),
})

export const {
  useListWebhooksQuery,
  useCreateWebhookMutation,
  usePatchWebhookMutation,
  useDeleteWebhookMutation,
  useListWebhookDeliveriesQuery,
  useRetryWebhookDeliveryMutation,
} = webhooksApi
