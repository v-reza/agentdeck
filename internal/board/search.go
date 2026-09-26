package board

// Search — ARCHITECTURE 6.2.19.
//
// Both searches are read-only and org-scoped, and both live here rather than in
// the handler so the limit clamp and the "empty query is not a search" rule are
// enforced in one place. The HTTP layer's job is then only to turn a query
// string into these arguments and a result into JSON.

import (
	"context"
	"strings"
)

// SearchTasks is 6.2.19's task search.
//
// An empty or whitespace-only `q` is ErrInvalidInput rather than "return
// everything": a search box that answers a blank query with the whole board is
// indistinguishable from a listing, and the listing endpoint already exists with
// proper pagination. Refusing is the honest answer.
func (s *Service) SearchTasks(ctx context.Context, orgID, q, boardID string, limit int32) ([]Task, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.SearchTasks(ctx, orgID, q, boardID, ClampSearchLimit(limit))
}

// SearchRuns is 6.2.19's run search.
//
// Unlike the task search, every filter here is optional — "cari run berdasarkan
// kegagalan atau metadata" is a filter over a set, not a text lookup, so a
// caller narrowing by `failure_kind=transient` alone is a legitimate query. The
// one thing that is refused is a request with no constraint at all, because that
// is a listing with no pagination behind it.
func (s *Service) SearchRuns(ctx context.Context, orgID string, f RunSearchFilter) ([]Run, error) {
	f.TaskID = strings.TrimSpace(f.TaskID)
	f.FailureKind = strings.TrimSpace(f.FailureKind)
	f.Outcome = strings.TrimSpace(f.Outcome)
	f.Q = strings.TrimSpace(f.Q)
	if f.TaskID == "" && f.FailureKind == "" && f.Outcome == "" && f.Q == "" {
		return nil, ErrInvalidInput
	}
	f.Limit = ClampSearchLimit(f.Limit)
	return s.repo.SearchRuns(ctx, orgID, f)
}
