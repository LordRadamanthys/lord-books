package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/library"
	"github.com/LordRadamanthys/lord-books/internal/metadata"
)

func New(web fs.FS, store *db.DB, lib *library.Library, meta *metadata.Client) http.Handler {
	s := &server{db: store, lib: lib, meta: meta}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/books/upload", s.uploadBooks)
	mux.HandleFunc("GET /api/books", s.listBooks)
	mux.HandleFunc("GET /api/books/{id}", s.getBook)
	mux.HandleFunc("PUT /api/books/{id}", s.updateBook)
	mux.HandleFunc("DELETE /api/books/{id}", s.deleteBook)
	mux.HandleFunc("GET /api/books/{id}/cover", s.bookCover)
	mux.HandleFunc("GET /api/books/{id}/file", s.bookFile)

	mux.HandleFunc("GET /api/authors", s.listAuthors)
	mux.HandleFunc("GET /api/series", s.listSeries)
	mux.HandleFunc("GET /api/tags", s.listTags)
	mux.HandleFunc("POST /api/tags", s.createTag)
	mux.HandleFunc("PUT /api/tags/{id}", s.updateTag)
	mux.HandleFunc("DELETE /api/tags/{id}", s.deleteTag)
	mux.HandleFunc("POST /api/books/{id}/tags", s.addBookTag)
	mux.HandleFunc("DELETE /api/books/{id}/tags/{tagID}", s.removeBookTag)
	mux.HandleFunc("GET /api/metadata/search", s.searchMetadata)
	mux.HandleFunc("POST /api/books/{id}/metadata", s.applyMetadata)

	mux.Handle("/", http.FileServerFS(web))
	return mux
}

type server struct {
	db   *db.DB
	lib  *library.Library
	meta *metadata.Client
}

func (s *server) listBooks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	authorID, _ := strconv.Atoi(q.Get("author_id"))
	seriesID, _ := strconv.Atoi(q.Get("series_id"))
	tagID, _ := strconv.Atoi(q.Get("tag_id"))

	books, total, err := s.db.ListBooks(r.Context(), db.ListFilter{
		Query:      q.Get("q"),
		ReadStatus: q.Get("status"),
		AuthorID:   authorID,
		SeriesID:   seriesID,
		TagID:      tagID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"books": books, "total": total})
}

func (s *server) getBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	book, err := s.db.GetBook(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	writeJSON(w, book)
}

func (s *server) updateBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	var req struct {
		Title      string `json:"title"`
		ReadStatus string `json:"read_status"`
		Rating     int    `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if err := s.db.UpdateBook(r.Context(), id, req.Title, req.ReadStatus, req.Rating); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *server) deleteBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	if err := s.db.DeleteBook(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *server) bookCover(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	book, err := s.db.GetBook(r.Context(), id)
	if err != nil || book.CoverPath == "" {
		http.Error(w, "no cover", 404)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, book.CoverPath)
}

func (s *server) bookFile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	book, err := s.db.GetBook(r.Context(), id)
	if err != nil || book.FilePath == "" {
		http.Error(w, "no file", 404)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+book.Title+"."+book.Format+`"`)
	http.ServeFile(w, r, book.FilePath)
}

func (s *server) listAuthors(w http.ResponseWriter, r *http.Request) {
	authors, err := s.db.ListAuthors(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, authors)
}

func (s *server) listSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.db.ListSeries(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, series)
}

func (s *server) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.db.ListTags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, tags)
}

