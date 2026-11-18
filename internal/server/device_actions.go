package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/devices"
)

type deviceActionRequest struct {
	Device string   `json:"device"`
	Paths  []string `json:"paths"`
}

type deviceActionResult struct {
	Path   string `json:"path"`
	Status string `json:"status"` // deleted | imported | skipped | error
	BookID int    `json:"book_id,omitempty"`
	Title  string `json:"title,omitempty"`
	Error  string `json:"error,omitempty"`
}

func decodeDeviceAction(w http.ResponseWriter, r *http.Request) (deviceActionRequest, bool) {
	var req deviceActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Device == "" || len(req.Paths) == 0 {
		http.Error(w, "device and paths required", 400)
		return req, false
	}
	return req, true
}

// deleteFromDevice permanently removes books from a USB device or a Wi-Fi reader.
func (s *server) deleteFromDevice(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeDeviceAction(w, r)
	if !ok {
		return
	}
	results := make([]deviceActionResult, 0, len(req.Paths))

	if req.Device == wifiDeviceID {
		sess := s.wireless.Session()
		if sess == nil {
			http.Error(w, "o aparelho sem fio foi desconectado", 404)
			return
		}
		err := sess.DeleteBooks(req.Paths)
		for _, p := range req.Paths {
			res := deviceActionResult{Path: p, Status: "deleted"}
			if err != nil {
				res.Status, res.Error = "error", err.Error()
			}
			results = append(results, res)
		}
		writeJSON(w, map[string]any{"results": results})
		return
	}

	dev, err := devices.Find(r.Context(), req.Device)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	for _, p := range req.Paths {
		res := deviceActionResult{Path: p, Status: "deleted"}
		if err := devices.DeleteBook(dev, p); err != nil {
			res.Status, res.Error = "error", err.Error()
		}
		results = append(results, res)
	}
	writeJSON(w, map[string]any{"results": results})
}

// importFromDevice copies books that exist only on a device into the library.
func (s *server) importFromDevice(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeDeviceAction(w, r)
	if !ok {
		return
	}
	var dev devices.Device
	if req.Device != wifiDeviceID {
		var err error
		if dev, err = devices.Find(r.Context(), req.Device); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
	}

	results := make([]deviceActionResult, 0, len(req.Paths))
	for _, p := range req.Paths {
		res := deviceActionResult{Path: p}
		name := filepath.Base(filepath.FromSlash(p))
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".epub" && ext != ".pdf" {
			res.Status, res.Error = "error", "a biblioteca só aceita EPUB e PDF"
			results = append(results, res)
			continue
		}

		var src string
		if req.Device == wifiDeviceID {
			tmp, err := s.fetchFromWifi(p, ext)
			if err != nil {
				res.Status, res.Error = "error", err.Error()
				results = append(results, res)
				continue
			}
			defer os.Remove(tmp)
			src = tmp
		} else {
			abs, err := devices.BookFile(dev, p)
			if err != nil {
				res.Status, res.Error = "error", err.Error()
				results = append(results, res)
				continue
			}
			src = abs
		}

		imp, err := s.lib.Import(r.Context(), src, name)
		switch {
		case err != nil:
			res.Status, res.Error = "error", err.Error()
		case imp.Skipped:
			res.Status, res.Error = "skipped", "já está na biblioteca"
		default:
			res.Status, res.BookID, res.Title = "imported", imp.BookID, imp.Title
		}
		results = append(results, res)
	}
	writeJSON(w, map[string]any{"results": results})
}

func (s *server) fetchFromWifi(lpath, ext string) (string, error) {
	sess := s.wireless.Session()
	if sess == nil {
		return "", errNoWifiDevice
	}
	f, err := os.CreateTemp("", "lordbooks-wifi-*"+ext)
	if err != nil {
		return "", err
	}
	err = sess.FetchBook(lpath, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
