package main

// Tes RBAC dan gerbang tenant untuk 6.2.18 (webhook).
//
// Dua hal yang cuma bisa dibuktikan lewat mux nyata:
//
//  1. Gerbang peran. Seluruh modul ini Admin, dan `member` adalah aktor yang
//     menarik: dia di atas floor Viewer, jadi route yang salah di-set Member
//     akan menjawab 200 dan kriterianya gagal diam-diam.
//  2. Gerbang tenant. Admin org A tidak boleh menempelkan webhook ke board org
//     B dengan menebak id-nya — dan tidak boleh membedakan "board tidak ada"
//     dari "board bukan milikmu" (dua-duanya 404).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentdeck/internal/board"
	"agentdeck/internal/webhook"
)

// webhookFixture memasang tujuh route webhook di atas mux RBAC yang sudah ada,
// dengan repo board palsu supaya resolusi board-nya nyata.
func webhookFixture(t *testing.T) (rbacTestAPI, *http.ServeMux, *fakeRepoForWebhook) {
	t.Helper()
	scenario := newRBACTestAPI(t)

	boardRepo := newFakeBoardRepo()
	seedBoard(t, boardRepo, scenario.orgA, "proj-a", "board-a")
	seedBoard(t, boardRepo, scenario.orgB, "proj-b", "board-b")

	hooks := newFakeRepoForWebhook()
	svc := webhook.NewService(hooks, func(s string) ([]byte, error) { return []byte("SEALED:" + s), nil })

	mux := http.NewServeMux()
	registerWebhookRoutes(mux, scenario.api, board.NewService(boardRepo), svc)
	return scenario, mux, hooks
}

func seedBoard(t *testing.T, repo *fakeBoardRepo, orgID, projectID, boardID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.CreateProject(ctx, board.Project{
		ID: projectID, OrgID: orgID, Slug: projectID, Name: "Demo",
	}); err != nil {
		t.Fatalf("seed project %s: %v", projectID, err)
	}
	if _, err := repo.CreateBoard(ctx, board.Board{
		ID: boardID, OrgID: orgID, ProjectID: projectID,
		Slug: boardID, Name: "Sprint",
		Columns:           board.DefaultColumns,
		BudgetDailyMicros: board.DefaultBudgetDailyMicros,
	}); err != nil {
		t.Fatalf("seed board %s: %v", boardID, err)
	}
}

func webhookReq(t *testing.T, scenario rbacTestAPI, mux *http.ServeMux, actor, method, path, orgID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = bytes.NewReader(raw)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+scenario.tokens[actor+"@x.test"])
	req.Header.Set("X-Org-ID", orgID)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// TestWebhookCreateRequiresAdmin: member dan viewer ditolak, admin dan owner lolos.
