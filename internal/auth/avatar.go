package auth

import (
	"strings"
	"unicode"
)

// avatarSizePX is the monogram diameter the design source bakes into the shell
// (`15-profile.html`: "Diameter baku: 26px • Warna: #101014"). It is a server
// field rather than a client constant because US-AD89 AC1 makes `avatar_user`
// part of the `GET /api/v1/auth/me` payload: the screen renders the value the
// API returned, not a second copy that could drift from it.
const avatarSizePX = 26

// avatarBackground is the monogram disc colour from the same design line.
const avatarBackground = "#101014"

// Avatar is the `avatar_user` object of `GET /api/v1/auth/me` (US-AD89 AC1).
//
// The field set is frozen: `kind`, `initials`, `bg_color`, `size_px`. The name
// is deliberately not `avatar_url` — a profile that has uploaded a real image
// reports kind `image` plus `url`, while one that has not reports the monogram
// the shell draws. A single object keeps the screen's four fields identical to
// the response, which is the literal wording of AC1.
type Avatar struct {
	Kind     string `json:"kind"`
	Initials string `json:"initials"`
	BgColor  string `json:"bg_color"`
	SizePX   int    `json:"size_px"`
	// URL is the uploaded image, when there is one. Empty for a monogram.
	URL string `json:"url,omitempty"`
}

// AvatarFor derives the avatar payload for one user. `avatarURL` wins when set
// (users.avatar_url, ARCHITECTURE 3.3); otherwise the monogram falls back to
// the display name, then to the email's local part, so a user whose name is
// empty still gets a deterministic disc instead of a blank circle.
func AvatarFor(name, email, avatarURL string) Avatar {
	if url := strings.TrimSpace(avatarURL); url != "" {
		return Avatar{
			Kind:     "image",
			Initials: initials(name, email),
			BgColor:  avatarBackground,
			SizePX:   avatarSizePX,
			URL:      url,
		}
	}
	return Avatar{
		Kind:     "monogram",
		Initials: initials(name, email),
		BgColor:  avatarBackground,
		SizePX:   avatarSizePX,
	}
}

// initials builds the 1–2 glyph monogram the shell shows ("RZ" in the design).
// It takes the first rune of the first two words; a single-word name yields one
// glyph, an empty name falls back to the email local part, and a name with no
// letters at all yields "?" rather than an empty disc.
func initials(name, email string) string {
	source := strings.TrimSpace(name)
	if source == "" {
		local := strings.TrimSpace(email)
		if at := strings.Index(local, "@"); at >= 0 {
			local = local[:at]
		}
		source = local
	}

	fields := strings.FieldsFunc(source, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var out strings.Builder
	for _, field := range fields {
		for _, r := range field {
			out.WriteRune(unicode.ToUpper(r))
			break
		}
		if out.Len() >= 2 {
			break
		}
	}
	if out.Len() == 0 {
		return "?"
	}
	return out.String()
}