func (s *server) uploadBooks(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 500 << 20 // 500 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "file too large or bad form", 400)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "no files", 400)
		return
	}

	var results []map[string]any
	for _, fh := range files {
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if ext != ".epub" && ext != ".pdf" {
			results = append(results, map[string]any{
				"filename": fh.Filename,
				"error":    "unsupported format (epub/pdf only)",
			})
			continue
		}

		src, err := fh.Open()
		if err != nil {
			results = append(results, map[string]any{
				"filename": fh.Filename,
				"error":    err.Error(),
			})
			continue
		}

		tmp, err := os.CreateTemp("", "lordbooks-upload-*"+ext)
		if err != nil {
			src.Close()
			results = append(results, map[string]any{
				"filename": fh.Filename,
				"error":    err.Error(),
			})
			continue
		}

		_, err = io.Copy(tmp, src)
		src.Close()
		tmp.Close()
		if err != nil {
			os.Remove(tmp.Name())
			results = append(results, map[string]any{
				"filename": fh.Filename,
				"error":    err.Error(),
			})
			continue
		}

		res, err := s.lib.Import(r.Context(), tmp.Name())
		os.Remove(tmp.Name())
		if err != nil {
			results = append(results, map[string]any{
				"filename": fh.Filename,
				"error":    err.Error(),
			})
			continue
		}

		entry := map[string]any{"filename": fh.Filename}
		if res.Skipped {
			entry["skipped"] = true
			entry["reason"] = res.Reason
		} else {
			entry["book_id"] = res.BookID
			entry["title"] = res.Title
		}
		results = append(results, entry)
	}

	writeJSON(w, map[string]any{"results": results})
}

func (s *server) createTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	id, err := s.db.CreateTag(r.Context(), req.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"id": id, "name": req.Name})
}

func (s *server) updateTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	if err := s.db.RenameTag(r.Context(), id, req.Name); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *server) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	if err := s.db.DeleteTag(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *server) addBookTag(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	var req struct {
		TagID int    `json:"tag_id"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	tagID := req.TagID
	if tagID == 0 && req.Name != "" {
		tagID, err = s.db.GetOrCreateTag(r.Context(), req.Name)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if tagID == 0 {
		http.Error(w, "tag_id or name required", 400)
		return
	}
	if err := s.db.AddBookTag(r.Context(), bookID, tagID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"tag_id": tagID})
}

func (s *server) removeBookTag(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid book id", 400)
		return
	}
	tagID, err := strconv.Atoi(r.PathValue("tagID"))
	if err != nil {
		http.Error(w, "invalid tag id", 400)
		return
	}
	if err := s.db.RemoveBookTag(r.Context(), bookID, tagID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *server) searchMetadata(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	isbn := strings.NewReplacer("-", "", " ", "").Replace(r.URL.Query().Get("isbn"))
	if q == "" && isbn == "" {
		http.Error(w, "q or isbn required", 400)
		return
	}
	results, warnings := s.meta.Search(r.Context(), q, isbn)
	if results == nil {
		results = []metadata.Candidate{}
	}
	writeJSON(w, map[string]any{"results": results, "warnings": warnings})
}

func (s *server) applyMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	book, err := s.db.GetBook(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}

	var req struct {
		Title       *string  `json:"title"`
		Authors     []string `json:"authors"`
		ISBN        *string  `json:"isbn"`
		Publisher   *string  `json:"publisher"`
		PubDate     *string  `json:"pub_date"`
		Language    *string  `json:"language"`
		Description *string  `json:"description"`
		CoverURL    string   `json:"cover_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		http.Error(w, "title cannot be empty", 400)
		return
	}

	upd := db.MetadataUpdate{
		Title:       req.Title,
		Authors:     req.Authors,
		ISBN:        req.ISBN,
		Publisher:   req.Publisher,
		PubDate:     req.PubDate,
		Language:    req.Language,
		Description: req.Description,
	}

	var oldCover string
	if req.CoverURL != "" {
		data, ext, err := s.meta.DownloadCover(r.Context(), req.CoverURL)
		if err != nil {
			http.Error(w, "capa: "+err.Error(), 502)
			return
		}
		coverPath := filepath.Join(filepath.Dir(book.FilePath), "cover"+ext)
		if err := os.WriteFile(coverPath, data, 0o644); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if book.CoverPath != "" && book.CoverPath != coverPath {
			oldCover = book.CoverPath
		}
		upd.CoverPath = &coverPath
	}

	if err := s.db.UpdateMetadata(r.Context(), id, upd); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if oldCover != "" {
		os.Remove(oldCover)
	}

	updated, err := s.db.GetBook(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, updated)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
