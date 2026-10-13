// cellart atlas.png cell...: prints 8x8 cells of the game's font atlas as text art (alpha as characters).
package main

import (
	"fmt"
	"image/png"
	"os"
	"strconv"
)

// asc atlas.png cell...: prints 8x8 cells as alpha art
func main() {
	f, _ := os.Open(os.Args[1])
	img, _ := png.Decode(f)
	for _, a := range os.Args[2:] {
		c, _ := strconv.Atoi(a)
		fmt.Println("cell", c)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				_, _, _, al := img.At(c%16*8+x, c/16*8+y).RGBA()
				fmt.Print(string(" .:-=+*#%@"[al*9/65535]))
			}
			fmt.Println()
		}
	}
}
