package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &DB{conn: conn}, nil
}

func (d *DB) Close() error { return d.conn.Close() }

type Book struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	ISBN        string   `json:"isbn,omitempty"`
	Language    string   `json:"language,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	PubDate     string   `json:"pub_date,omitempty"`
	Description string   `json:"description,omitempty"`
	FilePath    string   `json:"-"`
	CoverPath   string   `json:"-"`
	FileHash    string   `json:"file_hash,omitempty"`
	FileSize    int64    `json:"file_size,omitempty"`
	Format      string   `json:"format"`
	Rating      int      `json:"rating"`
	ReadStatus  string   `json:"read_status"`
	SeriesName  string   `json:"series,omitempty"`
	SeriesIndex float64  `json:"series_index,omitempty"`
	Authors     []string `json:"authors"`
	Tags        []string `json:"tags,omitempty"`
	DateAdded   string   `json:"date_added"`
	HasCover    bool     `json:"has_cover"`
}

type ListFilter struct {
	Query      string
	ReadStatus string
	AuthorID   int
	SeriesID   int
	TagID      int
	Limit      int
	Offset     int
}

func (d *DB) ListBooks(ctx context.Context, f ListFilter) ([]Book, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if f.Query != "" {
		where = append(where, "(b.title LIKE ? COLLATE NOCASE OR a.name LIKE ? COLLATE NOCASE)")
		q := "%" + f.Query + "%"
		args = append(args, q, q)
	}
	if f.ReadStatus != "" {
		where = append(where, "b.read_status = ?")
		args = append(args, f.ReadStatus)
	}
	if f.AuthorID > 0 {
		where = append(where, "ba.author_id = ?")
		args = append(args, f.AuthorID)
	}
	if f.SeriesID > 0 {
		where = append(where, "b.series_id = ?")
		args = append(args, f.SeriesID)
	}
	if f.TagID > 0 {
		where = append(where, "bt.tag_id = ?")
		args = append(args, f.TagID)
	}

	w := strings.Join(where, " AND ")

	countQ := `SELECT COUNT(DISTINCT b.id) FROM books b
		LEFT JOIN book_authors ba ON ba.book_id = b.id
		LEFT JOIN authors a ON a.id = ba.author_id
		LEFT JOIN book_tags bt ON bt.book_id = b.id
		WHERE ` + w

	var total int
	if err := d.conn.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count books: %w", err)
	}

	if f.Limit <= 0 {
		f.Limit = 50
	}

	q := `SELECT DISTINCT b.id, b.title, b.isbn, b.language, b.publisher, b.pub_date,
		       b.description, b.file_path, b.cover_path, b.file_hash, b.file_size,
		       b.format, b.rating, b.read_status, COALESCE(s.name,''),
		       b.series_index, b.date_added
		FROM books b
		LEFT JOIN book_authors ba ON ba.book_id = b.id
		LEFT JOIN authors a ON a.id = ba.author_id
		LEFT JOIN series s ON s.id = b.series_id
		LEFT JOIN book_tags bt ON bt.book_id = b.id
		WHERE ` + w + `
		ORDER BY b.date_added DESC
		LIMIT ? OFFSET ?`

	args = append(args, f.Limit, f.Offset)
	rows, err := d.conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.ISBN, &b.Language, &b.Publisher,
			&b.PubDate, &b.Description, &b.FilePath, &b.CoverPath, &b.FileHash,
			&b.FileSize, &b.Format, &b.Rating, &b.ReadStatus, &b.SeriesName,
			&b.SeriesIndex, &b.DateAdded); err != nil {
			return nil, 0, fmt.Errorf("scan book: %w", err)
		}
		b.HasCover = b.CoverPath != ""
		books = append(books, b)
	}

	for i := range books {
		authors, err := d.bookAuthors(ctx, books[i].ID)
		if err != nil {
			return nil, 0, err
		}
		books[i].Authors = authors

		tags, err := d.bookTags(ctx, books[i].ID)
		if err != nil {
			return nil, 0, err
		}
		books[i].Tags = tags
	}

	return books, total, nil
}

func (d *DB) bookAuthors(ctx context.Context, bookID int) ([]string, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT a.name FROM authors a JOIN book_authors ba ON ba.author_id = a.id WHERE ba.book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func (d *DB) bookTags(ctx context.Context, bookID int) ([]string, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT t.name FROM tags t JOIN book_tags bt ON bt.tag_id = t.id WHERE bt.book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func (d *DB) GetBook(ctx context.Context, id int) (*Book, error) {
	b := &Book{}
	err := d.conn.QueryRowContext(ctx, `SELECT b.id, b.title, b.isbn, b.language, b.publisher,
		b.pub_date, b.description, b.file_path, b.cover_path, b.file_hash, b.file_size,
		b.format, b.rating, b.read_status, COALESCE(s.name,''), b.series_index, b.date_added
		FROM books b LEFT JOIN series s ON s.id = b.series_id WHERE b.id = ?`, id).Scan(
		&b.ID, &b.Title, &b.ISBN, &b.Language, &b.Publisher, &b.PubDate,
		&b.Description, &b.FilePath, &b.CoverPath, &b.FileHash, &b.FileSize,
		&b.Format, &b.Rating, &b.ReadStatus, &b.SeriesName, &b.SeriesIndex, &b.DateAdded)
	if err != nil {
		return nil, err
	}
	b.HasCover = b.CoverPath != ""
	b.Authors, _ = d.bookAuthors(ctx, b.ID)
	b.Tags, _ = d.bookTags(ctx, b.ID)
	return b, nil
}

type InsertBook struct {
	Title       string
	ISBN        string
	Language    string
	Publisher   string
	PubDate     string
	Description string
	FilePath    string
	CoverPath   string
	FileHash    string
	FileSize    int64
	Format      string
	Authors     []string
	SeriesName  string
	SeriesIndex float64
}

func (d *DB) InsertBook(ctx context.Context, in InsertBook) (int, error) {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var seriesID *int
	if in.SeriesName != "" {
		tx.ExecContext(ctx, `INSERT OR IGNORE INTO series(name) VALUES(?)`, in.SeriesName)
		var sid int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM series WHERE name = ?`, in.SeriesName).Scan(&sid); err == nil {
			seriesID = &sid
		}
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO books
		(title, isbn, language, publisher, pub_date, description,
		 file_path, cover_path, file_hash, file_size, format,
		 series_id, series_index)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.Title, in.ISBN, in.Language, in.Publisher, in.PubDate, in.Description,
		in.FilePath, in.CoverPath, in.FileHash, in.FileSize, in.Format,
		seriesID, in.SeriesIndex)
	if err != nil {
		return 0, fmt.Errorf("insert book: %w", err)
	}

	bookID64, _ := res.LastInsertId()
	bookID := int(bookID64)

	for _, name := range in.Authors {
		if name == "" {
			continue
		}
		tx.ExecContext(ctx, `INSERT OR IGNORE INTO authors(name, sort_name) VALUES(?, ?)`, name, sortName(name))
		var aid int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM authors WHERE name = ?`, name).Scan(&aid); err == nil {
			tx.ExecContext(ctx, `INSERT OR IGNORE INTO book_authors(book_id, author_id) VALUES(?, ?)`, bookID, aid)
		}
	}

	return bookID, tx.Commit()
}

func (d *DB) UpdateBook(ctx context.Context, id int, title, readStatus string, rating int) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE books SET title = ?, read_status = ?, rating = ?, date_modified = datetime('now') WHERE id = ?`,
		title, readStatus, rating, id)
	return err
}

