package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Converter wraps Calibre's ebook-convert, used to turn EPUB into AZW3 for Kindles.
type Converter struct {
	Path string
}

// FindConverter looks for ebook-convert on PATH and in Calibre's default install
// locations; it returns nil when Calibre isn't installed.
func FindConverter() *Converter {
	if p, err := exec.LookPath("ebook-convert"); err == nil {
		return &Converter{Path: p}
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/calibre.app/Contents/MacOS/ebook-convert"}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, "Applications/calibre.app/Contents/MacOS/ebook-convert"))
		}
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if dir := os.Getenv(env); dir != "" {
				candidates = append(candidates, filepath.Join(dir, "Calibre2", "ebook-convert.exe"))
			}
		}
	default:
		candidates = []string{"/opt/calibre/ebook-convert", "/usr/bin/ebook-convert"}
	}
	for _, c := range candidates {
		if fileExists(c) {
			return &Converter{Path: c}
		}
	}
	return nil
}

const convertTimeout = 5 * time.Minute

// Convert returns a path to it converted to format. The result is cached next to the
// source book, keyed on the source file and the metadata, so resending is instant and
// editing the book's metadata produces a fresh conversion.
func (c *Converter) Convert(ctx context.Context, it Item, format string) (string, error) {
	st, err := os.Stat(it.Path)
	if err != nil {
		return "", err
	}
	var coverStamp string
	if it.Cover != "" {
		if cst, err := os.Stat(it.Cover); err == nil {
			coverStamp = fmt.Sprint(cst.Size(), cst.ModTime().UnixNano())
		}
	}
	h := sha256.New()
	fmt.Fprintln(h, st.Size(), st.ModTime().UnixNano(), coverStamp, format,
		it.Title, strings.Join(it.Authors, "\x00"), it.Series, it.SeriesIndex,
		it.Publisher, it.ISBN, it.Language, it.PubDate)
	key := hex.EncodeToString(h.Sum(nil))[:16]

	dir := filepath.Dir(it.Path)
	out := filepath.Join(dir, ".lb-convert-"+key+"."+format)
	if fileExists(out) {
		return out, nil
	}

	// ebook-convert picks the output format from the extension, so the temp name keeps it.
	tmp := filepath.Join(dir, ".lb-convert-"+key+".part."+format)
	args := []string{it.Path, tmp}
	if format == "azw3" || format == "mobi" {
		args = append(args, "--output-profile=kindle_pw3")
	}
	// "--opt=value" form keeps values that start with "-" from being read as options.
	addOpt := func(name, val string) {
		if strings.TrimSpace(val) != "" {
			args = append(args, "--"+name+"="+val)
		}
	}
	addOpt("title", it.Title)
	addOpt("authors", strings.Join(it.Authors, " & "))
	addOpt("series", it.Series)
	if it.Series != "" && it.SeriesIndex > 0 {
		addOpt("series-index", strconv.FormatFloat(it.SeriesIndex, 'f', -1, 64))
	}
	addOpt("publisher", it.Publisher)
	addOpt("isbn", it.ISBN)
	addOpt("language", it.Language)
	addOpt("pubdate", it.PubDate)
	if it.Cover != "" && fileExists(it.Cover) {
		addOpt("cover", it.Cover)
	}

	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("conversão demorou mais de %s", convertTimeout)
		}
		return "", fmt.Errorf("ebook-convert falhou: %s", lastLine(output))
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", err
	}

	// Drop conversions made from older metadata so the cache doesn't grow forever.
	old, _ := filepath.Glob(filepath.Join(dir, ".lb-convert-*."+format))
	for _, f := range old {
		if f != out {
			os.Remove(f)
		}
	}
	return out, nil
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return "sem detalhes"
}
