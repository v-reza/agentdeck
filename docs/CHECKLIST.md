# CHECKLIST — AgentDeck M0 → M4

> Dihasilkan oleh `tools/update_checklist.py` dari `docs/COVERAGE.md` + `tools/checklist_status.json`.
> **Jangan edit file ini** — edit sidecar-nya, lalu jalankan ulang tool-nya.
> Status: ✅ PASS · 🔨 dikerjakan · ⬜ belum · ⏸️ ditunda (keputusan user) · ❌ gagal

| Story | Pri | MS | Judul | Layar | Status |
|---|---|---|---|---|---|
| `US-AD01` | Must | M0 | Registrasi akun baru | `02-register` | ✅ PASS |
| `US-AD02` | Must | M0 | Login dan logout | `01-login` | ✅ PASS |
| `US-AD03` | Should | M0 | Manajemen organisasi (CRUD) | `37-workspace-settings` | ✅ PASS |
| `US-AD04` | Should | M0 | Manajemen anggota & role | `38-members` | ✅ PASS |
| `US-AD07` | Must | M0 | Isolasi data antar Org (keamanan inti) | *backend-only* | ✅ PASS |
| `US-AD77` | Must | M0 | Halaman settings org | `37-workspace-settings` | ✅ PASS |
| `US-AD88` | Must | M0 | Reset password (lupa password) | `03-reset-request`, `04-reset-confirm` | ✅ PASS |
| `US-AD89` | Must | M0 | Profil akun sendiri | `15-profile` | ✅ PASS |
| `US-AD92` | Must | M0 | Alur pertama kali (first-run onboarding) | `12-onboarding` | ⏸️ ditunda |
| `US-AD93` | Must | M0 | Konteks ruang kerja aktif | *backend-only* | ✅ PASS |
| `US-AD08` | Must | M1 | Membuat project dalam Org | `11-project-list` | ✅ PASS |
| `US-AD09` | Must | M1 | Membuat board kanban | `18-kanban` | ✅ PASS |
| `US-AD10` | Must | M1 | Mengedit kolom board | `22-column-editor` | ✅ PASS |
| `US-AD11` | Must | M1 | Membuat task | `18-kanban`, `21-task-create` | ✅ PASS |
| `US-AD12` | Must | M1 | Drag task antar kolom | `18-kanban` | ✅ PASS |
| `US-AD13` | Must | M1 | Task detail drawer | `20-task-drawer` | ✅ PASS |
| `US-AD14` | Must | M1 | Assign task ke agent | `21-task-create` | ✅ PASS |
| `US-AD15` | Must | M1 | Filter task berdasarkan status dan assignee | `18-kanban` | ✅ PASS |
| `US-AD16` | Should | M1 | Cari task | `18-kanban` | ✅ PASS |
| `US-AD17` | Should | M1 | Priority task (urgent, high, medium, low) | `18-kanban` | ✅ PASS |
| `US-AD20` | Must | M1 | Agent registry (CRUD) | `25-agent-registry` | ✅ PASS |
| `US-AD21` | Must | M1 | Dispatcher mengklaim task | *backend-only* | ✅ PASS |
| `US-AD22` | Must | M1 | Run lifecycle: claim → finish | *backend-only* | ✅ PASS |
| `US-AD23` | Must | M1 | Heartbeat berkala | *backend-only* | ✅ PASS |
| `US-AD24` | Must | M1 | Reclaim task stale | *backend-only* | ✅ PASS |
| `US-AD58` | Must | M1 | Menandai task selesai (done) | `21-task-create` | ✅ PASS |
| `US-AD59` | Must | M1 | Task archived | `18-kanban`, `20-task-drawer` | ✅ PASS |
| `US-AD60` | Must | M1 | View toggle: tampilan mobile | `46-mobile-board` | ⬜ |
| `US-AD62` | Should | M1 | Filter tasks berdasarkan tanggal | `18-kanban` | ✅ PASS |
| `US-AD64` | Must | M1 | Empty state board | `44-state-empty` | ✅ PASS |
| `US-AD66` | Must | M1 | Max runtime per task (N9) | *backend-only* | ✅ PASS |
| `US-AD67` | Must | M1 | Menentukan model dan provider per agent | `26-agent-form`, `28-agent-detail` | 🔨 dikerjakan |
| `US-AD73` | Must | M1 | Menonaktifkan (archive) agent | `25-agent-registry`, `28-agent-detail` | 🔨 dikerjakan |
| `US-AD75` | Must | M1 | Workspace management: menentukan workspace_kind | *backend-only* | ✅ PASS |
| `US-AD76` | Should | M1 | Halaman dashboard | `42-dashboard` | ✅ PASS |
| `US-AD79` | Must | M1 | Reassign task ke agent lain | `20-task-drawer` | ✅ PASS |
| `US-AD81` | Must | M1 | Filter board berdasarkan kolom | `18-kanban` | ✅ PASS |
| `US-AD82` | Must | M1 | Scroll sinkron antar kolom | `18-kanban` | ✅ PASS |
| `US-AD83` | Must | M1 | Mengubah nama board | `23-board-settings` | ✅ PASS |
| `US-AD84` | Must | M1 | Menghapus board | `23-board-settings` | ✅ PASS |
| `US-AD90` | Must | M1 | Ganti password dan sesi aktif | `16-security` | ⬜ |
| `US-AD91` | Must | M1 | Daftar board lintas project | `10-board-list`, `11-project-list` | ✅ PASS |
| `US-AD106` | Must | M2 | Provider BYO (bring your own) | `26-agent-form` | ✅ PASS |
| `US-AD107` | Should | M2 | Skill library per ruang kerja | `26b-agent-skills` | ⬜ |
| `US-AD108` | Must | M2 | Estimasi biaya: label dan sumber harga | `26-agent-form` | 🔨 dikerjakan |
| `US-AD109` | Must | M2 | Provider registry: daftar kredensial sekali pakai | `47-providers` | ✅ PASS |
| `US-AD18` | Must | M2 | Dependency DAG antar task | *backend-only* | ✅ PASS |
| `US-AD19` | Should | M2 | Visualisasi dependency di board | `18-kanban`, `24-dependency-view` | ✅ PASS |
| `US-AD25` | Must | M2 | Menulis step (trace) dalam run | *backend-only* | ✅ PASS |
| `US-AD27` | Must | M2 | Cost ledger: mencatat pemakaian token per step | `30-ledger-explorer` | ⬜ |
| `US-AD28` | Must | M2 | Agregasi biaya harian per board | `29-cost-overview` | ✅ PASS |
| `US-AD29` | Must | M2 | Budget guardrail: hard stop per run ($2) | `29-cost-overview` | ✅ PASS |
| `US-AD30` | Must | M2 | Budget harian board ($20) | `29-cost-overview` | ✅ PASS |
| `US-AD31` | Must | M2 | Alert budget 80% | `29-cost-overview` | ✅ PASS |
| `US-AD32` | Must | M2 | Melihat total biaya task di card | `18-kanban` | ✅ PASS |
| `US-AD68` | Must | M2 | Provider harga dinamis (price_version) | *backend-only* | ✅ PASS |
| `US-AD69` | Should | M2 | Menampilkan total biaya project | `29-cost-overview` | ✅ PASS |
| `US-AD70` | Must | M2 | Proteksi siklus dependency | *backend-only* | ✅ PASS |
| `US-AD80` | Must | M2 | Menghapus task (soft delete) | `18-kanban`, `20-task-drawer` | ✅ PASS |
| `US-AD86` | Must | M2 | Simpan kredensial provider LLM per agent | `27-agent-provider-key` | ✅ PASS |
| `US-AD94` | Must | M2 | Panel detail step dan payload | `34-step-payload` | ✅ PASS |
| `US-AD96` | Must | M2 | Formulir agent (buat dan ubah) | `26-agent-form` | ✅ PASS |
| `US-AD97` | Must | M2 | Cost rail: panel biaya sisi kanan | `18-kanban` | ✅ PASS |
| `US-AD26` | Must | M3 | Melihat timeline step per run | `33-run-timeline` | ✅ PASS |
| `US-AD33` | Must | M3 | Approval gate: request approval | *backend-only* | ✅ PASS |
| `US-AD34` | Must | M3 | Approval: approve | `36-approval-detail` | ✅ PASS |
| `US-AD35` | Must | M3 | Approval: reject | `36-approval-detail` | ✅ PASS |
| `US-AD36` | Must | M3 | Approval: expired otomatis | *backend-only* | ✅ PASS |
| `US-AD37` | Must | M3 | Melihat antrean approval | `35-approval-inbox` | ✅ PASS |
| `US-AD38` | Must | M3 | Approval gate mode per agent | `35-approval-inbox` | ✅ PASS |
| `US-AD39` | Must | M3 | Event stream SSE: perubahan status task | `18-kanban` | ✅ PASS |
| `US-AD40` | Must | M3 | Timeline event per task | `20-task-drawer` | ✅ PASS |
| `US-AD41` | Must | M3 | Halaman detail run | `32-run-detail` | ✅ PASS |
| `US-AD42` | Must | M3 | Menambahkan komentar ke task | `20-task-drawer` | ✅ PASS |
| `US-AD61` | Must | M3 | Notifikasi in-app | `13-notifications` | ✅ PASS |
| `US-AD63` | Must | M3 | Skeleton loading state | `43-state-loading` | ✅ PASS |
| `US-AD65` | Must | M3 | Error boundary UI | `45-state-error` | ✅ PASS |
| `US-AD71` | Should | M3 | Comment dengan mention | `20-task-drawer` | ✅ PASS |
| `US-AD85` | Must | M3 | Rate limit per endpoint | *backend-only* | ⬜ |
| `US-AD43` | Must | M4 | Failue taxonomy: klasifikasi otomatis | *backend-only* | ✅ PASS |
| `US-AD44` | Must | M4 | Retry otomatis berdasarkan failure_kind | *backend-only* | ✅ PASS |
| `US-AD45` | Must | M4 | Max attempts dan dead letter | *backend-only* | ✅ PASS |
| `US-AD46` | Must | M4 | Upload artifact ke R2 | *backend-only* | ✅ PASS |
| `US-AD47` | Must | M4 | Download artifact | *backend-only* | ✅ PASS |
| `US-AD48` | Must | M4 | Menampilkan daftar artifact per task | `20-task-drawer` | ✅ PASS |
| `US-AD49` | Must | M4 | UI dwibahasa EN/ID | *backend-only* | ✅ PASS |
| `US-AD50` | Must | M4 | Deteksi string keras (hardcoded) di CI | *backend-only* | ⬜ |
| `US-AD74` | Must | M4 | Error handling: provider LLM down | *backend-only* | ✅ PASS |
| `US-AD87` | Must | M4 | Agent gagal karena kredensial invalid | `27-agent-provider-key` | ✅ PASS |

