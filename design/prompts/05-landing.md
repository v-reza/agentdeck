# 05-landing — Landing page

Route: `/` · Shell: Blueprint P (publik, standalone)

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

### `US-AD100` — Halaman harga (Should, M6)

- **AC1** Halaman Pricing dapat diakses tanpa login dan memuat dua paket: gratis (solo) dan berbayar, dengan daftar isi tiap paket.
- **AC3** Harga Pro ditampilkan sebagai `$5` per bulan, flat — tidak boleh diganti placeholder atau nominal lain.

## Spec halaman (WAJIB diikuti, ini sumber kebenaran isi halaman)

## Blok konteks (kirim bareng prompt)

AgentDeck itu **self-hosted orchestration board buat developer yang jalanin
fleet AI agent sendiri**. Bukan SaaS enterprise. Pembelinya: indie hacker,
solo builder, dev yang punya 5-50 agent jalan otonom dan mulai kehilangan
kendali atas **biaya** dan **apa yang sebenarnya agent-nya lakuin**.

Tiga janji produk (semuanya nyata, bukan marketing kosong):
1. **Tiap run ada harganya.** Cost ledger per step, satuan mikro-sen. Bukan
   estimasi token yang diagregat sebulan kemudian.
2. **Tiap aksi berbahaya bisa digate.** Ada state `awaiting_approval` di state
   machine. Agent berhenti, manusia putuskan, baru lanjut.
3. **Murah dijalanin sendiri.** Satu binary Go, cuma butuh Postgres. Tanpa
   Redis, tanpa Kafka, tanpa Kubernetes.

Rasa visual: **terang, tenang, presisi, teknis tapi nggak dingin.** Ini
halaman jualan, jadi boleh lebih lapang dari app-nya — tapi tetap harus
kelihatan seperti produk engineering, bukan template SaaS.

Font: **Inter** (UI/teks) + **JetBrains Mono** (angka, biaya, ID, perintah).

## Token

Palet terang. Pakai nilai ini persis, jangan karang:

- Page `#f6f7f6` (off-white hangat, BUKAN putih tulang dingin) — khusus landing,
  app pakai `#f6f7f6`. Panel/kartu `#ffffff`.
- Teks utama `#0e1110` · sekunder `#4d5553` · tersier `#6b7370` · kuarter `#98a09d`
- Border hairline `rgba(12,26,22,0.07)` · lebih tegas `rgba(12,26,22,0.12)`
- **Aksen tunggal: `#0d7a70`** (teal). Hover `#0a625a`. Tint `#e6f2f0`.
- Status (dipakai HANYA di visual produk, bukan dekorasi):
  running `#b45309` · awaiting `#6d28d9` · done `#15803d` · failed `#b91c1c`
- Radius: 14 (kartu/modal) · 10 (input/drawer) · 6 (tombol) · 4 (badge) · 9999 (pill)
- Shadow: `0 1px 2px rgba(12,26,22,.05), 0 0 0 1px rgba(12,26,22,.06)`

**Larangan warna:** JANGAN gradient (terutama ungu→biru), JANGAN blob/mesh
background, JANGAN glow, JANGAN glassmorphism/backdrop-blur, JANGAN shadow
tebal, JANGAN lebih dari satu warna aksen.

## Struktur halaman (urutan wajib, satu halaman panjang)

Container maks 1120px, konten rata kiri. Padding vertikal antar section 96px.
**JANGAN** bikin tiap section jadi kartu mengambang.

### 1. Nav (tinggi 64px, sticky, background page + border-bottom hairline)
- Kiri: wordmark "AgentDeck" Inter 16 weight 700 + titik teal 6px setelah teks.
- Tengah: 4 link Inter 14 `text-secondary`: "Product", "Pricing", "Docs", "GitHub".

**Tujuan tiap link (WAJIB, JANGAN pakai `href="#"`):**

| Label | href |
|---|---|
| Product | `#product` (section di halaman ini) |
| Pricing | `/pricing` |
| Docs | `/docs/quickstart` |
| GitHub | `/github` |
| Sign in | `/login` |
| Get started / Start free / Download | `/register` |
| Changelog | `/changelog` |
| Quickstart | `/docs/quickstart` |
| REST API | `/docs/api` |
| Telemetry Schema | `/docs/telemetry` |
| Discord | `/community` |
| Releases | `/github` |
| brand wordmark (navbar + footer) | `/` |

Setiap link wajib punya tujuan nyata. `href="#"` DILARANG kecuali `#product`.
- Kanan: link "Sign in" + tombol primary teal "Get started" (radius 6, padding 8px 16px).

### 2. Hero (padding atas 72px, bawah 56px)
- Eyebrow pill kecil: border hairline, radius full, padding 4px 10px, Inter 12
  `text-secondary`: "v0.1 · self-hosted · single binary". Tanpa ikon.
