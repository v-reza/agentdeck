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
