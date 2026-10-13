// colmean image.png: mean brightness of the outer pixel columns - finds dark seams at picture edges.
package main

import (
	"fmt"
	"image/png"
	"os"
)

func main() {
	f, _ := os.Open(os.Args[1])
	img, _ := png.Decode(f)
	b := img.Bounds()
	for _, x := range []int{0, 1, 2, 5, b.Max.X - 3, b.Max.X - 2, b.Max.X - 1} {
		var s uint32
		for y := 0; y < b.Max.Y; y++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			s += (r + g + bl) >> 8
		}
		fmt.Printf("col %d mean %d\n", x, s/uint32(b.Max.Y*3))
	}
}
