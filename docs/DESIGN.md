---
version: alpha
name: AgentDeck
description: Orchestration board for AI agent fleets — every run priced, every action gated. Status-driven, green-neutral, mono-forward.
colors:
  # --- teks: hijau-hitam, bukan netral biru ---
  primary: "#0e1110"
  secondary: "#4d5553"
  tertiary: "#6b7370"
  quaternary: "#98a09d"
  text-primary: "#0e1110"
  text-secondary: "#4d5553"
  text-tertiary: "#6b7370"
  text-quaternary: "#98a09d"
  # --- permukaan: neutral hijau-tinted ---
  neutral: "#f6f7f6"
  surface-page: "#f6f7f6"
  surface-panel: "#ffffff"
  surface-elevated: "#ffffff"
  surface-hover: "#eff2f1"
  surface-sunken: "#e8ecea"
  surface-well: "#eef1f0"
  # --- border: tinta hijau, bukan biru ---
  border-subtle: "rgba(12,26,22,0.07)"
  border-standard: "rgba(12,26,22,0.12)"
  border-strong: "rgba(12,26,22,0.20)"
  # --- aksen tunggal: Signal Teal ---
  accent: "#0d7a70"
  accent-hover: "#0a625a"
  accent-tint: "#e6f2f0"
  on-accent: "#ffffff"
  on-primary: "#ffffff"
  # --- semantik ---
  success: "#15803d"
  warning: "#b45309"
  danger: "#b91c1c"
  info: "#1d4ed8"
  on-success: "#ffffff"
  on-warning: "#ffffff"
  on-danger: "#ffffff"
  on-info: "#ffffff"
  # --- 10 status task: ini palet visual utama, bukan pelengkap ---
  status-backlog: "#5c6b7a"
  status-ready: "#1d4ed8"
  status-running: "#b45309"
  status-awaiting-approval: "#6d28d9"
  status-blocked: "#c2410c"
  status-review: "#0e7490"
  status-done: "#15803d"
  status-failed: "#b91c1c"
  status-cancelled: "#8a9490"
  status-archived: "#c3cac7"
  on-status-backlog: "#ffffff"
  on-status-ready: "#ffffff"
  on-status-running: "#ffffff"
  on-status-awaiting-approval: "#ffffff"
  on-status-blocked: "#ffffff"
  on-status-review: "#ffffff"
  on-status-done: "#ffffff"
  on-status-failed: "#ffffff"
  on-status-cancelled: "#ffffff"
  on-status-archived: "#0e1110"
  # --- metrik: warna angka, bukan dekorasi ---
  token-in: "#1d4ed8"
  token-out: "#6d28d9"
  cost: "#0d7a70"
  cost-over: "#b91c1c"
  overlay: "rgba(12,18,16,0.44)"
typography:
  display-xl:
    fontFamily: Inter
    fontSize: 2rem
    fontWeight: 700
    lineHeight: 1.08
    letterSpacing: "-0.03em"
  display-lg:
    fontFamily: Inter
    fontSize: 1.5rem
    fontWeight: 700
    lineHeight: 1.15
    letterSpacing: "-0.02em"
  heading-section:
    fontFamily: Inter
    fontSize: 1.125rem
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: "-0.01em"
  heading-card:
    fontFamily: Inter
    fontSize: 0.875rem
    fontWeight: 600
    lineHeight: 1.4
  body:
    fontFamily: Inter
    fontSize: 0.8125rem
    fontWeight: 400
    lineHeight: 1.5
  body-sm:
    fontFamily: Inter
    fontSize: 0.75rem
    fontWeight: 400
    lineHeight: 1.45
  label:
    fontFamily: Inter
    fontSize: 0.6875rem
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: 0.06em
  mono-metric:
    fontFamily: JetBrains Mono
    fontSize: 0.875rem
    fontWeight: 600
    lineHeight: 1.3
    fontFeature: tnum
  mono-number:
    fontFamily: JetBrains Mono
    fontSize: 0.8125rem
    fontWeight: 500
    lineHeight: 1.4
    fontFeature: tnum
  mono-code:
    fontFamily: JetBrains Mono
    fontSize: 0.75rem
    fontWeight: 400
    lineHeight: 1.5
  mono-xs:
    fontFamily: JetBrains Mono
    fontSize: 0.6875rem
    fontWeight: 400
    lineHeight: 1.35
