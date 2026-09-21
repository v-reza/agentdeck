# Design inventory — design vs implementasi

Dihitung dari **parse 50 file design** (`design/stitch-output/v2/*.html`) dan
**91 file impl** (`frontend/src/**/*.tsx|ts`), bukan dari ingatan.

Cakupan: **50 layar design** → 36 punya impl, **14 nol impl**.

Cara pakai: lu centang yang salah, gw kerjain. Dokumen ini daftar kerja.

---

## 1. Yang paling gede: icon

| | design | impl |
|---|---|---|
| `<svg>` | **319** | **3** |
| `<rect>` (grafik batang / sparkline) | **119** | **3** |
| `animate-pulse` (skeleton) | **55** | **4** |

Design pakai **satu set konsisten: Lucide**. `<polyline points="20 6 9 17 4 12">`
= check, `<circle cx="12" cy="12" r="3">` = settings. **207 path unik di 48 layar.**
Ini satu dependency, bukan kerjaan gambar manual per tombol.

Impl sekarang: 3 `<svg>` di seluruh `frontend/src` — `ColumnEditor.tsx`,
`Check.tsx`, `AgentDetailForm.tsx`. Itu akar "kerasa beda" yang paling gede.

Layar paling parah (design svg → impl svg):

| Layar | design | impl |
|---|---|---|
| 32-run-detail | 27 | 0 |
| 35-approval-inbox | 23 | 0 |
| 20-task-drawer | 21 | 0 |
| 26-agent-form | 21 | 0 |
| 21-task-create | 20 | 0 |
| 39-api-keys | 20 | 0 |
| 27-agent-provider-key | 19 | 0 |
| 44-state-empty | 19 | 0 |
| 11-project-list | 18 | 0 |
| 41-audit-log | 18 | 0 |
| 10-board-list | 17 | 0 |
| 28-agent-detail | 16 | **1** |
| 40-webhooks | 15 | 0 |
| 25-agent-registry | 13 | 0 |
| 29-cost-overview | 12 | 0 |
| 18-kanban | 11 | 0 |

---

## 2. Warna status: 10 dari 10 BEDA

`DECISIONS.md` §8 bilang palet ini **beku**. `DESIGN.md` setuju. `design/*.html`
memakai nilainya. `frontend/src/index.css` **memakai set lain** — 10 dari 10 beda:

| Status | beku (DESIGN.md + mockup) | impl (`index.css`) |
|---|---|---|
| backlog | `#64748b` | `#5c6b7a` |
| ready | `#2563eb` | `#1d4ed8` |
| running | `#d97706` | `#b45309` |
| awaiting_approval | `#7c3aed` | `#6d28d9` |
| blocked | `#ea580c` | `#c2410c` |
| review | `#0891b2` | `#0e7490` |
| done | `#16a34a` | `#15803d` |
| failed | `#dc2626` | `#b91c1c` |
| cancelled | `#94a3b8` | `#8a9490` |
| archived | `#cbd5e1` | `#c3cac7` |

Semua nilai impl lebih gelap/pekat. Kesan visualnya: badge status di impl
kelihatan "berat", mockup kelihatan ringan.

**Ini paling murah dibetulin dan paling kerasa hasilnya** — 10 baris di
`index.css`. Tapi keputusan lu yang menentukan mana menang: mockup atau
`DECISIONS.md` §8.

---

## 3. Token spacing mati

Enam token dideklarasikan di `index.css` tapi **nol dipakai** di 91 file impl:

| Token | Nilai | Dipakai |
|---|---|---|
| `--spacing-rail` | 44px | 0 file |
| `--spacing-sidebar` | 224px | 0 file |
| `--spacing-topbar` | 52px | 0 file |
| `--spacing-costrail` | 264px | 0 file |
| `--spacing-row` | 32px | 0 file |
| `--spacing-row-dense` | 28px | 0 file |

Impl menulis ukurannya sebagai literal Tailwind (`w-[264px]`, `h-[52px]`,
`h-[32px]`). Token-nya ada tapi bukan sumber kebenaran — nilainya bisa
diam-diam beda dari mockup dan nol yang gagal.

