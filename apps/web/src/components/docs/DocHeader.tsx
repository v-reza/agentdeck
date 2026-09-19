// Breadcrumb, title, and lead paragraph at the top of a docs article.

function DocHeader({
  section,
  title,
  description,
  id,
}: {
  section: string
  title: string
  description: string
  id: string
}) {
  return (
    <>
      <div className="docs-breadcrumb">
        Documentation <span>/</span> {section} <span>/</span> <b>{id}</b>
      </div>
      <h1>{title}</h1>
      <p className="docs-lead">{description}</p>
    </>
  )
}

export { DocHeader }
