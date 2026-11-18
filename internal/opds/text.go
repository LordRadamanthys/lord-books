package opds

import (
	"html"
	"regexp"
	"strings"
)

var (
	breakTags = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li)\s*/?>`)
	anyTag    = regexp.MustCompile(`<[^>]*>`)
	blankRuns = regexp.MustCompile(`\n{3,}`)
)

// plainText turns an HTML description (common in EPUB and Calibre metadata) into text;
// OPDS readers render summaries as plain text and would show the tags literally.
func plainText(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return strings.TrimSpace(s)
	}
	s = breakTags.ReplaceAllString(s, "\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(blankRuns.ReplaceAllString(s, "\n\n"))
}
