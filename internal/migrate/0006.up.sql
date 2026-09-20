-- M0 — audit trail required by US-AD77 AC2.
-- The viewer remains a later M5 story; this migration only enables fail-closed
-- recording of workspace-setting mutations.
CREATE TABLE IF NOT EXISTS audit_log (
    id             BIGSERIAL   NOT NULL,
    org_id         TEXT        NOT NULL,
    actor_user_id  TEXT,
    actor_agent_id TEXT,
    action         TEXT        NOT NULL,
    target_type    TEXT        NOT NULL,
    target_id      TEXT        NOT NULL,
    before_json    JSONB,
    after_json     JSONB,
    ip             TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audit_log_pk PRIMARY KEY (id),
    CONSTRAINT audit_log_org_fk FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE,
    CONSTRAINT audit_log_actor_user_fk FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS audit_log_org_action_idx ON audit_log (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_log_target_idx ON audit_log (target_type, target_id, created_at DESC);
