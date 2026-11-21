// Package text sanitises user-supplied display text (names, titles, chat).
package text

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Clean NFC-normalises s, strips control characters and bidi overrides
// (U+202A–202E, U+2066–2069) while keeping ZWJ/ZWNJ for emoji and scripts
// that need them, collapses whitespace runs and trims. It reports whether the
// result has between minLen and maxLen runes.
func Clean(s string, minLen, maxLen int) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	cleaned := collapseSpaces(strings.Map(keepRune, norm.NFC.String(s)))
	n := utf8.RuneCountInString(cleaned)
	return cleaned, n >= minLen && n <= maxLen
}

// CleanMultiline is Clean for chat messages: newlines are preserved.
func CleanMultiline(s string, maxLen int) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	cleaned := strings.TrimSpace(strings.Map(keepRuneOrNewline, norm.NFC.String(s)))
	n := utf8.RuneCountInString(cleaned)
	return cleaned, n >= 1 && n <= maxLen
}

func keepRune(r rune) rune {
	if isBidiControl(r) || unicode.IsControl(r) {
		if unicode.IsSpace(r) {
			return ' '
		}
		return -1
	}
	return r
}

func keepRuneOrNewline(r rune) rune {
	if r == '\n' {
		return r
	}
	return keepRune(r)
}

func isBidiControl(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
