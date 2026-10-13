// brand -game <game folder>: draws the Clusterball Re-Enhanced and Team ClusterForce marks into the upscaled menu
// textures (<game>\textures\up and up16, made by build_texture_pack.ps1 / build_ui_pack.ps1) and writes them to
// <game>\textures\brand\up and brand\up16; assets\brand_pack.ps1 compresses those into the hd/hd16 packs.
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
	// The loading progress bar, measured on screen as de0000 after the renderer's default grading (contrast 1.05,
	// saturation 1.1) and taken back through it.
	barRed = color.NRGBA{202, 0, 0, 255}
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
	// Loading box (358 x 130, stored bottom-up): Clusterball word mark at x 110-248, y 65-80 from the top; the game
	// draws its red progress bar over rows 88-94 and "Laedt..." from row 111. The mark sits centred under the word,
	// between the bar and "Laedt...", in the bar's own red.
	{"ab219e4e8b4f5492", 358, 130, true, func(c *canvas) {
		c.textCentered("RE-ENHANCED", barRed, 7, 179, 200, 105)
	}},
}

// Menu entries (256 x 32): white extended capitals with a dark shadow, the highlighted state with a frame.
// The upscaled originals carry the stray dark pixels around the letters as crumbs, so they are drawn anew in
// Michroma, a Microgramma-style face like the original, fitted to the original's text and frame.
var words = []struct {
	hash, text string
	box        bool
}{
	{"2b5ce9a49ffc878c", "TRAINING", false}, {"f033c5e15bfdb262", "TRAINING", true},
	{"5f2be42dd2342509", "SPIEL HOSTEN", false}, {"525a6be054738692", "SPIEL HOSTEN", true},
	{"d3baaa70129e1cde", "SPIEL BEITRETEN", false}, {"2de230a21dcba43e", "SPIEL BEITRETEN", true},
	{"ad978b78be68d2b7", "GESP. SPIELE", false}, {"8d8939667fa656c7", "GESP. SPIELE", true},
	{"65340e195a1d52de", "SETUP", false}, {"b3c75defb141093d", "SETUP", true},
	{"cc84efceb8afd6eb", "ENDE", false}, {"2a50ae57051ad87b", "ENDE", true},
	{"b98fbe17a4976b76", "WELT VERLASSEN", true},
	{"afbb605c3655fc09", "HOST MATCH", false}, {"1919830190985dd1", "HOST MATCH", true},
	{"ea70eb058d11dde2", "JOIN MATCH", false}, {"fb24d0e9daa4f982", "JOIN MATCH", true},
	{"775414207255d1eb", "VIEW MATCH", false}, {"0efc5d189d5605e4", "VIEW MATCH", true},
	{"93c60949da33854b", "QUIT", false}, {"fbb440496162b5e3", "QUIT", true},
	{"a948f06ed0814d16", "LEAVE VENUE", true},
}

// Tabs (84 x 24, stored bottom-up): the upscaled tab shape stays, only its label is drawn anew - colour and
// place come from the original (black on the active tab, grey on the others).
var tabs = []struct{ hash, text string }{
	{"012586b84024e3e8", "PILOTEN"}, {"d90f9d5f1d963f15", "PILOTEN"},
	{"084c36dc52f0d86c", "SPIELER"}, {"80a344dd61f1ffbf", "SPIELER"},
	{"2a252065229017e3", "LAN"}, {"15047d8f6c56f255", "LAN"},
	{"321edda91ac24a1d", "SYSTEM"}, {"3905cf74fa6aeff8", "SYSTEM"},
	{"35c915fa656ed5e0", "SETUP"}, {"ee1b714458f87d5b", "SETUP"},
	{"f9b559b57182a053", "CONTROLS"}, {"389203e31f99bb02", "CONTROLS"},
	{"40ccbbc985b80a59", "OPTIONEN"}, {"a2e50406863b3c3a", "OPTIONEN"},
	{"4c1c7f675128dab5", "STEUERUNG"}, {"42b3f0eb8697ce48", "STEUERUNG"},
	{"43e7e1d0356de7d0", "INTERNET"}, {"b8df12dc78920845", "INTERNET"},
	{"595a9268b446a44f", "PLAYER"}, {"7a12f6778d33c848", "PLAYER"},
	{"e7c127a1ab25b37d", "OPTIONS"}, {"6eeb9b1e84325ad9", "OPTIONS"},
	{"da942a9faec3fd2a", "CHAT"}, {"7f354ae72dc54c16", "CHAT"},
	{"cd99686c678d2965", "FLYERS"}, {"dc1a9e023031deb4", "FLYERS"},
}

