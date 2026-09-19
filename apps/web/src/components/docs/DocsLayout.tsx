// Docs shell: public chrome plus the nav rail and article column.
import type { ReactNode } from 'react'
import { PublicShell } from '../PublicShell'
import { DocsNav } from './DocsNav'

function DocsLayout({ active, children }: { active: string; children: ReactNode }) {
  return (
    <PublicShell active="docs">
      <div className="docs-page">
        <div className="docs-grid">
          <DocsNav active={active} />
          <article className="docs-article">{children}</article>
        </div>
      </div>
    </PublicShell>
  )
}

export { DocsLayout }
