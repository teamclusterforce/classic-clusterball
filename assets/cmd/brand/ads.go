// assets/cmd/brand/ads.go
// Replaces the "Reklam Company" billboards of the arenas: the original boards advertise real companies of 2006
// (or say "Company Inc"), so every one of them is cleared and drawn anew - a framed panel per slot, the same in
// every venue: one ClusterForce board, one clusterball.de board and five well-known memes.
// Version: 2026.10.13
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"

	_ "embed"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed assets/team_clusterforce_logo.png
var teamCFLogoPNG []byte

//go:embed assets/clusterball_de_banner.png
var cbdeBannerPNG []byte

// The billboard textures (reklamcompany<slot>_<venue>, named after the level archives) and their slot. All are
// 2:1; the second billboard texture of Egal is a light cone, not a board, and stays.
var adBoards = []struct {
	hash string
	slot int
}{
	{"277b9b817aa45d66", 1}, {"60a84b3e88dc0fb4", 1}, {"81d6a2a5ae903746", 1}, {"881de75650108f46", 1},
	{"cb5a5483c823349c", 1}, {"d7466478b191447c", 1},
	{"241e1dd23ea566a2", 2}, {"2d3dbfac1709516e", 2}, {"6077fbb5a23db399", 2}, {"da2ae6aba12ae5a4", 2},
	{"209f66b213028a66", 3}, {"37b21db5eb101c65", 3}, {"408418dc6499f5fd", 3}, {"49dc66088d465b6a", 3},
	{"76c7f7afaa9f1622", 3},
	{"03e9c88575ac451a", 4}, {"5e7782c37f831d98", 4}, {"78bd0c84110bd0ae", 4}, {"877c1e63e34aa132", 4},
	{"1948c3fd4b7bd143", 5}, {"3c69c717e1292af0", 5}, {"4200fce3b6534fce", 5}, {"438374f4cf2b9299", 5},
	{"e4e97c24b65ce685", 5},
	{"19022db96737f922", 6}, {"45bb64b7f8a90097", 6}, {"9c8a2bebb71a7b41", 6}, {"b4529ed6d263ee40", 6},
	{"f09b086168a15b4b", 6},
	{"0a47b11b4ad40def", 7}, {"27d86640cd53a1e6", 7}, {"9567b54eba9c0a1a", 7}, {"a5daf3e4851c6649", 7},
}

// Board layout in units of a 256 x 128 board: the frame around the panel.
const (
	adW, adH         = 256.0, 128.0
	panelX0, panelY0 = 11.0, 10.0
	panelX1, panelY1 = 245.0, 118.0
	white            = 255
)

type adDesign struct {
	top, bottom color.NRGBA // panel gradient
	draw        func(c *canvas)
}

var adDesigns = map[int]adDesign{
	1: {rgb(18, 22, 32), rgb(6, 8, 12), func(c *canvas) {
		c.picture(teamCFLogoPNG, 128, 52, 206, 50)
		c.textCentered("WWW.CLUSTERFORCE.NET", rgb(200, 206, 216), 7, 128, 200, 103)
	}},
	2: {rgb(30, 48, 104), rgb(16, 26, 62), func(c *canvas) {
		c.picture(cbdeBannerPNG, 128, 64, 214, 61)
	}},
	// The memes: picture over the whole panel (cropped to it, around the given height), captions in the meme
	// style. Positions are board units; for the pictures they were read off the panel as the crop places them.
	3: {rgb(0, 0, 0), rgb(0, 0, 0), func(c *canvas) {
		c.cover(memeDistracted, 0.24)
		c.memeText("ONE MORE BALL", 9, 77, 70, 98)
		c.memeText("ME", 10, 153, 40, 88)
		c.memeText("THE GOAL ZONE", 9, 207, 66, 98)
	}},
	4: {rgb(0, 0, 0), rgb(0, 0, 0), func(c *canvas) {
		c.cover(memeThisIsFine, 0.5)
		c.memeText("3 ROCKETS LOCKED ON", 8, 69, 106, 25)
	}},
	5: {rgb(242, 232, 206), rgb(226, 208, 170), func(c *canvas) {
		c.picture(memeDoge, 65, 64, 108, 108)
		c.fadeInto(103, 119, rgb(242, 232, 206), rgb(226, 208, 170))
		c.dogeText("such ball", rgb(230, 0, 230), 9, 132, 32)
		c.dogeText("very chain", rgb(0, 150, 255), 9, 170, 58)
		c.dogeText("much goal", rgb(0, 170, 40), 9, 140, 86)
		c.dogeText("so rocket", rgb(255, 120, 0), 8, 196, 102)
		c.dogeText("wow", rgb(220, 0, 0), 12, 214, 30)
	}},
	6: {rgb(0, 0, 0), rgb(0, 0, 0), func(c *canvas) {
		c.cover(memeAlwaysHasBeen, 0.3)
		c.memeText("WAIT, IT'S ALL BALLS?", 10, 128, 214, 27)
		c.memeText("ALWAYS HAS BEEN", 10, 128, 214, 112)
	}},
	7: {rgb(0, 0, 0), rgb(0, 0, 0), func(c *canvas) {
		c.cover(memeEvilKermit, 0.5)
		c.memeText("ME: PLAY IT SAFE", 10, 128, 214, 27)
		c.memeText("ALSO ME: TAKE ALL THE BALLS", 10, 128, 214, 112)
	}},
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

// drawAds writes the boards for every pack the game folder has (textures\hd -> brand\up, hd16 -> brand\up16), each
// at the size the pack's texture has.
func drawAds(game string) {
	tex := filepath.Join(game, "textures")
	for _, p := range [][2]string{{"hd", "up"}, {"hd16", "up16"}} {
		if _, err := os.Stat(filepath.Join(tex, p[0])); err != nil {
			continue
		}
		out := filepath.Join(tex, "brand", p[1])
		os.MkdirAll(out, 0o755)
		for _, b := range adBoards {
			w, h, err := ddsSize(filepath.Join(tex, p[0], b.hash+".dds"))
			if err != nil {
				fmt.Println("skip ad", p[0], b.hash+":", err)
				continue
			}
			img := drawAd(b.slot, w, h)
			f, err := os.Create(filepath.Join(out, b.hash+".png"))
			if err != nil {
				panic(err)
			}
			png.Encode(f, img)
			f.Close()
			fmt.Println("ad", p[0], b.hash, "slot", b.slot, w, "x", h)
		}
	}
}

// ddsSize reads width and height from a DDS header.
func ddsSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var hdr [20]byte
	if _, err := f.Read(hdr[:]); err != nil || string(hdr[:4]) != "DDS " {
		return 0, 0, fmt.Errorf("not a DDS file")
	}
	return int(binary.LittleEndian.Uint32(hdr[16:])), int(binary.LittleEndian.Uint32(hdr[12:])), nil
}

