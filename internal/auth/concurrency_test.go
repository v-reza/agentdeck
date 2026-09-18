package auth

import (
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
	store := NewStore()

	const workers = 8
	const iterations = 25

	var wait sync.WaitGroup
	wait.Add(workers * 2)

	for worker := 0; worker < workers; worker++ {
		go func(index int) {
			defer wait.Done()

			for i := 0; i < iterations; i++ {
				email := "user" + strconv.Itoa(index) + "@example.com"
				_, _, token, err := store.Register(email, "password1", "", "")
				if err == nil && token != "" {
					if _, ok := store.Authenticate(token); !ok {
						t.Errorf("worker %d: fresh session did not authenticate", index)
					}
					store.Logout(token)
					if _, ok := store.Authenticate(token); ok {
						t.Errorf("worker %d: logged-out session authenticated", index)
					}
				}
			}
		}(worker)

		go func(index int) {
			defer wait.Done()

			for i := 0; i < iterations; i++ {
				email := "user" + strconv.Itoa(index) + "@example.com"
				if _, err := store.Login(email, "password1"); err == nil &&
					!store.Authorize("ws-"+email, email, Owner) {
					t.Errorf("worker %d: owner authorization failed", index)
				}
			}
		}(worker)
	}

	wait.Wait()

	for worker := 0; worker < workers; worker++ {
		email := "user" + strconv.Itoa(worker) + "@example.com"
		if list := store.Workspaces(email); len(list) != 1 {
			t.Errorf("worker %d: workspaces = %d, want 1", worker, len(list))
		}
	}
}
