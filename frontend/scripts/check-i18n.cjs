#!/usr/bin/env node
/**
 * US-AD50 — reject hardcoded UI text in components.
 *
 * The PRD names the tool (`check-i18n.js`) and the rule (a string literal longer
 * than 3 characters outside an i18n wrapper), and it assumes the wrapper is
 * `<Trans>` or `t()`. This repo's i18n is not shaped that way: there is no `t()`
 * function and no `<Trans>` anywhere in `src/`. Components read a dictionary —
 * `const t = useT()` then `t['security.title']` — and the error boundary calls a
 * plain `translate()` because `useT` suspends and cannot be called from a class
 * component. So the wrappers recognised here are the ones this codebase actually
 * has, plus the two the PRD names, because dropping them would make a future
 * `<Trans>` a false positive:
 *
 *   t['some.key']                 element access on `t`, including chains
 *   t('some.key')                 the PRD's call form
 *   translate('some.key')         the synchronous helper
 *   <Trans>text</Trans>           the PRD's component form
 *
 * Two kinds of text are reported:
 *   string     a string literal outside a wrapper
 *   jsx-text   the literal text between tags, `<h1>Recent runs</h1>`
 *
 * The second kind is not strictly a "string literal" in the AC's wording, but it
 * is what "komponen yang berisi string teks tanpa wrapper i18n" means, and a check
 * that missed it would pass the app's most common hardcoded-text bug.
 *
 * Implementation: the TypeScript lexer (`createScanner`), not a regex and not the
 * TS AST. Two reasons.
 *
 * 1. A regex over `.tsx` cannot tell a string from a word in a comment, and a
 *    check that reports comments is a check people learn to ignore.
 * 2. This repo is on TypeScript 7, which dropped the JavaScript parser entirely —
 *    `typescript` here exports a version stub, and `typescript/unstable/ast` has
 *    no `createSourceFile`. The lexer is the real one and is still exported, so
 *    the tokens (and therefore strings and comments) are exact.
 *
 * What the lexer does not give is the parse tree, so JSX context is tracked by
 * hand: tag state, brace depth, and an element stack. That is a heuristic, and it
 * is bounded by the self-test below rather than by hope.
 *
 * Usage:
 *   node scripts/check-i18n.cjs               scan src/, exit 1 on any finding
 *   node scripts/check-i18n.cjs --self-test   run the fixtures, exit 1 on failure
 *   node scripts/check-i18n.cjs --list        print every finding, do not fail
 */

const fs = require('node:fs')
const path = require('node:path')

const ROOT = path.resolve(__dirname, '..')
const BASELINE = path.join(ROOT, 'scripts', 'i18n-baseline.json')
const SRC = path.join(ROOT, 'src')

/** Attributes whose value is never user-visible prose. */
const NON_PROSE_ATTRIBUTES = new Set([
  'className',
  'class',
  'id',
  'name',
  'type',
  'role',
  'href',
  'src',
  'to',
  'path',
  'd',
  'viewBox',
  'fill',
  'stroke',
  'strokeWidth',
  'strokeLinecap',
  'width',
  'height',
  'xmlns',
  'rel',
  'target',
  'method',
  'autoComplete',
  'inputMode',
  'key',
  'tabIndex',
  'aria-hidden',
  'aria-live',
  'aria-modal',
  'scope',
  'lang',
  // Design-token props: their values name a variant, never a sentence.
  'variant',
  'size',
  'tone',
  'align',
  'justify',
  'orientation',
  'as',
  'mode',
  'status',
  'kind',
  'color',
  'scheme',
  'layout',
  'position',
  'direction',
  // SVG presentation attributes. Their values are geometry and paint, never prose.
  'strokeLinejoin',
  'strokeLinecap',
  'strokeDasharray',
  'strokeDashoffset',
  'strokeMiterlimit',
  'strokeOpacity',
  'fillOpacity',
  'fillRule',
  'clipRule',
  'clipPath',
  'preserveAspectRatio',
  'gradientUnits',
  'stopColor',
  'stopOpacity',
  'patternUnits',
  'points',
  'transform',
  'opacity',
  'offset',
  'x1',
  'y1',
  'x2',
  'y2',
  'cx',
  'cy',
  'r',
  'rx',
  'ry',
  'fx',
  'fy',
  'dx',
  'dy',
  'rotate',
  'textAnchor',
  'dominantBaseline',
  'fontSize',
  'fontWeight',
  'letterSpacing',
])

