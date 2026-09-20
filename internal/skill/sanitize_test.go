package skill

import (
	"strings"
	"testing"
)

// TestSanitizeMarkdownDropsExecutableHTML is US-AD107 AC3 at the unit level: the
// body is user input, so the constructs that execute on render must not survive.
// The assertion is on the *dangerous fragment*, not on the whole string, so a
// future change that keeps harmless text is not a false failure.
func TestSanitizeMarkdownDropsExecutableHTML(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		removed []string
		kept    []string
	}{
		{
			name:    "script block with content",
			body:    "# Title\n\n<script>alert(document.cookie)</script>\n\nAfter.",
			removed: []string{"<script", "alert(document.cookie)", "</script>"},
			kept:    []string{"# Title", "After."},
		},
		{
			name:    "unclosed iframe",
			body:    `<iframe src="https://evil.test/x">`,
			removed: []string{"<iframe", "evil.test"},
		},
		{
			name:    "inline handler",
			body:    `<a href="https://ok.test" onclick="steal()">link</a>`,
			removed: []string{"onclick", "steal()"},
			kept:    []string{"https://ok.test", "link"},
		},
		{
			name:    "javascript scheme",
			body:    `[click](javascript:alert(1))`,
			removed: []string{"javascript:alert(1)"},
		},
		{
			name:    "javascript scheme with a tab inside it",
			body:    "<a href=\"java\tscript:alert(1)\">x</a>",
			removed: []string{"script:alert(1)"},
		},
		{
			name:    "svg with onload",
			body:    `<svg onload="alert(1)"></svg>`,
			removed: []string{"<svg", "onload"},
		},
		{
			name:    "style block",
			body:    `<style>body{background:url(javascript:1)}</style>`,
			removed: []string{"<style", "background:url"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeMarkdown(tc.body)
			for _, fragment := range tc.removed {
				if strings.Contains(got, fragment) {
					t.Errorf("sanitized body still contains %q:\n%s", fragment, got)
				}
			}
			for _, fragment := range tc.kept {
				if !strings.Contains(got, fragment) {
					t.Errorf("sanitized body dropped benign %q:\n%s", fragment, got)
				}
			}
		})
	}
}

// TestSanitizeMarkdownKeepsOrdinaryMarkdown: the sanitizer runs on every body,
// so it must not mangle the markdown a user actually writes. An over-eager
// filter that ate a code fence would be a worse bug than the one it fixes.
func TestSanitizeMarkdownKeepsOrdinaryMarkdown(t *testing.T) {
	body := "# Code review\n\n" +
		"1. Baca `diff` dulu.\n" +
		"2. Jalankan `go test ./...`.\n\n" +
		"```go\nif err != nil { return err }\n```\n\n" +
		"Lihat [dokumentasi](https://example.test/docs) dan <https://example.test>.\n"

	if got := SanitizeMarkdown(body); got != body {
		t.Fatalf("ordinary markdown was modified:\n--- got ---\n%s\n--- want ---\n%s", got, body)
	}
}

// TestSanitizeMarkdownIsIdempotent: a body is sanitized on every render, so a
// second pass must not keep changing the text. A sanitizer that rewrote its own
// output would make the preview flicker on each keystroke.
func TestSanitizeMarkdownIsIdempotent(t *testing.T) {
	body := `<script>x()</script><a href="javascript:1" onclick="y()">t</a> keep`
	once := SanitizeMarkdown(body)
	if twice := SanitizeMarkdown(once); twice != once {
		t.Fatalf("not idempotent:\nonce  = %q\ntwice = %q", once, twice)
	}
}
