// Product page: reuses the landing feature rows with its own header.
import { PublicShell } from '../../components/PublicShell'
import { FeatureSections } from '../../components/landing/FeatureSections'

function FeaturesPage() {
  return (
    <PublicShell active="features">
      <div className="public-page">
        <div className="docs-breadcrumb">
          AgentDeck <span>/</span> <b>Product</b>
        </div>
        <h1>Everything you need to run agents responsibly.</h1>
        <p className="page-lead">
          A board for orchestration, a ledger for cost, and a gate for actions that should not run unattended.
        </p>
        <FeatureSections />
      </div>
    </PublicShell>
  )
}

export { FeaturesPage }
