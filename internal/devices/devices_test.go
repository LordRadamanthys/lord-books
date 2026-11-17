package devices

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIdentify(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		dirs []string
		file string
		want Kind
	}{
		{"KOBOeReader", nil, "", Kobo},
		{"VOL1", []string{".kobo"}, "", Kobo},
		{"Kindle", nil, "", Kindle},
		{"VOL2", []string{"documents", "system"}, "system/version.txt", Kindle},
		{"VOL3", []string{"documents", "system"}, "", Generic}, // no version.txt: not enough to call it a Kindle
		{"SDCARD", []string{".crosspoint"}, "", Xteink},
		{"XTEINK", nil, "", Xteink},
		{"NO NAME", []string{"DCIM"}, "", Generic},
	}
	for _, c := range cases {
		mount := filepath.Join(root, c.name)
		mkdirs(t, mount, c.dirs...)
		if c.file != "" {
			os.WriteFile(filepath.Join(mount, c.file), []byte("x"), 0o644)
		}
		if got := identify(mount, c.name); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestDestinationStaysInsideDevice(t *testing.T) {
	d := Device{Mount: t.TempDir()}
	for _, folder := range []string{"", "Lord Books", "a/b", "../../etc", "/etc", `..\..\x`, "a/../../.."} {
		dest, err := Destination(d, folder)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(d.Mount, dest)
		if strings.HasPrefix(rel, "..") {
			t.Errorf("folder %q escaped the device: %s", folder, dest)
		}
	}
}

func TestDeviceFileName(t *testing.T) {
	got := deviceFileName(`O "melhor": livro?`, "Autor/Um", "EPUB")
	if strings.ContainsAny(got, `/\:*?"<>|`) || !strings.HasSuffix(got, ".epub") {
		t.Errorf("unsafe name: %q", got)
	}
	long := deviceFileName(strings.Repeat("ç", 300), "", "pdf")
	if len(long) > 245 || !strings.HasSuffix(long, ".pdf") {
		t.Errorf("long name not trimmed: %d bytes", len(long))
	}
	if got := deviceFileName("Livro.", "Desconhecido", "epub"); got != "Livro.epub" {
		t.Errorf("got %q", got)
	}
}

func TestSend(t *testing.T) {
	src := filepath.Join(t.TempDir(), "book.epub")
	os.WriteFile(src, []byte("epub-bytes"), 0o644)
	pdf := filepath.Join(t.TempDir(), "doc.pdf")
	os.WriteFile(pdf, []byte("pdf"), 0o644)
	dest := filepath.Join(t.TempDir(), "Lord Books")

	items := []Item{
		{BookID: 1, Title: "Livro", Authors: []string{"Ana"}, Format: "epub", Path: src},
		{BookID: 2, Title: "Doc", Format: "pdf", Path: pdf},
	}
	res := Send(context.Background(), Profiles[Xteink], dest, items, nil)
	if res[0].Status != "sent" || res[1].Status != "unsupported" {
		t.Fatalf("first send: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dest, "Livro - Ana.epub")); err != nil {
		t.Fatal("file not copied:", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dest, "*.lbpart")); len(matches) > 0 {
		t.Error("temp file left behind")
	}
	res = Send(context.Background(), Profiles[Xteink], dest, items[:1], nil)
	if res[0].Status != "exists" {
		t.Errorf("second send: %+v", res[0])
	}

	// Kindle can't open EPUB; without Calibre's converter it must be refused, not copied.
	kdest := filepath.Join(t.TempDir(), "documents")
	res = Send(context.Background(), Profiles[Kindle], kdest, items, nil)
	if res[0].Status != "unsupported" || res[1].Status != "sent" {
		t.Errorf("kindle without converter: %+v", res)
	}
}
