package auth

import (
	"context"
	"strconv"
	"sync"
	"testing"
)

// TestStoreConcurrentAccess exercises the store's mutexes without the race
// detector: goroutines hammer register/login/authenticate/logout in parallel,
// then verifies every expected state change landed. This host has no C
// compiler so -race cannot build; a deterministic concurrency check is the
// fallback (ARCHITECTURE 17.1 unit-test layer).
func TestStoreConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	store := NewStore(NewMemoryRepository())

	const workers = 8
	const iterations = 25

	var wait sync.WaitGroup
	wait.Add(workers * 2)

	for worker := 0; worker < workers; worker++ {
		go func(index int) {
			defer wait.Done()

			for i := 0; i < iterations; i++ {
				email := "user" + strconv.Itoa(index) + "@example.com"
				_, _, token, err := store.Register(ctx, email, "password1", "", "", SessionMeta{})
				if err == nil && token != "" {
					if _, ok := store.Authenticate(ctx, token); !ok {
						t.Errorf("worker %d: fresh session did not authenticate", index)
					}
					store.Logout(ctx, token)
					if _, ok := store.Authenticate(ctx, token); ok {
						t.Errorf("worker %d: logged-out session authenticated", index)
					}
				}
			}
		}(worker)

		go func(index int) {
			defer wait.Done()

			for i := 0; i < iterations; i++ {
				email := "user" + strconv.Itoa(index) + "@example.com"
				if _, err := store.Login(ctx, email, "password1", SessionMeta{}); err == nil &&
					!store.Authorize(ctx, "ws-"+email, email, Owner) {
					t.Errorf("worker %d: owner authorization failed", index)
				}
			}
		}(worker)
	}

	wait.Wait()

	for worker := 0; worker < workers; worker++ {
		email := "user" + strconv.Itoa(worker) + "@example.com"
		list, lerr := store.Workspaces(ctx, email)
		if lerr != nil || len(list) != 1 {
			t.Errorf("worker %d: workspaces = %d, want 1", worker, len(list))
		}
	}
}
