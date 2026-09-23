# Design inventory — design vs implementasi

**Dihasilkan `tools/design_audit.py`.** Jangan diedit tangan: angkanya
dihitung dari parse `design/stitch-output/v2/*.html` dan
`frontend/src/**` (137 file), bukan dari ingatan. Jalankan ulang
scriptnya kalau ada yang berubah.

## 1. Ringkasan

| | design | impl |
|---|---|---|
| `<svg>` | 319 | 3 |
| blok skeleton (abu berukuran) | 323 | 20 |
| `<select>` bawaan HTML | 13 | 0 |
| file memakai ligature Material Symbols | 22 | 0 |
| file memakai `lucide-react` | 0 | 19 |

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
| 43-state-loading | 105 | 6 | 0 |
| 38-members | 25 | 5 | 1 |
| 22-column-editor | 24 | 5 | 1 |
| 19-table-view | 19 | 5 | 4 |
| 42-dashboard | 19 | 4 | 3 |
| 15-profile | 15 | 5 | 2 |
| 47-providers | 15 | 5 | 2 |
| 30-ledger-explorer | 14 | 4 | 3 |
| 24-dependency-view | 12 | 4 | 4 |
| 37-workspace-settings | 10 | 5 | 1 |
| 14-command-palette | 9 | 4 | 3 |
| 16-security | 8 | 4 | 2 |

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
| 01-login | `/login` | `router.tsx`, `AuthShell.tsx`, `Login.tsx`, `variants.ts`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 02-register | `/register` | `router.tsx`, `AuthShell.tsx`, `Register.tsx`, `variants.ts`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 03-reset-request | `/reset` | `router.tsx`, `AuthShell.tsx`, `ResetRequest.tsx`, `variants.ts`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 04-reset-confirm | `/reset/:token` | `router.tsx`, `AuthShell.tsx`, `ResetConfirm.tsx`, `variants.ts`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 05-landing | `/` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 9/0 | 0 | 4 |
| 06-docs-quickstart | `/docs/quickstart` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 4/0 | 0 | 4 |
| 06b-docs-api | `/docs/api` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 06c-docs-telemetry | `/docs/telemetry` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 07-pricing | `/pricing` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 08-changelog | `/changelog` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 09-features | `/product` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 1/0 | 0 | 4 |
| 09b-github | `/github` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 3/0 | 0 | 4 |
| 09c-community | `/community` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 2/0 | 0 | 4 |
| 10-board-list | `/boards` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `BoardList.tsx`, `CreateBoardForm.tsx`, `CostOverview.tsx`, `ProjectDetail.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 17/0 | 0 | 6 |
| 11-project-list | `/projects` | `router.tsx`, `CostRail.tsx`, `WorkspaceSidebar.tsx`, `WorkspaceTopbar.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `CreateProjectForm.tsx`, `ProjectDirectory.tsx`, `ProjectList.tsx`, `ProjectSummary.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 18/0 | 0 | 6 |
| 12-onboarding | `/onboarding` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 13-notifications | `/notifications` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 14-command-palette | `(overlay)` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 15-profile | `/settings/profile` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Profile.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 5 |
| 16-security | `/settings/security` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 17-close-account | `/settings/close` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 18-kanban | `/boards/:id` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `KanbanBoard.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 11/0 | 0 | 5 |
| 19-table-view | `/boards/:id?view=table` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `TableView.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 5 |
| 20-task-drawer | `(drawer)` | `router.tsx`, `StepTimeline.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `TaskDetailDrawer.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 21/0 | 0 | 5 |
| 21-task-create | `(modal)` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `TaskCreateForm.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 20/0 | 0 | 4 |
| 22-column-editor | `(panel)` | `router.tsx`, `ColumnEditor.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `BoardSettings.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/1 | 0 | 5 |
| 23-board-settings | `/boards/:id/settings` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `BoardSettings.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 24-dependency-view | `/boards/:id/graph` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 1/0 | 0 | 4 |
| 25-agent-registry | `/agents` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `AgentRegistry.tsx`, `AgentSpecCards.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 13/0 | 0 | 5 |
| 26-agent-form | `/agents/new` | `router.tsx`, `ProviderKeyFields.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `CreateAgentFields.tsx`, `CreateAgentForm.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 21/0 | 0 | 4 |
| 26b-agent-skills | `(panel)` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | —/0 | 0 | 4 |
| 27-agent-provider-key | `(panel)` | `router.tsx`, `AgentProviderKeyPanel.tsx`, `ProviderKeyFields.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 19/0 | 0 | 4 |
| 28-agent-detail | `/agents/:id` | `router.tsx`, `Layout.tsx`, `AgentAssignment.tsx`, `AgentDetail.tsx`, `AgentDetailForm.tsx`, `AgentDetailOptions.ts`, `AgentDetailParts.tsx`, `AgentPricingCard.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 16/1 | 0 | 5 |
| 29-cost-overview | `/cost` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 12/0 | 0 | 4 |
| 30-ledger-explorer | `/cost/ledger` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 31-cost-export | `(modal)` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 32-run-detail | `/runs/:id` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 27/0 | 0 | 4 |
| 33-run-timeline | `/runs/:id/timeline` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 9/0 | 0 | 4 |
| 34-step-payload | `(panel)` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 35-approval-inbox | `/approvals` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 23/0 | 0 | 4 |
| 36-approval-detail | `/approvals/:id` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 37-workspace-settings | `/settings/workspace` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `WorkspaceSettings.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 5 |
| 38-members | `/settings/members` | `router.tsx`, `role-badge.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Members.tsx`, `Webhooks.tsx`, `members-actions.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 5 |
| 39-api-keys | `/settings/api-keys` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 20/0 | 0 | 4 |
| 40-webhooks | `/settings/webhooks` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 15/0 | 0 | 4 |
| 41-audit-log | `/settings/audit` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 18/0 | 0 | 4 |
| 42-dashboard | `/dashboard` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 43-state-loading | `(varian)` | `router.tsx`, `skeleton.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 6 |
| 44-state-empty | `(varian)` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 19/0 | 0 | 4 |
| 45-state-error | `(varian)` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 46-mobile-board | `/m/boards/:id` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Webhooks.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 4 |
| 47-providers | `/settings/providers` | `router.tsx`, `Layout.tsx`, `AgentDetail.tsx`, `ApprovalInbox.tsx`, `CostOverview.tsx`, `ApiKeys.tsx`, `Providers.tsx`, `Webhooks.tsx`, `providers-actions.tsx`, `ChangelogPage.tsx`, `CommunityPage.tsx`, `DocsPage.tsx`, `FeaturesPage.tsx`, `GitHubPage.tsx`, `LandingPage.tsx`, `NotFoundPage.tsx`, `PricingPage.tsx`, `SupportPage.tsx`, `agents.ts`, `boards.ts`, `finops.ts`, `providers.ts`, `session.ts`, `index.ts`, `index.ts` | 0/0 | 0 | 5 |

## 10. File tanpa layar (shell & komponen bersama)

72 file tidak dipetakan ke satu layar. Ini wajar untuk
layout, komponen UI, dan store — tapi ikut dihitung di total impl.

- `App.tsx`
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
- `routes/dashboard/agents/StatusFilter.tsx`
- `routes/dashboard/boards/TaskDrawerHost.tsx`
- `routes/dashboard/finops/LedgerExplorer.tsx`
- `store/api/base.ts`
- `store/api/releases.ts`
- `store/api/stream.ts`
- `store/hooks.ts`
- `store/listeners/budgetAlert.ts`
- `store/listeners/toast.ts`
- `store/slices/directorySlice.ts`
- `store/slices/langSlice.ts`
- `store/slices/sessionSlice.ts`
- `store/slices/toastSlice.ts`
- `store/slices/uiSlice.ts`
- `vite-env.d.ts`

