// alphasheet out.png zoom in.png...: stacks images over magenta (shows alpha), zoomed by an integer factor.
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// sheet out zoom in...: composites each PNG over magenta, zoomed, stacked vertically with a gap.
func main() {
	z := int(os.Args[2][0] - '0')
	var imgs []image.Image
	w, h := 0, 0
	for _, fn := range os.Args[3:] {
		f, err := os.Open(fn)
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		imgs = append(imgs, img)
		if img.Bounds().Dx()*z > w {
			w = img.Bounds().Dx() * z
		}
		h += img.Bounds().Dy()*z + 6
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	y0 := 0
	for _, img := range imgs {
		b := img.Bounds()
		for y := 0; y < b.Dy()*z; y++ {
			for x := 0; x < b.Dx()*z; x++ {
				r, g, bl, a := img.At(b.Min.X+x/z, b.Min.Y+y/z).RGBA()
				// premultiplied colour (RGBA() is premultiplied) over (160, 0, 160)
				af := float64(a) / 65535
				px := func(c uint32, bg float64) uint8 { return uint8(float64(c)/257 + bg*(1-af)) }
				out.Set(x, y0+y, color.RGBA{px(r, 160), px(g, 0), px(bl, 160), 255})
			}
		}
		y0 += b.Dy()*z + 6
	}
	o, _ := os.Create(os.Args[1])
	png.Encode(o, out)
	o.Close()
}
