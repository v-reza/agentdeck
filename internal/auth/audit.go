package auth

// Audit log + in-app notifications — ARCHITECTURE 6.2.19, US-AD95, US-AD61.
//
// These two live in this package rather than in `internal/board` because
// `audit_log` already has its writer here (RenameOrgWithAudit commits a rename
// and its audit row in one statement), and because both are scoped to a *user*
// and a *workspace* rather than to a board. Splitting the read into another
// package would mean a second place that knows the audit_log column list.

import (
	"context"
	"time"

	"agentdeck/internal/ulid"
)

// AuditFilter is US-AD95 AC2's filter set. Every field is optional; the zero
// value means "no filter".
//
// The two actors are separate fields rather than one because `audit_log` has two
// separate columns: an action is performed by a user or by an agent, and a
// caller filtering by "who" should not have to know which kind they are asking
// about. `ActorUserID` is the one the HTTP surface exposes today; the agent
// column has no producer yet (no agent-driven audited action exists), so it is
// deliberately not in the filter — adding it now would be a parameter that
// always matches nothing.
type AuditFilter struct {
	// Cursor is the last id the caller saw. 0 means first page. Pagination is
	// by id, not offset: a new audit row arriving between two page requests
	// must not shift a row from page 2 onto page 1 and make it invisible.
	Cursor int64
	// ActorUserID filters to one actor.
	ActorUserID *string
	// Action filters to one action name, e.g. "org.rename".
	Action *string
	// From and To bound created_at. Zero time means unbounded on that side.
	From time.Time
	To   time.Time
	// Limit caps the page. Zero means DefaultAuditLimit.
	Limit int32
}

// DefaultAuditLimit is the page size when the caller does not ask for one.
// 50 is the number the audit screen's table shows without scrolling on a laptop
// viewport, so the default page is one screen.
const DefaultAuditLimit = 50

// MaxAuditLimit caps a caller-supplied limit. Without a ceiling, `?limit=100000`
// turns one request into a full-table read.
const MaxAuditLimit = 200

// Notification is one in-app notification (ARCHITECTURE 3.21, US-AD61).
//
// `ReadAt` nil means unread — that is the badge's whole input. `TargetType` and
// `TargetID` are the deep link: AC2 says clicking a notification navigates to
// the task it is about, and those two fields are the only thing that makes that
// possible without parsing the title.
type Notification struct {
	ID         string
	UserID     string
	OrgID      string
	Kind       string
	Title      string
	Body       string
	TargetType string
	TargetID   string
	ReadAt     *time.Time
	CreatedAt  time.Time
}

// ValidNotificationKinds mirrors the notifications_kind_chk CHECK in 3.21. It is
// repeated here so a producer with a typo gets ErrInvalidInput instead of a
// constraint violation surfacing as a 500.
var ValidNotificationKinds = map[string]bool{
	"approval.requested": true,
	"budget.warning":     true,
	"run.failed":         true,
	"credential.invalid": true,
}

// ValidNotificationTargets is 3.21's comment on `target_type`: 'task' | 'run' |
// 'board'. A notification whose target cannot be opened is a dead end for AC2.
var ValidNotificationTargets = map[string]bool{
	"task":  true,
	"run":   true,
	"board": true,
}

// ClampAuditLimit normalises a caller-supplied page size.
func ClampAuditLimit(limit int32) int32 {
	switch {
	case limit <= 0:
		return DefaultAuditLimit
	case limit > MaxAuditLimit:
		return MaxAuditLimit
	default:
		return limit
	}
}

// ---- Store methods (the domain boundary the HTTP layer calls) ---------------

// AuditLog reads the workspace trail. The org and the filter both come from the
// caller, but only the org is trusted for scoping — the filter narrows within it.
func (s *Store) AuditLog(ctx context.Context, orgID string, f AuditFilter) ([]AuditEntry, error) {
	return s.repo.ListAuditLog(ctx, orgID, f)
}

