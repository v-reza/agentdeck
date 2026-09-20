-- M0 — password reset tokens (US-AD88 AC1).
--
-- Only the SHA-256 of the emailed token is stored, exactly like sessions: a
-- store dump yields no usable reset link. used_at is the single-use marker and
-- expires_at the 30-minute window (AC3), so both rules live in the data and a
-- concurrent redemption cannot slip past a handler-level check.

CREATE TABLE IF NOT EXISTS password_resets (
    token_hash text        NOT NULL,
    user_id    text        NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT password_resets_pk        PRIMARY KEY (token_hash),
    CONSTRAINT password_resets_hash_chk  CHECK (char_length(token_hash) = 64),
    CONSTRAINT password_resets_user_fk   FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- The reaper and the "drop a user's outstanding links" path both scan by user.
CREATE INDEX IF NOT EXISTS password_resets_user_idx ON password_resets (user_id);

-- Expiry sweeps are ordered by time, so the index is on expires_at alone.
CREATE INDEX IF NOT EXISTS password_resets_expires_idx ON password_resets (expires_at);