**Total M0–M4: 89 story** (79 PASS, 1 ditunda)

## Progres per milestone

| Milestone | Story | PASS | Sisa |
|---|---|---|---|
| M0 | 10 | 9 | 1 |
| M1 | 32 | 28 | 4 |
| M2 | 21 | 18 | 3 |
| M3 | 16 | 15 | 1 |
| M4 | 10 | 9 | 1 |

### Catatan status — `US-AD73` (🔨 dikerjakan)

**backend PASS, UI sebagian.**

- Sudah jalan: `PATCH /api/v1/agents/{id}` dengan `{"archived": true|false}`; guard 409 saat agent masih memegang run; guard 403 untuk member/viewer; `archived_at` dikembalikan di `GET`/list/`POST`; badge `DIARSIP` dan filter `DIARSIP` di registry; agent terarsip dikeluarkan dari hitungan siap-ditugaskan.
- AC2 sudah punya sumber data: `GET /api/v1/boards/{board_id}/assignable-agents` memanggil `ListAssignableAgentsForBoard` (`cmd/api/boards.go`, `internal/board/pgx.go`), dan predikatnya mengecualikan agent terarsip. Catatan lama yang bilang "tidak ada handler maupun endpoint yang memakainya" sudah tidak benar dan dikoreksi di sini.
- AC1 (task `running` tetap tuntas saat agent diarsip) baru bisa dibuktikan end-to-end setelah executor M4 ada.
- Status PASS ditahan sampai kedua AC itu bisa dibuktikan — bukan karena gate merah.

