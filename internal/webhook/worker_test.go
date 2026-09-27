package webhook

// Tes service dan worker.
//
// Yang diuji di sini adalah keputusan yang tidak kelihatan dari HTTP: secret
// tidak pernah tersimpan terbuka, retry manual benar-benar mengembalikan jatah
// percobaan, dan worker menandai `dead` di percobaan terakhir — bukan `failed`
// yang berarti "akan dicoba lagi".

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- fake store --------------------------------------------------------------

type fakeRepo struct {
	mu         sync.Mutex
	hooks      []Webhook
	deliveries []Delivery
	events     map[int64]Event
	sealed     map[string]string // webhook id -> plaintext yang "terenkripsi"
	failNext   error
	createdN   int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{events: map[int64]Event{}, sealed: map[string]string{}}
}

func (f *fakeRepo) Create(_ context.Context, w Webhook) (Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.CreatedAt = time.Unix(1700000000, 0).UTC()
	f.hooks = append(f.hooks, w)
	f.sealed[w.ID] = string(w.SecretEnc)
	return w, nil
}

func (f *fakeRepo) ListByBoard(_ context.Context, orgID, boardID string) ([]Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Webhook
	for _, h := range f.hooks {
		if h.OrgID == orgID && h.BoardID == boardID {
			out = append(out, h)
		}
	}
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, orgID, id string) (Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, h := range f.hooks {
		if h.ID == id && h.OrgID == orgID {
			return h, nil
		}
	}
	return Webhook{}, ErrNotFound
}

func (f *fakeRepo) Update(_ context.Context, orgID, id, url string, active bool) (Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, h := range f.hooks {
		if h.ID == id && h.OrgID == orgID {
			f.hooks[i].URL = url
			f.hooks[i].Active = active
			return f.hooks[i], nil
		}
	}
	return Webhook{}, ErrNotFound
}

