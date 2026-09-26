-- notifications — notifikasi in-app (ARCHITECTURE 3.21, DECISIONS §6 baris 200).
--
-- Ditulis lengkap di 3.21 sejak awal dan tidak pernah dibuat: nol migrasi yang
-- menyebutnya. Skemanya disalin apa adanya — nama kolom persis DECISIONS §6,
-- constraint dan index persis ARCHITECTURE.
--
-- `IF NOT EXISTS` mengikuti konvensi 0013 ke atas: Apply mengulang migrasi di atas
-- skema yang sudah terisi di jalur repair
-- (TestPostgresRepairMigrationGrantsPrivilegesOnLegacySchema), jadi pernyataan
-- yang tidak tahan diulang membuat migrasi gagal di jalur itu.
CREATE TABLE IF NOT EXISTS notifications (
    id          TEXT        NOT NULL,
    user_id     TEXT        NOT NULL,
    org_id      TEXT        NOT NULL,
    kind        TEXT        NOT NULL,
    title       TEXT        NOT NULL,
    body        TEXT,
    target_type TEXT,
    target_id   TEXT,
    read_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notifications_pk         PRIMARY KEY (id),
    CONSTRAINT notifications_id_ulid_chk CHECK (char_length(id) = 26),
    CONSTRAINT notifications_kind_chk   CHECK (kind IN ('approval.requested','budget.warning','run.failed','credential.invalid')),
    CONSTRAINT notifications_user_fk    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT notifications_org_fk     FOREIGN KEY (org_id)  REFERENCES orgs(id)  ON DELETE CASCADE
);

-- Melayani: badge belum dibaca + daftar notifikasi (US-AD61).
CREATE INDEX IF NOT EXISTS notifications_unread_idx ON notifications (user_id, created_at DESC) WHERE read_at IS NULL;