type MetadataUpdate struct {
	Title       *string
	Authors     []string
	ISBN        *string
	Publisher   *string
	PubDate     *string
	Language    *string
	Description *string
	CoverPath   *string
}

// UpdateMetadata changes only the fields that are set; non-empty Authors replaces the author list.
func (d *DB) UpdateMetadata(ctx context.Context, id int, m MetadataUpdate) error {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	sets := []string{}
	args := []any{}
	add := func(col string, v *string) {
		if v != nil {
			sets = append(sets, col+" = ?")
			args = append(args, *v)
		}
	}
	add("title", m.Title)
	add("isbn", m.ISBN)
	add("publisher", m.Publisher)
	add("pub_date", m.PubDate)
	add("language", m.Language)
	add("description", m.Description)
	add("cover_path", m.CoverPath)
	sets = append(sets, "date_modified = datetime('now')")
	args = append(args, id)

	if _, err := tx.ExecContext(ctx, `UPDATE books SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return fmt.Errorf("update book: %w", err)
	}

	if len(m.Authors) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM book_authors WHERE book_id = ?`, id); err != nil {
			return err
		}
		for _, name := range m.Authors {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO authors(name, sort_name) VALUES(?, ?)`, name, sortName(name)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO book_authors(book_id, author_id)
				SELECT ?, id FROM authors WHERE name = ?`, id, name); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (d *DB) DeleteBook(ctx context.Context, id int) error {
	_, err := d.conn.ExecContext(ctx, `DELETE FROM books WHERE id = ?`, id)
	return err
}

func (d *DB) HashExists(ctx context.Context, hash string) (bool, error) {
	var n int
	err := d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM books WHERE file_hash = ?`, hash).Scan(&n)
	return n > 0, err
}

func (d *DB) ListAuthors(ctx context.Context) ([]struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT a.id, a.name FROM authors a
		 JOIN book_authors ba ON ba.author_id = a.id
		 GROUP BY a.id ORDER BY a.sort_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	for rows.Next() {
		var a struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (d *DB) ListSeries(ctx context.Context) ([]struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT s.id, s.name FROM series s
		 JOIN books b ON b.series_id = s.id
		 GROUP BY s.id ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	for rows.Next() {
		var s struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

type Tag struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (d *DB) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT t.id, t.name, COUNT(bt.book_id) AS cnt FROM tags t
		 LEFT JOIN book_tags bt ON bt.tag_id = t.id
		 GROUP BY t.id ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (d *DB) CreateTag(ctx context.Context, name string) (int, error) {
	res, err := d.conn.ExecContext(ctx, `INSERT INTO tags(name) VALUES(?)`, name)
	if err != nil {
		return 0, fmt.Errorf("create tag: %w", err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (d *DB) RenameTag(ctx context.Context, id int, name string) error {
	_, err := d.conn.ExecContext(ctx, `UPDATE tags SET name = ? WHERE id = ?`, name, id)
	return err
}

func (d *DB) DeleteTag(ctx context.Context, id int) error {
	_, err := d.conn.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	return err
}

func (d *DB) AddBookTag(ctx context.Context, bookID, tagID int) error {
	_, err := d.conn.ExecContext(ctx, `INSERT OR IGNORE INTO book_tags(book_id, tag_id) VALUES(?, ?)`, bookID, tagID)
	return err
}

func (d *DB) RemoveBookTag(ctx context.Context, bookID, tagID int) error {
	_, err := d.conn.ExecContext(ctx, `DELETE FROM book_tags WHERE book_id = ? AND tag_id = ?`, bookID, tagID)
	return err
}

func (d *DB) GetOrCreateTag(ctx context.Context, name string) (int, error) {
	var id int
	err := d.conn.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ? COLLATE NOCASE`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	return d.CreateTag(ctx, name)
}

func sortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) <= 1 {
		return name
	}
	return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
}
