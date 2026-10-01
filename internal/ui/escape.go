package ui

import (
	"fmt"
	"strings"
	"unicode"
)

// Escape makes text safe to display: control, format (bidi, zero-width), and non-ASCII
// characters are shown as visible escapes so hidden or look-alike text can't mislead.
func Escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r > 0x7e || unicode.Is(unicode.Cf, r):
			fmt.Fprintf(&b, `\u{%X}`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Quote renders one argument shell-style after escaping, so the user sees argument boundaries.
func Quote(a string) string {
	e := Escape(a)
	if a != "" && e == a && !strings.ContainsAny(a, " '\"\\$`|&;<>()*?[]{}~#!") {
		return a
	}
	if e != a {
		return "$'" + strings.ReplaceAll(e, "'", `\'`) + "'"
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

func QuoteArgv(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = Quote(a)
	}
	return strings.Join(q, " ")
}
