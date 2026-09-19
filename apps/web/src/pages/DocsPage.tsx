// Documentation pages: quickstart, API reference, telemetry schema.
import { Link } from '../lib/router'
import { DocsLayout } from '../components/docs/DocsLayout'
import { DocHeader } from '../components/docs/DocHeader'
import { DocSection } from '../components/docs/DocSection'
import { Endpoint } from '../components/docs/Endpoint'
import { DataTable } from '../components/docs/DataTable'
import { CodeBlock } from '../components/CodeBlock'
import { docsNav } from '../data/content'

function DocsPage({ kind }: { kind: string }) {
  if (kind === '/docs/api')
    return (
      <DocsLayout active={kind}>
        <DocHeader
          section="API reference"
          id="US-AD104"
          title="REST API reference"
          description="The official HTTP API for AgentDeck Core. All routes use /api/v1 and Bearer token authentication."
        />
        <div className="docs-divider" />
        <DocSection title="Tasks" copy="Create, inspect, and control work in the orchestration queue.">
          <Endpoint
            method="POST"
            path="/api/v1/tasks"
            role="member"
            description="Create a task and send it to the board backlog."
            code={
              '{\n  "title": "Refactor auth module",\n  "board_id": "brd_sprint24",\n  "assigned_agent": "agent-backend",\n  "priority": "P0"\n}'
            }
          />
          <Endpoint
            method="GET"
            path="/api/v1/tasks/{id}"
            role="viewer"
            description="Return the task, current status, assigned agent, and cost summary."
            code={'{\n  "task_id": "task-771b",\n  "status": "ready",\n  "cost_micros": 2800\n}'}
          />
        </DocSection>
        <DocSection title="Runs" copy="Observe execution and inspect immutable ledger entries.">
          <Endpoint
            method="GET"
            path="/api/v1/runs/{id}"
            role="viewer"
            description="Return run status, outcome, steps, and total micro-USD cost."
            code={
              '{\n  "run_id": "run-91af",\n  "outcome": "completed",\n  "cost_micros": 58100,\n  "price_version": 1\n}'
            }
          />
        </DocSection>
      </DocsLayout>
    )
  if (kind === '/docs/telemetry')
    return (
      <DocsLayout active={kind}>
        <DocHeader
          section="API reference"
          id="US-AD105"
          title="Run telemetry schema"
          description="Server-Sent Events and immutable step payloads for run monitoring. Costs are integer micro-USD, never floating point."
        />
        <div className="docs-divider" />
        <DocSection
          title="SSE event envelope"
          copy="Subscribe to /api/v1/runs/{id}/events. Each event has a monotonic id for reconnecting with Last-Event-ID."
        >
          <DataTable
            rows={[
              ['id', 'integer', 'required', 'Sequential event counter'],
              ['event', 'string', 'required', 'step.started, step.finished, run.completed, run.failed'],
              ['retry', 'integer', 'optional', 'Reconnect delay in milliseconds'],
              ['data', 'object', 'required', 'Structured step or run payload'],
            ]}
          />
        </DocSection>
        <DocSection
          title="step.finished payload"
          copy="The step payload records tokens, cache usage, and the price snapshot used for the calculation."
        >
          <CodeBlock
            label="JSON"
            code={
              '{\n  "run_id": "run-91af",\n  "step_id": "step_03_codegen",\n  "input_tokens": 14880,\n  "output_tokens": 4320,\n  "cost_micros": 58100,\n  "price_version": 1\n}'
            }
          />
        </DocSection>
      </DocsLayout>
    )
  const title =
    kind === '/docs/quickstart' || kind === '/docs'
      ? 'Quickstart'
      : (docsNav.flatMap((group) => group.items).find(([href]) => href === kind)?.[1] ?? 'Documentation')
  const isQuickstart = title === 'Quickstart'
  return (
    <DocsLayout active={isQuickstart ? '/docs/quickstart' : kind}>
      <DocHeader
        section={isQuickstart ? 'Getting started' : 'Documentation'}
        id={isQuickstart ? 'US-AD103' : 'DRAFT'}
        title={title}
        description={
          isQuickstart
            ? 'Run AgentDeck locally in minutes: one Go binary, one PostgreSQL connection, and a clear audit trail for every agent run.'
            : 'This page is part of the AgentDeck documentation set and is being prepared against the same public contract.'
        }
      />
      <div className="docs-divider" />
      {isQuickstart ? (
        <>
          <DocSection
            title="1. Prerequisites"
            copy="Use PostgreSQL 14+ and download the single binary for your platform."
          >
            <CodeBlock code={'curl -sSL https://get.agentdeck.dev/v0.1 | bash'} />
          </DocSection>
          <DocSection
            title="2. Migrate the database"
            copy="Create the 23 relational tables and the immutable telemetry ledger schema."
          >
            <CodeBlock code={'agentdeck migrate --db-url="postgres://postgres:***@localhost:5432/agentdeck"'} />
          </DocSection>
          <DocSection title="3. Start the daemon" copy="Start the web UI, REST API, and telemetry server on port 8080.">
            <CodeBlock code={'agentdeck serve --port=8080 --config=./agentdeck.yaml'} />
          </DocSection>
          <DocSection title="4. Register your first agent" copy="Connect a provider-backed worker to the board.">
            <CodeBlock code={'agentdeck agent add --provider openai --name agent-backend'} />
          </DocSection>
        </>
      ) : (
        <div className="docs-coming">
          <span>In progress</span>
          <h2>This document is being written.</h2>
          <p>Use the Quickstart, REST API, or Telemetry pages for the currently published contract.</p>
          <Link href="/docs/quickstart" className="btn-primary">
            Back to Quickstart
          </Link>
        </div>
      )}
    </DocsLayout>
  )
}

export { DocsPage }
