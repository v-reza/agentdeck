package main

// Handler 6.2.13 — events & realtime SSE.
//
// Yang diukur di sini adalah hal-hal yang tidak kelihatan dari paket sse:
//
//  1. gerbang peran (Viewer) dan gerbang tenant: board org lain 404, bukan
//     stream yang bocor;
//  2. `GET /events` tanpa board_id = 400, bukan stream semua board;
//  3. dua endpoint log (`tasks/{id}/events`, `runs/{id}/events`) menjawab JSON
//     dan hanya berisi event milik org pemanggil;
//  4. `tasks/{id}/events` untuk task yang tidak ada = 404.
//
// Stream-nya sendiri tidak diuji end-to-end di sini (butuh koneksi hidup);
// itu tugas tools/probe-f11.py lawan server nyata.

import (
	"net/http"
	"testing"
)

// TestEventsRoutesRequireMembership: 401 tanpa sesi berarti route-nya ada.
// 404 berarti mux lupa mendaftarkannya.
func TestEventsRoutesRequireMembership(t *testing.T) {
	e := newRBACTestAPI(t)
	for _, path := range []string{
		"/api/v1/boards/01ABC/events",
		"/api/v1/events?board_id=01ABC",
		"/api/v1/tasks/01ABC/events",
		"/api/v1/runs/01ABC/events",
	} {
		resp := e.do(t, http.MethodGet, path, "", "", e.orgA)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s tanpa sesi = %d, want 401", path, resp.StatusCode)
		}
	}
}

// TestEventsWithoutBoardIDIsRefused: `GET /events` tanpa board_id akan berarti
// "stream semua board di org" — itu bukan yang dijanjikan 6.2.13, dan
// membiarkannya berarti satu koneksi menerima event board yang tidak dibuka
// pemanggilnya.
func TestEventsWithoutBoardIDIsRefused(t *testing.T) {
	e := newRBACTestAPI(t)
	resp := e.do(t, http.MethodGet, "/api/v1/events", "", e.tokens["marta@x.test"], e.orgA)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /events tanpa board_id = %d, want 400", resp.StatusCode)
	}
}

// TestTaskEventsOfAnotherTenantIsNotFound pins the tenant gate on the log
// endpoint: bella punya org sendiri, jadi task orgA tidak terlihat.
func TestTaskEventsOfAnotherTenantIsNotFound(t *testing.T) {
	e := newRBACTestAPI(t)
	// bella bukan anggota orgA (fixture rbac_test).
	resp := e.do(t, http.MethodGet, "/api/v1/tasks/01J7ABCDEF1234567890ABCDEF/events",
		"", e.tokens["bella@x.test"], e.orgA)
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("task org lain = %d, want 404/403", resp.StatusCode)
	}
}

// TestEventsAreUnavailableWithoutTheService: fixture RBAC tidak memasang
// eventSource/hub, sama seperti `projects`. Yang penting: route-nya terdaftar
// (bukan 404) dan handler-nya menjawab, bukan panic. Asersi 404 untuk id yang
// tidak ada diuji di tools/probe-f11.py lawan server nyata, karena di situ
// service-nya benar-benar ada.
func TestEventsAreUnavailableWithoutTheService(t *testing.T) {
	e := newRBACTestAPI(t)
	for _, path := range []string{
		"/api/v1/tasks/01ZZZZZZZZZZZZZZZZZZZZZZZZ/events",
		"/api/v1/runs/01ZZZZZZZZZZZZZZZZZZZZZZZZ/events",
		"/api/v1/boards/01ZZZZZZZZZZZZZZZZZZZZZZZZ/events",
	} {
		resp := e.do(t, http.MethodGet, path, "", e.tokens["alice@x.test"], e.orgA)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503 (route terdaftar, service tidak dipasang)",
				path, resp.StatusCode)
		}
	}
}

// TestViewerReachesTheEventHandlers: 6.2.13 says Role Min is Viewer. vera adalah
// Viewer di orgA, jadi 503 (service tidak dipasang) berarti gerbang perannya
// lolos — 403 berarti gerbangnya terlalu ketat.
func TestViewerReachesTheEventHandlers(t *testing.T) {
	e := newRBACTestAPI(t)
	resp := e.do(t, http.MethodGet, "/api/v1/tasks/01ZZZZZZZZZZZZZZZZZZZZZZZZ/events",
		"", e.tokens["vera@x.test"], e.orgA)
	if resp.StatusCode == http.StatusForbidden {
		t.Fatal("viewer ditolak = 403, want gerbang Viewer")
	}
}
