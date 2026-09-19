// Final call to action.
import { Link } from '../../lib/router'

function ClosingCta() {
  return (
    <section className="closing-cta">
      <div className="container">
        <h2>Ship the fleet. Keep the receipt.</h2>
        <Link href="/register" className="btn-primary">
          Get started free
        </Link>
        <p>Self-hosted · MIT-licensed core</p>
      </div>
    </section>
  )
}

export { ClosingCta }
