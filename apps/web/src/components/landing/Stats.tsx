// Deployment footprint figures.

function Stats() {
  return (
    <section className="stats-section">
      <div className="container">
        <div className="stats-grid">
          {[
            ['109', 'API endpoints'],
            ['21', 'tables, no ORM magic'],
            ['30 MB', 'binary size cap'],
            ['80 MB', 'idle RAM'],
          ].map(([number, label]) => (
            <div className="stat-item" key={label}>
              <div className="stat-number">{number}</div>
              <div className="stat-label">{label}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

export { Stats }
