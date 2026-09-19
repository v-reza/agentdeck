// Left rail navigation for the docs pages.
import { Link } from '../../lib/router'
import { docsNav } from '../../data/content'

function DocsNav({ active }: { active: string }) {
  return (
    <aside className="docs-nav">
      {docsNav.map((section) => (
        <div key={section.group}>
          <div className="docs-nav-title">{section.group}</div>
          <ul>
            {section.items.map(([href, label]) => (
              <li key={href}>
                <Link href={href} className={active === href ? 'active' : ''}>
                  {label}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </aside>
  )
}

export { DocsNav }
