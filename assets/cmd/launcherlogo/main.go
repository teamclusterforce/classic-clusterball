// launcherlogo -game <game folder> -out logo_cb.png -cf logo_cf.png -cbde logo_cbde.png: the logos of the launcher's dark header.
// logo_cb: the Clusterball emblem and word mark of the loading box (16x upscale, black on yellow) in white on
// transparent - the darkness of the original becomes the alpha, so the emblem's mark stays cut out - with
// "RE-ENHANCED" in red Michroma centred under the word mark, as in the game's menu. logo_cf: the gold ClusterForce
// emblem with "TEAM" in Michroma above the gold CLUSTERFORCE word mark. logo_cbde: the rocket emblem and word mark of
// the community site clusterball.de, light, with its address under the word mark.
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
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Regions in the loading box's 16x upscale, which is stored bottom-up.
var (
	emblemRect = image.Rect(2330, 1040, 3370, 2020)
	wordRect   = image.Rect(1760, 780, 3880, 1050) // without the small trade mark at the right
)

// The word mark in clusterball_de_logo.png (706x120), above the site's subline.
var siteWordRect = image.Rect(118, 30, 706, 96)

const (
	alignLeft = iota
	alignCentre
	alignRight
)

const H = 240 // output height; the launcher shows it at 50 px

var (
	white = color.NRGBA{255, 255, 255, 255}
	red   = color.NRGBA{226, 40, 46, 255} // RE-ENHANCED, a touch lighter than in the game for the dark header
	grey  = color.NRGBA{200, 208, 222, 255}
)

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
	return trimAlpha(a)
}

// alphaFrom takes the alpha of a dark-on-transparent picture inside r as a mask, trimmed to its ink.
func alphaFrom(img image.Image, r image.Rectangle) *image.Alpha {
	a := image.NewAlpha(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			_, _, _, al := img.At(r.Min.X+x, r.Min.Y+y).RGBA()
			a.SetAlpha(x, y, color.Alpha{uint8(al >> 8)})
		}
	}
	return trimAlpha(a)
}

// visible is the bounding box of the pixels whose alpha exceeds 24 inside r.
func visible(alphaAt func(x, y int) uint8, r image.Rectangle) image.Rectangle {
	b := image.Rectangle{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if alphaAt(x, y) > 24 {
				b = b.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return b
}

func trimAlpha(a *image.Alpha) *image.Alpha {
	b := visible(func(x, y int) uint8 { return a.AlphaAt(x, y).A }, a.Bounds())
	out := image.NewAlpha(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Copy(out, image.Point{}, a, b, draw.Src, nil)
	return out
}

// canvas is the output: premultiplied, so scaled shapes keep clean edges.
type canvas struct{ *image.RGBA }

// mask paints colour c through the alpha mask a, scaled into r.
func (cv canvas) mask(a *image.Alpha, r image.Rectangle, c color.NRGBA) {
	s := image.NewAlpha(r)
	draw.CatmullRom.Scale(s, r, a, a.Bounds(), draw.Src, nil)
	draw.DrawMask(cv, r, image.NewUniform(c), image.Point{}, s, r.Min, draw.Over)
}

// picture scales an image into r.
func (cv canvas) picture(src image.Image, sr, r image.Rectangle) {
	draw.CatmullRom.Scale(cv, r, src, sr, draw.Over, nil)
}

// text renders s in Michroma with the given cap height, its ink's left edge at x and baseline at y; it returns
// the ink's width. align moves the anchor x to the ink's centre (alignCentre) or right edge (alignRight).
// Michroma is thin, so it is drawn emboldened.
func (cv canvas) text(face *opentype.Font, s string, capH float64, x, y int, c color.NRGBA, align int) int {
	// Cap height of Michroma is 0.7 of its em.
	fc, _ := opentype.NewFace(face, &opentype.FaceOptions{Size: capH / 0.7, DPI: 72, Hinting: font.HintingNone})
	defer fc.Close()
	w := font.MeasureString(fc, s).Ceil()
	m := image.NewAlpha(image.Rect(0, 0, w+8, int(capH*2)))
	d := font.Drawer{Dst: m, Src: image.Opaque, Face: fc, Dot: fixed.P(4, int(capH*1.5))}
	d.DrawString(s)
	m = embolden(m, int(capH/16))
	ink := visible(func(x, y int) uint8 { return m.AlphaAt(x, y).A }, m.Bounds())
	switch align {
	case alignCentre:
		x -= ink.Dx() / 2
	case alignRight:
		x -= ink.Dx()
	}
	dst := image.Rect(x, y-(int(capH*1.5)-ink.Min.Y), x+ink.Dx(), y-(int(capH*1.5)-ink.Min.Y)+ink.Dy())
	draw.DrawMask(cv, dst, image.NewUniform(c), image.Point{}, m, ink.Min, draw.Over)
	return ink.Dx()
}

// embolden widens the strokes of a text mask by r pixels (maximum over a disc), as the game's menu does for
// Michroma, which is too thin at small sizes.
func embolden(m *image.Alpha, r int) *image.Alpha {
	if r <= 0 {
		return m
	}
	b := m.Bounds()
	out := image.NewAlpha(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := uint8(0)
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					if dx*dx+dy*dy <= r*r {
						if a := m.AlphaAt(x+dx, y+dy).A; a > v {
							v = a
						}
					}
				}
			}
			out.SetAlpha(x, y, color.Alpha{v})
		}
	}
	return out
}

func load(p string) image.Image {
	f, err := os.Open(p)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		fmt.Println(p, err)
		os.Exit(1)
	}
	return img
}

