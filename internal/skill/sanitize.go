package skill

import (
	"regexp"
	"strings"
)

// dangerousTags are the elements whose *content* executes or embeds. Their body
// has to be dropped along with the tags, which is why each name gets its own
// pair of patterns: Go's regexp is RE2, so a backreference (`</\1>`) that would
// let one pattern handle all of them does not compile.
var dangerousTags = []string{"script", "style", "iframe", "object", "embed", "form", "svg", "math"}

var (
	// blockPatterns match an open/close pair including everything between them.
	blockPatterns = compileTags(`(?is)<%s\b[^>]*>.*?</%s\s*>`)
	// tagPatterns match a lone or unclosed tag of the same name, which the block
	// pattern misses (e.g. `<iframe src=...>` with no closer).
	tagPatterns = compileTags(`(?is)</?%s\b[^>]*>`)

	// eventAttr is every `on*` inline handler (`onclick`, `onerror`, ...).
	eventAttr = regexp.MustCompile(`(?is)\son[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	// unsafeURL is an href/src carrying a URL value.
	unsafeURL = regexp.MustCompile(`(?is)\b(href|src|xlink:href)\s*=\s*("[^"]*"|'[^']*'|[^\s>]*)`)
	// mdLink is a markdown link destination: `](javascript:...)`. A renderer that
	// turns this into an anchor would carry the scheme through, so it is cleaned
	// here rather than trusted to the renderer.
	mdLink = regexp.MustCompile(`(?is)\]\s*\(\s*([^)\s]*)`)
)

// executableScheme reports whether a URL value carries a scheme a renderer would
// execute. Compared after trimming quotes, whitespace, and the control
// characters a browser ignores — `java\tscript:` is javascript: to a browser.
func executableScheme(value string) bool {
	v := strings.Trim(strings.TrimSpace(value), `"'`)
	v = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r == '\x00' {
			return -1
		}
		return r
	}, v)
	v = strings.ToLower(v)
	return strings.HasPrefix(v, "javascript:") ||
		strings.HasPrefix(v, "vbscript:") ||
		strings.HasPrefix(v, "data:text/html") ||
		strings.HasPrefix(v, "data:image/svg+xml")
}

func compileTags(format string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(dangerousTags))
	for _, tag := range dangerousTags {
		out = append(out, regexp.MustCompile(strings.ReplaceAll(format, "%s", regexp.QuoteMeta(tag))))
	}
	return out
}

// SanitizeMarkdown strips the HTML constructs that execute when a markdown body
// is rendered: script/style/iframe/object/embed/form/svg/math elements (with
// their content), `on*` event-handler attributes, and `javascript:`,
// `vbscript:`, `data:text/html`, `data:image/svg+xml` URLs. The body is user
// input (US-AD107 AC3) and the API stores it verbatim, so anything that renders
// it has to clean it first.
//
// ponytail: a regex pass is defense in depth, not a security boundary — a
// hand-rolled sanitizer has bypasses. The real boundary is the renderer: React
// escapes text by default, so a preview that does not use
// dangerouslySetInnerHTML is already safe. Add a real sanitizer (a dependency)
// only if this output is ever inserted as raw HTML.
func SanitizeMarkdown(body string) string {
	for _, re := range blockPatterns {
		body = re.ReplaceAllString(body, "")
	}
	for _, re := range tagPatterns {
		body = re.ReplaceAllString(body, "")
	}
	body = eventAttr.ReplaceAllString(body, "")
	body = unsafeURL.ReplaceAllStringFunc(body, func(match string) string {
		name, value, found := strings.Cut(match, "=")
		if !found || !executableScheme(value) {
			return match
		}
		return strings.TrimSpace(name) + `="#"`
	})
	// `[label](javascript:...)` has no href attribute to catch, so the link
	// destination is neutralized in place and the label is left alone.
	return mdLink.ReplaceAllStringFunc(body, func(match string) string {
		value := strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(match, "]")), "(")
		if !executableScheme(value) {
			return match
		}
		return "](#)"
	})
}
