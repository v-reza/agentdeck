-- AgentDeck schema v0.1, M0 baseline (Postgres 16).
-- Canonical DDL follows docs/ARCHITECTURE.md section 3. Run by internal/migrate
-- (embed.FS); every statement is guarded so re-applying the file is a no-op.
-- Extension and runtime-role block: ARCHITECTURE section 3.1. The runtime role
-- owns no DDL and cannot bypass row-level security.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'agentdeck_app') THEN
        CREATE ROLE agentdeck_app LOGIN;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO agentdeck_app;

-- The runtime role owns no objects: 0001 runs as the schema owner, so these
-- grants must be TO the role, never FROM it. Privileges granted BY the owner
-- would attach to agentdeck_app as pg_shdepend entries and make the role
-- undroppable (SQLSTATE 2BP01) -- the runtime role must stay disposable.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;

-- orgs: the tenant boundary. Every domain row carries this id.
CREATE TABLE IF NOT EXISTS orgs (
    id          text        NOT NULL,
    slug        text        NOT NULL,
    name        text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT orgs_pk              PRIMARY KEY (id),
    CONSTRAINT orgs_id_ulid_chk     CHECK (char_length(id) = 26),
    CONSTRAINT orgs_slug_chk        CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'),
    CONSTRAINT orgs_name_chk        CHECK (btrim(name) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS orgs_slug_key ON orgs (lower(slug));

-- users: global identity. One person may belong to several orgs, so org_id
-- lives on memberships, not here.
CREATE TABLE IF NOT EXISTS users (
    id            text        NOT NULL,
    email         text        NOT NULL,
    name          text        NOT NULL,
    password_hash text        NOT NULL,
    avatar_url    text,
    deleted_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_pk              PRIMARY KEY (id),
    CONSTRAINT users_id_ulid_chk     CHECK (char_length(id) = 26),
    CONSTRAINT users_email_chk       CHECK (position('@' IN email) > 1),
    CONSTRAINT users_password_chk    CHECK (password_hash LIKE '$argon2id$%')
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_key ON users (lower(email));

-- memberships: the user-to-tenant bridge. Composite key means one role per org.
CREATE TABLE IF NOT EXISTS memberships (
    org_id     text        NOT NULL,
    user_id    text        NOT NULL,
    role       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT memberships_pk          PRIMARY KEY (org_id, user_id),
    CONSTRAINT memberships_role_chk    CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    CONSTRAINT memberships_org_fk      FOREIGN KEY (org_id)  REFERENCES orgs(id)  ON DELETE CASCADE,
    CONSTRAINT memberships_user_fk     FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS memberships_user_idx ON memberships (user_id, org_id);

-- sessions: opaque server-side login sessions. token_hash is SHA-256 of the
-- 64-byte token; the raw token only ever exists in the cookie.
CREATE TABLE IF NOT EXISTS sessions (
    id            text        NOT NULL,
    user_id       text        NOT NULL,
    token_hash    text        NOT NULL,
    user_agent    text,
    ip            inet,
    last_seen_at  timestamptz,
    expires_at    timestamptz NOT NULL,
    deleted_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sessions_pk            PRIMARY KEY (id),
    CONSTRAINT sessions_id_ulid_chk   CHECK (char_length(id) = 26),
    CONSTRAINT sessions_hash_chk      CHECK (char_length(token_hash) = 64),
    CONSTRAINT sessions_user_fk       FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- NOTE: ARCHITECTURE 3.19 specifies "WHERE expires_at < now()" for this partial
-- index. Postgres rejects that: now() is not IMMUTABLE, so it cannot appear in
-- an index predicate. A plain index serves the same cleaner query
-- (DELETE FROM sessions WHERE expires_at < now()) without the restriction.
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);
