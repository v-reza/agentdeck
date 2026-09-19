import type { ReactNode } from 'react'
import { Link } from '../lib/router'

export const FOOTER_COLUMNS: { title: string; links: [label: string, href: string][] }[] = [
  {
    title: 'Product',
    links: [
      ['Features', '/features'],
      ['Pricing', '/pricing'],
      ['Download', '/download'],
      ['Changelog', '/changelog'],
    ],
  },
  {
    title: 'Developers',
    links: [
      ['Quickstart', '/docs/quickstart'],
      ['Architecture', '/docs/architecture'],
      ['REST API', '/docs/api'],
      ['CLI reference', '/docs/cli'],
    ],
  },
  {
    title: 'Company',
    links: [
      ['About', '/about'],
      ['Community', '/community'],
      ['Contact', '/contact'],
      ['GitHub', '/github'],
    ],
  },
]

export function Brand() {
  return <Link href="/" className="brand-wordmark">AgentDeck<span className="brand-mark" /></Link>
}

export function Header({ active = '' }: { active?: string }) {
  return <header className="navbar"><div className="container nav-inner"><Brand /><ul className="nav-links">
    <li><Link href="/features" className={`nav-link${active === 'features' ? ' active' : ''}`}>Product</Link></li>
    <li><Link href="/pricing" className={`nav-link${active === 'pricing' ? ' active' : ''}`}>Pricing</Link></li>
    <li><Link href="/docs/quickstart" className={`nav-link${active === 'docs' ? ' active' : ''}`}>Docs</Link></li>
    <li><Link href="/github" className={`nav-link${active === 'github' ? ' active' : ''}`}>GitHub</Link></li>
  </ul><div className="nav-actions"><Link href="/login" className="btn-ghost">Sign in</Link><Link href="/register" className="btn-primary">Get started</Link></div></div></header>
}

export function FooterColumn({ title, links }: { title: string; links: [label: string, href: string][] }) {
  return <div><div className="footer-title">{title}</div>{links.map(([label, href]) => <Link key={href} href={href} className="footer-link">{label}</Link>)}</div>
}

export function Footer() {
  return <footer className="footer"><div className="container footer-inner"><div className="footer-brand"><Brand /><p>Self-hosted orchestration board for AI agent fleets.</p></div><div className="footer-columns">{FOOTER_COLUMNS.map((column) => <FooterColumn key={column.title} title={column.title} links={column.links} />)}</div></div><div className="footer-bottom"><div className="container"><span>© 2026 AgentDeck. Apache-2.0.</span><span>Self-hosted by design.</span></div></div></footer>
}

export function PublicShell({ children, active }: { children: ReactNode; active?: string }) {
  return <><Header active={active} /><main>{children}</main><Footer /></>
}
