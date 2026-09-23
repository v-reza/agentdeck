// Breadcrumb, title, and lead paragraph at the top of a docs article.
//
// The breadcrumb used to end in the story id (`/ US-AD104`), carried straight
// from the mockup. That is spec annotation burned into a static page: a reader
// on the public docs has no idea what the ticket number means, and the contract
// forbids it in rendered UI. The section alone says where they are.

function DocHeader({ section, title, description }: { section: string; title: string; description: string }) {
  return (
    <>
      <div className="docs-breadcrumb">
        Documentation <span>/</span> {section}
      </div>
      <h1>{title}</h1>
      <p className="docs-lead">{description}</p>
    </>
  )
}

export { DocHeader }
