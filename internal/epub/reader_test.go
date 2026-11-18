package epub

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeEPUB(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "book.epub")
	f, _ := os.Create(p)
	z := zip.NewWriter(f)
	for name, body := range files {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	f.Close()
	return p
}

const testContainer = `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`

func TestReadStructureEPUB3(t *testing.T) {
	p := writeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Livro</dc:title></metadata>
<manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
<item id="c1" href="Text/cap%201.xhtml" media-type="application/xhtml+xml"/>
<item id="c2" href="Text/cap2.xhtml" media-type="application/xhtml+xml"/>
<item id="img" href="Images/x.jpg" media-type="image/jpeg"/></manifest>
<spine><itemref idref="c1"/><itemref idref="c2" linear="no"/></spine></package>`,
		"OEBPS/nav.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>
<nav epub:type="toc"><ol><li><a href="Text/cap%201.xhtml">Capítulo  um</a><ol><li><a href="Text/cap%201.xhtml#s2">Seção</a></li></ol></li>
<li><a href="Text/cap2.xhtml">Dois</a></li></ol></nav>
<nav epub:type="landmarks"><ol><li><a href="Text/cap2.xhtml">Ignorar</a></li></ol></nav></body></html>`,
		"OEBPS/Text/cap 1.xhtml": "<html><body><p>um</p></body></html>",
		"OEBPS/Text/cap2.xhtml":  "<html><body><p>dois</p></body></html>",
	})
	s, err := ReadStructure(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Livro" || len(s.Spine) != 2 || s.Spine[0].Href != "OEBPS/Text/cap 1.xhtml" || s.Spine[1].Linear {
		t.Errorf("spine: %+v", s)
	}
	if s.Spine[0].Size == 0 {
		t.Error("sizes missing")
	}
	want := []TOCItem{
		{"Capítulo um", "OEBPS/Text/cap 1.xhtml", 0},
		{"Seção", "OEBPS/Text/cap 1.xhtml#s2", 1},
		{"Dois", "OEBPS/Text/cap2.xhtml", 0},
	}
	if len(s.TOC) != len(want) {
		t.Fatalf("toc: %+v", s.TOC)
	}
	for i := range want {
		if s.TOC[i] != want[i] {
			t.Errorf("toc[%d] = %+v, want %+v", i, s.TOC[i], want[i])
		}
	}

	rc, z, n, err := OpenFile(p, "OEBPS/Text/cap 1.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	z.Close()
	if string(b) != "<html><body><p>um</p></body></html>" || n != int64(len(b)) {
		t.Errorf("content: %q", b)
	}
	if _, _, _, err := OpenFile(p, "../../etc/passwd"); err == nil {
		t.Error("paths outside the book must not resolve")
	}
}

// A 300 KB EPUB whose cover expands to 300 MB used to be read whole into memory.
func TestZipBombIsNotLoaded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bomb.epub")
	f, _ := os.Create(p)
	z := zip.NewWriter(f)
	w, _ := z.Create("META-INF/container.xml")
	w.Write([]byte(testContainer))
	w, _ = z.Create("OEBPS/content.opf")
	w.Write([]byte(`<package><metadata><meta name="cover" content="img"/></metadata><manifest><item id="img" href="cover.jpg" media-type="image/jpeg"/><item id="c" href="c.html" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c"/></spine></package>`))
	w, _ = z.Create("OEBPS/c.html")
	w.Write([]byte("<p>x</p>"))
	w, _ = z.Create("OEBPS/cover.jpg")
	zero := make([]byte, 1<<20)
	for i := 0; i < 300; i++ {
		w.Write(zero)
	}
	z.Close()
	f.Close()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	m, err := Parse(p)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("metadata should still be read: %v", err)
	}
	if len(m.CoverData) != 0 {
		t.Errorf("oversized cover loaded: %d bytes", len(m.CoverData))
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 64<<20 {
		t.Errorf("parsing allocated %d MB", alloc>>20)
	}
}

func TestReadStructureNCX(t *testing.T) {
	p := writeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf": `<package><metadata/><manifest><item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
<item id="c1" href="c1.html" media-type="application/xhtml+xml"/></manifest><spine toc="ncx"><itemref idref="c1"/></spine></package>`,
		"OEBPS/toc.ncx": `<ncx><navMap><navPoint><navLabel><text>Início</text></navLabel><content src="c1.html"/>
<navPoint><navLabel><text>Sub</text></navLabel><content src="c1.html#a"/></navPoint></navPoint></navMap></ncx>`,
		"OEBPS/c1.html": "<p>x</p>",
	})
	s, err := ReadStructure(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.TOC) != 2 || s.TOC[1].Href != "OEBPS/c1.html#a" || s.TOC[1].Level != 1 {
		t.Errorf("toc: %+v", s.TOC)
	}
}
