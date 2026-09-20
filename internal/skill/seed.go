package skill

// SystemSkillSlugs is the contract of US-AD107 AC5: exactly these eight slugs,
// in this order, exist in every workspace. A test compares the seeded library
// against this list, so a ninth skill added "helpfully" to systemSkills is a
// failing test rather than a silent scope change.
var SystemSkillSlugs = []string{
	"code_review",
	"e2e_test",
	"debug",
	"refactor",
	"test_write",
	"docs",
	"migration",
	"security_review",
}

// systemSkills are the eight defaults every workspace starts with (US-AD107
// AC5, PLAN-WAVE2 3[8]). They are data, not code: the user may edit the body of
// a system skill (its version rises like any other), they just may not delete
// it, because agents already reference the slug.
//
// The bodies are instructions, not filler: this is the first thing a user reads
// in the library, and an empty default would teach nothing.
var systemSkills = []struct {
	slug string
	name string
	body string
}{
	{
		slug: "code_review",
		name: "Code review",
		body: "# Code review\n\n" +
			"## Kapan dipakai\n" +
			"Ada diff atau pull request yang perlu dinilai sebelum masuk `main`.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Baca diff lengkap dulu, pahami tujuannya — jangan mengomentari baris sebelum tahu maksudnya.\n" +
			"2. Periksa: kebenaran logika, jalur gagal (error handling), batas tenant/izin, dan test yang menyertai perubahan.\n" +
			"3. Laporkan temuan satu per satu: `file:line`, apa yang salah, kenapa berbahaya, usulan perbaikan.\n" +
			"4. Pisahkan blocker dari nit; jangan campur keduanya dalam satu daftar.\n" +
			"5. Kalau tidak ada temuan, katakan eksplisit \"tidak ada temuan\". Jangan mengarang pujian.\n",
	},
	{
		slug: "e2e_test",
		name: "E2E test",
		body: "# E2E test\n\n" +
			"## Kapan dipakai\n" +
			"Perilaku harus dibuktikan dari sisi pengguna: alur login, alur buat board, alur agent jalan.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Tulis langkah sebagai aksi pengguna (buka halaman, isi field, klik tombol), bukan pemanggilan fungsi internal.\n" +
			"2. Pakai selektor yang stabil dan semantik (`role`, label teks). Hindari selektor CSS yang mengikat struktur DOM.\n" +
			"3. Assert hasil yang terlihat pengguna: teks, status HTTP yang tampil, item yang muncul di daftar.\n" +
			"4. Uji juga satu jalur gagal (input tidak valid, izin ditolak) — bukan hanya happy path.\n" +
			"5. Jangan mengunci test pada data yang bisa berubah; buat data sendiri per test.\n",
	},
	{
		slug: "debug",
		name: "Debug",
		body: "# Debug\n\n" +
			"## Kapan dipakai\n" +
			"Ada kegagalan yang belum dijelaskan: test merah, error produksi, hasil yang tidak sesuai harapan.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Tuliskan dulu gejala yang bisa diamati dan langkah pasti untuk mereproduksinya.\n" +
			"2. Cari akar masalah sebelum mengubah kode. Perbaikan tanpa akar masalah akan kembali sebagai bug lain.\n" +
			"3. Periksa hipotesis satu per satu dengan bukti (log, query, test kecil), bukan dengan dugaan.\n" +
			"4. Kalau hipotesis gugur, catat kenapa gugur — itu mempersempit ruang berikutnya.\n" +
			"5. Setelah diperbaiki, tulis test yang gagal sebelum perbaikan dan lulus sesudahnya.\n" +
			"6. Laporkan akar masalah, bukan hanya perbaikannya.\n",
	},
	{
		slug: "refactor",
		name: "Refactor",
		body: "# Refactor\n\n" +
			"## Kapan dipakai\n" +
			"Struktur kode menghambat perubahan berikutnya, sementara perilakunya sudah benar.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Pastikan ada test yang mengunci perilaku sekarang. Tanpa itu, ini bukan refactor — ini perubahan perilaku.\n" +
			"2. Ubah satu hal per langkah, jalankan test di antara langkah.\n" +
			"3. Jangan mencampur refactor dengan perubahan perilaku dalam satu diff.\n" +
			"4. Hapus kode yang jadi tidak terpakai; jangan menyimpannya \"untuk nanti\".\n" +
			"5. Kalau ada penyederhanaan yang sengaja ditunda, tulis alasannya di komentar `ponytail:` beserta kapan harus ditambahkan.\n",
	},
	{
		slug: "test_write",
		name: "Test writing",
		body: "# Test writing\n\n" +
			"## Kapan dipakai\n" +
			"Menambah atau memperbaiki test untuk perilaku yang belum terjaga.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Tulis test yang gagal dulu, dan pastikan gagalnya karena alasan yang benar (bukan salah import atau typo).\n" +
			"2. Satu test = satu alasan gagal. Nama test menyebut perilaku, bukan nama fungsi.\n" +
			"3. Uji jalur gagal, bukan hanya jalur sukses: input tidak valid, izin ditolak, resource tidak ada.\n" +
			"4. Untuk setiap guard baru, buktikan load-bearing lewat mutasi: rusak guard-nya, test harus GAGAL.\n" +
			"5. Jangan menguji implementasi internal; uji kontrak yang dijanjikan (status, response, baris yang tersimpan).\n" +
			"6. Test harus deterministik: tanpa waktu nyata, tanpa urutan map, tanpa data bersama antar test.\n",
	},
	{
		slug: "docs",
		name: "Documentation",
		body: "# Documentation\n\n" +
			"## Kapan dipakai\n" +
			"Kontrak berubah: endpoint, kolom, aturan izin, atau alur kerja.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Perbarui dokumen yang memuat kontraknya (ARCHITECTURE, DECISIONS, PRD) di perubahan yang sama dengan kodenya.\n" +
			"2. Tulis apa yang berubah dan **kenapa**, bukan hanya apa yang baru.\n" +
			"3. Cantumkan kode status dan bentuk payload yang sebenarnya, bukan yang diharapkan.\n" +
			"4. Kalau keputusan masih terbuka, tandai `ASSUMPTION:` atau `BLOCKED-HUMAN` — jangan disamarkan sebagai fakta.\n" +
			"5. Hapus dokumen yang sudah menyesatkan; dokumen usang lebih berbahaya daripada tidak ada dokumen.\n",
	},
	{
		slug: "migration",
		name: "Migration",
		body: "# Migration\n\n" +
			"## Kapan dipakai\n" +
			"Skema database berubah, atau data lama perlu dibawa ke bentuk baru.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Tulis migration sebagai file baru bernomor; jangan mengubah migration yang sudah pernah dijalankan.\n" +
			"2. Buat idempoten (`IF NOT EXISTS`, guard eksplisit) supaya database yang sudah sebagian terapkan tetap konvergen.\n" +
			"3. Urutan penting: tabel yang direferensikan harus ada lebih dulu (FK tidak bisa menunggu).\n" +
			"4. Jangan lupa GRANT untuk role runtime — tanpa itu migration \"sukses\" tapi setiap query gagal `permission denied`.\n" +
			"5. Uji dengan Postgres sungguhan: jalankan dua kali, lalu pastikan jumlah baris dan kolomnya sesuai harapan.\n" +
			"6. Sediakan jalur mundur atau jelaskan kenapa tidak ada.\n",
	},
	{
		slug: "security_review",
		name: "Security review",
		body: "# Security review\n\n" +
			"## Kapan dipakai\n" +
			"Perubahan menyentuh input pengguna, autentikasi, izin, kredensial, atau batas tenant.\n\n" +
			"## Yang harus dilakukan\n" +
			"1. Periksa batas kepercayaan: setiap input dari luar (body, query, header, markdown) harus divalidasi di sisi server.\n" +
			"2. Pastikan setiap query ter-scope tenant (`org_id`), dan resource milik tenant lain menjawab 404, bukan 403.\n" +
			"3. Cek izin per peran: siapa yang boleh menulis, dan buktikan peran di bawahnya mendapat 403.\n" +
			"4. Pastikan rahasia (kunci, token, password) tidak pernah muncul di log, audit, atau response.\n" +
			"5. Waspadai eksekusi konten: HTML mentah dan skema `javascript:` tidak boleh sampai ke renderer tanpa sanitasi.\n" +
			"6. Laporkan temuan dengan dampak dan langkah reproduksinya; sebut yang tidak bisa dibuktikan sebagai belum diverifikasi.\n",
	},
}

// SystemSkills returns the seeded defaults. It is exported so a caller (the
// composition root, or a test) can reason about the baseline without reaching
// into the data.
func SystemSkills() []Skill {
	out := make([]Skill, 0, len(systemSkills))
	for _, s := range systemSkills {
		out = append(out, Skill{Slug: s.slug, Name: s.name, BodyMD: s.body, IsSystem: true})
	}
	return out
}
