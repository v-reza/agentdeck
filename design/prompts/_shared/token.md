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
