package server

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/epub"
)

// readerInfo gives the reader a book's layout (spine and table of contents) and the
// last saved position.
func (s *server) readerInfo(w http.ResponseWriter, r *http.Request) {
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
	progress, _ := s.db.GetProgress(r.Context(), id)
	resp := map[string]any{"id": book.ID, "title": book.Title, "authors": book.Authors, "format": book.Format, "progress": progress}
	if book.Format == "epub" {
		st, err := epub.ReadStructure(book.FilePath)
		if err != nil {
			http.Error(w, "não foi possível abrir o livro: "+err.Error(), 422)
			return
		}
		resp["spine"], resp["toc"] = st.Spine, st.TOC
	}
	writeJSON(w, resp)
}

// readerContent serves a file from inside an EPUB to the reader's iframe. Book content
// is untrusted, so the response forbids scripts and plugins; the iframe is sandboxed
// without allow-scripts as well.
func (s *server) readerContent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	book, err := s.db.GetBook(r.Context(), id)
	if err != nil || book.Format != "epub" {
		http.Error(w, "not found", 404)
		return
	}
	name := path.Clean("/" + r.PathValue("path"))[1:]
	rc, z, size, err := epub.OpenFile(book.FilePath, name)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	defer z.Close()
	defer rc.Close()

	ext := strings.ToLower(path.Ext(name))
	ctype := mime.TypeByExtension(ext)
	switch ext {
	case ".xhtml", ".xht", ".xml":
		ctype = "application/xhtml+xml; charset=utf-8"
		// Not every EPUB is well-formed XML; the reader retries those as HTML.
		if r.URL.Query().Get("html") == "1" {
			ctype = "text/html; charset=utf-8"
		}
	case ".html", ".htm":
		ctype = "text/html; charset=utf-8"
	case ".css":
		ctype = "text/css; charset=utf-8"
	case ".svg":
		ctype = "image/svg+xml"
	case ".otf", ".ttf", ".woff", ".woff2":
		ctype = "font/" + ext[1:]
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'self' data: blob:; script-src 'none'; object-src 'none'; "+
		"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; frame-ancestors 'self'; form-action 'none'")
	h.Set("Cache-Control", "private, max-age=300")
	io.Copy(w, rc)
}

func (s *server) saveProgress(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	var p db.Progress
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.Spine < 0 || p.Fraction < 0 || p.Fraction > 1 || p.Percent < 0 || p.Percent > 100 {
		http.Error(w, "posição inválida", 400)
		return
	}
	if _, err := s.db.GetBook(r.Context(), id); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if err := s.db.SaveProgress(r.Context(), id, p); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
