package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/epub"
)

type Library struct {
	db      *db.DB
	baseDir string
}

func New(store *db.DB, baseDir string) *Library {
	return &Library{db: store, baseDir: baseDir}
}

type ImportResult struct {
	BookID int
	Title  string
	Skipped bool
	Reason  string
}

func (lib *Library) Import(ctx context.Context, srcPath string) (*ImportResult, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	hash, size, err := hashFile(f)
	if err != nil {
		return nil, fmt.Errorf("hash file: %w", err)
	}

	exists, err := lib.db.HashExists(ctx, hash)
	if err != nil {
		return nil, err
	}
	if exists {
		return &ImportResult{Skipped: true, Reason: "duplicate (same hash)"}, nil
	}

	ext := strings.ToLower(filepath.Ext(srcPath))
	format := strings.TrimPrefix(ext, ".")

	var meta *epub.Metadata
	if format == "epub" {
		meta, err = epub.Parse(srcPath)
		if err != nil {
			meta = nil
		}
	}

	title := titleFromMeta(meta, srcPath)
	authors := authorsFromMeta(meta)

	authorDir := sanitizePath(joinAuthors(authors))
	if authorDir == "" {
		authorDir = "Desconhecido"
	}
	titleDir := sanitizePath(title)
	if titleDir == "" {
		titleDir = "Sem Título"
	}

	destDir := filepath.Join(lib.baseDir, "library", authorDir, titleDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	destFile := filepath.Join(destDir, sanitizePath(title)+ext)
	if err := copyFile(srcPath, destFile); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	var coverPath string
	if meta != nil && len(meta.CoverData) > 0 {
		coverPath = filepath.Join(destDir, "cover"+meta.CoverExt)
		if err := os.WriteFile(coverPath, meta.CoverData, 0o644); err != nil {
			coverPath = ""
		}
	}

	in := db.InsertBook{
		Title:    title,
		Format:   format,
		FilePath: destFile,
		FileHash: hash,
		FileSize: size,
		CoverPath: coverPath,
		Authors:  authors,
	}
	if meta != nil {
		in.ISBN = meta.ISBN
		in.Language = meta.Language
		in.Publisher = meta.Publisher
		in.PubDate = meta.PubDate
		in.Description = meta.Description
	}

	id, err := lib.db.InsertBook(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("insert book: %w", err)
	}

	return &ImportResult{BookID: id, Title: title}, nil
}

func titleFromMeta(meta *epub.Metadata, path string) string {
	if meta != nil && meta.Title != "" {
		return meta.Title
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func authorsFromMeta(meta *epub.Metadata) []string {
	if meta != nil && len(meta.Authors) > 0 {
		return meta.Authors
	}
	return []string{"Desconhecido"}
}

func joinAuthors(authors []string) string {
	if len(authors) == 0 {
		return ""
	}
	return strings.Join(authors, ", ")
}

func hashFile(f *os.File) (string, int64, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func sanitizePath(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
