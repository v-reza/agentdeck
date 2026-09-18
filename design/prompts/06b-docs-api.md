# 06b-docs-api — Dokumentasi — REST API

Route: `/docs/api` · Shell: Blueprint P-DOCS (publik, standalone, nav dokumen)

## PROMPT

# Konteks aplikasi — blok 1 (kirim sekali di awal sesi, sebelum prompt halaman)

Aku merancang UI untuk **AgentDeck**, bukan untuk aplikasi generik. Baca ini
sebelum merancang halaman apa pun.

## Apa ini

AgentDeck adalah **orchestration board untuk armada AI agent**, dipakai 4-8 jam
sehari oleh orang yang menjalankan agent-nya sendiri. Setiap run AI dihargai
dalam micro-USD, setiap aksi sensitif digate approval manusia. Saingan langsung
Hermes Kanban + LangSmith + Temporal UI. Menang di: cost ledger per step,
approval gate built-in, budget guardrail, self-host murah (Go binary, <= $10/bln).

## Siapa pemakainya

**Siapa pun bisa daftar sendiri.** Tidak ada sales call, tidak ada kode invite,
tidak ada "contact us for pricing". Satu orang yang punya 5 agent bisa langsung
jalan dalam 5 menit.

- **Solo builder / indie hacker** — pemakai utama. Self-host, nggak mau ngurus
  procurement. Dia owner, admin, dan member sekaligus.
- **Small team (2-10 orang)** — muncul belakangan, setelah produk kepakai.
- **Viewer** — auditor atau klien, read-only.

Konsekuensi desain: **role itu fitur lanjutan, bukan gerbang masuk.** Halaman
awal harus bisa dipakai solo tanpa konsep "organisasi" yang nyangkut. JANGAN
bikin UI yang mewajibkan bikin org/workspace dulu sebelum bisa lihat apa pun.

Mereka membuka board, lihat 50+ task sekaligus, dan langsung baca: status, agent
yang pegang, biaya, token. Bukan pengguna mobile-first; pakai monitor
1440-1920px. Density tinggi = produktivitas.

## Target rasa (vibe)

- **Utility-driven, bukan fancy.** Rasa Linear / Vercel / Stripe Dashboard:
  permukaan tenang, hairline border, satu aksen, monospace untuk angka.
- **Density tinggi.** Baris tabel 28px. Board 5 kolom x 268px. Minimal 8 baris
  kelihatan per layar.
- **Permukaan berkarakter, bukan abu netral.** Neutral produk ini hijau-tint
  (`#f6f7f6`), bukan abu biru. Tinta border juga hijau-tint
  (`rgba(12,26,22,...)`). Ini yang bikin AgentDeck nggak kelihatan seperti
  dashboard SaaS generik.
- **Angka itu warga kelas satu.** Biaya, token, durasi pakai JetBrains Mono
  dengan tabular figures supaya sejajar vertikal di tabel padat.

## Stack (informasi, jangan generate kode)

React 19 + Vite 6 + TypeScript + Tailwind v4 + shadcn/ui. Redux Toolkit + RTK
Query. Yang aku minta dari kamu adalah **satu halaman HTML statis** yang
merepresentasikan desainnya, bukan kode aplikasi.

## Font

Inter untuk UI, JetBrains Mono untuk angka/biaya/token/ID.

# Token — blok 2 (kirim sekali; nilai SUDAH dikunci server-side via design system)

## Warna — permukaan

| Peran | Hex |
|---|---|
| halaman / kanvas | `#f6f7f6` |
| panel / kartu | `#ffffff` |
| hover permukaan | `#eff2f1` |
| sunken (rail) | `#e8ecea` |
| well (cost rail) | `#eef1f0` |

Neutral produk ini **hijau-tint**, bukan abu-biru. JANGAN pakai `#f7f8f9`,
`#f1f3f4`, `#eceeef` — itu neutral Portico, bukan AgentDeck.

### Permukaan mana untuk elemen apa (JANGAN ngarang hex lain)

