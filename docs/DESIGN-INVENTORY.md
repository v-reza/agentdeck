# Design inventory — design vs implementasi

**Dihasilkan `tools/design_audit.py`.** Jangan diedit tangan: angkanya
dihitung dari parse `design/stitch-output/v2/*.html` dan
`frontend/src/**` (137 file), bukan dari ingatan. Jalankan ulang
scriptnya kalau ada yang berubah.

## 1. Ringkasan

| | design | impl |
|---|---|---|
| `<svg>` | 111 | 61 |

Dua angka icon di baris pertama itu **bukan** jumlah elemen unik. Mockup
menggambar ulang icon yang sama di tiap kartu (satu jam per kartu approval),
sementara implementasi mendeklarasikan komponennya sekali lalu merendernya
di dalam `.map()`. Jadi rasio mentahnya selalu terlihat lebih buruk daripada
kenyataan, dan satu-satunya cara membacanya adalah per jenis icon di layar
yang punya file impl — lihat §3.
| blok skeleton (abu berukuran) | 323 | 20 |
| `<select>` bawaan HTML | 13 | 0 |
| file memakai ligature Material Symbols | 22 | 0 |
| file memakai `lucide-react` | 0 | 21 |

## 2. Icon — dua set, bukan satu

`docs/DESIGN-INVENTORY.md` yang lama bilang mockup memakai satu set
konsisten Lucide. Itu salah, dan script ini yang membuktikannya:

- **22 dari 51 file mockup** memakai ligature
  `<span class="material-symbols-outlined">nama_icon</span>`.
- Sisanya memakai `<svg>` inline yang geometrinya cocok Lucide.

Keputusan repo: **Lucide menang** (memory + konvensi repo). Ligature
Material Symbols tidak dibawa ke produk.

File mockup yang memakai ligature:

- `07-pricing`
- `12-onboarding`
- `13-notifications`
- `14-command-palette`
- `15-profile`
- `16-security`
- `17-close-account`
- `19-table-view`
- `22-column-editor`
- `23-board-settings`
- `24-dependency-view`
- `30-ledger-explorer`
- `31-cost-export`
- `34-step-payload`
- `36-approval-detail`
- `37-workspace-settings`
- `38-members`
- `42-dashboard`
- `43-state-loading`
- `45-state-error`
- `46-mobile-board`
- `47-providers`

## 3. Skeleton vs dot status

Dua hal berbeda yang pernah dihitung jadi satu:

- **blok skeleton** — `bg-[#e8ecea]` + ukuran, placeholder isi
- **dot status** — `animate-pulse` + `rounded-full` + 1.5-2px

| layar | design skeleton | impl skeleton | design dot |
|---|---|---|---|
| 43-state-loading | 105 | 2 | 0 |
| 38-members | 25 | 1 | 1 |
| 22-column-editor | 24 | 1 | 1 |
| 19-table-view | 19 | 1 | 4 |
| 42-dashboard | 19 | 0 | 3 |
| 15-profile | 15 | 1 | 2 |
| 47-providers | 15 | 1 | 2 |
| 30-ledger-explorer | 14 | 0 | 3 |
| 24-dependency-view | 12 | 0 | 4 |
| 37-workspace-settings | 10 | 1 | 1 |
| 14-command-palette | 9 | 0 | 3 |
| 16-security | 8 | 0 | 2 |

## 4. Dropdown bawaan HTML yang masih hidup

Kontrak repo: satu komponen `Combobox` buat semua select, token design
wajib, jangan token bawaan browser. Sisa native:

| file | jumlah | baris |
|---|---|---|

## 5. Warna status: beku vs impl

Sumber beku: `DECISIONS.md` §8 paragraf "Status color".

| status | beku (DECISIONS §8) | impl (`index.css`) | |
|---|---|---|---|
| archived | `#cbd5e1` | `#cbd5e1` | sama |
| awaiting_approval | `#7c3aed` | `#7c3aed` | sama |
| backlog | `#64748b` | `#64748b` | sama |
| blocked | `#ea580c` | `#ea580c` | sama |
| cancelled | `#94a3b8` | `#94a3b8` | sama |
| done | `#16a34a` | `#16a34a` | sama |
| failed | `#dc2626` | `#dc2626` | sama |
| ready | `#2563eb` | `#2563eb` | sama |
| review | `#0891b2` | `#0891b2` | sama |
| running | `#d97706` | `#d97706` | sama |

**10 sama, 0 beda.**

## 6. Token spacing

Dideklarasikan: 6. Nol dipakai: 0.

| token | nilai | dipakai |
|---|---|---|
| `--spacing-costrail` | 264px | 2 |
| `--spacing-rail` | 44px | 2 |
| `--spacing-row` | 32px | 3 |
| `--spacing-row-dense` | 28px | 4 |
| `--spacing-sidebar` | 224px | 2 |
| `--spacing-topbar` | 52px | 4 |

## 7. State archived

| penanda | design | impl |
|---|---|---|
| `line-through` | 6 | 2 |
| `opacity-*` | 37 | 14 |

## 8. Jargon yang bocor ke UI yang dirender

