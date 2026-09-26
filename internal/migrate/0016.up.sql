-- orgs.deleted_at — supaya "tutup akun" bisa lunak, bukan keras.
--
-- US-AD98 AC2 bilang penutupan akun menghapus ruang kerja yang hanya dimiliki
-- pengguna itu, dan AC5 bilang penutupannya lunak 30 hari — data bisa dipulihkan
-- operator sebelum dihapus permanen. Dua AC itu cuma bisa benar bersamaan kalau
-- ruang kerjanya punya penanda hapus; tanpa kolom ini, satu-satunya cara
-- "menghapus" ruang kerja adalah DELETE, dan AC5 jadi bohong.
--
-- `users.deleted_at` sudah ada sejak awal dan sudah disaring di setiap query
-- pengguna (GetUserByID/GetUserByEmail/UpdateUserProfile), jadi menutup akun
-- tidak butuh kolom baru. Sisi organisasi yang belum punya padanannya.

ALTER TABLE orgs ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Setiap pembaca daftar organisasi menyaring `deleted_at IS NULL`, dan itu
-- predikat yang sama di semua query, jadi index parsial ini yang melayaninya.
-- Ruang kerja hidup jauh lebih banyak daripada yang dihapus, jadi index-nya
-- sengaja hanya memuat baris hidup.
CREATE INDEX IF NOT EXISTS orgs_live_idx ON orgs (id) WHERE deleted_at IS NULL;

-- Perbaikan yang sama dengan 0003/0008/0014/0015: ALL TABLES hanya cocok untuk
-- objek yang sudah ada saat itu, dan role runtime tidak memiliki apa pun
-- (ARCHITECTURE 3.1).
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;
