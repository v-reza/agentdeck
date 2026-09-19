import { Check } from './Check'

export const terminalLines = [
  { prompt: '$', text: 'agentdeck task create --board core --title "Refactor auth module"', class: 'cmd' },
  { prompt: '›', text: 'dispatched to agent-backend · estimated $0.32', class: 'ok' },
  { prompt: '›', text: 'tool call: github.read → allowed by policy', class: 'dim' },
  { prompt: '!', text: 'tool call: postgres.write → awaiting human approval', class: 'warn' },
  { prompt: '$', text: 'agentdeck ledger balance --workspace production', class: 'cmd' },
  { prompt: '›', text: 'spent today $4.12 of $40.00 monthly cap', class: 'ok' },
]

export function CodeBlock({ code }: { code: string }) {
  return (
    <pre className="code-block">
      <code>{code}</code>
    </pre>
  )
}

export function FeatureList({ items }: { items: string[] }) {
  return (
    <ul className="feature-list">
      {items.map((item) => (
        <li key={item}>
          <Check />
          {item}
        </li>
      ))}
    </ul>
  )
}
