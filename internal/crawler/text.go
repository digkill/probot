package crawler

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var htmlTagRe = regexp.MustCompile(`(?is)<[^>]*>`)
var spaceRe = regexp.MustCompile(`\s+`)

func StripHTML(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func LooksRussian(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Cyrillic) {
			return true
		}
	}
	return false
}
