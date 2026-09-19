import type { ReactNode } from 'react'

export type Faq = [question: string, answer: string]

export const faqs: Faq[] = [
  ['Do I need Kubernetes?', 'No. One binary and a Postgres connection string.'],
  ['Where does my agent code run?', 'On your machine. AgentDeck schedules and records; it does not execute your code.'],
  ['Can I gate only some tools?', 'Yes, per agent. Set the gate mode to require, auto, or off.'],
  ['What happens when I hit a budget cap?', 'The run stops with outcome budget_exceeded and the task goes back to ready. Nothing is silently dropped.'],
  ['Which providers and models can I use?', 'AgentDeck is designed to stay provider-agnostic. Register the provider-backed worker you already run, then track its runs, tool calls, and cost in one board.'],
  ['Does my data leave my infrastructure?', 'No. AgentDeck is self-hosted by design. The target deployment is one Go binary and PostgreSQL, so your board data and ledger stay where you run them.'],
  ['Can my team use one AgentDeck instance?', 'Yes. Pro is $5/month flat with unlimited teammates, shared boards, role controls, webhooks, and audit history.'],
  ['Is v0.1 ready for production?', 'v0.1 is a public preview of the frontend and product contract. The Go runtime, auth API, dispatcher, ledger persistence, and worker execution are shipping next across the roadmap.'],
]

export type MiniCard = [title: string, agent: string, cost: string]

export type MiniColumn = {
  title: string
  color: string
  cards: MiniCard[]
  approval?: boolean
}

export const miniColumns: MiniColumn[] = [
  { title: 'Running', color: 'var(--status-running)', cards: [['Refactor auth', 'agent-backend', '$0.318'], ['Sync stripe', 'agent-billing', '$0.102']] },
  { title: 'Awaiting approval', color: 'var(--status-awaiting)', cards: [['Drop stale replicas', 'agent-infra', '$0.044'], ['Rotate secrets', 'agent-sec', '$0.012']], approval: true },
  { title: 'Done', color: 'var(--status-done)', cards: [['Audit deps', 'agent-sec', '$0.305'], ['Tag release', 'agent-docs', '$0.008']] },
]

export type Step = [num: string, title: string, desc: string, command: string]

export const steps: Step[] = [
  ['01', 'Point it at Postgres', 'Connect your existing Postgres instance and run migrations.', 'agentdeck migrate'],
  ['02', 'Start the binary', 'Launches the web UI, REST API, and telemetry server.', 'agentdeck serve'],
  ['03', 'Register an agent', 'Connect your first agent worker using your preferred provider.', 'agentdeck agent add --provider openai'],
]

export type Stat = [number: string, label: string]

export const stats: Stat[] = [
  ['109', 'API endpoints'],
  ['21', 'tables, no ORM magic'],
  ['30 MB', 'binary size cap'],
  ['80 MB', 'idle RAM'],
]

export type LedgerRow = [step: string, tokens: string, cost: string]

export const ledgerRows: LedgerRow[] = [
  ['step_01_query', '1,420 in / 184 out', '$0.0028'],
  ['step_02_decompose', '8,940 in / 2,104 out', '$0.0242'],
  ['step_03_codegen', '14,880 in / 4,320 out', '$0.0581'],
]

export type Plan = {
  name: string
  price: string
  period: string
  subtitle: string
  features: string[]
  action: string
  pro?: boolean
}

export const plans: Plan[] = [
  {
    name: 'Solo',
    price: '$0',
    period: '/ forever',
    subtitle: 'For one developer.',
    features: ['Self-host, unlimited agents', 'Cost ledger + approval gates', 'Community support'],
    action: 'Download',
  },
  {
    name: 'Pro',
    price: '$5',
    period: '/ month, flat',
    subtitle: 'For small teams that need shared audit and control.',
    features: ['Everything in Solo', 'Unlimited teammates, no per-seat fee', 'Shared boards + role controls', 'Webhook + audit log', 'Priority support'],
    action: 'Start free trial',
    pro: true,
  },
]

export type Milestone = {
  id: string
  state: string
  title: string
  copy: string
  items: string[]
}

export const milestones: Milestone[] = [
  { id: 'M0', state: 'NEXT', title: 'Identity & workspace', copy: 'Self-serve signup, sessions, personal workspace, tenant isolation, and RBAC.', items: ['Registration + login', 'Personal workspace', 'Owner/admin/member/viewer'] },
  { id: 'M1', state: 'NEXT', title: 'Board & task loop', copy: 'The smallest useful loop: create a board, dispatch a task, and see it reach done.', items: ['Projects + boards', 'Task lifecycle', 'Postgres dispatcher'] },
  { id: 'M2', state: 'PLANNED', title: 'Cost & agent control', copy: 'Know exactly what every run costs and which provider-backed agent spent it.', items: ['Micro-USD ledger', 'Agent registry', 'Budget guardrails'] },
  { id: 'M3', state: 'PLANNED', title: 'Approval & realtime ops', copy: 'Risky actions stop for a human while every state change streams to the UI.', items: ['Approval inbox', 'SSE event stream', 'Run replay trace'] },
  { id: 'M4', state: 'PLANNED', title: 'Reliability & artifacts', copy: 'Recover transient failures and keep the output needed to reproduce a run.', items: ['Failure taxonomy', 'Retry backoff', 'R2 artifacts'] },
  { id: 'M5', state: 'PLANNED', title: 'Governance & integrations', copy: 'Give small teams an audit trail and safe programmatic access to the fleet.', items: ['Audit log', 'API keys', 'Webhooks'] },
  { id: 'M6', state: 'LATER', title: 'Operator quality-of-life', copy: 'Make daily operations faster without adding platform sprawl.', items: ['Cmd+K palette', 'Bulk actions', 'Saved views'] },
]