/** `data-*` and `aria-*` carry machine values and identifiers, never prose. */
function isNonProseAttribute(name) {
  return NON_PROSE_ATTRIBUTES.has(name) || name.startsWith('data-') || name.startsWith('aria-')
}

/**
 * A literal is a finding when it is longer than 3 characters, carries at least
 * one letter, and is not purely digits/punctuation/symbols. `{n} of {m}`, `—`,
 * and `1.2s` are all correctly ignored; `Recent runs` is not.
 */
function isFinding(text) {
  const trimmed = text.trim()
  if (trimmed.length <= 3) return false
  if (!/\p{L}/u.test(trimmed)) return false
  if (/^[\s\d\p{P}\p{S}]*$/u.test(trimmed)) return false
  // An interpolation marker — `{count}`, `{shown}` — is substituted before
  // render and is never shown literally, so it is not hardcoded UI text.
  if (/^(\{[a-zA-Z_][a-zA-Z0-9_]*\})+$/.test(trimmed)) return false
  return true
}

/** i18n helpers that take a key as their first argument. */
const TRANSLATE_CALLEES = new Set(['t', 'translate', 'tPlural'])
/** Components that wrap translated text. */
const TRANSLATE_COMPONENTS = new Set(['Trans'])

/**
 * Index just past the template literal that starts at `start` (a backtick).
 *
 * The TypeScript lexer needs the caller to re-scan inside a template: after it
 * returns `TemplateHead` the next `scan()` continues INSIDE the literal, so a
 * naive token loop produced tokens that began at a `}` and ran to the end of the
 * file. Those garbage spans were reported as findings and drowned the real ones.
 * Skipping the literal by hand and resetting the scanner past it keeps every
 * later token honest.
 */
function endOfTemplate(src, start) {
  let i = start + 1
  while (i < src.length) {
    const ch = src[i]
    if (ch === '\\') {
      i += 2
      continue
    }
    if (ch === '`') return i + 1
    if (ch === '$' && src[i + 1] === '{') {
      let depth = 1
      i += 2
      while (i < src.length && depth > 0) {
        const c = src[i]
        if (c === '\\') {
          i += 2
          continue
        }
        if (c === '`') {
          i = endOfTemplate(src, i)
          continue
        }
        if (c === '{') depth++
        else if (c === '}') depth--
        i++
      }
      continue
    }
    i++
  }
  return src.length
}

