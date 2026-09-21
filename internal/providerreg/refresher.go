package providerreg

// AC7's automatic half: a model list older than 24 hours is refreshed without
// anyone pressing the button.
//
// Why a background ticker and not "refresh when the Provider page is read": the
// read endpoint's floor is Viewer, and this call spends the workspace's
// credential against the operator's upstream. A viewer opening a page must not
// be able to make outbound calls — that is the same authority the Admin floor on
// POST /providers/{id}/models exists to protect. The refresher runs as nobody,
// on its own schedule, so no role is required to trigger it.
//
// Why not a worker/queue: the repo has no scheduler at all (ARCHITECTURE 18.3 —
// dispatcher, executor, sse, storage, webhook are all unbuilt), and this is one
// UPDATE per stale row. A goroutine with a ticker is the smallest thing that is
// still correct; a queue would be infrastructure for a workload that is
// currently 47 rows.
//
// ponytail: single-process. Two API replicas would both tick and double-fetch.
// The fix when that matters is a Postgres advisory lock around each batch
// (pg_try_advisory_lock), not a queue.

import (
	"context"
	"log/slog"
	"time"
)

const (
	// RefresherInterval is how often the refresher looks for stale rows. It is
	// deliberately much shorter than ModelsStaleAfter: the tick is a cheap
	// indexed read that usually finds nothing, while the fetch it may trigger is
	// the expensive part. A 24-hour tick would also mean a provider registered
	// just after a tick waits a full day for its first list.
	RefresherInterval = 15 * time.Minute
	// RefresherBatchSize bounds one pass. It caps how much upstream traffic a
	// single tick can cause, so a workspace with hundreds of stale providers
	// degrades into "a few ticks" rather than a burst.
	RefresherBatchSize = 25
	// refresherTimeout bounds one whole pass, including every upstream call in
	// it. Without it a hung provider would stall the loop forever.
	refresherTimeout = 2 * time.Minute
)

// ModelRefresher walks every workspace's stale providers and refetches their
// model lists. It owns no HTTP surface: it is started from main and stopped by
// cancelling its context.
type ModelRefresher struct {
	svc    *Service
	logger *slog.Logger
	// now is a field so a test can age a row without sleeping.
	now func() time.Time
}

// NewModelRefresher builds the refresher over a service.
func NewModelRefresher(svc *Service, logger *slog.Logger) *ModelRefresher {
	if logger == nil {
		logger = slog.Default()
	}
	return &ModelRefresher{svc: svc, logger: logger, now: time.Now}
}

// Run ticks until ctx is cancelled. It is meant to be started as a goroutine.
//
// The first pass runs immediately rather than after one interval: a provider
// that has never been fetched is stale from the moment it is created, and making
// the operator wait 15 minutes for the loop's first look would be a delay with no
// purpose.
func (r *ModelRefresher) Run(ctx context.Context) {
	// One pass up front, before waiting for the first tick. A provider that has
	// never been fetched is stale from the moment it is created, and a workspace
	// that registers one right after a tick would otherwise wait a full interval
	// for its first model list — a delay with no purpose.
	r.RefreshOnce(ctx)

	ticker := time.NewTicker(RefresherInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.RefreshOnce(ctx)
		}
	}
}

// RefreshOnce performs one pass and reports how many lists it refreshed.
//
// It never returns an error: a failing upstream is the ordinary case (a provider
// the operator has not finished configuring, a laptop that is off), and the
// loop's job is to keep going and try again next tick. Failures are logged per
// provider, without the credential, and never abort the pass — one broken
// provider must not stop the other 46 from being refreshed.
func (r *ModelRefresher) RefreshOnce(ctx context.Context) int {
	ctx, cancel := context.WithTimeout(ctx, refresherTimeout)
	defer cancel()

	stale, err := r.svc.StaleProviders(ctx, r.now(), RefresherBatchSize)
	if err != nil {
		r.logger.Warn("provider model refresher: listing stale providers failed", "error", err)
		return 0
	}

	refreshed := 0
	for _, p := range stale {
		if ctx.Err() != nil {
			break
		}
		if _, err := r.svc.RefreshModels(ctx, p.OrgID, p.ID); err != nil {
			// The provider id and name are safe to log; the credential and the
			// address are not part of the message either way (see redact).
			r.logger.Warn("provider model refresher: refresh failed",
				"provider_id", p.ID, "provider_name", p.Name, "error", err)
			continue
		}
		refreshed++
	}
	if refreshed > 0 {
		r.logger.Info("provider model refresher: refreshed model lists",
			"count", refreshed, "stale_seen", len(stale))
	}
	return refreshed
}