| Elemen | Permukaan yang BENAR |
|---|---|
| badge / chip / pill netral | `#eef1f0` |
| tab tidak aktif di dalam group tab | `#ffffff`; group tab-nya `#eef1f0` |
| tab aktif | `#ffffff` + border `rgba(12,26,22,0.12)` |
| avatar inisial | `#eef1f0` + teks `#4d5553` |
| baris tabel: hover | `#eff2f1` |
| baris tabel: terpilih | `#e6f2f0` |
| header tabel | `#eef1f0` |
| kotak kode / pre | `#eef1f0` + teks `#0e1110` |
| skeleton shimmer (base) | `#e8ecea` |
| input field | `#ffffff` + border `rgba(12,26,22,0.12)` |
| input field: disabled | `#eef1f0` |

Tidak ada permukaan netral lain. Kalau butuh nuansa lebih terang dari
`#eef1f0`, pakai `#f6f7f6` atau `#ffffff` — JANGAN mengarang abu baru.

## Warna — teks (hijau-hitam, BUKAN biru)

| Peran | Hex |
|---|---|
| utama | `#0e1110` |
| sekunder | `#4d5553` |
| tersier | `#6b7370` |
| kuaterner | `#98a09d` |

## Warna — border (tinta hijau, BUKAN biru)

| Peran | Nilai |
|---|---|
| subtle | `rgba(12,26,22,0.07)` |
| standard | `rgba(12,26,22,0.12)` |
| strong | `rgba(12,26,22,0.20)` |

JANGAN `rgba(15,23,42,...)` — itu tinta biru.

## Warna — aksen & semantik

| Peran | Hex |
|---|---|
| aksen (Signal Teal) | `#0d7a70` |
| aksen hover | `#0a625a` |
| aksen tint | `#e6f2f0` |
| di atas aksen | `#ffffff` |
| success | `#15803d` |
| warning | `#b45309` |
| danger | `#b91c1c` |
| info | `#1d4ed8` |

Aksen TUNGGAL. JANGAN `#0f766e` (teal v1), JANGAN `#6750a4` / `#4f378a`
(Material purple).

## Warna — 10 status task (palet visual utama)

| Status | Hex |
|---|---|
| backlog | `#5c6b7a` |
| ready | `#1d4ed8` |
| running | `#b45309` |
| awaiting-approval | `#6d28d9` |
| blocked | `#c2410c` |
| review | `#0e7490` |
| done | `#15803d` |
| failed | `#b91c1c` |
| cancelled | `#6b7280` |
| archived | `#cbd5e1` |

Sepuluh status, bukan sembilan. Tiap status muncul sebagai badge pill dan
sebagai top-edge 3px di kolom board.

## Warna — metrik

| Peran | Hex |
|---|---|
| token masuk | `#0e7490` |
| token keluar | `#6d28d9` |
| biaya | `#0e1110` |
| biaya lewat pagu | `#b91c1c` |

## Tipografi

| Token | Font | Ukuran | Berat |
|---|---|---|---|
| display-xl | Inter | 36px | 700 |
| heading-section | Inter | 24px | 600 |
| heading-card | Inter | 16px | 600 |
| body | Inter | 14px | 400 |
| body-sm | Inter | 13px | 400 |
| label | Inter | 12px | 600 |
| mono-number | JetBrains Mono | 14px | 500 |
| mono-code | JetBrains Mono | 13px | 400 |
| mono-xs | JetBrains Mono | 11px | 400 |

Angka pakai tabular figures (`font-variant-numeric: tabular-nums`).

## Radius & spacing

Radius: `4px` (xs) · `6px` (sm) · `10px` (md) · `14px` (lg) · `9999px` (pill).
Maksimum 14px — JANGAN lebih bulat dari itu.

Spacing: `4px` · `8px` · `12px` · `16px` · `24px` · `32px`.

## Larangan warna