function analyse(ts, relFile, text) {
  const SyntaxKind = ts.SyntaxKind
  const scanner = ts.createScanner(true /* skipTrivia */, 1 /* JSX variant */, text)

  // ---- tokens -------------------------------------------------------------
  const tokens = []
  // TS7 names this token `EndOfFile`; `EndOfFileToken` is undefined, and an
  // undefined terminator turns this loop into an out-of-memory hang. The token
  // count guard is a second line of defence: the loop can never run forever on a
  // terminator this build renames.
  const EOF = SyntaxKind.EndOfFile ?? SyntaxKind.EndOfFileToken
  const maxTokens = text.length + 16
  for (let token = scanner.scan(); token !== EOF && tokens.length < maxTokens; token = scanner.scan()) {
    tokens.push({ kind: token, start: scanner.getTokenStart(), end: scanner.getTokenEnd() })
  }

  // ---- line/column --------------------------------------------------------
  const lineStarts = [0]
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '\n') lineStarts.push(i + 1)
  }
  function positionOf(offset) {
    let lo = 0
    let hi = lineStarts.length - 1
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1
      if (lineStarts[mid] <= offset) lo = mid
      else hi = mid - 1
    }
    return { line: lo + 1, column: offset - lineStarts[lo] + 1 }
  }

  const kindName = (kind) => SyntaxKind[kind] || String(kind)

  // The lexer reports `{` in JSX as `FirstPunctuation` (the same kind it uses for
  // `(`), not as `OpenBraceToken`. Classifying braces by their TEXT rather than by
  // their kind is what makes expression containers visible at all — matching on
  // `OpenBraceToken` silently skipped every `{...}` in a `.tsx` file.
  const isOpenBrace = (tk) => text.slice(tk.start, tk.end) === '{'
  const isCloseBrace = (tk) => text.slice(tk.start, tk.end) === '}'

  /**
   * The attribute name ending just before `offset`.
   *
   * Read from the source rather than from the token before it: the lexer splits
   * `data-testid` into `data`, `-`, `testid`, so the previous token is only the
   * tail of the name and every `data-*` / `aria-*` attribute would be classified
   * by the wrong name.
   */
  /**
   * Is this literal a module path rather than UI text?
   *
   * `import x from 'react-router-dom'`, `import('../../routes/foo')` and
   * `require('typescript')` are all string literals, none of them are rendered,
   * and a check that reports them buries the real findings in noise.
   */
  function isModuleSpecifier(i) {
    const prev = tokens[i - 1]
    if (!prev) return false
    if (prev.kind === SyntaxKind.FromKeyword) return true
    if (prev.kind === SyntaxKind.OpenParenToken) {
      const callee = tokens[i - 2]
      if (!callee) return false
      const name = text.slice(callee.start, callee.end)
      // Module paths only. The CSS-class helpers used to be listed here too,
      // but `insideClassHelper` already covers cn/clsx/classNames for the WHOLE
      // call, including arguments behind `&&`; the copy here only ever saw the
      // first argument. Deleting it changed no output and no fixture — a
      // redundant branch that a mutation could not kill because the surviving
      // path was doing the work.
      return name === 'import' || name === 'require'
    }
    return false
  }

  /**
   * Is this literal inside a `cn(...)` / `clsx(...)` / `classNames(...)` call?
   *
   * The direct-argument check in `isModuleSpecifier` only covers the first
   * argument. `cn('flex gap-2', ok && 'text-red')` puts a class list behind `&&`
   * as well, so the whole call has to be recognised.
   */
  function insideClassHelper(i) {
    let depth = 0
    for (let k = i - 1; k >= 0 && k > i - 400; k--) {
      const tk = tokens[k]
      if (tk.kind === SyntaxKind.CloseParenToken) depth++
      else if (tk.kind === SyntaxKind.OpenParenToken) {
        if (depth > 0) {
          depth--
          continue
        }
        const callee = tokens[k - 1]
        if (isIdentifier(callee)) {
          const name = text.slice(callee.start, callee.end)
          if (name === 'cn' || name === 'clsx' || name === 'classNames') return true
        }
        return false
      }
    }
    return false
  }

  function attributeNameBefore(offset) {
    let i = offset - 1
    while (i >= 0 && /[A-Za-z0-9_$-]/.test(text[i])) i--
    return text.slice(i + 1, offset)
  }
  const isIdentifier = (t) => t && t.kind === SyntaxKind.Identifier
  // Attribute names are often reserved words — `type`, `default`, `for`, `in`.
  // The lexer reports those as keyword kinds, so an element written
  // `<input type="date" />` was rejected as "not a tag" and its name was then
  // reported as JSX text.
  const isIdentifierLike = (t) =>
    isIdentifier(t) || (t && t.kind >= SyntaxKind.FirstKeyword && t.kind <= SyntaxKind.LastKeyword)
  const isStringToken = (t) =>
    t && (t.kind === SyntaxKind.StringLiteral || t.kind === SyntaxKind.NoSubstitutionTemplateLiteral)

  /** Unquoted value of a string token. */
  function stringValue(t) {
    const raw = text.slice(t.start, t.end)
    return raw.slice(1, -1)
  }

  /**
   * Is the string literal at index `i` an i18n key rather than UI text?
   *
   * `t['key']` and `t['a']['b']` are recognised by walking back through the
   * bracket chain; `t('key')` / `translate('key')` by the call form. Both are
   * shape checks on the token stream, not on the AST.
   */
  function isWrappedString(i) {
    const prev = tokens[i - 1]
    if (!prev) return false

    // translate('key') / t('key')
    if (prev.kind === SyntaxKind.OpenParenToken) {
      const callee = tokens[i - 2]
      if (isIdentifier(callee) && TRANSLATE_CALLEES.has(text.slice(callee.start, callee.end))) return true
      return false
    }

    // t['key'] and t['a']['b'] — walk back over `[` 'literal' `]` groups.
    if (prev.kind === SyntaxKind.OpenBracketToken) {
      let k = i - 1
      for (let guard = 0; guard < 16; guard++) {
        if (!tokens[k] || tokens[k].kind !== SyntaxKind.OpenBracketToken) return false
        k--
        if (isIdentifier(tokens[k])) {
          return text.slice(tokens[k].start, tokens[k].end) === 't'
        }
        if (!isStringToken(tokens[k])) return false
        k--
        if (!tokens[k] || tokens[k].kind !== SyntaxKind.CloseBracketToken) return false
        k--
      }
    }
    return false
  }

  // ---- JSX context walk ---------------------------------------------------
  // `inTag`   : between `<` and the closing `>` of a tag, attributes included
  // `brace`   : depth of `{ }`, which in JSX children holds expressions
  // `elements`: open element names, so `<Trans>` can be recognised
  let inTag = false
  let closingTag = false
  let tagName = null
  // A `>` inside an attribute expression is a comparison, not the end of the tag:
  // `subtitle={sessions.length > 0 ? ... }`. The tag only closes at the brace
  // depth it was opened at.
  let tagOpenBrace = 0
  // `brace` is the code brace depth, and it is NOT zero inside a component: a
  // function body is a brace. `elements` therefore records the depth each element
  // was opened at, so "am I in JSX children" is a comparison rather than `=== 0`.
  // Gating the tag detection on `brace === 0` is what made this scan report
  // nothing in real files while every fixture passed — the fixtures had no body.
  let brace = 0
  const elements = []
  const findings = []

  function report(offset, kind, value) {
    const { line, column } = positionOf(offset)
    findings.push({ file: relFile, line, column, kind, text: value })
  }

  /** Is the innermost open element a translation wrapper? */
  function insideTrans() {
    return elements.length > 0 && TRANSLATE_COMPONENTS.has(elements[elements.length - 1].name)
  }

  /**
   * Is a `<` at index `i` the start of a JSX tag?
   *
   * `< 2` is arithmetic, `<h1>` and `<>` are tags. The discriminator is what
   * follows: an identifier followed by `>`, `/`, or another identifier
   * (an attribute), or a `>` directly (a fragment).
   */
  function isTagStart(i) {
    // A JSX element can only follow one of these. Without this check,
    // `VariantProps<typeof x>` and `Array<string>` read as elements, and every
    // generic type argument becomes a false positive.
    const before = tokens[i - 1]
    if (before) {
      const legal = new Set([
        SyntaxKind.OpenParenToken,
        SyntaxKind.OpenBraceToken,
        SyntaxKind.OpenBracketToken,
        SyntaxKind.CommaToken,
        SyntaxKind.ColonToken,
        SyntaxKind.QuestionToken,
        SyntaxKind.FirstAssignment,
        SyntaxKind.EqualsGreaterThanToken,
        SyntaxKind.SemicolonToken,
        SyntaxKind.ReturnKeyword,
        SyntaxKind.AmpersandAmpersandToken,
        SyntaxKind.BarBarToken,
        SyntaxKind.CloseParenToken,
        // `>` closes a tag, and the very next `<` is its child element. Without
        // this, every nested element read as text and its tag name was reported.
        SyntaxKind.GreaterThanToken,
        // `}` ends an expression container, so `{t['x.y']}<input ... />` — a
        // translated label followed by a control — is an element, not text.
        SyntaxKind.CloseBraceToken,
        SyntaxKind.CloseBracketToken,
      ])
      // Inside JSX children a `<` is always a tag: text and elements live there,
      // generic type arguments do not. `<AgentDeck <span>` was the case that made
      // this necessary — the text run before it ended in an Identifier.
      const inChildren = elements.length > 0 && !inTag && brace === elements[elements.length - 1].brace
      if (!legal.has(before.kind) && !inChildren) return false
    }
    const next = tokens[i + 1]
    if (!next) return false
    if (next.kind === SyntaxKind.GreaterThanToken) return true
    if (!isIdentifierLike(next)) return false
    const after = tokens[i + 2]
    if (!after) return false
    return (
      after.kind === SyntaxKind.GreaterThanToken ||
      after.kind === SyntaxKind.SlashToken ||
      isIdentifierLike(after) ||
      after.kind === SyntaxKind.FirstAssignment
    )
  }

  let textRunStart = null
  let textRunEnd = null

  function flushTextRun() {
    if (textRunStart === null) return
    const raw = text.slice(textRunStart, textRunEnd)
    const value = raw.replace(/\s+/g, ' ').trim()
    if (isFinding(value) && !insideTrans()) {
      report(textRunStart, 'jsx-text', value)
    }
    textRunStart = null
    textRunEnd = null
  }

  const TAG_START = new Set([
    SyntaxKind.FirstBinaryOperator, // `<` in a JSX-ish position
    SyntaxKind.LessThanToken,
  ])

  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i]
    const kind = t.kind

    // --- string literals, in children and in tags alike.
    //
    // Checked first, before the tag handling `continue`s: a string literal that is
    // the value of a JSX attribute is inside a tag, and the tag branch would skip
    // it. There is no `brace === 0` guard on purpose — `{'Recent runs'}` is an
    // expression container in children, not a code block, and it is exactly the
    // shape the AC is about.
    // Only inside JSX. A string literal in a component's own code is usually not
    // UI text — `'task.created'` is a webhook event kind, `'Escape'` is a DOM key,
    // `'relative'` is a CSS class — and a lexer cannot tell those from prose. The
    // AC is about text the component RENDERS, so that is what is scanned.
    const insideJsx = inTag || elements.length > 0
    if (
      insideJsx &&
      isStringToken(t) &&
      !insideTrans() &&
      !isWrappedString(i) &&
      !isModuleSpecifier(i) &&
      !insideClassHelper(i)
    ) {
      const value = stringValue(t)
      if (isFinding(value)) {
        // A literal that is an attribute's value is reported under the attribute's
        // name, and skipped when that name is not prose (`className`, `data-*`).
        // Without this, `data-testid="recent-runs"` reads as UI text.
        const eq = tokens[i - 1]
        const attr = eq && eq.kind === SyntaxKind.FirstAssignment ? attributeNameBefore(eq.start) : ''
        if (attr) {
          if (!isNonProseAttribute(attr)) report(t.start, `attr:${attr}`, value)
        } else {
          report(t.start, 'string', value)
        }
      }
    }
    const inChildren = elements.length > 0 && !inTag && brace === elements[elements.length - 1].brace
    // A token immediately followed by `{` opens an expression container, so it is
    // code (`{count}`), not literal text. Without this, `count` reads as prose.
    const opensExpression = tokens[i + 1] && isOpenBrace(tokens[i + 1])
    // A token followed by `=` is an attribute name (`<polyline points="0,0">`), so
    // it is not text and it ends any text run in progress. Without this,
    // `points` and `className` got glued onto the tag name and reported as prose.
    const isAttributeName = tokens[i + 1] && tokens[i + 1].kind === SyntaxKind.FirstAssignment

    // --- JSX text runs: consecutive tokens in children with no code between.
    const isTextToken =
      inChildren &&
      !opensExpression &&
      !isAttributeName &&
      (kind === SyntaxKind.Identifier ||
        kind === SyntaxKind.WhitespaceTrivia ||
        (kind >= SyntaxKind.FirstKeyword && kind <= SyntaxKind.LastKeyword))
    const previous = tokens[i - 1]
    const adjacent = previous && text.slice(previous.end, t.start).trim() === ''

    if (isTextToken && textRunStart !== null && adjacent) {
      textRunEnd = t.end
      continue
    }
    if (isTextToken) {
      flushTextRun()
      textRunStart = t.start
      textRunEnd = t.end
      continue
    }
    flushTextRun()

    // --- structural tokens.
    if (TAG_START.has(kind) && !inTag && isTagStart(i)) {
      inTag = true
      closingTag = false
      tagName = null
      tagOpenBrace = brace
      continue
    }

    if (kind === SyntaxKind.LessThanSlashToken) {
      inTag = true
      closingTag = true
      tagName = null
      tagOpenBrace = brace
      continue
    }

    // Braces are tracked BEFORE the in-tag skip below: an attribute expression
    // opens a brace inside a tag, and the closing `>` of that expression must not
    // be read as the end of the tag.
    // A template literal is skipped whole (see endOfTemplate). Its parts are
    // dynamic strings — class names, interpolated messages — not the literal UI
    // text this rule is about; the hole is documented in the file header.
    if (text.slice(t.start, t.end).startsWith('`')) {
      const after = endOfTemplate(text, t.start)
      scanner.resetTokenState(after)
      continue
    }

    if (isOpenBrace(t)) {
      brace++
      continue
    }
    if (isCloseBrace(t)) {
      if (brace > 0) brace--
      continue
    }

    if (kind === SyntaxKind.GreaterThanToken && inTag && brace === tagOpenBrace) {
      const selfClosing = previous && previous.kind === SyntaxKind.SlashToken
      if (closingTag) {
        elements.pop()
      } else if (!selfClosing) {
        elements.push({ name: tagName, brace })
      }
      inTag = false
      closingTag = false
      tagName = null
      continue
    }

    if (inTag) {
      // The first identifier inside a tag is the element name.
      if (isIdentifier(t) && tagName === null) tagName = text.slice(t.start, t.end)
      continue
    }
  }
  flushTextRun()

  return findings
}

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules') continue
      walk(full, out)
    } else if (entry.name.endsWith('.tsx') && !entry.name.endsWith('.test.tsx')) {
      // Test files are excluded: their literals are assertions and fixtures, not
      // rendered text, and `@testing-library` queries legitimately pass UI strings.
      out.push(full)
    }
  }
  return out
}

