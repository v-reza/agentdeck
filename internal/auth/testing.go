package auth

import (
	"testing"
	"time"
)

// setSessionExpiry and setSessionLastSeen move a stored session across the
// US-AD02 AC4 boundaries. The domain layer deliberately has no API for forging
// an expired session: the tests reach into the repository instead, so the
// Store's public surface stays honest.
func setSessionExpiry(t *testing.T, s *Store, sessionToken string, expiresAt time.Time) {
	t.Helper()
	repo, ok := s.repo.(*memoryRepository)
	if !ok {
		t.Fatalf("setSessionExpiry: need *memoryRepository, got %T", s.repo)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	hash := tokenHash(sessionToken)
	session, exists := repo.sessions[hash]
	if !exists {
		t.Fatalf("setSessionExpiry: no session for token")
	}
	session.ExpiresAt = expiresAt
	repo.sessions[hash] = session
}

func setSessionLastSeen(t *testing.T, s *Store, sessionToken string, lastSeen time.Time) {
	t.Helper()
	repo, ok := s.repo.(*memoryRepository)
	if !ok {
		t.Fatalf("setSessionLastSeen: need *memoryRepository, got %T", s.repo)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	hash := tokenHash(sessionToken)
	session, exists := repo.sessions[hash]
	if !exists {
		t.Fatalf("setSessionLastSeen: no session for token")
	}
	session.LastSeenAt = lastSeen
	repo.sessions[hash] = session
}

func sessionLastSeen(t *testing.T, s *Store, sessionToken string) time.Time {
	t.Helper()
	repo, ok := s.repo.(*memoryRepository)
	if !ok {
		t.Fatalf("sessionLastSeen: need *memoryRepository, got %T", s.repo)
	}
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	session, exists := repo.sessions[tokenHash(sessionToken)]
	if !exists {
		t.Fatalf("sessionLastSeen: no session for token")
	}
	return session.LastSeenAt
}
