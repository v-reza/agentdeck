// Static page content. Kept out of components so copy edits stay one-line diffs.

export type MiniColumn = {
  title: string
  color: string
  cards: [string, string, string][]
  approval?: boolean
}

const miniColumns = [
  {
    title: 'Running',
    color: 'var(--status-running)',
    cards: [
      ['Refactor auth', 'agent-backend', '$0.318'],
      ['Sync stripe', 'agent-billing', '$0.102'],
    ],
  },
  {
    title: 'Awaiting approval',
    color: 'var(--status-awaiting)',
    cards: [
      ['Drop stale replicas', 'agent-infra', '$0.044'],
      ['Rotate secrets', 'agent-sec', '$0.012'],
    ],
    approval: true,
  },
  {
    title: 'Done',
    color: 'var(--status-done)',
    cards: [
      ['Audit deps', 'agent-sec', '$0.305'],
      ['Tag release', 'agent-docs', '$0.008'],
    ],
  },
]

export type Faq = [question: string, answer: string]

const faqs = [
  ['Do I need Kubernetes?', 'No. One binary and a Postgres connection string.'],
  ['Where does my agent code run?', 'On your machine. AgentDeck schedules and records; it does not execute your code.'],
  ['Can I gate only some tools?', 'Yes, per agent. Set the gate mode to require, auto, or off.'],
  [
    'What happens when I hit a budget cap?',
    'The run stops with outcome budget_exceeded and the task goes back to ready. Nothing is silently dropped.',
  ],
  [
    'Which providers and models can I use?',
    'AgentDeck is designed to stay provider-agnostic. Register the provider-backed worker you already run, then track its runs, tool calls, and cost in one board.',
  ],
  [
    'Does my data leave my infrastructure?',
    'No. AgentDeck is self-hosted by design. The target deployment is one Go binary and PostgreSQL, so your board data and ledger stay where you run them.',
  ],
  [
    'Can my team use one AgentDeck instance?',
    'Yes. Pro is $5/month flat with unlimited teammates, shared boards, role controls, webhooks, and audit history.',
  ],
  [
    'Is v0.1 ready for production?',
    'v0.1 is a public preview of the frontend and product contract. The Go runtime, auth API, dispatcher, ledger persistence, and worker execution are shipping next across the roadmap.',
  ],
]

export type DocsNavGroup = { group: string; items: [string, string][] }

const docsNav = [
  {
    group: 'Getting started',
    items: [
      ['/docs/quickstart', 'Quickstart'],
      ['/docs/architecture', 'Architecture & daemon'],
      ['/docs/yaml', 'YAML configuration'],
    ],
  },
  {
    group: 'Orchestration',
    items: [
      ['/docs/agents', 'Agent registration'],
      ['/docs/lifecycle', 'Run lifecycle'],
      ['/docs/approvals', 'Approval gates'],
    ],
  },
  {
    group: 'API reference',
    items: [
      ['/docs/api', 'REST endpoints'],
      ['/docs/webhooks', 'Webhooks & events'],
      ['/docs/telemetry', 'Telemetry schema'],
      ['/docs/cli', 'CLI reference'],
      ['/docs/errors-rbac', 'Errors & RBAC'],
    ],
  },
]

export type Milestone = { id: string; state: string; title: string; copy: string; items: string[] }

const milestones = [
  {
    id: 'M0',
    state: 'NEXT',
    title: 'Identity & workspace',
    copy: 'Self-serve signup, sessions, personal workspace, tenant isolation, and RBAC.',
    items: ['Registration + login', 'Personal workspace', 'Owner/admin/member/viewer'],
  },
  {
    id: 'M1',
    state: 'NEXT',
    title: 'Board & task loop',
    copy: 'The smallest useful loop: create a board, dispatch a task, and see it reach done.',
    items: ['Projects + boards', 'Task lifecycle', 'Postgres dispatcher'],
  },
  {
    id: 'M2',
    state: 'PLANNED',
    title: 'Cost & agent control',
    copy: 'Know exactly what every run costs and which provider-backed agent spent it.',
    items: ['Micro-USD ledger', 'Agent registry', 'Budget guardrails'],
  },
  {
    id: 'M3',
    state: 'PLANNED',
    title: 'Approval & realtime ops',
    copy: 'Risky actions stop for a human while every state change streams to the UI.',
    items: ['Approval inbox', 'SSE event stream', 'Run replay trace'],
  },
  {
    id: 'M4',
    state: 'PLANNED',
    title: 'Reliability & artifacts',
    copy: 'Recover transient failures and keep the output needed to reproduce a run.',
    items: ['Failure taxonomy', 'Retry backoff', 'R2 artifacts'],
  },
  {
    id: 'M5',
    state: 'PLANNED',
    title: 'Governance & integrations',
    copy: 'Give small teams an audit trail and safe programmatic access to the fleet.',
    items: ['Audit log', 'API keys', 'Webhooks'],
  },
  {
    id: 'M6',
    state: 'LATER',
    title: 'Operator quality-of-life',
    copy: 'Make daily operations faster without adding platform sprawl.',
    items: ['Cmd+K palette', 'Bulk actions', 'Saved views'],
  },
]

export { miniColumns, faqs, docsNav, milestones }
