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
