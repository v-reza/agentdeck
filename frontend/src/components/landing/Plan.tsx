// Single pricing card. Shared by the landing page and /pricing.
import { Check } from '../Check'
import { Link } from 'react-router-dom'

function Plan({
  name,
  price,
  period,
  subtitle,
  features,
  action,
  pro = false,
}: {
  name: string
  price: string
  period: string
  subtitle: string
  features: string[]
  action: string
  pro?: boolean
}) {
  return (
    <div className={`pricing-card${pro ? ' pro' : ''}`}>
      {pro && <div className="popular-badge">Most popular</div>}
      <div>
        <div className="plan-name">{name}</div>
        <div className="plan-price-row">
          <div className="plan-price">{price}</div>
          <div className="plan-period">{period}</div>
        </div>
        <div className="plan-subtitle">{subtitle}</div>
        <ul className="plan-features">
          {features.map((feature) => (
            <li className="plan-feature-item" key={feature}>
              <Check />
              {feature}
            </li>
          ))}
        </ul>
      </div>
      <Link to="/register" className={pro ? 'btn-primary full' : 'btn-outline full'}>
        {action}
      </Link>
    </div>
  )
}

export { Plan }
