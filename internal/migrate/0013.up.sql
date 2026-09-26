-- M4 — tiga tabel yang dipakai runtime dan belum pernah dibuat.
--
-- `runs`, `events`, dan `ledger_entries` sudah ada sejak `0008`; yang kurang
-- justru tiga tabel yang membuatnya bisa dibaca kembali:
--
--   steps     -- trace per langkah. Tanpa ini `runs` cuma punya angka total,
--                dan pertanyaan "token ini habis di langkah mana" tidak bisa
--                dijawab dari mana pun. Layar trace run menunggu tabel ini.
--   approvals -- gate keputusan manusia. DDL-nya mengikat `decision` dan
--                `gate_mode` ke enum yang sama dengan DECISIONS §4, jadi
--                approval yang tidak bisa diputuskan tidak mungkin tersimpan.
--   artifacts -- metadata file hasil eksekusi. `sha256` dipatok 64 char karena
--                kolomnya ada untuk diverifikasi, bukan sekadar dicatat; baris
--                yang checksumnya tidak berbentuk digest tidak ada gunanya.
--
-- Ketiganya memakai bentuk yang sudah tertulis di ARCHITECTURE §3.13–3.15 —
-- migrasi ini tidak mengarang skema, cuma menjalankannya.

-- Per-step cost. `payload_json` sengaja JSONB dan boleh NULL: langkah yang
-- belum selesai belum punya request/response, dan NULL membedakannya dari '{}'
-- yang berarti "ada, tapi kosong".
CREATE TABLE IF NOT EXISTS steps (
    id           BIGSERIAL   NOT NULL,
    org_id       TEXT        NOT NULL,
    run_id       TEXT        NOT NULL,
    seq          SMALLINT    NOT NULL,
    kind         TEXT        NOT NULL,
    name         TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'running',
    tokens_in    BIGINT      NOT NULL DEFAULT 0,
    tokens_out   BIGINT      NOT NULL DEFAULT 0,
    cost_micros  BIGINT      NOT NULL DEFAULT 0,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at     TIMESTAMPTZ,
    payload_json JSONB,
    CONSTRAINT steps_pk          PRIMARY KEY (id),
    CONSTRAINT steps_status_chk  CHECK (status IN ('running','succeeded','failed')),
    CONSTRAINT steps_cost_chk    CHECK (cost_micros >= 0),
    CONSTRAINT steps_tokens_chk  CHECK (tokens_in >= 0 AND tokens_out >= 0),
    CONSTRAINT steps_seq_chk     CHECK (seq BETWEEN 1 AND 5000),
    CONSTRAINT steps_run_fk      FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
    -- Satu langkah satu nomor. Tanpa ini dua langkah paralel bisa menulis `seq`
    -- yang sama dan trace-nya jadi ambigu, sementara `ORDER BY seq` tetap
    -- terlihat benar.
    CONSTRAINT steps_run_seq_key UNIQUE (run_id, seq)
);

-- Melayani: timeline Run trace — SELECT ... FROM steps WHERE run_id = $1 ORDER BY seq.
CREATE INDEX IF NOT EXISTS steps_run_seq_idx ON steps (run_id, seq);

-- Gate keputusan manusia. `gate_mode` disalin dari policy saat gate diciptakan,
-- bukan dibaca ulang saat diputuskan: mengubah policy setelah gate dibuat tidak
-- boleh mengubah arti gate yang sudah mengantre.
CREATE TABLE IF NOT EXISTS approvals (
    id           TEXT        NOT NULL,
    org_id       TEXT        NOT NULL,
    task_id      TEXT        NOT NULL,
    run_id       TEXT        NOT NULL,
    requested_by TEXT        NOT NULL,
    decided_by   TEXT,
    decision     TEXT        NOT NULL DEFAULT 'pending',
    gate_mode    TEXT        NOT NULL,
    reason       TEXT,
    preview_json JSONB,
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',
    decided_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT approvals_pk            PRIMARY KEY (id),
    CONSTRAINT approvals_id_ulid_chk   CHECK (char_length(id) = 26),
    CONSTRAINT approvals_decision_chk  CHECK (decision IN ('pending','approved','rejected','expired')),
    CONSTRAINT approvals_gate_mode_chk CHECK (gate_mode IN ('auto','require','deny')),
    CONSTRAINT approvals_org_fk        FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE,
    CONSTRAINT approvals_task_fk       FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT approvals_run_fk        FOREIGN KEY (run_id)  REFERENCES runs(id)  ON DELETE CASCADE
);

-- Melayani: approval inbox — cari approval pending yang belum expired.
CREATE INDEX IF NOT EXISTS approvals_pending_idx ON approvals (org_id, decision, expires_at)
    WHERE decision = 'pending';

-- Melayani: daftar approval per task, terbaru dulu.
CREATE INDEX IF NOT EXISTS approvals_task_idx ON approvals (task_id, created_at DESC);

-- Metadata artifact. Berkasnya sendiri ada di R2; baris ini yang membuat
-- pemiliknya bisa ditemukan dan checksum-nya bisa dibuktikan.
CREATE TABLE IF NOT EXISTS artifacts (
    id           TEXT        NOT NULL,
    org_id       TEXT        NOT NULL,
    task_id      TEXT        NOT NULL,
    run_id       TEXT        NOT NULL,
    filename     TEXT        NOT NULL,
    content_type TEXT        NOT NULL DEFAULT 'application/octet-stream',
    size         INTEGER     NOT NULL,
    storage_key  TEXT        NOT NULL,
    sha256       TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT artifacts_pk          PRIMARY KEY (id),
    CONSTRAINT artifacts_id_ulid_chk CHECK (char_length(id) = 26),
    CONSTRAINT artifacts_size_chk    CHECK (size BETWEEN 1 AND 26214400),
    CONSTRAINT artifacts_sha256_chk  CHECK (char_length(sha256) = 64),
    CONSTRAINT artifacts_task_fk     FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT artifacts_run_fk      FOREIGN KEY (run_id)  REFERENCES runs(id)  ON DELETE CASCADE
);

-- Melayani: daftar artifact per task.
CREATE INDEX IF NOT EXISTS artifacts_task_idx ON artifacts (task_id, created_at DESC);

-- Same repair as 0003/0008: ALL TABLES only matches objects that exist at that
-- moment, and the runtime role owns nothing (ARCHITECTURE 3.1), so the tables
-- above are invisible to the API until they are granted here. Without this the
-- migration "succeeds" and every new query fails with permission denied.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
