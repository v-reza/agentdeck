# 27-agent-provider-key — Kredensial provider agent

Route: `(panel)` · Shell: Blueprint C + cost rail (panel 420px)

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

### `US-AD86` — Simpan kredensial provider LLM per agent (Must, M2)

- **AC1** Menyimpan kredensial: nilai API key provider dienkripsi AES-256-GCM sebelum disimpan ke kolom `agents.provider_api_key_enc` (BYTEA). Ciphertext + nonce (12B) + tag (16B) disimpan; plaintext tidak pernah di-log maupun dikembalikan API.

### `US-AD87` — Agent gagal karena kredensial invalid (Must, M4)

- Layar ini hanya menampilkan ringkasan; tidak ada AC berimplikasi visual.


## State yang diminta

Bangun HANYA state `default` sekarang. JANGAN membangun state lain di frame ini.

State lain dikirim sebagai permintaan TERPISAH nanti dengan shell identik, hanya isi pane yang berubah: `masked`, `invalid`.


## Instruksi layar

Buat layar **Kredensial provider agent** pada route `(panel)`. Blueprint C + cost rail (panel 420px). JANGAN membuat state switcher, tab varian, tombol demo, toggle "default/empty/loading/error", atau fungsi `switchState()` dalam bentuk apa pun. Layar ini hanya menampilkan satu state. Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.
