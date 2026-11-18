package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Delete removes a book from the database and its files from the library: the book,
// its cover and cached conversions. The book's folder (and an emptied author folder)
// is removed only when nothing else is left in it, and nothing outside the library
// folder is ever touched.
func (lib *Library) Delete(ctx context.Context, id int) error {
	book, err := lib.db.GetBook(ctx, id)
	if err != nil {
		return err
	}
	if err := lib.db.DeleteBook(ctx, id); err != nil {
		return err
	}

	root := filepath.Join(lib.baseDir, "library")
	inLibrary := func(p string) bool {
		rel, err := filepath.Rel(root, p)
		return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
	}
	for _, p := range []string{book.FilePath, book.CoverPath} {
		if p != "" && inLibrary(p) {
			os.Remove(p)
		}
	}
	if book.FilePath == "" || !inLibrary(book.FilePath) {
		return nil
	}
	dir := filepath.Dir(book.FilePath)
	if caches, _ := filepath.Glob(filepath.Join(dir, ".lb-convert-*")); len(caches) > 0 {
		for _, c := range caches {
			os.Remove(c)
		}
	}
	os.Remove(filepath.Join(dir, ".DS_Store"))
	// os.Remove fails on non-empty folders, which is exactly the safety we want.
	if os.Remove(dir) == nil {
		if parent := filepath.Dir(dir); inLibrary(parent) {
			os.Remove(filepath.Join(parent, ".DS_Store"))
			os.Remove(parent)
		}
	}
	return nil
}
