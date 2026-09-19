// Landing-page pricing block.
import { Plan } from './Plan'

function PricingSection() {
  return (
    <section className="pricing-section" id="pricing">
      <div className="container">
        <div className="pricing-heading-wrap">
          <h2 className="section-heading">Priced for one developer, not a procurement team.</h2>
        </div>
        <div className="pricing-grid">
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
        <div className="pricing-footer-note">
          Prices in USD. Cancel anytime. Self-hosted — your data never leaves your infrastructure.
        </div>
      </div>
    </section>
  )
}

export { PricingSection }
