package sse

// Tes unit hub — 7.1-7.4.
//
// Yang diuji di sini adalah bagian yang tidak bisa dilihat dari endpoint: bentuk
// frame, penolakan checkpoint yang tidak masuk akal, scoping fan-out, dan
// kebijakan slow-consumer. Ketiganya adalah keputusan yang kalau salah tidak
// menghasilkan error — hanya stream yang diam atau event yang bocor ke tenant
// lain.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	events map[int64]Event
	after  []Event
	// notifyErr memaksa Listener mengembalikan error, untuk menguji jalur gagal.
	notifyErr error
}

func (f *fakeStore) GetEvent(_ context.Context, id int64) (Event, error) {
	ev, ok := f.events[id]
	if !ok {
		return Event{}, errors.New("no event")
	}
	return ev, nil
}

func (f *fakeStore) ListBoardEventsAfter(_ context.Context, _, _ string, _ int64, _ int) ([]Event, error) {
	return f.after, nil
}

func (f *fakeStore) Listener(ctx context.Context, _ string, _ func(string)) error {
	if f.notifyErr != nil {
		return f.notifyErr
	}
	<-ctx.Done()
	return nil
}

type fakeMetrics struct {
	sets map[string]float64
	adds map[string]float64
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{sets: map[string]float64{}, adds: map[string]float64{}}
}

func (m *fakeMetrics) Set(name string, v float64, _ ...string) { m.sets[name] = v }
func (m *fakeMetrics) Add(name string, d float64, _ ...string) { m.adds[name] += d }

func testHub(t *testing.T, store Store) (*Hub, *fakeMetrics) {
	t.Helper()
	m := newFakeMetrics()
	h := NewHub(store, slog.New(slog.NewTextHandler(io.Discard, nil)), m, "agentdeck_events")
	h.pingInterval = 10 * time.Millisecond
	return h, m
}

// newRequestWith merakit request dengan header/query yang diuji.
func newRequestWith(t *testing.T, header, query string) *http.Request {
	t.Helper()
	target := "/api/v1/boards/b1/events"
	if query != "" {
		target += "?last_event_id=" + query
	}
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if header != "" {
		r.Header.Set("Last-Event-ID", header)
	}
	return r
}

// TestFormatFrameMatchesTheW3CShape pins 7.1: `id`, `event`, `data`, and the
// blank line that terminates a frame. Without the blank line a client never
// considers the frame complete and nothing renders.
func TestFormatFrameMatchesTheW3CShape(t *testing.T) {
	frame := string(FormatFrame(Event{
		ID: 1048576, Kind: "task.status_changed",
		Payload: json.RawMessage(`{"task_id":"t1","to":"running"}`),
	}))

	if !strings.HasPrefix(frame, "id: 1048576\n") {
		t.Fatalf("frame tidak diawali id: %q", frame)
	}
	if !strings.Contains(frame, "event: task.status_changed\n") {
		t.Fatalf("frame tidak memuat event: %q", frame)
	}
	if !strings.Contains(frame, `data: {"task_id":"t1","to":"running"}`+"\n") {
		t.Fatalf("frame tidak memuat data: %q", frame)
	}
	if !strings.HasSuffix(frame, "\n\n") {
		t.Fatalf("frame tidak diakhiri baris kosong: %q", frame)
	}
}

// TestFormatFrameKeepsDataOnOneLine: payload JSON tidak boleh mengandung
// newline, karena satu baris `data:` yang berisi newline memecah frame dan
// klien menerima JSON yang terpotong.
func TestFormatFrameKeepsDataOnOneLine(t *testing.T) {
	frame := string(FormatFrame(Event{
		ID: 7, Kind: "run.finished",
		Payload: json.RawMessage("{\"a\":1,\n\"b\":2}"),
	}))
	for _, line := range strings.Split(strings.TrimSuffix(frame, "\n\n"), "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Count(frame, "data: ") != 1 {
			t.Fatalf("frame punya lebih dari satu baris data: %q", frame)
		}
	}
	if strings.Count(frame, "data: ") != 1 {
		t.Fatalf("frame punya %d baris data, want 1: %q", strings.Count(frame, "data: "), frame)
	}
}

