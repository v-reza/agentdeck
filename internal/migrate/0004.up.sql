-- M1 schema: projects, boards, agents, tasks, task_links.
--
-- Extracted verbatim from docs/ARCHITECTURE.md sections 3.5-3.9 so the DDL has
-- exactly one source of truth. Tables arrive in FK order: projects -> boards ->
-- agents -> tasks -> task_links.
--
-- Every statement is guarded with IF NOT EXISTS, matching 0001/0002: re-applying
-- the file is a no-op. The migration runner records versions, but a database
-- whose bookkeeping was rewound (see repair_test.go) must still apply cleanly.
-- The runtime role's privileges come from the default privileges set in 0001
-- and reasserted in 0003, so no GRANT is needed here.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS projects (
    id         TEXT        NOT NULL,
    org_id     TEXT        NOT NULL,
    slug       TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT projects_pk          PRIMARY KEY (id),
    CONSTRAINT projects_id_ulid_chk CHECK (char_length(id) = 26),
    CONSTRAINT projects_slug_chk    CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'),
    CONSTRAINT projects_org_fk      FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE
);

-- Melayani: GET /api/v1/projects (daftar project per org), dan slug unik per org.
CREATE UNIQUE INDEX IF NOT EXISTS projects_org_slug_key ON projects (org_id, slug);

