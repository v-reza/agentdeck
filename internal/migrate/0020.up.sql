-- Trigger NOTIFY untuk SSE — ARCHITECTURE 7.2 ("via Postgres LISTEN agentdeck_events").
--
-- §7.2 menetapkan bahwa hub SSE menerima event baru lewat `LISTEN agentdeck_events`,
-- dan §7.4 menegaskan polling BUKAN jalur utama. Yang tidak pernah ada adalah
-- apa pun yang melakukan `NOTIFY`: nol trigger di seluruh migrasi. Jadi channel
-- yang di-LISTEN hub tidak punya pengirim. Ini pengirimnya.
--
-- Payload NOTIFY hanya `id`, bukan `payload_json`. Batas payload NOTIFY ~8000
-- byte sementara N21 mengizinkan payload event sampai 64 KB — mengirim isinya
-- lewat NOTIFY berarti event besar gagal terkirim tanpa error yang terlihat.
-- Hub membaca barisnya setelah bangun; satu query, dan ukuran payload tidak
-- lagi jadi masalah.
--
-- pg_notify dipanggil di dalam fungsi trigger (bukan `NOTIFY` langsung) supaya
-- id-nya bisa dikirim sebagai argumen, bukan lewat string yang dirakit.
--
-- IF NOT EXISTS / DROP IF EXISTS mengikuti konvensi sejak 0013: migrasi repo
-- ini dijalankan ulang di atas skema yang sudah terisi oleh
-- TestPostgresRepairMigration*, jadi DDL yang tidak idempoten menggagalkan
-- suite migrasi.
CREATE OR REPLACE FUNCTION agentdeck_events_notify() RETURNS trigger AS $$
BEGIN
    -- Hanya id: penerima membaca barisnya sendiri, sehingga payload 64 KB (N21)
    -- tidak pernah menyentuh batas 8000 byte milik NOTIFY.
    PERFORM pg_notify('agentdeck_events', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS events_notify_trigger ON events;
CREATE TRIGGER events_notify_trigger
    AFTER INSERT ON events
    FOR EACH ROW
    EXECUTE FUNCTION agentdeck_events_notify();
