package server

import (
	"encoding/json"
	"net/http"

	"github.com/LordRadamanthys/lord-books/internal/devices"
)

func (s *server) listDevices(w http.ResponseWriter, r *http.Request) {
	list, err := devices.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{
		"devices":   list,
		"profiles":  devices.Profiles,
		"converter": devices.FindConverter() != nil,
	})
}

func (s *server) sendToDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device  string `json:"device"`
		Kind    string `json:"kind"`
		Folder  string `json:"folder"`
		BookIDs []int  `json:"book_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.BookIDs) == 0 {
		http.Error(w, "device and book_ids required", 400)
		return
	}
	dev, err := devices.Find(r.Context(), req.Device)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	// The user may override the detected type, e.g. an Xteink SD card in a card reader.
	profile := dev.Profile
	if p, ok := devices.Profiles[devices.Kind(req.Kind)]; ok {
		profile = p
	}
	dest, err := devices.Destination(dev, req.Folder)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	items := make([]devices.Item, 0, len(req.BookIDs))
	missing := []devices.Result{}
	for _, id := range req.BookIDs {
		b, err := s.db.GetBook(r.Context(), id)
		if err != nil || b.FilePath == "" {
			missing = append(missing, devices.Result{BookID: id, Status: "error", Error: "livro não encontrado"})
			continue
		}
		items = append(items, devices.Item{
			BookID:      b.ID,
			Title:       b.Title,
			Authors:     b.Authors,
			Format:      b.Format,
			Path:        b.FilePath,
			Cover:       b.CoverPath,
			Series:      b.SeriesName,
			SeriesIndex: b.SeriesIndex,
			Publisher:   b.Publisher,
			ISBN:        b.ISBN,
			Language:    b.Language,
			PubDate:     b.PubDate,
		})
	}
	results := append(devices.Send(r.Context(), profile, dest, items, devices.FindConverter()), missing...)
	free, _ := devicesFree(r, dev.ID)
	writeJSON(w, map[string]any{"results": results, "free_bytes": free})
}

func devicesFree(r *http.Request, id string) (uint64, bool) {
	d, err := devices.Find(r.Context(), id)
	if err != nil {
		return 0, false
	}
	return d.FreeBytes, true
}

func (s *server) ejectDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device string `json:"device"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	dev, err := devices.Find(r.Context(), req.Device)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if err := devices.Eject(r.Context(), dev); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
