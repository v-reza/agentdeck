package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/store"
	"agentdeck/internal/ulid"
)

// pgxRepository is the production Repository: every committed write lands in
// Postgres and survives a process restart (ARCHITECTURE P4). Queries are the
// sqlc-generated type-safe bindings (P2): no query builder, no string-formed
// SQL, and every org-scoped query carries its org_id parameter explicitly.
type pgxRepository struct {
	pool *pgxpool.Pool
	q    *store.Queries
}

// NewPgxRepository builds the Postgres-backed Repository over a pool.
func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool, q: store.New(pool)}
}

func (r *pgxRepository) CreateUser(ctx context.Context, email, name, passwordHash string, isShadow bool) (User, error) {
	// The domain contract is "the row exists with this email when this
	// returns", not "a fresh row is inserted": AddMember creates the shadow
	// and Register later claims it, so a shadow already present is the row the
	// caller asked for. Inserting a second one would collide on
	// users_email_key and read as ErrEmailExists, which no caller of a shadow
	// row expects.
	if isShadow {
		if existing, err := r.q.GetUserByEmail(ctx, email); err == nil {
			if !existing.IsShadow {
				return User{}, ErrEmailExists
			}
			return userRowToUser(existing), nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return User{}, mapPgError(err)
		}
	}

	row, err := r.q.CreateUser(ctx, store.CreateUserParams{
		ID:           ulid.Must(),
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
		IsShadow:     isShadow,
	})
	if err != nil {
		return User{}, mapPgError(err)
	}
	return userRowToUser(row), nil
}
func (r *pgxRepository) ClaimShadowUser(ctx context.Context, email, name, passwordHash string) error {
	// The unique users_email_key index is on lower(email), so this lookup is
	// the stable handle for the pending-invitation row.
	user, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return mapPgError(err)
	}
	if !user.IsShadow {
		// A real account already owns this email; claiming it would overwrite
		// someone's password, so report the conflict instead.
		return ErrEmailExists
	}
	err = r.q.ClaimShadowUser(ctx, store.ClaimShadowUserParams{
		Lower:        user.Email,
		Name:         name,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return User{}, mapPgError(err)
	}
	return userRowToUser(row), nil
}

func (r *pgxRepository) GetUserByID(ctx context.Context, id string) (User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, mapPgError(err)
	}
	return userRowToUser(row), nil
}

// UpdateUserProfile writes the caller's own row in one statement. The
// `deleted_at IS NULL` predicate means a closed account matches zero rows, which
// pgx reports as ErrNoRows — mapped here to ErrUserNotFound so the handler can
// answer 404 without knowing whether the account exists (US-AD89 AC4).
func (r *pgxRepository) UpdateUserProfile(ctx context.Context, id, name, email, avatarURL string) (User, error) {
	row, err := r.q.UpdateUserProfile(ctx, store.UpdateUserProfileParams{
		ID:        id,
		Name:      name,
		Email:     email,
		AvatarURL: avatarURL,
	})
	if err != nil {
		// A taken address is the users_email_key unique violation, which
		// mapNotFoundWithEmail turns into ErrEmailExists → 409 (AC3). Any
		// other failure keeps its own mapping.
		return User{}, mapNotFoundWithEmail(err, ErrUserNotFound)
	}
	return userRowToUser(row), nil
}