/**
 * The fixtures. Each is a fragment of a component; the expected count is the
 * number of findings. These are what keep the JSX heuristic honest — if the
 * context tracking regresses, they fail before the scan does.
 */
const FIXTURES = [
  [`export const A = () => <h1>{t['a.b']}</h1>`, 0, 'i18n key via element access'],
  [`export const A = () => <h1>Recent runs</h1>`, 1, 'jsx text'],
  [`export const A = () => <h1>{'Recent runs'}</h1>`, 1, 'string literal in an expression'],
  [`export const A = () => <h1>{translate('a.b')}</h1>`, 0, 'translate() call'],
  [`export const A = () => <h1>{t('a.b')}</h1>`, 0, 't() call'],
  [`export const A = () => <Trans>Recent runs</Trans>`, 0, 'Trans wrapper'],
  [`export const A = () => <div title="Recent runs" />`, 1, 'prose in a title attribute'],
  [`export const A = () => <div className="flex gap-2 items-center" />`, 0, 'class names'],
  [`export const A = () => <div data-testid="recent-runs-list" />`, 0, 'data-* attribute'],
  [`export const A = () => <span>{t['x.y'].replace('{n}', '3')}</span>`, 0, 'interpolation argument'],
  [`export const A = () => <span>{n} of {m}</span>`, 0, 'no letters'],
  [`export const A = () => <span>{'—'}</span>`, 0, 'symbol only'],
  [`export const A = () => <span>OK</span>`, 0, 'at the 3-character limit'],
  [`export const A = () => <span>Save</span>`, 1, 'a 4-character string is over the limit'],
  [`// Recent runs\nimport x from 'y'`, 0, 'comment is not scanned'],
  [`const A = () => <div>{cond ? <b>Yes</b> : <i>No</i>}</div>`, 0, 'nested elements, short text'],
  [`const A = () => <div>{t['a.b']['c.d']}</div>`, 0, 'chained element access'],
  [`const A = () => <div title={t['a.b']} />`, 0, 'translated attribute'],
  [`const A = () => <div title="Recent runs" />`, 1, 'hardcoded attribute'],
  // Only 'many' is a finding: 'few' is exactly 3 characters, and the AC's rule is
  // "longer than 3". The boundary is asserted on purpose in both directions —
  // `OK` below is the other side of it.
  [`const A = () => <p>{count < 5 ? 'few' : 'many'}</p>`, 1, 'ternary branch is prose'],
  [`const A = () => <p>{count < 5}</p>`, 0, 'less-than is not a tag'],
  [`const EVENTS = ['task.created', 'run.finished']`, 0, 'wire values outside JSX'],
  [`useEffect(() => { window.addEventListener('mousedown', h) }, [])`, 0, 'DOM event name'],
  [`const cls = 'flex gap-2 items-center'`, 0, 'CSS class outside JSX'],
  [`type P = VariantProps<typeof buttonVariants>`, 0, 'generic type argument is not a tag'],
  [`const A = () => <div className={cn('flex gap-2', ok && 'text-red')} />`, 0, 'cn() class list'],
  [`export function Button(props: ButtonProps) {}`, 0, 'function signature'],
  [
    `const A = () => <svg viewBox="0 0 8 8"><polyline points="1,1 7,7" strokeLinejoin="round" /></svg>`,
    0,
    'SVG attributes',
  ],
  [`const A = () => <Link to="/"><span className="mark" /></Link>`, 0, 'nested elements after a tag'],
  [`const A = () => <label>{t['a.b']}<input type="date" /></label>`, 0, 'element after an expression'],
  [`const A = () => <span>{t['a.b'].replace('{count}', String(n))}</span>`, 0, 'placeholder marker'],
  [`const A = () => <Button variant="secondary" />`, 0, 'design-token prop'],
]

