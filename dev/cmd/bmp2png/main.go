// bmp2png file.bmp...: converts the layer's 24-bit BMP screenshots to PNG next to them and prints their mean brightness.
package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
)

func main() {
	for _, in := range os.Args[1:] {
		b, _ := os.ReadFile(in)
		off := binary.LittleEndian.Uint32(b[10:])
		w := int(int32(binary.LittleEndian.Uint32(b[18:])))
		h := int(int32(binary.LittleEndian.Uint32(b[22:])))
		stride := (w*3 + 3) &^ 3
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		var sum uint64
		for y := 0; y < h; y++ {
			row := b[int(off)+(h-1-y)*stride:]
			for x := 0; x < w; x++ {
				bl, g, r := row[x*3], row[x*3+1], row[x*3+2]
				sum += uint64(r) + uint64(g) + uint64(bl)
				img.Set(x, y, color.RGBA{r, g, bl, 255})
			}
		}
		f, _ := os.Create(strings.TrimSuffix(in, ".bmp") + ".png")
		png.Encode(f, img)
		f.Close()
		println(in, w, h, "mean", sum/uint64(w*h*3))
	}
}
