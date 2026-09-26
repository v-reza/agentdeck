-- api_keys — akses programatik (ARCHITECTURE 3.18, 11.1, 2281-2292).
--
-- Ditulis lengkap di 3.18 sejak awal dan tidak pernah dibuat: nol migrasi yang
-- menyebutnya. Skemanya disalin apa adanya — kolom, constraint, dan kedua index
-- persis seperti di kontrak.
--
-- IF NOT EXISTS mengikuti konvensi sejak 0013: migrasi repo ini dijalankan ulang
-- di atas skema yang sudah terisi oleh TestPostgresRepairMigration*, jadi tabel
-- baru yang tidak idempoten menggagalkan suite migrasi.
--
-- Melayani (3.18): autentikasi programatik — `SELECT ... WHERE prefix = $1 AND
-- revoked_at IS NULL`. `prefix` adalah 8 karakter pertama token, dan index unik
-- parsial itulah yang membuat tabrakan prefix hanya berlaku antar-key aktif.
--
-- Melayani: daftar key milik user.
CREATE TABLE IF NOT EXISTS api_keys (
    id           TEXT        NOT NULL,
    org_id       TEXT        NOT NULL,
    user_id      TEXT        NOT NULL,       -- pemilik key; untuk RBAC dan audit
    name         TEXT        NOT NULL,       -- label yang dikenali pemilik
    prefix       TEXT        NOT NULL,       -- 8 karakter pertama `adk_...` prefiks
    token_hash   TEXT        NOT NULL,       -- SHA-256 (bukan bcrypt — key sudah high entropy)
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,               -- NULL = aktif; diisi saat revoke
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT api_keys_pk           PRIMARY KEY (id),
    CONSTRAINT api_keys_id_ulid_chk  CHECK (char_length(id) = 26),
    CONSTRAINT api_keys_prefix_chk   CHECK (char_length(prefix) = 8),
    CONSTRAINT api_keys_hash_chk     CHECK (char_length(token_hash) = 64),
    CONSTRAINT api_keys_name_chk     CHECK (btrim(name) <> ''),
    CONSTRAINT api_keys_user_fk      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT api_keys_org_fk       FOREIGN KEY (org_id)  REFERENCES orgs(id)  ON DELETE CASCADE
);

-- Melayani: autentikasi — SELECT ... WHERE prefix = $1 AND revoked_at IS NULL.
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_prefix_uniq ON api_keys (prefix) WHERE revoked_at IS NULL;

-- Melayani: daftar key untuk user.
CREATE INDEX IF NOT EXISTS api_keys_user_idx ON api_keys (user_id, org_id);
