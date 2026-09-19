import type { ReactNode } from 'react'
import { Footer, FooterColumn, Header } from './Shell'
import { FOOTER_COLUMNS } from './Shell'

export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <>
      <Header />
      <main>{children}</main>
      <Footer />
    </>
  )
}

export function FooterColumns() {
  return (
    <>
      {FOOTER_COLUMNS.map((column) => (
        <FooterColumn key={column.title} title={column.title} links={column.links} />
      ))}
    </>
  )
}
