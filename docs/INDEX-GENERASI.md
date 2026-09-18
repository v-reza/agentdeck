# INDEX generasi Stitch — AgentDeck

Urutan kirim: blok shared (`_shared/*.md`) SEKALI di awal sesi, lalu tiap prompt di kolom Prompt.

| # | Layar | Route | Story | Frame | Prompt |
|---|---|---|---|---|---|
| 1 | Login | `/login` | 1 | 2 | `stitch_prompts_v2/01-login.md` |
| 2 | Register | `/register` | 1 | 2 | `stitch_prompts_v2/02-register.md` |
| 3 | Lupa password — minta tautan | `/reset` | 1 | 2 | `stitch_prompts_v2/03-reset-request.md` |
| 4 | Lupa password — password baru | `/reset/:token` | 1 | 2 | `stitch_prompts_v2/04-reset-confirm.md` |
| 5 | Landing page | `/` | 1 | 1 | `stitch_prompts_v2/05-landing.md` |
| 6 | Dokumentasi — mulai cepat | `/docs/quickstart` | 1 | 1 | `stitch_prompts_v2/06-docs-quickstart.md` |
| 7 | Dokumentasi — REST API | `/docs/api` | 1 | 1 | `stitch_prompts_v2/06b-docs-api.md` |
| 8 | Dokumentasi — skema telemetri | `/docs/telemetry` | 1 | 1 | `stitch_prompts_v2/06c-docs-telemetry.md` |
| 9 | Harga | `/pricing` | 1 | 1 | `stitch_prompts_v2/07-pricing.md` |
| 10 | Changelog | `/changelog` | 1 | 1 | `stitch_prompts_v2/08-changelog.md` |
| 11 | Product / fitur | `/product` | 1 | 1 | `stitch_prompts_v2/09-features.md` |
| 12 | Repositori & rilis | `/github` | 1 | 2 | `stitch_prompts_v2/09b-github.md` |
| 13 | Komunitas & dukungan | `/community` | 1 | 2 | `stitch_prompts_v2/09c-community.md` |
| 14 | Daftar board | `/boards` | 2 | 4 | `stitch_prompts_v2/10-board-list.md` |
| 15 | Daftar project | `/projects` | 2 | 3 | `stitch_prompts_v2/11-project-list.md` |
| 16 | First-run onboarding | `/onboarding` | 1 | 3 | `stitch_prompts_v2/12-onboarding.md` |
| 17 | Notifikasi | `/notifications` | 1 | 2 | `stitch_prompts_v2/13-notifications.md` |
| 18 | Command palette | `(overlay)` | 1 | 2 | `stitch_prompts_v2/14-command-palette.md` |
| 19 | Profil akun | `/settings/profile` | 1 | 2 | `stitch_prompts_v2/15-profile.md` |
| 20 | Keamanan: password + sesi aktif | `/settings/security` | 1 | 2 | `stitch_prompts_v2/16-security.md` |
| 21 | Tutup akun | `/settings/close` | 1 | 2 | `stitch_prompts_v2/17-close-account.md` |
| 22 | Board kanban | `/boards/:id` | 15 | 4 | `stitch_prompts_v2/18-kanban.md` |
| 23 | Tabel task | `/boards/:id?view=table` | 2 | 3 | `stitch_prompts_v2/19-table-view.md` |
| 24 | Detail task (drawer) | `(drawer)` | 8 | 3 | `stitch_prompts_v2/20-task-drawer.md` |
| 25 | Buat task (modal) | `(modal)` | 3 | 2 | `stitch_prompts_v2/21-task-create.md` |
| 26 | Editor kolom board | `(panel)` | 1 | 2 | `stitch_prompts_v2/22-column-editor.md` |
| 27 | Pengaturan board | `/boards/:id/settings` | 2 | 2 | `stitch_prompts_v2/23-board-settings.md` |
| 28 | Graf dependency | `/boards/:id/graph` | 1 | 1 | `stitch_prompts_v2/24-dependency-view.md` |
| 29 | Registry agent | `/agents` | 2 | 2 | `stitch_prompts_v2/25-agent-registry.md` |
| 30 | Formulir agent | `/agents/new` | 2 | 3 | `stitch_prompts_v2/26-agent-form.md` |
| 31 | Kredensial provider agent | `(panel)` | 2 | 3 | `stitch_prompts_v2/27-agent-provider-key.md` |
| 32 | Detail agent | `/agents/:id` | 2 | 2 | `stitch_prompts_v2/28-agent-detail.md` |
| 33 | Ringkasan biaya | `/cost` | 5 | 2 | `stitch_prompts_v2/29-cost-overview.md` |
| 34 | Ledger biaya | `/cost/ledger` | 1 | 2 | `stitch_prompts_v2/30-ledger-explorer.md` |
| 35 | Ekspor CSV biaya | `(modal)` | 1 | 2 | `stitch_prompts_v2/31-cost-export.md` |
| 36 | Detail run | `/runs/:id` | 1 | 3 | `stitch_prompts_v2/32-run-detail.md` |
| 37 | Timeline step | `/runs/:id/timeline` | 1 | 2 | `stitch_prompts_v2/33-run-timeline.md` |
| 38 | Detail step & payload | `(panel)` | 1 | 3 | `stitch_prompts_v2/34-step-payload.md` |
| 39 | Antrean approval | `/approvals` | 2 | 2 | `stitch_prompts_v2/35-approval-inbox.md` |
| 40 | Detail approval | `/approvals/:id` | 2 | 3 | `stitch_prompts_v2/36-approval-detail.md` |
| 41 | Pengaturan ruang kerja | `/settings/workspace` | 2 | 1 | `stitch_prompts_v2/37-workspace-settings.md` |
| 42 | Anggota & role | `/settings/members` | 2 | 2 | `stitch_prompts_v2/38-members.md` |
| 43 | API key | `/settings/api-keys` | 1 | 3 | `stitch_prompts_v2/39-api-keys.md` |
| 44 | Webhook | `/settings/webhooks` | 2 | 3 | `stitch_prompts_v2/40-webhooks.md` |
| 45 | Audit log | `/settings/audit` | 1 | 3 | `stitch_prompts_v2/41-audit-log.md` |
| 46 | Dashboard | `/dashboard` | 1 | 2 | `stitch_prompts_v2/42-dashboard.md` |
| 47 | State: loading (skeleton) | `(varian)` | 1 | 1 | `stitch_prompts_v2/43-state-loading.md` |
| 48 | State: empty | `(varian)` | 1 | 1 | `stitch_prompts_v2/44-state-empty.md` |
| 49 | State: error | `(varian)` | 1 | 1 | `stitch_prompts_v2/45-state-error.md` |
| 50 | Board mobile | `/m/boards/:id` | 1 | 1 | `stitch_prompts_v2/46-mobile-board.md` |

**Total: 50 layar, 103 frame, 90 penugasan story.**
