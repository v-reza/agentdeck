import { usePathname } from './lib/router'
import { LandingPage } from './pages/LandingPage'
import { FeaturesPage } from './pages/FeaturesPage'
import { PricingPage } from './pages/PricingPage'
import { DocsPage } from './pages/DocsPage'
import { GitHubPage } from './pages/GitHubPage'
import { ChangelogPage } from './pages/ChangelogPage'
import { CommunityPage } from './pages/CommunityPage'
import { SupportPage } from './pages/SupportPage'
import { AuthPage } from './pages/AuthPage'
import { NotFoundPage } from './pages/NotFoundPage'

export default function App() {
  const pathname = usePathname().replace(/\/$/, '') || '/'

  if (pathname === '/') return <LandingPage />
  if (pathname === '/features' || pathname === '/product') return <FeaturesPage />
  if (pathname === '/pricing') return <PricingPage />
  if (pathname === '/github') return <GitHubPage />
  if (pathname === '/community') return <CommunityPage />
  if (pathname === '/changelog') return <ChangelogPage />
  if (pathname === '/about' || pathname === '/contact' || pathname === '/download') {
    return <SupportPage kind={pathname.slice(1) as 'about' | 'contact' | 'download'} />
  }
  if (pathname === '/login' || pathname === '/register') {
    return <AuthPage mode={pathname.slice(1) as 'login' | 'register'} />
  }
  if (pathname === '/docs' || pathname.startsWith('/docs/')) return <DocsPage kind={pathname} />
  return <NotFoundPage />
}
