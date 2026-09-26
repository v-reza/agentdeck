-- comments — diskusi task (ARCHITECTURE 3.16, DECISIONS §6 baris 194).
--
-- Tabelnya didokumentasikan sebagai DDL lengkap di 3.16 sejak awal dan tidak
-- pernah dibuat: nol migrasi yang menyebutnya. Skemanya di sini disalin dari
-- 3.16 apa adanya — nama kolom persis DECISIONS §6, constraint dan index persis
-- yang ditulis ARCHITECTURE. Yang ditambahkan cuma satu: batas panjang body,
-- yang dipakai US-AD42 AC3 ("melebihi batas panjang") tapi tidak pernah
-- ditentukan angkanya di kontrak mana pun. Lihat komentar di bawah.
CREATE TABLE IF NOT EXISTS comments (
    id              BIGSERIAL   NOT NULL,
    org_id          TEXT        NOT NULL,
    task_id         TEXT        NOT NULL,
    author_user_id  TEXT,                     -- NULL kalau komentar dari agent
    author_agent_id TEXT,                     -- NULL kalau dari user
    body            TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT comments_pk         PRIMARY KEY (id),
    CONSTRAINT comments_body_chk   CHECK (btrim(body) <> ''),
    CONSTRAINT comments_author_chk CHECK (
        (author_user_id IS NOT NULL AND author_agent_id IS NULL)
        OR (author_user_id IS NULL AND author_agent_id IS NOT NULL)
    ),
    CONSTRAINT comments_task_fk    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT comments_org_fk     FOREIGN KEY (org_id)  REFERENCES orgs(id)  ON DELETE CASCADE
);

-- `IF NOT EXISTS` mengikuti konvensi 0013 ke atas: Apply mengulang migrasi di atas
-- skema yang sudah terisi (lihat TestPostgresRepairMigrationGrantsPrivilegesOnLegacySchema),
-- jadi pernyataan yang tidak tahan diulang membuat migrasi gagal di jalur repair.

-- Melayani: timeline diskusi per task.
CREATE INDEX IF NOT EXISTS comments_task_idx ON comments (task_id, id);

-- Melayani: notifikasi — ambil komentar terbaru untuk board.
CREATE INDEX IF NOT EXISTS comments_org_created_idx ON comments (org_id, created_at DESC);
