// About / Contact / Download — one component, three copy variants.
import { Link } from '../lib/router'
import { PublicShell } from '../components/PublicShell'
import { CodeBlock } from '../components/CodeBlock'

function SupportPage({ kind }: { kind: 'about' | 'contact' | 'download' }) {
  const copy = {
    about: [
      'About AgentDeck',
      'AgentDeck is a small, self-hosted control plane for teams running AI agent fleets. We focus on the parts that need an audit trail: what ran, what it cost, and what a human approved.',
    ],
    contact: [
      'Contact',
      'For product questions, security reports, or partnership notes, use the channels below. We do not require a sales call to start.',
    ],
    download: [
      'Download AgentDeck',
      'Start with the free Solo tier. The v0.1 binary is designed for one developer, one PostgreSQL connection, and a small VPS.',
    ],
  }[kind]
  return (
    <PublicShell>
      <div className="public-page narrow-page">
        <div className="docs-breadcrumb">
          AgentDeck <span>/</span> <b>{copy[0]}</b>
        </div>
        <h1>{copy[0]}</h1>
        <p className="page-lead">{copy[1]}</p>
        {kind === 'download' ? (
          <>
            <CodeBlock code={'curl -sSL https://get.agentdeck.dev/v0.1 | bash'} />
            <div className="action-row">
              <Link href="/docs/quickstart" className="btn-primary">
                Read Quickstart
              </Link>
              <Link href="/github" className="btn-outline">
                Review source
              </Link>
            </div>
          </>
        ) : kind === 'contact' ? (
          <div className="contact-list">
            <a href="mailto:hello@agentdeck.dev">hello@agentdeck.dev</a>
            <Link href="/community">Community support →</Link>
            <Link href="/github">Security & source →</Link>
          </div>
        ) : (
          <div className="about-points">
            <div>
              <b>Self-hosted</b>
              <span>Your data remains in infrastructure you control.</span>
            </div>
            <div>
              <b>Auditable</b>
              <span>Runs, costs, and approvals are explicit records.</span>
            </div>
            <div>
              <b>Small by design</b>
              <span>One Go binary and PostgreSQL, no platform sprawl.</span>
            </div>
          </div>
        )}
      </div>
    </PublicShell>
  )
}

export { SupportPage }
