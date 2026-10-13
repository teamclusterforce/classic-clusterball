// pixdiff a.png b.png x y w h: counts differing pixels and the mean difference inside a rectangle.
package main

import (
	"fmt"
	"image/png"
	"os"
	"strconv"
)

// diff a.png b.png x0 y0 w h: counts differing pixels and the mean absolute difference in a rectangle.
func main() {
	fa, _ := os.Open(os.Args[1])
	a, _ := png.Decode(fa)
	fb, _ := os.Open(os.Args[2])
	b, _ := png.Decode(fb)
	n := func(i int) int { v, _ := strconv.Atoi(os.Args[i]); return v }
	x0, y0, w, h := n(3), n(4), n(5), n(6)
	cnt, sum := 0, 0.0
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			d := abs(int(r1)-int(r2)) + abs(int(g1)-int(g2)) + abs(int(b1)-int(b2))
			if d > 3*257 {
				cnt++
			}
			sum += float64(d) / 3 / 257
		}
	}
	fmt.Printf("differing %d of %d, mean abs %.2f\n", cnt, w*h, sum/float64(w*h))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
