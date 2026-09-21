# COVERAGE — AgentDeck: story ↔ layar ↔ prompt

Dihasilkan otomatis oleh `screens.py`. Jangan diedit tangan.

## Ringkasan

| Item | Jumlah |
|---|---|
| Story | 109 |
| Story dengan layar | 84 |
| Story backend-only (alasan tertulis) | 25 |
| Layar | 52 |
| Frame state yang wajib digenerate | 108 |

## A. Story → Layar

| Story | Prioritas | MS | Judul | Layar |
|---|---|---|---|---|
| `US-AD01` | Must | M0 | Registrasi akun baru | `02-register` |
| `US-AD02` | Must | M0 | Login dan logout | `01-login` |
| `US-AD03` | Should | M0 | Manajemen organisasi (CRUD) | `37-workspace-settings` |
| `US-AD04` | Should | M0 | Manajemen anggota & role | `38-members` |
| `US-AD05` | Could | M5 | Session logout paksa (server-side) | *backend-only* |
| `US-AD06` | Must | M5 | API Key (CRUD) | `39-api-keys` |
| `US-AD07` | Must | M0 | Isolasi data antar Org (keamanan inti) | *backend-only* |
| `US-AD08` | Must | M1 | Membuat project dalam Org | `11-project-list` |
| `US-AD09` | Must | M1 | Membuat board kanban | `18-kanban` |
| `US-AD10` | Must | M1 | Mengedit kolom board | `22-column-editor` |
| `US-AD11` | Must | M1 | Membuat task | `18-kanban`, `21-task-create` |
| `US-AD12` | Must | M1 | Drag task antar kolom | `18-kanban` |
| `US-AD13` | Must | M1 | Task detail drawer | `20-task-drawer` |
| `US-AD14` | Must | M1 | Assign task ke agent | `21-task-create` |
| `US-AD15` | Must | M1 | Filter task berdasarkan status dan assignee | `18-kanban` |
| `US-AD16` | Should | M1 | Cari task | `18-kanban` |
| `US-AD17` | Should | M1 | Priority task (urgent, high, medium, low) | `18-kanban` |
| `US-AD18` | Must | M2 | Dependency DAG antar task | *backend-only* |
| `US-AD19` | Should | M2 | Visualisasi dependency di board | `18-kanban`, `24-dependency-view` |
| `US-AD20` | Must | M1 | Agent registry (CRUD) | `25-agent-registry` |
| `US-AD21` | Must | M1 | Dispatcher mengklaim task | *backend-only* |
| `US-AD22` | Must | M1 | Run lifecycle: claim → finish | *backend-only* |
| `US-AD23` | Must | M1 | Heartbeat berkala | *backend-only* |
| `US-AD24` | Must | M1 | Reclaim task stale | *backend-only* |
| `US-AD25` | Must | M2 | Menulis step (trace) dalam run | *backend-only* |
| `US-AD26` | Must | M3 | Melihat timeline step per run | `33-run-timeline` |
| `US-AD27` | Must | M2 | Cost ledger: mencatat pemakaian token per step | `30-ledger-explorer` |
| `US-AD28` | Must | M2 | Agregasi biaya harian per board | `29-cost-overview` |
| `US-AD29` | Must | M2 | Budget guardrail: hard stop per run ($2) | `29-cost-overview` |
| `US-AD30` | Must | M2 | Budget harian board ($20) | `29-cost-overview` |
| `US-AD31` | Must | M2 | Alert budget 80% | `29-cost-overview` |
| `US-AD32` | Must | M2 | Melihat total biaya task di card | `18-kanban` |
| `US-AD33` | Must | M3 | Approval gate: request approval | *backend-only* |
| `US-AD34` | Must | M3 | Approval: approve | `36-approval-detail` |
| `US-AD35` | Must | M3 | Approval: reject | `36-approval-detail` |
| `US-AD36` | Must | M3 | Approval: expired otomatis | *backend-only* |
| `US-AD37` | Must | M3 | Melihat antrean approval | `35-approval-inbox` |
| `US-AD38` | Must | M3 | Approval gate mode per agent | `35-approval-inbox` |
| `US-AD39` | Must | M3 | Event stream SSE: perubahan status task | `18-kanban` |
| `US-AD40` | Must | M3 | Timeline event per task | `20-task-drawer` |
| `US-AD41` | Must | M3 | Halaman detail run | `32-run-detail` |
| `US-AD42` | Must | M3 | Menambahkan komentar ke task | `20-task-drawer` |
| `US-AD43` | Must | M4 | Failue taxonomy: klasifikasi otomatis | *backend-only* |
| `US-AD44` | Must | M4 | Retry otomatis berdasarkan failure_kind | *backend-only* |
| `US-AD45` | Must | M4 | Max attempts dan dead letter | *backend-only* |
| `US-AD46` | Must | M4 | Upload artifact ke R2 | *backend-only* |
| `US-AD47` | Must | M4 | Download artifact | *backend-only* |
| `US-AD48` | Must | M4 | Menampilkan daftar artifact per task | `20-task-drawer` |
| `US-AD49` | Must | M4 | UI dwibahasa EN/ID | *backend-only* |
| `US-AD50` | Must | M4 | Deteksi string keras (hardcoded) di CI | *backend-only* |
| `US-AD51` | Must | M5 | Audit log: mencatat mutasi administratif | *backend-only* |
| `US-AD52` | Should | M5 | Webhook: daftar webhook per board | `40-webhooks` |
| `US-AD53` | Should | M5 | Webhook delivery dan retry | `40-webhooks` |
| `US-AD54` | Should | M5 | Tampilan data table (list view) | `19-table-view` |
| `US-AD55` | Should | M6 | Command palette (Cmd+K) | `14-command-palette` |
| `US-AD56` | Should | M6 | Ekspor laporan biaya CSV | `31-cost-export` |
| `US-AD57` | Could | M6 | Bulk action: pindahkan task | `19-table-view` |
| `US-AD58` | Must | M1 | Menandai task selesai (done) | `21-task-create` |
| `US-AD59` | Must | M1 | Task archived | `18-kanban`, `20-task-drawer` |
| `US-AD60` | Must | M1 | View toggle: tampilan mobile | `46-mobile-board` |
| `US-AD61` | Must | M3 | Notifikasi in-app | `13-notifications` |
| `US-AD62` | Should | M1 | Filter tasks berdasarkan tanggal | `18-kanban` |
| `US-AD63` | Must | M3 | Skeleton loading state | `43-state-loading` |
| `US-AD64` | Must | M1 | Empty state board | `44-state-empty` |
| `US-AD65` | Must | M3 | Error boundary UI | `45-state-error` |
| `US-AD66` | Must | M1 | Max runtime per task (N9) | *backend-only* |
| `US-AD67` | Must | M1 | Menentukan model dan provider per agent | `26-agent-form`, `28-agent-detail` |
| `US-AD68` | Must | M2 | Provider harga dinamis (price_version) | *backend-only* |
| `US-AD69` | Should | M2 | Menampilkan total biaya project | `29-cost-overview` |
| `US-AD70` | Must | M2 | Proteksi siklus dependency | *backend-only* |
| `US-AD71` | Should | M3 | Comment dengan mention | `20-task-drawer` |
| `US-AD72` | Could | M6 | Prepopulate board dari template | `10-board-list` |
| `US-AD73` | Must | M1 | Menonaktifkan (archive) agent | `25-agent-registry`, `28-agent-detail` |
| `US-AD74` | Must | M4 | Error handling: provider LLM down | *backend-only* |
| `US-AD75` | Must | M1 | Workspace management: menentukan workspace_kind | *backend-only* |
| `US-AD76` | Should | M1 | Halaman dashboard | `42-dashboard` |
| `US-AD77` | Must | M0 | Halaman settings org | `37-workspace-settings` |
| `US-AD78` | Could | M6 | Invite link | `38-members` |
| `US-AD79` | Must | M1 | Reassign task ke agent lain | `20-task-drawer` |
| `US-AD80` | Must | M2 | Menghapus task (soft delete) | `18-kanban`, `20-task-drawer` |
| `US-AD81` | Must | M1 | Filter board berdasarkan kolom | `18-kanban` |
| `US-AD82` | Must | M1 | Scroll sinkron antar kolom | `18-kanban` |
| `US-AD83` | Must | M1 | Mengubah nama board | `23-board-settings` |
| `US-AD84` | Must | M1 | Menghapus board | `23-board-settings` |
| `US-AD85` | Must | M3 | Rate limit per endpoint | *backend-only* |
| `US-AD86` | Must | M2 | Simpan kredensial provider LLM per agent | `27-agent-provider-key` |
| `US-AD87` | Must | M4 | Agent gagal karena kredensial invalid | `27-agent-provider-key` |
| `US-AD88` | Must | M0 | Reset password (lupa password) | `03-reset-request`, `04-reset-confirm` |
| `US-AD89` | Must | M0 | Profil akun sendiri | `15-profile` |
| `US-AD90` | Must | M1 | Ganti password dan sesi aktif | `16-security` |
| `US-AD91` | Must | M1 | Daftar board lintas project | `10-board-list`, `11-project-list` |
| `US-AD92` | Must | M0 | Alur pertama kali (first-run onboarding) | `12-onboarding` |
| `US-AD93` | Must | M0 | Konteks ruang kerja aktif | *backend-only* |
| `US-AD94` | Must | M2 | Panel detail step dan payload | `34-step-payload` |
| `US-AD95` | Must | M5 | Penampil audit log | `41-audit-log` |
| `US-AD96` | Must | M2 | Formulir agent (buat dan ubah) | `26-agent-form` |
| `US-AD97` | Must | M2 | Cost rail: panel biaya sisi kanan | `18-kanban` |
| `US-AD98` | Should | M6 | Menutup akun sendiri | `17-close-account` |
| `US-AD99` | Should | M6 | Halaman dokumentasi | `08-changelog`, `09-features` |
| `US-AD100` | Should | M6 | Halaman harga | `05-landing`, `07-pricing` |
| `US-AD101` | Should | M6 | Halaman repositori & rilis | `09b-github` |
| `US-AD102` | Should | M6 | Halaman komunitas & dukungan | `09c-community` |
| `US-AD103` | Should | M6 | Dokumentasi: mulai cepat | `06-docs-quickstart` |
| `US-AD104` | Should | M6 | Dokumentasi: referensi REST API | `06b-docs-api` |
| `US-AD105` | Should | M6 | Dokumentasi: skema telemetri | `06c-docs-telemetry` |
| `US-AD106` | Must | M2 | Provider BYO (bring your own) | `26-agent-form` |
| `US-AD107` | Should | M2 | Skill library per ruang kerja | `26b-agent-skills` |
| `US-AD108` | Must | M2 | Estimasi biaya: label dan sumber harga | `26-agent-form` |
| `US-AD109` | Must | M2 | Provider registry: daftar kredensial sekali pakai | `47-providers` |

