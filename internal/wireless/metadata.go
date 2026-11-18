package wireless

import (
	"fmt"
	"strings"
	"time"
)

// BookInfo is what Lord Books knows about a book being sent.
type BookInfo struct {
	ID          int
	Title       string
	Authors     []string
	Publisher   string
	PubDate     string
	Language    string // two-letter code
	ISBN        string
	Tags        []string
	Series      string
	SeriesIndex float64
	Description string
	FileHash    string
	Format      string
	DateAdded   string
}

var languages3 = map[string]string{"pt": "por", "en": "eng", "es": "spa", "fr": "fra", "de": "deu", "it": "ita", "ja": "jpn", "ru": "rus"}

var mimeTypes = map[string]string{
	"epub": "application/epub+zip", "pdf": "application/pdf",
	"azw3": "application/vnd.amazon.ebook", "mobi": "application/x-mobipocket-ebook",
}

const undefinedDate = "0101-01-01T00:00:00+00:00" // calibre's "no date"

// Metadata builds the book dictionary calibre sends with SEND_BOOK, which readers
// store and show in their library (title, authors, series...).
func Metadata(b BookInfo) map[string]any {
	now := time.Now().UTC().Format("2006-01-02T15:04:05+00:00")
	authors := b.Authors
	if len(authors) == 0 {
		authors = []string{"Desconhecido"}
	}
	sortMap := map[string]string{}
	for _, a := range authors {
		sortMap[a] = authorSort(a)
	}
	var series, seriesIndex any
	if b.Series != "" {
		series, seriesIndex = b.Series, b.SeriesIndex
	}
	var langs []string
	if l := languages3[b.Language]; l != "" {
		langs = []string{l}
	} else if b.Language != "" {
		langs = []string{b.Language}
	}
	ids := map[string]string{}
	if b.ISBN != "" {
		ids["isbn"] = b.ISBN
	}
	tags := b.Tags
	if tags == nil {
		tags = []string{}
	}
	return map[string]any{
		"title":           b.Title,
		"title_sort":      b.Title,
		"authors":         authors,
		"author_sort":     authorSort(authors[0]),
		"author_sort_map": sortMap,
		"uuid":            uuidFromHash(b.FileHash, b.ID),
		"application_id":  b.ID,
		"db_id":           b.ID,
		"publisher":       nullIfEmpty(b.Publisher),
		"pubdate":         isoDate(b.PubDate),
		"timestamp":       isoDate(b.DateAdded),
		"last_modified":   now,
		"languages":       langs,
		"identifiers":     ids,
		"tags":            tags,
		"series":          series,
		"series_index":    seriesIndex,
		"_series_sort_":   b.Series,
		"comments":        nullIfEmpty(b.Description),
		"mime":            mimeTypes[b.Format],
		"thumbnail":       nil,
		"cover":           nil,
		"rating":          nil,
		"book_producer":   nil,
		"user_categories": map[string]any{},
		"user_metadata":   map[string]any{},
		"link_maps":       map[string]any{},
	}
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func isoDate(s string) string {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02", "2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05+00:00")
		}
	}
	return undefinedDate
}

func authorSort(name string) string {
	parts := strings.Fields(name)
	if len(parts) <= 1 {
		return name
	}
	return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
}

// uuidFromHash gives each book a stable UUID (Lord Books has no UUID column) so the
// reader recognizes the same book when it is sent again.
func uuidFromHash(hash string, id int) string {
	h := strings.ToLower(hash)
	if h == "" {
		h = fmt.Sprintf("%x", id)
	}
	if len(h) < 32 {
		h = strings.Repeat("0", 32-len(h)) + h
	}
	return h[0:8] + "-" + h[8:12] + "-4" + h[13:16] + "-a" + h[17:20] + "-" + h[20:32]
}
