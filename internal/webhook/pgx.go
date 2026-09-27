package webhook

// Adapter Postgres — Repo (CRUD 6.2.18) dan Store (worker 13.1).

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/store"
)

// pgxRepo memenuhi Repo dan Store sekaligus: keduanya berbicara ke tabel yang
// sama, dan memisahkannya jadi dua tipe berarti dua tempat yang harus tahu
// daftar kolomnya.
type pgxRepo struct {
	q    *store.Queries
	pool *pgxpool.Pool
	// openSecret membuka secret terenkripsi. Diteruskan dari wiring supaya
	// paket ini tidak mengimpor internal/crypto dan tesnya bisa memakai kunci
	// nyata tanpa menyentuh environment.
	openSecret func(sealed []byte) (string, error)
}

// NewPgxRepository membangun Repo dan Store berbasis Postgres.
func NewPgxRepository(pool *pgxpool.Pool, openSecret func([]byte) (string, error)) *pgxRepo {
	return &pgxRepo{q: store.New(pool), pool: pool, openSecret: openSecret}
}

func (r *pgxRepo) Create(ctx context.Context, w Webhook) (Webhook, error) {
	row, err := r.q.CreateWebhook(ctx, store.CreateWebhookParams{
		ID:         w.ID,
		OrgID:      w.OrgID,
		BoardID:    w.BoardID,
		Url:        w.URL,
		SecretEnc:  w.SecretEnc,
		EventsJson: EventsJSON(w.Events),
		Active:     w.Active,
	})
	if err != nil {
		return Webhook{}, err
	}
	return webhookFrom(row), nil
}

func (r *pgxRepo) ListByBoard(ctx context.Context, orgID, boardID string) ([]Webhook, error) {
	rows, err := r.q.ListBoardWebhooks(ctx, store.ListBoardWebhooksParams{BoardID: boardID, OrgID: orgID})
	if err != nil {
		return nil, err
	}
	out := make([]Webhook, 0, len(rows))
	for _, row := range rows {
		out = append(out, webhookFromListRow(row))
	}
	return out, nil
}

func (r *pgxRepo) Get(ctx context.Context, orgID, id string) (Webhook, error) {
	row, err := r.q.GetWebhook(ctx, store.GetWebhookParams{ID: id, OrgID: orgID})
	if err != nil {
		return Webhook{}, noRows(err)
	}
	return webhookFromGetRow(row), nil
}

func (r *pgxRepo) Update(ctx context.Context, orgID, id, url string, active bool) (Webhook, error) {
	row, err := r.q.UpdateWebhook(ctx, store.UpdateWebhookParams{
		ID: id, OrgID: orgID, Url: url, Active: active,
	})
	if err != nil {
		return Webhook{}, noRows(err)
	}
	return webhookFromUpdateRow(row), nil
}

