-- US-AD109 fase 6 — `agents.base_url` ditinggalkan, dan constraint yang mengikatnya
-- ikut dibuang.
--
-- Kenapa kolom ini hilang: US-AD106 menaruh endpoint BYO di agent
-- (`agents.base_url` + `agents.provider_api_key_enc`). DECISIONS 6A.J
-- menggantinya sepenuhnya: endpoint dan kredensial pindah ke `providers`, dan
-- agent cuma menunjuk lewat `agents.provider_id`. Fase 5 memindahkan form agent
-- ke dropdown provider, jadi sejak itu nol pembaca tersisa — kolom yang tak
-- dibaca siapa pun adalah tempat sampah yang cepat atau lambat diisi ulang.
--
-- Constraint-nya dibuang lebih dulu, dan urutannya bukan gaya: bunyinya
-- `(provider = 'openai_compatible') = (base_url IS NOT NULL)`, jadi dia
-- mereferensikan kolom yang sedang dibuang. `ALTER TABLE ... DROP COLUMN` akan
-- menolak selama constraint itu masih ada (2BP01: dependent objects).
--
-- `agents.provider` TETAP. Artinya berubah (protokol/dialect, diturunkan dari
-- `providers.protocol`), tapi dia masih dibaca pricing §9 dan masih jadi nilai
-- yang sah untuk agent tanpa provider — agent dengan `provider_id IS NULL`
-- memakai env default deployment (§16).
--
-- Nol FK dan nol index menyentuh `base_url`, jadi nol objek lain ikut turun.
-- `SyncAgentBaseURLForProvider` — jembatan sementara yang menyalin alamat
-- provider ke agent selama kolomnya masih dirender — dihapus dari queries.sql
-- di perubahan yang sama; query itu tak punya tujuan lagi begitu kolomnya hilang.

ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_base_url_chk;
ALTER TABLE agents DROP COLUMN IF EXISTS base_url;

-- Same repair as 0003/0008/0010: a DROP does not change grants, but this file
-- is the last one touching `agents`, and the runtime role owns nothing
-- (ARCHITECTURE 3.1). Re-stating the grant keeps the table's visibility a fact
-- of the migration history rather than of whichever file ran last.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
