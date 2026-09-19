import { Route, Routes } from 'react-router-dom'
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
  return (
    <Routes>
      <Route path="/" element={<LandingPage />} />
      <Route path="/features" element={<FeaturesPage />} />
      {/* /product is a legacy alias for the features page. */}
      <Route path="/product" element={<FeaturesPage />} />
      <Route path="/pricing" element={<PricingPage />} />
      <Route path="/github" element={<GitHubPage />} />
      <Route path="/community" element={<CommunityPage />} />
      <Route path="/changelog" element={<ChangelogPage />} />
      <Route path="/about" element={<SupportPage kind="about" />} />
      <Route path="/contact" element={<SupportPage kind="contact" />} />
      <Route path="/download" element={<SupportPage kind="download" />} />
      <Route path="/login" element={<AuthPage mode="login" />} />
      <Route path="/register" element={<AuthPage mode="register" />} />
      <Route path="/docs" element={<DocsPage />} />
      <Route path="/docs/*" element={<DocsPage />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}
