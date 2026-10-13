package main

import (
	"image"
	"image/color"
)

// degrade reproduces the old fullscreen look: a 16-bit (RGB565) framebuffer with 4x4 ordered dithering.
func degrade(src image.Image) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(b)
	bayer := [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}
	q := func(v uint32, bits uint, d float64) uint8 {
		levels := float64(int(1)<<bits - 1)
		f := float64(v>>8)/255*levels + (d/16 - 0.5)
		if f < 0 {
			f = 0
		}
		if f > levels {
			f = levels
		}
		return uint8(int(f+0.5) * 255 / int(levels))
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := src.At(x, y).RGBA()
			d := bayer[y&3][x&3]
			out.Set(x, y, color.RGBA{q(r, 5, d), q(g, 6, d), q(bl, 5, d), 255})
		}
	}
	return out
}
