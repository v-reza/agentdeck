-- US-AD109 / DECISIONS 6A.J, fase 1 — provider registry, plus the backfill that
-- moves every existing agent's address + credential onto it.
--
-- Why the table exists: today the address and the credential live on the agent
-- (`agents.base_url` + `agents.provider_api_key_enc`), so one upstream key is
-- copied once per agent that uses it and rotating it means editing every one of
-- them. `providers` holds that pair once per workspace; agents point at it via
-- `agents.provider_id`, so one edit reaches all of them (AC6).
--
-- Order is forced, not stylistic: `providers` must exist before
-- `agents.provider_id` can be filled, and both before the backfill reads them.
--
-- Everything is guarded (IF NOT EXISTS / ON CONFLICT DO NOTHING): Apply runs
-- each file once and records the version, but a database that already carries
-- part of this — from a hand-run hotfix, or a re-run after the version row is
-- removed — must still converge instead of aborting the whole transaction.

CREATE TABLE IF NOT EXISTS providers (
    id                text        NOT NULL,
    org_id            text        NOT NULL,
    name              text        NOT NULL,
    protocol          text        NOT NULL,
    base_url          text        NOT NULL,
    api_key_enc       bytea,
    models_json       jsonb       NOT NULL DEFAULT '[]'::jsonb,
    models_fetched_at timestamptz,
    last_verified_at  timestamptz,
    is_default        boolean     NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT providers_pk           PRIMARY KEY (id),
    CONSTRAINT providers_id_ulid_chk  CHECK (char_length(id) = 26),
    CONSTRAINT providers_name_chk     CHECK (btrim(name) <> ''),
    CONSTRAINT providers_protocol_chk CHECK (protocol IN ('openai_compatible','anthropic','google')),
    CONSTRAINT providers_base_url_chk CHECK (btrim(base_url) <> ''),
    CONSTRAINT providers_models_chk   CHECK (jsonb_typeof(models_json) = 'array'),
    CONSTRAINT providers_org_fk       FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE
);

-- One name per workspace, and at most one default per workspace. The partial
-- index is what makes "delete the default provider -> the workspace simply has
-- none" (AC9) a database fact rather than a convention.
CREATE UNIQUE INDEX IF NOT EXISTS providers_org_name_key    ON providers (org_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS providers_org_default_key ON providers (org_id) WHERE is_default;

-- Serves: GET /api/v1/providers, and counting users before a delete (AC5).
CREATE INDEX IF NOT EXISTS providers_org_idx ON providers (org_id);

-- Nullable on purpose, and it stays that way after the backfill below. NULL
-- means "this agent has no provider of its own and uses the platform default",
-- which is the same statement `provider_api_key_enc IS NULL` already makes
-- (ARCHITECTURE 16). An agent with neither an address nor a credential has
-- nothing to move onto a provider, and inventing a vendor base URL for it would
-- contradict DECISIONS 6A.J ("no ready-made OpenAI/Anthropic templates").
--
-- No FK to providers(id): the contract's DDL (ARCHITECTURE 3) does not declare
-- one, and an FK cannot express the part that matters — that the provider
-- belongs to the same org as the agent. AC5 must query the users anyway,
-- because the 409 has to name them.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS provider_id text;

-- Backfill. Grouped by (org_id, base_url, api_key_enc), NOT by org: two agents
-- in one workspace pointing at different addresses are two providers, and
-- collapsing them would silently repoint one agent at the other's endpoint.
-- Only agents that actually carry an address are migrated — an agent with
-- `base_url IS NULL` has nothing to move.
--
-- The id is the lowest agent id in the group. Those are ULIDs already: 26 chars,
-- unique, and ordered by creation time, so the provider id satisfies the ID rule
-- (DECISIONS 6 "Aturan ID") without a second ULID implementation living in SQL.
-- Deterministic, which is also what lets the re-run below converge instead of
-- inserting a fresh random id per attempt.
--
-- `protocol` is provably 'openai_compatible': agents_base_url_chk (still in
-- force — phase 6 drops it, not this migration) states that base_url is NOT NULL
-- exactly when provider = 'openai_compatible'.
--
-- models_json is seeded with the models the grouped agents are actually using,
-- so a migrated agent is not immediately in a state where its own model is
-- missing from its provider's allowlist (the gate DECISIONS 6A.J puts back in
-- place for phase 5). models_fetched_at stays NULL: this is not the result of a
-- fetch. Phase 3 must therefore read NULL as "never fetched, fetch now" rather
-- than "fetched long ago, no refresh needed".
WITH existing AS (
    SELECT a.org_id,
           a.base_url,
           a.provider_api_key_enc                                        AS api_key_enc,
           min(a.id)                                                     AS provider_id,
           regexp_replace(a.base_url, '^[A-Za-z][A-Za-z0-9+.-]*://([^/]+).*$', '\1') AS host,
           jsonb_agg(DISTINCT to_jsonb(a.model))                          AS models
    FROM agents a
    WHERE a.base_url IS NOT NULL
    GROUP BY a.org_id, a.base_url, a.provider_api_key_enc
),
numbered AS (
    SELECT e.*,
           row_number() OVER (PARTITION BY e.org_id, e.host ORDER BY e.base_url) AS host_seq,
           count(*)     OVER (PARTITION BY e.org_id)                             AS org_providers
    FROM existing e
)
INSERT INTO providers (id, org_id, name, protocol, base_url, api_key_enc, models_json, models_fetched_at, is_default)
SELECT n.provider_id,
       n.org_id,
       -- Two addresses on the same host in one workspace still need distinct
       -- names (providers_org_name_key); the common single-provider case keeps
       -- the bare host.
       CASE WHEN n.host_seq = 1 THEN n.host ELSE n.host || ' ' || n.host_seq END,
       'openai_compatible',
       n.base_url,
       n.api_key_enc,
       n.models,
       NULL,
       -- The workspace's only provider becomes its default, which preserves the
       -- behaviour the migrated agents already had. With more than one there is
       -- no obvious default, and AC9 allows the form to ask.
       (n.org_providers = 1)
FROM numbered n
ON CONFLICT DO NOTHING;

UPDATE agents a
SET provider_id = p.id
FROM providers p
WHERE a.provider_id IS NULL
  AND a.base_url IS NOT NULL
  AND p.org_id = a.org_id
  AND p.base_url = a.base_url
  AND p.api_key_enc IS NOT DISTINCT FROM a.provider_api_key_enc;

-- Same repair as 0003 and 0008: GRANT ... ON ALL TABLES only matches objects
-- that exist at that moment, and the runtime role owns nothing (ARCHITECTURE
-- 3.1), so `providers` is invisible to the API until it is granted here.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
