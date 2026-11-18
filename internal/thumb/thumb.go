// Package thumb makes small JPEG thumbnails of book covers, as readers expect in
// calibre's wireless protocol and in a Kindle's thumbnail folder.
package thumb

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
)

// FromFile decodes a JPEG, PNG or GIF cover and returns a JPEG no taller than maxHeight
// (smaller images are kept as they are), with its final width and height.
func FromFile(path string, maxHeight int) ([]byte, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, 0, err
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || maxHeight <= 0 {
		return nil, 0, 0, errors.New("imagem vazia")
	}
	h := min(maxHeight, b.Dy())
	w := max(1, b.Dx()*h/b.Dy())
	dst := downscale(src, w, h)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), w, h, nil
}

// downscale averages every source pixel that falls into each destination pixel
// (a box filter), which keeps covers sharp without moiré when shrinking a lot.
func downscale(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		y0 := b.Min.Y + y*sh/h
		y1 := max(y0+1, b.Min.Y+(y+1)*sh/h)
		for x := 0; x < w; x++ {
			x0 := b.Min.X + x*sw/w
			x1 := max(x0+1, b.Min.X+(x+1)*sw/w)
			var r, g, bl, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					r, g, bl = r+uint64(cr), g+uint64(cg), bl+uint64(cb)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 255})
		}
	}
	return dst
}
