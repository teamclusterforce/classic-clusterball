// pixdump image.png x y w h: prints the RGBA values of a small rectangle.
package main

import (
	"fmt"
	"image/png"
	"os"
	"strconv"
)

// px img x0 y0 w h: prints RGBA of a small rectangle
func main() {
	f, _ := os.Open(os.Args[1])
	img, _ := png.Decode(f)
	n := func(i int) int { v, _ := strconv.Atoi(os.Args[i]); return v }
	x0, y0, w, h := n(2), n(3), n(4), n(5)
	for y := y0; y < y0+h; y++ {
		fmt.Printf("%3d:", y)
		for x := x0; x < x0+w; x++ {
			c := img.At(x, y)
			r, g, b, a := c.RGBA()
			fmt.Printf(" %02x%02x%02x/%02x", r>>8, g>>8, b>>8, a>>8)
		}
		fmt.Println()
	}
}