rounded:
  xs: 4px
  sm: 6px
  md: 10px
  lg: 14px
  full: 9999px
spacing:
  xxs: 2px
  xs: 4px
  sm: 8px
  md: 10px
  lg: 16px
  xl: 24px
  "2xl": 32px
components:
  # ============ SHELL (kontrak frame, angka dipatok) ============
  shell-rail:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.text-tertiary}"
    width: 44px
    borderColor: "{colors.border-subtle}"
  shell-sidebar:
    backgroundColor: "{colors.surface-page}"
    textColor: "{colors.text-secondary}"
    width: 224px
  shell-topbar:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    height: 52px
    borderColor: "{colors.border-subtle}"
  shell-costrail:
    backgroundColor: "{colors.surface-well}"
    textColor: "{colors.text-secondary}"
    width: 264px
  # ============ NAVIGASI ============
  rail-icon:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.text-tertiary}"
    rounded: "{rounded.sm}"
    width: 32px
    height: 32px
  rail-icon-active:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.sm}"
    width: 32px
    height: 32px
  sidebar-item:
    backgroundColor: "{colors.surface-page}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.sm}"
    padding: 6px 10px
  sidebar-item-active:
    backgroundColor: "{colors.accent-tint}"
    textColor: "{colors.accent}"
    rounded: "{rounded.sm}"
    padding: 6px 10px
  # ============ TOMBOL (padding 8px, radius 6 — bukan 12) ============
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.sm}"
    padding: 8px 14px
  button-primary-hover:
    backgroundColor: "{colors.accent-hover}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.sm}"
    padding: 8px 14px
  button-secondary:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.sm}"
    padding: 8px 14px
  button-ghost:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.sm}"
    padding: 8px 14px
  button-danger:
    backgroundColor: "{colors.danger}"
    textColor: "{colors.on-danger}"
    rounded: "{rounded.sm}"
    padding: 8px 14px
  # ============ INPUT ============
  input:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.sm}"
    padding: 8px 10px
  input-focus:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.sm}"
    padding: 8px 10px
  textarea:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.sm}"
    padding: 8px 10px
  select:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.sm}"
    padding: 8px 10px
  checkbox:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.xs}"
    width: 14px
    height: 14px
  # ============ BOARD (kolom ber-tint status + top edge) ============
  board-background:
    backgroundColor: "{colors.surface-page}"
    textColor: "{colors.text-primary}"
  board-column:
    backgroundColor: "{colors.surface-well}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 10px
  board-column-header:
    backgroundColor: "{colors.surface-well}"
    textColor: "{colors.text-secondary}"
    height: 30px
  column-edge-backlog:
    backgroundColor: "{colors.status-backlog}"
    height: 3px
  column-edge-ready:
    backgroundColor: "{colors.status-ready}"
    height: 3px
  column-edge-running:
    backgroundColor: "{colors.status-running}"
    height: 3px
  column-edge-awaiting:
    backgroundColor: "{colors.status-awaiting-approval}"
    height: 3px
  column-edge-review:
    backgroundColor: "{colors.status-review}"
    height: 3px
  column-edge-done:
    backgroundColor: "{colors.status-done}"
    height: 3px
  # ============ KARTU TASK (padding 10, radius 10) ============
  card-task:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 10px
    borderColor: "{colors.border-subtle}"
  card-task-hover:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 10px
    borderColor: "{colors.border-standard}"
  card-task-dragging:
    backgroundColor: "{colors.surface-elevated}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 10px
    borderColor: "{colors.border-strong}"
  # ============ TABEL (baris 28px — paling padat) ============
  table-header:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.text-secondary}"
    height: 32px
    borderColor: "{colors.border-standard}"
  table-row:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    height: 28px
    borderColor: "{colors.border-subtle}"
  table-row-hover:
    backgroundColor: "{colors.surface-hover}"
    textColor: "{colors.text-primary}"
    height: 28px
  table-row-selected:
    backgroundColor: "{colors.accent-tint}"
    textColor: "{colors.text-primary}"
    height: 28px
  # ============ BADGE STATUS (10 status) ============
  badge-status-backlog:
    backgroundColor: "{colors.status-backlog}"
    textColor: "{colors.on-status-backlog}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-ready:
    backgroundColor: "{colors.status-ready}"
    textColor: "{colors.on-status-ready}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-running:
    backgroundColor: "{colors.status-running}"
    textColor: "{colors.on-status-running}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-awaiting-approval:
    backgroundColor: "{colors.status-awaiting-approval}"
    textColor: "{colors.on-status-awaiting-approval}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-blocked:
    backgroundColor: "{colors.status-blocked}"
    textColor: "{colors.on-status-blocked}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-review:
    backgroundColor: "{colors.status-review}"
    textColor: "{colors.on-status-review}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-done:
    backgroundColor: "{colors.status-done}"
    textColor: "{colors.on-status-done}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-failed:
    backgroundColor: "{colors.status-failed}"
    textColor: "{colors.on-status-failed}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-cancelled:
    backgroundColor: "{colors.status-cancelled}"
    textColor: "{colors.on-status-cancelled}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-status-archived:
    backgroundColor: "{colors.status-archived}"
    textColor: "{colors.on-status-archived}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  # ============ COST RAIL (panel kanan, identitas AgentDeck) ============
  costrail-panel:
    backgroundColor: "{colors.surface-well}"
    textColor: "{colors.text-primary}"
    padding: 12px
    borderColor: "{colors.border-subtle}"
  costrail-metric:
    backgroundColor: "{colors.surface-well}"
    textColor: "{colors.text-primary}"
    padding: 4px 0
  budget-meter:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.accent}"
    rounded: "{rounded.full}"
    height: 6px
  budget-meter-warning:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.warning}"
    rounded: "{rounded.full}"
    height: 6px
  budget-meter-over:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.cost-over}"
    rounded: "{rounded.full}"
    height: 6px
  sparkline-bar:
    backgroundColor: "{colors.accent}"
    width: 5px
    rounded: "{rounded.xs}"
  # ============ APPROVAL ============
  approval-banner:
    backgroundColor: "{colors.accent-tint}"
    textColor: "{colors.accent}"
    padding: 8px 12px
    borderColor: "{colors.border-subtle}"
  approval-card:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 12px
    borderColor: "{colors.border-standard}"
  approval-card-gated:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 12px
    borderColor: "{colors.status-awaiting-approval}"
  code-block:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.sm}"
    padding: 10px
  # ============ OVERLAY / PANEL ============
  drawer:
    backgroundColor: "{colors.surface-panel}"
    textColor: "{colors.text-primary}"
    width: 420px
    borderColor: "{colors.border-standard}"
  modal:
    backgroundColor: "{colors.surface-elevated}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: 20px
  overlay-backdrop:
    backgroundColor: "{colors.overlay}"
    textColor: "{colors.text-primary}"
  tooltip:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.xs}"
    padding: 4px 8px
  # ============ METRIK & STATUS ============
  badge-cost:
    backgroundColor: "{colors.accent-tint}"
    textColor: "{colors.cost}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-token-in:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.token-in}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-token-out:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.token-out}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-success:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.success}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-warning:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.warning}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-danger:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.danger}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  badge-info:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.info}"
    rounded: "{rounded.xs}"
    padding: 2px 6px
  # ============ LAIN-LAIN ============
  avatar-agent:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.xs}"
    width: 20px
    height: 20px
  avatar-user:
    backgroundColor: "{colors.accent-tint}"
    textColor: "{colors.accent}"
    rounded: "{rounded.full}"
    width: 26px
    height: 26px
  progress-bar:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.full}"
    height: 4px
  skeleton:
    backgroundColor: "{colors.surface-hover}"
    textColor: "{colors.text-quaternary}"
    rounded: "{rounded.sm}"
    height: 12px
  empty-state:
    backgroundColor: "{colors.surface-page}"
    textColor: "{colors.text-tertiary}"
    padding: 32px
  toast-success:
    backgroundColor: "{colors.success}"
    textColor: "{colors.on-success}"
    rounded: "{rounded.sm}"
    padding: 10px 12px
  toast-error:
    backgroundColor: "{colors.danger}"
    textColor: "{colors.on-danger}"
    rounded: "{rounded.sm}"
    padding: 10px 12px
  kbd:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.xs}"
    padding: 2px 5px
  divider:
    backgroundColor: "{colors.border-subtle}"
    height: 1px
  divider-subtle:
    backgroundColor: "{colors.border-subtle}"
    height: 1px
  divider-strong:
    backgroundColor: "{colors.border-standard}"
    height: 1px



---

**Stack:** React 19 + Vite 6 · TypeScript · Tailwind v4 · shadcn/ui · Redux Toolkit + RTK Query
