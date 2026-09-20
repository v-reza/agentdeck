-- M0 — password reset (US-AD88).
--
-- The token is single-use and time-boxed, so the row is consumed by an UPDATE
-- that also stamps used_at: a SELECT-then-UPDATE pair would let two concurrent
-- redemptions both read an unconsumed row. Expiry and single-use are both in
-- the WHERE clause, so an expired or already-spent token matches zero rows and
-- the caller cannot tell the two apart (AC3).

-- name: CreatePasswordReset :exec
INSERT INTO password_resets (token_hash, user_id, expires_at, created_at)
VALUES ($1, $2, $3, $4);

-- name: ConsumePasswordReset :execrows
UPDATE password_resets
SET used_at = now()
WHERE token_hash = $1
  AND used_at IS NULL
  AND expires_at > now();

-- name: GetPasswordReset :one
SELECT token_hash, user_id, expires_at, used_at, created_at
FROM password_resets
WHERE token_hash = $1;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2
WHERE id = $1 AND deleted_at IS NULL;

-- Revokes every session of one user (US-AD88 AC2). Deliberately not scoped by
-- token: after a reset the operator has no trusted device, so all of them go.
-- name: DeleteUserSessions :exec
UPDATE sessions
SET deleted_at = now()
WHERE user_id = $1 AND deleted_at IS NULL;