export const supportCopy: Record<'about' | 'contact' | 'download', [title: string, lead: string]> = {
  about: ['About AgentDeck', 'AgentDeck is a small, self-hosted control plane for teams running AI agent fleets. We focus on the parts that need an audit trail: what ran, what it cost, and what a human approved.'],
  contact: ['Contact', 'For product questions, security reports, or partnership notes, use the channels below. We do not require a sales call to start.'],
  download: ['Download AgentDeck', 'Start with the free Solo tier. The v0.1 binary is designed for one developer, one PostgreSQL connection, and a small VPS.'],
}

export type DocsNavItem = [href: string, label: string]
export type DocsNavGroup = { group: string; items: DocsNavItem[] }

export const docsNav: DocsNavGroup[] = [
  { group: 'Getting started', items: [['/docs/quickstart', 'Quickstart'], ['/docs/architecture', 'Architecture & daemon'], ['/docs/yaml', 'YAML configuration']] },
  { group: 'Orchestration', items: [['/docs/agents', 'Agent registration'], ['/docs/lifecycle', 'Run lifecycle'], ['/docs/approvals', 'Approval gates']] },
  { group: 'API reference', items: [['/docs/api', 'REST endpoints'], ['/docs/webhooks', 'Webhooks & events'], ['/docs/telemetry', 'Telemetry schema'], ['/docs/cli', 'CLI reference'], ['/docs/errors-rbac', 'Errors & RBAC']] },
]

export const docsTableHead = ['Field', 'Type', 'Required', 'Description']

export type TelemetryRow = [name: string, type: string, required: string, description: string]

export const telemetryRows: TelemetryRow[] = [
  ['id', 'integer', 'required', 'Sequential event counter'],
  ['event', 'string', 'required', 'step.started, step.finished, run.completed, run.failed'],
  ['retry', 'integer', 'optional', 'Reconnect delay in milliseconds'],
  ['data', 'object', 'required', 'Structured step or run payload'],
]

export const endpoints = {
  tasks: {
    create: {
      method: 'POST',
      path: '/api/v1/tasks',
      role: 'member',
      description: 'Create a task and send it to the board backlog.',
      code: '{\n  "title": "Refactor auth module",\n  "board_id": "brd_sprint24",\n  "assigned_agent": "agent-backend",\n  "priority": "P0"\n}',
    },
    get: {
      method: 'GET',
      path: '/api/v1/tasks/{id}',
      role: 'viewer',
      description: 'Return the task, current status, assigned agent, and cost summary.',
      code: '{\n  "task_id": "task-771b",\n  "status": "ready",\n  "cost_micros": 2800\n}',
    },
  },
  runs: {
    get: {
      method: 'GET',
      path: '/api/v1/runs/{id}',
      role: 'viewer',
      description: 'Return run status, outcome, steps, and total micro-USD cost.',
      code: '{\n  "run_id": "run-91af",\n  "outcome": "completed",\n  "cost_micros": 58100,\n  "price_version": 1\n}',
    },
  },
}

export const telemetryPayload = '{\n  "run_id": "run-91af",\n  "step_id": "step_03_codegen",\n  "input_tokens": 14880,\n  "output_tokens": 4320,\n  "cost_micros": 58100,\n  "price_version": 1\n}'

export const quickstartSteps = [
  { title: '1. Prerequisites', copy: 'Use PostgreSQL 14+ and download the single binary for your platform.', code: 'curl -sSL https://get.agentdeck.dev/v0.1 | bash' },
  { title: '2. Migrate the database', copy: 'Create the 23 relational tables and the immutable telemetry ledger schema.', code: 'agentdeck migrate --db-url="postgres://postgres:***@localhost:5432/agentdeck"' },
  { title: '3. Start the daemon', copy: 'Start the web UI, REST API, and telemetry server on port 8080.', code: 'agentdeck serve --port=8080 --config=./agentdeck.yaml' },
  { title: '4. Register your first agent', copy: 'Connect a provider-backed worker to the board.', code: 'agentdeck agent add --provider openai --name agent-backend' },
]

export const supportCards = [
  { title: 'Discord community', meta: 'discord.gg/agentdeck', body: 'Real-time discussion for setup, agent workers, approval gates, and telemetry.', action: 'Join Discord', href: 'https://discord.gg/agentdeck' },
  { title: 'GitHub issues', meta: 'github.com/v-reza/agentdeck/issues', body: 'Report bugs, propose changes, and track fixes in the public repository.', action: 'Open issue tracker', href: 'https://github.com/v-reza/agentdeck/issues' },
]

export const aboutPoints = [
  { title: 'Self-hosted', copy: 'Your data remains in infrastructure you control.' },
  { title: 'Auditable', copy: 'Runs, costs, and approvals are explicit records.' },
  { title: 'Small by design', copy: 'One Go binary and PostgreSQL, no platform sprawl.' },
]

export const terminalLines = {
  line1: '$ ./agentdeck --config agentdeck.yaml',
  line2: 'listening on :8080 · 23 tables · 108 routes',
  line3: '[ok] postgres pool 8/8 · migrations up to date',
  lineLines: ['[warn] budget 88% of $300 — approval gate armed'],
}

export type IconProps = { children?: ReactNode }