- H1 Inter 56px weight 700, line-height 1.05, letter-spacing -0.03em, max 2 baris:
  **"Your agents run all night. You should know what they cost."**
- Subhead Inter 17px `text-secondary`, max-width 560px, 2-3 baris:
  "AgentDeck is the orchestration board for AI agent fleets. Every run priced
  to the micro-cent, every risky action gated behind a human approval, all in
  one Go binary you host yourself."
- Dua tombol: primary teal "Get started free" + ghost (border hairline,
  background putih) "Read the docs" dengan ikon panah 14px.
- Di bawah tombol, satu baris Inter 13 `text-tertiary`:
  "No credit card · Postgres + one binary · ~80 MB idle RAM"
- **Kanan hero: visual produk.** Tampilkan mini board AgentDeck: 3 kolom
  (Running / Awaiting approval / Done), 2 kartu per kolom. Kartu: judul 13px,
  nama agent mono 11px, biaya mono 12px warna teal. Kolom "Awaiting approval"
  punya satu kartu dengan border kiri 3px `#6d28d9` (status awaiting-approval) dan strip tombol
  "Approve" / "Reject". Kasih frame: border hairline, radius 8, shadow token.
  **JANGAN** miringkan, JANGAN bikin 3D, JANGAN tambah browser chrome palsu.

### 3. Strip masalah (satu baris, background `#eef1f0`, padding 28px 0)
Tiga item sejajar, dipisah border hairline vertikal. Tiap item: angka mono
tebal + label Inter 13 `text-secondary`.
- "$0.00" / "what you actually spent"
- "?" / "what your agent just did"
- "3 a.m." / "when you find out"
JANGAN kasih ikon.

### 4. Tiga pilar fitur (section utama)
Judul section: Inter 32px weight 700 "Built for the parts nobody demos."
Di bawahnya 3 blok bersusun **vertikal** (bukan 3 kartu sejajar). Tiap blok:
kiri teks (max 460px), kanan visual kecil.

1. **"Every run is priced. Per step."**
   Body: "Token in, token out, cache read, cache write — written to a ledger
   row the moment the step finishes. Prices live in a versioned snapshot, so
   an old run still costs what it cost."
   Visual: potongan tabel 3 baris — Step | Tokens | Cost, angka mono, baris
   terakhir di-highlight tint teal.

2. **"Risky actions stop and wait for you."**
   Body: "Gate any tool behind approval. The agent pauses in `awaiting_approval`,
   you get a diff of exactly what it wants to do, and it only continues when
   you say so."
   Visual: panel approval mini — header "agent-backend wants to run", blok
   kode mono 2 baris, dua tombol Approve/Reject.

3. **"Cheap to run, because it does less."**
   Body: "One Go binary, one Postgres. No Redis, no Kafka, no queue service,
   no Kubernetes. Idles under 80 MB and fits in a $6 VPS."
   Visual: baris mono terminal: `./agentdeck --config agentdeck.yaml` lalu
   satu baris output `listening on :8080 · 23 tables · 109 routes`.

### 5. Cara kerjanya (3 langkah, sejajar horizontal)
Judul: "Three commands to a running fleet."
1. "Point it at Postgres" — mono `agentdeck migrate`
2. "Start the binary" — mono `agentdeck serve`
3. "Register an agent" — mono `agentdeck agent add --provider openai`
Tiap langkah: nomor mono teal 12px, judul Inter 15 weight 600, satu baris body
`text-secondary`, blok kode mono 13px dengan background `#eef1f0`, radius 6,
border hairline. JANGAN kasih ikon besar atau lingkaran bernomor.

### 5b. Panel terminal (blok gelap, di section "Cheap to run")

Satu panel gelap berisi output CLI. Ini blok gelap kedua di halaman, dan HARUS
pakai palet v2, bukan warna default Tailwind.

- Panel: background `#0e1110` (token `primary` — HANYA token ini, JANGAN
  `#101014`, JANGAN `#14161a`, JANGAN `#000`), radius 14px, padding 20px,
  border hairline `rgba(255,255,255,0.08)`. JANGAN shadow tebal.
- Titlebar: 3 titik 10px (`rgba(255,255,255,0.18)`), judul mono 12px
  `rgba(255,255,255,0.55)` = `agentdeck — fleet: production-west`.
- Isi mono 13px, 5 baris, warna dari palet (JANGAN warna Tailwind default):
  1. `$ ./agentdeck --config agentdeck.yaml` — `#e8ecea` (text di atas gelap)
  2. `listening on :8080 · 23 tables · 109 routes` — `#8fa8a4` (redup)
  3. `[ok] postgres pool 8/8 · migrations up to date` — `#5eead4` (hijau-teal)
  4. `[warn] budget 88% of $300 — approval gate armed` — `#fcd34d` (kuning)
  5. `▌` kursor blok, `#0d7a70`
- JANGAN syntax-highlight warna-warni (JANGAN biru `#60a5fa`, JANGAN ungu
  `#a78bfa`, JANGAN merah). Hanya 4 warna di atas.

