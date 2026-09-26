-- daily_board_costs — the aggregate the dispatcher reads before claiming.
--
-- ARCHITECTURE 3.8 defines this table and 4c reads it, but no migration ever
-- created it: the budget guardrail (N16 cap, N18 80% alert) had a documented
-- query with no table under it. The cost of that gap is not a missing figure in
-- the UI — it is the dispatcher, which cannot enforce a cap it cannot read.
--
-- The three counters are maintained by the executor as steps finish, and the
-- dispatcher reads them instead of summing ledger_entries on every tick: the
-- aggregate is one row per board per day, while the ledger grows without bound
-- (4c says exactly this, and prices the alternative as the dominant cost).
--
-- `day` is a UTC date. DECISIONS 6 makes every money field an integer count of
-- micro-USD, so total_micros is BIGINT with a non-negative CHECK; a float here
-- would round every run and drift.

CREATE TABLE IF NOT EXISTS daily_board_costs (
    org_id          TEXT        NOT NULL,
    board_id        TEXT        NOT NULL,
    day             DATE        NOT NULL,          -- UTC date
    total_micros    BIGINT      NOT NULL DEFAULT 0,
    run_count       INTEGER     NOT NULL DEFAULT 0,
    tokens_in       BIGINT      NOT NULL DEFAULT 0,
    tokens_out      BIGINT      NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT daily_board_costs_pk PRIMARY KEY (board_id, day),
    CONSTRAINT daily_board_costs_org_fk FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE,
    CONSTRAINT daily_board_costs_fk FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
    CONSTRAINT daily_board_costs_nonneg_chk CHECK (total_micros >= 0 AND run_count >= 0)
);

CREATE INDEX IF NOT EXISTS daily_board_costs_org_day_idx ON daily_board_costs (org_id, day);

-- Same repair as 0003/0008: ALL TABLES only matches objects that exist at that
-- moment, and the runtime role owns nothing (ARCHITECTURE 3.1), so the table
-- above is invisible to the API until it is granted here. Without this the
-- migration "succeeds" and every new query fails with permission denied.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