func (r *pgxRepo) Delete(ctx context.Context, orgID, id string) error {
	n, err := r.q.DeleteWebhook(ctx, store.DeleteWebhookParams{ID: id, OrgID: orgID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepo) ListDeliveries(ctx context.Context, webhookID string, limit int) ([]Delivery, error) {
	rows, err := r.q.ListWebhookDeliveries(ctx, store.ListWebhookDeliveriesParams{
		WebhookID: webhookID, Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Delivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, deliveryFrom(row))
	}
	return out, nil
}

func (r *pgxRepo) GetDelivery(ctx context.Context, webhookID string, id int64) (Delivery, error) {
	row, err := r.q.GetWebhookDelivery(ctx, id)
	if err != nil {
		return Delivery{}, noRows(err)
	}
	// Scoping: delivery id yang benar tapi milik webhook lain harus tetap 404,
	// jadi kepemilikannya diperiksa di sini, bukan di query.
	if row.WebhookID != webhookID {
		return Delivery{}, ErrNotFound
	}
	return deliveryFrom(row), nil
}

func (r *pgxRepo) ResetDelivery(ctx context.Context, id int64) error {
	return r.q.ResetWebhookDelivery(ctx, id)
}

// ---- Store (worker 13.1) ----------------------------------------------------

func (r *pgxRepo) EventByID(ctx context.Context, id int64) (Event, error) {
	row, err := r.q.GetEvent(ctx, id)
	if err != nil {
		return Event{}, noRows(err)
	}
	return eventFrom(row), nil
}

func (r *pgxRepo) MatchingSubscriptions(ctx context.Context, boardID, kind string) ([]Subscription, error) {
	rows, err := r.q.ListMatchingWebhooks(ctx, store.ListMatchingWebhooksParams{BoardID: boardID, Column2: kind})
	if err != nil {
		return nil, err
	}
	out := make([]Subscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, Subscription{
			ID: row.ID, OrgID: row.OrgID, BoardID: row.BoardID,
			URL: row.Url, SecretEnc: row.SecretEnc,
		})
	}
	return out, nil
}

func (r *pgxRepo) CreateDelivery(ctx context.Context, webhookID string, eventID int64) (int64, error) {
	row, err := r.q.CreateWebhookDelivery(ctx, store.CreateWebhookDeliveryParams{
		WebhookID: webhookID, EventID: eventID,
	})
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *pgxRepo) Retryable(ctx context.Context, limit int) ([]Pending, error) {
	rows, err := r.q.ListRetryableDeliveries(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]Pending, 0, len(rows))
	for _, row := range rows {
		p := Pending{
			Delivery: Delivery{
				ID: row.ID, WebhookID: row.WebhookID, EventID: row.EventID,
				Status: row.Status, Attempts: int(row.Attempts),
				CreatedAt: row.CreatedAt.Time,
			},
			Subscription: Subscription{
				ID: row.WebhookID, OrgID: row.OrgID, URL: row.Url, SecretEnc: row.SecretEnc,
			},
		}
		// LEFT JOIN: event bisa sudah dipurge retensi 30 hari (3.24), dan
		// Kind-nya NULL di baris itu. ID tetap diisi supaya worker tahu baris
		// ini punya event yang hilang, bukan tidak punya event sama sekali.
		if row.Kind != nil {
			p.Event = Event{
				ID: row.EventID, Kind: *row.Kind,
				OrgID: row.OrgID, BoardID: derefString(row.BoardID),
				TaskID: derefString(row.TaskID), RunID: derefString(row.RunID),
				Payload: row.PayloadJson, CreatedAt: row.EventCreatedAt.Time,
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *pgxRepo) MarkDelivery(ctx context.Context, id int64, status string, attempts int, code *int, lastErr string) error {
	var responseCode *int16
	if code != nil {
		c := int16(*code)
		responseCode = &c
	}
	return r.q.UpdateWebhookDelivery(ctx, store.UpdateWebhookDeliveryParams{
		ID: id, Status: status, Attempts: int16(attempts),
		ResponseCode: responseCode, LastError: &lastErr,
	})
}

func (r *pgxRepo) OpenSecret(ctx context.Context, sub Subscription) (string, error) {
	if r.openSecret == nil {
		return "", errors.New("master key tidak tersedia")
	}
	return r.openSecret(sub.SecretEnc)
}

// Listener menjalankan LISTEN pada satu koneksi khusus.
//
// Koneksinya SELALU dikembalikan lewat defer: koneksi yang bocor menghabiskan
// pool setelah beberapa kali reconnect, dan gejalanya muncul jauh dari sini.
func (r *pgxRepo) Listener(ctx context.Context, channel string, fn func(payload string)) error {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+quoteIdent(channel)); err != nil {
		return err
	}
	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}
		fn(notification.Payload)
	}
}

func quoteIdent(name string) string { return `"` + name + `"` }

func noRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ---- row mapping ------------------------------------------------------------

func eventsFromJSON(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func eventFrom(row store.Event) Event {
	return Event{
		ID: row.ID, Kind: row.Kind, OrgID: row.OrgID,
		BoardID: derefString(row.BoardID), TaskID: derefString(row.TaskID), RunID: derefString(row.RunID),
		Payload: row.PayloadJson, CreatedAt: row.CreatedAt.Time,
	}
}

func deliveryFrom(row store.WebhookDelivery) Delivery {
	var code *int
	if row.ResponseCode != nil {
		c := int(*row.ResponseCode)
		code = &c
	}
	lastErr := ""
	if row.LastError != nil {
		lastErr = *row.LastError
	}
	return Delivery{
		ID: row.ID, WebhookID: row.WebhookID, EventID: row.EventID,
		Status: row.Status, Attempts: int(row.Attempts),
		ResponseCode: code, LastError: lastErr, CreatedAt: row.CreatedAt.Time,
	}
}

func webhookFrom(row store.CreateWebhookRow) Webhook {
	return Webhook{
		ID: row.ID, OrgID: row.OrgID, BoardID: row.BoardID, URL: row.Url,
		Events: eventsFromJSON(row.EventsJson), Active: row.Active,
		CreatedAt: row.CreatedAt.Time,
	}
}

func webhookFromListRow(row store.ListBoardWebhooksRow) Webhook {
	return Webhook{
		ID: row.ID, OrgID: row.OrgID, BoardID: row.BoardID, URL: row.Url,
		Events: eventsFromJSON(row.EventsJson), Active: row.Active,
		CreatedAt: row.CreatedAt.Time,
	}
}

func webhookFromGetRow(row store.Webhook) Webhook {
	return Webhook{
		ID: row.ID, OrgID: row.OrgID, BoardID: row.BoardID, URL: row.Url,
		SecretEnc: row.SecretEnc, Events: eventsFromJSON(row.EventsJson),
		Active: row.Active, CreatedAt: row.CreatedAt.Time,
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func webhookFromUpdateRow(row store.UpdateWebhookRow) Webhook {
	return Webhook{
		ID: row.ID, OrgID: row.OrgID, BoardID: row.BoardID, URL: row.Url,
		Events: eventsFromJSON(row.EventsJson), Active: row.Active,
		CreatedAt: row.CreatedAt.Time,
	}
}
