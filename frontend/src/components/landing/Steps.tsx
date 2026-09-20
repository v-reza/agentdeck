// Three-command quickstart strip.

function Steps() {
  return (
    <section className="steps-section">
      <div className="container">
        <h2 className="section-heading">Three commands to a running fleet.</h2>
        <div className="steps-grid">
          {[
            [
              '01',
              'Point it at Postgres',
              'Connect your existing Postgres instance and run migrations.',
              'agentdeck migrate',
            ],
            ['02', 'Start the binary', 'Launches the web UI, REST API, and telemetry server.', 'agentdeck serve'],
            [
              '03',
              'Register an agent',
              'Connect your first agent worker using your preferred provider.',
              'agentdeck agent add --provider openai',
            ],
          ].map(([num, title, desc, command]) => (
            <div className="step-card" key={num}>
              <div>
                <div className="step-num">{num}</div>
                <div className="step-title">{title}</div>
                <div className="step-desc">{desc}</div>
              </div>
              <div className="step-code">{command}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

export { Steps }
