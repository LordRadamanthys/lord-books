package devices

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/LordRadamanthys/lord-books/internal/thumb"
)

// Kindles draw library covers for side-loaded books from
// system/thumbnails/thumbnail_<ASIN>_<cdetype>_portrait.jpg rather than from the book,
// so without that file the book shows a generic cover. calibre writes it the same way.
const kindleThumbHeight = 330

// mobiIDs reads the ASIN (EXTH 113, or 504) and document type (EXTH 501) from a
// MOBI/AZW3 file's header.
func mobiIDs(path string) (asin, cdetype string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	head := make([]byte, 86)
	if _, err := io.ReadFull(f, head); err != nil {
		return "", "", err
	}
	if string(head[60:68]) != "BOOKMOBI" {
		return "", "", errors.New("não é um arquivo MOBI/AZW3")
	}
	rec0 := int64(binary.BigEndian.Uint32(head[78:82]))

	// Record 0: 16-byte PalmDOC header, then the MOBI header, then EXTH.
	mobi := make([]byte, 0x84)
	if _, err := f.ReadAt(mobi, rec0+16); err != nil {
		return "", "", err
	}
	if string(mobi[0:4]) != "MOBI" {
		return "", "", errors.New("cabeçalho MOBI ausente")
	}
	mobiLen := int64(binary.BigEndian.Uint32(mobi[4:8]))
	if binary.BigEndian.Uint32(mobi[0x70:0x74])&0x40 == 0 {
		return "", "", errors.New("sem metadados EXTH")
	}

	exthAt := rec0 + 16 + mobiLen
	hdr := make([]byte, 12)
	if _, err := f.ReadAt(hdr, exthAt); err != nil {
		return "", "", err
	}
	if string(hdr[0:4]) != "EXTH" {
		return "", "", errors.New("bloco EXTH inválido")
	}
	exthLen := binary.BigEndian.Uint32(hdr[4:8])
	if exthLen < 12 || exthLen > 1<<20 {
		return "", "", errors.New("bloco EXTH com tamanho inválido")
	}
	exth := make([]byte, exthLen-12)
	if _, err := f.ReadAt(exth, exthAt+12); err != nil {
		return "", "", err
	}
	count := binary.BigEndian.Uint32(hdr[8:12])
	var asin504 string
	for i, p := uint32(0), 0; i < count && p+8 <= len(exth); i++ {
		typ := binary.BigEndian.Uint32(exth[p : p+4])
		size := int(binary.BigEndian.Uint32(exth[p+4 : p+8]))
		if size < 8 || p+size > len(exth) {
			break
		}
		val := strings.TrimSpace(string(exth[p+8 : p+size]))
		switch typ {
		case 113:
			asin = val
		case 504:
			asin504 = val
		case 501:
			cdetype = val
		}
		p += size
	}
	if asin == "" {
		asin = asin504
	}
	if asin == "" {
		return "", "", errors.New("livro sem ASIN")
	}
	if cdetype == "" {
		cdetype = "EBOK"
	}
	return asin, cdetype, nil
}

// writeKindleThumbnail stores the cover thumbnail a Kindle uses for a side-loaded book.
// Failures are not fatal: the book itself was copied, only its grid cover is missing.
func writeKindleThumbnail(mount, bookPath, coverPath string) error {
	if coverPath == "" {
		return errors.New("livro sem capa")
	}
	if !isDir(mount, "system") {
		return errors.New("não parece um Kindle (sem a pasta system)")
	}
	asin, cdetype, err := mobiIDs(bookPath)
	if err != nil {
		return err
	}
	// The values become part of a file name: allow only what ASINs and types contain.
	for _, v := range []string{asin, cdetype} {
		if strings.ContainsAny(v, `/\:*?"<>|`) || strings.Contains(v, "..") || len(v) > 64 {
			return fmt.Errorf("identificador inválido: %q", v)
		}
	}
	data, _, _, err := thumb.FromFile(coverPath, kindleThumbHeight)
	if err != nil {
		return err
	}
	dir := filepath.Join(mount, "system", "thumbnails")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, fmt.Sprintf("thumbnail_%s_%s_portrait.jpg", asin, cdetype)), data, 0o644)
}
