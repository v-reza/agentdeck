package main

// HTTP surface untuk 6.2.16 Artifacts (ARCHITECTURE 12.3, 3.15).
//
// Lima endpoint, dua peran:
//
//	List / metadata / download  Viewer  — membaca hasil kerja sama dengan
//	                                      membacanya di UI.
//	upload-url / register       Worker  — jalur ini yang mengunggah, dan
//	                                      worker-lah yang menghasilkan file.
//
// Gerbang perannya dari kolom Role di tabel 6.2.16. Presigned URL TIDAK pernah
// dibuat untuk aktor yang belum lewat Authorize, jadi URL itu sendiri sudah
// ter-scope: ia menunjuk satu key di bawah prefix org yang memintanya.

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentdeck/internal/artifact"
	"agentdeck/internal/auth"
)

type artifactAPI struct {
	authAPI
	artifacts *artifact.Service
}

// registerArtifactRoutes memasang kelima route.
//
// `svc` boleh nil: deployment tanpa storage tetap punya route-nya, dan
// handler-nya membalas 503. Route yang hilang total akan membingungkan —
// 404 menyiratkan endpoint-nya belum ada, padahal yang belum ada
// konfigurasinya.
func registerArtifactRoutes(mux *http.ServeMux, api authAPI, svc *artifact.Service) {
	a := artifactAPI{authAPI: api, artifacts: svc}
	boardRoute := func(pattern string, h http.HandlerFunc, minimum auth.Role) {
		mux.Handle(pattern, api.orgHeaderContextMiddleware(api.requireRole(h, minimum)))
	}
	boardRoute("GET /api/v1/tasks/{id}/artifacts", http.HandlerFunc(a.listArtifacts), auth.Viewer)
	boardRoute("POST /api/v1/tasks/{id}/artifacts/upload-url", http.HandlerFunc(a.artifactUploadURL), auth.Member)
	boardRoute("POST /api/v1/tasks/{id}/artifacts", http.HandlerFunc(a.registerArtifact), auth.Member)
	boardRoute("GET /api/v1/artifacts/{id}", http.HandlerFunc(a.getArtifact), auth.Viewer)
	boardRoute("GET /api/v1/artifacts/{id}/download", http.HandlerFunc(a.downloadArtifact), auth.Viewer)
}

// artifactContext menyelesaikan org aktif, sama seperti handler board lain.
func (a artifactAPI) artifactContext(w http.ResponseWriter, r *http.Request) (string, bool) {
	if a.artifacts == nil {
		// 503, bukan 404: endpoint-nya ADA, yang belum ada konfigurasinya.
		http.Error(w, "artifact storage not configured", http.StatusServiceUnavailable)
		return "", false
	}
	ctx, err := currentOrgContext(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return "", false
	}
	return ctx.workspace.ID, true
}

// GET /api/v1/tasks/{id}/artifacts
func (a artifactAPI) listArtifacts(w http.ResponseWriter, r *http.Request) {
	orgID, ok := a.artifactContext(w, r)
	if !ok {
		return
	}
	items, err := a.artifacts.List(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, artifactResponse(item))
	}
	writeJSONResponse(w, http.StatusOK, out)
}

// POST /api/v1/tasks/{id}/artifacts/upload-url
func (a artifactAPI) artifactUploadURL(w http.ResponseWriter, r *http.Request) {
	orgID, ok := a.artifactContext(w, r)
	if !ok {
		return
	}
	var body struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
		// RunID wajib: key objeknya memuat run (3.15), jadi URL-nya tidak bisa
		// dibentuk tanpa itu. Lihat komentar UploadURL.
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	ticket, err := a.artifacts.UploadURL(r.Context(), orgID, r.PathValue("id"),
		body.RunID, body.Filename, body.ContentType, body.Size)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{
		"upload_url":    ticket.URL,
		"storage_key":   ticket.StorageKey,
		"artifact_id":   ticket.ArtifactID,
		"expires_at":    ticket.ExpiresAt.UTC().Format(time.RFC3339),
		"max_file_size": ticket.MaxFileSize,
	})
}

// POST /api/v1/tasks/{id}/artifacts
func (a artifactAPI) registerArtifact(w http.ResponseWriter, r *http.Request) {
	orgID, ok := a.artifactContext(w, r)
	if !ok {
		return
	}
	var body struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
		SHA256      string `json:"sha256"`
		StorageKey  string `json:"storage_key"`
		RunID       string `json:"run_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, err := a.artifacts.Register(r.Context(), orgID, r.PathValue("id"), artifact.RegisterInput{
		Filename:    body.Filename,
		ContentType: body.ContentType,
		Size:        body.Size,
		SHA256:      body.SHA256,
		StorageKey:  body.StorageKey,
		RunID:       body.RunID,
	})
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusCreated, artifactResponse(created))
}

// GET /api/v1/artifacts/{id}
func (a artifactAPI) getArtifact(w http.ResponseWriter, r *http.Request) {
	orgID, ok := a.artifactContext(w, r)
	if !ok {
		return
	}
	item, err := a.artifacts.Get(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, artifactResponse(item))
}

// GET /api/v1/artifacts/{id}/download
//
// 302 ke presigned GET (6.2.16). Redirect, bukan proxy: 12.3 memilih presigned
// URL justru supaya byte-nya tidak melewati server Go.
func (a artifactAPI) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	orgID, ok := a.artifactContext(w, r)
	if !ok {
		return
	}
	url, expires, err := a.artifacts.DownloadURL(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Expires", expires.UTC().Format(http.TimeFormat))
	http.Redirect(w, r, url, http.StatusFound)
}

func artifactResponse(a artifact.Artifact) map[string]any {
	return map[string]any{
		"id":           a.ID,
		"task_id":      a.TaskID,
		"run_id":       a.RunID,
		"filename":     a.Filename,
		"content_type": a.ContentType,
		"size":         a.Size,
		"sha256":       a.SHA256,
		"created_at":   a.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// writeArtifactError memetakan sentinel paket artifact ke status HTTP.
func writeArtifactError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, artifact.ErrNotFound), errors.Is(err, artifact.ErrTaskNotFound),
		errors.Is(err, artifact.ErrRunNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, artifact.ErrTooLarge):
		http.Error(w, "file exceeds the 25 MB limit", http.StatusBadRequest)
	case errors.Is(err, artifact.ErrTaskQuota):
		http.Error(w, "task artifact quota exceeded", http.StatusBadRequest)
	case errors.Is(err, artifact.ErrSHA256Match):
		http.Error(w, "sha256 does not match the uploaded object", http.StatusBadRequest)
	case errors.Is(err, artifact.ErrObjectMissing):
		http.Error(w, "object not found in storage", http.StatusBadRequest)
	case errors.Is(err, artifact.ErrInvalidInput):
		http.Error(w, "invalid request", http.StatusBadRequest)
	case errors.Is(err, artifact.ErrNotConfigured):
		http.Error(w, "artifact storage not configured", http.StatusServiceUnavailable)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
