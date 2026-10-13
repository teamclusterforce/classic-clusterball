// atlasfind dumpdir: lists dumped textures that look like a glyph atlas (grey ink on transparency).
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
)

// Lists dumped textures that look like a glyph atlas: grey-only opaque pixels, many transparent ones.
func main() {
	files, _ := filepath.Glob(filepath.Join(os.Args[1], "*.png"))
	for _, fn := range files {
		f, err := os.Open(fn)
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		b := img.Bounds()
		if b.Dx() < 64 || b.Dy() < 32 {
			continue
		}
		n, clear, grey, opaque := 0, 0, 0, 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := img.At(x, y).RGBA()
				n++
				if a < 0x1000 {
					clear++
					continue
				}
				opaque++
				d := int(r) - int(g)
				d2 := int(g) - int(bl)
				if d < 0x0c00 && d > -0x0c00 && d2 < 0x0c00 && d2 > -0x0c00 {
					grey++
				}
			}
		}
		if opaque == 0 {
			continue
		}
		cf := float64(clear) / float64(n)
		gf := float64(grey) / float64(opaque)
		if cf > 0.4 && cf < 0.97 && gf > 0.98 {
			fmt.Printf("%s %dx%d clear=%.2f grey=%.2f\n", filepath.Base(fn), b.Dx(), b.Dy(), cf, gf)
		}
	}
}