// Fixtures for the baseline rule, which is not a lexer question: [baseline, now,
// expected file names flagged]. The baseline gate is the whole of AC2, so it is
// asserted on its own rather than inferred from a fixture count.
const BASELINE_FIXTURES = [
  [{ 'a.tsx': 3 }, { 'a.tsx': 3 }, [], 'unchanged count passes'],
  [{ 'a.tsx': 3 }, { 'a.tsx': 2 }, [], 'a count that FALLS passes'],
  [{ 'a.tsx': 3 }, { 'a.tsx': 4 }, ['a.tsx'], 'growth fails'],
  [{}, { 'new.tsx': 1 }, ['new.tsx'], 'a new file with text fails'],
  [{ 'a.tsx': 3 }, { 'b.tsx': 1 }, ['b.tsx'], 'a new file is judged on its own'],
  [{ 'a.tsx': 3, 'b.tsx': 1 }, { 'a.tsx': 9, 'b.tsx': 5 }, ['a.tsx', 'b.tsx'], 'both grown'],
]

function selfTest(ts) {
  let failures = 0
  for (const [src, want, label] of FIXTURES) {
    const got = analyse(ts, 'fixture.tsx', src).length
    if (got !== want) {
      failures++
      console.error(`  FAIL  ${label}: want ${want}, got ${got}  ${JSON.stringify(src)}`)
      for (const f of analyse(ts, 'fixture.tsx', src)) {
        console.error(`          -> ${f.kind} ${JSON.stringify(f.text)} at ${f.line}:${f.column}`)
      }
    } else {
      console.log(`  ok    ${label}`)
    }
  }
  for (const [baseline, now, want, label] of BASELINE_FIXTURES) {
    const got = regressionsAgainst(baseline, now).map((r) => r.file)
    if (JSON.stringify(got) !== JSON.stringify(want)) {
      failures++
      console.error(`  FAIL  baseline/${label}: want ${JSON.stringify(want)}, got ${JSON.stringify(got)}`)
    } else {
      console.log(`  ok    baseline/${label}`)
    }
  }

  const total = FIXTURES.length + BASELINE_FIXTURES.length
  console.log(failures === 0 ? `\nself-test: ${total} fixtures pass` : `\nself-test: ${failures} of ${total} FAILED`)
  return failures === 0 ? 0 : 1
}

