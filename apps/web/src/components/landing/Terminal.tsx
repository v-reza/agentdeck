// Static startup transcript shown under the feature rows.

function Terminal() {
  return (
    <section className="terminal-section">
      <div className="container">
        <div className="terminal-panel">
          <div className="terminal-titlebar">
            <div className="terminal-dots">
              <span />
              <span />
              <span />
            </div>
            <span className="terminal-title">agentdeck — fleet: production-west</span>
          </div>
          <div className="terminal-body">
            <div className="term-line-1">$ ./agentdeck --config agentdeck.yaml</div>
            <div className="term-line-2">listening on :8080 · 23 tables · 109 routes</div>
            <div className="term-line-3">[ok] postgres pool 8/8 · migrations up to date</div>
            <div className="term-line-4">[warn] budget 88% of $300 — approval gate armed</div>
            <div>
              <span className="term-cursor">▌</span>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

export { Terminal }
