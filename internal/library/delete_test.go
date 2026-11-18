package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/LordRadamanthys/lord-books/internal/db"
)

func TestDeleteRemovesBookFiles(t *testing.T) {
	base := t.TempDir()
	store, err := db.Open(filepath.Join(base, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lib := New(store, base)

	src := filepath.Join(t.TempDir(), "Livro Um.pdf")
	os.WriteFile(src, []byte("%PDF-1.4 one"), 0o644)
	res, err := lib.Import(context.Background(), src, "Livro Um.pdf")
	if err != nil {
		t.Fatal(err)
	}
	book, _ := store.GetBook(context.Background(), res.BookID)
	dir := filepath.Dir(book.FilePath)
	os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("img"), 0o644)
	os.WriteFile(filepath.Join(dir, ".lb-convert-abc.azw3"), []byte("conv"), 0o644)
	cover := filepath.Join(dir, "cover.jpg")
	store.UpdateMetadata(context.Background(), book.ID, db.MetadataUpdate{CoverPath: &cover})

	// A second book by the same (unknown) author keeps the author folder alive.
	src2 := filepath.Join(t.TempDir(), "Livro Dois.pdf")
	os.WriteFile(src2, []byte("%PDF-1.4 two"), 0o644)
	if _, err := lib.Import(context.Background(), src2, "Livro Dois.pdf"); err != nil {
		t.Fatal(err)
	}

	if err := lib.Delete(context.Background(), book.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("book folder should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(dir)); err != nil {
		t.Errorf("author folder with another book must stay: %v", err)
	}
	if _, err := store.GetBook(context.Background(), book.ID); err == nil {
		t.Error("book still in database")
	}
}

func TestDeleteKeepsFoldersWithForeignFiles(t *testing.T) {
	base := t.TempDir()
	store, _ := db.Open(filepath.Join(base, "test.db"))
	defer store.Close()
	lib := New(store, base)
	src := filepath.Join(t.TempDir(), "X.pdf")
	os.WriteFile(src, []byte("%PDF x"), 0o644)
	res, _ := lib.Import(context.Background(), src, "X.pdf")
	book, _ := store.GetBook(context.Background(), res.BookID)
	notes := filepath.Join(filepath.Dir(book.FilePath), "minhas-notas.txt")
	os.WriteFile(notes, []byte("keep me"), 0o644)

	lib.Delete(context.Background(), book.ID)
	if _, err := os.Stat(notes); err != nil {
		t.Errorf("a file the user put there must not be deleted: %v", err)
	}
}
