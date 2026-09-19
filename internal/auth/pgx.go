package auth

import (
	"context"
	"errors"
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

func (r *pgxRepository) CreateOrg(ctx context.Context, id, slug, name, kind string) (Workspace, error) {
	row, err := r.q.CreateOrg(ctx, store.CreateOrgParams{ID: id, Slug: slug, Name: name})
	if err != nil {
		return Workspace{}, mapPgError(err)
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

func (r *pgxRepository) GetOrgByID(ctx context.Context, id string) (Workspace, error) {
	row, err := r.q.GetOrgByID(ctx, id)
	if err != nil {
		return Workspace{}, mapPgError(err)
	}
	return orgRowToWorkspace(row), nil
}

func (r *pgxRepository) UpdateOrgName(ctx context.Context, id, name string) error {
	err := r.q.UpdateOrgName(ctx, store.UpdateOrgNameParams{ID: id, Name: name})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) CreateMembership(ctx context.Context, orgID, userID string, role Role) error {
	err := r.q.CreateMembership(ctx, store.CreateMembershipParams{
		OrgID:  orgID,
		UserID: userID,
		Role:   string(role),
	})
	if err != nil {
		return mapPgError(err)
	}
	return nil
}

func (r *pgxRepository) GetMembership(ctx context.Context, orgID, userID string) (MembershipRow, error) {
	row, err := r.q.GetMembership(ctx, store.GetMembershipParams{OrgID: orgID, UserID: userID})
	if err != nil {
		return MembershipRow{}, mapPgError(err)
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
		return nil, mapPgError(err)
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
		return nil, mapPgError(err)
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
		return 0, mapPgError(err)
	}
	return int(count), nil
}

func (r *pgxRepository) CreateSession(ctx context.Context, sess Session) error {
	err := r.q.CreateSession(ctx, store.CreateSessionParams{
		ID:         sess.ID,
		UserID:     sess.UserID,
		TokenHash:  sess.TokenHash,
		ExpiresAt:  pgTimestamptz(sess.ExpiresAt),
		LastSeenAt: pgTimestamptz(sess.LastSeenAt),
		CreatedAt:  pgTimestamptz(timeNow()),
	})
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
		return Session{}, mapPgError(err)
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
// checks via IsShadow. All three sqlc row types carry the same columns, so the
// generic keeps one converter instead of three copies.
type userRow interface {
	store.CreateUserRow | store.GetUserByEmailRow | store.GetUserByIDRow
}

func userRowToUser[T userRow](row T) User {
	var out User
	switch any(row).(type) {
	case store.CreateUserRow:
		r := any(row).(store.CreateUserRow)
		out = User{ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash,
			IsShadow: r.IsShadow, CreatedAt: r.CreatedAt.Time}
	case store.GetUserByEmailRow:
		r := any(row).(store.GetUserByEmailRow)
		out = User{ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash,
			IsShadow: r.IsShadow, CreatedAt: r.CreatedAt.Time}
	case store.GetUserByIDRow:
		r := any(row).(store.GetUserByIDRow)
		out = User{ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash,
			IsShadow: r.IsShadow, CreatedAt: r.CreatedAt.Time}
	}
	return out
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timeNow() time.Time { return time.Now() }

// mapPgError turns a driver/pgx error into the domain error the HTTP layer
// maps to a status code. A unique violation on users(email) is 409
// (ErrEmailExists); a missing row is the domain not-found for that query;
// anything else is a 500-class internal failure the handler reports without
// detail.

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

func mapPgError(err error) error {
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
	return domainNotFound(err, ErrUserNotFound)
}
