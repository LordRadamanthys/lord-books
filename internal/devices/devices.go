// Package devices finds e-readers and memory cards mounted as USB mass storage and
// copies books onto them.
package devices

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Kind string

const (
	Kindle  Kind = "kindle"
	Kobo    Kind = "kobo"
	Xteink  Kind = "xteink"
	Generic Kind = "generic"
)

type Profile struct {
	Kind    Kind     `json:"kind"`
	Label   string   `json:"label"`
	Folder  string   `json:"folder"`  // default destination, relative to the volume root
	Formats []string `json:"formats"` // formats the device can open
	// Convert maps a format the device can't open to one it can, done with Calibre.
	Convert map[string]string `json:"convert,omitempty"`
	Note    string            `json:"note,omitempty"` // shown when no converter is installed
}

var Profiles = map[Kind]Profile{
	Kindle: {Kind: Kindle, Label: "Kindle", Folder: "documents", Formats: []string{"azw3", "mobi", "pdf"},
		Convert: map[string]string{"epub": "azw3"},
		Note:    "O Kindle não abre EPUB copiado por USB. Instale o Calibre para converter EPUB em AZW3 automaticamente."},
	Kobo:    {Kind: Kobo, Label: "Kobo", Folder: "Lord Books", Formats: []string{"epub", "pdf"}},
	Xteink:  {Kind: Xteink, Label: "Xteink", Folder: "", Formats: []string{"epub"}},
	Generic: {Kind: Generic, Label: "Cartão SD / USB", Folder: "Lord Books", Formats: []string{"epub", "pdf"}},
}

