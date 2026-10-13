// celltab atlas.png: prints the per-cell ink centre table of the game's font atlas (source of fontCellMid in the Command Center).
package main

import (
	"fmt"
	"image/png"
	"os"
	"strings"
)

// Prints the per-cell table of the game's 8x8 font atlas: ink centre x in atlas pixels, or -1 for an empty cell.
func main() {
	f, _ := os.Open(os.Args[1])
	img, _ := png.Decode(f)
	var b strings.Builder
	for c := 0; c < 256; c++ {
		x0, x1, any := 8, -1, false
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				_, _, _, a := img.At(c%16*8+x, c/16*8+y).RGBA()
				if float64(a)/65535 > 0.1 {
					any = true
				}
				if float64(a)/65535 > 0.25 {
					x0, x1 = min(x0, x), max(x1, x)
				}
			}
		}
		v := -1.0
		if any {
			v = 4
			if x1 >= 0 {
				v = float64(x0+x1+1) / 2
			}
		}
		if c%16 == 0 {
			b.WriteString("\t")
		}
		fmt.Fprintf(&b, "%g,", v)
		if c%16 == 15 {
			b.WriteString(" // row " + fmt.Sprint(c/16) + "\n")
		} else {
			b.WriteString(" ")
		}
	}
	fmt.Print(b.String())
}
