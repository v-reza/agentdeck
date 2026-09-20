-- US-AD86 — the registry needs to know whether an agent carries its own
-- provider credential, without ever reading the credential itself.
--
-- `has_provider_key` is derived, not stored: a generated column cannot drift
-- from the ciphertext it describes, so `ClearAgentProviderKey` (which sets the
-- column to NULL) cannot leave a stale "true" behind. Storing it as a plain
-- BOOLEAN would make that a second write every caller has to remember, and the
-- one that forgets silently mislabels an agent as ready.
--
-- It is generated rather than computed in Go on purpose. The alternative —
-- selecting `provider_api_key_enc` in every agent query so the mapper can test
-- it — would pull sealed ciphertext into every list and get response struct,
-- where it is one careless log line or JSON tag away from leaving the process.
-- A generated column keeps the bytes confined to GetAgentProviderKey, which is
-- the single reader.
--
-- `IS NOT NULL` on BYTEA is immutable, so the column is valid as STORED.
--
-- IF NOT EXISTS keeps a database that already carries the column (from a
-- hand-run hotfix) re-applying cleanly instead of aborting the transaction.

ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS has_provider_key BOOLEAN
    GENERATED ALWAYS AS (provider_api_key_enc IS NOT NULL) STORED;
