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
-- once: `base_url` and `reasoning_effort` were bound only by UpdateAgent, so a
-- BYO create (US-AD106 AC1) failed agents_base_url_chk as a 500 and a client's
-- reasoning_effort was dropped in silence. internal/store/queries_columns_test.go
-- is the guard that keeps the two lists in step.
INSERT INTO agents (id, org_id, project_id, name, provider, model, reasoning_effort,
                    skills_json, tools_json, max_runtime_seconds, retry_policy, max_attempts,
                    base_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts, base_url,
          archived_at, created_at, has_provider_key;

-- name: GetAgent :one
SELECT id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
       tools_json, max_runtime_seconds, retry_policy, max_attempts, base_url,
       archived_at, created_at, has_provider_key
FROM agents
WHERE id = $1 AND org_id = $2;

-- name: ListAgents :many
-- Returns archived rows too, deliberately. The registry is where a user finds an
-- agent again to unarchive it, so filtering them out here would make archiving
-- irreversible from the UI. Callers that must not offer a retired agent (the
-- assign dropdown, US-AD73 AC2) filter on `archived_at` themselves — see
-- ListAgentsUsingSkill and the task-assign path.
SELECT id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
       tools_json, max_runtime_seconds, retry_policy, max_attempts, base_url,
       archived_at, created_at, has_provider_key
FROM agents
WHERE org_id = $1 AND project_id = $2
ORDER BY name;

-- name: DeleteAgent :exec
DELETE FROM agents WHERE id = $1 AND org_id = $2;

-- name: UpdateAgent :one
-- US-AD96/US-AD106: the edit form replaces every mutable field at once, so this
-- is a full update rather than a partial patch. `provider` moves together with
-- `base_url` because the DB constraint (agents_base_url_chk) requires them to
-- agree: 'openai_compatible' iff base_url IS NOT NULL.
UPDATE agents
SET name = $3, provider = $4, model = $5, reasoning_effort = $6,
    skills_json = $7, tools_json = $8, max_runtime_seconds = $9,
    retry_policy = $10, max_attempts = $11, base_url = $12
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts, base_url,
          archived_at, created_at, has_provider_key;

-- name: ArchiveAgent :one
-- US-AD73: archived agents keep their row (running tasks still resolve their
-- retry/limit source) but disappear from every assign dropdown.
UPDATE agents
SET archived_at = now()
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts, base_url,
          archived_at, created_at, has_provider_key;

-- name: UnarchiveAgent :one
UPDATE agents
SET archived_at = NULL
WHERE id = $1 AND org_id = $2
RETURNING id, org_id, project_id, name, provider, model, reasoning_effort, skills_json,
          tools_json, max_runtime_seconds, retry_policy, max_attempts, base_url,
          archived_at, created_at, has_provider_key;

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
       tools_json, max_runtime_seconds, retry_policy, max_attempts, base_url,
       archived_at, created_at, has_provider_key
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
SELECT id, org_id, board_id, title, body, status, priority, assignee_agent_id, created_by,
       idempotency_key, block_kind, consecutive_failures, workspace_kind, workspace_path,
       branch_name, completion_contract, goal_mode, goal_max_turns, current_run_id,
       cost_micros, tokens_in, tokens_out, created_at, started_at, completed_at, archived_at
FROM tasks
WHERE org_id = $1 AND board_id = $2 AND status != 'archived'
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
