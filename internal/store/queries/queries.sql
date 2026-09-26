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

-- One statement, so a taken email (23505 on users_email_key) rolls the whole
-- profile update back — no half-applied name change (US-AD89 AC2/AC3). The
-- `deleted_at IS NULL` guard is what turns a closed account into zero rows.
--
-- The cast on $4 is load-bearing: `NULLIF($4, '')` alone gives sqlc no type to
-- infer, so it falls back to a positional `Column4 interface{}` whose exact
-- spelling drifts between sqlc releases and breaks the caller. `sqlc.arg` names
-- the parameter and `::text` pins the type, so the generated field is stable.
-- name: UpdateUserProfile :one
UPDATE users
SET name       = $2,
    email      = $3,
    avatar_url = NULLIF(sqlc.arg(avatar_url)::text, '')
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, email, name, password_hash, avatar_url, is_shadow, deleted_at, created_at;

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

-- name: RenameOrgWithAudit :exec
-- Keep the rename and its audit row in one statement: if the INSERT fails, the
-- data-modifying CTE is rolled back too (US-AD77 fail-closed).
WITH renamed AS (
    UPDATE orgs
    SET name = $2
    WHERE id = $1
    RETURNING id
)
INSERT INTO audit_log (org_id, actor_user_id, action, target_type, target_id, before_json, after_json, ip)
SELECT $1, $3, 'org.rename', 'org', $1, $4::jsonb, $5::jsonb, $6
FROM renamed;

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

-- ============================================================================
-- M1 — projects, boards, agents, tasks, task_links, events
--
-- Tenant scoping rule (ARCHITECTURE 17): every query below that touches an
-- org-scoped row carries org_id explicitly, even when the id alone would be
-- unique. There is no second code path that could forget the scope.
-- ============================================================================

-- name: CreateProject :one
INSERT INTO projects (id, org_id, slug, name)
VALUES ($1, $2, $3, $4)
RETURNING id, org_id, slug, name, created_at;

-- name: GetProject :one
SELECT id, org_id, slug, name, created_at
FROM projects
WHERE id = $1 AND org_id = $2;

-- name: ListProjects :many
SELECT id, org_id, slug, name, created_at
FROM projects
WHERE org_id = $1
ORDER BY created_at DESC;

-- name: UpdateProjectName :exec
UPDATE projects
SET name = $3
WHERE id = $1 AND org_id = $2;

-- name: DeleteProject :exec
DELETE FROM projects
WHERE id = $1 AND org_id = $2;

-- Boards. columns_json is the board's view of status, never a new status.
-- name: CreateBoard :one
INSERT INTO boards (id, org_id, project_id, slug, name, columns_json, budget_daily_micros)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, org_id, project_id, slug, name, columns_json, budget_daily_micros, created_at;

-- name: GetBoard :one
SELECT id, org_id, project_id, slug, name, columns_json, budget_daily_micros, created_at
FROM boards
WHERE id = $1 AND org_id = $2;

-- name: ListBoards :many
SELECT id, org_id, project_id, slug, name, columns_json, budget_daily_micros, created_at
FROM boards
WHERE org_id = $1 AND project_id = $2
ORDER BY created_at DESC;

-- name: UpdateBoardColumns :exec
UPDATE boards
SET columns_json = $3
WHERE id = $1 AND org_id = $2;

-- name: UpdateBoardName :exec
UPDATE boards
SET name = $3
WHERE id = $1 AND org_id = $2;

-- name: UpdateBoardBudget :exec
UPDATE boards
SET budget_daily_micros = $3
WHERE id = $1 AND org_id = $2;

-- name: DeleteBoard :exec
DELETE FROM boards
WHERE id = $1 AND org_id = $2;

-- name: CountBoardsInProject :one
SELECT count(*)::int
FROM boards
WHERE org_id = $1 AND project_id = $2;

