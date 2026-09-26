-- runs.cancel_requested_at — permintaan batal yang tahan proses.
--
-- Sebelum ini, pembatalan hanya hidup di memori satu proses: dispatcher punya
-- map `cancelled[runID]` yang diisi jalur budget (N17). Itu cukup selama satu
-- binary memegang peran API dan dispatcher sekaligus, dan §66 memang
-- mengizinkan `-role=all`. Tapi begitu perannya dipisah — dan §66 mendaftarkan
-- `-role=api|dispatcher|worker` — POST /tasks/{id}/cancel menulis flag ke map
-- proses API, sementara run-nya jalan di proses dispatcher. Pembatalannya
-- dilaporkan 200 dan tidak pernah terjadi.
--
-- Karena itu sinyalnya ditaruh di baris `runs` yang sedang dibaca dispatcher:
-- satu kolom TIMESTAMPTZ, bukan tabel baru, karena yang dibutuhkan cuma
-- "sejak kapan batal diminta" — dan itu juga yang membuat pembatalan bisa
-- dilaporkan idempoten tanpa state tambahan.
--
-- NULL berarti tidak ada permintaan. Tidak ada DEFAULT, jadi baris lama tetap
-- NULL dan run yang sudah berjalan tidak tiba-tiba terlihat dibatalkan.

ALTER TABLE runs ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ;

-- Index parsial: satu-satunya pembaca yang butuh ini adalah dispatcher yang
-- menanyakan run yang belum selesai. Run yang sudah `ended` tidak pernah
-- ditanyakan lagi, jadi memasukkannya hanya memperbesar index tanpa guna.
CREATE INDEX IF NOT EXISTS runs_cancel_requested_idx
    ON runs (id)
    WHERE cancel_requested_at IS NOT NULL AND status <> 'ended';

-- Perbaikan yang sama dengan 0003/0008/0014: ALL TABLES hanya cocok untuk
-- objek yang sudah ada saat itu, dan role runtime tidak memiliki apa pun
-- (ARCHITECTURE 3.1). Tanpa GRANT ini, migrasinya "berhasil" tapi setiap query
-- baru gagal dengan permission denied.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
