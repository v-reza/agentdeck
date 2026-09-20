package auth

import (
	"context"
	"net/url"
	"strings"
)

// ProfileUpdate is a partial update of the caller's own profile (US-AD89 AC2).
//
// Each field is a pointer because PATCH has three states, not two: absent
// ("leave it alone"), present with a value ("set it"), and present but empty
// ("clear it" — rejected, since a nameless account and a blank avatar URL are
// both meaningless). A plain string collapses absent and empty into one case, so
// `{"name": ""}` would silently keep the old name instead of telling the caller
// the value is not acceptable.
type ProfileUpdate struct {
	Name      *string
	Email     *string
	AvatarURL *string
}

// UpdateProfile implements PATCH /api/v1/auth/me (ARCHITECTURE 6.2.2).
//
// The target is always the authenticated user's own id, never a client-supplied
// one: US-AD89 AC4 is satisfied structurally, because no code path exists in
// which a caller names the account it mutates. An unknown id is
// ErrUserNotFound (404) and never ErrForbidden — a 403 would confirm the
// account is real.
//
// A taken email surfaces as ErrEmailExists (409) from the repository and the
// write does not land: the conflict is raised by the users_email_key index
// inside the single UPDATE, so there is no read-then-write window in which two
// accounts could both claim the same address.
func (s *Store) UpdateProfile(ctx context.Context, userID string, patch ProfileUpdate) (User, error) {
	current, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}

	name := current.Name
	if patch.Name != nil {
		trimmed := strings.TrimSpace(*patch.Name)
		if trimmed == "" {
			return User{}, ErrInvalidInput
		}
		name = trimmed
	}

	email := current.Email
	if patch.Email != nil {
		normalized, err := normalizeEmail(*patch.Email)
		if err != nil {
			return User{}, ErrInvalidInput
		}
		email = normalized
	}

	avatar := current.AvatarURL
	if patch.AvatarURL != nil {
		if err := validateAvatarURL(*patch.AvatarURL); err != nil {
			return User{}, err
		}
		avatar = strings.TrimSpace(*patch.AvatarURL)
	}

	if name == current.Name && email == current.Email && avatar == current.AvatarURL {
		// Nothing to write. Returning the current identity keeps PATCH
		// idempotent (ARCHITECTURE 6.2.2 marks it so) and skips an UPDATE
		// that would take a row lock to store the values already there.
		return current, nil
	}

	return s.repo.UpdateUserProfile(ctx, userID, name, email, avatar)
}

// UserByID loads one live identity by id. It backs the "own profile only" read
// path of US-AD89 AC4: an unknown id is ErrUserNotFound (404).
func (s *Store) UserByID(ctx context.Context, userID string) (User, error) {
	return s.repo.GetUserByID(ctx, strings.TrimSpace(userID))
}

// validateAvatarURL accepts only an absolute http(s) URL. The value is rendered
// as an image source on the profile screen, so a `javascript:` or `data:` URL is
// a stored-XSS vector; rejecting the scheme here means no screen has to remember
// to sanitise it.
func validateAvatarURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ErrInvalidInput
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidInput
	}
	if parsed.Host == "" {
		return ErrInvalidInput
	}
	return nil
}
