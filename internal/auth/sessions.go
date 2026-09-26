package auth

import (
	"context"
	"errors"
	"strings"
)

// Sesi aktif dan ganti password — US-AD90, plus pencabutan oleh admin US-AD05.

// Sessions lists the caller's own live sessions, most recently used first.
//
// US-AD90 AC2 wants the list to carry "perangkat, IP, waktu masuk terakhir, dan
// penanda 'perangkat ini'". The current-device flag is not stored: it is a
// comparison against the request's own session id, which only the caller knows.
// Storing it would mean writing a row every time a user opens the page from a
// different device, to record something derivable from one equality.
func (s *Store) Sessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	return s.repo.ListSessionsForUser(ctx, userID)
}

// ChangePassword implements US-AD90 AC1/AC3.
//
// Order matters: verify, then write, then revoke. Revoking before the write
// would leave the user with a new password and no sessions if the write failed;
// revoking before verifying would be a way to log someone out by guessing.
//
// The caller's own session is kept (`RevokeOtherSessions`), because AC1 says the
// current session stays active — signing the user out of the tab they just used
// to change their password is the behaviour the criterion exists to prevent.
func (s *Store) ChangePassword(ctx context.Context, userID, currentSessionID, oldPassword, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return ErrInvalidInput
	}
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !verifyPassword(user.PasswordHash, oldPassword) {
		// AC3: 401 and nothing changes. No failure counter here — the caller is
		// already authenticated, so this is not a credential-stuffing surface,
		// and locking an account because its owner mistyped once would be a
		// denial of service with the user as the attacker.
		return ErrWrongPassword
	}
	if err := s.repo.UpdatePassword(ctx, userID, hashPassword(newPassword)); err != nil {
		return err
	}
	return s.repo.RevokeOtherSessions(ctx, userID, currentSessionID)
}

// RevokeSession implements US-AD90 AC4 and US-AD05 AC2.
//
// Two floors in one route, like the archive endpoint:
//
//   - your own session: allowed, and a second request is a no-op rather than an
//     error (AC4's list is a list of buttons, and a double click is not a fault).
//   - someone else's: owner/admin only, AND only for a member of the workspace
//     the caller is acting in. Without the membership check, "admin" would mean
//     admin of the installation — any admin could sign out any user anywhere.
//
// US-AD05 AC3 says revoking your OWN session through the forced endpoint is
// refused ("use logout"). US-AD90 AC4 says the opposite. ARCHITECTURE 6.2.2's
// row resolves it toward AC4, and that is the reading implemented here: the
// self case is allowed, because a session list whose own row cannot be removed
// is a list with a button that does nothing. The conflict is noted in
// docs/OPEN-ISSUES.md.
func (s *Store) RevokeSession(ctx context.Context, actorID, actorEmail, orgID, sessionID string, actorIsAdmin bool) error {
	target, err := s.repo.GetSessionAnyUser(ctx, sessionID)
	if err != nil {
		return err
	}

	if target.UserID == actorID {
		_, err := s.repo.RevokeSessionByID(ctx, sessionID, actorID)
		return err
	}

	if !actorIsAdmin {
		return ErrForbidden
	}
	member, err := s.repo.IsOrgMember(ctx, orgID, target.UserID)
	if err != nil {
		return err
	}
	if !member {
		// Same answer as an unknown session, on purpose: "that session belongs
		// to someone outside your workspace" is itself information about another
		// tenant, and US-AD89 AC4's reasoning applies.
		return ErrSessionNotFound
	}
	_, err = s.repo.RevokeSessionByID(ctx, sessionID, target.UserID)
	return err
}

// CloseAccount implements US-AD98.
//
// AC1 requires typing the email; AC5 makes the closure soft for 30 days. Both
// are why this is not a DELETE: a hard delete could not be undone, and the
// confirmation would be theatre.
//
// AC3 is the interesting one: a user who is the last owner of a workspace that
// still has other people in it is refused. Otherwise closing an account would
// silently orphan a shared workspace — the remaining members would have a
// workspace nobody can administer.
//
// AC2 deletes "workspaces only this user owns". Ownership here is membership:
// a workspace with exactly one member, this user. A workspace with other people
// in it is left alone (and, per AC3, blocks the closure if the caller owns it).
func (s *Store) CloseAccount(ctx context.Context, userID, confirmEmail string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(confirmEmail), user.Email) {
		return ErrAccountClosureConfirm
	}

	orgs, err := s.repo.ListOrgsForUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, org := range orgs {
		owners, err := s.repo.CountOrgOwners(ctx, org.OrgID)
		if err != nil {
			return err
		}
		members, err := s.repo.CountOrgMembers(ctx, org.OrgID)
		if err != nil {
			return err
		}
		lastOwner := owners <= 1
		// AC3: the caller is the last owner and other people are still in the
		// workspace. Refuse rather than transfer ownership silently — a silent
		// promotion is a permission change nobody asked for or saw.
		if lastOwner && members > 1 && org.Role == Owner {
			return ErrLastOwner
		}
		// AC2: a workspace with no other members goes with the account. Ones
		// with other people stay, so their work is not destroyed by someone
		// else's account closure.
		if members <= 1 {
			if err := s.repo.SoftDeleteOrg(ctx, org.OrgID); err != nil {
				return err
			}
		}
	}

	if err := s.repo.SoftDeleteUser(ctx, userID); err != nil {
		return err
	}
	// AC2: every session goes. A live session for a closed account is refused by
	// Authenticate on its next use, but revoking is the honest state — and it
	// makes the closure observable immediately rather than at next request.
	return s.repo.DeleteUserSessions(ctx, userID)
}

// ErrNoSessionForCaller is returned when a handler needs the caller's own
// session id and cannot find it. It should be unreachable: every route that
// calls ChangePassword or RevokeSession is behind the session middleware, which
// has already resolved a session to reach the handler.
var ErrNoSessionForCaller = errors.New("caller session could not be resolved")
