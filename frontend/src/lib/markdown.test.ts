import { describe, expect, it } from 'vitest'
import { renderMarkdown, escapeHtml } from '@/lib/markdown'

/**
 * US-AD107 AC2 (markdown previewable) and AC3 (raw HTML is never executed).
 *
 * AC3 is the reason this is a unit test and not an e2e assertion. A browser test
 * can only observe that a script did NOT run, which is indistinguishable from a
 * script that ran and did nothing visible — the negative is unprovable from the
 * outside. Here the property is stated directly on the output: nothing from the
 * input reaches the result as markup unless this file put it there.
 *
 * The XSS cases are the ones that actually get past naive renderers:
 *   - a raw tag in a paragraph
 *   - a tag smuggled inside a heading, a list item and a code span
 *   - an `onerror`-style attribute in an inline HTML fragment
 *   - a `javascript:` link, including the whitespace/newline-obfuscated forms
 *   - an attribute-breaking quote in a link label
 */

describe('markdown rendering escapes before it formats', () => {
  /**
   * The renderer is allowed to emit exactly these tags. Anything else in the
   * output is a tag the INPUT produced, which is the failure AC3 is about.
   *
   * Written this way rather than as a list of banned substrings on purpose: a
   * banned-substring check passes on `onerror=` appearing as escaped TEXT (which
   * is safe and correct) and fails to notice a tag this list has never heard of.
   * Stripping the allowed tags and looking for a leftover `<` is the property
   * itself, so it holds for inputs nobody thought to enumerate.
   */
  const ALLOWED = /<\/?(p|h[1-6]|ul|ol|li|blockquote|pre|code|strong|em|hr|a)(\s[^>]*)?\/?>/gi

  it('never emits a tag the input produced', () => {
    const hostile = [
      '<script>alert(1)</script>',
      '# <img src=x onerror=alert(1)>',
      '- <svg/onload=alert(1)>',
      '> <iframe src="javascript:alert(1)"></iframe>',
      '`<script>alert(1)</script>`',
      'plain <div class="x">nested</div>',
    ]
    for (const source of hostile) {
      const html = renderMarkdown(source)
      const leftover = html.replace(ALLOWED, '')
      expect(leftover, `${source} -> ${leftover}`).not.toContain('<')
      // The text is still shown, just not as markup.
      expect(html, source).toContain('&lt;')
    }
  })

  it('refuses a javascript: link, including the obfuscated forms', () => {
    for (const href of ['javascript:alert(1)', 'java\nscript:alert(1)', '  javascript:alert(1)']) {
      const html = renderMarkdown(`[click](${href})`)
      expect(html, href).not.toContain('href=')
      // The label survives as text rather than becoming a dead link.
      expect(html, href).toContain('click')
    }
  })

  it('keeps a link whose scheme is allowed, with the attributes it adds itself', () => {
    const html = renderMarkdown('[docs](https://example.com/a?b=1)')
    expect(html).toContain('href="https://example.com/a?b=1"')
    expect(html).toContain('rel="noopener noreferrer"')
  })

  it('cannot be broken out of via a quote in the label', () => {
    const html = renderMarkdown('["><img src=x>](https://example.com)')
    expect(html).not.toContain('<img')
    expect(html).toContain('&quot;')
  })

  it('renders the structure skills actually use', () => {
    const html = renderMarkdown(
      [
        '# Title',
        '',
        'Some **bold** and *italic* and `code`.',
        '',
        '- one',
        '- two',
        '',
        '1. first',
        '',
        '> note',
        '',
        '---',
        '',
        '```',
        'raw <b>text</b>',
        '```',
      ].join('\n'),
    )
    expect(html).toContain('<h1>Title</h1>')
    expect(html).toContain('<strong>bold</strong>')
    expect(html).toContain('<em>italic</em>')
    expect(html).toContain('<code>code</code>')
    expect(html).toContain('<ul><li>one</li><li>two</li></ul>')
    expect(html).toContain('<ol><li>first</li></ol>')
    expect(html).toContain('<blockquote>note</blockquote>')
    expect(html).toContain('<hr />')
    // Inside a fence the markup is shown literally — the one place a tag is
    // expected in the output, and it is escaped, not live.
    expect(html).toContain('<pre><code>raw &lt;b&gt;text&lt;/b&gt;</code></pre>')
  })

  it('escapeHtml covers the five characters that matter in an attribute or a text node', () => {
    expect(escapeHtml(`&<>"'`)).toBe('&amp;&lt;&gt;&quot;&#39;')
  })
})
