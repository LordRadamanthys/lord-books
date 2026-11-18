package freebooks

import (
	"context"
	"testing"
)

func TestGutenbergName(t *testing.T) {
	cases := map[string]string{
		"Assis, Machado de":  "Machado de Assis",
		"Austen, Jane":       "Jane Austen",
		"Homer":              "Homer",
		"Tolstoy, Leo, graf": "Tolstoy, Leo, graf",
	}
	for in, want := range cases {
		if got := gutenbergName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestDownloadRejectsForeignIDs(t *testing.T) {
	c := New()
	for _, tc := range []struct{ source, id string }{
		{Gutenberg, "../../etc/passwd"},
		{Gutenberg, "-5"},
		{Gutenberg, "https://evil.example/book.epub"},
		{Standard, "https://evil.example/book"},
		{Standard, "/ebooks/../../admin"},
		{Standard, "/ebooks/a"},
		{"zlib", "1"},
	} {
		if _, _, err := c.Download(context.Background(), tc.source, tc.id); err == nil {
			t.Errorf("%s %q should be rejected", tc.source, tc.id)
		}
	}
}

func TestParseStandardSearch(t *testing.T) {
	page := `<ol><li typeof="schema:Book" about="/ebooks/charles-dickens/barnaby-rudge">
	<img src="/images/covers/x/cover@2x.jpg" alt="" property="schema:image" height="335"/>
	<p><a href="/ebooks/charles-dickens/barnaby-rudge" property="schema:url"><span property="schema:name">Barnaby Rudge</span></a></p>
	<p class="author" typeof="schema:Person" property="schema:author" resource="/ebooks/charles-dickens"><a href="https://standardebooks.org/ebooks/charles-dickens" property="schema:url"><span property="schema:name">Charles Dickens</span></a></p>
	</li></ol>`
	m := seItem.FindAllStringSubmatch(page, -1)
	if len(m) != 1 || m[0][1] != "/ebooks/charles-dickens/barnaby-rudge" {
		t.Fatalf("items: %v", m)
	}
	if got := seTitle.FindStringSubmatch(m[0][2]); got == nil || got[1] != "Barnaby Rudge" {
		t.Errorf("title: %v", got)
	}
	if got := seAuthor.FindStringSubmatch(m[0][2]); got == nil || got[1] != "Charles Dickens" {
		t.Errorf("author: %v", got)
	}
}
