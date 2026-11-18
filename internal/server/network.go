package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/settings"
	"github.com/LordRadamanthys/lord-books/internal/wireless"
)

func (s *server) networkStatus(w http.ResponseWriter, r *http.Request) {
	addrs := wireless.LocalIPs()
	cfg := s.cfg.Get()
	writeJSON(w, map[string]any{
		"wireless":      s.wireless.Status(),
		"opds":          s.opds.Status(addrs),
		"opds_port":     cfg.OPDSPort,
		"opds_password": cfg.OPDSPassword != "",
		"addresses":     addrs,
	})
}

func (s *server) toggleWireless(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.On {
		if err := s.wireless.Start(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else {
		s.wireless.Stop()
	}
	s.networkStatus(w, r)
}

func (s *server) toggleOPDS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On       bool    `json:"on"`
		Port     int     `json:"port"`
		Password *string `json:"password"` // nil keeps the saved one; "" removes it
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.Port != 0 && (req.Port < 1024 || req.Port > 65535) {
		http.Error(w, "porta inválida (use entre 1024 e 65535)", 400)
		return
	}
	if req.Port != 0 || req.Password != nil {
		err := s.cfg.Update(func(d *settings.Data) {
			if req.Port != 0 {
				d.OPDSPort = req.Port
			}
			if req.Password != nil {
				d.OPDSPassword = strings.TrimSpace(*req.Password)
			}
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	// Restart so a new port or password takes effect.
	s.opds.Stop()
	if req.On {
		cfg := s.cfg.Get()
		if err := s.opds.Start(cfg.OPDSPort, cfg.OPDSPassword); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	s.networkStatus(w, r)
}