## B. Layar → Story

| Layar | Judul | Route | Story | State wajib |
|---|---|---|---|---|
| `01-login` | Login | `/login` | `US-AD02` | default, error |
| `02-register` | Register | `/register` | `US-AD01` | default, error |
| `03-reset-request` | Lupa password — minta tautan | `/reset` | `US-AD88` | default, sent |
| `04-reset-confirm` | Lupa password — password baru | `/reset/:token` | `US-AD88` | default, expired |
| `05-landing` | Landing page | `/` | `US-AD100` | default |
| `06-docs-quickstart` | Dokumentasi — mulai cepat | `/docs/quickstart` | `US-AD103` | default |
| `06b-docs-api` | Dokumentasi — REST API | `/docs/api` | `US-AD104` | default |
| `06c-docs-telemetry` | Dokumentasi — skema telemetri | `/docs/telemetry` | `US-AD105` | default |
| `07-pricing` | Harga | `/pricing` | `US-AD100` | default |
| `08-changelog` | Changelog | `/changelog` | `US-AD99` | default |
| `09-features` | Product / fitur | `/product` | `US-AD99` | default |
| `09b-github` | Repositori & rilis | `/github` | `US-AD101` | default, no-metadata |
| `09c-community` | Komunitas & dukungan | `/community` | `US-AD102` | default, inactive |
| `10-board-list` | Daftar board | `/boards` | `US-AD72`, `US-AD91` | default, empty, loading, from-template |
| `11-project-list` | Daftar project | `/projects` | `US-AD08`, `US-AD91` | default, empty, create-modal |
| `12-onboarding` | First-run onboarding | `/onboarding` | `US-AD92` | default, step2, done |
| `13-notifications` | Notifikasi | `/notifications` | `US-AD61` | default, empty |
| `14-command-palette` | Command palette | `(overlay)` | `US-AD55` | default, no-results |
| `15-profile` | Profil akun | `/settings/profile` | `US-AD89` | default, error |
| `16-security` | Keamanan: password + sesi aktif | `/settings/security` | `US-AD90` | default, error |
| `17-close-account` | Tutup akun | `/settings/close` | `US-AD98` | default, blocked |
| `18-kanban` | Board kanban | `/boards/:id` | `US-AD09`, `US-AD11`, `US-AD12`, `US-AD15`, `US-AD16`, `US-AD17`, `US-AD19`, `US-AD32`, `US-AD39`, `US-AD59`, `US-AD62`, `US-AD80`, `US-AD81`, `US-AD82`, `US-AD97` | default, empty, loading, filtered-empty |
| `19-table-view` | Tabel task | `/boards/:id?view=table` | `US-AD54`, `US-AD57` | default, selected, empty |
| `20-task-drawer` | Detail task (drawer) | `(drawer)` | `US-AD13`, `US-AD40`, `US-AD42`, `US-AD48`, `US-AD59`, `US-AD71`, `US-AD79`, `US-AD80` | default, timeline-tab, artifacts-tab |
| `21-task-create` | Buat task (modal) | `(modal)` | `US-AD11`, `US-AD14`, `US-AD58` | default, error |
| `22-column-editor` | Editor kolom board | `(panel)` | `US-AD10` | default, error |
| `23-board-settings` | Pengaturan board | `/boards/:id/settings` | `US-AD83`, `US-AD84` | default, confirm-delete |
| `24-dependency-view` | Graf dependency | `/boards/:id/graph` | `US-AD19` | default |
| `25-agent-registry` | Registry agent | `/agents` | `US-AD20`, `US-AD73` | default, empty |
| `26-agent-form` | Formulir agent | `/agents/new` | `US-AD96`, `US-AD67`, `US-AD106`, `US-AD108` | default, no-credential, error |
| `26b-agent-skills` | Skill library agent | `(panel)` | `US-AD107` | default, empty, editor |
| `27-agent-provider-key` | Kredensial provider agent | `(panel)` | `US-AD86`, `US-AD87` | default, masked, invalid |
| `28-agent-detail` | Detail agent | `/agents/:id` | `US-AD67`, `US-AD73` | default, archived |
| `29-cost-overview` | Ringkasan biaya | `/cost` | `US-AD28`, `US-AD29`, `US-AD30`, `US-AD31`, `US-AD69` | default, over-budget |
| `30-ledger-explorer` | Ledger biaya | `/cost/ledger` | `US-AD27` | default, empty |
| `31-cost-export` | Ekspor CSV biaya | `(modal)` | `US-AD56` | default, preview |
| `32-run-detail` | Detail run | `/runs/:id` | `US-AD41` | default, running, failed |
| `33-run-timeline` | Timeline step | `/runs/:id/timeline` | `US-AD26` | default, running |
| `34-step-payload` | Detail step & payload | `(panel)` | `US-AD94` | default, truncated, unavailable |
| `35-approval-inbox` | Antrean approval | `/approvals` | `US-AD37`, `US-AD38` | default, empty |
| `36-approval-detail` | Detail approval | `/approvals/:id` | `US-AD34`, `US-AD35` | default, expired, rejected |
| `37-workspace-settings` | Pengaturan ruang kerja | `/settings/workspace` | `US-AD03`, `US-AD77` | default |
| `38-members` | Anggota & role | `/settings/members` | `US-AD04`, `US-AD78` | default, empty |
| `39-api-keys` | API key | `/settings/api-keys` | `US-AD06` | default, created, empty |
| `40-webhooks` | Webhook | `/settings/webhooks` | `US-AD52`, `US-AD53` | default, empty, failing |
| `41-audit-log` | Audit log | `/settings/audit` | `US-AD95` | default, empty, forbidden |
| `42-dashboard` | Dashboard | `/dashboard` | `US-AD76` | default, empty |
| `43-state-loading` | State: loading (skeleton) | `(varian)` | `US-AD63` | default |
| `44-state-empty` | State: empty | `(varian)` | `US-AD64` | default |
| `45-state-error` | State: error | `(varian)` | `US-AD65` | default |
| `46-mobile-board` | Board mobile | `/m/boards/:id` | `US-AD60` | default |
| `47-providers` | Provider LLM | `/settings/providers` | `US-AD109` | default, empty |

