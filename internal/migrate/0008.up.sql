-- M1 — the run trace, the cost ledger, and the org skill library exist as
-- contracts but had no DDL: ARCHITECTURE 3.7b/3.10/3.14 were documented (and
-- DECISIONS 6A declared the pricing/cost-ledger contract frozen), yet none of
-- these tables was ever created. Every code path that records what an agent
-- did, what it cost, or which skill it loaded had nowhere to write.
--
-- Order is forced, not stylistic: ledger_entries carries
-- `FOREIGN KEY (run_id) REFERENCES runs(id)`, and runs did not exist, so
-- ledger_entries cannot be created before it. runs itself needs tasks and
-- agents (both already present). agent_skills depends only on orgs.
--
-- The two agents columns come from DECISIONS 6A.F (provider BYO) and US-AD73
-- (archive an agent without deleting it). They are added with IF NOT EXISTS so
-- a database that already carries them — from a hand-run hotfix — re-applies
-- cleanly instead of aborting the whole migration transaction.
--
-- Everything is IF NOT EXISTS: Apply runs each file exactly once and records
-- the version, so a second pass is a no-op, but a partially applied database
-- (or a developer running the file by hand) must still converge.

ALTER TABLE agents ADD COLUMN IF NOT EXISTS base_url TEXT;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

-- Postgres has no ADD CONSTRAINT IF NOT EXISTS, so the guard is explicit. The
-- constraint is the arbiter (not application code): a base_url on a built-in
-- provider is a misconfiguration that would silently route traffic to an
-- attacker-chosen host, so the database refuses the row.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'agents_base_url_chk'
          AND conrelid = 'agents'::regclass
    ) THEN
        ALTER TABLE agents ADD CONSTRAINT agents_base_url_chk
            CHECK ((provider = 'openai_compatible') = (base_url IS NOT NULL));
    END IF;
END $$;