// drawAd draws the board of a slot at w x h pixels.
func drawAd(slot, w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	c := &canvas{img: img, k: float64(w) / adW, h: int(adH)}
	d := adDesigns[slot]
	// Frame: a grey tube, lit from above, with a dark rim outside and a dark seam at the panel.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			u, v := (float64(x)+0.5)/c.k, (float64(y)+0.5)/c.k
			var col color.NRGBA
			if u >= panelX0 && u < panelX1 && v >= panelY0 && v < panelY1 {
				t := (v - panelY0) / (panelY1 - panelY0)
				col = mix(d.top, d.bottom, t)
				// a soft gloss across the upper third
				if t < 0.4 {
					col = mix(col, rgb(255, 255, 255), 0.07*(1-t/0.4))
				}
			} else {
				// distance into the frame band, 0 at the outer edge and 1 at the panel
				// (the side with the smallest one, which mitres the corners)
				sides := [4]float64{v / panelY0, u / panelX0, (adH - v) / (adH - panelY1), (adW - u) / (adW - panelX1)}
				a, side := sides[0], 0
				for i, s := range sides[1:] {
					if s < a {
						a, side = s, i+1
					}
				}
				g := 70 + 95*math.Sin(math.Pi*math.Min(1, a))
				if side < 2 {
					g += 18 // the upper and left sides face the light
				} else {
					g -= 14
				}
				if a < 0.12 || a > 0.9 {
					g = 48
				}
				gg := uint8(math.Max(0, math.Min(255, g)))
				col = rgb(gg, gg, gg)
			}
			img.SetNRGBA(x, y, col)
		}
	}
	d.draw(c)
	return img
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

// picture draws an embedded PNG centred on (cx, cy), fitted into maxW x maxH (board units), over the canvas.
func (c *canvas) picture(data []byte, cx, cy, maxW, maxH float64) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	sb := src.Bounds()
	s := math.Min(maxW/float64(sb.Dx()), maxH/float64(sb.Dy())) * c.k
	pw, ph := int(math.Round(float64(sb.Dx())*s)), int(math.Round(float64(sb.Dy())*s))
	x0, y0 := int(math.Round(cx*c.k))-pw/2, int(math.Round(cy*c.k))-ph/2
	draw.CatmullRom.Scale(c.img, image.Rect(x0, y0, x0+pw, y0+ph), src, sb, draw.Over, nil)
}

// adPreview draws the seven boards at 1024 x 512 one under the other into a PNG, for looking at them before a
// pack is built.
func adPreview(path string) {
	const w, h = 1024, 512
	sheet := image.NewNRGBA(image.Rect(0, 0, 2*w, 4*h))
	for slot := 1; slot <= 7; slot++ {
		x, y := (slot-1)%2*w, (slot-1)/2*h
		draw.Draw(sheet, image.Rect(x, y, x+w, y+h), drawAd(slot, w, h), image.Point{}, draw.Src)
	}
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	png.Encode(f, sheet)
	f.Close()
}