---

## 4. State visual archived: ILANG total

| Elemen | design | impl |
|---|---|---|
| `line-through` | **6** | **0** |
| background `#fafafa` | 1 | 0 |
| `opacity-75` | 2 | 0 |
| border kiri `#94a3b8` (archived) | 1 | 0 |
| border kiri `#d97706` (butuh perhatian) | 1 | 0 |

Design punya **dua** border kiri pembeda: `#94a3b8` (archived) dan `#d97706`
(warning). Dua-duanya nggak ada di impl. Archived cuma dibedakan lewat label pill.

---

## 5. 14 layar NOL implementasi

| Layar | Isi |
|---|---|
| 12-onboarding | alur pertama kali login |
| 13-notifications | pusat notifikasi |
| 14-command-palette | palette perintah |
| 16-security | keamanan akun |
| 17-close-account | tutup akun |
| 24-dependency-view | tampilan dependency task |
| 31-cost-export | ekspor biaya |
| 32-run-detail | detail run |
| 33-run-timeline | timeline run |
| 34-step-payload | payload step |
| 36-approval-detail | detail approval |
| 41-audit-log | audit log |
| 42-dashboard | dashboard utama |
| 46-mobile-board | board mobile |

Sebagian besar nunggu backend (modul `runs`, `steps`, `approvals`, `ledger`,
`audit` **nol endpoint**). Tapi **12-onboarding** dan **14-command-palette**
nggak butuh backend baru — itu bisa dikerjain sekarang.

---

## 6. Per layar (36 yang ada impl)

Format: `design/impl`. `0/0` = dua-duanya nggak punya, bukan masalah.

| Layar | svg | bar | skeleton | rail 264 | drawer 420 | row 28 | hdr 32 |
|---|---|---|---|---|---|---|---|
| 05-landing | 9/0 | 0/0 | 0/0 | 0/0 | 0/0 | 3/0 | 1/0 |
| 06-docs-quickstart | 4/0 | 4/0 | 0/0 | 0/0 | 0/0 | 0/0 | 0/0 |
| 10-board-list | 17/0 | 0/0 | 2/1 | 4/0 | 0/0 | 0/0 | 0/0 |
| 11-project-list | 18/0 | 11/0 | 2/1 | 2/0 | 0/0 | 6/2 | 1/1 |
| 15-profile | 0/0 | 0/0 | 2/0 | 2/1 | 4/2 | 0/0 | 0/0 |
| 18-kanban | 11/0 | 6/0 | 0/0 | 4/0 | 0/0 | 0/0 | 0/0 |
| 19-table-view | 0/0 | 0/0 | 4/0 | 2/0 | 0/0 | 0/0 | 13/0 |
| 20-task-drawer | 21/0 | 9/0 | 0/0 | 3/0 | 4/2 | 0/0 | 0/0 |
| 21-task-create | 20/0 | 5/0 | 1/0 | 3/0 | 0/0 | 0/0 | 0/0 |
| 22-column-editor | 0/1 | 0/3 | 1/0 | 2/0 | 4/3 | 0/0 | 0/0 |
| 23-board-settings | 0/0 | 0/0 | 1/0 | 2/0 | 3/1 | 5/0 | 1/0 |
| 25-agent-registry | 13/0 | 9/0 | 1/0 | 2/0 | 0/0 | 6/1 | 7/2 |
| 26-agent-form | 21/0 | 11/0 | 1/0 | 4/0 | 6/5 | 2/0 | 3/0 |
| 27-agent-provider-key | 19/0 | 0/0 | 2/0 | 2/0 | 3/3 | 5/0 | 1/0 |
| 28-agent-detail | 16/1 | 10/0 | 1/1 | 3/0 | 0/0 | 2/0 | 3/0 |
| 29-cost-overview | 12/0 | 0/0 | 1/0 | 2/0 | 0/0 | 9/0 | 3/0 |
| 30-ledger-explorer | 0/0 | 0/0 | 3/0 | 2/0 | 0/0 | 12/0 | 1/0 |
| 35-approval-inbox | 23/0 | 0/0 | 2/0 | 3/0 | 0/0 | 0/0 | 0/0 |
| 37-workspace-settings | 0/0 | 0/0 | 1/0 | 2/0 | 3/0 | 6/0 | 1/0 |
| 38-members | 0/0 | 0/0 | 1/0 | 2/0 | 3/0 | 3/0 | 1/0 |
| 39-api-keys | 20/0 | 0/0 | 1/0 | 2/0 | 2/0 | 8/0 | 1/0 |
| 40-webhooks | 15/0 | 0/0 | 1/0 | 2/0 | 2/0 | 12/0 | 2/0 |
| 43/44/45 state | 19/0 | 25/0 | 2/0 | 2/0 | 0/3 | 0/0 | 5/0 |

