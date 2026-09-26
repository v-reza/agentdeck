package board

// CancelRun dan Run.CancelRequestedAt terhadap Postgres nyata.
//
// Kenapa di sini dan bukan di tes unit: `cancel_requested_at` harus benar-benar
// sampai dari kolom ke pemanggil. Field-nya ada di domain sejak F2 tapi tidak
// pernah diisi oleh adapter, jadi ada nilai yang tersimpan di DB dan tidak
// pernah terlihat oleh siapa pun yang membacanya lewat Go. Tes unit dengan fake
// tidak bisa menangkap itu — fake-nya mengisi field yang sama-sama tidak
// diisinya.
//
// Gate: AGENTDECK_TEST_DATABASE_URL, sama seperti paket ini.

import (
	"context"
	"errors"
	"testing"
)

// TestPgCancelRunRecordsAndReadsBackTheRequest — ARCHITECTURE 6.2.11.
func TestPgCancelRunRecordsAndReadsBackTheRequest(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	f.start(t)

	before, err := f.svc.GetRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if before.CancelRequestedAt != nil {
		t.Fatal("a fresh run already has cancel_requested_at set")
	}

	cancelled, err := f.svc.CancelRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.CancelRequestedAt == nil {
		t.Fatal("cancel returned without cancel_requested_at: the column was written but never read back")
	}

	// And a fresh read agrees, so the value survives the round trip rather than
	// living only in the value the handler returned.
	reread, err := f.svc.GetRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if reread.CancelRequestedAt == nil {
		t.Fatal("cancel_requested_at did not survive a re-read")
	}
	// The run itself is still `running`: cancellation is a request, and the
	// dispatcher is what ends it. A CancelRun that closed the run would take
	// the outcome decision away from the component that owns it.
	if reread.Status != RunRunning {
		t.Fatalf("cancel changed the run's status to %q; it must stay running until the dispatcher ends it", reread.Status)
	}
}

// TestPgCancelRunIsIdempotent — §6.2.11 marks the route idempotent.
func TestPgCancelRunIsIdempotent(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	f.start(t)

	first, err := f.svc.CancelRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	second, err := f.svc.CancelRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	// COALESCE in RequestRunCancel means the second request does not move the
	// timestamp: "since when was this cancelled" must not drift every time a
	// caller retries.
	if !second.CancelRequestedAt.Equal(*first.CancelRequestedAt) {
		t.Fatalf("a repeat cancel moved the timestamp: %v -> %v", first.CancelRequestedAt, second.CancelRequestedAt)
	}
}

// TestPgCancelRunRefusesAClosedRun — a run that has already ended is returned
// as-is, and its outcome is not rewritten.
func TestPgCancelRunRefusesAClosedRun(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	f.start(t)

	if _, err := f.svc.EndRun(ctx, f.runID, f.orgID, RunSummary{Outcome: "succeeded", Summary: "ok"}); err != nil {
		t.Fatalf("end run: %v", err)
	}

	after, err := f.svc.CancelRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("cancel a finished run should not error: %v", err)
	}
	if after.Outcome != "succeeded" {
		t.Fatalf("a finished run's outcome became %q", after.Outcome)
	}
	if after.Status != RunEnded {
		t.Fatalf("a finished run was reopened: status=%q", after.Status)
	}
}

// TestPgCancelRunIsScopedToItsOrg — an id from another tenant is a 404, and the
// run is untouched.
func TestPgCancelRunIsScopedToItsOrg(t *testing.T) {
	f := newRuntimeFixture(t, "transient_only", 3)
	ctx := context.Background()
	f.start(t)

	otherOrg := newOrg(t, ctx)
	if _, err := f.svc.CancelRun(ctx, f.runID, otherOrg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant cancel: want ErrNotFound, got %v", err)
	}
	reread, err := f.svc.GetRun(ctx, f.runID, f.orgID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if reread.CancelRequestedAt != nil {
		t.Fatal("a cross-tenant cancel request was recorded")
	}
}
