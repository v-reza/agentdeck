-- M2 — tingkat 1 resolusi harga: override per-model milik org.
--
-- Tanpa tabel ini, `pricing.Resolve` cuma punya tiga tingkat yang bisa dipakai
-- (katalog, pattern, unpriced), dan nama model BYO tidak akan pernah cocok
-- dengan keduanya: pattern yang ada semuanya spesifik vendor (`grok-*`,
-- `minimax-*`, `*-codex-mini`), jadi nol catch-all. Terukur: `my-own-llama-70b`
-- resolve ke `unpriced` dengan in=0 out=0.
--
-- Akibatnya bukan sekadar "belum akurat": baris `ledger_entries` akan mencatat
-- `cost_micros = 0`, dan nol tidak bisa dibedakan dari "gratis". Gate biaya
-- (US-AD32) juga tidak akan pernah menyala untuk agent BYO. Karena itu kolom
-- `price_source` di `ledger_entries` sudah menyediakan nilai `manual` sejak
-- `0008` — yang belum ada cuma tempat menyimpannya.
--
-- Tiga kolom terakhir DEFAULT -1, dan itu disengaja: -1 adalah sentinel
-- `pricing.missing` untuk "field ini tidak ada di sumber, pakai fallback"
-- (cached ?? input, reasoning ?? output, cache_write ?? input). NULL akan
-- berarti "tidak diisi" dan 0 berarti "gratis" — dua arti yang salah di sini.
-- CHECK `>= -1` menjaga sentinel itu tidak pernah bertabrakan dengan harga sah.

CREATE TABLE IF NOT EXISTS agent_model_prices (
    id                        TEXT        NOT NULL,
    org_id                    TEXT        NOT NULL,
    model                     TEXT        NOT NULL,
    input_micros_per_1m       BIGINT      NOT NULL DEFAULT 0,
    output_micros_per_1m      BIGINT      NOT NULL DEFAULT 0,
    cached_micros_per_1m      BIGINT      NOT NULL DEFAULT -1,
    reasoning_micros_per_1m   BIGINT      NOT NULL DEFAULT -1,
    cache_write_micros_per_1m BIGINT      NOT NULL DEFAULT -1,
    created_by                TEXT,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_model_prices_pk          PRIMARY KEY (id),
    CONSTRAINT agent_model_prices_id_ulid_chk CHECK (char_length(id) = 26),
    CONSTRAINT agent_model_prices_model_chk   CHECK (btrim(model) <> ''),
    CONSTRAINT agent_model_prices_in_chk      CHECK (input_micros_per_1m  >= 0),
    CONSTRAINT agent_model_prices_out_chk     CHECK (output_micros_per_1m >= 0),
    CONSTRAINT agent_model_prices_cached_chk  CHECK (cached_micros_per_1m  >= -1),
    CONSTRAINT agent_model_prices_reason_chk  CHECK (reasoning_micros_per_1m >= -1),
    CONSTRAINT agent_model_prices_cw_chk      CHECK (cache_write_micros_per_1m >= -1),
    CONSTRAINT agent_model_prices_org_fk      FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE
);

-- Satu harga per model per ruang kerja. Dua baris untuk nama yang sama akan
-- membuat `Resolve` bergantung pada urutan baris yang dikembalikan Postgres —
-- dan harga yang bergantung pada urutan bukan harga yang bisa diaudit.
CREATE UNIQUE INDEX IF NOT EXISTS agent_model_prices_org_model_key
    ON agent_model_prices (org_id, model);

CREATE INDEX IF NOT EXISTS agent_model_prices_org_idx ON agent_model_prices (org_id);

-- Tabel baru butuh grant eksplisit: runtime role tidak memiliki apa pun
-- (ARCHITECTURE 3.1), dan `GRANT ... ON ALL TABLES` di migrasi sebelumnya hanya
-- berlaku untuk tabel yang sudah ada saat itu.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
