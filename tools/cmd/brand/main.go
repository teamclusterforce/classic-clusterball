// brand -game <game folder>: draws the Clusterball Re-Enhanced and Team ClusterForce marks into the upscaled menu
// textures (<game>\textures\up and up16, made by build_texture_pack.ps1 / build_ui_pack.ps1) and writes them to
// <game>\textures\brand\up and brand\up16; tools\brand_pack.ps1 compresses those into the hd/hd16 packs.
// The sources stay untouched, so the tool can run again after the packs were rebuilt.
//
// Marks (positions in the game's own pixels, measured on the original textures):
//   - "RE-ENHANCED" to the right of the Clusterball logo on the menu header (right header tile)
//   - the ClusterForce emblem and "TEAM CLUSTERFORCE" on the left of the yellow menu bar
//   - "RE-ENHANCED" next to the Clusterball logo of the loading box
package main

import (
	"bytes"
	_ "embed"
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

//go:embed assets/michroma.ttf
var michromaTTF []byte

//go:embed assets/clusterforce_logo.png
var cfLogoPNG []byte

var (
	red   = color.NRGBA{200, 16, 30, 255} // the red of the Clusterball slogan
	black = color.NRGBA{0, 0, 0, 255}     // like the Clusterball emblem on the yellow bar
)

// canvas is an upscaled texture; k converts the original's pixels to the canvas's. flipped: the game stores
// the texture bottom-up (pictures it composes at run time), so y runs from the bottom.
type canvas struct {
	img     *image.NRGBA
	k       float64
	h       int // original height
	flipped bool
}

type mark struct {
	hash    string
	w, h    int // original size
	flipped bool
	draw    func(c *canvas)
}

var marks = []mark{
	// Clusterball logo of the menu header (256 x 64): word mark in rows 11-42, the slogan "THE EXTREME SPORTS
	// REVOLUTION" in rows 51-56, x 6-237. The slogan gives way to "RE-ENHANCED", centred under the word mark.
	{"c3d8173507326b40", 256, 64, false, func(c *canvas) {
		c.clear(0, 46, 256, 62)
		c.textCentered("RE-ENHANCED", red, 8, 121.5, 200, 58)
	}},
	// Left yellow bar tile (256 x 64): yellow from row 6 to 42.
	{"2d6e54ec3d6932bb", 256, 64, false, func(c *canvas) {
		w := c.emblem(black, 14, 9, 30)
		c.text("TEAM CLUSTERFORCE", black, 7, 14+w+8, 250, 28)
	}},
	// Loading box (358 x 130, stored bottom-up): Clusterball word mark at x 110-248, y 65-80 from the top.
	{"ab219e4e8b4f5492", 358, 130, true, func(c *canvas) {
		c.text("RE-ENHANCED", red, 7, 254, 344, 78)
	}},
}

var face *opentype.Font

func main() {
	game := flag.String("game", "", "game folder")
	flag.Parse()
	if *game == "" {
		fmt.Fprintln(os.Stderr, "usage: brand -game <game folder>")
		os.Exit(2)
	}
	var err error
	if face, err = opentype.Parse(michromaTTF); err != nil {
		panic(err)
	}
	tex := filepath.Join(*game, "textures")
	for _, pack := range []string{"up", "up16"} {
		out := filepath.Join(tex, "brand", pack)
		os.MkdirAll(out, 0o755)
		for _, m := range marks {
			src := filepath.Join(tex, pack, m.hash+".png")
			img, err := loadNRGBA(src)
			if err != nil {
				fmt.Println("skip", src+":", err)
				continue
			}
			c := &canvas{img: img, k: float64(img.Bounds().Dx()) / float64(m.w), h: m.h, flipped: m.flipped}
			m.draw(c)
			f, err := os.Create(filepath.Join(out, m.hash+".png"))
			if err != nil {
				panic(err)
			}
			png.Encode(f, img)
			f.Close()
			fmt.Println("branded", pack, m.hash)
		}
	}
}

func loadNRGBA(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	img := image.NewNRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	return img, nil
}

// put blends a coverage mask (upright, in canvas pixels, its origin at canvas (x0, y0) counted from the top as the
// player sees it) in colour col into the canvas.
func (c *canvas) put(mask *image.Alpha, x0, y0 int, col color.NRGBA) {
	H := c.img.Bounds().Dy()
	mb := mask.Bounds()
	for y := 0; y < mb.Dy(); y++ {
		cy := y0 + y
		if c.flipped {
			cy = H - 1 - cy
		}
		if cy < 0 || cy >= H {
			continue
		}
		for x := 0; x < mb.Dx(); x++ {
			cx := x0 + x
			if cx < 0 || cx >= c.img.Bounds().Dx() {
				continue
			}
			a := float64(mask.AlphaAt(mb.Min.X+x, mb.Min.Y+y).A) / 255
			if a == 0 {
				continue
			}
			p := c.img.NRGBAAt(cx, cy)
			p.R = uint8(float64(col.R)*a + float64(p.R)*(1-a))
			p.G = uint8(float64(col.G)*a + float64(p.G)*(1-a))
			p.B = uint8(float64(col.B)*a + float64(p.B)*(1-a))
			p.A = uint8(math.Max(float64(p.A), a*255))
			c.img.SetNRGBA(cx, cy, p)
		}
	}
}

// clear makes a rectangle of the original's pixels (y from the top) transparent.
func (c *canvas) clear(x0, y0, x1, y1 float64) {
	H := c.img.Bounds().Dy()
	for y := int(y0 * c.k); y < int(y1*c.k) && y < H; y++ {
		cy := y
		if c.flipped {
			cy = H - 1 - y
		}
		for x := int(x0 * c.k); x < int(x1*c.k) && x < c.img.Bounds().Dx(); x++ {
			c.img.SetNRGBA(x, cy, color.NRGBA{})
		}
	}
}

// textCentered draws s centred on cx, at most maxW wide; otherwise like text.
func (c *canvas) textCentered(s string, col color.NRGBA, capH, cx, maxW, baseline float64) {
	w := c.textWidth(s, capH, maxW)
	c.text(s, col, capH, cx-w/2, cx-w/2+maxW, baseline)
}

// fontSize is the size (canvas pixels) at which s has cap height capH and is at most maxW wide (original's pixels).
func (c *canvas) fontSize(s string, capH, maxW float64) float64 {
	measure := func(size float64) (w, capPx float64) {
		fc, _ := opentype.NewFace(face, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
		defer fc.Close()
		b, _ := font.BoundString(fc, "H")
		return float64(font.MeasureString(fc, s)) / 64, float64(-b.Min.Y) / 64
	}
	_, cap100 := measure(100)
	size := 100 * capH * c.k / cap100
	if w, _ := measure(size); w > maxW*c.k {
		size *= maxW * c.k / w
	}
	return size
}

// textWidth is the width of s as text draws it, in the original's pixels.
func (c *canvas) textWidth(s string, capH, maxW float64) float64 {
	fc, _ := opentype.NewFace(face, &opentype.FaceOptions{Size: c.fontSize(s, capH, maxW), DPI: 72, Hinting: font.HintingNone})
	defer fc.Close()
	return float64(font.MeasureString(fc, s)) / 64 / c.k
}

// textBold thickens the strokes (per side, in the original's pixels): Michroma is a single thin weight, which at
// these sizes would come out hairline next to the bold Clusterball lettering.
const textBold = 0.4

// text draws s with its cap height capH, starting at x0 and shrunk to end before xmax, on baseline y (all in the
// original's pixels, y from the top).
func (c *canvas) text(s string, col color.NRGBA, capH, x0, xmax, baseline float64) {
	fc, _ := opentype.NewFace(face, &opentype.FaceOptions{Size: c.fontSize(s, capH, xmax-x0), DPI: 72, Hinting: font.HintingNone})
	defer fc.Close()
	r := textBold * c.k
	pad := int(math.Ceil(r)) + 2
	w := int(math.Ceil(float64(font.MeasureString(fc, s))/64)) + 2*pad
	m := fc.Metrics()
	asc, desc := m.Ascent.Ceil(), m.Descent.Ceil()
	mask := image.NewAlpha(image.Rect(0, 0, w, asc+desc+2*pad))
	d := font.Drawer{Dst: mask, Src: image.Opaque, Face: fc, Dot: fixed.P(pad, pad+asc)}
	d.DrawString(s)
	c.put(dilateMask(mask, r), int(math.Round(x0*c.k))-pad, int(math.Round(baseline*c.k))-asc-pad, col)
}

// dilateMask widens the coverage by r pixels with a round, soft-edged brush.
func dilateMask(a *image.Alpha, r float64) *image.Alpha {
	if r <= 0 {
		return a
	}
	b := a.Bounds()
	out := image.NewAlpha(b)
	ri := int(math.Ceil(r))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			m := 0.0
			for dy := -ri; dy <= ri; dy++ {
				for dx := -ri; dx <= ri; dx++ {
					p := image.Pt(x+dx, y+dy)
					if !p.In(b) {
						continue
					}
					wt := math.Min(1, r+0.5-math.Hypot(float64(dx), float64(dy)))
					if wt <= 0 {
						continue
					}
					if v := float64(a.AlphaAt(p.X, p.Y).A) * wt; v > m {
						m = v
					}
				}
			}
			out.SetAlpha(x, y, color.Alpha{uint8(m)})
		}
	}
	return out
}

