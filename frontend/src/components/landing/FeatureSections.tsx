// Three product claims, each with its own inline visual.

function FeatureSections() {
  return (
    <section className="features-section">
      <div className="container">
        <h2 className="section-heading">Built for the parts nobody demos.</h2>
        <div className="feature-row">
          <div className="feature-text">
            <h3>Every run is priced. Per step.</h3>
            <p>
              Token in, token out, cache read, cache write — written to a ledger row the moment the step finishes.
              Prices live in a versioned snapshot, so an old run still costs what it cost.
            </p>
          </div>
          <div className="feature-visual">
            <div className="ledger-table-box">
              <table className="ledger-table">
                <thead>
                  <tr>
                    <th>Step</th>
                    <th>Tokens</th>
                    <th className="align-right">Cost</th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    ['step_01_query', '1,420 in / 184 out', '$0.0028'],
                    ['step_02_decompose', '8,940 in / 2,104 out', '$0.0242'],
                    ['step_03_codegen', '14,880 in / 4,320 out', '$0.0581'],
                  ].map(([step, tokens, cost]) => (
                    <tr key={step}>
                      <td className="col-step">{step}</td>
                      <td>{tokens}</td>
                      <td className="col-cost">{cost}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
        <div className="feature-row">
          <div className="feature-text">
            <h3>Risky actions stop and wait for you.</h3>
            <p>
              Gate any tool behind approval. The agent pauses in <code>awaiting_approval</code>, you get a diff of
              exactly what it wants to do, and it only continues when you say so.
            </p>
          </div>
          <div className="feature-visual">
            <div className="approval-box">
              <div className="approval-header">
                <span className="approval-dot" />
                agent-backend wants to run
              </div>
              <div className="approval-code-block">
                EXEC tool="migrate_dns" zone="prod.internal" ttl=60
                <br />
                REPLACE 10.0.1.4 -&gt; 10.0.2.18 ttl=300
              </div>
              <div className="approval-actions">
                <button className="btn-primary small">Approve</button>
                <button className="btn-outline small">Reject</button>
              </div>
            </div>
          </div>
        </div>
        <div className="feature-row">
          <div className="feature-text">
            <h3>Cheap to run, because it does less.</h3>
            <p>
              One Go binary, one Postgres. No Redis, no Kafka, no queue service, no Kubernetes. Idles under 80 MB and
              fits in a $6 VPS.
            </p>
          </div>
          <div className="feature-visual">
            <div className="arch-spec-box">
              <div className="terminal-line">$ ./agentdeck --config agentdeck.yaml</div>
              <div className="terminal-line muted">listening on :8080 · 23 tables · 109 routes</div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

export { FeatureSections }
