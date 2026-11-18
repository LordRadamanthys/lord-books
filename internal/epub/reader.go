package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

// Structure is what the in-app reader needs to lay out a book.
type Structure struct {
	Title string      `json:"title"`
	Spine []SpineItem `json:"spine"`
	TOC   []TOCItem   `json:"toc"`
}

// SpineItem is one document in reading order; Href is a path inside the zip.
type SpineItem struct {
	Href   string `json:"href"`
	Size   int64  `json:"size"`
	Linear bool   `json:"linear"`
}

// TOCItem points at a zip path plus an optional "#fragment".
type TOCItem struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Level int    `json:"level"`
}

type readerPackage struct {
	Metadata struct {
		Title string `xml:"title"`
	} `xml:"metadata"`
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		Toc   string `xml:"toc,attr"`
		Items []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// ReadStructure parses the spine and table of contents of an EPUB.
func ReadStructure(file string) (*Structure, error) {
	r, err := zip.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("open epub: %w", err)
	}
	defer r.Close()

	opfPath, err := findOPF(r)
	if err != nil {
		return nil, err
	}
	data, err := readFromZip(r, opfPath)
	if err != nil {
		return nil, fmt.Errorf("read opf: %w", err)
	}
	var pkg readerPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse opf: %w", err)
	}
	opfDir := path.Dir(opfPath)

	sizes := map[string]int64{}
	for _, f := range r.File {
		sizes[f.Name] = int64(f.UncompressedSize64)
	}
	type item struct{ href, mediaType, props string }
	byID := map[string]item{}
	var navHref string
	for _, it := range pkg.Manifest {
		href := joinHref(opfDir, it.Href)
		byID[it.ID] = item{href, it.MediaType, it.Properties}
		if strings.Contains(" "+it.Properties+" ", " nav ") {
			navHref = href
		}
	}

	s := &Structure{Title: strings.TrimSpace(pkg.Metadata.Title)}
	for _, ref := range pkg.Spine.Items {
		it, ok := byID[ref.IDRef]
		if !ok {
			continue
		}
		if !strings.Contains(it.mediaType, "html") {
			continue // images or SVG in the spine are rare and not paginated here
		}
		s.Spine = append(s.Spine, SpineItem{Href: it.href, Size: sizes[it.href], Linear: ref.Linear != "no"})
	}
	if len(s.Spine) == 0 {
		return nil, errors.New("o livro não tem capítulos legíveis")
	}

	if navHref != "" {
		if nav, err := readFromZip(r, navHref); err == nil {
			s.TOC = parseNav(nav, path.Dir(navHref))
		}
	}
	if len(s.TOC) == 0 && pkg.Spine.Toc != "" {
		if ncx, ok := byID[pkg.Spine.Toc]; ok {
			if data, err := readFromZip(r, ncx.href); err == nil {
				s.TOC = parseNCX(data, path.Dir(ncx.href))
			}
		}
	}
	return s, nil
}

// joinHref resolves an href (URL-encoded, relative) against a directory in the zip.
func joinHref(dir, href string) string {
	frag := ""
	if i := strings.IndexByte(href, '#'); i >= 0 {
		href, frag = href[:i], href[i:]
	}
	if u, err := url.PathUnescape(href); err == nil {
		href = u
	}
	p := path.Clean(path.Join(dir, href))
	p = strings.TrimPrefix(p, "./")
	if p == "." {
		p = ""
	}
	return p + frag
}

// parseNav reads an EPUB 3 navigation document's <nav epub:type="toc"> list.
func parseNav(data []byte, dir string) []TOCItem {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	var items []TOCItem
	inTOC, depth, navDepth := false, 0, 0
	var cur *TOCItem
	var label strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "nav":
				if !inTOC {
					for _, a := range t.Attr {
						if a.Name.Local == "type" && strings.Contains(a.Value, "toc") {
							inTOC, navDepth = true, 0
						}
					}
				}
			case "ol":
				if inTOC {
					depth++
				}
			case "a", "span":
				if inTOC && cur == nil {
					href := ""
					for _, a := range t.Attr {
						if a.Name.Local == "href" {
							href = a.Value
						}
					}
					if href != "" {
						cur = &TOCItem{Href: joinHref(dir, href), Level: max(depth-1, 0)}
						label.Reset()
					}
				}
			}
			if inTOC && t.Name.Local == "nav" {
				navDepth++
			}
		case xml.CharData:
			if cur != nil {
				label.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "a", "span":
				if cur != nil {
					cur.Label = strings.Join(strings.Fields(label.String()), " ")
					if cur.Label != "" {
						items = append(items, *cur)
					}
					cur = nil
				}
			case "ol":
				if inTOC {
					depth--
				}
			case "nav":
				if inTOC {
					navDepth--
					if navDepth <= 0 {
						return items
					}
				}
			}
		}
	}
	return items
}

type ncxPoint struct {
	Label   string `xml:"navLabel>text"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []ncxPoint `xml:"navPoint"`
}

// parseNCX reads an EPUB 2 toc.ncx.
func parseNCX(data []byte, dir string) []TOCItem {
	var doc struct {
		Points []ncxPoint `xml:"navMap>navPoint"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var items []TOCItem
	var walk func(ps []ncxPoint, level int)
	walk = func(ps []ncxPoint, level int) {
		for _, p := range ps {
			if l := strings.Join(strings.Fields(p.Label), " "); l != "" && p.Content.Src != "" {
				items = append(items, TOCItem{Label: l, Href: joinHref(dir, p.Content.Src), Level: level})
			}
			walk(p.Children, level+1)
		}
	}
	walk(doc.Points, 0)
	return items
}

// OpenFile opens a file inside the EPUB by its zip path (case-insensitive fallback).
// The caller must close both the returned reader and the zip.
func OpenFile(file, name string) (io.ReadCloser, *zip.ReadCloser, int64, error) {
	z, err := zip.OpenReader(file)
	if err != nil {
		return nil, nil, 0, err
	}
	var match *zip.File
	for _, f := range z.File {
		if f.Name == name {
			match = f
			break
		}
		if match == nil && strings.EqualFold(f.Name, name) {
			match = f
		}
	}
	if match == nil || strings.HasSuffix(match.Name, "/") {
		z.Close()
		return nil, nil, 0, errors.New("arquivo não encontrado no livro")
	}
	rc, err := match.Open()
	if err != nil {
		z.Close()
		return nil, nil, 0, err
	}
	return rc, z, int64(match.UncompressedSize64), nil
}
