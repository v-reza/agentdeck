# 18-kanban

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

### `US-AD09` — Membuat board kanban (Must, M1)

- **AC1** Membuat board mengembalikan 201 + kolom default tersebut. `columns_json` menyimpan urutan dan nama kolom.
- **AC4** (B2C) Pembuatan board tidak memerlukan pemilihan organisasi maupun project baru bila pengguna belum punya project.

### `US-AD11` — Membuat task (Must, M1)

- **AC5** (B2C) Membuat task tidak memerlukan pemilihan assignee; task tanpa agent tetap valid dan berstatus `backlog`.

### `US-AD12` — Drag task antar kolom (Must, M1)

- **AC4** (permission) `member` dapat memindahkan task; `viewer` mendapat 403 dan kartu tidak bergerak.

### `US-AD15` — Filter task berdasarkan status dan assignee (Must, M1)

- **AC2** Kombinasi beberapa filter diterapkan sebagai AND; hasil kosong bukan error (200 + array kosong).
- **AC3** (jalur gagal) Filter dengan nilai enum tidak dikenal mengembalikan 400, bukan hasil kosong.
- **AC5** (permission) Filter hanya mengembalikan task dalam org pengguna; percobaan lintas org diabaikan tanpa membocorkan keberadaan task.

### `US-AD16` — Cari task (Should, M1)

- Layar ini hanya menampilkan ringkasan; tidak ada AC berimplikasi visual.


### `US-AD17` — Priority task (urgent, high, medium, low) (Should, M1)

- Layar ini hanya menampilkan ringkasan; tidak ada AC berimplikasi visual.


### `US-AD19` — Visualisasi dependency di board (Should, M2)

- **AC1** Kartu yang memiliki dependensi menampilkan badge jumlah dependency + garis tipis ke parent.

### `US-AD32` — Melihat total biaya task di card (Must, M2)

- **AC1** Kartu menampilkan badge biaya (format: `$0.04` atau `$1.23`).
- **AC2** Badge berwarna hijau jika < $0.50, kuning $0.50-$1.50, merah > $1.50.
- **AC3** (jalur gagal) Rollup untuk hari tanpa data menghasilkan baris nol, bukan error.
- **AC4** (permission) Total biaya hanya ditampilkan untuk task dalam org pengguna; `viewer` melihat angka, tidak melihat detail per-step.

### `US-AD39` — Event stream SSE: perubahan status task (Must, M3)

- Layar ini hanya menampilkan ringkasan; tidak ada AC berimplikasi visual.


### `US-AD59` — Task archived (Must, M1)

- **AC1** Arsip → task status `archived`, tidak muncul di board default (toggle filter `archived`).

### `US-AD62` — Filter tasks berdasarkan tanggal (Should, M1)

- Layar ini hanya menampilkan ringkasan; tidak ada AC berimplikasi visual.


### `US-AD80` — Menghapus task (soft delete) (Must, M2)

- Layar ini hanya menampilkan ringkasan; tidak ada AC berimplikasi visual.


### `US-AD81` — Filter board berdasarkan kolom (Must, M1)

- **AC1** Klik header kolom → board menampilkan hanya kolom itu; "Show all" mengembalikan tampilan penuh.
- **AC2** Mode kolom tunggal hanya mengubah presentasi; filter dan status task tidak berubah.
- **AC3** (jalur gagal) Kolom tanpa task tetap dapat dipilih dan menampilkan empty state kolom.

### `US-AD82` — Scroll sinkron antar kolom (Must, M1)

- **AC1** Scroll kolom A tidak mempengaruhi posisi scroll kolom B.
- **AC3** (jalur gagal) Kolom dengan tinggi berbeda tetap sinkron tanpa memicu loop scroll.

### `US-AD97` — Cost rail: panel biaya sisi kanan (Must, M2)

- **AC1** Panel kanan lebar tetap 264px, selalu tampil di halaman board, berisi empat blok: TODAY, 7-DAY, TOP SPENDERS, RUNNING NOW.
- **AC2** Blok TODAY menampilkan biaya hari ini dalam format monospace dan meter pagu harian; meter berubah warna pada 80% (`budget-meter-warning`) dan saat lewat pagu (`budget-meter-over`).
- **AC3** Blok 7-DAY menampilkan 7 batang `sparkline-bar` dengan hari yang melewati pagu ditandai warna peringatan.
- **AC4** (jalur gagal) Bila pagu harian board belum diset, meter tidak ditampilkan sama sekali dan blok TODAY tetap menampilkan angka biaya tanpa error.
- **AC5** (permission) Panel hanya menampilkan angka dari ruang kerja pengguna; `viewer` melihat total biaya tetapi tidak rincian per-step.

## State yang diminta

Bangun HANYA state `default` sekarang. JANGAN membangun state lain di frame ini.

State lain dikirim sebagai permintaan TERPISAH nanti dengan shell identik, hanya isi pane yang berubah: `empty`, `loading`, `filtered-empty`.


## Instruksi layar

Buat layar **Board kanban** pada route `/boards/:id`. Blueprint B + cost rail. JANGAN membuat state switcher, tab varian, tombol demo, toggle "default/empty/loading/error", atau fungsi `switchState()` dalam bentuk apa pun. Layar ini hanya menampilkan satu state. Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.