-- Melayani: pencarian project by slug lintas org (tidak dipakai runtime; jaga-jaga untuk audit manual).
CREATE INDEX IF NOT EXISTS projects_org_created_idx ON projects (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS boards (
    id                   TEXT        NOT NULL,
    org_id               TEXT        NOT NULL,
    project_id           TEXT        NOT NULL,
    slug                 TEXT        NOT NULL,
    name                 TEXT        NOT NULL,
    columns_json         JSONB       NOT NULL DEFAULT
        '[{"key":"backlog","name":"Backlog"},{"key":"ready","name":"Ready"},
          {"key":"running","name":"Running"},{"key":"review","name":"Review"},
          {"key":"done","name":"Done"}]'::jsonb,   -- kolom default (DECISIONS §3); kolom = view dari status, bukan status baru
    budget_daily_micros  BIGINT      NOT NULL DEFAULT 20000000,  -- $20/hari (N16); micro-USD, BIGINT
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT boards_pk            PRIMARY KEY (id),
    CONSTRAINT boards_id_ulid_chk   CHECK (char_length(id) = 26),
    CONSTRAINT boards_slug_chk      CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'),
    CONSTRAINT boards_budget_chk    CHECK (budget_daily_micros >= 0),
    CONSTRAINT boards_columns_chk   CHECK (jsonb_typeof(columns_json) = 'array'),
    CONSTRAINT boards_org_fk        FOREIGN KEY (org_id)     REFERENCES orgs(id)     ON DELETE CASCADE,
    CONSTRAINT boards_project_fk    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Melayani: GET /api/v1/projects/{id}/boards dan resolusi slug board di URL.
CREATE UNIQUE INDEX IF NOT EXISTS boards_project_slug_key ON boards (project_id, slug);

-- Melayani: budget guardrail — ambil cap semua board org ini sebelum menghitung pemakaian harian (N16: $20/hari; lihat §9).
CREATE INDEX IF NOT EXISTS boards_org_budget_idx ON boards (org_id, id) WHERE budget_daily_micros > 0;

-- Melayani: daftar board per project untuk UI (urut terbaru dulu).
CREATE INDEX IF NOT EXISTS boards_project_created_idx ON boards (project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agents (
    id                   TEXT        NOT NULL,
    org_id               TEXT        NOT NULL,
    project_id           TEXT        NOT NULL,
    name                 TEXT        NOT NULL,
    provider             TEXT        NOT NULL,   -- mis. 'anthropic', 'openai', 'openrouter', 'local'
    model                TEXT        NOT NULL,   -- string model apa adanya, dipakai sebagai kunci pricing (§9)
    reasoning_effort     TEXT        NOT NULL DEFAULT 'medium',  -- passthrough ke provider; bukan enum kontrak §4 → tidak di-CHECK
    skills_json          JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- daftar skill yang boleh dimuat agent
    tools_json           JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- allowlist tool; tool di luar ini = failure_kind 'capability'
    max_runtime_seconds  INTEGER     NOT NULL DEFAULT 14400,        -- 4 jam (N9)
    retry_policy         TEXT        NOT NULL DEFAULT 'transient_only',
    max_attempts         INTEGER     NOT NULL DEFAULT 3,
    provider_api_key_enc BYTEA,                  -- AES-256-GCM encrypted (nonce 12B + ciphertext + tag 16B); NULL jika pakai env default (§16)
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agents_pk               PRIMARY KEY (id),
    CONSTRAINT agents_id_ulid_chk      CHECK (char_length(id) = 26),
    CONSTRAINT agents_name_chk         CHECK (btrim(name) <> ''),
    CONSTRAINT agents_runtime_chk      CHECK (max_runtime_seconds BETWEEN 1 AND 86400),
    CONSTRAINT agents_retry_policy_chk CHECK (retry_policy IN ('never','transient_only','always')),
    CONSTRAINT agents_max_attempts_chk CHECK (max_attempts BETWEEN 1 AND 10),
    CONSTRAINT agents_skills_chk       CHECK (jsonb_typeof(skills_json) = 'array'),
    CONSTRAINT agents_tools_chk        CHECK (jsonb_typeof(tools_json)  = 'array'),
    CONSTRAINT agents_org_fk           FOREIGN KEY (org_id)     REFERENCES orgs(id)     ON DELETE CASCADE,
    CONSTRAINT agents_project_fk       FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Melayani: GET /api/v1/projects/{id}/agents, dan lookup agent per nama di UI.
CREATE UNIQUE INDEX IF NOT EXISTS agents_project_name_key ON agents (project_id, name);

-- Melayani: dropdown pemilihan assignee pada form task (hanya kolom ringan, tidak menarik tools_json).
CREATE INDEX IF NOT EXISTS agents_org_name_idx ON agents (org_id, name);

CREATE TABLE IF NOT EXISTS tasks (
    id                      TEXT        NOT NULL,
    org_id                  TEXT        NOT NULL,
    board_id                TEXT        NOT NULL,
    title                   TEXT        NOT NULL,
    body                    TEXT        NOT NULL DEFAULT '',
    status                  TEXT        NOT NULL DEFAULT 'backlog',
    priority                SMALLINT    NOT NULL DEFAULT 0,       -- lebih tinggi = lebih prioritas, sema-mata untuk sortir
    assignee_agent_id       TEXT,                                 -- NULL = belum diassign/agent dihapus
    created_by              TEXT        NOT NULL,                 -- user_id yang membuat
    idempotency_key         TEXT,                                 -- opsional; unik global selama 24 jam
    block_kind              TEXT,                                 -- hanya relevan kalau status='blocked'
    consecutive_failures    SMALLINT    NOT NULL DEFAULT 0,
    workspace_kind          TEXT        NOT NULL DEFAULT 'scratch',
    workspace_path          TEXT,                                 -- diisi executor saat Run dimulai
    branch_name             TEXT,                                 -- untuk git worktree kind
    completion_contract     TEXT,                                 -- JSON bebas: panduan untuk agent reviewer
    goal_mode               TEXT        NOT NULL DEFAULT 'auto',  -- 'auto'|'manual'; bukan enum kontrak → tidak di-CHECK
    goal_max_turns          INTEGER     NOT NULL DEFAULT 25,      -- guardrail: maks langkah eksekusi
    current_run_id          TEXT,                                 -- ULID run yang sedang memegang (kalau status='running')
    cost_micros             BIGINT      NOT NULL DEFAULT 0,       -- agregasi all runs task ini, untuk ditampilkan
    tokens_in               BIGINT      NOT NULL DEFAULT 0,
    tokens_out              BIGINT      NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at              TIMESTAMPTZ,                          -- first run.claimed → running
    completed_at            TIMESTAMPTZ,                          -- task masuk terminal (done/failed/cancelled)
    archived_at             TIMESTAMPTZ,                          -- status='archived'
    CONSTRAINT tasks_pk                      PRIMARY KEY (id),
    CONSTRAINT tasks_id_ulid_chk             CHECK (char_length(id) = 26),
    CONSTRAINT tasks_status_chk              CHECK (status IN (
        'backlog','ready','running','awaiting_approval','blocked','review','done','failed','cancelled','archived'
    )),
    CONSTRAINT tasks_block_kind_chk          CHECK (block_kind IN (
        'dependency','needs_input','capability','policy','budget','external'
    )),
    CONSTRAINT tasks_workspace_kind_chk      CHECK (workspace_kind IN ('scratch','dir','worktree','container')),
    CONSTRAINT tasks_goal_mode_chk           CHECK (goal_mode IN ('auto','manual')),
    CONSTRAINT tasks_cost_nonneg_chk         CHECK (cost_micros >= 0),
    CONSTRAINT tasks_tokens_chk              CHECK (tokens_in >= 0 AND tokens_out >= 0),
    CONSTRAINT tasks_priority_chk            CHECK (priority BETWEEN -10 AND 10),
    CONSTRAINT tasks_consecutive_fail_chk    CHECK (consecutive_failures BETWEEN 0 AND 10),
    CONSTRAINT tasks_board_fk                FOREIGN KEY (board_id)      REFERENCES boards(id) ON DELETE CASCADE,
    CONSTRAINT tasks_agent_fk                FOREIGN KEY (assignee_agent_id) REFERENCES agents(id) ON DELETE SET NULL,
    CONSTRAINT tasks_idempotency_uniq        UNIQUE (idempotency_key)    -- opsional; di-set NULL secara eksplisit untuk yang tidak pakai
);

-- Melayani: query kolom board — daftar task yang visible, diurutkan priority + created.
CREATE INDEX IF NOT EXISTS tasks_board_status_idx ON tasks (board_id, status, priority DESC, created_at DESC)
    WHERE status != 'archived';

-- Melayani: dispatcher — claim task status='ready' untuk board yang tidak di-archive (N19: tick 2 s, N20: batch 20 task).
CREATE INDEX IF NOT EXISTS tasks_ready_claim_idx ON tasks (board_id, priority DESC, created_at DESC)
    WHERE status = 'ready' AND current_run_id IS NULL;

-- Melayani: query "blocked by dependency" — cari task yang blocked oleh task tertentu (task_links, §4d).
CREATE INDEX IF NOT EXISTS tasks_blocked_status_idx ON tasks (id) WHERE status = 'blocked';

-- Melayani: budget guardrail — agregasi biaya per task yang pernah running di board tertentu.
CREATE INDEX IF NOT EXISTS tasks_board_cost_idx ON tasks (board_id) WHERE cost_micros > 0;

-- Melayani: search full-text (title + body) dengan pg_trgm untuk fuzzy match.
-- Filter wajib org_id dipasang di query aplikasi; partial index ini hanya untuk akselerasi.
CREATE INDEX IF NOT EXISTS tasks_title_trgm_idx ON tasks USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS tasks_body_trgm_idx ON tasks USING gin (body gin_trgm_ops);

CREATE TABLE IF NOT EXISTS task_links (
    parent_id   TEXT        NOT NULL,
    child_id    TEXT        NOT NULL,
    CONSTRAINT task_links_pk         PRIMARY KEY (parent_id, child_id),
    CONSTRAINT task_links_parent_fk  FOREIGN KEY (parent_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT task_links_child_fk   FOREIGN KEY (child_id)  REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT task_links_no_self    CHECK (parent_id <> child_id)
);

-- Melayani: "apa saja dependensi task ini (ancestors)" — dispatcher evaluasi ready → check child dulu.
CREATE INDEX IF NOT EXISTS task_links_child_idx ON task_links (child_id, parent_id);

-- Melayani: "apa saja yang tergantung task ini (descendants)" — waktu task selesai, bangunkan anak.
CREATE INDEX IF NOT EXISTS task_links_parent_idx ON task_links (parent_id, child_id);

-- ======================================================================
CREATE TABLE IF NOT EXISTS events (
    id          BIGSERIAL   NOT NULL,    -- BIGSERIAL untuk urutan global; cursor pagination
    org_id      TEXT        NOT NULL,
    board_id    TEXT,                     -- opsional; NULL kalau event bukan bagian board (mis. user.created)
    task_id     TEXT,
    run_id      TEXT,
    kind        TEXT        NOT NULL,
    payload_json JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT events_pk            PRIMARY KEY (id),
    CONSTRAINT events_kind_chk      CHECK (kind IN (
        'task.created','task.status_changed','task.assigned',
        'run.claimed','run.heartbeat','run.finished','run.reclaimed',
        'step.started','step.finished','step.failed',
        'approval.requested','approval.decided','approval.expired',
        'artifact.created','ledger.entry','comment.created','budget.threshold_crossed'
    )),
    CONSTRAINT events_payload_chk   CHECK (pg_column_size(payload_json) <= 65536)  -- N21: 64 KB max
);

-- Melayani: timeline task — SELECT ... FROM events WHERE task_id = $1 ORDER BY id.
CREATE INDEX IF NOT EXISTS events_task_idx ON events (task_id, id);

-- Melayani: timeline run — replay (4d).
CREATE INDEX IF NOT EXISTS events_run_idx ON events (run_id, id);

-- Melayani: timeline board (SSE resume) — SELECT ... FROM events WHERE board_id = $1 AND id > $2 ORDER BY id.
CREATE INDEX IF NOT EXISTS events_board_id_idx ON events (board_id, org_id, id);

-- Melayani: retensi hot events (N10) — DELETE ... WHERE created_at < now() - interval '30 days'.
-- Partial: hanya event yang sudah cukup tua yang di-index, supaya index tidak terus membesar.
-- (Evaluasi di v1 murni seq scan DELETE kecil — di-deploy kalau perlu.)
-- CREATE INDEX events_retention_idx ON events (created_at) WHERE created_at < now() - interval '25 days';
