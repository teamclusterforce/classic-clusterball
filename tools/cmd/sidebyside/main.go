// sidebyside old.png new.png out.png oldlabel newlabel [stretch]: labelled before/after image.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func load(fn string) image.Image {
	f, err := os.Open(fn)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		panic(err)
	}
	return img
}

// label draws text with the 7x13 bitmap font, enlarged by an integer factor.
func label(dst *image.RGBA, x, y, scale int, s string) {
	small := image.NewRGBA(image.Rect(0, 0, len(s)*7+2, 16))
	d := &font.Drawer{Dst: small, Src: image.White, Face: basicfont.Face7x13, Dot: fixed.P(1, 12)}
	d.DrawString(s)
	xdraw.NearestNeighbor.Scale(dst, image.Rect(x, y, x+small.Bounds().Dx()*scale, y+16*scale), small, small.Bounds(), draw.Over, nil)
}

// sbs <old.png> <new.png> <out.png> <old label> <new label> [stretch]
// stretch: old image as a fullscreen 4:3 mode on a 16:9 monitor (16-bit, full width)
func main() {
	old, nw := load(os.Args[1]), load(os.Args[2])
	stretch := len(os.Args) > 6 && os.Args[6] == "stretch"
	if stretch {
		old = degrade(old)
	}
	const h, gap, band = 1080, 24, 70
	ow := old.Bounds().Dx() * h / old.Bounds().Dy()
	if stretch {
		ow = h * 16 / 9
	}
	out := image.NewRGBA(image.Rect(0, 0, ow+gap+nw.Bounds().Dx(), h+band))
	draw.Draw(out, out.Bounds(), &image.Uniform{color.RGBA{20, 20, 24, 255}}, image.Point{}, draw.Src)
	// the old image as a 1080p monitor shows a 640x480 mode: bilinear upscale
	xdraw.BiLinear.Scale(out, image.Rect(0, band, ow, band+h), old, old.Bounds(), draw.Src, nil)
	draw.Draw(out, image.Rect(ow+gap, band, ow+gap+nw.Bounds().Dx(), band+h), nw, nw.Bounds().Min, draw.Src)
	label(out, 10, 12, 3, os.Args[4])
	label(out, ow+gap+10, 12, 3, os.Args[5])
	f, _ := os.Create(os.Args[3])
	png.Encode(f, out)
	f.Close()
}