function loadLexer() {
  try {
    // TypeScript 7 exports no `createSourceFile`, but the lexer is still there.
    return require('typescript/unstable/ast')
  } catch (err) {
    try {
      return require('typescript')
    } catch {
      console.error('check-i18n: cannot load the TypeScript lexer. Is `typescript` installed?')
      console.error(String(err))
      return null
    }
  }
}

/**
 * Baseline: the hardcoded strings that already exist.
 *
 * US-AD50 AC1 says the script fails on any finding, and it does — `--strict` is
 * that rule and it is what the check reports on. The repo is not i18n-complete
 * yet (803 findings across 40 files when this was written), so a strict gate in
 * CI would be red on every commit and would be turned off within a week.
 *
 * The baseline keeps the gate MEANINGFUL instead of red: counts are frozen per
 * file, a file that grows past its count fails, and a NEW file with any finding
 * fails. Removing hardcoded text is always allowed (the count may drop).
 *
 * What this does NOT catch, and it is a real hole: swapping one translated
 * string for one hardcoded string inside a baselined file leaves the count
 * unchanged. The count is the cheap signal; `--strict` is the honest one, and
 * progress is measured by running it and watching the number fall.
 */
function loadBaseline(file) {
  if (!fs.existsSync(file)) return null
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8'))
  } catch (error) {
    console.error(`check-i18n: cannot read the baseline at ${file}: ${error.message}`)
    process.exit(2)
  }
}

