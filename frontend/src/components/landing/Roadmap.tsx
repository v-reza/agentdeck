// M0-M6 milestone track rendered on /github.
import { Check } from '../Check'
import { Link } from 'react-router-dom'
import { milestones } from '../../data/content'

function Roadmap() {
  return (
    <section className="roadmap-section" aria-labelledby="roadmap-title">
      <div className="roadmap-heading">
        <div>
          <div className="page-kicker">BUILD IN PUBLIC</div>
          <h2 id="roadmap-title">From first run to fleet control.</h2>
        </div>
        <Link to="/docs/quickstart" className="text-link">
          Read the product contract →
        </Link>
      </div>
      <p className="roadmap-lead">
        AgentDeck is shipping the operational loop in layers. Each milestone earns its place by making agent work
        cheaper, safer, or easier to replay.
      </p>
      <div className="roadmap-track">
        {milestones.map((milestone) => (
          <article className={`roadmap-milestone ${milestone.current ? 'current' : ''}`} key={milestone.id}>
            <div className="roadmap-node">
              <span>{milestone.id}</span>
            </div>
            <div className="roadmap-card">
              <div className="roadmap-card-top">
                <span className="roadmap-state">{milestone.state}</span>
                <span className="roadmap-version">v0.x</span>
              </div>
              <h3>{milestone.title}</h3>
              <p>{milestone.copy}</p>
              <ul>
                {milestone.items.map((item) => (
                  <li key={item}>
                    <Check />
                    {item}
                  </li>
                ))}
              </ul>
            </div>
          </article>
        ))}
      </div>
      <div className="roadmap-footer">
        <span>
          Current public preview: <b>v0.1</b>
        </span>
        <Link to="/changelog" className="text-link">
          See changelog →
        </Link>
      </div>
    </section>
  )
}

export { Roadmap }