JANGAN Material purple (`#6750a4`, `#4f378a`), JANGAN neutral abu-biru
(`#f7f8f9`, `#f1f3f4`), JANGAN teal v1 (`#0f766e`), JANGAN tinta v1
(`#101014`, `#14161a`), JANGAN hitam murni (`#000`), JANGAN gradien.

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

# Aturan global AgentDeck — blok larangan (wajib dilekat di setiap prompt)

## Identitas produk

AgentDeck = orchestration board untuk armada AI agent. Bukan CRM, bukan project
management generik, bukan dashboard analytics. Kalau ragu, pilih yang lebih
padat dan lebih teknis.

## Larangan penemuan (anti-slop)

JANGAN mengarang hal yang tidak diminta:

- JANGAN bikin state switcher, tab varian, tombol demo, atau toggle
  "default/empty/loading/error". Satu layar = satu state.
- JANGAN bikin breadcrumb.
- JANGAN bikin workspace/organization switcher.
- JANGAN bikin tombol social login (Google/GitHub/SSO).
- JANGAN bikin kartu statistik dengan angka karangan.
- JANGAN bikin tab nav yang tidak diminta.
- JANGAN isi sidebar dengan menu karangan. Menu yang benar: Boards, Agents,
  Approvals, Cost, Settings.
- JANGAN bikin footer di dalam aplikasi (footer hanya di landing page).

## Larangan visual

- JANGAN gradien (linear maupun radial) untuk dekorasi.
- JANGAN glassmorphism / `backdrop-filter: blur()`.
- JANGAN drop shadow tebal. Kedalaman lewat border hairline, bukan bayangan.
- JANGAN sudut lebih bulat dari 14px (kecuali badge pill 9999px).
- JANGAN emoji sebagai ikon. Pakai SVG inline.
- JANGAN animasi berlebihan. Transisi maksimal 150ms pada warna/border.
- JANGAN ilustrasi atau gambar dekoratif.

## Larangan layout

- JANGAN pakai putih untuk rail, sidebar, atau cost rail.
- JANGAN bikin topbar membentang sejajar rail + sidebar + cost rail.
- JANGAN bikin lebih dari satu pane yang scroll.
- JANGAN bikin konten melebar penuh sampai tepi layar tanpa padding.
- JANGAN tumpuk kartu di dalam kartu lebih dari satu level.

## Bahasa dan angka

- Seluruh teks UI dalam **bahasa Inggris**. Bahasa Indonesia hanya untuk
  komentar prompt, bukan untuk yang tampil di layar.
- Angka biaya pakai format `$0.042` atau micro-USD eksplisit, konsisten.
- Angka pakai JetBrains Mono + tabular figures.
- Harga Pro yang sudah ditetapkan selalu tampil sebagai `$5` per bulan, flat.
- Jangan mengarang nominal lain atau menghidupkan kembali placeholder harga.

## Kebutuhan dari user story (WAJIB muncul semua)

Layar ini adalah wujud visual dari acceptance criteria berikut. Setiap butir di bawah harus kelihatan di layar. JANGAN mengarang kebutuhan yang tidak tertulis di sini.

### `US-AD104` — Dokumentasi: referensi REST API (Should, M6)

- **AC1** Halaman menampilkan daftar endpoint dikelompokkan per sumber daya, dengan metode HTTP, jalur, dan peran minimum.
- **AC2** Setiap endpoint menampilkan contoh body request dan contoh response dalam blok kode monospace.

## State yang diminta

Bangun HANYA state `default` sekarang. JANGAN membangun state lain di frame ini.


## Instruksi layar

Halaman ini **PUBLIK** -- pengunjung BELUM login. JANGAN gambar rail ikon, sidebar aplikasi, cost rail, avatar pengguna, atau menu akun. Halaman scroll normal dari atas ke bawah.

Buat layar **Dokumentasi — REST API** pada route `/docs/api`. Blueprint P-DOCS (publik, standalone, nav dokumen). JANGAN membuat state switcher, tab varian, tombol demo, toggle "default/empty/loading/error", atau fungsi `switchState()` dalam bentuk apa pun. Layar ini hanya menampilkan satu state. Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.