-- Skill is data, not a constant: users read and edit the markdown. Agents may
-- only read it, which is why nothing here grants a write path to an agent.
CREATE TABLE IF NOT EXISTS agent_skills (
    id          TEXT        NOT NULL,
    org_id      TEXT        NOT NULL,
    slug        TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    body_md     TEXT        NOT NULL DEFAULT '',
    version     INTEGER     NOT NULL DEFAULT 1,
    is_system   BOOLEAN     NOT NULL DEFAULT false,
    created_by  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_skills_pk        PRIMARY KEY (id),
    CONSTRAINT agent_skills_id_chk    CHECK (char_length(id) = 26),
    CONSTRAINT agent_skills_slug_chk  CHECK (slug ~ '^[a-z0-9_]{1,64}$'),
    CONSTRAINT agent_skills_org_fk    FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS agent_skills_org_slug_key ON agent_skills (org_id, slug);

-- One execution of one task by one agent. The row is the unit of truth for
-- reclaim (stale heartbeat), retry accounting (attempt), and cost rollup.
CREATE TABLE IF NOT EXISTS runs (
    id                   TEXT        NOT NULL,
    org_id               TEXT        NOT NULL,
    task_id              TEXT        NOT NULL,
    agent_id             TEXT        NOT NULL,
    attempt              SMALLINT    NOT NULL DEFAULT 1,
    status               TEXT        NOT NULL DEFAULT 'pending',
    outcome              TEXT,
    failure_kind         TEXT,
    claim_lock           TEXT,
    claim_expires        TIMESTAMPTZ,
    worker_pid           INTEGER,
    last_heartbeat_at    TIMESTAMPTZ,
    max_runtime_seconds  INTEGER     NOT NULL DEFAULT 14400,
    cost_micros          BIGINT      NOT NULL DEFAULT 0,
    tokens_in            BIGINT      NOT NULL DEFAULT 0,
    tokens_out           BIGINT      NOT NULL DEFAULT 0,
    summary              TEXT,
    error                TEXT,
    metadata_json        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at             TIMESTAMPTZ,
    CONSTRAINT runs_pk               PRIMARY KEY (id),
    CONSTRAINT runs_id_ulid_chk      CHECK (char_length(id) = 26),
    CONSTRAINT runs_status_chk       CHECK (status IN ('pending','claiming','running','ended')),
    CONSTRAINT runs_outcome_chk      CHECK (outcome IN ('succeeded','failed','timed_out','cancelled','reclaimed','budget_exceeded')),
    CONSTRAINT runs_failure_kind_chk CHECK (failure_kind IN (
        'transient','needs_input','capability','dependency','policy','budget','unknown'
    )),
    CONSTRAINT runs_cost_nonneg_chk  CHECK (cost_micros >= 0),
    CONSTRAINT runs_tokens_chk       CHECK (tokens_in >= 0 AND tokens_out >= 0),
    CONSTRAINT runs_attempt_chk      CHECK (attempt BETWEEN 1 AND 10),
    CONSTRAINT runs_task_fk          FOREIGN KEY (task_id)  REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT runs_agent_fk         FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS runs_stale_claim_idx ON runs (status, last_heartbeat_at)
    WHERE status = 'running' AND claim_expires IS NOT NULL;
CREATE INDEX IF NOT EXISTS runs_task_idx ON runs (task_id, attempt DESC);
CREATE INDEX IF NOT EXISTS runs_status_cost_idx ON runs (status) WHERE status = 'ended';

-- Per-step cost. cost_micros is the computed price × quantity, not a unit
-- price: the ledger must stay readable after a price table changes, which is
-- also why price_version is mandatory. Only a priced entry is ever written —
-- an unpriced call is recorded as price_source='unpriced' with cost 0 rather
-- than dropped, so spend underreporting is visible instead of silent.
CREATE TABLE IF NOT EXISTS ledger_entries (
    id                 BIGSERIAL   NOT NULL,
    org_id             TEXT        NOT NULL,
    run_id             TEXT        NOT NULL,
    task_id            TEXT        NOT NULL,
    provider           TEXT        NOT NULL,
    model              TEXT        NOT NULL,
    kind               TEXT        NOT NULL DEFAULT 'llm',
    tokens_in          BIGINT      NOT NULL DEFAULT 0,
    tokens_out         BIGINT      NOT NULL DEFAULT 0,
    cache_read_tokens  BIGINT      NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT      NOT NULL DEFAULT 0,
    reasoning_tokens   BIGINT      NOT NULL DEFAULT 0,
    cost_micros        BIGINT      NOT NULL,
    price_version      INTEGER     NOT NULL,
    price_source       TEXT        NOT NULL DEFAULT 'catalog',
    pricing_model      TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ledger_entries_pk                PRIMARY KEY (id),
    CONSTRAINT ledger_entries_kind_chk          CHECK (kind IN ('llm','cache_read','cache_write','tool')),
    CONSTRAINT ledger_entries_price_source_chk  CHECK (price_source IN ('manual','catalog','pattern','unpriced')),
    CONSTRAINT ledger_entries_cost_chk          CHECK (cost_micros >= 0),
    CONSTRAINT ledger_entries_tokens_chk        CHECK (tokens_in >= 0 AND tokens_out >= 0 AND cache_read_tokens >= 0 AND cache_write_tokens >= 0 AND reasoning_tokens >= 0),
    CONSTRAINT ledger_entries_run_fk            FOREIGN KEY (run_id)  REFERENCES runs(id) ON DELETE CASCADE,
    CONSTRAINT ledger_entries_task_fk           FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ledger_entries_org_time_idx ON ledger_entries (org_id, created_at);
CREATE INDEX IF NOT EXISTS ledger_entries_run_idx ON ledger_entries (run_id);

-- Same repair as 0003: ALL TABLES only matches objects that exist at that
-- moment, and the runtime role owns nothing (ARCHITECTURE 3.1), so the tables
-- above are invisible to the API until they are granted here. Without this the
-- migration "succeeds" and every new query fails with permission denied.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
