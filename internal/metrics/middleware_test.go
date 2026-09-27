package metrics

// Regresi: middleware tidak boleh menyembunyikan kemampuan ResponseWriter.
//
// Bug yang dijaga tes ini: `statusRecorder` menyematkan `http.ResponseWriter`,
// dan interface itu tidak punya `Flush`. Jadi `w.(http.Flusher)` di dalam
// handler selalu gagal — setiap handler streaming (SSE) menjawab 500
// "streaming unsupported". Tidak ada tes lain yang menangkapnya, karena handler
// yang tidak pernah flush berjalan baik-baik saja di belakang wrapper.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMiddlewareKeepsFlusherReachable: handler di belakang Middleware harus
// masih bisa mengakses Flusher dan mengirim byte sebelum handler selesai.
func TestMiddlewareKeepsFlusherReachable(t *testing.T) {
	reg := NewRegistry()
	streamed := make(chan string, 1)

	stream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("data: hello\n\n")); err != nil {
			t.Errorf("write: %v", err)
			return
		}
		f.Flush()
		streamed <- "flushed"
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/boards/b1/events", nil)
	reg.Middleware(stream).ServeHTTP(rec, req)

	select {
	case <-streamed:
	default:
		t.Fatal("handler tidak pernah sampai ke Flush")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (Flusher tidak terlihat)", rec.Code)
	}
	if got := rec.Body.String(); got != "data: hello\n\n" {
		t.Fatalf("body = %q, want frame SSE", got)
	}
	if !rec.Flushed {
		t.Fatal("recorder tidak mencatat flush — middleware menelan Flush")
	}
}

// TestMiddlewareKeepsUnwrapReachable: http.ResponseController mencari
// kemampuan lewat rantai Unwrap. Tanpa Unwrap, stream panjang tidak bisa
// memperpanjang deadline tulisnya.
//
// Writer dasarnya harus benar-benar mendukung SetWriteDeadline — httptest
// .ResponseRecorder tidak, jadi recorder selalu menjawab ErrNotSupported dan
// tesnya akan gagal meski Unwrap-nya benar.
func TestMiddlewareKeepsUnwrapReachable(t *testing.T) {
	reg := NewRegistry()
	want := time.Now().Add(time.Hour)
	base := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(want); err != nil {
			t.Errorf("SetWriteDeadline: %v", err)
		}
	})

	reg.Middleware(h).ServeHTTP(base, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t1/events", nil))

	if !base.got.Equal(want) {
		t.Fatalf("writer asli tidak menerima deadline: got %v, want %v", base.got, want)
	}
}

// deadlineWriter adalah ResponseWriter yang mendukung SetWriteDeadline, seperti
// http.response di server sungguhan.
type deadlineWriter struct {
	*httptest.ResponseRecorder
	got time.Time
}

func (d *deadlineWriter) SetWriteDeadline(t time.Time) error {
	d.got = t
	return nil
}

// TestMiddlewareStillRecordsStatus: perbaikan Flush/Unwrap tidak boleh
// merusak tujuan asli wrapper — status code tetap tercatat.
func TestMiddlewareStillRecordsStatus(t *testing.T) {
	reg := NewRegistry()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	// Mux sungguhan: label `path` berasal dari pola route yang benar-benar
	// dicocokkan (r.Pattern), jadi tanpa mux tidak ada yang bisa diuji.
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/tasks/{id}", h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t1", nil)
	reg.Middleware(mux).ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", rec.Code)
	}
	var sb strings.Builder
	if _, err := reg.WriteTo(&sb); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !strings.Contains(sb.String(), `status="418"`) {
		t.Fatalf("status 418 tidak tercatat:\n%s", sb.String())
	}
	if !strings.Contains(sb.String(), `path="/api/v1/tasks/{id}"`) {
		t.Fatalf("path label salah:\n%s", sb.String())
	}
}