Kontrak: `US-AD`, `AC`, `M0`-`M6` dilarang masuk UI yang dirender.
Komentar kode boleh. Yang di bawah ini **bukan** komentar — dihitung
setelah semua komentar JS dan JSX dibuang.

Nol.

## 9. Per layar

| layar | route | file impl | svg design/impl | select impl | skeleton impl |
|---|---|---|---|---|---|
| 01-login | `/login` | `AuthShell.tsx`, `Login.tsx`, `variants.ts` | 0/0 | 0 | 0 |
| 02-register | `/register` | `AuthShell.tsx`, `Register.tsx`, `variants.ts` | 0/0 | 0 | 0 |
| 03-reset-request | `/reset` | `AuthShell.tsx`, `ResetRequest.tsx`, `variants.ts` | 0/0 | 0 | 0 |
| 04-reset-confirm | `/reset/:token` | `AuthShell.tsx`, `ResetConfirm.tsx`, `variants.ts` | 0/0 | 0 | 0 |
| 05-landing | `/` | `LandingPage.tsx` | 9/0 | 0 | 0 |
| 06-docs-quickstart | `/docs/quickstart` | — | 4/0 | 0 | 0 |
| 06b-docs-api | `/docs/api` | — | 0/0 | 0 | 0 |
| 06c-docs-telemetry | `/docs/telemetry` | — | 0/0 | 0 | 0 |
| 07-pricing | `/pricing` | `PricingPage.tsx` | 0/0 | 0 | 0 |
| 08-changelog | `/changelog` | `ChangelogPage.tsx` | 0/0 | 0 | 0 |
| 09-features | `/product` | `FeaturesPage.tsx` | 1/0 | 0 | 0 |
| 09b-github | `/github` | `GitHubPage.tsx` | 3/0 | 0 | 0 |
| 09c-community | `/community` | `CommunityPage.tsx` | 2/0 | 0 | 0 |
| 10-board-list | `/boards` | `BoardList.tsx`, `CreateBoardForm.tsx`, `ProjectDetail.tsx` | 9/2 | 0 | 2 |
| 11-project-list | `/projects` | `CostRail.tsx`, `WorkspaceSidebar.tsx`, `WorkspaceTopbar.tsx`, `CreateProjectForm.tsx`, `ProjectDirectory.tsx`, `ProjectList.tsx`, `ProjectSummary.tsx` | 6/12 | 0 | 2 |
| 12-onboarding | `/onboarding` | — | 0/0 | 0 | 0 |
| 13-notifications | `/notifications` | — | 0/0 | 0 | 0 |
| 14-command-palette | `(overlay)` | — | 0/0 | 0 | 0 |
| 15-profile | `/settings/profile` | `Profile.tsx` | 0/7 | 0 | 1 |
| 16-security | `/settings/security` | — | 0/0 | 0 | 0 |
| 17-close-account | `/settings/close` | — | 0/0 | 0 | 0 |
| 18-kanban | `/boards/:id` | `KanbanBoard.tsx` | 5/0 | 0 | 1 |
| 19-table-view | `/boards/:id?view=table` | `TableView.tsx` | 0/0 | 0 | 1 |
| 20-task-drawer | `(drawer)` | `StepTimeline.tsx`, `TaskDetailDrawer.tsx` | 12/0 | 0 | 1 |
| 21-task-create | `(modal)` | `TaskCreateForm.tsx` | 7/0 | 0 | 0 |
| 22-column-editor | `(panel)` | `ColumnEditor.tsx`, `BoardSettings.tsx` | 0/6 | 0 | 1 |
| 23-board-settings | `/boards/:id/settings` | `BoardSettings.tsx` | 0/0 | 0 | 0 |
| 24-dependency-view | `/boards/:id/graph` | — | 1/0 | 0 | 0 |
| 25-agent-registry | `/agents` | `AgentRegistry.tsx`, `AgentSpecCards.tsx` | 1/1 | 0 | 1 |
| 26-agent-form | `/agents/new` | `ProviderKeyFields.tsx`, `CreateAgentFields.tsx`, `CreateAgentForm.tsx` | 2/1 | 0 | 0 |
| 26b-agent-skills | `(panel)` | — | —/0 | 0 | 0 |
| 27-agent-provider-key | `(panel)` | `AgentProviderKeyPanel.tsx`, `ProviderKeyFields.tsx` | 1/0 | 0 | 0 |
| 28-agent-detail | `/agents/:id` | `AgentAssignment.tsx`, `AgentDetail.tsx`, `AgentDetailForm.tsx`, `AgentDetailOptions.ts`, `AgentDetailParts.tsx`, `AgentPricingCard.tsx` | 4/6 | 0 | 2 |
| 29-cost-overview | `/cost` | `CostOverview.tsx` | 0/0 | 0 | 1 |
| 30-ledger-explorer | `/cost/ledger` | — | 0/0 | 0 | 0 |
| 31-cost-export | `(modal)` | — | 0/0 | 0 | 0 |
| 32-run-detail | `/runs/:id` | — | 18/0 | 0 | 0 |
| 33-run-timeline | `/runs/:id/timeline` | — | 4/0 | 0 | 0 |
| 34-step-payload | `(panel)` | — | 0/0 | 0 | 0 |
| 35-approval-inbox | `/approvals` | `ApprovalInbox.tsx` | 10/3 | 0 | 1 |
| 36-approval-detail | `/approvals/:id` | — | 0/0 | 0 | 0 |
| 37-workspace-settings | `/settings/workspace` | `WorkspaceSettings.tsx` | 0/8 | 0 | 1 |
| 38-members | `/settings/members` | `role-badge.tsx`, `Members.tsx`, `members-actions.tsx` | 0/1 | 0 | 1 |
| 39-api-keys | `/settings/api-keys` | `ApiKeys.tsx` | 7/0 | 0 | 0 |
| 40-webhooks | `/settings/webhooks` | `Webhooks.tsx` | 1/0 | 0 | 0 |
| 41-audit-log | `/settings/audit` | — | 2/0 | 0 | 0 |
| 42-dashboard | `/dashboard` | — | 0/0 | 0 | 0 |
| 43-state-loading | `(varian)` | `skeleton.tsx` | 0/0 | 0 | 2 |
| 44-state-empty | `(varian)` | — | 2/0 | 0 | 0 |
| 45-state-error | `(varian)` | — | 0/0 | 0 | 0 |
| 46-mobile-board | `/m/boards/:id` | — | 0/0 | 0 | 0 |
| 47-providers | `/settings/providers` | `Providers.tsx`, `providers-actions.tsx` | 0/4 | 0 | 1 |

