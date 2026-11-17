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
	calibre calibreJob
}

func New(store *db.DB, baseDir string) *Library {
	return &Library{db: store, baseDir: baseDir}
}

type ImportResult struct {
	BookID  int
	Title   string
	Skipped bool
	Reason  string
}

// Import adds srcPath to the library. origName is the file's original name, used for the
// title when the file has no metadata (uploads arrive as temp files with random names).
func (lib *Library) Import(ctx context.Context, srcPath, origName string) (*ImportResult, error) {
	hash, size, dup, err := lib.checkDuplicate(ctx, srcPath)
	if err != nil || dup {
		return dupResult(dup), err
	}

	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(srcPath)), ".")
	var meta *epub.Metadata
	if format == "epub" {
		meta, _ = epub.Parse(srcPath)
	}

	in := db.InsertBook{
		Title:   titleFromMeta(meta, origName),
		Authors: authorsFromMeta(meta),
	}
	var cover []byte
	var coverExt string
	if meta != nil {
		in.ISBN = meta.ISBN
		in.Language = meta.Language
		in.Publisher = meta.Publisher
		in.PubDate = meta.PubDate
		in.Description = meta.Description
		cover, coverExt = meta.CoverData, meta.CoverExt
	}
	return lib.store(ctx, srcPath, hash, size, in, cover, coverExt)
}

func dupResult(dup bool) *ImportResult {
	if !dup {
		return nil
	}
	return &ImportResult{Skipped: true, Reason: "duplicate (same hash)"}
}

func (lib *Library) checkDuplicate(ctx context.Context, srcPath string) (hash string, size int64, dup bool, err error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", 0, false, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	hash, size, err = hashFile(f)
	if err != nil {
		return "", 0, false, fmt.Errorf("hash file: %w", err)
	}
	dup, err = lib.db.HashExists(ctx, hash)
	return hash, size, dup, err
}

// store copies the file into the library tree, writes the cover and inserts the book.
func (lib *Library) store(ctx context.Context, srcPath, hash string, size int64, in db.InsertBook, cover []byte, coverExt string) (*ImportResult, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))

	authorDir := sanitizePath(joinAuthors(in.Authors))
	if authorDir == "" {
		authorDir = "Desconhecido"
	}
	titleDir := sanitizePath(in.Title)
	if titleDir == "" {
		titleDir = "Sem Título"
	}

	destDir, err := uniqueDir(filepath.Join(lib.baseDir, "library", authorDir), titleDir)
	if err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	destFile := filepath.Join(destDir, titleDir+ext)
	if err := copyFile(srcPath, destFile); err != nil {
		os.RemoveAll(destDir)
		return nil, fmt.Errorf("copy file: %w", err)
	}

	if len(cover) > 0 {
		coverPath := filepath.Join(destDir, "cover"+coverExt)
		if os.WriteFile(coverPath, cover, 0o644) == nil {
			in.CoverPath = coverPath
		}
	}

	in.Format = strings.TrimPrefix(ext, ".")
	in.FilePath = destFile
	in.FileHash = hash
	in.FileSize = size

	id, err := lib.db.InsertBook(ctx, in)
	if err != nil {
		os.RemoveAll(destDir)
		return nil, fmt.Errorf("insert book: %w", err)
	}
	return &ImportResult{BookID: id, Title: in.Title}, nil
}

// uniqueDir creates parent/name, or parent/name (2), (3)... if it already exists,
// so two editions with the same title never share (and overwrite) a folder.
func uniqueDir(parent, name string) (string, error) {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	for i := 1; i < 1000; i++ {
		dir := filepath.Join(parent, name)
		if i > 1 {
			dir = filepath.Join(parent, fmt.Sprintf("%s (%d)", name, i))
		}
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return dir, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("too many folders named %q", name)
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
