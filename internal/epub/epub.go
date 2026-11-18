package epub

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

type Metadata struct {
	Title       string
	Authors     []string
	ISBN        string
	Language    string
	Publisher   string
	PubDate     string
	Description string
	CoverData   []byte
	CoverExt    string
}

func Parse(file string) (*Metadata, error) { return parse(file, true) }

// ParseInfo reads only the text metadata, skipping the cover image; much cheaper on
// slow storage such as an e-reader exposed over USB.
func ParseInfo(file string) (*Metadata, error) { return parse(file, false) }

func parse(file string, withCover bool) (*Metadata, error) {
	r, err := zip.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("open epub: %w", err)
	}
	defer r.Close()

	opfPath, err := findOPF(r)
	if err != nil {
		return nil, err
	}

	opfDir := path.Dir(opfPath)
	opfData, err := readFromZip(r, opfPath)
	if err != nil {
		return nil, fmt.Errorf("read opf: %w", err)
	}

	var pkg opfPackage
	if err := xml.Unmarshal(opfData, &pkg); err != nil {
		return nil, fmt.Errorf("parse opf: %w", err)
	}

	m := &Metadata{
		Title:       pkg.Metadata.Title,
		Language:    pkg.Metadata.Language,
		Publisher:   pkg.Metadata.Publisher,
		PubDate:     pkg.Metadata.Date,
		Description: strings.TrimSpace(pkg.Metadata.Description),
	}

	for _, c := range pkg.Metadata.Creator {
		if name := strings.TrimSpace(c.Value); name != "" {
			m.Authors = append(m.Authors, name)
		}
	}

	for _, id := range pkg.Metadata.Identifier {
		v := strings.TrimSpace(id.Value)
		if isISBN(v) {
			m.ISBN = normalizeISBN(v)
			break
		}
	}

	if !withCover {
		return m, nil
	}
	coverHref := findCoverHref(pkg, opfDir)
	if coverHref != "" {
		if data, err := readFromZipMax(r, coverHref, maxCoverBytes); err == nil {
			m.CoverData = data
			m.CoverExt = extFromPath(coverHref)
		}
	}

	return m, nil
}

type container struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type opfPackage struct {
	Metadata opfMetadata `xml:"metadata"`
	Manifest []opfItem   `xml:"manifest>item"`
	Guide    []opfRef    `xml:"guide>reference"`
}

type opfMetadata struct {
	Title       string       `xml:"title"`
	Language    string       `xml:"language"`
	Publisher   string       `xml:"publisher"`
	Date        string       `xml:"date"`
	Description string       `xml:"description"`
	Creator     []opfCreator `xml:"creator"`
	Identifier  []opfIdent   `xml:"identifier"`
	Meta        []opfMeta    `xml:"meta"`
}

type opfCreator struct {
	Value string `xml:",chardata"`
	Role  string `xml:"role,attr"`
}

type opfIdent struct {
	Value  string `xml:",chardata"`
	Scheme string `xml:"scheme,attr"`
}

type opfMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
}

type opfItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

type opfRef struct {
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

func findOPF(r *zip.ReadCloser) (string, error) {
	data, err := readFromZip(r, "META-INF/container.xml")
	if err != nil {
		return "", err
	}
	var c container
	if err := xml.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("parse container.xml: %w", err)
	}
	if len(c.Rootfiles) == 0 {
		return "", fmt.Errorf("no rootfile in container.xml")
	}
	return c.Rootfiles[0].FullPath, nil
}

func findCoverHref(pkg opfPackage, opfDir string) string {
	var coverID string
	for _, meta := range pkg.Metadata.Meta {
		if meta.Name == "cover" {
			coverID = meta.Content
			break
		}
	}

	if coverID != "" {
		for _, item := range pkg.Manifest {
			if item.ID == coverID {
				return resolvePath(opfDir, item.Href)
			}
		}
	}

	for _, item := range pkg.Manifest {
		if strings.Contains(strings.ToLower(item.ID), "cover") && isImageType(item.MediaType) {
			return resolvePath(opfDir, item.Href)
		}
	}

	for _, ref := range pkg.Guide {
		if ref.Type == "cover" {
			return resolvePath(opfDir, ref.Href)
		}
	}

	for _, item := range pkg.Manifest {
		if isImageType(item.MediaType) && strings.Contains(strings.ToLower(item.Href), "cover") {
			return resolvePath(opfDir, item.Href)
		}
	}

	return ""
}

func openInZip(r *zip.ReadCloser, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if strings.EqualFold(f.Name, name) {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("file not found in zip: %s", name)
}

// Size caps for files read whole from an EPUB. A tiny crafted zip can expand to
// gigabytes ("zip bomb"), and a book's structure files and cover are far smaller.
const (
	maxDocBytes   = 16 << 20
	maxCoverBytes = 20 << 20
)

var errTooBig = errors.New("arquivo grande demais dentro do EPUB")

func readFromZip(r *zip.ReadCloser, name string) ([]byte, error) {
	return readFromZipMax(r, name, maxDocBytes)
}

// readFromZipMax checks the declared size first, then also caps the actual read,
// since the size in a zip header can lie.
func readFromZipMax(r *zip.ReadCloser, name string, max int64) ([]byte, error) {
	for _, f := range r.File {
		if strings.EqualFold(f.Name, name) && f.UncompressedSize64 > uint64(max) {
			return nil, errTooBig
		}
	}
	rc, err := openInZip(r, name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errTooBig
	}
	return data, nil
}

func resolvePath(dir, href string) string {
	if dir == "" || dir == "." {
		return href
	}
	return dir + "/" + href
}

func isImageType(mt string) bool {
	return strings.HasPrefix(mt, "image/")
}

func extFromPath(p string) string {
	ext := path.Ext(p)
	if ext == "" {
		return ".jpg"
	}
	return ext
}

func isISBN(s string) bool {
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	if strings.HasPrefix(strings.ToLower(s), "urn:isbn:") {
		s = s[9:]
	}
	n := len(s)
	return n == 10 || n == 13
}

func normalizeISBN(s string) string {
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	if strings.HasPrefix(strings.ToLower(s), "urn:isbn:") {
		s = s[9:]
	}
	return s
}
