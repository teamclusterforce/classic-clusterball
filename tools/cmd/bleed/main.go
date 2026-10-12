package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// bleed outdir in...: copies PNGs, giving fully transparent texels the colour of their opaque neighbours
// (repeatedly), so an upscaler or a filter does not pull the colour-key colour into the edges.
func main() {
	out := os.Args[1]
	for _, fn := range os.Args[2:] {
		f, _ := os.Open(fn)
		src, err := png.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		b := src.Bounds()
		img := image.NewNRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				img.Set(x, y, color.NRGBAModel.Convert(src.At(x, y)))
			}
		}
		w, h := b.Dx(), b.Dy()
		known := make([]bool, w*h)
		for i := range known {
			known[i] = img.Pix[i*4+3] > 0
		}
		for pass := 0; pass < 64; pass++ {
			changed := false
			next := append([]bool(nil), known...)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					i := y*w + x
					if known[i] {
						continue
					}
					var r, g, bl, n int
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							xx, yy := x+dx, y+dy
							if xx < 0 || yy < 0 || xx >= w || yy >= h || !known[yy*w+xx] {
								continue
							}
							j := (yy*w + xx) * 4
							r += int(img.Pix[j]); g += int(img.Pix[j+1]); bl += int(img.Pix[j+2]); n++
						}
					}
					if n > 0 {
						img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2] = uint8(r/n), uint8(g/n), uint8(bl/n)
						next[i] = true
						changed = true
					}
				}
			}
			known = next
			if !changed {
				break
			}
		}
		o, _ := os.Create(filepath.Join(out, filepath.Base(fn)))
		png.Encode(o, img)
		o.Close()
	}
}
