# 03-reset-request

## Kontrak shell AgentDeck v2 (angka dipatok, JANGAN diubah)

**Permukaan (WAJIB, JANGAN pakai putih untuk rail/sidebar/cost rail):**
- rail kiri: background `#e8ecea`
- sidebar: background `#f6f7f6`
- topbar + panel/kartu: `#ffffff`
- cost rail kanan: background `#eef1f0`
- halaman/kanvas: `#f6f7f6`

**Geometri:**
- rail **44px** (ikon saja, tanpa label) + sidebar **224px** + cost rail kanan **264px**
- topbar **52px**, HANYA membentang di atas pane konten - JANGAN sejajar rail+sidebar+costrail
- `body` overflow hidden; hanya pane konten yang scroll

| Blueprint | Dipakai untuk | Layout |
|---|---|---|
| **A** | halaman default | rail 44 + sidebar 224 + topbar 52 + konten (scroll) + cost rail 264 |
| **B** | board | seperti A, konten = 5 kolom x **268px**, gap **8px**, tiap kolom **top edge 3px** warna status, tiap kolom scroll sendiri |
| **C** | detail/drawer | A + drawer **420px** dari kanan, DI ANTARA konten dan cost rail |
| **D** | login/register saja | kartu **400px** tengah, TANPA rail/sidebar/costrail |

Ukuran lain: baris tabel **28px**, header tabel **32px**, padding kartu **10px**,
header kolom **30px**, avatar agent **20px**, avatar user **26px**, budget meter **6px**,
radius **6/10/14px**. Aksen tunggal Signal Teal `#0d7a70`. Tinta `#0e1110`/`#4d5553`/`#6b7370`.
Border `rgba(12,26,22,0.07)` subtle, `rgba(12,26,22,0.12)` standard.

Cost rail (blueprint A/B/C) isi tetap: TODAY (biaya + meter pagu), 7-DAY (sparkline),
TOP SPENDERS (3 baris), RUNNING NOW. JANGAN tambah isi lain.

## Kebutuhan dari user story (WAJIB muncul semua)

Layar ini adalah wujud visual dari acceptance criteria berikut. Setiap butir di bawah harus kelihatan di layar. JANGAN mengarang kebutuhan yang tidak tertulis di sini.

### `US-AD88` — Reset password (lupa password) (Must, M0)

- **AC2** `POST /api/v1/auth/password/reset` dengan token valid + password baru minimum 8 karakter mengembalikan 200, memperbarui `users.password_hash`, dan mencabut seluruh baris `sessions` milik pengguna itu.

## State yang diminta

Bangun HANYA state `default` sekarang. JANGAN membangun state lain di frame ini.

State lain dikirim sebagai permintaan TERPISAH nanti dengan shell identik, hanya isi pane yang berubah: `sent`.


## Instruksi layar

Buat layar **Lupa password — minta tautan** pada route `/reset`. Blueprint D centered. JANGAN membuat state switcher, tab varian, tombol demo, toggle "default/empty/loading/error", atau fungsi `switchState()` dalam bentuk apa pun. Layar ini hanya menampilkan satu state. Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.
