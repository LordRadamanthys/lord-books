package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/LordRadamanthys/lord-books/internal/epub"
)

// DeviceBook is an e-book file found on a device.
type DeviceBook struct {
	Path    string   `json:"path"` // relative to the volume root, "/"-separated
	Format  string   `json:"format"`
	Size    int64    `json:"size"`
	Title   string   `json:"title"`
	Authors []string `json:"authors"`

	abs  string
	mod  time.Time
	hash string // known up front only for files Lord Books just wrote
}

var bookExts = map[string]string{".epub": "epub", ".pdf": "pdf", ".azw3": "azw3", ".azw": "azw", ".mobi": "mobi"}

// Folders that hold reader software, caches or dictionaries rather than the user's books.
var scanSkip = map[string]bool{"system": true, "xtcache": true, "fonts": true, "dictionaries": true}

const maxScanDepth = 8

type fileStamp struct {
	size int64
	mod  time.Time
}

type metaEntry struct {
	stamp   fileStamp
	title   string
	authors []string
}

type hashEntry struct {
	stamp fileStamp
	hash  string
}

// Per-file caches keyed by absolute path. A file is re-read only when its size or
// modification time changes: e-readers exposed over USB can read at ~1 MB/s.
var metaCache, hashCache sync.Map

// Scan lists the e-books on the device with title and authors, from EPUB metadata or
// from "Title - Author" file names for other formats. Contents are not hashed here;
// Hash does that lazily, only for files that could be identical to a given book.
func Scan(ctx context.Context, d Device) ([]DeviceBook, error) {
	books := []DeviceBook{}
	err := filepath.WalkDir(d.Mount, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := e.Name()
		rel, _ := filepath.Rel(d.Mount, p)
		if e.IsDir() {
			if p == d.Mount {
				return nil
			}
			lname := strings.ToLower(name)
			if strings.HasPrefix(name, ".") || systemFolders[lname] || scanSkip[lname] ||
				strings.Count(rel, string(filepath.Separator)) >= maxScanDepth {
				return fs.SkipDir
			}
			return nil
		}
		// "._name" files are macOS metadata twins written on FAT cards, not books.
		if strings.HasPrefix(name, ".") {
			return nil
		}
		format, ok := bookExts[strings.ToLower(filepath.Ext(name))]
		if !ok {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return nil
		}
		stamp := fileStamp{info.Size(), info.ModTime()}
		b := DeviceBook{Path: filepath.ToSlash(rel), Format: format, Size: info.Size(), abs: p, mod: info.ModTime()}

		if c, ok := metaCache.Load(p); ok && c.(metaEntry).stamp == stamp {
			b.Title, b.Authors = c.(metaEntry).title, c.(metaEntry).authors
		} else {
			if format == "epub" {
				if m, err := epub.ParseInfo(p); err == nil {
					b.Title, b.Authors = strings.TrimSpace(m.Title), m.Authors
				}
			}
			if b.Title == "" {
				b.Title, b.Authors = titleAuthorsFromName(name)
			}
			metaCache.Store(p, metaEntry{stamp: stamp, title: b.Title, authors: b.Authors})
		}
		books = append(books, b)
		return nil
	})
	return books, err
}

// BookFile resolves a device-relative path to the absolute path of an e-book file on
// that device, refusing anything outside it or that isn't a regular e-book file.
func BookFile(d Device, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", errors.New("caminho vazio")
	}
	abs, err := Destination(d, rel)
	if err != nil {
		return "", err
	}
	if _, ok := bookExts[strings.ToLower(filepath.Ext(abs))]; !ok {
		return "", errors.New("não é um arquivo de livro")
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return "", errors.New("arquivo não encontrado no dispositivo")
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("não é um arquivo comum")
	}
	return abs, nil
}

// DeleteBook removes an e-book file from the device, with the "._" metadata twin macOS
// leaves next to it on FAT cards.
func DeleteBook(d Device, rel string) error {
	abs, err := BookFile(d, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		return err
	}
	os.Remove(filepath.Join(filepath.Dir(abs), "._"+filepath.Base(abs)))
	metaCache.Delete(abs)
	hashCache.Delete(abs)
	return nil
}

// Hash returns the sha256 of the file's contents, computing it at most once per version.
func (b DeviceBook) Hash() string {
	if b.hash != "" {
		return b.hash
	}
	if b.abs == "" {
		return ""
	}
	stamp := fileStamp{b.Size, b.mod}
	if c, ok := hashCache.Load(b.abs); ok && c.(hashEntry).stamp == stamp {
		return c.(hashEntry).hash
	}
	h, err := hashFile(b.abs)
	if err != nil {
		return ""
	}
	hashCache.Store(b.abs, hashEntry{stamp: stamp, hash: h})
	return h
}

// titleAuthorsFromName reads the "Title - Author, Author.ext" names that Lord Books and
// Calibre write; anything else is taken as a bare title.
func titleAuthorsFromName(name string) (string, []string) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = strings.TrimSuffix(base, ".kepub")
	i := strings.LastIndex(base, " - ")
	if i <= 0 {
		return strings.TrimSpace(base), nil
	}
	var authors []string
	for _, a := range strings.Split(base[i+3:], ",") {
		if a = strings.TrimSpace(a); a != "" {
			authors = append(authors, a)
		}
	}
	return strings.TrimSpace(base[:i]), authors
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Index finds books already on a device, by exact content or by title and author.
type Index struct {
	bySize map[int64][]DeviceBook
	byKey  map[string]DeviceBook
}

func NewIndex(books []DeviceBook) *Index {
	ix := &Index{bySize: map[int64][]DeviceBook{}, byKey: map[string]DeviceBook{}}
	for _, b := range books {
		ix.Add(b)
	}
	return ix
}

func (ix *Index) Add(b DeviceBook) {
	ix.bySize[b.Size] = append(ix.bySize[b.Size], b)
	for _, k := range MatchKeys(b.Title, b.Authors) {
		ix.byKey[k] = b
	}
}

// Match returns the device copy of a book: "exact" when the content is identical,
// "likely" when only the title and an author agree (e.g. Calibre rewrote the file's
// metadata when it sent it), "" when there is none. Only device files of exactly the
// same size are hashed.
func (ix *Index) Match(hash string, size int64, title string, authors []string) (DeviceBook, string) {
	if hash != "" {
		for _, b := range ix.bySize[size] {
			if b.Hash() == hash {
				return b, "exact"
			}
		}
	}
	for _, k := range MatchKeys(title, authors) {
		if b, ok := ix.byKey[k]; ok {
			return b, "likely"
		}
	}
	return DeviceBook{}, ""
}

// MatchKeys builds comparison keys for title+author pairs, ignoring case, accents and
// punctuation. A book without authors only matches on its title.
func MatchKeys(title string, authors []string) []string {
	t := Fold(title)
	if t == "" {
		return nil
	}
	var keys []string
	for _, a := range authors {
		if fa := Fold(a); fa != "" && fa != "desconhecido" {
			keys = append(keys, t+"|"+fa)
		}
	}
	if len(keys) == 0 {
		keys = append(keys, t+"|")
	}
	return keys
}

var accentFolder = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "ý", "y", "ÿ", "y",
)

func Fold(s string) string {
	s = accentFolder.Replace(strings.ToLower(s))
	var b strings.Builder
	space := true
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