Sisanya (01–04, 07–09c) nol selisih — layar publik & auth memang sudah cocok.

---

## 7. Rincian per elemen: 25-agent-registry

| Elemen design | design | impl | Status |
|---|---|---|---|
| Icon SVG (aksi/cari/filter) | 13 | 0 | **ILANG** |
| Row archived: `line-through` | 1 | 0 | **ILANG** |
| Row archived: background `#fafafa` | 1 | 0 | **ILANG** |
| Row archived: border kiri `#94a3b8` | 1 | 0 | **ILANG** |
| Row archived: `opacity-75` | 2 | 0 | **ILANG** |
| Status pill `ARCHIVED` | 3 | 27 | ADA |
| Grafik batang | 9 | 0 | **ILANG** |
| Tombol: Arsip, Edit, Pulihkan, Set Kredensial | 5 | 4 | ADA (nol icon) |
| Border kiri `#d97706` (butuh perhatian) | 1 | 0 | **ILANG** |

## 8. Rincian per elemen: 26-agent-form

| Elemen design | design | impl | Status |
|---|---|---|---|
| Icon SVG | 21 | 0 (di layar ini) | **ILANG** |
| Tombol "Simpan Draf" | 1 | 1 | **BEDA** — labelnya "Simpan" (`agents.create.submit`) |
| Tombol "Uji kredensial" | 1 | 1 | ADA (`agents.cred.test`) |
| Tombol "Tarik daftar model" | 1 | 1 | ADA (`agents.create.fetchModels`) |
| Label jargon `Org Scoped (US-AD96)` | 1 | — | **jangan dibawa** (jargon) |
| Label jargon `PROVIDER & MODEL (US-AD67)` | 1 | — | **jangan dibawa** (jargon) |
| `line-through` (state nonaktif) | 1 | 0 | **ILANG** |

## 9. Rincian per elemen: 28-agent-detail

| Elemen design | design | impl | Status |
|---|---|---|---|
| Icon SVG | 16 | 1 | **ILANG** |
| Grafik batang | 10 | 0 | **ILANG** |
| Label jargon `internal/pricing verified` | 1 | — | **jangan dibawa** (jargon) |

---

## Aturan yang berlaku buat semua layar

1. **Icon wajib, satu set: Lucide.** 319 svg di design, 3 di impl. Satu
   dependency, bukan gambar manual.
2. **State visual wajib.** Archived = strikethrough + bg abu + border kiri +
   opacity. Bukan cuma label di pill.
3. **Jargon dibuang, elemen dipertahankan.** Mockup pakai `US-AD96` sebagai label
   section — buang labelnya, section-nya tetap ada.
4. **Density ≠ padding seragam.** Design punya row `h-[28px]` dan header
   `h-[32px]`. Jangan samain semua.
5. **Warna ambil dari token beku**, bukan hex baru. 10 dari 10 warna status
   sekarang melenceng.
6. **Nol `?` di kolom impl.** Tanda tanya bikin tabel keliatan selesai padahal
   kosong — persis masalah yang sama dengan gate status endpoint.
