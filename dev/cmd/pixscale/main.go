// pixscale: enlarges small UI sprites with Scale2x (EPX), applied -passes times (2 = 4x, 4 = 16x).
// Unlike a learned upscaler it never invents shapes, so 5-px text stays legible; it rounds the
// staircase of diagonals. Colours closer than -tol per channel count as equal (the sprites carry
// a little noise from their JPEG/BMP sources).
//
//	pixscale -passes 2 -o outdir in1.png in2.png ...
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

var tol = 24

func eq(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool {
		if x > y {
			return int(x-y) <= tol
		}
		return int(y-x) <= tol
	}
	if a.A == 0 && b.A == 0 {
		return true
	}
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

func scale2x(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w*2, h*2))
	at := func(x, y int) color.NRGBA {
		x, y = max(0, min(w-1, x)), max(0, min(h-1, y))
		return src.NRGBAAt(b.Min.X+x, b.Min.Y+y)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := at(x, y)
			a, bb, c, d := at(x, y-1), at(x+1, y), at(x-1, y), at(x, y+1)
			e1, e2, e3, e4 := p, p, p, p
			if eq(c, a) && !eq(c, d) && !eq(a, bb) {
				e1 = a
			}
			if eq(a, bb) && !eq(a, c) && !eq(bb, d) {
				e2 = bb
			}
			if eq(d, c) && !eq(d, bb) && !eq(c, a) {
				e3 = c
			}
			if eq(bb, d) && !eq(bb, a) && !eq(d, c) {
				e4 = d
			}
			dst.SetNRGBA(2*x, 2*y, e1)
			dst.SetNRGBA(2*x+1, 2*y, e2)
			dst.SetNRGBA(2*x, 2*y+1, e3)
			dst.SetNRGBA(2*x+1, 2*y+1, e4)
		}
	}
	return dst
}

func main() {
	passes := flag.Int("passes", 2, "Scale2x passes (2 = 4x, 4 = 16x)")
	out := flag.String("o", ".", "output folder")
	flag.IntVar(&tol, "tol", 24, "per-channel tolerance for equal colours")
	flag.Parse()
	n := 0
	for _, fn := range flag.Args() {
		f, err := os.Open(fn)
		if err != nil {
			continue
		}
		src, err := png.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		img := image.NewNRGBA(src.Bounds())
		for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
			for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
				img.Set(x, y, src.At(x, y))
			}
		}
		for i := 0; i < *passes; i++ {
			img = scale2x(img)
		}
		o, err := os.Create(filepath.Join(*out, filepath.Base(fn)))
		if err != nil {
			continue
		}
		png.Encode(o, img)
		o.Close()
		n++
	}
	fmt.Println(n, "images scaled")
}
