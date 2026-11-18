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
		{"NO NAME 2", []string{"XTCache", "XTData", "XTApps", "Fonts"}, "", Xteink}, // stock firmware card
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

func TestFolders(t *testing.T) {
	d := Device{Mount: t.TempDir()}
	mkdirs(t, d.Mount, "livros/ficcao", "Books", ".Trashes", "System Volume Information", "DCIM")
	os.WriteFile(filepath.Join(d.Mount, "arquivo.epub"), []byte("x"), 0o644)

	rel, got, err := Folders(d, "")
	if err != nil || rel != "" || strings.Join(got, ",") != "Books,livros" {
		t.Errorf("root: rel=%q folders=%v err=%v", rel, got, err)
	}
	rel, got, err = Folders(d, "livros")
	if err != nil || rel != "livros" || strings.Join(got, ",") != "ficcao" {
		t.Errorf("livros: rel=%q folders=%v err=%v", rel, got, err)
	}
	rel, _, err = Folders(d, "../..")
	if err != nil || rel != "" {
		t.Errorf("escape attempt should clamp to root: rel=%q err=%v", rel, err)
	}
}

func TestSendSkipsDuplicates(t *testing.T) {
	dev := Device{Mount: t.TempDir()}
	src := filepath.Join(t.TempDir(), "book.epub")
	os.WriteFile(src, []byte("the real book"), 0o644)

	// Same content under another name (e.g. copied by Calibre into Downloads/Calibre).
	mkdirs(t, dev.Mount, "Downloads/Calibre", "outra")
	os.WriteFile(filepath.Join(dev.Mount, "Downloads/Calibre", "perdido_em_marte.epub"), []byte("the real book"), 0o644)
	// Same title/author, different bytes (metadata rewritten by another program).
	os.WriteFile(filepath.Join(dev.Mount, "outra", "O Ceifador - Neal Shusterman.epub"), []byte("rewritten"), 0o644)

	books, err := Scan(context.Background(), dev)
	if err != nil || len(books) != 2 {
		t.Fatalf("scan: %v %+v", err, books)
	}
	ceifadorSrc := filepath.Join(t.TempDir(), "c.epub")
	os.WriteFile(ceifadorSrc, []byte("library copy"), 0o644)
	items := []Item{
		{BookID: 1, Title: "Perdido em Marte", Authors: []string{"Andy Weir"}, Format: "epub", Path: src},
		{BookID: 2, Title: "O ceifador", Authors: []string{"Neal Shusterman", "Guilherme Miranda"}, Format: "epub", Path: ceifadorSrc},
	}
	dest := filepath.Join(dev.Mount, "Lord Books")

	res := Send(context.Background(), Profiles[Generic], dest, items, SendOptions{OnDevice: NewIndex(books), Mount: dev.Mount})
	if res[0].Status != "exists" || res[0].Existing != "Downloads/Calibre/perdido_em_marte.epub" {
		t.Errorf("identical copy elsewhere: %+v", res[0])
	}
	if res[1].Status != "duplicate" || res[1].Existing != "outra/O Ceifador - Neal Shusterman.epub" {
		t.Errorf("same title/author: %+v", res[1])
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("nothing should have been copied, got %d files", len(entries))
	}

	res = Send(context.Background(), Profiles[Generic], dest, items[1:], SendOptions{OnDevice: NewIndex(books), Mount: dev.Mount, Force: true})
	if res[0].Status != "sent" {
		t.Errorf("forced send: %+v", res[0])
	}

	// The same book twice in one batch is copied once.
	fresh := Device{Mount: t.TempDir()}
	twice := []Item{items[0], items[0]}
	res = Send(context.Background(), Profiles[Generic], fresh.Mount, twice, SendOptions{OnDevice: NewIndex(nil), Mount: fresh.Mount})
	if res[0].Status != "sent" || res[1].Status != "exists" {
		t.Errorf("same book twice: %+v", res)
	}
}

func TestDeleteBookOnlyTouchesBooksOnDevice(t *testing.T) {
	dev := Device{Mount: t.TempDir()}
	mkdirs(t, dev.Mount, "Lord Books")
	book := filepath.Join(dev.Mount, "Lord Books", "Livro.epub")
	os.WriteFile(book, []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dev.Mount, "Lord Books", "._Livro.epub"), []byte("meta"), 0o644)
	os.WriteFile(filepath.Join(dev.Mount, "config.ini"), []byte("keep"), 0o644)
	outside := filepath.Join(t.TempDir(), "outside.epub")
	os.WriteFile(outside, []byte("keep"), 0o644)

	for _, bad := range []string{"config.ini", "", "Lord Books", "../" + filepath.Base(filepath.Dir(outside)) + "/outside.epub", "nao-existe.epub"} {
		if err := DeleteBook(dev, bad); err == nil {
			t.Errorf("deleting %q should fail", bad)
		}
	}
	if err := DeleteBook(dev, "Lord Books/Livro.epub"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(book); !os.IsNotExist(err) {
		t.Error("book still there")
	}
	if _, err := os.Stat(filepath.Join(dev.Mount, "Lord Books", "._Livro.epub")); !os.IsNotExist(err) {
		t.Error("macOS twin file left behind")
	}
	for _, keep := range []string{filepath.Join(dev.Mount, "config.ini"), outside} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s must not be touched", keep)
		}
	}
}

func TestTitleAuthorsFromName(t *testing.T) {
	title, authors := titleAuthorsFromName("O crime do bom nazista - Prêmio Jabuti 2024 - Samir Machado de Machado.epub")
	if title != "O crime do bom nazista - Prêmio Jabuti 2024" || len(authors) != 1 || authors[0] != "Samir Machado de Machado" {
		t.Errorf("got %q %v", title, authors)
	}
	if Fold("Perdido Em Marte!") != Fold("perdido em marte") || Fold("Ficção") != "ficcao" {
		t.Error("fold should ignore case, punctuation and accents")
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
	res := Send(context.Background(), Profiles[Xteink], dest, items, SendOptions{})
	if res[0].Status != "sent" || res[1].Status != "unsupported" {
		t.Fatalf("first send: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dest, "Livro - Ana.epub")); err != nil {
		t.Fatal("file not copied:", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dest, "*.lbpart")); len(matches) > 0 {
		t.Error("temp file left behind")
	}
	res = Send(context.Background(), Profiles[Xteink], dest, items[:1], SendOptions{})
	if res[0].Status != "exists" {
		t.Errorf("second send: %+v", res[0])
	}

	// Kindle can't open EPUB; without Calibre's converter it must be refused, not copied.
	kdest := filepath.Join(t.TempDir(), "documents")
	res = Send(context.Background(), Profiles[Kindle], kdest, items, SendOptions{})
	if res[0].Status != "unsupported" || res[1].Status != "sent" {
		t.Errorf("kindle without converter: %+v", res)
	}
}
