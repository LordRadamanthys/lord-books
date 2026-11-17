package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/LordRadamanthys/lord-books/internal/calibre"
	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/epub"
	"github.com/LordRadamanthys/lord-books/internal/metadata"
)

type CalibreProgress struct {
	Running     bool     `json:"running"`
	Path        string   `json:"path"`
	Total       int      `json:"total"`
	Done        int      `json:"done"`
	Imported    int      `json:"imported"`
	Skipped     int      `json:"skipped"`
	Unsupported int      `json:"unsupported"`
	Errors      []string `json:"errors"`
}

type calibreJob struct {
	mu       sync.Mutex
	progress CalibreProgress
}

func (lib *Library) CalibreStatus() CalibreProgress {
	lib.calibre.mu.Lock()
	defer lib.calibre.mu.Unlock()
	p := lib.calibre.progress
	p.Errors = append([]string(nil), p.Errors...)
	return p
}

// StartCalibreImport reads the Calibre library synchronously (so a bad path fails
// right away) and copies the books in the background; poll CalibreStatus for progress.
func (lib *Library) StartCalibreImport(ctx context.Context, dir string) (CalibreProgress, error) {
	job := &lib.calibre
	job.mu.Lock()
	if job.progress.Running {
		job.mu.Unlock()
		return CalibreProgress{}, fmt.Errorf("já existe uma importação em andamento")
	}
	job.progress = CalibreProgress{Running: true, Path: dir}
	job.mu.Unlock()

	books, err := calibre.Read(ctx, dir)
	if err != nil {
		job.mu.Lock()
		job.progress.Running = false
		job.mu.Unlock()
		return CalibreProgress{}, err
	}

	job.mu.Lock()
	job.progress.Total = len(books)
	job.mu.Unlock()

	go lib.runCalibreImport(books)
	return lib.CalibreStatus(), nil
}

func (lib *Library) runCalibreImport(books []calibre.Book) {
	job := &lib.calibre
	ctx := context.Background()
	for _, b := range books {
		res, err := lib.importCalibreBook(ctx, b)
		job.mu.Lock()
		job.progress.Done++
		switch {
		case err != nil:
			job.progress.Errors = append(job.progress.Errors, fmt.Sprintf("%s: %v", b.Title, err))
		case res == nil:
			job.progress.Unsupported++
		case res.Skipped:
			job.progress.Skipped++
		default:
			job.progress.Imported++
		}
		job.mu.Unlock()
	}
	job.mu.Lock()
	job.progress.Running = false
	job.mu.Unlock()
}

// importCalibreBook returns (nil, nil) for books with no EPUB/PDF file.
func (lib *Library) importCalibreBook(ctx context.Context, b calibre.Book) (*ImportResult, error) {
	if b.FilePath == "" {
		return nil, nil
	}
	hash, size, dup, err := lib.checkDuplicate(ctx, b.FilePath)
	if err != nil || dup {
		return dupResult(dup), err
	}

	authors := b.Authors
	if len(authors) == 0 {
		authors = []string{"Desconhecido"}
	}
	in := db.InsertBook{
		Title:       b.Title,
		Authors:     authors,
		ISBN:        b.ISBN,
		Publisher:   b.Publisher,
		PubDate:     b.PubDate,
		Language:    metadata.ShortLang(b.Language),
		Description: b.Description,
		SeriesName:  b.SeriesName,
		SeriesIndex: b.SeriesIndex,
		Tags:        b.Tags,
		Rating:      b.Rating,
	}
	if b.Read {
		in.ReadStatus = "read"
	}

	var cover []byte
	var coverExt string
	if b.CoverPath != "" {
		if data, err := os.ReadFile(b.CoverPath); err == nil {
			cover, coverExt = data, filepath.Ext(b.CoverPath)
		}
	}
	if len(cover) == 0 && b.Format == "epub" {
		if meta, err := epub.Parse(b.FilePath); err == nil {
			cover, coverExt = meta.CoverData, meta.CoverExt
		}
	}
	return lib.store(ctx, b.FilePath, hash, size, in, cover, coverExt)
}
