package model

import (
	"fmt"
	"strings"
)

// MidFromMessageID converts a Message-ID header value to a mid: URI (RFC
// 2392): surrounding whitespace and the angle brackets are removed, and every
// character a URL may not carry literally is percent-encoded. "/" is encoded
// too, because RFC 2392 uses it to separate a Content-ID.
func MidFromMessageID(v string) (string, error) {
	s := strings.TrimSpace(v)
	if strings.HasPrefix(s, "<") != strings.HasSuffix(s, ">") {
		return "", fmt.Errorf("Message-ID %q has unbalanced angle brackets", v)
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "<"), ">")
	if s == "" || strings.ContainsAny(s, "<>") {
		return "", fmt.Errorf("Message-ID %q is empty or malformed", v)
	}
	var b strings.Builder
	b.WriteString("mid:")
	for _, c := range []byte(s) {
		if midSafe(c) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String(), nil
}

// midSafe is RFC 3986 unreserved, the sub-delims, and "@", less "/" and "%".
func midSafe(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("-._~!$&'()*+,;=@", c) >= 0
}
