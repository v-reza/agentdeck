# AgentDeck — Landing page prompt (B2C, light, standalone)

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
   satu baris output `listening on :8080 · 24 tables · 115 routes`.

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
  2. `listening on :8080 · 24 tables · 115 routes` — `#8fa8a4` (redup)
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

### 8. FAQ (8 item, accordion, semua TERTUTUP default)
1. "Do I need Kubernetes?" → "No. One binary and a Postgres connection string."
2. "Where does my agent code run?" → "On your machine. AgentDeck schedules and
   records; it does not execute your code."
3. "Can I gate only some tools?" → "Yes, per agent. Set the gate mode to
   require, auto, or off."
4. "What happens when I hit a budget cap?" → "The run stops with outcome
   budget_exceeded and the task goes back to ready. Nothing is silently dropped."
5. "Which providers and models can I use?" → "AgentDeck is designed to stay
   provider-agnostic. Register the provider-backed worker you already run, then
   track its runs, tool calls, and cost in one board."
6. "Does my data leave my infrastructure?" → "No. AgentDeck is self-hosted by
   design. The target deployment is one Go binary and PostgreSQL, so your board
   data and ledger stay where you run them."
7. "Can my team use one AgentDeck instance?" → "Yes. Pro is $5/month flat with
   unlimited teammates, shared boards, role controls, webhooks, and audit history."
8. "Is v0.1 ready for production?" → "v0.1 is a public preview of the frontend
   and product contract. The Go runtime, auth API, dispatcher, ledger persistence,
   and worker execution are shipping next across the roadmap."

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
