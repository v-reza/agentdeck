// Community and support channels.
import { PublicShell } from '../components/PublicShell'
import { SupportCard } from '../components/SupportCard'

function CommunityPage() {
  return (
    <PublicShell>
      <div className="public-page">
        <div className="docs-breadcrumb">
          AgentDeck <span>/</span> <b>Community</b>
        </div>
        <h1>Community & support</h1>
        <p className="page-lead">Get help, compare deployment patterns, and share what you build with AgentDeck.</p>
        <div className="support-grid">
          <SupportCard
            title="Discord community"
            meta="discord.gg/agentdeck"
            body="Real-time discussion for setup, agent workers, approval gates, and telemetry."
            action="Join Discord"
            href="https://discord.gg/agentdeck"
          />
          <SupportCard
            title="GitHub issues"
            meta="github.com/v-reza/agentdeck/issues"
            body="Report bugs, propose changes, and track fixes in the public repository."
            action="Open issue tracker"
            href="https://github.com/v-reza/agentdeck/issues"
          />
        </div>
        <div className="support-note">
          <b>Support hours:</b> Monday–Friday, 09:00–18:00 WIB. Community replies remain available outside those hours.
        </div>
      </div>
    </PublicShell>
  )
}

export { CommunityPage }