## 10. File tanpa layar (shell & komponen bersama)

84 file tidak dipetakan ke satu layar. Ini wajar untuk
layout, komponen UI, dan store — tapi ikut dihitung di total impl.

- `App.tsx`
- `app/router.tsx`
- `components/Check.tsx`
- `components/CodeBlock.tsx`
- `components/PublicShell.tsx`
- `components/ReleaseEntry.tsx`
- `components/Shell.tsx`
- `components/SupportCard.tsx`
- `components/approvals/ActionButtons.tsx`
- `components/approvals/DiffViewer.tsx`
- `components/approvals/RiskBadge.tsx`
- `components/docs/DataTable.tsx`
- `components/docs/DocHeader.tsx`
- `components/docs/DocSection.tsx`
- `components/docs/DocsLayout.tsx`
- `components/docs/DocsNav.tsx`
- `components/docs/Endpoint.tsx`
- `components/kanban/Column.tsx`
- `components/kanban/TaskCard.tsx`
- `components/landing/ClosingCta.tsx`
- `components/landing/FeatureSections.tsx`
- `components/landing/MiniBoard.tsx`
- `components/landing/Plan.tsx`
- `components/landing/PricingSection.tsx`
- `components/landing/Roadmap.tsx`
- `components/landing/Stats.tsx`
- `components/landing/Steps.tsx`
- `components/landing/Terminal.tsx`
- `components/layout/AccountMenu.tsx`
- `components/layout/AppShell.tsx`
- `components/layout/IconRail.tsx`
- `components/layout/LangToggle.tsx`
- `components/ui/avatar.tsx`
- `components/ui/button.tsx`
- `components/ui/card.tsx`
- `components/ui/combobox.tsx`
- `components/ui/input.tsx`
- `components/ui/modal.tsx`
- `components/ui/toast.tsx`
- `data/content.ts`
- `hooks/use-action-form.ts`
- `hooks/use-board-health.ts`
- `hooks/use-cost-rail.ts`
- `hooks/use-directory-totals.ts`
- `hooks/use-directory.ts`
- `hooks/use-optimistic-card.ts`
- `hooks/use-orgs.ts`
- `hooks/use-projects.ts`
- `hooks/use-sse-cache.ts`
- `hooks/use-t.ts`
- `lib/cn.ts`
- `lib/domain.ts`
- `lib/field-error.ts`
- `lib/format.ts`
- `lib/formatters.ts`
- `lib/i18n.ts`
- `lib/useGitHubReleases.ts`
- `main.tsx`
- `routes/dashboard/Layout.tsx`
- `routes/dashboard/agents/StatusFilter.tsx`
- `routes/dashboard/boards/TaskDrawerHost.tsx`
- `routes/dashboard/finops/LedgerExplorer.tsx`
- `routes/public/DocsPage.tsx`
- `routes/public/NotFoundPage.tsx`
- `routes/public/SupportPage.tsx`
- `store/api/agents.ts`
- `store/api/base.ts`
- `store/api/boards.ts`
- `store/api/finops.ts`
- `store/api/providers.ts`
- `store/api/releases.ts`
- `store/api/session.ts`
- `store/api/stream.ts`
- `store/hooks.ts`
- `store/index.ts`
- `store/listeners/budgetAlert.ts`
- `store/listeners/index.ts`
- `store/listeners/toast.ts`
- `store/slices/directorySlice.ts`
- `store/slices/langSlice.ts`
- `store/slices/sessionSlice.ts`
- `store/slices/toastSlice.ts`
- `store/slices/uiSlice.ts`
- `vite-env.d.ts`

