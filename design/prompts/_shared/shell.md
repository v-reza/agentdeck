# Shell blueprint AgentDeck v2 — kontrak frame (kirim sekali di awal sesi)

## Perbedaan dari v1 (WAJIB, ini yang bikin identitas AgentDeck)

- Rail kiri **44px**, ikon saja, background `#e8ecea` (v1: 56px putih).
- Sidebar **224px**, background `#f6f7f6` (v1: 236px putih).
- **Cost rail kanan 264px** — pane baru, full height, background `#eef1f0`.
  Ini tanda tangan visual AgentDeck; v1 tidak punya.
- Topbar **52px**, **HANYA di atas pane konten** — tidak membentang sejajar
  rail + sidebar + cost rail.
- Kolom board **268px**, gap 8px. Baris tabel **28px**, header tabel **32px**.

## Blueprint A — Shell + konten (default)

```
┌────┬──────────┬──────────────────────────────────┬──────────────┐
│rail│ sidebar  │ topbar 52px (hanya di sini)      │  cost rail   │
│44px│ 224px    ├──────────────────────────────────┤    264px     │
│    │          │ konten (SATU-SATUNYA pane scroll)│              │
└────┴──────────┴──────────────────────────────────┴──────────────┘
```

## Blueprint B — Board (5 kolom + cost rail)

Sama seperti A, konten berisi 5 kolom x **268px**, gap **8px**. Tiap kolom punya
**top edge 3px** berwarna status, header kolom **30px**, dan **scroll sendiri**.
JANGAN bikin grid seragam — kolom harus bisa scroll independen.

## Blueprint C — Detail (drawer + cost rail)

A + drawer **420px** dari kanan, diposisikan **DI ANTARA** konten dan cost rail
(bukan menutupi cost rail, bukan full-screen).

## Blueprint D — Centered (login/register/modal saja)

Kartu **400px** di tengah layar. **TANPA** rail, sidebar, topbar, dan cost rail.
JANGAN paksakan chrome shell di halaman auth.

## Tabel: halaman → blueprint → pane yang scroll

| Halaman | Blueprint | Yang scroll |
|---|---|---|
| Login, register, reset | D | kartu (kalau perlu) |
| Board kanban | B | tiap kolom sendiri |
| Board table view | A | pane konten |
| Task detail | C | konten + drawer |
| Cost / ledger / audit | A | pane konten |
| Settings | A | pane konten |

## Patokan ukuran (ANGKA, bukan kira-kira)

rail `44px` · sidebar `224px` · cost rail `264px` · topbar `52px` ·
kolom board `268px` · gap kolom `8px` · baris tabel `28px` · header tabel `32px` ·
padding kartu `10px` · header kolom `30px` · avatar agent `20px` ·
avatar user `26px` · budget meter `6px` · drawer `420px` · kartu auth `400px` ·
radius `6/10/14px`.

## Top edge warna per kolom

Kolom board memakai top edge 3px sesuai status: backlog `#5c6b7a`,
ready `#1d4ed8`, running `#b45309`, awaiting-approval `#6d28d9`,
blocked `#c2410c`, review `#0e7490`, done `#15803d`.

## Cost rail — isi tetap (jangan dikarang)

Empat blok, urutan tetap, TIDAK ADA tambahan:

1. **TODAY** — biaya hari ini + meter pagu (meter 6px).
2. **7-DAY** — sparkline batang, 7 batang.
3. **TOP SPENDERS** — tepat 3 baris.
4. **RUNNING NOW** — jumlah run aktif.

JANGAN menambah grafik, donut, tabel, atau metrik lain di cost rail.

## Recover prompt (kirim kalau shell rusak)

> Shell-nya salah. Ulangi: rail kiri 44px `#e8ecea` ikon saja; sidebar 224px
> `#f6f7f6`; cost rail kanan 264px `#eef1f0`; topbar 52px `#ffffff` HANYA di
> atas pane konten; hanya pane konten yang scroll. Hapus elemen lain.
