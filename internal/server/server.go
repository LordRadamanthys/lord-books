package server

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/calibre"
	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/dialog"
	"github.com/LordRadamanthys/lord-books/internal/library"
	"github.com/LordRadamanthys/lord-books/internal/metadata"
	"github.com/LordRadamanthys/lord-books/internal/opds"
	"github.com/LordRadamanthys/lord-books/internal/settings"
	"github.com/LordRadamanthys/lord-books/internal/wireless"
)

// New returns the app's HTTP handler and a function that stops the Wi-Fi services
// (wireless device connection and OPDS catalog) if they were started.
func New(web fs.FS, store *db.DB, lib *library.Library, meta *metadata.Client, cfg *settings.Store) (http.Handler, func()) {
	s := &server{db: store, lib: lib, meta: meta, cfg: cfg, opds: opds.New(store)}
	s.wireless = wireless.New(cfg.Get().LibraryUUID, func() int { return s.cfg.Get().OPDSPort })
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
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /api/import/calibre", s.calibreStatus)
	mux.HandleFunc("POST /api/import/calibre", s.startCalibreImport)
	mux.HandleFunc("POST /api/dialog/folder", s.pickFolder)
	mux.HandleFunc("GET /api/metadata/search", s.searchMetadata)
	mux.HandleFunc("POST /api/books/{id}/metadata", s.applyMetadata)
	mux.HandleFunc("PUT /api/books/{id}/cover", s.uploadCover)
	mux.HandleFunc("GET /api/devices", s.listDevices)
	mux.HandleFunc("GET /api/devices/folders", s.deviceFolders)
	mux.HandleFunc("GET /api/devices/books", s.deviceBooks)
	mux.HandleFunc("POST /api/devices/send", s.sendToDevice)
	mux.HandleFunc("POST /api/devices/eject", s.ejectDevice)
	mux.HandleFunc("POST /api/devices/delete", s.deleteFromDevice)
	mux.HandleFunc("POST /api/devices/import", s.importFromDevice)
	mux.HandleFunc("GET /api/network", s.networkStatus)
	mux.HandleFunc("POST /api/network/wireless", s.toggleWireless)
	mux.HandleFunc("POST /api/network/opds", s.toggleOPDS)

	mux.Handle("/", http.FileServerFS(web))
	stop := func() {
		s.wireless.Stop()
		s.opds.Stop()
	}
	return guard(mux), stop
}

type server struct {
	db       *db.DB
	lib      *library.Library
	meta     *metadata.Client
	cfg      *settings.Store
	wireless *wireless.Server
	opds     *opds.Server
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
		Sort:       q.Get("sort"),
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
	if err := s.lib.Delete(r.Context(), id); err != nil {
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
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": book.Title + "." + book.Format}))
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

		res, err := s.lib.Import(r.Context(), tmp.Name(), filepath.Base(fh.Filename))
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

func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	st, err := s.db.Stats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, st)
}

func (s *server) calibreStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"default_path": calibre.DefaultPath(),
		"progress":     s.lib.CalibreStatus(),
	})
}

func (s *server) startCalibreImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	dir := strings.TrimSpace(req.Path)
	if strings.HasPrefix(dir, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
	}
	dir = strings.TrimSuffix(filepath.Clean(dir), string(filepath.Separator)+"metadata.db")
	if dir == "" || !calibre.IsLibrary(dir) {
		http.Error(w, "nenhuma biblioteca do Calibre (metadata.db) nessa pasta", 400)
		return
	}
	progress, err := s.lib.StartCalibreImport(r.Context(), dir)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, progress)
}

// uploadCover replaces a book's cover with the image sent as the raw request body.
func (s *server) uploadCover(w http.ResponseWriter, r *http.Request) {
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
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 15<<20))
	if err != nil {
		http.Error(w, "imagem muito grande (máx. 15 MB)", 413)
		return
	}
	var ext string
	switch http.DetectContentType(data) {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	default:
		http.Error(w, "o arquivo não é uma imagem JPG, PNG, WEBP ou GIF", 415)
		return
	}
	coverPath := filepath.Join(filepath.Dir(book.FilePath), "cover"+ext)
	if err := os.WriteFile(coverPath, data, 0o644); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.db.UpdateMetadata(r.Context(), id, db.MetadataUpdate{CoverPath: &coverPath}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if book.CoverPath != "" && book.CoverPath != coverPath {
		os.Remove(book.CoverPath)
	}
	w.WriteHeader(204)
}

func (s *server) pickFolder(w http.ResponseWriter, r *http.Request) {
	// Opening a window on the user's desktop must only be triggered by our own page:
	// a JSON body forces a CORS preflight for other origins, and Sec-Fetch-Site rejects them outright.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		http.Error(w, "forbidden", 403)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "json required", 415)
		return
	}
	var req struct {
		Start string `json:"start"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	start := strings.TrimSpace(req.Start)
	if st, err := os.Stat(start); err != nil || !st.IsDir() {
		start, _ = os.UserHomeDir()
	}

	path, err := dialog.PickFolder(r.Context(), "Selecione a pasta da biblioteca do Calibre", start)
	switch {
	case errors.Is(err, dialog.ErrCanceled):
		writeJSON(w, map[string]any{"canceled": true})
	case err != nil:
		http.Error(w, err.Error(), 501)
	default:
		writeJSON(w, map[string]any{"path": path, "is_calibre": calibre.IsLibrary(path)})
	}
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
		SeriesName  *string  `json:"series"`
		SeriesIndex *float64 `json:"series_index"`
		CoverURL    string   `json:"cover_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" {
			http.Error(w, "o título não pode ficar vazio", 400)
			return
		}
		req.Title = &t
	}
	if req.Authors != nil {
		var clean []string
		for _, a := range req.Authors {
			if a = strings.TrimSpace(a); a != "" {
				clean = append(clean, a)
			}
		}
		if len(clean) == 0 {
			http.Error(w, "informe ao menos um autor", 400)
			return
		}
		req.Authors = clean
	}

	upd := db.MetadataUpdate{
		Title:       req.Title,
		Authors:     req.Authors,
		ISBN:        req.ISBN,
		Publisher:   req.Publisher,
		PubDate:     req.PubDate,
		Language:    req.Language,
		Description: req.Description,
		SeriesName:  req.SeriesName,
		SeriesIndex: req.SeriesIndex,
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