func TestWebhookCreateRequiresAdmin(t *testing.T) {
	cases := []struct {
		actor string
		role  string
		want  int
	}{
		{"vera", "viewer", http.StatusForbidden},
		{"marta", "member", http.StatusForbidden},
		{"andre", "admin", http.StatusCreated},
		{"alice", "owner", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			scenario, mux, _ := webhookFixture(t)
			rec := webhookReq(t, scenario, mux, tc.actor, http.MethodPost,
				"/api/v1/boards/board-a/webhooks", scenario.orgA,
				map[string]any{"url": "https://example.com/hook", "secret": "s"})
			if rec.Code != tc.want {
				t.Errorf("%s bikin webhook: status = %d, want %d (body %q)",
					tc.actor, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestWebhookReadRequiresAdmin: bahkan membaca daftar webhook itu Admin, karena
// daftar itu memuat URL tujuan data keluar.
func TestWebhookReadRequiresAdmin(t *testing.T) {
	for _, actor := range []string{"vera", "marta"} {
		scenario, mux, _ := webhookFixture(t)
		rec := webhookReq(t, scenario, mux, actor, http.MethodGet,
			"/api/v1/boards/board-a/webhooks", scenario.orgA, nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s baca webhook: status = %d, want 403", actor, rec.Code)
		}
	}
}

// TestWebhookCreateRefusesAnotherTenantsBoard: board org B lewat admin org A.
func TestWebhookCreateRefusesAnotherTenantsBoard(t *testing.T) {
	scenario, mux, _ := webhookFixture(t)
	rec := webhookReq(t, scenario, mux, "alice", http.MethodPost,
		"/api/v1/boards/board-b/webhooks", scenario.orgA,
		map[string]any{"url": "https://example.com/hook", "secret": "s"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (board bukan milik tenant)", rec.Code)
	}
}

// TestWebhookUnknownBoardIsIndistinguishable: board yang tidak ada dan board
// tenant lain harus jawaban yang sama.
func TestWebhookUnknownBoardIsIndistinguishable(t *testing.T) {
	scenario, mux, _ := webhookFixture(t)
	missing := webhookReq(t, scenario, mux, "alice", http.MethodPost,
		"/api/v1/boards/board-tidak-ada/webhooks", scenario.orgA,
		map[string]any{"url": "https://example.com/hook", "secret": "s"})
	other := webhookReq(t, scenario, mux, "alice", http.MethodPost,
		"/api/v1/boards/board-b/webhooks", scenario.orgA,
		map[string]any{"url": "https://example.com/hook", "secret": "s"})
	if missing.Code != other.Code {
		t.Fatalf("board hilang = %d, board tenant lain = %d; harus sama", missing.Code, other.Code)
	}
}

// TestWebhookGetRefusesAnotherTenant: webhook org B tidak boleh terbaca org A.
func TestWebhookGetRefusesAnotherTenant(t *testing.T) {
	scenario, mux, hooks := webhookFixture(t)
	hooks.hooks = []webhook.Webhook{{
		ID: "w-orgb", OrgID: scenario.orgB, BoardID: "board-b",
		URL: "https://example.com/hook", Active: true,
	}}
	rec := webhookReq(t, scenario, mux, "alice", http.MethodGet, "/api/v1/webhooks/w-orgb", scenario.orgA, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestWebhookCreateRejectsHTTPToTheInternet lewat HTTP nyata, bukan hanya service.
func TestWebhookCreateRejectsHTTPToTheInternet(t *testing.T) {
	scenario, mux, _ := webhookFixture(t)
	rec := webhookReq(t, scenario, mux, "alice", http.MethodPost,
		"/api/v1/boards/board-a/webhooks", scenario.orgA,
		map[string]any{"url": "http://evil.example/hook", "secret": "s"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestWebhookDeleteThenGetIs404 menaku siklus hidup.
func TestWebhookDeleteThenGetIs404(t *testing.T) {
	scenario, mux, _ := webhookFixture(t)
	created := webhookReq(t, scenario, mux, "alice", http.MethodPost,
		"/api/v1/boards/board-a/webhooks", scenario.orgA,
		map[string]any{"url": "https://example.com/hook", "secret": "s"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d (%s)", created.Code, created.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	del := webhookReq(t, scenario, mux, "alice", http.MethodDelete, "/api/v1/webhooks/"+body.ID, scenario.orgA, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", del.Code)
	}
	get := webhookReq(t, scenario, mux, "alice", http.MethodGet, "/api/v1/webhooks/"+body.ID, scenario.orgA, nil)
	if get.Code != http.StatusNotFound {
		t.Fatalf("get setelah delete = %d, want 404", get.Code)
	}
}

// TestWebhookResponseNeverCarriesTheSecret: secret masuk, tidak keluar.
func TestWebhookResponseNeverCarriesTheSecret(t *testing.T) {
	scenario, mux, _ := webhookFixture(t)
	rec := webhookReq(t, scenario, mux, "alice", http.MethodPost,
		"/api/v1/boards/board-a/webhooks", scenario.orgA,
		map[string]any{"url": "https://example.com/hook", "secret": "jangan-sampai-bocor"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("jangan-sampai-bocor")) {
		t.Fatalf("respons memuat secret: %s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("SEALED:")) {
		t.Fatalf("respons memuat ciphertext: %s", rec.Body.String())
	}
}

// TestWebhookRoutesRequireAuthentication: tanpa sesi, 401 — bukan 404. Route
// yang tidak terdaftar menjawab 404, jadi 401 membuktikan route-nya ada.
func TestWebhookRoutesRequireAuthentication(t *testing.T) {
	_, mux, _ := webhookFixture(t)
	paths := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/boards/board-a/webhooks"},
		{http.MethodPost, "/api/v1/boards/board-a/webhooks"},
		{http.MethodGet, "/api/v1/webhooks/w1"},
		{http.MethodPatch, "/api/v1/webhooks/w1"},
		{http.MethodDelete, "/api/v1/webhooks/w1"},
		{http.MethodGet, "/api/v1/webhooks/w1/deliveries"},
		{http.MethodPost, "/api/v1/webhooks/w1/deliveries/1/retry"},
	}
	for _, p := range paths {
		req := httptest.NewRequest(p.method, p.path, bytes.NewReader(nil))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", p.method, p.path, rec.Code)
		}
	}
}

// ---- fake webhook repo ------------------------------------------------------

// fakeRepoForWebhook memenuhi webhook.Repo untuk tes HTTP. Tes ini mengukur
// gerbang peran dan tenant, bukan perilaku penyimpanan — jadi repo-nya cukup
// menyimpan baris di memori dan menghormati scoping org.
type fakeRepoForWebhook struct {
	hooks      []webhook.Webhook
	deliveries []webhook.Delivery
}

func newFakeRepoForWebhook() *fakeRepoForWebhook {
	return &fakeRepoForWebhook{}
}

func (f *fakeRepoForWebhook) Create(_ context.Context, w webhook.Webhook) (webhook.Webhook, error) {
	f.hooks = append(f.hooks, w)
	return w, nil
}

func (f *fakeRepoForWebhook) ListByBoard(_ context.Context, orgID, boardID string) ([]webhook.Webhook, error) {
	var out []webhook.Webhook
	for _, h := range f.hooks {
		if h.OrgID == orgID && h.BoardID == boardID {
			out = append(out, h)
		}
	}
	return out, nil
}

func (f *fakeRepoForWebhook) Get(_ context.Context, orgID, id string) (webhook.Webhook, error) {
	for _, h := range f.hooks {
		if h.ID == id && h.OrgID == orgID {
			return h, nil
		}
	}
	return webhook.Webhook{}, webhook.ErrNotFound
}

func (f *fakeRepoForWebhook) Update(_ context.Context, orgID, id, url string, active bool) (webhook.Webhook, error) {
	for i, h := range f.hooks {
		if h.ID == id && h.OrgID == orgID {
			f.hooks[i].URL = url
			f.hooks[i].Active = active
			return f.hooks[i], nil
		}
	}
	return webhook.Webhook{}, webhook.ErrNotFound
}

func (f *fakeRepoForWebhook) Delete(_ context.Context, orgID, id string) error {
	for i, h := range f.hooks {
		if h.ID == id && h.OrgID == orgID {
			f.hooks = append(f.hooks[:i], f.hooks[i+1:]...)
			return nil
		}
	}
	return webhook.ErrNotFound
}

func (f *fakeRepoForWebhook) ListDeliveries(_ context.Context, webhookID string, limit int) ([]webhook.Delivery, error) {
	var out []webhook.Delivery
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

func (f *fakeRepoForWebhook) GetDelivery(_ context.Context, webhookID string, id int64) (webhook.Delivery, error) {
	for _, d := range f.deliveries {
		if d.ID == id && d.WebhookID == webhookID {
			return d, nil
		}
	}
	return webhook.Delivery{}, webhook.ErrNotFound
}

func (f *fakeRepoForWebhook) ResetDelivery(_ context.Context, id int64) error {
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = webhook.StatusPending
			f.deliveries[i].Attempts = 0
			return nil
		}
	}
	return webhook.ErrNotFound
}
