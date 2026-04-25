package rcon

import (
	"errors"
	"strings"
)

// ErrControlChar is returned by FormatArg when the input contains ASCII
// control characters; PZ doesn't handle them and they're a sign of injection.
var ErrControlChar = errors.New("rcon: argument contains a control character")

// FormatArg formats a single argument for inclusion in an RCON command line.
// Bare values (no whitespace, quote, or backslash) pass through unchanged;
// otherwise the value is wrapped in double quotes with embedded quotes and
// backslashes escaped. ASCII control characters (0x00-0x1F, 0x7F) are
// rejected.
func FormatArg(s string) (string, error) {
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			return "", ErrControlChar
		}
	}
	if s == "" {
		return `""`, nil
	}
	if !needsQuoting(s) {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String(), nil
}

func needsQuoting(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '"', '\\':
			return true
		}
	}
	return false
}