-- Agents. The agent is the retry/limit source for every run it executes.
-- name: CreateAgent :one
-- Every mutable column UpdateAgent writes is written here too. The two drifted
-- once: `reasoning_effort` was bound only by UpdateAgent, so a client's value
-- was dropped in silence on create. internal/store/queries_columns_test.go is
-- the guard that keeps the two lists in step.
--
-- provider_id is the registry reference (US-AD109). It is written here for the
-- same reason as everything else on this list: a create that omits it leaves
-- the agent pointing at no provider, which phase 5 reads as "no credential".
INSERT INTO agents (id, org_id, project_id, name, provider, model, reasoning_effort,
                    skills_json, tools_json, max_runtime_seconds, retry_policy, max_attempts,
                    provider_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts,
          archived_at, created_at, has_provider_key, provider_id;

-- name: GetAgent :one
SELECT id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
       tools_json, max_runtime_seconds, retry_policy, max_attempts,
       archived_at, created_at, has_provider_key, provider_id
FROM agents
WHERE id = $1 AND org_id = $2;

-- name: ListAgents :many
-- Returns archived rows too, deliberately. The registry is where a user finds an
-- agent again to unarchive it, so filtering them out here would make archiving
-- irreversible from the UI. Callers that must not offer a retired agent (the
-- assign dropdown, US-AD73 AC2) filter on `archived_at` themselves — see
-- ListAgentsUsingSkill and the task-assign path.
SELECT id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
       tools_json, max_runtime_seconds, retry_policy, max_attempts,
       archived_at, created_at, has_provider_key, provider_id
FROM agents
WHERE org_id = $1 AND project_id = $2
ORDER BY name;

-- name: DeleteAgent :exec
DELETE FROM agents WHERE id = $1 AND org_id = $2;

-- name: UpdateAgent :one
-- US-AD96: the edit form replaces every mutable field at once, so this is a
-- full update rather than a partial patch.
--
-- provider_id is the registry reference (US-AD109). It is nullable and stays
-- that way: an agent with no provider of its own uses the workspace default.
UPDATE agents
SET name = $3, provider = $4, model = $5, reasoning_effort = $6,
    skills_json = $7, tools_json = $8, max_runtime_seconds = $9,
    retry_policy = $10, max_attempts = $11, provider_id = $12
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts,
          archived_at, created_at, has_provider_key, provider_id;

-- name: ArchiveAgent :one
-- US-AD73: archived agents keep their row (running tasks still resolve their
-- retry/limit source) but disappear from every assign dropdown.
UPDATE agents
SET archived_at = now()
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts,
          archived_at, created_at, has_provider_key, provider_id;

-- name: UnarchiveAgent :one
UPDATE agents
SET archived_at = NULL
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts,
          archived_at, created_at, has_provider_key, provider_id;

-- name: SetAgentProviderKey :one
-- US-AD86: store the sealed credential. Encryption/decryption lives in
-- internal/crypto; this statement only ever sees ciphertext, so a DB dump alone
-- cannot recover a provider key. Returning the derived flag lets the handler
-- answer without a second round-trip.
UPDATE agents
SET provider_api_key_enc = $3
WHERE id = $1 AND org_id = $2
RETURNING id, has_provider_key;

-- name: ClearAgentProviderKey :one
-- Rotation and revocation are the same statement with a NULL ciphertext.
UPDATE agents
SET provider_api_key_enc = NULL
WHERE id = $1 AND org_id = $2
RETURNING id, has_provider_key;

-- name: GetAgentProviderKey :one
-- The ONLY reader of the ciphertext column. Returns it alone so the sealed bytes
-- never travel inside a struct that gets logged, cached, or serialised.
SELECT provider_api_key_enc
FROM agents
WHERE id = $1 AND org_id = $2;

-- name: ListAssignableAgents :many
-- US-AD73 AC2: the assign dropdown must never offer a retired agent. This is the
-- deliberate counterpart to ListAgents, which returns archived rows so the
-- registry can unarchive them.
SELECT id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
       tools_json, max_runtime_seconds, retry_policy, max_attempts,
       archived_at, created_at, has_provider_key, provider_id
FROM agents
WHERE org_id = $1 AND archived_at IS NULL
ORDER BY name;

-- name: CountAgentRunningTasks :one
-- Guards US-AD20 AC4: an agent holding a task in `running` may not be deleted,
-- because the run it is executing would lose its retry/limit source mid-flight.
SELECT count(*) FROM tasks
WHERE assignee_agent_id = $1 AND org_id = $2 AND status = 'running';

-- Agent skills (US-AD107). Skill is org-scoped data: users read and edit the
-- markdown, agents only read it. Nothing here exposes a write path an agent
-- could reach, and every query carries org_id explicitly.

-- name: CreateAgentSkill :one
INSERT INTO agent_skills (id, org_id, slug, name, body_md, is_system, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, org_id, slug, name, body_md, version, is_system, created_by, created_at, updated_at;

-- name: GetAgentSkill :one
SELECT id, org_id, slug, name, body_md, version, is_system, created_by, created_at, updated_at
FROM agent_skills
WHERE id = $1 AND org_id = $2;

-- name: ListAgentSkillsWithUsage :many
-- The list needs "dipakai N agent" on every row, so usage is resolved in the
-- same round trip: a per-row query would be N+1 against a table the user scrolls.
-- `?` is jsonb containment for a top-level array element, i.e. the slug is in
-- agents.skills_json. Archived agents are excluded: they no longer receive work,
-- so they must not make a skill look live.
SELECT s.id, s.org_id, s.slug, s.name, s.body_md, s.version, s.is_system, s.created_by,
       s.created_at, s.updated_at,
       (SELECT count(*) FROM agents a
         WHERE a.org_id = s.org_id AND a.archived_at IS NULL AND a.skills_json ? s.slug) AS used_by
FROM agent_skills s
WHERE s.org_id = $1
ORDER BY s.is_system DESC, s.slug;

-- name: ListAgentsUsingSkill :many
-- Feeds the "dipakai oleh" list under the editor; each name routes to the agent
-- detail page, which is what stops a user from editing a live skill blindly.
SELECT id, name FROM agents
WHERE org_id = $1 AND archived_at IS NULL AND skills_json ? $2
ORDER BY name;

-- name: UpdateAgentSkill :one
-- AC6: version rises on every edit and older content is never rewritten in
-- place, so a run that already loaded v3 keeps meaning what it meant.
UPDATE agent_skills
SET name = $3, body_md = $4, version = version + 1, updated_at = now()
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, slug, name, body_md, version, is_system, created_by, created_at, updated_at;

-- name: DeleteAgentSkill :exec
-- System skills are not deletable: they are the baseline every workspace starts
-- from, and removing one would silently strip capability from existing agents.
DELETE FROM agent_skills WHERE id = $1 AND org_id = $2 AND is_system = false;

-- Providers (US-AD109, DECISIONS 6A.J). The credential and the address live
-- here, once per workspace, instead of once per agent: agents point at a
-- provider row, so one edit reaches every agent that uses it (AC6). Every query
-- carries org_id explicitly, like the rest of this file.

-- name: CreateProvider :one
INSERT INTO providers (id, org_id, name, protocol, base_url, api_key_enc, is_default)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, org_id, name, protocol, base_url, api_key_enc, models_json,
          models_fetched_at, last_verified_at, is_default, created_at;

-- name: GetProvider :one
-- Returns api_key_enc because the handler layer is what decrypts, and it needs
-- the sealed bytes to do so. Nothing in this file puts them in a response.
SELECT id, org_id, name, protocol, base_url, api_key_enc, models_json,
       models_fetched_at, last_verified_at, is_default, created_at
FROM providers
WHERE id = $1 AND org_id = $2;

-- name: ListProviders :many
-- The default sorts first because it is what the agent form preselects (AC9).
SELECT id, org_id, name, protocol, base_url, api_key_enc, models_json,
       models_fetched_at, last_verified_at, is_default, created_at
FROM providers
WHERE org_id = $1
ORDER BY is_default DESC, name;

-- name: CountProviders :one
-- Decides whether a provider being created becomes the workspace default: the
-- first one does, so a workspace that has exactly one never has to pick.
SELECT count(*)::int FROM providers WHERE org_id = $1;

-- name: UpdateProvider :one
-- A partial update, deliberately — not the full-replace shape UpdateAgent uses.
-- A full replace would blank every column the caller omitted, and three of them
-- (models_json, models_fetched_at, last_verified_at) belong to phase 3: renaming
-- a provider would silently erase its model list and its verification
-- timestamp. COALESCE keeps whatever the request leaves out, and those three
-- columns are not in the SET list at all.
--
-- There is no way to clear api_key_enc here: passing NULL means "leave it", so
-- a provider that has a credential keeps it. Removing a credential is not in
-- the contract's seven endpoints.
UPDATE providers
SET name        = COALESCE(sqlc.narg('name'), name),
    protocol    = COALESCE(sqlc.narg('protocol'), protocol),
    base_url    = COALESCE(sqlc.narg('base_url'), base_url),
    api_key_enc = COALESCE(sqlc.narg('api_key_enc'), api_key_enc),
    is_default  = COALESCE(sqlc.narg('is_default'), is_default)
WHERE id = sqlc.arg('id') AND org_id = sqlc.arg('org_id')
RETURNING id, org_id, name, protocol, base_url, api_key_enc, models_json,
          models_fetched_at, last_verified_at, is_default, created_at;

-- name: ClearDefaultProvider :exec
-- providers_org_default_key is a non-deferrable partial unique index, so a
-- single `UPDATE ... SET is_default = (id = $x)` can still raise 23505: Postgres
-- checks the index per row, and the new default may be visited before the old
-- one is unset. Clearing first and setting second are two statements that
-- cannot collide. AC9 makes the cleared state legitimate, not a half-write.
UPDATE providers SET is_default = false WHERE org_id = $1 AND is_default;

-- name: DeleteProvider :exec
DELETE FROM providers WHERE id = $1 AND org_id = $2;

-- name: ListAgentsUsingProvider :many
-- AC5: the 409 has to name the agents pinning this provider, so this is a read
-- of names the caller can already list, not a count.
SELECT id, name FROM agents
WHERE org_id = $1 AND provider_id = $2
ORDER BY name;

-- name: GetProviderKey :one
-- The one query that hands out ciphertext, for the one path that decrypts it
-- before probing an upstream (phase 3). Returning it as a scalar rather than as
-- a column of GetProvider is deliberate: the domain Provider carries no
-- ciphertext, so no read path can leak what it never receives.
-- api_key_enc is nullable (a local endpoint that checks nothing), and a NULL
-- scans into a nil []byte rather than an error.
SELECT api_key_enc FROM providers WHERE id = $1 AND org_id = $2;

-- name: SetProviderModels :exec
-- AC7: the fetched list and the moment it was fetched, written together so a
-- freshness check can never see one without the other.
UPDATE providers SET models_json = $3, models_fetched_at = $4
WHERE id = $1 AND org_id = $2;

-- name: SetProviderVerifiedAt :exec
-- AC3: stamped only after the inference probe passes. Nothing else writes this
-- column, so a non-null value always means a probe succeeded.
UPDATE providers SET last_verified_at = $3
WHERE id = $1 AND org_id = $2;

-- name: ListStaleProviderModels :many
-- AC7's automatic half. This is the ONE provider query deliberately not scoped by
-- org_id: the background refresher has no tenant in hand — it walks every
-- workspace — so a `WHERE org_id = $1` here would make it impossible to write.
-- Request-serving code must never call this; the reads that answer a user are
-- List/Get, and both carry org_id.
--
-- NULL models_fetched_at is stale, not fresh: "never fetched" is exactly the
-- state that needs a fetch. The ORDER BY puts those first so a workspace that
-- just registered a provider is served before one that merely aged out.
--
-- The id tiebreaker makes the batch deterministic. Without it the order among
-- equally-stale rows is unspecified, so with more stale providers than the batch
-- cap the same subset can be chosen every pass and the rest starve.
-- ponytail: deterministic is not the same as fair — a permanently broken provider
-- stays stale forever and keeps its slot. Serving every workspace round-robin
-- needs a last_attempt_at column; add it if a provider is ever observed never
-- getting its turn.
SELECT id, org_id, name, protocol, base_url, api_key_enc, models_json,
       models_fetched_at, last_verified_at, is_default, created_at
FROM providers
WHERE models_fetched_at IS NULL OR models_fetched_at < sqlc.arg('before')
ORDER BY models_fetched_at NULLS FIRST, id
LIMIT sqlc.arg('max_rows');

-- Tasks. created_by is the acting user; assignee_agent_id is nullable.
-- name: CreateTask :one
INSERT INTO tasks (id, org_id, board_id, title, body, status, priority, assignee_agent_id,
                   created_by, idempotency_key, workspace_kind, goal_mode, goal_max_turns)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, org_id, board_id, title, body, status, priority, assignee_agent_id, created_by,
          idempotency_key, block_kind, consecutive_failures, workspace_kind, workspace_path,
          branch_name, completion_contract, goal_mode, goal_max_turns, current_run_id,
          cost_micros, tokens_in, tokens_out, created_at, started_at, completed_at, archived_at;

-- name: GetTask :one
SELECT id, org_id, board_id, title, body, status, priority, assignee_agent_id, created_by,
       idempotency_key, block_kind, consecutive_failures, workspace_kind, workspace_path,
       branch_name, completion_contract, goal_mode, goal_max_turns, current_run_id,
       cost_micros, tokens_in, tokens_out, created_at, started_at, completed_at, archived_at
FROM tasks
WHERE id = $1 AND org_id = $2;

-- name: ListBoardTasks :many
-- Filters are optional and nullable: NULL means "no constraint", which is why
-- each is written as `sqlc.narg(...) IS NULL OR ...` rather than assembled in
-- Go. The contract (§6.2.16) advertises `status, assignee, search`; a filter
-- that only exists in the client is a filter that silently diverges from the
-- one the API documents.
--
-- `sqlc.narg` rather than a sentinel: an empty string is a legitimate search
-- term for "no match", so it cannot double as "unset".
SELECT id, org_id, board_id, title, body, status, priority, assignee_agent_id, created_by,
       idempotency_key, block_kind, consecutive_failures, workspace_kind, workspace_path,
       branch_name, completion_contract, goal_mode, goal_max_turns, current_run_id,
       cost_micros, tokens_in, tokens_out, created_at, started_at, completed_at, archived_at
FROM tasks
WHERE org_id = $1 AND board_id = $2 AND status != 'archived'
  -- An array, not a scalar: the board's filter chips are multi-select, and a
  -- scalar here would mean the UI silently dropping every status but the first.
  AND (sqlc.narg('status')::text[] IS NULL OR status = ANY(sqlc.narg('status')::text[]))
  AND (sqlc.narg('assignee')::text IS NULL OR assignee_agent_id = sqlc.narg('assignee')::text)
  -- ponytail: ILIKE on title only, no trigram index. Board lists are per-board
  -- (tens of rows), so a sequential match is free here. Add a `pg_trgm` GIN
  -- index the day one board holds thousands of tasks.
  AND (sqlc.narg('search')::text IS NULL OR title ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY priority DESC, created_at DESC;

-- The status transition guard is in the WHERE clause, not in Go: two writers
-- racing on the same task produce one winner and one zero-row result, which the
-- service maps to ErrConflict. That is what makes move/claim safe under load.
-- name: UpdateTaskStatus :one
UPDATE tasks
SET status = $4
WHERE id = $1 AND org_id = $2 AND status = $3
RETURNING id, org_id, board_id, title, body, status, priority, assignee_agent_id, created_by,
          idempotency_key, block_kind, consecutive_failures, workspace_kind, workspace_path,
          branch_name, completion_contract, goal_mode, goal_max_turns, current_run_id,
          cost_micros, tokens_in, tokens_out, created_at, started_at, completed_at, archived_at;

-- name: AssignTask :one
UPDATE tasks
SET assignee_agent_id = $3
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, board_id, title, body, status, priority, assignee_agent_id, created_by,
          idempotency_key, block_kind, consecutive_failures, workspace_kind, workspace_path,
          branch_name, completion_contract, goal_mode, goal_max_turns, current_run_id,
          cost_micros, tokens_in, tokens_out, created_at, started_at, completed_at, archived_at;

-- name: UpdateTaskFields :one
UPDATE tasks
SET title = $3, body = $4, priority = $5
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, board_id, title, body, status, priority, assignee_agent_id, created_by,
          idempotency_key, block_kind, consecutive_failures, workspace_kind, workspace_path,
          branch_name, completion_contract, goal_mode, goal_max_turns, current_run_id,
          cost_micros, tokens_in, tokens_out, created_at, started_at, completed_at, archived_at;

-- name: DeleteTask :exec
DELETE FROM tasks
WHERE id = $1 AND org_id = $2;

-- Dispatcher claim (ARCHITECTURE 4b): one atomic statement. SKIP LOCKED lets
-- concurrent dispatchers claim disjoint batches instead of serialising on the
-- board row, and the status='ready' predicate means a second claimer sees an
-- empty set rather than a duplicate claim.
-- name: ListAssignedTasks :many
-- Screen 28-agent-detail draws the "simulasi penugasan" section: which tasks this
-- agent holds, and the guarantee that archiving it does not cut a running one
-- off (US-AD73 AC1). Archived tasks are excluded (they are gone from the board,
-- like ListBoardTasks), and the order puts live work first so the running row is
-- the one an operator sees without scrolling.
SELECT id, board_id, title, status, cost_micros, created_at
FROM tasks
WHERE org_id = $1 AND assignee_agent_id = $2 AND status != 'archived'
ORDER BY (status = 'running') DESC, created_at DESC;

-- name: CountArchivedAgents :one
-- The assign picker hides archived agents (US-AD73 AC2). The count is what lets
-- the board say "N hidden" instead of silently omitting rows — the design's own
-- `✕ 1 agent diarsip disembunyikan` line.
SELECT count(*) FROM agents
WHERE org_id = $1 AND archived_at IS NOT NULL;

-- name: ListAssignableAgentsForBoard :many
-- The real assign picker for a board: every active agent of the board's own
-- project, ordered the way the picker shows them. It is the source of the
-- preview's "Filter Active Only" claim, so the section proves AC2 against the
-- same rows the task-create modal would offer.
-- `has_provider_key` here is the PROVIDER's key, not the agent's column. DECISIONS
-- 6A.J moved credentials to `providers` ("sekali per ruang kerja, dirujuk banyak
-- agent") and US-AD109 AC6 says the agent keeps no copy, so reading
-- `a.has_provider_key` would mark an agent unready while its provider holds a
-- working key. An agent with no provider row keeps its own column as the answer:
-- nothing else can hold a credential for it. This is the same rule the registry
-- and the agent detail apply (frontend `agentState`).
SELECT a.id,
       a.name,
       (CASE WHEN a.provider_id IS NULL THEN a.has_provider_key
             ELSE COALESCE(p.api_key_enc IS NOT NULL, false) END)::boolean AS has_provider_key
FROM agents a
JOIN boards b ON b.project_id = a.project_id AND b.org_id = a.org_id
LEFT JOIN providers p ON p.id = a.provider_id AND p.org_id = a.org_id
WHERE a.org_id = $1 AND b.id = $2 AND a.archived_at IS NULL
ORDER BY a.name;

-- name: ClaimReadyTasks :many
WITH claimed AS (
    SELECT t.id
    FROM tasks t
    WHERE t.org_id = $1 AND t.board_id = $2
      AND t.status = 'ready'
      AND t.current_run_id IS NULL
    ORDER BY t.priority DESC, t.created_at DESC
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE tasks t
SET status = 'running',
    current_run_id = $4,
    started_at = COALESCE(t.started_at, now())
FROM claimed c
WHERE t.id = c.id
RETURNING t.id, t.org_id, t.board_id, t.title, t.body, t.status, t.priority,
          t.assignee_agent_id, t.created_by, t.idempotency_key, t.block_kind,
          t.consecutive_failures, t.workspace_kind, t.workspace_path, t.branch_name,
          t.completion_contract, t.goal_mode, t.goal_max_turns, t.current_run_id,
          t.cost_micros, t.tokens_in, t.tokens_out, t.created_at, t.started_at,
          t.completed_at, t.archived_at;

-- Dependency DAG. Self-links are rejected by the table CHECK, and the service
-- rejects cycles before insert.
-- name: CreateTaskLink :exec
INSERT INTO task_links (parent_id, child_id)
VALUES ($1, $2)
ON CONFLICT (parent_id, child_id) DO NOTHING;

-- name: DeleteTaskLink :exec
DELETE FROM task_links
WHERE parent_id = $1 AND child_id = $2;

-- name: ListTaskParents :many
SELECT l.parent_id, t.title, t.status
FROM task_links l
JOIN tasks t ON t.id = l.parent_id
WHERE l.child_id = $1
ORDER BY t.created_at;

-- name: ListTaskChildren :many
SELECT l.child_id, t.title, t.status
FROM task_links l
JOIN tasks t ON t.id = l.child_id
WHERE l.parent_id = $1
ORDER BY t.created_at;

-- Counts unfinished parents: the dispatcher promotes a child to ready only when
-- this returns zero (ARCHITECTURE 4e).
-- name: CountUnfinishedParents :one
SELECT count(*)::int
FROM task_links l
JOIN tasks t ON t.id = l.parent_id
WHERE l.child_id = $1 AND t.status NOT IN ('done', 'cancelled', 'archived');

-- Append-only event log. No UPDATE or DELETE is ever issued against events.
-- name: CreateEvent :one
INSERT INTO events (org_id, board_id, task_id, run_id, kind, payload_json)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, org_id, board_id, task_id, run_id, kind, payload_json, created_at;

-- name: ListTaskEvents :many
SELECT id, org_id, board_id, task_id, run_id, kind, payload_json, created_at
FROM events
WHERE task_id = $1
ORDER BY id;

-- SSE resume: events newer than the client's Last-Event-ID for one board.
-- name: ListBoardEventsAfter :many
SELECT id, org_id, board_id, task_id, run_id, kind, payload_json, created_at
FROM events
WHERE board_id = $1 AND org_id = $2 AND id > $3
ORDER BY id
LIMIT $4;

-- ---------------------------------------------------------------- harga manual --
-- Tingkat 1 resolusi harga (DECISIONS 6A.C). Baris di sini MENANG atas tabel
-- katalog exact maupun pattern: itu yang membuat `price_source` bernilai
-- 'manual' di ledger. Nama model disimpan apa adanya, sama seperti yang
-- dicocokkan `pricing.Resolve`.

-- name: UpsertModelPrice :one
INSERT INTO agent_model_prices (
    id, org_id, model, input_micros_per_1m, output_micros_per_1m,
    cached_micros_per_1m, reasoning_micros_per_1m, cache_write_micros_per_1m, created_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (org_id, model) DO UPDATE SET
    input_micros_per_1m       = EXCLUDED.input_micros_per_1m,
    output_micros_per_1m      = EXCLUDED.output_micros_per_1m,
    cached_micros_per_1m      = EXCLUDED.cached_micros_per_1m,
    reasoning_micros_per_1m   = EXCLUDED.reasoning_micros_per_1m,
    cache_write_micros_per_1m = EXCLUDED.cache_write_micros_per_1m,
    updated_at                = now()
RETURNING id, org_id, model, input_micros_per_1m, output_micros_per_1m,
          cached_micros_per_1m, reasoning_micros_per_1m, cache_write_micros_per_1m,
          created_by, created_at, updated_at;

-- name: GetModelPrice :one
SELECT id, org_id, model, input_micros_per_1m, output_micros_per_1m,
       cached_micros_per_1m, reasoning_micros_per_1m, cache_write_micros_per_1m,
       created_by, created_at, updated_at
FROM agent_model_prices
WHERE org_id = $1 AND model = $2;

-- name: ListModelPrices :many
SELECT id, org_id, model, input_micros_per_1m, output_micros_per_1m,
       cached_micros_per_1m, reasoning_micros_per_1m, cache_write_micros_per_1m,
       created_by, created_at, updated_at
FROM agent_model_prices
WHERE org_id = $1
ORDER BY model;

-- name: DeleteModelPrice :execrows
DELETE FROM agent_model_prices WHERE org_id = $1 AND model = $2;
