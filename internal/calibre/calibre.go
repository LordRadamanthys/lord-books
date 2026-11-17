// Package calibre reads books from a Calibre library folder (metadata.db + book folders).
package calibre

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Book struct {
	CalibreID   int
	Title       string
	Authors     []string
	ISBN        string
	Publisher   string
	PubDate     string
	Language    string
	Description string
	SeriesName  string
	SeriesIndex float64
	Tags        []string
	Rating      int // 0-5 stars
	Read        bool
	FilePath    string // empty when the book has no supported format
	Format      string
	CoverPath   string
}

// DefaultPath returns the library configured in Calibre's preferences, or "" if none is found.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{}
	for _, cfg := range []string{
		filepath.Join(home, "Library", "Preferences", "calibre", "global.py.json"),
		filepath.Join(home, ".config", "calibre", "global.py.json"),
		filepath.Join(os.Getenv("APPDATA"), "calibre", "global.py.json"),
	} {
		if p := libraryPathFromConfig(cfg); p != "" {
			candidates = append(candidates, p)
		}
	}
	candidates = append(candidates, filepath.Join(home, "Calibre Library"))
	for _, p := range candidates {
		if IsLibrary(p) {
			return p
		}
	}
	return ""
}

func libraryPathFromConfig(cfg string) string {
	data, err := os.ReadFile(cfg)
	if err != nil {
		return ""
	}
	// global.py.json is JSON, but only one key is needed and a tolerant scan avoids
	// failing on Calibre's occasional non-standard values.
	s := string(data)
	i := strings.Index(s, `"library_path"`)
	if i < 0 {
		return ""
	}
	s = s[i+len(`"library_path"`):]
	start := strings.Index(s, `"`)
	if start < 0 {
		return ""
	}
	s = s[start+1:]
	end := strings.Index(s, `"`)
	if end < 0 {
		return ""
	}
	return strings.ReplaceAll(s[:end], `\\`, `\`)
}

func IsLibrary(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "metadata.db"))
	return err == nil && st.Mode().IsRegular()
}

// preferred formats, best first; Lord Books stores one file per book.
var formats = []string{"EPUB", "PDF"}

func Read(ctx context.Context, dir string) ([]Book, error) {
	if !IsLibrary(dir) {
		return nil, fmt.Errorf("metadata.db não encontrado em %s", dir)
	}
	dsn := (&url.URL{Scheme: "file", Path: filepath.Join(dir, "metadata.db"), RawQuery: "mode=ro&_pragma=query_only(1)"}).String()
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	rows, err := conn.QueryContext(ctx, `SELECT b.id, b.title, b.path, COALESCE(b.pubdate,''), b.series_index, b.has_cover,
		COALESCE((SELECT c.text FROM comments c WHERE c.book = b.id), ''),
		COALESCE((SELECT s.name FROM series s JOIN books_series_link l ON l.series = s.id WHERE l.book = b.id), ''),
		COALESCE((SELECT p.name FROM publishers p JOIN books_publishers_link l ON l.publisher = p.id WHERE l.book = b.id), ''),
		COALESCE((SELECT g.lang_code FROM languages g JOIN books_languages_link l ON l.lang_code = g.id WHERE l.book = b.id ORDER BY l.item_order LIMIT 1), ''),
		COALESCE((SELECT r.rating FROM ratings r JOIN books_ratings_link l ON l.rating = r.id WHERE l.book = b.id), 0),
		COALESCE((SELECT i.val FROM identifiers i WHERE i.book = b.id AND i.type = 'isbn'), '')
		FROM books b ORDER BY b.id`)
	if err != nil {
		return nil, fmt.Errorf("ler livros do Calibre: %w", err)
	}
	var books []Book
	byID := map[int]*Book{}
	for rows.Next() {
		var b Book
		var path, pubdate string
		var hasCover bool
		if err := rows.Scan(&b.CalibreID, &b.Title, &path, &pubdate, &b.SeriesIndex, &hasCover,
			&b.Description, &b.SeriesName, &b.Publisher, &b.Language, &b.Rating, &b.ISBN); err != nil {
			rows.Close()
			return nil, err
		}
		b.Rating /= 2
		b.PubDate = cleanDate(pubdate)
		bookDir := filepath.Join(dir, filepath.FromSlash(path))
		if hasCover {
			if c := filepath.Join(bookDir, "cover.jpg"); fileExists(c) {
				b.CoverPath = c
			}
		}
		b.FilePath = bookDir // resolved to a file below
		books = append(books, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range books {
		byID[books[i].CalibreID] = &books[i]
	}

	if err := eachRow(ctx, conn, `SELECT l.book, a.name FROM books_authors_link l JOIN authors a ON a.id = l.author ORDER BY l.book, l.id`,
		func(id int, v string) {
			if b := byID[id]; b != nil {
				b.Authors = append(b.Authors, strings.ReplaceAll(v, "|", ","))
			}
		}); err != nil {
		return nil, err
	}
	if err := eachRow(ctx, conn, `SELECT l.book, t.name FROM books_tags_link l JOIN tags t ON t.id = l.tag ORDER BY l.book, t.name`,
		func(id int, v string) {
			if b := byID[id]; b != nil {
				b.Tags = append(b.Tags, v)
			}
		}); err != nil {
		return nil, err
	}

	files := map[int]map[string]string{}
	if err := eachRow2(ctx, conn, `SELECT book, format, name FROM data`, func(id int, format, name string) {
		if files[id] == nil {
			files[id] = map[string]string{}
		}
		files[id][strings.ToUpper(format)] = name
	}); err != nil {
		return nil, err
	}
	for i := range books {
		b := &books[i]
		bookDir := b.FilePath
		b.FilePath = ""
		for _, f := range formats {
			name, ok := files[b.CalibreID][f]
			if !ok {
				continue
			}
			p := filepath.Join(bookDir, name+"."+strings.ToLower(f))
			if fileExists(p) {
				b.FilePath, b.Format = p, strings.ToLower(f)
				break
			}
		}
	}

	readCol, err := readColumn(ctx, conn)
	if err == nil && readCol != "" {
		eachRow(ctx, conn, `SELECT book, value FROM `+readCol, func(id int, v string) {
			if b := byID[id]; b != nil && (v == "1" || strings.EqualFold(v, "true")) {
				b.Read = true
			}
		})
	}

	return books, nil
}

// readColumn finds a yes/no custom column commonly used to mark books as read.
func readColumn(ctx context.Context, conn *sql.DB) (string, error) {
	var id int
	err := conn.QueryRowContext(ctx, `SELECT id FROM custom_columns
		WHERE datatype = 'bool' AND lower(label) IN ('read', 'lido', 'lidos') LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("custom_column_%d", id), nil
}

func eachRow(ctx context.Context, conn *sql.DB, q string, fn func(int, string)) error {
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			return err
		}
		fn(id, v)
	}
	return rows.Err()
}

func eachRow2(ctx context.Context, conn *sql.DB, q string, fn func(int, string, string)) error {
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var a, b string
		if err := rows.Scan(&id, &a, &b); err != nil {
			return err
		}
		fn(id, a, b)
	}
	return rows.Err()
}

// cleanDate drops Calibre's "undefined" date (year 101) and keeps YYYY-MM-DD.
func cleanDate(s string) string {
	if len(s) < 10 || s[:4] < "1000" {
		return ""
	}
	return s[:10]
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
