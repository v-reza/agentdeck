# Shell PUBLIK AgentDeck — kontrak frame (halaman tanpa login)

Halaman ini **BUKAN aplikasi**. Tidak ada chrome aplikasi sama sekali.

## Yang HARAM ada di halaman publik

**Halaman ini BUKAN aplikasi.** Stitch cenderung memasang shell aplikasi secara default
karena sebagian besar layar AgentDeck memakainya. JANGAN ikuti kecenderungan itu.

Blok-blok ini **dilarang keras** muncul:

- rail ikon 44px di sisi kiri
- sidebar aplikasi 224px dengan menu Boards / Agents / Approvals / Cost
- cost rail 264px di sisi kanan
- topbar aplikasi 52px di atas pane konten
- panel metrik TODAY / 7-DAY / TOP SPENDERS / RUNNING NOW
- avatar user, menu profil, tombol Sign out

Semuanya menandakan user sudah login. Halaman publik TIDAK boleh terlihat seperti itu.

**Cek diri sebelum mengirim:** apakah layar terlihat seperti dashboard aplikasi
yang sudah login? Kalau iya, salah. Hapus chrome aplikasinya dan ganti dengan
header publik di bawah ini.

## Yang WAJIB ada

```
┌────────────────────────────────────────────────────────────────────────────┐
│  HEADER PUBLIK (64px)                                                      │
│  AgentDeck          Docs  Pricing  Changelog          Sign in  Get started │
└────────────────────────────────────────────────────────────────────────────┘
│        konten, max-width 1120px, DI TENGAH (margin auto)                    │
```

- Header publik 64px: brand AgentDeck di kiri, link navigasi publik
  (Docs, Pricing, Changelog), tombol `Sign in` dan `Get started` di kanan.
- Konten `max-width: 1120px`, di tengah (`margin: 0 auto`).
- Tinggi baris teks panjang dijaga nyaman dibaca (sekitar 160% line-height).

## Blueprint P — marketing

Landing, pricing, product/features, changelog, github, community.

- Pakai header publik di atas.
- Konten marketing max-width 1120px di tengah.
- Boleh ada hero, kartu fitur, tabel harga, daftar rilis.
- TIDAK boleh ada rail/sidebar/cost rail aplikasi.

## Blueprint P-DOCS — dokumentasi publik

`/docs/quickstart`, `/docs/api`, `/docs/telemetry`.

```
┌────────────────────────────────────────────────────────────────────────────┐
│  HEADER PUBLIK (64px)                                                      │
│  AgentDeck          Docs  Pricing  Changelog          Sign in  Get started │
└────────────────────────────────────────────────────────────────────────────┘
│   konten, max-width 1120px, DI TENGAH (margin auto)                        │
│  ┌────────────┬────────────────────────────────────────────┐               │
│  │ 240px      │ max-width 720px                            │               │
│  │ NAV DOKUMEN│ ARTIKEL                                    │               │
│  │            │                                            │               │
│  │ Mulai Cepat│                                            │               │
│  │ REST API   │                                            │               │
│  │ Telemetri  │                                            │               │
│  └────────────┴────────────────────────────────────────────┘               │
```

- Header publik, sama dengan Blueprint P.
- Konten dibungkus max-width 1120px di tengah, dua kolom:
  - **240px** = navigasi dokumen publik (Mulai Cepat, REST API, Telemetri,
    Referensi). Ini boleh ada dan memang harus ada.
  - **720px** = artikel dokumentasi.
- Artikel: lebar maksimum 720px supaya baris nyaman dibaca.
- Navigasi dokumen **bukan** sidebar aplikasi. Yang membedakan:
  navigasi dokumen hanya berisi link materi dokumentasi, dan TIDAK ada
  Boards, Agents, Approvals, Cost, avatar user, atau Sign out.

## Permukaan warna (publik)

- header publik: `#ffffff`
- halaman/kanvas: `#f6f7f6`
- panel/kartu: `#ffffff`
- aksen tunggal: Signal Teal `#0d7a70`
- tinta: `#0e1110` / `#4d5553` / `#6b7370`
- border `rgba(12,26,22,0.07)` subtle, `rgba(12,26,22,0.12)` standard

**Neutral produk ini hijau-tint, bukan abu-biru.** JANGAN pakai `#f7f8f9`,
`#f1f3f4`, `#eceeef`, atau `#e8eaed`. Lihat tabel komponen di `token.md`.