// TestFormatFrameDefaultsEmptyPayload: payload kosong jadi `{}`, bukan `data: `
// kosong yang akan gagal di-parse klien.
func TestFormatFrameDefaultsEmptyPayload(t *testing.T) {
	if frame := string(FormatFrame(Event{ID: 1, Kind: "x"})); !strings.Contains(frame, "data: {}\n") {
		t.Fatalf("payload kosong tidak jadi {}: %q", frame)
	}
}

// TestLastEventIDReadsBothSources: 7.2 — header lebih dulu, query param sebagai
// cadangan untuk klien yang memakai fetch manual.
func TestLastEventIDReadsBothSources(t *testing.T) {
	cases := []struct {
		name   string
		header string
		query  string
		want   int64
		wantOK bool
	}{
		{"header", "42", "", 42, true},
		{"query", "", "42", 42, true},
		{"header menang", "42", "7", 42, true},
		{"nol tetap checkpoint", "", "0", 0, true},
		{"header nol", "0", "", 0, true},
		{"tanpa keduanya", "", "", 0, false},
		{"bukan angka", "abc", "", 0, false},
		{"negatif", "-5", "", 0, false},
		{"spasi", " 42 ", "", 42, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRequestWith(t, c.header, c.query)
			got, ok := LastEventID(r)
			if got != c.want || ok != c.wantOK {
				t.Fatalf("LastEventID = (%d, %v), want (%d, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}

// TestBroadcastOnlyReachesTheMatchingBoardAndOrg is the tenant boundary of the
// hub. A bug here leaks one workspace's events into another's stream, and no
// HTTP-level test would catch it because the stream still "works".
func TestBroadcastOnlyReachesTheMatchingBoardAndOrg(t *testing.T) {
	h, _ := testHub(t, &fakeStore{})

	mine := h.Subscribe("org-1", "board-1")
	otherBoard := h.Subscribe("org-1", "board-2")
	otherOrg := h.Subscribe("org-2", "board-1")
	defer h.Unsubscribe(otherBoard)
	defer h.Unsubscribe(otherOrg)
	defer h.Unsubscribe(mine)

	h.broadcast(Event{ID: 1, OrgID: "org-1", BoardID: "board-1", Kind: "task.created",
		Payload: json.RawMessage("{}")})

	select {
	case <-mine.Events():
	default:
		t.Fatal("klien yang cocok tidak menerima event")
	}
	for name, sub := range map[string]*Subscriber{"board lain": otherBoard, "org lain": otherOrg} {
		select {
		case <-sub.Events():
			t.Fatalf("%s menerima event yang bukan miliknya", name)
		default:
		}
	}
}

// TestSlowConsumerIsDroppedNotBlocked is 7.3: a client whose 128-event buffer
// fills is closed, so one stuck browser cannot stall fan-out for everyone else.
func TestSlowConsumerIsDroppedNotBlocked(t *testing.T) {
	h, m := testHub(t, &fakeStore{})
	slow := h.Subscribe("org-1", "board-1")
	// Sengaja tidak dibaca: buffer-nya penuh.
	for i := 0; i < clientBuffer+5; i++ {
		h.broadcast(Event{ID: int64(i), OrgID: "org-1", BoardID: "board-1", Kind: "task.created"})
	}

	if h.Clients() != 0 {
		t.Fatalf("klien lambat masih terdaftar: %d", h.Clients())
	}
	if m.adds["agentdeck_sse_dropped_total"] == 0 {
		t.Fatal("drop tidak tercatat di metrik")
	}
	// Channel-nya ditutup. Pembaca masih melihat frame yang sudah ada di buffer
	// (menutup channel tidak membuang isinya), lalu ok=false — dan itulah yang
	// membuat handler berhenti dan klien reconnect.
	drained, closed := 0, false
	for i := 0; i < clientBuffer+10; i++ {
		if _, ok := <-slow.Events(); !ok {
			closed = true
			break
		}
		drained++
	}
	if !closed {
		t.Fatalf("channel klien yang di-drop tidak pernah tertutup (drain %d)", drained)
	}
	if drained == 0 {
		t.Fatal("tidak ada frame yang sempat terkirim ke klien lambat")
	}
	// Unsubscribe setelah drop tidak boleh panic (double close).
	h.Unsubscribe(slow)
}

// TestBroadcastDoesNotWaitForAStuckClient adalah separuh lain dari 7.3.
//
// Tes slow-consumer di atas memaku bahwa klien macet AKHIRNYA di-drop. Tes ini
// memaku bahwa fan-out tidak MENUNGGU klien itu dulu: implementasi blocking yang
// tetap drop setelah timeout akan lolos tes di atas sambil menahan setiap klien
// lain selama timeout itu.
//
// Yang diukur adalah broadcast-nya SENDIRI harus kembali cepat. Mengukur "klien
// sehat menerima frame" saja tidak cukup: `subs` adalah map, jadi urutan
// kunjungannya acak — kalau klien sehat kebetulan dikunjungi lebih dulu, tesnya
// lolos meski broadcast kemudian menggantung di klien macet.
func TestBroadcastDoesNotWaitForAStuckClient(t *testing.T) {
	h, _ := testHub(t, &fakeStore{})
	stuck := h.Subscribe("org-1", "board-1")
	defer h.Unsubscribe(stuck)

	// Penuhkan buffer klien macet tanpa pernah membacanya.
	for i := 0; i < clientBuffer; i++ {
		h.broadcast(Event{ID: int64(i), OrgID: "org-1", BoardID: "board-1", Kind: "task.created"})
	}

	done := make(chan struct{})
	go func() {
		h.broadcast(Event{ID: 999, OrgID: "org-1", BoardID: "board-1", Kind: "task.created"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast menggantung di klien yang buffer-nya penuh — fan-out ikut menunggu")
	}
}

// TestUnsubscribeIsIdempotent: handler memanggilnya lewat defer, dan jalur
// slow-consumer sudah menghapus kliennya — jadi panggilan kedua harus aman.
func TestUnsubscribeIsIdempotent(t *testing.T) {
	h, _ := testHub(t, &fakeStore{})
	sub := h.Subscribe("org-1", "board-1")
	if h.Clients() != 1 {
		t.Fatalf("Clients = %d, want 1", h.Clients())
	}
	h.Unsubscribe(sub)
	h.Unsubscribe(sub)
	if h.Clients() != 0 {
		t.Fatalf("Clients = %d, want 0", h.Clients())
	}
}

// TestReplaySendsEventsInOrder: 7.2 — backlog dikirim sinkron sebelum koneksi
// didaftarkan, dan urutannya ASC supaya klien tidak melompat mundur.
func TestReplaySendsEventsInOrder(t *testing.T) {
	store := &fakeStore{after: []Event{
		{ID: 10, OrgID: "org-1", BoardID: "board-1", Kind: "a", Payload: json.RawMessage("{}")},
		{ID: 11, OrgID: "org-1", BoardID: "board-1", Kind: "b", Payload: json.RawMessage("{}")},
	}}
	h, _ := testHub(t, store)

	var got []string
	err := h.Replay(context.Background(), "org-1", "board-1", 9, func(frame []byte) error {
		got = append(got, string(frame))
		return nil
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("replay mengirim %d frame, want 2", len(got))
	}
	if !strings.HasPrefix(got[0], "id: 10\n") || !strings.HasPrefix(got[1], "id: 11\n") {
		t.Fatalf("urutan replay salah: %q", got)
	}
}

// TestReplayStopsOnWriteError: kalau klien pergi di tengah replay, sisanya tidak
// perlu dikirim — dan error-nya harus sampai ke pemanggil, bukan ditelan.
func TestReplayStopsOnWriteError(t *testing.T) {
	store := &fakeStore{after: []Event{
		{ID: 1, OrgID: "o", BoardID: "b", Kind: "a"},
		{ID: 2, OrgID: "o", BoardID: "b", Kind: "b"},
	}}
	h, _ := testHub(t, store)

	calls := 0
	sentinel := errors.New("klien pergi")
	err := h.Replay(context.Background(), "o", "b", 0, func([]byte) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Replay error = %v, want sentinel", err)
	}
	if calls != 1 {
		t.Fatalf("Replay memanggil send %d kali setelah error, want 1", calls)
	}
}

// TestHeartbeatIsAComment: `: ping` harus jadi komentar SSE, bukan event —
// kalau jadi event, klien yang menghitung event akan salah hitung.
func TestHeartbeatIsAComment(t *testing.T) {
	hb := string(Heartbeat())
	if !strings.HasPrefix(hb, ": ") {
		t.Fatalf("heartbeat bukan komentar: %q", hb)
	}
	if !strings.HasSuffix(hb, "\n\n") {
		t.Fatalf("heartbeat tidak diakhiri baris kosong: %q", hb)
	}
	if strings.Contains(hb, "event:") || strings.Contains(hb, "data:") {
		t.Fatalf("heartbeat punya field event/data: %q", hb)
	}
}

// TestRunPropagatesListenerFailure: hub tanpa LISTEN hanya mengirim heartbeat.
// Itu harus terlihat sebagai error, bukan diam.
func TestRunPropagatesListenerFailure(t *testing.T) {
	boom := errors.New("tidak bisa LISTEN")
	h, _ := testHub(t, &fakeStore{notifyErr: boom})
	if err := h.Run(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Run = %v, want %v", err, boom)
	}
}

// TestRunDeliversNotifiedEvents: jalur penuh NOTIFY → GetEvent → broadcast.
func TestRunDeliversNotifiedEvents(t *testing.T) {
	store := &fakeStore{events: map[int64]Event{
		5: {ID: 5, OrgID: "org-1", BoardID: "board-1", Kind: "task.created",
			Payload: json.RawMessage(`{"task_id":"t9"}`)},
	}}
	h, _ := testHub(t, store)
	sub := h.Subscribe("org-1", "board-1")
	defer h.Unsubscribe(sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listener palsu: panggil fn sekali, lalu berhenti saat ctx selesai.
	store.notifyErr = nil
	store.events = map[int64]Event{5: store.events[5]}
	notifying := &notifyOnceStore{inner: store, payload: "5"}
	h.store = notifying
	go func() { _ = h.Run(ctx) }()

	select {
	case frame := <-sub.Events():
		if !strings.Contains(string(frame), `"task_id":"t9"`) {
			t.Fatalf("frame tidak memuat payload event: %q", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event dari NOTIFY tidak sampai ke klien")
	}
}

// notifyOnceStore memanggil fn sekali lalu menunggu ctx.
type notifyOnceStore struct {
	inner   Store
	payload string
}

func (n *notifyOnceStore) GetEvent(ctx context.Context, id int64) (Event, error) {
	return n.inner.GetEvent(ctx, id)
}

func (n *notifyOnceStore) ListBoardEventsAfter(ctx context.Context, b, o string, after int64, limit int) ([]Event, error) {
	return n.inner.ListBoardEventsAfter(ctx, b, o, after, limit)
}

func (n *notifyOnceStore) Listener(ctx context.Context, _ string, fn func(string)) error {
	fn(n.payload)
	<-ctx.Done()
	return nil
}