### 6. Baris angka (background `#0e1110` (token `primary`) gelap, padding 40px 0, teks putih)
Empat angka mono besar + label kecil Inter 12 rgba(255,255,255,.6):
- "109" / "API endpoints"
- "21" / "tables, no ORM magic"
- "30 MB" / "binary size cap"
- "80 MB" / "idle RAM"
Ini blok gelap kedua (setelah panel terminal 5b). JANGAN tambah grafik.

### 7. Harga (flat, self-host, tanpa seat)
Judul: "Priced for one developer, not a procurement team."

PENTING: ini B2C. Harga **flat per bulan**, BUKAN per seat. Siapa pun bisa daftar
dan pakai tier gratis tanpa kartu kredit. JANGAN tulis "per seat", JANGAN tulis
"contact sales", JANGAN tulis "custom pricing".
Dua kartu sejajar, lebar sama:

- **Solo** — `$0` / selamanya. Untuk 1 developer.
  - Self-host, unlimited agents
  - Cost ledger + approval gates
  - Community support
  - Tombol ghost "Download"
- **Pro** — `$5` / bulan, flat. (KARTU INI DI-HIGHLIGHT: border
  2px teal, badge kecil "Most popular" di atas)
  - Everything in Solo
  - Unlimited teammates, no per-seat fee
  - Shared boards + role controls
  - Webhook + audit log
  - Priority support
  - Tombol primary teal "Start free trial"

Di bawah kartu, satu baris Inter 13 `text-tertiary`: "Prices in USD. Cancel
anytime. Self-hosted — your data never leaves your infrastructure."

**PENTING:** nominal Pro sudah ditetapkan `$5`/bulan. Tampilkan `$5` apa adanya, jangan mengganti dengan placeholder atau mengarang nominal lain.

### 8. FAQ (4 item, accordion, semua TERTUTUP default)
1. "Do I need Kubernetes?" → "No. One binary and a Postgres connection string."
2. "Where does my agent code run?" → "On your machine. AgentDeck schedules and
   records; it does not execute your code."
3. "Can I gate only some tools?" → "Yes, per agent. Set the gate mode to
   require, auto, or off."
4. "What happens when I hit a budget cap?" → "The run stops with outcome
   budget_exceeded and the task goes back to ready. Nothing is silently dropped."

### 9. CTA penutup (background tint teal `#e6f2f0`, padding 64px, border-top+bottom hairline)
H2 Inter 32 "Ship the fleet. Keep the receipt." + satu tombol primary teal
"Get started free" + baris kecil "Self-hosted · MIT-licensed core".

### 10. Footer (background page, border-top hairline, padding 40px 0)
Kiri: wordmark + satu baris "Orchestration board for AI agent fleets."
Kanan: 3 kolom link kecil Inter 13 — "Product" (Features, Pricing, Changelog),
"Developers" (Docs, API, GitHub), "Company" (About, Contact).
Baris paling bawah: "© 2026 AgentDeck" + "Built with Go and Postgres."

## Larangan global (berlaku semua section)

1. JANGAN gradient, blob, mesh, glow, glassmorphism, backdrop-blur.
2. JANGAN emoji. Nol.
3. JANGAN copy B2B: "Book a demo", "Contact sales", "Request access",
   "Talk to an expert", "Enterprise plan", "Schedule a call". Siapa pun harus
   bisa mulai sendiri dari halaman ini.
4. JANGAN logo perusahaan lain, JANGAN "trusted by", JANGAN testimonial,
   JANGAN foto orang, JANGAN avatar palsu.
4. JANGAN ikon di dalam lingkaran berwarna.
5. JANGAN angka karangan di luar yang ditulis di atas. Tidak ada
   "10,000+ developers" atau "99.9% uptime".
6. JANGAN tiga kartu fitur sejajar dengan ikon di atasnya (itu bentuk paling
   generik di internet). Pilar fitur harus bersusun vertikal.
7. JANGAN toggle bulanan/tahunan kalau tidak diminta.
8. JANGAN section "Integrations" dengan grid logo.
9. JANGAN lebih dari satu warna aksen. Teal saja.
10. JANGAN bikin teks rata tengah di semua section — hero boleh, sisanya
    rata kiri.

## State yang diminta

Bangun HANYA state `default` sekarang. JANGAN membangun state lain di frame ini.


## Instruksi layar

Halaman ini **PUBLIK** -- pengunjung BELUM login. JANGAN gambar rail ikon, sidebar aplikasi, cost rail, avatar pengguna, atau menu akun. Halaman scroll normal dari atas ke bawah.

Buat layar **Landing page** pada route `/`. Blueprint P (publik, standalone). JANGAN membuat state switcher, tab varian, tombol demo, toggle "default/empty/loading/error", atau fungsi `switchState()` dalam bentuk apa pun. Layar ini hanya menampilkan satu state. Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.
