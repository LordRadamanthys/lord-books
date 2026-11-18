package server

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/LordRadamanthys/lord-books/internal/freebooks"
)

func (s *server) searchFreeBooks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	results, err := s.free.Search(r.Context(), q.Get("source"), q.Get("q"), q.Get("lang"))
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, map[string]any{"results": results})
}

func (s *server) importFreeBook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"source"`
		ID     string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "source and id required", 400)
		return
	}
	if req.Source != freebooks.Gutenberg && req.Source != freebooks.Standard {
		http.Error(w, "fonte desconhecida", 400)
		return
	}
	path, name, err := s.free.Download(r.Context(), req.Source, req.ID)
	if err != nil {
		http.Error(w, "não foi possível baixar: "+err.Error(), 502)
		return
	}
	defer os.Remove(path)

	res, err := s.lib.Import(r.Context(), path, name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if res.Skipped {
		writeJSON(w, map[string]any{"status": "skipped"})
		return
	}
	writeJSON(w, map[string]any{"status": "imported", "book_id": res.BookID, "title": res.Title})
}
