// Standard public page frame: header, main content, footer.
import type { ReactNode } from 'react'
import { Footer, Header } from './Shell'

function PublicShell({ children, active }: { children: ReactNode; active?: string }) {
  return (
    <>
      <Header active={active} />
      <main>{children}</main>
      <Footer />
    </>
  )
}

export { PublicShell }
