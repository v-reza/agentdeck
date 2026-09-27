package main

// HTTP surface untuk 6.2.13 Events & Realtime SSE (7.1-7.4).
//
// Empat endpoint, dua bentuk response:
//
//   - `GET /boards/{id}/events` dan `GET /events?board_id=` adalah stream SSE;
//   - `GET /tasks/{id}/events` dan `GET /runs/{id}/events` adalah event log
//     JSON biasa (replay satu task / satu run). Kontraknya menyebut keduanya
//     "(JSON / SSE stream)" dan hanya board/events yang ditegaskan stream di
//     7.1, jadi yang di-stream cuma dua yang itu.
//
// Yang membuat handler stream lebih dari menulis header: urutan replay-lalu-
// daftar. Kalau klien didaftarkan ke broadcaster lebih dulu, event yang masuk
// di antara replay dan pendaftaran terkirim dua kali; kalau replay setelah
// pendaftaran, ada jendela di mana event bisa terlewat.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"agentdeck/internal/board"
	"agentdeck/internal/sse"
)

// eventSource adalah irisan board.Service yang dibutuhkan endpoint ini.
//
// Dipisah dari `projectSeeder` yang sudah ada karena yang dibutuhkan di sini
// berbeda: pembacaan board/task/run untuk gerbang tenant, plus event log.
// `projects` sengaja tetap sempit supaya tes yang tidak menyentuh seeding tidak
// perlu menyediakan apa pun.
type eventSource interface {
	GetBoard(ctx context.Context, id, orgID string) (board.Board, error)
	GetTask(ctx context.Context, id, orgID string) (board.Task, error)
	GetRun(ctx context.Context, id, orgID string) (board.Run, error)
	TaskHistory(ctx context.Context, taskID string) ([]board.Event, error)
	RunEventsAfter(ctx context.Context, runID, orgID string, afterID int64, limit int) ([]board.Event, error)
}

// Hub nyata yang dipakai handler. Interface supaya handler bisa diuji tanpa
// Postgres dan tanpa koneksi hidup.
type eventHub interface {
	Serve(w http.ResponseWriter, r *http.Request, orgID, boardID string)
	Subscribe(orgID, boardID string) *sse.Subscriber
	Unsubscribe(sub *sse.Subscriber)
	Replay(ctx context.Context, orgID, boardID string, afterID int64, send func([]byte) error) error
	PingInterval() time.Duration
	Log() *slog.Logger
}

type eventResponse struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	TaskID    string          `json:"task_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	BoardID   string          `json:"board_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func toEventResponse(ev board.Event) eventResponse {
	payload := ev.PayloadJSON
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return eventResponse{
		ID: ev.ID, Kind: ev.Kind, TaskID: ev.TaskID, RunID: ev.RunID,
		BoardID: ev.BoardID, Payload: payload, CreatedAt: ev.CreatedAt,
	}
}

// sseEvents menjawab GET /boards/{id}/events dan GET /events?board_id=.
func (a authAPI) sseEvents(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	boardID := strings.TrimSpace(r.PathValue("id"))
	if boardID == "" {
		boardID = strings.TrimSpace(r.URL.Query().Get("board_id"))
	}
	if boardID == "" {
		http.Error(w, "board_id required", http.StatusBadRequest)
		return
	}
	if a.events == nil || a.hub == nil {
		// Sama seperti `projects`: nil di tes yang tidak menyentuh permukaan ini.
		// 503, bukan panic — route-nya terdaftar, layanannya memang tidak ada.
		http.Error(w, "realtime unavailable", http.StatusServiceUnavailable)
		return
	}
	// Board harus ada DAN milik org pemanggil. Ini juga batas tenant: tanpa
	// pemeriksaan ini, id board org lain akan berlangganan stream yang bukan
	// miliknya.
	if _, err := a.events.GetBoard(r.Context(), boardID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	a.hub.Serve(w, r, orgCtx.workspace.ID, boardID)
}

// listTaskEvents menjawab GET /tasks/{id}/events sebagai JSON.
func (a authAPI) listTaskEvents(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if a.events == nil || a.hub == nil {
		// Sama seperti `projects`: nil di tes yang tidak menyentuh permukaan ini.
		// 503, bukan panic — route-nya terdaftar, layanannya memang tidak ada.
		http.Error(w, "realtime unavailable", http.StatusServiceUnavailable)
		return
	}
	taskID := r.PathValue("id")
	if _, err := a.events.GetTask(r.Context(), taskID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	events, err := a.events.TaskHistory(r.Context(), taskID)
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeEventList(w, events)
}

// listRunEvents menjawab GET /runs/{id}/events sebagai JSON (replay trace).
func (a authAPI) listRunEvents(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if a.events == nil || a.hub == nil {
		// Sama seperti `projects`: nil di tes yang tidak menyentuh permukaan ini.
		// 503, bukan panic — route-nya terdaftar, layanannya memang tidak ada.
		http.Error(w, "realtime unavailable", http.StatusServiceUnavailable)
		return
	}
	runID := r.PathValue("id")
	if _, err := a.events.GetRun(r.Context(), runID, orgCtx.workspace.ID); err != nil {
		writeBoardError(w, err)
		return
	}
	events, err := a.events.RunEventsAfter(r.Context(), runID, orgCtx.workspace.ID, 0, sse.ReplayLimit())
	if err != nil {
		writeBoardError(w, err)
		return
	}
	writeEventList(w, events)
}

func writeEventList(w http.ResponseWriter, events []board.Event) {
	out := make([]eventResponse, 0, len(events))
	for _, ev := range events {
		out = append(out, toEventResponse(ev))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
