// Support/community card with an external action link.

function SupportCard({
  title,
  meta,
  body,
  action,
  href,
}: {
  title: string
  meta: string
  body: string
  action: string
  href: string
}) {
  return (
    <div className="support-card">
      <div>
        <div className="support-card-head">
          <h2>{title}</h2>
          <span>Online</span>
        </div>
        <div className="support-meta">{meta}</div>
        <p>{body}</p>
      </div>
      <a href={href} target="_blank" rel="noreferrer" className="btn-primary">
        {action} ↗
      </a>
    </div>
  )
}

export { SupportCard }
