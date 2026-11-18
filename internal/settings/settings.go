// Package settings persists the few preferences that must survive restarts.
package settings

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Data struct {
	LibraryUUID  string `json:"library_uuid"`
	OPDSPort     int    `json:"opds_port"`
	OPDSPassword string `json:"opds_password"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data Data
}

const DefaultOPDSPort = 8080

// Load reads settings.json from dir, filling defaults; a missing file is not an error.
func Load(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "settings.json")}
	raw, err := os.ReadFile(s.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &s.data); err != nil {
			return nil, fmt.Errorf("settings.json inválido: %w", err)
		}
	}
	changed := false
	if s.data.LibraryUUID == "" {
		s.data.LibraryUUID = newUUID()
		changed = true
	}
	if s.data.OPDSPort == 0 {
		s.data.OPDSPort = DefaultOPDSPort
	}
	if changed {
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Get() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

func (s *Store) Update(fn func(*Data)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data)
	return s.save()
}

// save writes atomically; the file holds the OPDS password, so it is user-only.
func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
