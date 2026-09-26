package executor

import (
	"net/http"
	"time"
)

// client is the HTTP client every run uses. It is shared so connections are
// reused across steps, and it carries no timeout of its own: the per-run context
// is what bounds a call, because 5.3 gives the agent's max_runtime_seconds that
// job and a client-level timeout would cut long completions short.
//
// Redirects are NOT followed. A provider that answers 302 is pointing somewhere
// this run did not validate, and the credential is in the header being replayed:
// following it would hand the key to whatever host the redirect names.
//
// ponytail: one process-wide client. Upgrade path is a per-org pool if a tenant
// ever needs its own proxy or TLS settings.
var client = &http.Client{
	Timeout: 0,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
	},
}