## C. Story backend-only (tanpa layar, dengan alasan)

| Story | Alasan tidak punya layar |
|---|---|
| `US-AD05` | pencabutan sesi dari jarak jauh — aksi API; wujud visualnya adalah daftar sesi di layar 16-security |
| `US-AD07` | isolasi data antar tenant — invarian keamanan, diuji lewat test; tidak ada elemen UI |
| `US-AD18` | penyimpanan edge DAG — datanya ditampilkan di layar 24-dependency-view |
| `US-AD21` | dispatcher mengklaim task — loop server, hasilnya terlihat sebagai status running di board |
| `US-AD22` | siklus hidup run claim→finish — mesin status, wujudnya di layar 32-run-detail |
| `US-AD23` | heartbeat berkala — ping server; kegagalannya terlihat sebagai reclaim |
| `US-AD24` | reclaim task stale — pemulihan server; hasilnya terlihat di board |
| `US-AD25` | menulis step saat run — penulisan DB; dibaca di layar 33-run-timeline |
| `US-AD33` | permintaan approval dari agent — endpoint; kartunya di layar 35-approval-inbox |
| `US-AD36` | approval kedaluwarsa otomatis — pekerjaan terjadwal; state terlihat di 36-approval-detail |
| `US-AD43` | klasifikasi failure otomatis — logika server; labelnya tampil di 32-run-detail |
| `US-AD44` | retry otomatis berbasis failure_kind — logika dispatcher |
| `US-AD45` | max attempts & dead letter — kebijakan server; penanda dead-letter tampil di board |
| `US-AD46` | unggah artifact ke object storage — I/O server; daftarnya di 20-task-drawer |
| `US-AD47` | unduh artifact — endpoint streaming; tombolnya di 20-task-drawer |
| `US-AD49` | dwibahasa EN/ID — lintas seluruh layar, bukan layar tersendiri |
| `US-AD50` | deteksi string keras di CI — gerbang build, tidak punya UI |
| `US-AD51` | pencatatan audit — penulisan DB; pembacaannya di 41-audit-log |
| `US-AD66` | max runtime per task — penghentian oleh server |
| `US-AD68` | snapshot harga per version — data referensi; angkanya tampil di 30-ledger-explorer |
| `US-AD70` | proteksi siklus dependency — validasi server; galatnya muncul di 24-dependency-view |
| `US-AD74` | provider LLM down — penanganan galat server; tampil sebagai failed di board |
| `US-AD75` | penentuan workspace_kind — kolom data; terlihat sebagai jenis ruang kerja di 37-workspace-settings |
| `US-AD85` | rate limit per endpoint — middleware; tampil sebagai toast 429 |
| `US-AD93` | konteks ruang kerja aktif — perilaku lintas layar (pemilih ruang kerja di top bar) |
