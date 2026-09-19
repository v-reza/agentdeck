-- AgentDeck schema v0.1.1, M0 persistence layer (Postgres 16).
-- Extends the v0.1 baseline (orgs/users/memberships/sessions, still applied by
-- 0001.up.sql) with the M0 auth-workspace primitives the API actually writes:
-- personal workspaces are orgs, and invitations create pending memberships.
-- Every statement is guarded so re-applying the file is a no-op.
-- Follows docs/ARCHITECTURE.md section 3 / DECISIONS.md P2+P4; IDs are ULID TEXT(26).

-- Personal workspaces and invitation-created orgs record their provenance so the
-- UI/API can tell an invited org apart from one the user created themselves.
-- 'registration' = the personal workspace created on signup (US-AD01 AC5).
-- 'invitation'   = org created to satisfy a pending membership (US-AD04).
-- 'manual'       = created through POST /api/v1/orgs.
CREATE TABLE IF NOT EXISTS org_kinds (
    org_id     text        NOT NULL,
    kind       text        NOT NULL DEFAULT 'manual',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_kinds_pk       PRIMARY KEY (org_id),
    CONSTRAINT org_kinds_kind_chk CHECK (kind IN ('registration', 'invitation', 'manual')),
    CONSTRAINT org_kinds_org_fk   FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS org_kinds_kind_idx ON org_kinds (kind);

-- Registrations with a pending invitation need a shadow user row: the membership
-- must point at a real user_id even before the invitee signs up. A shadow has no
-- password and cannot log in until registration claims the row.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_shadow boolean NOT NULL DEFAULT false;

-- Index the pending-registration lookup: find the shadow row by email.
CREATE INDEX IF NOT EXISTS users_shadow_idx ON users (is_shadow, lower(email));
