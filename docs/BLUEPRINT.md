# AgentDeck UI Blueprint Contract

**Status: canonical.** Semua screen baru harus menunjuk ke satu blueprint di dokumen ini. Jangan membuat nama blueprint baru tanpa mengubah dokumen ini, generator, dan gate.

## Jawaban singkat

AgentDeck **bukan memakai satu blueprint untuk semua halaman**. Ada:

- **A**: aplikasi biasa/list/detail ringan.
- **B**: aplikasi dengan board/kolom.
- **C**: aplikasi dengan drawer atau side panel.
- **D**: auth sebelum login.
- **P**: halaman publik/marketing tanpa login.
- **P-DOCS**: halaman dokumentasi publik tanpa login, dengan navigasi dokumen sendiri.
- **Sub-shell**: modal/overlay/state/mobile yang muncul di atas shell induk; ini bukan blueprint baru.

Blueprint dipilih berdasarkan **bentuk dan konteks halaman**, bukan berdasarkan preferensi visual.

## Kontrak tiap blueprint

### Blueprint A — App Standard

Untuk halaman kerja biasa: daftar board/project, registry, dashboard, tabel, ledger, inbox, dan detail penuh yang tidak membutuhkan drawer.

Wajib:

- rail kiri **44px**, ikon saja;
- sidebar aplikasi **224px**;
- topbar **52px** di atas pane konten;
- pane konten scroll sendiri;
- cost rail kanan **264px**;
- cost rail berisi hanya `TODAY`, `7-DAY`, `TOP SPENDERS`, `RUNNING NOW`;
- permukaan: rail `#e8ecea`, sidebar/kanvas `#f6f7f6`, cost rail `#eef1f0`, topbar `#ffffff`.

Pengecualian eksplisit: onboarding boleh memakai **A tanpa cost rail**. Labelnya harus tetap `Blueprint A tanpa cost rail`.

### Blueprint B — Board

Untuk tampilan yang memang berupa board atau kumpulan kolom: kanban, dependency graph, timeline, approval detail berbasis kolom.

Blueprint B = seluruh shell A **ditambah**:

- konten berupa kolom **268px**;
- gap antarkolom **8px**;
- tiap kolom dapat scroll sendiri;
- top edge kolom memakai warna status.

Cost rail 264px tetap ada. Jadi label kanoniknya **`Blueprint B + cost rail`**.

### Blueprint C — App + Drawer/Panel

Untuk halaman settings/detail/form yang pekerjaan utamanya berlangsung di panel samping atau drawer.

Blueprint C = seluruh shell A **ditambah**:

- drawer/panel **420px**;
- posisi drawer di antara pane konten dan cost rail;
- drawer/panel tidak boleh menggantikan shell A.

Label kanonik:

- `Blueprint C + cost rail`
- `Blueprint C + cost rail (drawer 420px)`
- `Blueprint C + cost rail (panel 420px)`

Perbedaan label di atas hanya menjelaskan bentuk panel. Ketiganya tetap blueprint C.

### Blueprint D — Auth Centered

Untuk login, register, request reset password, dan confirm reset password.

Wajib:

- kartu centered **400px**;
- halaman standalone;
- tidak ada rail 44px;
- tidak ada sidebar 224px;
- tidak ada cost rail 264px;
- tidak ada shell aplikasi.

### Blueprint P — Public Marketing

Untuk landing, pricing, product/features, changelog, GitHub, dan community.

Halaman ini dapat dibuka dari landing page dan **tidak membutuhkan login**.

Wajib:

- public marketing header/nav sendiri;
- tidak ada rail aplikasi 44px;
- tidak ada sidebar aplikasi 224px;
- tidak ada cost rail 264px;
- tidak ada topbar aplikasi 52px;
- CTA login/register boleh ada, tetapi tidak membuat halaman terlihat sudah login.

### Blueprint P-DOCS — Public Documentation

Untuk `/docs/quickstart`, `/docs/api`, dan `/docs/telemetry`.

