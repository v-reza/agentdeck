// Public site chrome: brand wordmark, top navigation, and footer.
import { Link } from '../lib/router'

const Brand = () => (
  <Link href="/" className="brand-wordmark">
    AgentDeck
    <span className="brand-mark" />
  </Link>
)

function Header({ active = '' }: { active?: string }) {
  return (
    <header className="navbar">
      <div className="container nav-inner">
        <Brand />
        <ul className="nav-links">
          <li>
            <Link href="/features" className={`nav-link${active === 'features' ? ' active' : ''}`}>
              Product
            </Link>
          </li>
          <li>
            <Link href="/pricing" className={`nav-link${active === 'pricing' ? ' active' : ''}`}>
              Pricing
            </Link>
          </li>
          <li>
            <Link href="/docs/quickstart" className={`nav-link${active === 'docs' ? ' active' : ''}`}>
              Docs
            </Link>
          </li>
          <li>
            <Link href="/github" className={`nav-link${active === 'github' ? ' active' : ''}`}>
              GitHub
            </Link>
          </li>
        </ul>
        <div className="nav-actions">
          <Link href="/login" className="btn-ghost">
            Sign in
          </Link>
          <Link href="/register" className="btn-primary">
            Get started
          </Link>
        </div>
      </div>
    </header>
  )
}

function Footer() {
  return (
    <footer className="site-footer">
      <div className="container">
        <div className="footer-top">
          <div>
            <Brand />
            <p>Orchestration board for AI agent fleets.</p>
          </div>
          <FooterColumn
            title="Product"
            links={['Features', 'Pricing', 'Changelog']}
            hrefs={['/features', '/pricing', '/changelog']}
          />
          <FooterColumn
            title="Developers"
            links={['Docs', 'REST API', 'GitHub']}
            hrefs={['/docs/quickstart', '/docs/api', '/github']}
          />
          <FooterColumn
            title="Company"
            links={['About', 'Contact', 'Community']}
            hrefs={['/about', '/contact', '/community']}
          />
        </div>
        <div className="footer-bottom">
          <span>© 2026 AgentDeck</span>
          <span>Built with Go and Postgres.</span>
        </div>
      </div>
    </footer>
  )
}

function FooterColumn({ title, links, hrefs }: { title: string; links: string[]; hrefs: string[] }) {
  return (
    <div>
      <div className="footer-col-title">{title}</div>
      <ul className="footer-links">
        {links.map((link, index) => (
          <li key={link}>
            <Link href={hrefs[index]}>{link}</Link>
          </li>
        ))}
      </ul>
    </div>
  )
}

export { Brand, Header, Footer, FooterColumn }
