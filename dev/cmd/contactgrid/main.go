// contactgrid out.png columns in.png...: contact sheet of full-HD screenshots at a quarter of their size.
package main

import (
	"image"
	"image/png"
	"os"
)

// grid out cols in...: tiles images at 1/4 size
func main() {
	cols := int(os.Args[2][0] - '0')
	ins := os.Args[3:]
	const tw, th = 480, 270
	rows := (len(ins) + cols - 1) / cols
	out := image.NewRGBA(image.Rect(0, 0, cols*tw, rows*th))
	for i, fn := range ins {
		f, err := os.Open(fn)
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		ox, oy := i%cols*tw, i/cols*th
		for y := 0; y < th; y++ {
			for x := 0; x < tw; x++ {
				out.Set(ox+x, oy+y, img.At(x*4+2, y*4+2))
			}
		}
	}
	o, _ := os.Create(os.Args[1])
	png.Encode(o, out)
	o.Close()
}
