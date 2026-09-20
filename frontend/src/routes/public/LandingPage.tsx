// Marketing landing page.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { PublicShell } from '../../components/PublicShell'
import { faqs } from '../../data/content'
import { MiniBoard } from '../../components/landing/MiniBoard'
import { FeatureSections } from '../../components/landing/FeatureSections'
import { Terminal } from '../../components/landing/Terminal'
import { Steps } from '../../components/landing/Steps'
import { Stats } from '../../components/landing/Stats'
import { PricingSection } from '../../components/landing/PricingSection'
import { ClosingCta } from '../../components/landing/ClosingCta'

function LandingPage() {
  const [openFaq, setOpenFaq] = useState(-1)
  return (
    <PublicShell>
      <section className="hero-section">
        <div className="container hero-grid">
          <div>
            <div className="hero-badge">v0.1 · self-hosted · single binary</div>
            <h1 className="hero-title">Your agents run all night. You should know what they cost.</h1>
            <p className="hero-desc">
              AgentDeck is the orchestration board for AI agent fleets. Every run priced to the micro-cent, every risky
              action gated behind a human approval, all in one Go binary you host yourself.
            </p>
            <div className="hero-actions">
              <Link to="/register" className="btn-primary">
                Get started free
              </Link>
              <Link to="/docs/quickstart" className="btn-outline">
                Read the docs <span aria-hidden="true">→</span>
              </Link>
            </div>
            <div className="hero-meta">No credit card · Postgres + one binary · ~80 MB idle RAM</div>
          </div>
          <MiniBoard />
        </div>
      </section>
      <section className="product-strip" id="product">
        <div className="container product-strip-inner">
          <div>
            <strong>$0.00</strong>
            <span>what you actually spent</span>
          </div>
          <div>
            <strong>?</strong>
            <span>what your agent just did</span>
          </div>
          <div>
            <strong>3 a.m.</strong>
            <span>when you find out</span>
          </div>
        </div>
      </section>
      <FeatureSections />
      <Terminal />
      <Steps />
      <Stats />
      <PricingSection />
      <section className="faq-section">
        <div className="container">
          <h2 className="section-heading">Frequently asked questions</h2>
          <div className="faq-list">
            {faqs.map(([question, answer], index) => (
              <details
                className="faq-item"
                key={question}
                open={openFaq === index}
                onClick={(event) => {
                  event.preventDefault()
                  setOpenFaq(openFaq === index ? -1 : index)
                }}
              >
                <summary>{question}</summary>
                <p>{answer}</p>
              </details>
            ))}
          </div>
        </div>
      </section>
      <ClosingCta />
    </PublicShell>
  )
}

export { LandingPage }