func (r *pgxRepository) CreateOrg(ctx context.Context, id, slug, name, kind string) (Workspace, error) {
	row, err := r.q.CreateOrg(ctx, store.CreateOrgParams{ID: id, Slug: slug, Name: name})
	if err != nil {
		// The orgs_slug_key index is what makes an ErrSlugTaken here; a
		// no-row mapping is not reachable from an INSERT, but the sentinel
		// is still slug so the caller's 409 branch is the one that fires.
		return Workspace{}, mapSlugConflict(err)
	}
	if err := r.q.CreateOrgKind(ctx, store.CreateOrgKindParams{OrgID: id, Kind: kind}); err != nil {
		return Workspace{}, mapPgError(err)
	}
	return Workspace{
		ID:        row.ID,
		Slug:      row.Slug,
		Name:      row.Name,
		Kind:      kind,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *pgxRepository) PersonalWorkspace(ctx context.Context, userID string) (Workspace, error) {
	row, err := r.q.GetPersonalWorkspace(ctx, userID)
	if err != nil {
		// Register treats ErrWorkspaceNotFound as "create the personal
		// workspace now". ErrUserNotFound here would read as a vanishing user
		// and the registration would fail instead of provisioning one.
		return Workspace{}, domainNotFound(err, ErrWorkspaceNotFound)
	}
	return Workspace{
		ID:        row.ID,
		Slug:      row.Slug,
		Name:      row.Name,
		Kind:      personalOrgKind,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

// CountOrgOwners and CountOrgMembers back US-AD98 AC3. CountOrgMembers counts
// live users only: an account that is already closed is not "still in" the
// workspace, so a workspace whose only other member has themselves left is one
// this user may close.
func (r *pgxRepository) CountOrgMembers(ctx context.Context, orgID string) (int, error) {
	n, err := r.q.CountOrgMembers(ctx, orgID)
	return int(n), err
}

func (r *pgxRepository) SoftDeleteOrg(ctx context.Context, orgID string) error {
	return r.q.SoftDeleteOrg(ctx, orgID)
}

func (r *pgxRepository) SoftDeleteUser(ctx context.Context, userID string) error {
	return r.q.SoftDeleteUser(ctx, userID)
}

func (r *pgxRepository) GetOrgByID(ctx context.Context, id string) (Workspace, error) {
	row, err := r.q.GetOrgByID(ctx, id)
	if err != nil {
		return Workspace{}, mapNotFound(err, ErrWorkspaceNotFound)
	}
	return orgGetRowToWorkspace(row), nil
}

func (r *pgxRepository) GetOrgByIDIncludingDeleted(ctx context.Context, id string) (Workspace, error) {
	row, err := r.q.GetOrgByIDIncludingDeleted(ctx, id)
	if err != nil {
		return Workspace{}, mapNotFound(err, ErrWorkspaceNotFound)
	}
	return orgIncludingDeletedToWorkspace(row), nil
}

func (r *pgxRepository) UpdateOrgName(ctx context.Context, id, name string) error {
	err := r.q.UpdateOrgName(ctx, store.UpdateOrgNameParams{ID: id, Name: name})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) RenameOrgWithAudit(ctx context.Context, id, actorUserID, ip, beforeName, afterName string) error {
	before, err := json.Marshal(map[string]string{"name": beforeName})
	if err != nil {
		return err
	}
	after, err := json.Marshal(map[string]string{"name": afterName})
	if err != nil {
		return err
	}
	actor := actorUserID
	if actor == "" {
		actor = ""
	}
	err = r.q.RenameOrgWithAudit(ctx, store.RenameOrgWithAuditParams{
		OrgID: id, Name: afterName, ActorUserID: &actor, Column4: before, Column5: after, Ip: ip,
	})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) CreateMembership(ctx context.Context, orgID, userID string, role Role) error {
	// The generated query upserts, which would let a re-invite re-rank an
	// existing member — including an owner demoted by their own signup. The
	// inviter asked to add the user, not to change their role, so an existing
	// row is left exactly as it was and the create is reported as a no-op
	// success.
	if existing, err := r.q.GetMembership(ctx, store.GetMembershipParams{OrgID: orgID, UserID: userID}); err == nil {
		// A conflicting role would be a real state change; surface it instead
		// of pretending the invite landed with the requested rank.
		if Role(existing.Role) == role {
			return nil
		}
		return ErrMemberExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return memberNotFound(err)
	}
	err := r.q.CreateMembership(ctx, store.CreateMembershipParams{
		OrgID:  orgID,
		UserID: userID,
		Role:   string(role),
	})
	if err != nil {
		return memberNotFound(err)
	}
	return nil
}

func (r *pgxRepository) GetMembership(ctx context.Context, orgID, userID string) (MembershipRow, error) {
	row, err := r.q.GetMembership(ctx, store.GetMembershipParams{OrgID: orgID, UserID: userID})
	if err != nil {
		// resolveAndAuthorize decides 403 vs 404 on this error, so it must be
		// ErrMemberNotFound whether the pair has no row or one endpoint is
		// absent. Anything else reads as a server fault.
		return MembershipRow{}, memberNotFound(err)
	}
	return MembershipRow{
		OrgID:     row.OrgID,
		UserID:    row.UserID,
		Role:      Role(row.Role),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *pgxRepository) ListOrgsForUser(ctx context.Context, userID string) ([]OrgMembershipRow, error) {
	rows, err := r.q.ListOrgsForUser(ctx, userID)
	if err != nil {
		// A :many query never reports ErrNoRows; the only error here is a
		// hard one, so no not-found sentinel is invented for it.
		return nil, mapNotFound(err, ErrUserNotFound)
	}
	result := make([]OrgMembershipRow, 0, len(rows))
	for _, row := range rows {
		kind := ""
		if row.Kind != nil {
			kind = *row.Kind
		}
		result = append(result, OrgMembershipRow{
			OrgID: row.OrgID,
			Slug:  row.Slug,
			Name:  row.Name,
			Kind:  kind,
			Role:  Role(row.Role),
		})
	}
	return result, nil
}

func (r *pgxRepository) ListMembers(ctx context.Context, orgID string) ([]MemberRow, error) {
	rows, err := r.q.ListMembers(ctx, orgID)
	if err != nil {
		// A :many query returns no rows, not pgx.ErrNoRows, so this branch is
		// only for a hard failure; the org existence check is the caller's.
		return nil, mapNotFound(err, ErrWorkspaceNotFound)
	}
	result := make([]MemberRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, MemberRow{
			UserID:    row.UserID,
			Email:     row.Email,
			Name:      row.Name,
			Role:      Role(row.Role),
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return result, nil
}

func (r *pgxRepository) UpdateMembershipRole(ctx context.Context, orgID, userID string, role Role) error {
	err := r.q.UpdateMembershipRole(ctx, store.UpdateMembershipRoleParams{
		OrgID:  orgID,
		UserID: userID,
		Role:   string(role),
	})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) DeleteMembership(ctx context.Context, orgID, userID string) error {
	err := r.q.DeleteMembership(ctx, store.DeleteMembershipParams{OrgID: orgID, UserID: userID})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) CountOrgOwners(ctx context.Context, orgID string) (int, error) {
	count, err := r.q.CountOrgOwners(ctx, orgID)
	if err != nil {
		return 0, mapNotFound(err, ErrWorkspaceNotFound)
	}
	return int(count), nil
}

// ListSessionsForUser backs US-AD90 AC2. user_agent and ip are nullable in the
// DDL, so they are flattened to "" here: the screen shows "unknown device"
// rather than a literal "<nil>", and the distinction between "no header" and
// "empty header" does not survive to the UI anyway.
func (r *pgxRepository) ListSessionsForUser(ctx context.Context, userID string) ([]SessionInfo, error) {
	rows, err := r.q.ListSessionsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]SessionInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionInfo(row.ID, row.UserID, row.UserAgent, row.Ip, row.LastSeenAt, row.CreatedAt))
	}
	return out, nil
}

func (r *pgxRepository) GetSessionByID(ctx context.Context, id, userID string) (SessionInfo, error) {
	row, err := r.q.GetSessionByID(ctx, store.GetSessionByIDParams{ID: id, UserID: userID})
	if err != nil {
		return SessionInfo{}, mapNotFound(err, ErrSessionNotFound)
	}
	return sessionInfo(row.ID, row.UserID, row.UserAgent, row.Ip, row.LastSeenAt, row.CreatedAt), nil
}

func (r *pgxRepository) GetSessionAnyUser(ctx context.Context, id string) (SessionInfo, error) {
	row, err := r.q.GetSessionAnyUser(ctx, id)
	if err != nil {
		return SessionInfo{}, mapNotFound(err, ErrSessionNotFound)
	}
	return sessionInfo(row.ID, row.UserID, row.UserAgent, row.Ip, row.LastSeenAt, row.CreatedAt), nil
}

func (r *pgxRepository) RevokeSessionByID(ctx context.Context, id, userID string) (bool, error) {
	n, err := r.q.RevokeSessionByID(ctx, store.RevokeSessionByIDParams{ID: id, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *pgxRepository) RevokeOtherSessions(ctx context.Context, userID, keepID string) error {
	return r.q.RevokeOtherSessions(ctx, store.RevokeOtherSessionsParams{UserID: userID, ID: keepID})
}

func (r *pgxRepository) IsOrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	return r.q.IsOrgMember(ctx, store.IsOrgMemberParams{OrgID: orgID, UserID: userID})
}

func (r *pgxRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	return mapPgError(r.q.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{ID: userID, PasswordHash: passwordHash}))
}

// sessionInfo flattens the nullable wire columns. ip is a netip.Addr in the
// generated row because the column is inet; a NULL arrives as the zero value,
// which is what the IsValid check rejects.
func sessionInfo(id, userID string, agent *string, ip *netip.Addr, lastSeen, created pgtype.Timestamptz) SessionInfo {
	info := SessionInfo{ID: id, UserID: userID, LastSeenAt: lastSeen.Time, CreatedAt: created.Time}
	if agent != nil {
		info.UserAgent = *agent
	}
	if ip != nil && ip.IsValid() {
		info.IP = ip.String()
	}
	return info
}

func (r *pgxRepository) CreateSession(ctx context.Context, sess Session) error {
	err := r.q.CreateSession(ctx, store.CreateSessionParams{
		ID:         sess.ID,
		UserID:     sess.UserID,
		TokenHash:  sess.TokenHash,
		ExpiresAt:  pgTimestamptz(sess.ExpiresAt),
		LastSeenAt: pgTimestamptz(sess.LastSeenAt),
		CreatedAt:  pgTimestamptz(timeNow()),
		UserAgent:  sess.UserAgent,
		Ip:         sess.IP,
	})
	// Catatan: kolomnya nullable dan query memakai NULLIF, jadi string kosong
	// tersimpan sebagai NULL. IPv6 punya bentuk panjang (::ffff:127.0.0.1) dan
	// ditolak ::inet; nilai yang tidak valid disimpan sebagai NULL, bukan
	// menggagalkan pembuatan sesi — sesi yang tidak bisa dibuat berarti login
	// gagal, dan itu jauh lebih buruk daripada kolom IP yang kosong.
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) GetSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	row, err := r.q.GetSessionByTokenHash(ctx, store.GetSessionByTokenHashParams{
		TokenHash: tokenHash,
		Column2:   pgtype.Interval{Microseconds: int64(idleTimeout / time.Microsecond), Valid: true},
	})
	if err != nil {
		// The query's own WHERE filters already express the expiry and idle
		// window, so a missing row is the domain "no live session" and never
		// a missing user.
		return Session{}, mapNotFound(err, ErrSessionNotFound)
	}
	if err := r.q.TouchSession(ctx, tokenHash); err != nil {
		return Session{}, mapPgError(err)
	}
	return Session{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt.Time,
		LastSeenAt: row.LastSeenAt.Time,
	}, nil
}

func (r *pgxRepository) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	err := r.q.DeleteSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) DeleteExpiredSessions(ctx context.Context) error {
	err := r.q.DeleteExpiredSessions(ctx)
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) CreatePasswordReset(ctx context.Context, hash, userID string, expiresAt time.Time) error {
	err := r.q.CreatePasswordReset(ctx, store.CreatePasswordResetParams{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: pgTimestamptz(expiresAt),
		CreatedAt: pgTimestamptz(timeNow()),
	})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

// ConsumePasswordReset claims a token with a single conditional UPDATE
// (US-AD88 AC3). Zero affected rows means the token is unknown, already spent,
// or past its window — all three report the same error so the caller cannot
// tell them apart.
func (r *pgxRepository) ConsumePasswordReset(ctx context.Context, hash string) error {
	affected, err := r.q.ConsumePasswordReset(ctx, hash)
	if err != nil {
		return mapPgError(err)
	}
	if affected == 0 {
		return ErrResetTokenInvalid
	}
	return nil
}

func (r *pgxRepository) GetPasswordResetByTokenHash(ctx context.Context, hash string) (PasswordResetRow, error) {
	row, err := r.q.GetPasswordReset(ctx, hash)
	if err != nil {
		return PasswordResetRow{}, mapNotFound(err, ErrResetTokenInvalid)
	}
	reset := PasswordResetRow{
		TokenHash: row.TokenHash,
		UserID:    row.UserID,
		ExpiresAt: row.ExpiresAt.Time,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.UsedAt.Valid {
		usedAt := row.UsedAt.Time
		reset.UsedAt = &usedAt
	}
	return reset, nil
}

func (r *pgxRepository) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	err := r.q.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{ID: userID, PasswordHash: passwordHash})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) DeleteUserSessions(ctx context.Context, userID string) error {
	err := r.q.DeleteUserSessions(ctx, userID)
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

// orgRowToWorkspace converts the CreateOrg/GetOrg row into the domain
// Workspace. kind comes from the separate org_kinds table; callers that need
// it join it in the read path (ListOrgsForUser) rather than denormalizing.
func orgRowToWorkspace(row store.Org) Workspace {
	return Workspace{
		ID:        row.ID,
		Slug:      row.Slug,
		Name:      row.Name,
		CreatedAt: row.CreatedAt.Time,
	}
}

func orgGetRowToWorkspace(row store.Org) Workspace {
	return Workspace{
		ID:        row.ID,
		Slug:      row.Slug,
		Name:      row.Name,
		CreatedAt: row.CreatedAt.Time,
	}
}

// orgIncludingDeletedToWorkspace carries DeletedAt, unlike orgGetRowToWorkspace:
// the whole point of the query behind it is to tell an org that is gone from one
// that is merely closed.
func orgIncludingDeletedToWorkspace(row store.Org) Workspace {
	workspace := Workspace{
		ID:        row.ID,
		Slug:      row.Slug,
		Name:      row.Name,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.DeletedAt.Valid {
		deletedAt := row.DeletedAt.Time
		workspace.DeletedAt = &deletedAt
	}
	return workspace
}

func rowToUser(storeUser store.User) User {
	return User{
		ID:           storeUser.ID,
		Email:        storeUser.Email,
		Name:         storeUser.Name,
		PasswordHash: storeUser.PasswordHash,
		IsShadow:     storeUser.IsShadow,
		CreatedAt:    storeUser.CreatedAt.Time,
	}
}

// userRowToUser converts a CreateUser/GetUser row into the domain User. A
// shadow row is marked by its password-hash sentinel, which the domain layer
// checks via IsShadow. All four sqlc row types carry the same columns, so the
// generic keeps one converter instead of four copies.
type userRow interface {
	store.CreateUserRow | store.GetUserByEmailRow | store.GetUserByIDRow | store.UpdateUserProfileRow
}

func userRowToUser[T userRow](row T) User {
	var out User
	switch any(row).(type) {
	case store.CreateUserRow:
		r := any(row).(store.CreateUserRow)
		out = User{ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash,
			AvatarURL: avatarOrEmpty(r.AvatarURL), IsShadow: r.IsShadow, CreatedAt: r.CreatedAt.Time}
	case store.GetUserByEmailRow:
		r := any(row).(store.GetUserByEmailRow)
		out = User{ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash,
			AvatarURL: avatarOrEmpty(r.AvatarURL), IsShadow: r.IsShadow, CreatedAt: r.CreatedAt.Time}
	case store.GetUserByIDRow:
		r := any(row).(store.GetUserByIDRow)
		out = User{ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash,
			AvatarURL: avatarOrEmpty(r.AvatarURL), IsShadow: r.IsShadow, CreatedAt: r.CreatedAt.Time}
	case store.UpdateUserProfileRow:
		r := any(row).(store.UpdateUserProfileRow)
		out = User{ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash,
			AvatarURL: avatarOrEmpty(r.AvatarURL), IsShadow: r.IsShadow, CreatedAt: r.CreatedAt.Time}
	}
	return out
}

// avatarOrEmpty flattens the nullable column into the domain type: NULL means
// "no uploaded avatar", which is exactly the empty string the shell treats as
// "draw the monogram" (US-AD89 AC1). Keeping the pointer out of the domain
// removes a nil check from every screen that renders the avatar.
func avatarOrEmpty(url *string) string {
	if url == nil {
		return ""
	}
	return *url
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timeNow() time.Time { return time.Now() }

// mapPgError turns a driver/pgx error into the domain error the HTTP layer
// maps to a status code. A unique violation on users(email) is 409
// (ErrEmailExists); a missing row is the domain not-found the caller asks for
// (F3: the default sentinel is picked by what was queried, not hard-coded to
// ErrUserNotFound); anything else is a 500-class internal failure the handler
// reports without detail.
//
// domainNotFound converts a driver "no rows" into the domain not-found error
// the caller's switch expects. A query whose missing row means "user" is
// distinct from one whose missing row means "workspace": collapsing both into
// a single sentinel would make a first-time registration read as if the user
// vanished, so the sentinel is picked by what was queried.
func domainNotFound(err error, notFound error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	// pgx may surface the sentinel through a wrapper; unwrap it explicitly.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "21000" { // cardinality_violation
		return notFound
	}
	return err
}

// memberNotFound reports the error a membership lookup must surface: either no
// row matched, or the write could not have matched because one of its
// endpoints is absent. A foreign-key violation on memberships is the same
// fact as a missing membership row — the (org, user) pair has no record — so
// both become ErrMemberNotFound and never leak as a 500.
func memberNotFound(err error) error {
	if err == nil {
		return nil
	}
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrMemberNotFound
	}
	return domainNotFound(err, ErrMemberNotFound)
}

// mapNotFound maps a read whose missing row means exactly one thing, so the
// caller supplies the sentinel (F3: not-found is contextual, not always a
// missing user). It passes constraint violations through untouched: the only
// constraint that can fire on these reads is one the caller did not ask about,
// and reinterpreting it as an email conflict would 404 a 409.
func mapNotFound(err error, notFound error) error {
	if err == nil {
		return nil
	}
	return domainNotFound(err, notFound)
}

// mapSlugConflict reports a CreateOrg failure. The only constraint that can
// fire on an org insert is orgs_slug_key, and that is a 409 slug conflict —
// never the email conflict mapPgError returns for users.email (F3).
func mapSlugConflict(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return ErrSlugTaken
	}
	return err
}

// mapPgError is the default mapping for writes and for reads whose only
// not-found case is a missing user.
func mapPgError(err error) error {
	return mapNotFoundWithEmail(err, ErrUserNotFound)
}

// mapNotFoundWithEmail is the user-context mapping: a missing row is a missing
// user, a unique violation is users.email, and a foreign-key violation is a
// membership pair that has no row.
func mapNotFoundWithEmail(err error, notFound error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return ErrEmailExists
		case "23503": // foreign_key_violation
			return ErrMemberNotFound
		}
	}
	return domainNotFound(err, notFound)
}
