CREATE TABLE IF NOT EXISTS authors (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    sort_name  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS series (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS books (
    id           INTEGER PRIMARY KEY,
    title        TEXT    NOT NULL,
    isbn         TEXT    NOT NULL DEFAULT '',
    language     TEXT    NOT NULL DEFAULT '',
    publisher    TEXT    NOT NULL DEFAULT '',
    pub_date     TEXT    NOT NULL DEFAULT '',
    description  TEXT    NOT NULL DEFAULT '',
    file_path    TEXT    NOT NULL DEFAULT '',
    cover_path   TEXT    NOT NULL DEFAULT '',
    file_hash    TEXT    NOT NULL DEFAULT '',
    file_size    INTEGER NOT NULL DEFAULT 0,
    format       TEXT    NOT NULL DEFAULT '',
    rating       INTEGER NOT NULL DEFAULT 0,
    read_status  TEXT    NOT NULL DEFAULT 'want',
    series_id    INTEGER REFERENCES series(id) ON DELETE SET NULL,
    series_index REAL    NOT NULL DEFAULT 0,
    date_added   TEXT    NOT NULL DEFAULT (datetime('now')),
    date_modified TEXT   NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS book_authors (
    book_id   INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    author_id INTEGER NOT NULL REFERENCES authors(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, author_id)
);

CREATE TABLE IF NOT EXISTS tags (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS book_tags (
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_books_file_hash ON books(file_hash);
CREATE INDEX IF NOT EXISTS idx_books_isbn ON books(isbn);
CREATE INDEX IF NOT EXISTS idx_books_read_status ON books(read_status);
CREATE INDEX IF NOT EXISTS idx_books_title ON books(title COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS reading_progress (
    book_id  INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
    spine    INTEGER NOT NULL DEFAULT 0,
    fraction REAL    NOT NULL DEFAULT 0,
    percent  REAL    NOT NULL DEFAULT 0,
    updated  TEXT    NOT NULL DEFAULT (datetime('now'))
);
