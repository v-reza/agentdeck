-- webhooks + webhook_deliveries — ARCHITECTURE 3.22, 3.23, 13.
--
-- Kedua tabel ditulis lengkap di 3.22/3.23 sejak awal dan tidak pernah dibuat:
-- nol migrasi yang menyebutnya. Kolom, constraint, dan index disalin apa adanya,
-- kecuali dua hal yang §16 (keputusan user 2026-09-21) mengalahkan §3.22:
--
--  1. `secret` disimpan sebagai BYTEA terenkripsi (`secret_enc`), bukan TEXT
--     polos. §16 mewajibkan webhook secret dienkripsi di level aplikasi dengan
--     AES-256-GCM sebelum masuk DB, dan `agents.provider_api_key_enc` serta
--     `providers.api_key_enc` sudah memakai bentuk itu. §3.22 sendiri menulis
--     "dienkripsi di DB via AES-256-GCM, lihat §16" di komentarnya, lalu
--     mengetik `TEXT` — komentar dan tipe kolomnya bertentangan, dan §16 yang
--     mengikat.
--  2. `url` boleh `http` untuk tiga host loopback. §3.22 menulis
--     `CHECK (url ~ '^https://')`; §16 mengizinkan `localhost`, `127.0.0.1`,
--     dan `host.docker.internal` sebagai string persis, boleh lewat `http`,
--     karena server inference operator sering jalan di mesin mereka sendiri
--     tanpa TLS. Check di bawah menegakkan pengecualian itu PERSIS — `http`
--     hanya untuk ketiga nama itu, bukan `^https?://` yang akan membuka
--     seluruh internet.
--
-- `webhook_deliveries.event_id` sengaja TIDAK diberi FK. §3.23 menulis
-- komentar "FK ke events.id" tapi tidak mendaftarkan constraint-nya, dan itu
-- memang yang benar di sini: `agentdeck_cleanup()` (§3.24) menghapus `events`
-- lebih tua dari 30 hari, jadi FK cascade akan menghapus riwayat pengiriman
-- yang justru dibaca `GET /webhooks/{id}/deliveries`. Riwayat pengiriman
-- hidup lebih lama dari event yang memicunya.
--
-- `IF NOT EXISTS` mengikuti konvensi 0013 ke atas: Apply mengulang migrasi di
-- atas skema yang sudah terisi di jalur repair
-- (TestPostgresRepairMigrationGrantsPrivilegesOnLegacySchema).
CREATE TABLE IF NOT EXISTS webhooks (
    id          TEXT        NOT NULL,
    org_id      TEXT        NOT NULL,
    board_id    TEXT        NOT NULL,
    url         TEXT        NOT NULL,
    secret_enc  BYTEA       NOT NULL,
    events_json JSONB       NOT NULL DEFAULT '[]'::jsonb,
    active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT webhooks_pk              PRIMARY KEY (id),
    CONSTRAINT webhooks_id_ulid_chk     CHECK (char_length(id) = 26),
    CONSTRAINT webhooks_url_chk         CHECK (
        url ~ '^https://'
        OR url ~ '^http://(localhost|127\.0\.0\.1|host\.docker\.internal)([:/]|$)'
    ),
    CONSTRAINT webhooks_events_chk      CHECK (jsonb_typeof(events_json) = 'array'),
    CONSTRAINT webhooks_board_fk        FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
    CONSTRAINT webhooks_org_fk          FOREIGN KEY (org_id)   REFERENCES orgs(id)  ON DELETE CASCADE
);

-- Melayani: pengiriman event — SELECT ... WHERE board_id = $1 AND active = true AND events_json ? $kind.
CREATE INDEX IF NOT EXISTS webhooks_board_active_idx ON webhooks (board_id, active);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id            BIGSERIAL   NOT NULL,
    webhook_id    TEXT        NOT NULL,
    event_id      BIGINT      NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    attempts      SMALLINT    NOT NULL DEFAULT 0,
    response_code SMALLINT,
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT webhook_deliveries_pk          PRIMARY KEY (id),
    CONSTRAINT webhook_deliveries_status_chk  CHECK (status IN ('pending','delivered','failed','dead')),
    CONSTRAINT webhook_deliveries_attempt_chk CHECK (attempts BETWEEN 0 AND 10),
    CONSTRAINT webhook_deliveries_webhook_fk  FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE
);

-- Melayani: retry driver — ambil pending/failed delivery yang belum dead.
CREATE INDEX IF NOT EXISTS webhook_deliveries_retry_idx ON webhook_deliveries (webhook_id, status)
    WHERE status IN ('pending','failed');

-- Melayani: daftar riwayat pengiriman per webhook.
CREATE INDEX IF NOT EXISTS webhook_deliveries_webhook_idx ON webhook_deliveries (webhook_id, id DESC);
