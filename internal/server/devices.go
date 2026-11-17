package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/db"

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

func (s *server) deviceFolders(w http.ResponseWriter, r *http.Request) {
	dev, err := devices.Find(r.Context(), r.URL.Query().Get("device"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	rel, folders, err := devices.Folders(dev, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "não foi possível abrir a pasta: "+err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"path": rel, "folders": folders})
}

// deviceBooks lists the books on a device and marks the ones that are in the library.
func (s *server) deviceBooks(w http.ResponseWriter, r *http.Request) {
	dev, err := devices.Find(r.Context(), r.URL.Query().Get("device"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	onDevice, err := devices.Scan(r.Context(), dev)
	if err != nil {
		http.Error(w, "não foi possível ler os livros do dispositivo: "+err.Error(), 500)
		return
	}
	library, _, err := s.db.ListBooks(r.Context(), db.ListFilter{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Device files are hashed only when a library book has exactly the same size: reading
	// whole files from an e-reader over USB is slow.
	bySize := map[int64][]db.Book{}
	byKey := map[string]int{}
	for _, b := range library {
		if b.FileHash != "" {
			bySize[b.FileSize] = append(bySize[b.FileSize], b)
		}
		for _, k := range devices.MatchKeys(b.Title, b.Authors) {
			byKey[k] = b.ID
		}
	}

	type entry struct {
		devices.DeviceBook
		LibraryID int    `json:"library_id,omitempty"`
		Match     string `json:"match,omitempty"` // exact | likely
	}
	out := make([]entry, 0, len(onDevice))
	var total int64
	for _, b := range onDevice {
		e := entry{DeviceBook: b}
		for _, lb := range bySize[b.Size] {
			if b.Hash() == lb.FileHash {
				e.LibraryID, e.Match = lb.ID, "exact"
				break
			}
		}
		if e.Match == "" {
			for _, k := range devices.MatchKeys(b.Title, b.Authors) {
				if id, ok := byKey[k]; ok {
					e.LibraryID, e.Match = id, "likely"
					break
				}
			}
		}
		total += b.Size
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	writeJSON(w, map[string]any{"books": out, "total_size": total})
}

func (s *server) sendToDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device  string `json:"device"`
		Kind    string `json:"kind"`
		Folder  string `json:"folder"`
		BookIDs []int  `json:"book_ids"`
		Force   bool   `json:"force"`
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
			Hash:        b.FileHash,
			Cover:       b.CoverPath,
			Series:      b.SeriesName,
			SeriesIndex: b.SeriesIndex,
			Publisher:   b.Publisher,
			ISBN:        b.ISBN,
			Language:    b.Language,
			PubDate:     b.PubDate,
		})
	}
	onDevice, err := devices.Scan(r.Context(), dev)
	if err != nil {
		http.Error(w, "não foi possível ler os livros do dispositivo: "+err.Error(), 500)
		return
	}
	results := append(devices.Send(r.Context(), profile, dest, items, devices.SendOptions{
		Converter: devices.FindConverter(),
		OnDevice:  devices.NewIndex(onDevice),
		Force:     req.Force,
		Mount:     dev.Mount,
	}), missing...)
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
