package thumb

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestFromFileScalesDownKeepingRatio(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 600, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 600; x++ {
			src.Set(x, y, color.RGBA{200, 30, 30, 255})
		}
	}
	p := filepath.Join(t.TempDir(), "cover.png")
	f, _ := os.Create(p)
	png.Encode(f, src)
	f.Close()

	data, w, h, err := FromFile(p, 240)
	if err != nil {
		t.Fatal(err)
	}
	if w != 160 || h != 240 {
		t.Errorf("size %dx%d, want 160x240", w, h)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("not a JPEG:", err)
	}
	r, g, _, _ := img.At(80, 120).RGBA()
	if r>>8 < 180 || g>>8 > 60 {
		t.Errorf("colour not preserved: r=%d g=%d", r>>8, g>>8)
	}
}

// A tiny PNG that declares 100000×100000 pixels would make Decode allocate ~40 GB.
func TestFromFileRejectsHugeDimensions(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1)))
	data := buf.Bytes()
	// IHDR width and height live at bytes 16-23; patch them and fix the chunk CRC.
	binary.BigEndian.PutUint32(data[16:20], 100000)
	binary.BigEndian.PutUint32(data[20:24], 100000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	p := filepath.Join(t.TempDir(), "bomb.png")
	os.WriteFile(p, data, 0o644)

	if _, _, _, err := FromFile(p, 160); err == nil {
		t.Error("huge image should be refused before decoding")
	}
}

func TestFromFileRejectsNonImages(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.jpg")
	os.WriteFile(p, []byte("not an image"), 0o644)
	if _, _, _, err := FromFile(p, 100); err == nil {
		t.Error("expected an error")
	}
}
