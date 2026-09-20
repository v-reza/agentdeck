// Standalone pricing page.
import { PublicShell } from '../../components/PublicShell'
import { Plan } from '../../components/landing/Plan'

function PricingPage() {
  return (
    <PublicShell active="pricing">
      <div className="public-page centered-page">
        <div className="page-kicker">PUBLIC · NO LOGIN REQUIRED</div>
        <h1>Pricing that stays legible.</h1>
        <p className="page-lead">
          Flat pricing for self-hosted agent orchestration. No per-seat surprise and no sales call required.
        </p>
        <div className="pricing-grid pricing-page-grid">
          <Plan
            name="Solo"
            price="$0"
            period="/ forever"
            subtitle="For one developer."
            features={['Self-host, unlimited agents', 'Cost ledger + approval gates', 'Community support']}
            action="Download"
          />
          <Plan
            name="Pro"
            price="$5"
            period="/ month, flat"
            subtitle="For small teams that need shared audit and control."
            features={[
              'Everything in Solo',
              'Unlimited teammates, no per-seat fee',
              'Shared boards + role controls',
              'Webhook + audit log',
              'Priority support',
            ]}
            action="Start free trial"
            pro
          />
        </div>
        <p className="page-note">Prices in USD. Cancel anytime. Your data stays on your infrastructure.</p>
      </div>
    </PublicShell>
  )
}

export { PricingPage }