// Meme pictures (assets\memes) and Anton, a free face in the style of Impact that memes are captioned in.
var (
	//go:embed assets/memes/distracted_boyfriend.jpg
	memeDistracted []byte
	//go:embed assets/memes/this_is_fine.jpg
	memeThisIsFine []byte
	//go:embed assets/memes/doge.jpg
	memeDoge []byte
	//go:embed assets/memes/always_has_been.png
	memeAlwaysHasBeen []byte
	//go:embed assets/memes/evil_kermit.jpg
	memeEvilKermit []byte
	//go:embed assets/anton.ttf
	antonTTF []byte
)

var anton *opentype.Font

// cover fills the panel with a picture, scaled to cover it; what sticks out is cut off, horizontally around the
// middle and vertically around fy (0 = keep the top, 1 = keep the bottom).
func (c *canvas) cover(data []byte, fy float64) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	sb := src.Bounds()
	pw, ph := (panelX1-panelX0)*c.k, (panelY1-panelY0)*c.k
	s := math.Max(pw/float64(sb.Dx()), ph/float64(sb.Dy()))
	cw, ch := pw/s, ph/s // the part of the picture that shows, in its pixels
	x0 := float64(sb.Min.X) + (float64(sb.Dx())-cw)/2
	y0 := float64(sb.Min.Y) + (float64(sb.Dy())-ch)*fy
	crop := image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x0+cw)), int(math.Round(y0+ch)))
	dst := image.Rect(int(math.Round(panelX0*c.k)), int(math.Round(panelY0*c.k)), int(math.Round(panelX1*c.k)), int(math.Round(panelY1*c.k)))
	draw.CatmullRom.Scale(c.img, dst, src, crop, draw.Src, nil)
}

// antonMask renders s in Anton with cap height capH, at most maxW wide (board units): the coverage mask in canvas
// pixels with pad pixels of room on every side, and the ascent the baseline lies under.
func (c *canvas) antonMask(s string, capH, maxW float64, pad int) (*image.Alpha, int) {
	if anton == nil {
		var err error
		if anton, err = opentype.Parse(antonTTF); err != nil {
			panic(err)
		}
	}
	measure := func(size float64) (w, capPx float64) {
		fc, _ := opentype.NewFace(anton, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
		defer fc.Close()
		b, _ := font.BoundString(fc, "H")
		return float64(font.MeasureString(fc, s)) / 64, float64(-b.Min.Y) / 64
	}
	_, cap100 := measure(100)
	size := 100 * capH * c.k / cap100
	if w, _ := measure(size); w > maxW*c.k {
		size *= maxW * c.k / w
	}
	fc, _ := opentype.NewFace(anton, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	defer fc.Close()
	m := fc.Metrics()
	asc, desc := m.Ascent.Ceil(), m.Descent.Ceil()
	mask := image.NewAlpha(image.Rect(0, 0, int(math.Ceil(float64(font.MeasureString(fc, s))/64))+2*pad, asc+desc+2*pad))
	d := font.Drawer{Dst: mask, Src: image.Opaque, Face: fc, Dot: fixed.P(pad, pad+asc)}
	d.DrawString(s)
	return mask, asc
}

// memeText captions like a meme: white Anton capitals with a black outline, centred on cx, baseline y.
func (c *canvas) memeText(s string, capH, cx, maxW, baseline float64) {
	ow := capH * 0.1 * c.k
	pad := int(math.Ceil(ow)) + 2
	mask, asc := c.antonMask(s, capH, maxW, pad)
	x0 := int(math.Round(cx*c.k)) - mask.Bounds().Dx()/2
	y0 := int(math.Round(baseline*c.k)) - asc - pad
	c.put(dilateMask(mask, ow), x0, y0, rgb(0, 0, 0))
	c.put(mask, x0, y0, rgb(255, 255, 255))
}

// dogeText writes a doge line: lower case in one bright colour, no outline, centred on cx, baseline y.
func (c *canvas) dogeText(s string, col color.NRGBA, capH, cx, baseline float64) {
	mask, asc := c.antonMask(s, capH, 200, 2)
	c.put(mask, int(math.Round(cx*c.k))-mask.Bounds().Dx()/2, int(math.Round(baseline*c.k))-asc-2, col)
}

// fadeInto blends the panel from x0 to x1 (board units) into its gradient top..bottom, from not at all to fully:
// a picture that ends there runs out softly instead of with an edge.
func (c *canvas) fadeInto(x0, x1 float64, top, bottom color.NRGBA) {
	px0, px1 := int(math.Round(x0*c.k)), int(math.Round(x1*c.k))
	py0, py1 := int(math.Round(panelY0*c.k)), int(math.Round(panelY1*c.k))
	for y := py0; y < py1; y++ {
		bg := mix(top, bottom, float64(y-py0)/float64(py1-py0))
		for x := px0; x < px1; x++ {
			a := math.Min(1, math.Max(0, float64(x-px0)/float64(px1-px0)))
			c.img.SetNRGBA(x, y, mix(c.img.NRGBAAt(x, y), bg, a*a*(3-2*a)))
		}
	}
}
