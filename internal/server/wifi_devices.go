package server

import (
	"errors"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/devices"
	"github.com/LordRadamanthys/lord-books/internal/wireless"
)

// The reader connected over Wi-Fi shows up in the device list under this id.
const wifiDeviceID = "wifi"

var errNoWifiDevice = errors.New("o aparelho sem fio foi desconectado")

// wifiProfile derives what can be sent from the formats the reader announced.
func wifiProfile(d wireless.Device) devices.Profile {
	p := devices.Profile{Kind: "wireless", Label: "Wi-Fi"}
	for _, f := range []string{"epub", "pdf", "azw3", "mobi"} {
		if slices.Contains(d.Extensions, f) {
			p.Formats = append(p.Formats, f)
		}
	}
	if !slices.Contains(p.Formats, "epub") && slices.Contains(p.Formats, "azw3") {
		p.Convert = map[string]string{"epub": "azw3"}
	}
	return p
}

// wifiDeviceEntry is the wireless reader in the shape the device list uses.
func (s *server) wifiDeviceEntry() map[string]any {
	sess := s.wireless.Session()
	if sess == nil {
		return nil
	}
	d, _ := sess.Snapshot()
	p := wifiProfile(d)
	return map[string]any{
		"id": wifiDeviceID, "name": d.Name, "mount": "", "wireless": true,
		"free_bytes": d.FreeBytes, "total_bytes": 0, "address": d.Address,
		"kind": p.Kind, "label": p.Label, "folder": "", "formats": p.Formats, "convert": p.Convert,
	}
}

func (s *server) wifiBooks(w http.ResponseWriter, r *http.Request) {
	sess := s.wireless.Session()
	if sess == nil {
		http.Error(w, "o aparelho sem fio foi desconectado", 404)
		return
	}
	if err := sess.Refresh(); err != nil {
		http.Error(w, "não foi possível ler os livros do aparelho: "+err.Error(), 500)
		return
	}
	_, onDevice := sess.Snapshot()
	library, _, err := s.db.ListBooks(r.Context(), db.ListFilter{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	byKey := map[string][]db.Book{}
	for _, b := range library {
		for _, k := range devices.MatchKeys(b.Title, b.Authors) {
			byKey[k] = append(byKey[k], b)
		}
	}

	type entry struct {
		wireless.BookOnDevice
		LibraryID int    `json:"library_id,omitempty"`
		Match     string `json:"match,omitempty"`
	}
	out := make([]entry, 0, len(onDevice))
	var total int64
	for _, b := range onDevice {
		e := entry{BookOnDevice: b}
		// Contents can't be hashed over the network: same title/author and same size is
		// treated as the same file.
	match:
		for _, k := range devices.MatchKeys(b.Title, b.Authors) {
			for _, lb := range byKey[k] {
				if lb.FileSize == b.Size {
					e.LibraryID, e.Match = lb.ID, "exact"
					break match
				}
				if e.Match == "" {
					e.LibraryID, e.Match = lb.ID, "likely"
				}
			}
		}
		total += b.Size
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	writeJSON(w, map[string]any{"books": out, "total_size": total})
}

func (s *server) sendToWifi(w http.ResponseWriter, r *http.Request, bookIDs []int, force bool) {
	sess := s.wireless.Session()
	if sess == nil {
		http.Error(w, "o aparelho sem fio foi desconectado", 404)
		return
	}
	dev, onDevice := sess.Snapshot()
	profile := wifiProfile(dev)
	conv := devices.FindConverter()

	known := make([]devices.DeviceBook, 0, len(onDevice))
	for _, b := range onDevice {
		known = append(known, devices.DeviceBook{Path: b.Path, Format: b.Format, Size: b.Size, Title: b.Title, Authors: b.Authors})
	}
	index := devices.NewIndex(known)

	results := make([]devices.Result, 0, len(bookIDs))
	var queue []wireless.Outgoing
	var queued []int // index into results for each queued book
	for _, id := range bookIDs {
		b, err := s.db.GetBook(r.Context(), id)
		if err != nil || b.FilePath == "" {
			results = append(results, devices.Result{BookID: id, Status: "error", Error: "livro não encontrado"})
			continue
		}
		res := devices.Result{BookID: b.ID, Title: b.Title}
		format, src := strings.ToLower(b.Format), b.FilePath
		if !slices.Contains(profile.Formats, format) {
			target, ok := profile.Convert[format]
			if !ok || conv == nil {
				res.Status, res.Error = "unsupported", dev.Name+" não aceita "+strings.ToUpper(format)
				results = append(results, res)
				continue
			}
			out, err := conv.Convert(r.Context(), devices.Item{
				BookID: b.ID, Title: b.Title, Authors: b.Authors, Format: format, Path: b.FilePath,
				Cover: b.CoverPath, Series: b.SeriesName, SeriesIndex: b.SeriesIndex,
				Publisher: b.Publisher, ISBN: b.ISBN, Language: b.Language, PubDate: b.PubDate,
			}, target)
			if err != nil {
				res.Status, res.Error = "error", err.Error()
				results = append(results, res)
				continue
			}
			format, src, res.Converted = target, out, true
		}
		st, err := os.Stat(src)
		if err != nil {
			res.Status, res.Error = "error", err.Error()
			results = append(results, res)
			continue
		}
		lpath := devices.FileName(b.Title, strings.Join(b.Authors, ", "), format)
		res.File = lpath

		if existing := sameFileOnDevice(onDevice, lpath, st.Size()); existing != "" {
			res.Status, res.Existing = "exists", existing
			results = append(results, res)
			continue
		}
		if dup, kind := index.Match("", st.Size(), b.Title, b.Authors); kind != "" && !force {
			res.Status, res.Existing = "duplicate", dup.Path
			results = append(results, res)
			continue
		}

		info := wireless.BookInfo{
			ID: b.ID, Title: b.Title, Authors: b.Authors, Publisher: b.Publisher, PubDate: b.PubDate,
			Language: b.Language, ISBN: b.ISBN, Tags: b.Tags, Series: b.SeriesName,
			SeriesIndex: b.SeriesIndex, Description: b.Description, FileHash: b.FileHash,
			Format: format, DateAdded: b.DateAdded,
		}
		queue = append(queue, wireless.Outgoing{Path: src, Lpath: lpath, Metadata: wireless.Metadata(info)})
		queued = append(queued, len(results))
		results = append(results, res)
		// Same book twice in one request: the second is "exists".
		index.Add(devices.DeviceBook{Path: lpath, Format: format, Size: st.Size(), Title: b.Title, Authors: b.Authors})
		onDevice = append(onDevice, wireless.BookOnDevice{Title: b.Title, Authors: b.Authors, Path: lpath, Size: st.Size(), Format: format})
	}

	if len(queue) > 0 {
		for i, sr := range sess.SendBooks(queue) {
			res := &results[queued[i]]
			if sr.Err != nil {
				res.Status, res.Error = "error", sr.Err.Error()
			} else {
				res.Status, res.File = "sent", sr.Lpath
			}
		}
	}
	d, _ := sess.Snapshot()
	writeJSON(w, map[string]any{"results": results, "free_bytes": d.FreeBytes})
}

func sameFileOnDevice(books []wireless.BookOnDevice, lpath string, size int64) string {
	for _, b := range books {
		if strings.EqualFold(b.Path, lpath) && b.Size == size {
			return b.Path
		}
	}
	return ""
}