Ini **bukan Blueprint A/C**. Dokumentasi harus bisa dibaca langsung dari landing page tanpa login.

Wajib:

- public header dengan brand AgentDeck dan link `Get started`/`Sign in`;
- navigasi dokumen publik sendiri, bukan sidebar aplikasi;
- artikel dokumentasi publik;
- route aktif tetap di bawah `/docs/...`;
- tidak ada rail aplikasi 44px;
- tidak ada sidebar aplikasi 224px;
- tidak ada cost rail 264px;
- tidak ada metrik user seperti `TODAY`, `TOP SPENDERS`, `RUNNING NOW`, avatar akun, atau `Sign out`;
- tidak ada indikasi user sudah authenticated.

Sidebar navigasi dokumen **boleh ada**. Yang dilarang adalah sidebar aplikasi yang berisi Boards, Agents, Approvals, Cost, avatar akun, atau Sign out. Inilah sumber kebingungan sebelumnya: visualnya sama-sama kolom kiri, tetapi fungsi dan shell-nya berbeda.

## Sub-shell, bukan blueprint tambahan

| Nama | Induk | Aturan |
|---|---|---|
| Modal | A/B/C | Modal tampil di atas shell induk; jangan mengganti shell dengan halaman publik. |
| Overlay | B | Command palette atau overlay tetap berada di atas board. |
| State loading/empty/error | Shell induk | Hanya isi pane yang berubah; jangan menambah state switcher di layar. |
| Mobile | X, turunan B | Layout mobile board; bukan desktop A yang diperkecil. |

`sub-shell Blueprint A: modal` dan sejenisnya adalah label implementasi untuk menjelaskan induk. Itu bukan blueprint kelima.

## Daftar layar saat ini

### A

`10-board-list`, `11-project-list`, `12-onboarding`, `13-notifications`, `19-table-view`, `25-agent-registry`, `28-agent-detail`, `29-cost-overview`, `30-ledger-explorer`, `32-run-detail`, `35-approval-inbox`, `42-dashboard`.

### B

`18-kanban`, `24-dependency-view`, `33-run-timeline`, `36-approval-detail`.

### C

`15-profile`, `16-security`, `17-close-account`, `20-task-drawer`, `23-board-settings`, `26-agent-form`, `37-workspace-settings`, `38-members`, `39-api-keys`, `40-webhooks`, `41-audit-log`.

### D

`01-login`, `02-register`, `03-reset-request`, `04-reset-confirm`.

### P

`05-landing`, `07-pricing`, `08-changelog`, `09-features`, `09b-github`, `09c-community`.

### P-DOCS

`06-docs-quickstart`, `06b-docs-api`, `06c-docs-telemetry`.

### Sub-shell / responsive

`14-command-palette`, `21-task-create`, `22-column-editor`, `27-agent-provider-key`, `34-step-payload`, `43-state-loading`, `44-state-empty`, `45-state-error`, `46-mobile-board`.

## Decision tree

1. User belum login atau halaman marketing? **P** atau **D**.
2. Halaman dokumentasi publik? **P-DOCS**, bukan A.
3. Ada board/kolom yang menjadi fokus utama? **B**.
4. Ada drawer/panel 420px sebagai area kerja utama? **C**.
5. Auth card centered? **D**.
6. Selain itu, halaman kerja biasa? **A**.
7. Modal/overlay/state/mobile? Pakai shell induk dan tandai sebagai **sub-shell**, bukan blueprint baru.

## Source of truth teknis

- Metadata screen: `screens.py`
- Prompt generator: `gen_screens.py`
- Prompt shell publik: `stitch_prompts_v2/_shared/shell-publik.md`
- Prompt shell aplikasi: `stitch_prompts_v2/_shared/shell.md`
- Render gate: `verify_render.py`

Gate membaca huruf blueprint dari metadata, bukan mencocokkan kata seperti `panel` atau `drawer`. Dengan begitu normalisasi nama tidak bisa diam-diam melemahkan validasi.
