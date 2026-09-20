// Hero visual: a miniature fleet board with a pending approval card.
import { miniColumns } from '../../data/content'

function MiniBoard() {
  return (
    <div className="mini-board-frame">
      <div className="mini-board-header">
        <span>Fleet: production-west</span>
        <span>6 active tasks · $1.439 today</span>
      </div>
      <div className="mini-board-columns">
        {miniColumns.map((column) => (
          <div className="mini-col" key={column.title}>
            <div className="mini-col-title">
              <span className="mini-dot" style={{ background: column.color }} />
              {column.title}
            </div>
            {column.cards.map(([title, agent, cost], index) => (
              <div className="mini-card" key={title}>
                <div className="mini-card-title">{title}</div>
                <div className="mini-card-agent">{agent}</div>
                <div className="mini-card-cost">{cost}</div>
                {column.approval && index === 0 && (
                  <div className="mini-btn-group">
                    <button className="mini-btn-approve">Approve</button>
                    <button className="mini-btn-reject">Reject</button>
                  </div>
                )}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

export { MiniBoard }