// emblem draws the ClusterForce emblem (the logo without its word mark) as a silhouette of height h at (x, y)
// and returns its width - all in the original's pixels.
func (c *canvas) emblem(col color.NRGBA, x, y, h float64) float64 {
	logo, err := png.Decode(bytes.NewReader(cfLogoPNG))
	if err != nil {
		panic(err)
	}
	// The emblem is the part of the logo above its word mark (which starts below row 340 of 600).
	lb := logo.Bounds()
	x0, y0, x1, y1 := lb.Max.X, lb.Max.Y, -1, -1
	for yy := lb.Min.Y; yy < lb.Min.Y+340; yy++ {
		for xx := lb.Min.X; xx < lb.Max.X; xx++ {
			if _, _, _, a := logo.At(xx, yy).RGBA(); a > 0x8000 {
				x0, x1 = min(x0, xx), max(x1, xx)
				y0, y1 = min(y0, yy), max(y1, yy)
			}
		}
	}
	src := image.Rect(x0, y0, x1+1, y1+1)
	ph := int(math.Round(h * c.k))
	pw := int(math.Round(float64(src.Dx()) * float64(ph) / float64(src.Dy())))
	scaled := image.NewNRGBA(image.Rect(0, 0, pw, ph))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), logo, src, draw.Src, nil)
	mask := image.NewAlpha(scaled.Bounds())
	for i := range mask.Pix {
		mask.Pix[i] = scaled.Pix[i*4+3]
	}
	c.put(mask, int(math.Round(x*c.k)), int(math.Round(y*c.k)), col)
	return float64(pw) / c.k
}