type Device struct {
	ID         string `json:"id"` // the mount path; also what the client sends back
	Name       string `json:"name"`
	Mount      string `json:"mount"`
	FreeBytes  uint64 `json:"free_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
	Profile
}

// List returns the removable volumes mounted right now, each identified by its contents.
func List(ctx context.Context) ([]Device, error) {
	vols, err := listVolumes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(vols))
	for _, v := range vols {
		d := Device{ID: v.mount, Name: v.name, Mount: v.mount, Profile: Profiles[identify(v.mount, v.name)]}
		d.FreeBytes, d.TotalBytes = diskSpace(v.mount)
		out = append(out, d)
	}
	return out, nil
}

type volume struct{ mount, name string }

// listVolumes is a variable so the lbfake build can substitute folders for real drives.
var listVolumes = removableVolumes

func identify(mount, name string) Kind {
	lname := strings.ToLower(name)
	switch {
	case isDir(mount, ".kobo") || strings.Contains(lname, "kobo"):
		return Kobo
	case strings.Contains(lname, "kindle") || (isDir(mount, "documents") && isDir(mount, "system") && fileExists(filepath.Join(mount, "system", "version.txt"))):
		return Kindle
	// .crosspoint: CrossPoint firmware. XTCache/XTData/XTApps: Xteink's stock firmware.
	case isDir(mount, ".crosspoint") || isDir(mount, "XTCache") || isDir(mount, "XTData") ||
		isDir(mount, "XTApps") || strings.Contains(lname, "xteink"):
		return Xteink
	}
	return Generic
}

// Find returns the device mounted at id, refusing anything that isn't currently a
// removable volume, so a request can never write to an arbitrary folder.
func Find(ctx context.Context, id string) (Device, error) {
	list, err := List(ctx)
	if err != nil {
		return Device{}, err
	}
	for _, d := range list {
		if d.ID == id {
			return d, nil
		}
	}
	return Device{}, errors.New("dispositivo não encontrado; ele foi desconectado?")
}

// Destination resolves a user-chosen folder inside the device, rejecting escapes.
func Destination(d Device, folder string) (string, error) {
	folder = strings.TrimSpace(strings.ReplaceAll(folder, "\\", "/"))
	clean := filepath.Clean("/" + folder) // anchoring at "/" collapses any ".."
	dest := filepath.Join(d.Mount, clean)
	rel, err := filepath.Rel(d.Mount, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("pasta de destino inválida")
	}
	return dest, nil
}

// System folders that operating systems and cameras create on cards; never book destinations.
var systemFolders = map[string]bool{
	"system volume information": true, "$recycle.bin": true, "lost.dir": true,
	"lost+found": true, "android": true, "dcim": true,
}

// Folders lists the visible subfolders of rel inside the device and returns rel cleaned
// to a "/"-separated path relative to the volume root ("" for the root).
func Folders(d Device, rel string) (string, []string, error) {
	dir, err := Destination(d, rel)
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	folders := []string{}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || systemFolders[strings.ToLower(name)] {
			continue
		}
		folders = append(folders, name)
	}
	sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i]) < strings.ToLower(folders[j]) })
	clean, _ := filepath.Rel(d.Mount, dir)
	if clean == "." {
		clean = ""
	}
	return filepath.ToSlash(clean), folders, nil
}

type Item struct {
	BookID      int
	Title       string
	Authors     []string
	Format      string
	Path        string
	Hash        string // sha256 of Path, if already known
	Cover       string
	Series      string
	SeriesIndex float64
	Publisher   string
	ISBN        string
	Language    string
	PubDate     string
}

type Result struct {
	BookID    int    `json:"book_id"`
	Title     string `json:"title"`
	Status    string `json:"status"` // sent, exists, duplicate, unsupported, error
	Converted bool   `json:"converted,omitempty"`
	File      string `json:"file,omitempty"`
	Existing  string `json:"existing,omitempty"` // device path of the copy already there
	Error     string `json:"error,omitempty"`
}

type SendOptions struct {
	Converter *Converter // may be nil: books needing conversion are then unsupported
	OnDevice  *Index     // books already on the device; nil skips the duplicate check
	Force     bool       // send even when a copy with the same title and author exists
	Mount     string     // volume root, to report where sent files landed
}

// Send copies each book to dest, converting when the device can't open its format.
// Identical content anywhere on the device counts as already there; the same title and
// author with different content is a likely duplicate and skipped unless opts.Force.
func Send(ctx context.Context, p Profile, dest string, items []Item, opts SendOptions) []Result {
	conv := opts.Converter
	results := make([]Result, 0, len(items))
	mkErr := os.MkdirAll(dest, 0o755)
	for _, it := range items {
		r := Result{BookID: it.BookID, Title: it.Title}
		format, src := strings.ToLower(it.Format), it.Path
		target, canConvert := p.Convert[format]
		switch {
		case ctx.Err() != nil:
			r.Status, r.Error = "error", "cancelado"
		case !supports(p, format) && !(canConvert && conv != nil):
			r.Status, r.Error = "unsupported", fmt.Sprintf("%s não abre %s", p.Label, strings.ToUpper(format))
		case mkErr != nil:
			r.Status, r.Error = "error", mkErr.Error()
		default:
			hash := it.Hash
			if !supports(p, format) {
				out, err := conv.Convert(ctx, it, target)
				if err != nil {
					r.Status, r.Error = "error", err.Error()
					break
				}
				src, format, r.Converted, hash = out, target, true, ""
			}
			if hash == "" {
				hash, _ = hashFile(src)
			}
			author := ""
			if len(it.Authors) > 0 {
				author = strings.Join(it.Authors, ", ")
			}
			name := deviceFileName(it.Title, author, format)
			r.File = name
			dst := filepath.Join(dest, name)

			var size int64
			if st, err := os.Stat(src); err == nil {
				size = st.Size()
			}
			if opts.OnDevice != nil {
				if dup, kind := opts.OnDevice.Match(hash, size, it.Title, it.Authors); kind == "exact" {
					r.Status, r.Existing = "exists", dup.Path
					break
				} else if kind == "likely" && !opts.Force {
					r.Status, r.Existing = "duplicate", dup.Path
					break
				}
			}
			if same, _ := sameSize(src, dst); same {
				r.Status = "exists"
			} else if err := copyDurable(src, dst); err != nil {
				r.Status, r.Error = "error", err.Error()
			} else {
				r.Status = "sent"
				if opts.OnDevice != nil {
					rel, _ := filepath.Rel(opts.Mount, dst)
					// Recorded so the same book twice in one batch isn't copied twice.
					opts.OnDevice.Add(DeviceBook{Path: filepath.ToSlash(rel), Format: format, Size: size, Title: it.Title, Authors: it.Authors, hash: hash})
				}
			}
		}
		results = append(results, r)
	}
	return results
}

func supports(p Profile, format string) bool {
	for _, f := range p.Formats {
		if strings.EqualFold(f, format) {
			return true
		}
	}
	return false
}

func sameSize(src, dst string) (bool, error) {
	a, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	b, err := os.Stat(dst)
	if err != nil {
		return false, err
	}
	return a.Size() == b.Size(), nil
}

// copyDurable writes to a temp name, syncs, then renames, so unplugging mid-copy never
// leaves a truncated book with the final name.
func copyDurable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".lbpart"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(dst) // FAT/exFAT rename does not overwrite on every OS
	return os.Rename(tmp, dst)
}

// FileName is the name a book gets on a device: "Title - Author.ext".
func FileName(title, author, format string) string { return deviceFileName(title, author, format) }

// deviceFileName builds "Title - Author.ext" safe for FAT32/exFAT, which most readers use.
func deviceFileName(title, author, format string) string {
	base := strings.TrimSpace(title)
	if author != "" && author != "Desconhecido" {
		base += " - " + author
	}
	base = strings.Map(func(r rune) rune {
		switch {
		case strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		case r < 32 || !unicode.IsPrint(r):
			return -1
		}
		return r
	}, base)
	base = strings.TrimRight(strings.TrimSpace(base), ". ")
	if base == "" {
		base = "Livro"
	}
	ext := "." + strings.ToLower(format)
	const maxBytes = 240 // FAT long names cap at 255 UTF-16 units; leave room for ext and temp suffix
	for len(base)+len(ext) > maxBytes {
		r := []rune(base)
		base = strings.TrimRight(string(r[:len(r)-1]), ". ")
	}
	return base + ext
}

func isDir(parts ...string) bool {
	st, err := os.Stat(filepath.Join(parts...))
	return err == nil && st.IsDir()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
