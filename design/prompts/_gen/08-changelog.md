# 08-changelog

## Kontrak halaman PUBLIK AgentDeck (tanpa login)

Halaman ini **BUKAN aplikasi**. Pengunjung BELUM punya akun.

**HARAM ADA:** rail ikon 44px, sidebar aplikasi 224px, cost rail 264px, topbar 52px
di atas pane konten, avatar pengguna, menu akun, "Sign out", org/workspace switcher,
badge notifikasi, panel TODAY / 7-DAY / TOP SPENDERS / RUNNING NOW.

**WAJIB (Blueprint P):**
- top nav **64px** sticky, bg `#f6f7f6`, border-bottom `rgba(12,26,22,0.07)`
  - kiri: wordmark "AgentDeck" (`#0e1110`, 15px, 600) + mark aksen
  - tengah: link Inter 14 `#4d5553` -- Product, Pricing, Docs, GitHub
  - kanan: "Sign in" (teks polos) + "Get started free" (pill `#0d7a70`, teks `#ffffff`, radius 6px)
- konten **max-width 1120px, margin auto** (di tengah), bg halaman `#f6f7f6`
- footer: border-top `rgba(12,26,22,0.07)`, 4 kolom link Inter 13 `#4d5553`, padding 40px 0
- halaman **scroll normal** dari atas ke bawah; `body` JANGAN overflow hidden, JANGAN `h-screen`

**CEK DIRI SEBELUM KIRIM:** apakah layar ini terlihat seperti dashboard aplikasi
yang sudah login (rail kiri + sidebar + panel biaya kanan)? Kalau iya, SALAH. Hapus
semua chrome aplikasi itu dan ganti dengan top nav publik di atas.

**Blueprint P-DOCS (khusus halaman dokumentasi):** konten dua kolom --
nav dokumen **240px** + artikel **max-width 720px**.
- nav dokumen = **daftar link teks biasa**, TANPA background fill, TANPA border kotak, TANPA ikon
- grup pakai label uppercase kecil (`#98a09d`, 11px, 600, letter-spacing lebar)
- item aktif `#0d7a70` 600; item lain `#4d5553` 400
- nav dokumen tinggi **mengikuti isi**, JANGAN setinggi viewport
- blok kode: bg `#eef1f0`, radius 10px, JetBrains Mono 13px, padding 14px

**Token:** aksen tunggal Signal Teal `#0d7a70`, neutral hijau-tint `#f6f7f6`,
tinta `#0e1110`/`#4d5553`/`#6b7370`, radius maksimum 14px. JANGAN gradien dekoratif.
Satu section gelap (`#0e1110`) boleh, hanya kalau spec halaman memintanya.

## Kebutuhan dari user story (WAJIB muncul semua)

Layar ini adalah wujud visual dari acceptance criteria berikut. Setiap butir di bawah harus kelihatan di layar. JANGAN mengarang kebutuhan yang tidak tertulis di sini.

### `US-AD99` — Halaman dokumentasi (Should, M6)

- **AC1** Halaman Docs dapat diakses dari tautan "Docs" di navigasi dan footer, serta tersedia tanpa login.
- **AC3** (jalur gagal) Tautan ke halaman Docs yang belum ditulis menampilkan halaman "sedang disusun" dengan tautan balik, bukan 404 mentah.
- **AC4** Setiap blok perintah dapat disalin dengan satu klik dan ditampilkan dalam font monospace.

## State yang diminta

Bangun HANYA state `default` sekarang. JANGAN membangun state lain di frame ini.


## Instruksi layar

Halaman ini **PUBLIK** -- pengunjung BELUM login. JANGAN gambar rail ikon, sidebar aplikasi, cost rail, avatar pengguna, atau menu akun. Halaman scroll normal dari atas ke bawah.

Buat layar **Changelog** pada route `/changelog`. Blueprint P (publik, standalone). JANGAN membuat state switcher, tab varian, tombol demo, toggle "default/empty/loading/error", atau fungsi `switchState()` dalam bentuk apa pun. Layar ini hanya menampilkan satu state. Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.
