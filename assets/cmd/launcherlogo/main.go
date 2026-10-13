// launcherlogo -game <game folder> -out logo.png: the Clusterball logo for the launcher's dark header, white on
// transparent: the emblem and the word mark of the loading box (16x upscale, black on yellow), side by side. The
// darkness of the original becomes the alpha, so the emblem's mark stays cut out.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

// Regions in the loading box's 16x upscale, which is stored bottom-up.
var (
	emblemRect = image.Rect(2330, 1040, 3370, 2020)
	wordRect   = image.Rect(1760, 780, 3880, 1050) // without the small trade mark at the right
)

const H = 240 // output height; the launcher shows it at about 49 px

// alphaOf cuts r out of box, turns it upright and maps black to opaque, yellow to transparent.
func alphaOf(box image.Image, r image.Rectangle) *image.Alpha {
	a := image.NewAlpha(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			c := color.NRGBAModel.Convert(box.At(r.Min.X+x, r.Max.Y-1-y)).(color.NRGBA)
			l := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
			t := math.Min(1, math.Max(0, (1-l-0.25)/0.45))
			a.SetAlpha(x, y, color.Alpha{uint8(t * t * (3 - 2*t) * 255)})
		}
	}
	return trim(a)
}

// trim crops an alpha mask to its visible pixels.
func trim(a *image.Alpha) *image.Alpha {
	b := image.Rectangle{}
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.AlphaAt(x, y).A > 24 {
				b = b.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	out := image.NewAlpha(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Copy(out, image.Point{}, a, b, draw.Src, nil)
	return out
}

func main() {
	game := flag.String("game", `C:\temp\cfleg\run`, "game folder with textures\\up16")
	out := flag.String("out", "logo_cb.png", "output PNG")
	flag.Parse()
	f, err := os.Open(filepath.Join(*game, "textures", "up16", "ab219e4e8b4f5492.png"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	box, err := png.Decode(f)
	f.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	em, word := alphaOf(box, emblemRect), alphaOf(box, wordRect)
	eh := H * 92 / 100
	ew := em.Bounds().Dx() * eh / em.Bounds().Dy()
	wh := H * 46 / 100
	ww := word.Bounds().Dx() * wh / word.Bounds().Dy()
	gap := H * 16 / 100
	img := image.NewNRGBA(image.Rect(0, 0, ew+gap+ww, H))
	place := func(a *image.Alpha, r image.Rectangle) {
		s := image.NewAlpha(r)
		draw.CatmullRom.Scale(s, r, a, a.Bounds(), draw.Src, nil)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, s.AlphaAt(x, y).A})
			}
		}
	}
	place(em, image.Rect(0, (H-eh)/2, ew, (H-eh)/2+eh))
	place(word, image.Rect(ew+gap, (H-wh)/2, ew+gap+ww, (H-wh)/2+wh))
	o, err := os.Create(*out)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	png.Encode(o, img)
	o.Close()
	fmt.Printf("wrote %s: %dx%d\n", *out, img.Bounds().Dx(), H)
}
