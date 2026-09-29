/**
 * A minimal markdown renderer for skill bodies (US-AD107 AC2 + AC3).
 *
 * WHY THIS EXISTS INSTEAD OF A LIBRARY: AC3 requires that raw HTML is never
 * executed. The usual answer is `marked` + `dompurify` — two dependencies, and
 * the sanitizer is the load-bearing one. The approach here inverts that: every
 * piece of text is escaped FIRST, and only then is markdown structure applied on
 * top of the escaped string. There is no path from the input to the output that
 * does not pass through `escapeHtml`, so a `<script>` in a skill body is text
 * before anything else can happen to it. That is a property this file can state
 * and a test can pin, which is not true of a sanitizer allow-list.
 *
 * The trade-off is honest: this renders a SUBSET. Headings, fenced code, lists,
 * blockquotes, rules, bold, italic, inline code and links. No tables, no images,
 * no nested lists, no HTML passthrough. Skills are prose instructions for an
 * agent; the subset is what they actually contain. When a skill needs a table,
 * add tables here — do not swap in a library and lose the invariant above.
 *
 * Links are the one place an attribute is built from user text, so the scheme is
 * checked against an allow-list (`http`, `https`, `mailto`, and same-origin
 * paths). `javascript:` and friends render as plain text, not as a dead link.
 */

const ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (char) => ESCAPES[char])
}

/** An href is only kept when its scheme is one we will actually open. */
function safeLink(label: string, rawHref: string): string {
  // Whitespace and control characters are stripped before the check: `java\nscript:`
  // and `  javascript:` are both still `javascript:` to a browser.
  const href = rawHref.replace(/[\u0000-\u0020]/g, '')
  if (!/^(https?:\/\/|mailto:|\/)/i.test(href)) return label
  // `label` and `href` are already escaped, so neither can close the attribute
  // or the element.
  return `<a href="${href}" target="_blank" rel="noopener noreferrer">${label}</a>`
}

/** Inline spans, applied to text that has already been escaped. */
function inline(text: string): string {
  // Code spans first, and their content is escaped but NOT further transformed:
  // a `**` inside backticks is literal, which is the whole point of backticks.
  const parts = text.split(/(`[^`]*`)/g)
  return parts
    .map((part) => {
      if (part.length >= 2 && part.startsWith('`') && part.endsWith('`')) {
        return `<code>${escapeHtml(part.slice(1, -1))}</code>`
      }
      let out = escapeHtml(part)
      out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      out = out.replace(/(^|[\s(])\*([^*\n]+)\*/g, '$1<em>$2</em>')
      out = out.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_match, label: string, href: string) => safeLink(label, href))
      return out
    })
    .join('')
}

const BLOCK_START = /^\s*(#{1,6}\s|[-*]\s|\d+\.\s|>|```|-{3,}\s*$|\*{3,}\s*$)/

export function renderMarkdown(source: string): string {
  const lines = source.replace(/\r\n?/g, '\n').split('\n')
  const out: string[] = []
  let index = 0

  while (index < lines.length) {
    const line = lines[index]

    if (/^\s*```/.test(line)) {
      const body: string[] = []
      index += 1
      while (index < lines.length && !/^\s*```/.test(lines[index])) {
        body.push(lines[index])
        index += 1
      }
      index += 1 // the closing fence, or end of input
      out.push(`<pre><code>${escapeHtml(body.join('\n'))}</code></pre>`)
      continue
    }

    const heading = /^(#{1,6})\s+(.*)$/.exec(line)
    if (heading) {
      const level = heading[1].length
      out.push(`<h${level}>${inline(heading[2].trim())}</h${level}>`)
      index += 1
      continue
    }

    if (/^\s*(-{3,}|\*{3,})\s*$/.test(line)) {
      out.push('<hr />')
      index += 1
      continue
    }

    const bullet = /^\s*[-*]\s+(.*)$/
    if (bullet.test(line)) {
      const items: string[] = []
      while (index < lines.length && bullet.test(lines[index])) {
        items.push(`<li>${inline(bullet.exec(lines[index])![1])}</li>`)
        index += 1
      }
      out.push(`<ul>${items.join('')}</ul>`)
      continue
    }

    const ordered = /^\s*\d+\.\s+(.*)$/
    if (ordered.test(line)) {
      const items: string[] = []
      while (index < lines.length && ordered.test(lines[index])) {
        items.push(`<li>${inline(ordered.exec(lines[index])![1])}</li>`)
        index += 1
      }
      out.push(`<ol>${items.join('')}</ol>`)
      continue
    }

    if (/^\s*>\s?/.test(line)) {
      const quoted: string[] = []
      while (index < lines.length && /^\s*>\s?/.test(lines[index])) {
        quoted.push(lines[index].replace(/^\s*>\s?/, ''))
        index += 1
      }
      out.push(`<blockquote>${inline(quoted.join(' '))}</blockquote>`)
      continue
    }

    if (line.trim() === '') {
      index += 1
      continue
    }

    const paragraph: string[] = []
    while (index < lines.length && lines[index].trim() !== '' && !BLOCK_START.test(lines[index])) {
      paragraph.push(lines[index])
      index += 1
    }
    out.push(`<p>${inline(paragraph.join(' '))}</p>`)
  }

  return out.join('\n')
}