// Notifications returns one user's inbox in one workspace plus the unread count.
func (s *Store) Notifications(ctx context.Context, userID, orgID string, limit int32) ([]Notification, int, error) {
	return s.repo.ListNotifications(ctx, userID, orgID, limit)
}

// Notify stores one notification. It is the producer entry point: the dispatcher
// and the approval gate call it, and it validates kind and target here so a
// producer with a typo gets an error instead of a constraint violation.
func (s *Store) Notify(ctx context.Context, n Notification) (Notification, error) {
	if !ValidNotificationKinds[n.Kind] {
		return Notification{}, ErrInvalidInput
	}
	if n.TargetType != "" && !ValidNotificationTargets[n.TargetType] {
		return Notification{}, ErrInvalidInput
	}
	if n.Title == "" {
		return Notification{}, ErrInvalidInput
	}
	// The id is minted here, not by the caller. `notifications_id_ulid_chk`
	// demands a 26-character ULID, and a producer that forgets to set one would
	// otherwise get a constraint violation surfacing as a 500 — which is what
	// happened the first time this ran against Postgres. Every other id in this
	// package is minted by the domain for the same reason.
	if n.ID == "" {
		n.ID = ulid.Must()
	}
	return s.repo.CreateNotification(ctx, n)
}

// NotifyOperational adapts Store to board.NotificationWriter. It exists so
// `internal/board` never has to import this package: the board service declares
// the one method it needs, and the composition root passes this value in.
//
// It returns the error rather than swallowing it. The board's producer methods
// are the ones that decide a failed alert must not fail the run; a silent drop
// here would hide the failure from the one place that can log it.
func (s *Store) NotifyOperational(ctx context.Context, orgID, kind, title, body, targetType, targetID string) error {
	_, err := s.NotifyOnce(ctx, Notification{
		OrgID: orgID, Kind: kind, Title: title, Body: body,
		TargetType: targetType, TargetID: targetID,
	})
	return err
}

// NotifyOnce writes an operational notification to every owner and admin of the
// org, at most once per (recipient, kind, target, day).
//
// The return value is how many rows were actually written — zero on a repeat.
// The caller does not need it, but it is what lets a test prove the dedup works
// instead of assuming it.
//
// A failure to list recipients is returned, not swallowed. A notification that
// silently goes nowhere is worse than an error in a log: the operator who needed
// the alert never learns it did not arrive.
func (s *Store) NotifyOnce(ctx context.Context, n Notification) (int64, error) {
	if !ValidNotificationKinds[n.Kind] {
		return 0, ErrInvalidInput
	}
	if n.TargetType != "" && !ValidNotificationTargets[n.TargetType] {
		return 0, ErrInvalidInput
	}
	if n.Title == "" {
		return 0, ErrInvalidInput
	}
	recipients, err := s.repo.OrgAdminsAndOwners(ctx, n.OrgID)
	if err != nil {
		return 0, err
	}
	var written int64
	for _, userID := range recipients {
		one := n
		one.ID = ulid.Must()
		one.UserID = userID
		rows, err := s.repo.CreateNotificationOnce(ctx, one)
		if err != nil {
			return written, err
		}
		written += rows
	}
	return written, nil
}

// MarkNotificationsRead marks the named rows read, or every unread one when
// markAll is set (US-AD61 AC1). It returns how many changed, so the caller can
// answer with a count rather than a bare 200.
func (s *Store) MarkNotificationsRead(ctx context.Context, userID, orgID string, ids []string, markAll bool) (int, error) {
	if !markAll && len(ids) == 0 {
		// Neither named nor global: refuse rather than silently marking
		// everything, which is what a forgotten `all` would otherwise do.
		return 0, ErrInvalidInput
	}
	return s.repo.MarkNotificationsRead(ctx, userID, orgID, ids, markAll)
}
