// A titled docs section with supporting copy.
import type { ReactNode } from 'react'

function DocSection({ title, copy, children }: { title: string; copy: string; children?: ReactNode }) {
  return (
    <section className="docs-section">
      <h2>{title}</h2>
      <p>{copy}</p>
      {children}
    </section>
  )
}

export { DocSection }