function countByFile(findings) {
  const out = {}
  for (const f of findings) out[f.file] = (out[f.file] || 0) + 1
  return out
}

/**
 * Files whose hardcoded-string count grew past the baseline.
 *
 * Pure so it can be tested: this is the whole gate. A file absent from the
 * baseline allows zero — a NEW component with hardcoded text fails even though
 * nothing it replaced existed before. Counts may fall freely; only growth is a
 * regression.
 */
function regressionsAgainst(baseline, now) {
  const out = []
  for (const [file, count] of Object.entries(now)) {
    const allowed = baseline[file] || 0
    if (count > allowed) out.push({ file, count, allowed })
  }
  return out.sort((a, b) => a.file.localeCompare(b.file))
}

function main() {
  const ts = loadLexer()
  if (!ts) return 2
  if (typeof ts.createScanner !== 'function') {
    console.error('check-i18n: this TypeScript build exposes no createScanner; the check cannot run.')
    return 2
  }

  if (process.argv.includes('--self-test')) return selfTest(ts)

  const files = walk(SRC)
  const findings = []
  for (const file of files) {
    const rel = path.relative(ROOT, file).replace(/\\/g, '/')
    findings.push(...analyse(ts, rel, fs.readFileSync(file, 'utf8')))
  }

  if (process.argv.includes('--update-baseline')) {
    const snapshot = countByFile(findings)
    fs.writeFileSync(BASELINE, JSON.stringify(snapshot, Object.keys(snapshot).sort(), 2) + '\n')
    console.log(`check-i18n: baseline written to ${path.relative(ROOT, BASELINE)} (${findings.length} findings)`)
    return 0
  }

  if (process.argv.includes('--list')) {
    for (const f of findings) {
      console.log(`${f.file}:${f.line}:${f.column}  [${f.kind}]  ${JSON.stringify(f.text)}`)
    }
    console.log(`\n${findings.length} finding(s) in ${files.length} files`)
    return 0
  }

  if (findings.length === 0) {
    console.log(`check-i18n: ${files.length} .tsx files scanned, no hardcoded UI text`)
    return 0
  }

  // US-AD50 AC1: any finding fails. `--strict` is that rule, unchanged.
  const strict = process.argv.includes('--strict')
  if (!strict) {
    const baseline = loadBaseline(BASELINE)
    if (baseline) {
      const regressions = regressionsAgainst(baseline, countByFile(findings))
      if (regressions.length === 0) {
        const total = findings.length
        console.log(`check-i18n: ${files.length} .tsx files scanned, ${total} known finding(s), no regression`)
        console.log('  (run with --strict to list them, or --update-baseline after removing some)')
        return 0
      }
      console.error('check-i18n: hardcoded UI text grew in these files\n')
      for (const r of regressions) {
        console.error(`  ${r.file}: ${r.count} finding(s), baseline allows ${r.allowed}`)
      }
      console.error("\nNew text belongs in src/lib/i18n.ts, read via t['key'].")
      return 1
    }
  }

  const byFile = new Set(findings.map((f) => f.file))
  console.error(`check-i18n: ${findings.length} hardcoded string(s) in ${byFile.size} file(s)\n`)
  for (const f of findings) {
    console.error(`  ${f.file}:${f.line}:${f.column}  [${f.kind}]  ${JSON.stringify(f.text)}`)
  }
  console.error("\nAdd each one to src/lib/i18n.ts and read it via t['key'] (or wrap it in <Trans>).")
  return 1
}

process.exit(main())
