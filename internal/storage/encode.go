package storage

// URI-encoding versi SigV4.
//
// Ini BUKAN url.QueryEscape: QueryEscape meng-encode spasi jadi `+` dan
// membiarkan beberapa karakter yang SigV4 wajibkan di-encode. Signature yang
// dibangun dengan encoder yang salah akan ditolak server sebagai
// SignatureDoesNotMatch, tanpa petunjuk karakter mana yang salah.

import (
	"strings"
)

// unreserved adalah set karakter yang TIDAK di-encode SigV4.
func isUnreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == '.' || c == '~':
		return true
	}
	return false
}

const hexDigits = "0123456789ABCDEF"

// encodeQuery meng-encode satu komponen (nama atau nilai parameter).
// Setiap byte non-unreserved menjadi `%XX` huruf besar.
func encodeQuery(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0f])
	}
	return b.String()
}

// encodePath meng-encode path per segmen: `/` tetap `/`, sisanya di-encode
// seperti komponen query.
//
// Key artifact berbentuk `artifacts/{org}/{task}/{run}/{ulid}-{filename}`, dan
// filename bisa memuat spasi. Meng-encode seluruh path akan mengubah pemisah
// segmen jadi `%2F` dan signature-nya tidak akan cocok untuk key bersarang.
func encodePath(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = encodeQuery(seg)
	}
	return strings.Join(segments, "/")
}
