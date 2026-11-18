package db

import (
	"context"
	"database/sql"
	"errors"
)

// Progress is where the reader left a book: a document in the spine and how far
// through it, plus the overall percentage shown in the library.
type Progress struct {
	Spine    int     `json:"spine"`
	Fraction float64 `json:"fraction"`
	Percent  float64 `json:"percent"`
	Updated  string  `json:"updated"`
}

// GetProgress returns nil when the book was never opened in the reader.
func (d *DB) GetProgress(ctx context.Context, bookID int) (*Progress, error) {
	var p Progress
	err := d.conn.QueryRowContext(ctx, `SELECT spine, fraction, percent, updated FROM reading_progress WHERE book_id = ?`, bookID).
		Scan(&p.Spine, &p.Fraction, &p.Percent, &p.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SaveProgress records the position and moves a "want to read" book to "reading".
func (d *DB) SaveProgress(ctx context.Context, bookID int, p Progress) error {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO reading_progress(book_id, spine, fraction, percent, updated)
		VALUES(?, ?, ?, ?, datetime('now'))
		ON CONFLICT(book_id) DO UPDATE SET spine = excluded.spine, fraction = excluded.fraction,
			percent = excluded.percent, updated = excluded.updated`,
		bookID, p.Spine, p.Fraction, p.Percent); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE books SET read_status = 'reading' WHERE id = ? AND read_status = 'want'`, bookID); err != nil {
		return err
	}
	return tx.Commit()
}
