// pngmean: mean RGB and alpha of the same file name in two folders (compares texture pack passes).
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
)

// Mean RGB and alpha of the same file name in two folders.
func mean(fn string) (float64, float64) {
	f, err := os.Open(fn)
	if err != nil {
		return -1, -1
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return -2, -2
	}
	b := img.Bounds()
	var s, a float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, al := img.At(x, y).RGBA()
			s += float64(r+g+bl) / 3 / 257
			a += float64(al) / 257
		}
	}
	n := float64(b.Dx() * b.Dy())
	return s / n, a / n
}

func main() {
	files, _ := filepath.Glob(filepath.Join(os.Args[1], "*.png"))
	for i, f := range files {
		if i >= 8 {
			break
		}
		r1, a1 := mean(f)
		r2, a2 := mean(filepath.Join(os.Args[2], filepath.Base(f)))
		fmt.Printf("%s  orig rgb %.0f a %.0f   up rgb %.0f a %.0f\n", filepath.Base(f), r1, a1, r2, a2)
	}
}
