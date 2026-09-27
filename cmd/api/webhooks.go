package main

// HTTP surface untuk 6.2.18 Webhooks (ARCHITECTURE 3.22, 3.23, 13).
//
// Tujuh endpoint, semuanya Admin. Tiga di antaranya terikat ke board
// (`/boards/{board_id}/webhooks`), empat ke webhook itu sendiri
// (`/webhooks/{id}`) — dan yang terakhir tidak punya board di path-nya, jadi
// scoping tenant-nya lewat `org_id` yang tersimpan di barisnya.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"agentdeck/internal/board"
	"agentdeck/internal/webhook"
)

// webhookAPI memegang service webhook dan service board.
//
// `boards` ada di sini untuk satu hal: membuktikan board yang disebut di path
// benar-benar milik workspace pemanggil sebelum webhook ditulis ke bawahnya.
// Tanpa itu, admin org A bisa menempelkan webhook ke board org B hanya dengan
// menebak id-nya.
type webhookAPI struct {
	svc    *webhook.Service
	boards *board.Service
}

// ---- response ----------------------------------------------------------------

type webhookResponse struct {
	ID        string   `json:"id"`
	OrgID     string   `json:"org_id"`
	BoardID   string   `json:"board_id"`
	URL       string   `json:"url"`
	Events    []string `json:"events_json"`
	Active    bool     `json:"active"`
	CreatedAt string   `json:"created_at"`
}

type webhookDeliveryResponse struct {
	ID           int64  `json:"id"`
	WebhookID    string `json:"webhook_id"`
	EventID      int64  `json:"event_id"`
	Status       string `json:"status"`
	Attempts     int    `json:"attempts"`
	ResponseCode *int   `json:"response_code"`
	LastError    string `json:"last_error"`
	CreatedAt    string `json:"created_at"`
}

func toWebhookResponse(w webhook.Webhook) webhookResponse {
	events := w.Events
	if events == nil {
		events = []string{}
	}
	return webhookResponse{
		ID: w.ID, OrgID: w.OrgID, BoardID: w.BoardID, URL: w.URL,
		Events: events, Active: w.Active,
		CreatedAt: w.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toDeliveryResponse(d webhook.Delivery) webhookDeliveryResponse {
	return webhookDeliveryResponse{
		ID: d.ID, WebhookID: d.WebhookID, EventID: d.EventID, Status: d.Status,
		Attempts: d.Attempts, ResponseCode: d.ResponseCode, LastError: d.LastError,
		CreatedAt: d.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// webhookError memetakan sentinel service ke status HTTP.
//
// Not-found dan bukan-anggota sama-sama 404: perbedaan antara keduanya
// memberi tahu penebak bahwa id-nya ada (US-AD07).
func webhookError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, webhook.ErrNotFound):
		http.Error(w, "webhook not found", http.StatusNotFound)
	case errors.Is(err, webhook.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// boardInWorkspace memastikan board ada di workspace pemanggil.
func (a webhookAPI) boardInWorkspace(w http.ResponseWriter, r *http.Request, orgID, boardID string) bool {
	if _, err := a.boards.GetBoard(r.Context(), boardID, orgID); err != nil {
		// Board tidak ada dan board milik tenant lain sama-sama 404.
		http.Error(w, "board not found", http.StatusNotFound)
		return false
	}
	return true
}

// ---- handlers ----------------------------------------------------------------

// GET /api/v1/boards/{board_id}/webhooks
func (a webhookAPI) listWebhooks(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	boardID := r.PathValue("board_id")
	if !a.boardInWorkspace(w, r, orgCtx.workspace.ID, boardID) {
		return
	}
	list, err := a.svc.ListByBoard(r.Context(), orgCtx.workspace.ID, boardID)
	if err != nil {
		webhookError(w, err)
		return
	}
	out := make([]webhookResponse, 0, len(list))
	for _, item := range list {
		out = append(out, toWebhookResponse(item))
	}
	writeJSON(w, out)
}

// POST /api/v1/boards/{board_id}/webhooks
func (a webhookAPI) createWebhook(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	boardID := r.PathValue("board_id")
	if !a.boardInWorkspace(w, r, orgCtx.workspace.ID, boardID) {
		return
	}
	var in webhook.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, err := a.svc.Create(r.Context(), orgCtx.workspace.ID, boardID, in)
	if err != nil {
		webhookError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toWebhookResponse(created))
}

// GET /api/v1/webhooks/{id}
func (a webhookAPI) getWebhook(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	found, err := a.svc.Get(r.Context(), orgCtx.workspace.ID, r.PathValue("id"))
	if err != nil {
		webhookError(w, err)
		return
	}
	writeJSON(w, toWebhookResponse(found))
}

// PATCH /api/v1/webhooks/{id}
func (a webhookAPI) patchWebhook(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var in webhook.UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	updated, err := a.svc.Update(r.Context(), orgCtx.workspace.ID, r.PathValue("id"), in)
	if err != nil {
		webhookError(w, err)
		return
	}
	writeJSON(w, toWebhookResponse(updated))
}

// DELETE /api/v1/webhooks/{id}
func (a webhookAPI) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err := a.svc.Delete(r.Context(), orgCtx.workspace.ID, r.PathValue("id")); err != nil {
		webhookError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/webhooks/{id}/deliveries
func (a webhookAPI) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	// Webhook-nya dibaca lebih dulu: tanpa ini, id webhook milik tenant lain
	// akan mengembalikan daftar kosong (200) alih-alih 404, dan 200-kosong vs
	// 404 adalah sinyal keberadaan yang membocorkan tenant.
	if _, err := a.svc.Get(r.Context(), orgCtx.workspace.ID, id); err != nil {
		webhookError(w, err)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	list, err := a.svc.ListDeliveries(r.Context(), id, limit)
	if err != nil {
		webhookError(w, err)
		return
	}
	out := make([]webhookDeliveryResponse, 0, len(list))
	for _, d := range list {
		out = append(out, toDeliveryResponse(d))
	}
	writeJSON(w, out)
}

// POST /api/v1/webhooks/{id}/deliveries/{delivery_id}/retry
func (a webhookAPI) retryWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	orgCtx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if _, err := a.svc.Get(r.Context(), orgCtx.workspace.ID, id); err != nil {
		webhookError(w, err)
		return
	}
	deliveryID, err := strconv.ParseInt(r.PathValue("delivery_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid delivery id", http.StatusBadRequest)
		return
	}
	updated, err := a.svc.RetryDelivery(r.Context(), id, deliveryID)
	if err != nil {
		webhookError(w, err)
		return
	}
	writeJSON(w, toDeliveryResponse(updated))
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