func (f *fakeRepo) Delete(_ context.Context, orgID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, h := range f.hooks {
		if h.ID == id && h.OrgID == orgID {
			f.hooks = append(f.hooks[:i], f.hooks[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeRepo) ListDeliveries(_ context.Context, webhookID string, limit int) ([]Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Delivery
	for _, d := range f.deliveries {
		if d.WebhookID == webhookID {
			out = append(out, d)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) GetDelivery(_ context.Context, webhookID string, id int64) (Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.deliveries {
		if d.ID == id && d.WebhookID == webhookID {
			return d, nil
		}
	}
	return Delivery{}, ErrNotFound
}

func (f *fakeRepo) ResetDelivery(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = StatusPending
			f.deliveries[i].Attempts = 0
			f.deliveries[i].ResponseCode = nil
			f.deliveries[i].LastError = ""
			return nil
		}
	}
	return ErrNotFound
}

// ---- Store (worker) ---------------------------------------------------------

func (f *fakeRepo) Listener(context.Context, string, func(string)) error { return nil }

func (f *fakeRepo) EventByID(_ context.Context, id int64) (Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev, ok := f.events[id]
	if !ok {
		return Event{}, ErrNotFound
	}
	return ev, nil
}

func (f *fakeRepo) MatchingSubscriptions(_ context.Context, boardID, kind string) ([]Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Subscription
	for _, h := range f.hooks {
		if h.BoardID != boardID || !h.Active {
			continue
		}
		if len(h.Events) > 0 && !contains(h.Events, kind) {
			continue
		}
		out = append(out, Subscription{
			ID: h.ID, OrgID: h.OrgID, BoardID: h.BoardID, URL: h.URL, SecretEnc: h.SecretEnc,
		})
	}
	return out, nil
}

func (f *fakeRepo) CreateDelivery(_ context.Context, webhookID string, eventID int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createdN++
	id := int64(f.createdN)
	f.deliveries = append(f.deliveries, Delivery{
		ID: id, WebhookID: webhookID, EventID: eventID,
		Status: StatusPending, CreatedAt: time.Unix(1700000000, 0).UTC(),
	})
	return id, nil
}

func (f *fakeRepo) Retryable(_ context.Context, limit int) ([]Pending, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Pending
	for _, d := range f.deliveries {
		if d.Status != StatusPending && d.Status != StatusFailed {
			continue
		}
		var sub Subscription
		for _, h := range f.hooks {
			if h.ID == d.WebhookID {
				sub = Subscription{ID: h.ID, OrgID: h.OrgID, BoardID: h.BoardID, URL: h.URL, SecretEnc: h.SecretEnc}
			}
		}
		p := Pending{Delivery: d, Subscription: sub}
		// Event hanya ikut kalau masih ada — meniru LEFT JOIN di SQL.
		if ev, ok := f.events[d.EventID]; ok {
			p.Event = ev
		}
		out = append(out, p)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRepo) MarkDelivery(_ context.Context, id int64, status string, attempts int, code *int, lastErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = status
			f.deliveries[i].Attempts = attempts
			f.deliveries[i].ResponseCode = code
			f.deliveries[i].LastError = lastErr
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeRepo) OpenSecret(_ context.Context, sub Subscription) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sealed[sub.ID], nil
}

// discardLogger adalah logger yang membuang output: tes tidak perlu log-nya,
// tapi worker tetap butuh logger yang tidak nil.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// ---- service tests ----------------------------------------------------------

func TestCreateRejectsHTTPToTheInternet(t *testing.T) {
	svc := NewService(newFakeRepo(), func(s string) ([]byte, error) { return []byte(s), nil })
	_, err := svc.Create(context.Background(), "org", "board", CreateInput{
		URL: "http://example.com/hook", Secret: "s",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateAcceptsLoopbackHTTP(t *testing.T) {
	svc := NewService(newFakeRepo(), func(s string) ([]byte, error) { return []byte(s), nil })
	if _, err := svc.Create(context.Background(), "org", "board", CreateInput{
		URL: "http://127.0.0.1:9999/hook", Secret: "s",
	}); err != nil {
		t.Fatalf("loopback http ditolak: %v", err)
	}
}

// TestCreateSealsTheSecret: yang tersimpan harus hasil seal, bukan plaintext.
func TestCreateSealsTheSecret(t *testing.T) {
	repo := newFakeRepo()
	const plain = "super-rahasia"
	svc := NewService(repo, func(s string) ([]byte, error) {
		// Seal tiruan yang jelas bukan plaintext.
		return []byte("SEALED:" + s), nil
	})
	created, err := svc.Create(context.Background(), "org", "board", CreateInput{
		URL: "https://example.com/hook", Secret: plain,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if string(created.SecretEnc) == plain {
		t.Fatal("secret tersimpan sebagai plaintext")
	}
	if string(created.SecretEnc) != "SEALED:"+plain {
		t.Fatalf("secret = %q, want hasil seal", created.SecretEnc)
	}
	if string(repo.sealed[created.ID]) != string(created.SecretEnc) {
		t.Fatal("yang tersimpan di repo bukan hasil seal")
	}
}

// TestCreateWithoutSealerRefuses: tanpa kunci master, tidak ada yang ditulis.
func TestCreateWithoutSealerRefuses(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	_, err := svc.Create(context.Background(), "org", "board", CreateInput{
		URL: "https://example.com/hook", Secret: "s",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRejectsEmptySecret(t *testing.T) {
	svc := NewService(newFakeRepo(), func(s string) ([]byte, error) { return []byte(s), nil })
	_, err := svc.Create(context.Background(), "org", "board", CreateInput{
		URL: "https://example.com/hook", Secret: "",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestNormalizeEventsDedupesAndSorts(t *testing.T) {
	got, err := normalizeEvents([]string{"run.finished", " task.created ", "run.finished"})
	if err != nil {
		t.Fatalf("normalizeEvents: %v", err)
	}
	want := []string{"run.finished", "task.created"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNormalizeEventsRejectsBlankName(t *testing.T) {
	if _, err := normalizeEvents([]string{"a", "  "}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// TestUpdateKeepsURLWhenOmitted: PATCH yang hanya menonaktifkan tidak boleh
// mengosongkan URL.
func TestUpdateKeepsURLWhenOmitted(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, func(s string) ([]byte, error) { return []byte(s), nil })
	created, err := svc.Create(context.Background(), "org", "board", CreateInput{
		URL: "https://example.com/hook", Secret: "s",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	off := false
	updated, err := svc.Update(context.Background(), "org", created.ID, UpdateInput{Active: &off})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.URL != "https://example.com/hook" {
		t.Fatalf("url = %q, want dipertahankan", updated.URL)
	}
	if updated.Active {
		t.Fatal("active masih true")
	}
}

// TestRetryDeliveryResetsAttempts: delivery `dead` harus kembali ke antrean
// dengan jatah penuh, bukan langsung mati lagi.
func TestRetryDeliveryResetsAttempts(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, func(s string) ([]byte, error) { return []byte(s), nil })
	created, _ := svc.Create(context.Background(), "org", "board", CreateInput{
		URL: "https://example.com/hook", Secret: "s",
	})
	repo.deliveries = []Delivery{{
		ID: 1, WebhookID: created.ID, EventID: 5, Status: StatusDead, Attempts: maxAttempts,
	}}
	got, err := svc.RetryDelivery(context.Background(), created.ID, 1)
	if err != nil {
		t.Fatalf("RetryDelivery: %v", err)
	}
	if got.Status != StatusPending || got.Attempts != 0 {
		t.Fatalf("status=%q attempts=%d, want pending/0", got.Status, got.Attempts)
	}
	if repo.deliveries[0].Attempts != 0 {
		t.Fatal("attempts tidak di-reset di penyimpanan")
	}
}

// TestRetryDeliveryRefusesAnotherWebhooksDelivery: id delivery yang benar tapi
// milik webhook lain harus 404.
func TestRetryDeliveryRefusesAnotherWebhooksDelivery(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, func(s string) ([]byte, error) { return []byte(s), nil })
	a, _ := svc.Create(context.Background(), "org", "board", CreateInput{URL: "https://a.example/h", Secret: "s"})
	b, _ := svc.Create(context.Background(), "org", "board", CreateInput{URL: "https://b.example/h", Secret: "s"})
	repo.deliveries = []Delivery{{ID: 7, WebhookID: b.ID, EventID: 1, Status: StatusDead, Attempts: maxAttempts}}

	if _, err := svc.RetryDelivery(context.Background(), a.ID, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ---- worker tests -----------------------------------------------------------

// TestWorkerEnqueuesOneDeliveryPerMatchingWebhook menaku fan-out 13.1.
func TestWorkerEnqueuesOneDeliveryPerMatchingWebhook(t *testing.T) {
	repo := newFakeRepo()
	repo.hooks = []Webhook{
		{ID: "w1", OrgID: "o", BoardID: "b", URL: "https://a.example/h", Active: true},
		{ID: "w2", OrgID: "o", BoardID: "b", URL: "https://b.example/h", Active: true, Events: []string{"other.kind"}},
		{ID: "w3", OrgID: "o", BoardID: "b", URL: "https://c.example/h", Active: false},
		{ID: "w4", OrgID: "o", BoardID: "lain", URL: "https://d.example/h", Active: true},
	}
	repo.events[1] = Event{ID: 1, Kind: "task.created", OrgID: "o", BoardID: "b"}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	w.enqueue(context.Background(), 1)

	if len(repo.deliveries) != 1 {
		t.Fatalf("tercatat %d pengiriman, want 1 (hanya w1 yang cocok)", len(repo.deliveries))
	}
	if repo.deliveries[0].WebhookID != "w1" {
		t.Fatalf("webhook = %q, want w1", repo.deliveries[0].WebhookID)
	}
}

// TestWorkerDeliversWithValidSignature adalah tes intinya: pengiriman nyata ke
// server HTTP, dan HMAC-nya diverifikasi sisi penerima persis seperti resep
// 13.2. Kalau byte yang ditandatangani berbeda dari byte yang dikirim, tes ini
// gagal — dan itu satu-satunya cara membuktikannya.
func TestWorkerDeliversWithValidSignature(t *testing.T) {
	const secret = "rahasia-pelanggan"
	var (
		gotBody []byte
		gotSig  string
		gotCT   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = buf
		gotSig = r.Header.Get(SignatureHeader)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	repo.hooks = []Webhook{{ID: "w1", OrgID: "o", BoardID: "b", URL: srv.URL, Active: true}}
	repo.sealed["w1"] = secret
	repo.events[1] = Event{ID: 1, Kind: "task.created", OrgID: "o", BoardID: "b", Payload: []byte(`{"a":1}`)}
	repo.deliveries = []Delivery{{ID: 1, WebhookID: "w1", EventID: 1, Status: StatusPending, CreatedAt: time.Unix(1700000000, 0).UTC()}}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	w.drain(context.Background())

	if !VerifySignature(secret, gotBody, gotSig) {
		t.Fatalf("signature tidak cocok dengan body yang diterima (sig=%q body=%s)", gotSig, gotBody)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if repo.deliveries[0].Status != StatusDelivered {
		t.Fatalf("status = %q, want delivered", repo.deliveries[0].Status)
	}
	if repo.deliveries[0].Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", repo.deliveries[0].Attempts)
	}
}

// TestWorkerMarks4xxFailedWithoutRetry menaku 13.3: klien menolak, jangan ulangi.
func TestWorkerMarks4xxFailedWithoutRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	repo.hooks = []Webhook{{ID: "w1", OrgID: "o", BoardID: "b", URL: srv.URL, Active: true}}
	repo.sealed["w1"] = "s"
	repo.events[1] = Event{ID: 1, Kind: "task.created", BoardID: "b"}
	repo.deliveries = []Delivery{{ID: 1, WebhookID: "w1", EventID: 1, Status: StatusPending, CreatedAt: time.Unix(1700000000, 0).UTC()}}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	w.drain(context.Background())

	if repo.deliveries[0].Status != StatusFailed {
		t.Fatalf("status = %q, want failed", repo.deliveries[0].Status)
	}
	if repo.deliveries[0].ResponseCode == nil || *repo.deliveries[0].ResponseCode != 404 {
		t.Fatalf("response_code = %v, want 404", repo.deliveries[0].ResponseCode)
	}
	// 13.3: 4xx tidak di-retry. Statusnya `failed` (persis yang diminta 13.3),
	// tapi driver retry harus melewatkannya — dan itu diukur langsung lewat
	// due(), bukan disimpulkan dari status.
	pending := Pending{Delivery: repo.deliveries[0]}
	if w.due(pending, time.Unix(1700000000, 0).UTC().Add(72*time.Hour)) {
		t.Fatal("delivery 4xx dianggap jatuh tempo — akan di-retry, padahal 13.3 melarangnya")
	}
}

// TestWorkerRetries5xxThenGoesDead menaku jalur 5xx sampai dead-letter.
func TestWorkerRetries5xxThenGoesDead(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	repo.hooks = []Webhook{{ID: "w1", OrgID: "o", BoardID: "b", URL: srv.URL, Active: true}}
	repo.sealed["w1"] = "s"
	repo.events[1] = Event{ID: 1, Kind: "task.created", BoardID: "b"}
	repo.deliveries = []Delivery{{ID: 1, WebhookID: "w1", EventID: 1, Status: StatusPending, CreatedAt: time.Unix(1700000000, 0).UTC()}}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	// Waktu maju supaya setiap percobaan jatuh tempo tanpa tidur.
	now := time.Unix(1700000000, 0).UTC()
	w.now = func() time.Time { return now }

	for i := 0; i < maxAttempts; i++ {
		w.drain(context.Background())
		// Majukan jam melewati jeda terpanjang supaya percobaan berikutnya
		// selalu jatuh tempo.
		now = now.Add(3 * time.Hour)
	}

	if repo.deliveries[0].Attempts != maxAttempts {
		t.Fatalf("attempts = %d, want %d", repo.deliveries[0].Attempts, maxAttempts)
	}
	if repo.deliveries[0].Status != StatusDead {
		t.Fatalf("status = %q, want dead setelah %d percobaan", repo.deliveries[0].Status, maxAttempts)
	}
	if calls != maxAttempts {
		t.Fatalf("server dipanggil %d kali, want %d", calls, maxAttempts)
	}
}

// TestWorkerSkipsDeliveriesNotYetDue: jadwal backoff benar-benar menahan.
func TestWorkerSkipsDeliveriesNotYetDue(t *testing.T) {
	repo := newFakeRepo()
	repo.hooks = []Webhook{{ID: "w1", OrgID: "o", BoardID: "b", URL: "https://a.example/h", Active: true}}
	repo.sealed["w1"] = "s"
	repo.events[1] = Event{ID: 1, Kind: "task.created", BoardID: "b"}
	now := time.Unix(1700000000, 0).UTC()
	repo.deliveries = []Delivery{{
		ID: 1, WebhookID: "w1", EventID: 1, Status: StatusFailed,
		Attempts: 1, CreatedAt: now,
	}}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	w.now = func() time.Time { return now.Add(30 * time.Second) } // belum 1 menit
	w.drain(context.Background())

	if repo.deliveries[0].Attempts != 1 {
		t.Fatalf("attempts = %d, want tetap 1 (belum jatuh tempo)", repo.deliveries[0].Attempts)
	}
}

// TestWorkerMarksDeadWhenEventPurged: event yang sudah kena retensi 30 hari
// tidak boleh menggantung selamanya sebagai pending.
func TestWorkerMarksDeadWhenEventPurged(t *testing.T) {
	repo := newFakeRepo()
	repo.hooks = []Webhook{{ID: "w1", OrgID: "o", BoardID: "b", URL: "https://a.example/h", Active: true}}
	repo.sealed["w1"] = "s"
	// Tidak ada event dengan id 1 — meniru LEFT JOIN tanpa baris events.
	repo.deliveries = []Delivery{{ID: 1, WebhookID: "w1", EventID: 1, Status: StatusPending, CreatedAt: time.Unix(1700000000, 0).UTC()}}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	w.drain(context.Background())

	if repo.deliveries[0].Status != StatusDead {
		t.Fatalf("status = %q, want dead", repo.deliveries[0].Status)
	}
	if !strings.Contains(repo.deliveries[0].LastError, "purge") {
		t.Fatalf("last_error = %q, want menyebut purge", repo.deliveries[0].LastError)
	}
}

// TestWorkerTruncatesLongErrors: kolom last_error tidak tumbuh tanpa kendali.
func TestWorkerTruncatesLongErrors(t *testing.T) {
	repo := newFakeRepo()
	repo.hooks = []Webhook{{ID: "w1", OrgID: "o", BoardID: "b", URL: "http://127.0.0.1:1/h", Active: true}}
	repo.sealed["w1"] = "s"
	repo.events[1] = Event{ID: 1, Kind: "task.created", BoardID: "b"}
	repo.deliveries = []Delivery{{ID: 1, WebhookID: "w1", EventID: 1, Status: StatusPending, CreatedAt: time.Unix(1700000000, 0).UTC()}}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	w.drain(context.Background())

	if len(repo.deliveries[0].LastError) > 500 {
		t.Fatalf("last_error panjangnya %d, want <= 500", len(repo.deliveries[0].LastError))
	}
}

// TestDeliveryBodyCarriesEventFields: body harus memuat kind dan board dari
// event, bukan dari langganan.
func TestDeliveryBodyCarriesEventFields(t *testing.T) {
	body, err := BuildBody(Event{ID: 42, Kind: "run.finished", OrgID: "o1", BoardID: "b1", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatalf("BuildBody: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Kind != "run.finished" || env.BoardID != "b1" || env.ID != 42 {
		t.Fatalf("envelope = %+v", env)
	}
}
