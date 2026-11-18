package devices

import (
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// fakeMOBI builds the smallest file mobiIDs can read: a PalmDB header pointing at
// record 0, a PalmDOC header, a MOBI header with the EXTH flag, and EXTH records.
func fakeMOBI(t *testing.T, exth map[uint32]string) string {
	t.Helper()
	const rec0 = 86
	const mobiLen = 0xE8
	buf := make([]byte, rec0+16+mobiLen)
	copy(buf[60:68], "BOOKMOBI")
	binary.BigEndian.PutUint16(buf[76:78], 1)
	binary.BigEndian.PutUint32(buf[78:82], rec0)
	m := buf[rec0+16:]
	copy(m[0:4], "MOBI")
	binary.BigEndian.PutUint32(m[4:8], mobiLen)
	binary.BigEndian.PutUint32(m[0x70:0x74], 0x40)

	var recs []byte
	for typ, val := range exth {
		r := make([]byte, 8+len(val))
		binary.BigEndian.PutUint32(r[0:4], typ)
		binary.BigEndian.PutUint32(r[4:8], uint32(len(r)))
		copy(r[8:], val)
		recs = append(recs, r...)
	}
	hdr := make([]byte, 12)
	copy(hdr[0:4], "EXTH")
	binary.BigEndian.PutUint32(hdr[4:8], uint32(12+len(recs)))
	binary.BigEndian.PutUint32(hdr[8:12], uint32(len(exth)))
	buf = append(append(buf, hdr...), recs...)

	p := filepath.Join(t.TempDir(), "book.azw3")
	os.WriteFile(p, buf, 0o644)
	return p
}

func TestMobiIDs(t *testing.T) {
	asin, typ, err := mobiIDs(fakeMOBI(t, map[uint32]string{113: "abc-123", 501: "EBOK", 100: "Autor"}))
	if err != nil || asin != "abc-123" || typ != "EBOK" {
		t.Errorf("got %q %q %v", asin, typ, err)
	}
	asin, typ, _ = mobiIDs(fakeMOBI(t, map[uint32]string{504: "only-504"}))
	if asin != "only-504" || typ != "EBOK" {
		t.Errorf("fallbacks: %q %q", asin, typ)
	}
	if _, _, err := mobiIDs(fakeMOBI(t, map[uint32]string{100: "Autor"})); err == nil {
		t.Error("a book without ASIN must be refused")
	}
	notMobi := filepath.Join(t.TempDir(), "x.azw3")
	os.WriteFile(notMobi, make([]byte, 200), 0o644)
	if _, _, err := mobiIDs(notMobi); err == nil {
		t.Error("non-MOBI accepted")
	}
}

func TestWriteKindleThumbnail(t *testing.T) {
	mount := t.TempDir()
	cover := filepath.Join(t.TempDir(), "cover.png")
	f, _ := os.Create(cover)
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 400, 600)))
	f.Close()
	book := fakeMOBI(t, map[uint32]string{113: "asin-1", 501: "EBOK"})

	if err := writeKindleThumbnail(mount, book, cover); err == nil {
		t.Error("should refuse a volume without the Kindle system folder")
	}
	mkdirs(t, mount, "system")
	if err := writeKindleThumbnail(mount, book, cover); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mount, "system", "thumbnails", "thumbnail_asin-1_EBOK_portrait.jpg")); err != nil {
		t.Error("thumbnail not written:", err)
	}
	evil := fakeMOBI(t, map[uint32]string{113: "../../escape", 501: "EBOK"})
	if err := writeKindleThumbnail(mount, evil, cover); err == nil {
		t.Error("an ASIN with path characters must be refused")
	}
}