var face *opentype.Font
var gameDir string

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
	gameDir = *game
	for _, w := range words {
		w := w
		marks = append(marks, mark{w.hash, 256, 32, false, func(c *canvas) { c.word(w.hash, w.text, w.box) }})
	}
	for _, t := range tabs {
		t := t
		marks = append(marks, mark{t.hash, 84, 24, true, func(c *canvas) { c.tabLabel(t.hash, t.text) }})
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

// word draws a menu entry anew: white text with a dark shadow over exactly the area the original's text covers,
// and for the highlighted state the 1-pixel frame around it. Geometry is measured on the original texture
// (textures\dump), whose stray dark pixels are left out.
func (c *canvas) word(hash, text string, box bool) {
	orig, err := loadNRGBA(filepath.Join(gameDir, "textures", "dump", hash+".png"))
	if err != nil {
		fmt.Println("no original for", hash+":", err)
		return
	}
	brightBox := func(r image.Rectangle) image.Rectangle {
		out := image.Rectangle{}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				p := orig.NRGBAAt(x, y)
				if p.A > 128 && p.R > 180 && p.G > 180 && p.B > 180 {
					out = out.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return out
	}
	all := brightBox(orig.Bounds())
	textR := all
	if box {
		textR = brightBox(all.Inset(3))
	}
	if textR.Empty() {
		fmt.Println("no text found in", hash)
		return
	}
	W, H := orig.Bounds().Dx(), orig.Bounds().Dy()
	c.clear(0, 0, float64(W), float64(H))

	fitted := c.fittedText(text, textR, 0.45)
	x0, y0 := int(math.Round(float64(textR.Min.X)*c.k)), int(math.Round(float64(textR.Min.Y)*c.k))
	off := int(math.Round(c.k)) // shadow one original pixel down and right, like the original
	c.put(fitted, x0+off, y0+off, color.NRGBA{40, 40, 40, 255})
	c.put(fitted, x0, y0, color.NRGBA{255, 255, 255, 255})

	if box {
		t := int(math.Max(1, math.Round(c.k))) // frame: one original pixel wide
		fx0, fy0 := int(math.Round(float64(all.Min.X)*c.k)), int(math.Round(float64(all.Min.Y)*c.k))
		fx1, fy1 := int(math.Round(float64(all.Max.X)*c.k)), int(math.Round(float64(all.Max.Y)*c.k))
		edge := func(x0, y0, x1, y1 int) {
			m := image.NewAlpha(image.Rect(0, 0, x1-x0, y1-y0))
			for i := range m.Pix {
				m.Pix[i] = 255
			}
			c.put(m, x0, y0, color.NRGBA{255, 255, 255, 255})
		}
		edge(fx0, fy0, fx1, fy0+t)
		edge(fx0, fy1-t, fx1, fy1)
		edge(fx0, fy0, fx0+t, fy1)
		edge(fx1-t, fy0, fx1, fy1)
	}
}

// fittedText renders text emboldened by bold (original's pixels per side), crops it to its ink and scales it to
// exactly the rectangle r (original's pixels): the coverage mask in canvas pixels.
func (c *canvas) fittedText(text string, r image.Rectangle, bold float64) *image.Alpha {
	fc, _ := opentype.NewFace(face, &opentype.FaceOptions{Size: c.fontSize(text, float64(r.Dy()), 1e9), DPI: 72, Hinting: font.HintingNone})
	defer fc.Close()
	br := bold * c.k
	pad := int(math.Ceil(br)) + 2
	m := fc.Metrics()
	asc, desc := m.Ascent.Ceil(), m.Descent.Ceil()
	mask := image.NewAlpha(image.Rect(0, 0, int(math.Ceil(float64(font.MeasureString(fc, text))/64))+2*pad, asc+desc+2*pad))
	d := font.Drawer{Dst: mask, Src: image.Opaque, Face: fc, Dot: fixed.P(pad, pad+asc)}
	d.DrawString(text)
	mask = dilateMask(mask, br)
	ink := image.Rectangle{}
	for y := 0; y < mask.Bounds().Dy(); y++ {
		for x := 0; x < mask.Bounds().Dx(); x++ {
			if mask.AlphaAt(x, y).A > 0 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	tw, th := int(math.Round(float64(r.Dx())*c.k)), int(math.Round(float64(r.Dy())*c.k))
	fitted := image.NewAlpha(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(fitted, fitted.Bounds(), mask, ink, draw.Src, nil)
	return fitted
}

// tabLabel replaces the label of a tab: the area of the original's dark text (inside the tab's outline) is filled
// with the tab's colour from the upscale and the text drawn anew in the original's text colour.
func (c *canvas) tabLabel(hash, text string) {
	orig, err := loadNRGBA(filepath.Join(gameDir, "textures", "dump", hash+".png"))
	if err != nil {
		fmt.Println("no original for", hash+":", err)
		return
	}
	H := orig.Bounds().Dy()
	at := func(x, y int) color.NRGBA { // y counted from the top as the player sees it
		return orig.NRGBAAt(x, H-1-y)
	}
	lum := func(p color.NRGBA) int { return (int(p.R)*299 + int(p.G)*587 + int(p.B)*114) / 1000 }
	grey := func(p color.NRGBA) bool {
		return max(p.R, p.G, p.B)-min(p.R, p.G, p.B) < 40
	}
	// The tab's face: the bright grey area (the purple frame and the dark bar under inactive tabs lie outside).
	tab := image.Rectangle{}
	for y := 0; y < H; y++ {
		for x := 0; x < orig.Bounds().Dx(); x++ {
			if p := at(x, y); p.A > 128 && grey(p) && lum(p) > 200 {
				tab = tab.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	// The label: dark grey pixels between the first and the last bright pixel of each row (the tab's outline lies
	// outside), leaving out the dark bar that runs across the whole width.
	dark := func(p color.NRGBA) bool { return p.A > 128 && grey(p) && lum(p) < 170 }
	bright := func(p color.NRGBA) bool { return p.A > 128 && grey(p) && lum(p) > 200 }
	textR, darkest := image.Rectangle{}, 255
	for y := tab.Min.Y; y < tab.Max.Y; y++ {
		x0, x1, n := -1, -1, 0
		for x := tab.Min.X; x < tab.Max.X; x++ {
			p := at(x, y)
			if dark(p) {
				n++
			}
			if bright(p) {
				if x0 < 0 {
					x0 = x
				}
				x1 = x
			}
		}
		if x0 < 0 || n > tab.Dx()*7/10 {
			continue
		}
		for x := x0 + 1; x < x1; x++ {
			if p := at(x, y); dark(p) {
				textR = textR.Union(image.Rect(x, y, x+1, y+1))
				darkest = min(darkest, lum(p))
			}
		}
	}
	if textR.Empty() {
		fmt.Println("no label found in", hash)
		return
	}
	// Text colour: mean of the darkest label pixels; background: the upscale left of the label.
	var sr, sg, sb, n int
	for y := textR.Min.Y; y < textR.Max.Y; y++ {
		for x := textR.Min.X; x < textR.Max.X; x++ {
			if p := at(x, y); p.A > 128 && lum(p) <= darkest+40 {
				sr += int(p.R)
				sg += int(p.G)
				sb += int(p.B)
				n++
			}
		}
	}
	ink := color.NRGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}
	cy := int((float64(textR.Min.Y+textR.Max.Y) / 2) * c.k)
	bgx := int(float64(textR.Min.X-2) * c.k)
	CH := c.img.Bounds().Dy()
	bg := c.img.NRGBAAt(bgx, CH-1-cy)
	bg.A = 255
	cover := textR.Inset(-1)
	fill := image.NewAlpha(image.Rect(0, 0, int(float64(cover.Dx())*c.k), int(float64(cover.Dy())*c.k)))
	for i := range fill.Pix {
		fill.Pix[i] = 255
	}
	c.put(fill, int(float64(cover.Min.X)*c.k), int(float64(cover.Min.Y)*c.k), bg)
	c.put(c.fittedText(text, textR, 0.45), int(float64(textR.Min.X)*c.k), int(float64(textR.Min.Y)*c.k), ink)
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
