# 47-providers — spec isi halaman

Sumber kebenaran isi pane konten. Struktur di bawah WAJIB; Stitch cenderung
mengarang form detail kalau cuma diberi AC.

## Pane konten = DAFTAR provider, bukan form detail

Urutan dari atas ke bawah:

1. **Header pane**: judul `Provider LLM` + tombol utama `Tambah provider`.
2. **Tabel provider.** Header tabel 32px, tiap baris 28px. Kolom kiri→kanan:
   - **Nama** — teks tebal; baris kedua kecil berisi slug.
   - **Protokol** — badge teks, salah satu: `openai_compatible`, `anthropic`, `google`.
   - **Base URL** — monospace, boleh terpotong dengan elipsis.
   - **Kredensial** — HANYA masker `••••••••` + label kecil "terenkripsi".
     JANGAN tampilkan nilai kredensial dalam bentuk apa pun, di mana pun.
   - **Model** — jumlah model hasil tarik, mis. `12 model`.
   - **Verifikasi** — badge teks: `Terverifikasi` (tint aksen) atau `Belum diuji`
     (netral). `Terverifikasi` hanya untuk baris yang uji kredensialnya lolos.
   - **Sinkronisasi model** — teks relatif, mis. `2 jam lalu`. Bila hasil tarik
     terakhir lebih dari 24 jam, tulis `26 jam lalu (kedaluwarsa)` dengan warna
     peringatan.
   - **Default** — badge `Default` pada SATU baris saja di seluruh tabel.
   - **Aksi baris** — dua ikon kecil: `Uji` dan `Tarik model`.
3. **Drawer kanan 420px** — detail provider yang sedang dipilih: nama, protokol,
   base URL, kredensial (tetap termasker), daftar model hasil tarik, dan tombol
   uji kredensial. Drawer ini menampilkan SATU provider, bukan menggantikan tabel.

## Jangan

- Jangan jadikan form detail sebagai isi utama pane konten — tabelnya yang utama.
- Jangan tampilkan nilai kredensial asli di mana pun.
- Jangan tambah kartu ringkasan, statistik armada, panel aturan, atau badge
  kepatuhan yang tidak diminta.