func save(p string, cv canvas) {
	// Crop to the ink so the launcher can size the picture exactly.
	b := visible(func(x, y int) uint8 { return cv.RGBAAt(x, y).A }, cv.Bounds())
	b.Min.Y, b.Max.Y = 0, H
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), H))
	draw.Draw(out, out.Bounds(), cv, b.Min, draw.Src)
	f, err := os.Create(p)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	png.Encode(f, out)
	f.Close()
	fmt.Printf("wrote %s: %dx%d\n", p, out.Bounds().Dx(), H)
}

func main() {
	game := flag.String("game", `C:\temp\cfleg\run`, "game folder with textures\\up16")
	out := flag.String("out", "logo_cb.png", "Clusterball logo")
	cfOut := flag.String("cf", "logo_cf.png", "ClusterForce logo")
	cbdeOut := flag.String("cbde", "logo_cbde.png", "clusterball.de logo")
	assets := flag.String("assets", filepath.Join("cmd", "brand", "assets"), "folder with michroma.ttf, clusterforce_logo.png and the clusterball_de_*.png")
	flag.Parse()
	ttf, err := os.ReadFile(filepath.Join(*assets, "michroma.ttf"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	michroma, err := opentype.Parse(ttf)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Clusterball: emblem, word mark, RE-ENHANCED under the word mark.
	box := load(filepath.Join(*game, "textures", "up16", "ab219e4e8b4f5492.png"))
	em, word := alphaOf(box, emblemRect), alphaOf(box, wordRect)
	eh := H * 94 / 100
	ew := em.Bounds().Dx() * eh / em.Bounds().Dy()
	wh := H * 40 / 100
	ww := word.Bounds().Dx() * wh / word.Bounds().Dy()
	gap := H * 14 / 100
	capRE := float64(H) * 0.12
	blockTop := (H - (wh + int(capRE*0.75) + int(capRE))) / 2
	cv := canvas{image.NewRGBA(image.Rect(0, 0, ew+gap+ww+20, H))}
	cv.mask(em, image.Rect(0, (H-eh)/2, ew, (H-eh)/2+eh), white)
	cv.mask(word, image.Rect(ew+gap, blockTop, ew+gap+ww, blockTop+wh), white)
	cv.text(michroma, "RE-ENHANCED", capRE, ew+gap+ww/2, blockTop+wh+int(capRE*0.75)+int(capRE), red, alignCentre)
	save(*out, cv)

	// ClusterForce: the gold emblem, TEAM above the gold word mark.
	cf := load(filepath.Join(*assets, "clusterforce_logo.png"))
	alpha := func(x, y int) uint8 {
		_, _, _, a := cf.At(x, y).RGBA()
		return uint8(a >> 8)
	}
	all := visible(alpha, cf.Bounds())
	// The emblem sits above the word mark; split at the first empty row below the emblem's top.
	split := all.Min.Y
	for y := all.Min.Y; y < all.Max.Y; y++ {
		empty := true
		for x := all.Min.X; x < all.Max.X && empty; x++ {
			empty = alpha(x, y) <= 24
		}
		if empty && y > all.Min.Y+10 {
			split = y
			break
		}
	}
	cfEm := visible(alpha, image.Rect(all.Min.X, all.Min.Y, all.Max.X, split))
	cfWord := visible(alpha, image.Rect(all.Min.X, split, all.Max.X, all.Max.Y))
	ceh := H * 88 / 100
	cew := cfEm.Dx() * ceh / cfEm.Dy()
	cwh := H * 30 / 100
	cww := cfWord.Dx() * cwh / cfWord.Dy()
	capT := float64(H) * 0.15
	tTop := (H - (int(capT) + int(capT*0.7) + cwh)) / 2
	cv = canvas{image.NewRGBA(image.Rect(0, 0, cew+H*12/100+cww+20, H))}
	cv.picture(cf, cfEm, image.Rect(0, (H-ceh)/2, cew, (H-ceh)/2+ceh))
	tx := cew + H*12/100
	cv.text(michroma, "TEAM", capT, tx+2, tTop+int(capT), grey, alignLeft)
	wy := tTop + int(capT) + int(capT*0.7)
	cv.picture(cf, cfWord, image.Rect(tx, wy, tx+cww, wy+cwh))
	save(*cfOut, cv)

	// clusterball.de: the community site's rocket emblem and word mark, light on the dark header. The site's
	// own subline (the old network) gives way to the address.
	site := load(filepath.Join(*assets, "clusterball_de_emblem.png"))
	siteLogo := load(filepath.Join(*assets, "clusterball_de_logo.png"))
	sEm := alphaFrom(site, site.Bounds())
	sWord := alphaFrom(siteLogo, siteWordRect)
	seh := H * 84 / 100
	sew := sEm.Bounds().Dx() * seh / sEm.Bounds().Dy()
	swh := H * 26 / 100
	sww := sWord.Bounds().Dx() * swh / sWord.Bounds().Dy()
	capS := float64(H) * 0.095
	sTop := (H - (swh + int(capS*0.9) + int(capS))) / 2
	sx := sew + H*12/100
	cv = canvas{image.NewRGBA(image.Rect(0, 0, sx+sww+20, H))}
	cv.mask(sEm, image.Rect(0, (H-seh)/2, sew, (H-seh)/2+seh), grey)
	cv.mask(sWord, image.Rect(sx, sTop, sx+sww, sTop+swh), white)
	cv.text(michroma, "WWW.CLUSTERBALL.DE", capS, sx+sww, sTop+swh+int(capS*0.9)+int(capS), grey, alignRight)
	save(*cbdeOut, cv)
}
