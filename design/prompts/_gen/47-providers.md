# 47-providers

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

### `US-AD109` — Provider registry: daftar kredensial sekali pakai (Must, M2)

- **AC1** Provider punya nama, protokol, base URL, kredensial terenkripsi, dan daftar model hasil tarik. Protokol adalah enum: `openai_compatible`, `anthropic`, `google`.
- **AC3** Uji kredensial memakai **panggilan inference minimal** (`max_tokens: 1`), bukan hanya `GET {base_url}/models`. Indikator "terverifikasi" hanya muncul setelah panggilan itu lolos, karena ada provider yang tidak memeriksa autentikasi di endpoint model.
- **AC7** Daftar model disegarkan otomatis bila hasil tarik terakhir lebih dari 24 jam, dan dapat disegarkan manual.
- **AC9** Satu provider default per ruang kerja; form pendaftaran agent memilih provider default bila ada. Bila provider default dihapus, default menjadi kosong — bukan galat.

## Spec halaman (WAJIB diikuti, ini sumber kebenaran isi halaman)

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

## State yang diminta

Bangun HANYA state `default` sekarang. JANGAN membangun state lain di frame ini.

State lain dikirim sebagai permintaan TERPISAH nanti dengan shell identik, hanya isi pane yang berubah: `empty`.


## Instruksi layar

Buat layar **Provider LLM** pada route `/settings/providers`. Blueprint C + cost rail. JANGAN membuat state switcher, tab varian, tombol demo, toggle "default/empty/loading/error", atau fungsi `switchState()` dalam bentuk apa pun. Layar ini hanya menampilkan satu state. Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.
