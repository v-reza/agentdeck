-- name: CreateUser :one
INSERT INTO users (id, email, name, password_hash, is_shadow)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, email, name, password_hash, avatar_url, is_shadow, deleted_at, created_at;

-- name: ClaimShadowUser :exec
UPDATE users
SET name          = $2,
    password_hash = $3,
    is_shadow     = false
WHERE lower(email) = lower($1) AND is_shadow;

-- name: GetUserByEmail :one
SELECT id, email, name, password_hash, avatar_url, is_shadow, deleted_at, created_at
FROM users
WHERE lower(email) = lower($1) AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT id, email, name, password_hash, avatar_url, is_shadow, deleted_at, created_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateOrg :one
INSERT INTO orgs (id, slug, name)
VALUES ($1, $2, $3)
RETURNING id, slug, name, created_at;

-- name: GetOrgByID :one
SELECT id, slug, name, created_at
FROM orgs
WHERE id = $1;

-- name: UpdateOrgName :exec
UPDATE orgs
SET name = $2
WHERE id = $1;

-- name: GetPersonalWorkspace :one
-- The registration-kind org where THIS user is the owner: that is the only
-- shape that means "my personal workspace". Membership alone is not enough —
-- an invitee is a member of someone else's registration-kind org, and
-- resolving it here would hand them a workspace that is not theirs.
SELECT o.id, o.slug, o.name, o.created_at
FROM org_kinds k
JOIN orgs o ON o.id = k.org_id
WHERE k.kind = 'registration'
  AND EXISTS (
      SELECT 1 FROM memberships m
      WHERE m.org_id = o.id
        AND m.user_id = $1
        AND m.role   = 'owner'
  );

-- name: CreateOrgKind :exec
INSERT INTO org_kinds (org_id, kind)
VALUES ($1, $2);

-- name: CreateMembership :exec
INSERT INTO memberships (org_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (org_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

-- The single membership row that answers "is this user in this org, and as
-- what". Every org-scoped handler resolves its tenant through this query, so
-- there is no second code path that could forget the org_id scope.
-- name: GetMembership :one
SELECT org_id, user_id, role, created_at
FROM memberships
WHERE org_id = $1 AND user_id = $2;

-- The org roster a user belongs to, with their role in each. Scoping is
-- by membership, not by the caller's guess of an org id, so this cannot
-- leak a tenant the user is not part of (tenant isolation, ARCHITECTURE 17).
-- name: ListOrgsForUser :many
SELECT m.org_id, o.slug, o.name, k.kind, m.role
FROM memberships m
JOIN orgs o ON o.id = m.org_id
LEFT JOIN org_kinds k ON k.org_id = m.org_id
WHERE m.user_id = $1
ORDER BY m.created_at;

-- name: ListMembers :many
SELECT m.user_id, u.email, u.name, m.role, m.created_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = $1
ORDER BY m.created_at, u.email;

-- name: UpdateMembershipRole :exec
UPDATE memberships
SET role = $3
WHERE org_id = $1 AND user_id = $2;

-- name: DeleteMembership :exec
DELETE FROM memberships
WHERE org_id = $1 AND user_id = $2;

-- Guards the last-owner rule: an org must never be left without an owner.
-- name: CountOrgOwners :one
SELECT count(*)::int
FROM memberships
WHERE org_id = $1 AND role = 'owner';

-- token_hash is SHA-256 of the 64-byte raw token; the raw token only ever
-- exists in the Set-Cookie (ARCHITECTURE 3.19).
-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, token_hash, expires_at, last_seen_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- Sliding idle window (US-AD02 AC4): a session that has been idle longer than
-- the idle timeout is rejected, so last_seen_at is bumped on every use.
-- name: GetSessionByTokenHash :one
SELECT id, user_id, token_hash, last_seen_at, expires_at, created_at
FROM sessions
WHERE token_hash = $1
  AND deleted_at IS NULL
  AND expires_at > now()
  AND last_seen_at > now() - ($2::interval);

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = now()
WHERE token_hash = $1 AND deleted_at IS NULL;

-- name: DeleteSessionByTokenHash :exec
UPDATE sessions
SET deleted_at = now()
WHERE token_hash = $1 AND deleted_at IS NULL;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at < now();
