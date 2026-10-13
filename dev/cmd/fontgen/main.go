// fontgen: builds a high-resolution replacement for Clusterball's 8x8-cell menu font atlas (128x128).
// Every cell of the original gets the same character from a TrueType font, rendered in the font's own
// proportions. Cells are 64 px wide and, with -tall, 128 px high: the original 8x8 cell maps to the middle
// 64x64 and the D3D layer draws each glyph quad at twice its height, so accents and descenders get room
// above and below without changing the game's layout. Glyphs the font lacks are the original, upscaled.
//
//	fontgen -orig ../tex_2.png -out ../font_bahn -font C:\Windows\Fonts\bahnschrift.ttf -bold 0.04 -cap 6 -tall
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const grid = 16 // cells per row
const ocell = 8 // original cell size

var cell = 64  // output cell width
var cellH = 64 // output cell height
var yoff = 0   // rows above the original cell's area

func loadOrig(fn string) [][]float64 {
	f, err := os.Open(fn)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		panic(err)
	}
	out := make([][]float64, grid*grid)
	for c := range out {
		cx, cy := c%grid*ocell, c/grid*ocell
		out[c] = make([]float64, ocell*ocell)
		for y := 0; y < ocell; y++ {
			for x := 0; x < ocell; x++ {
				_, _, _, a := img.At(cx+x, cy+y).RGBA()
				out[c][y*ocell+x] = float64(a) / 65535
			}
		}
	}
	return out
}

func inkBox(a []float64, w, h int, thr float64) (x0, y0, x1, y1 int, ok bool) {
	x0, y0, x1, y1 = w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if a[y*w+x] > thr {
				ok = true
				x0, x1 = min(x0, x), max(x1, x)
				y0, y1 = min(y0, y), max(y1, y)
			}
		}
	}
	return
}

func inkMidX(a []float64, n int) float64 {
	x0, _, x1, _, ok := inkBox(a, n, n, 0.25)
	if !ok {
		return float64(n) / 2
	}
	return float64(x0+x1+1) / 2
}

var sbuf sfnt.Buffer

func hasGlyph(f *opentype.Font, r rune) bool {
	idx, err := f.GlyphIndex(&sbuf, r)
	return err == nil && idx != 0
}

// renderRaw draws rune r at size px with its baseline origin at (ox, oy) into an s x s canvas.
func renderRaw(f *opentype.Font, r rune, size float64, s int, ox, oy float64) []float64 {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic(err)
	}
	defer face.Close()
	dst := image.NewAlpha(image.Rect(0, 0, s, s))
	d := font.Drawer{Dst: dst, Src: image.Opaque, Face: face, Dot: fixed.Point26_6{X: fixed.Int26_6(ox * 64), Y: fixed.Int26_6(oy * 64)}}
	d.DrawString(string(r))
	out := make([]float64, s*s)
	for i := range out {
		out[i] = float64(dst.Pix[i]) / 255
	}
	return out
}

// dilate grows the coverage by rad px with a round, soft-edged brush (synthetic bold).
func dilate(a []float64, w, h int, rad float64) []float64 {
	if rad <= 0 {
		return a
	}
	out := make([]float64, len(a))
	ri := int(math.Ceil(rad))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m := 0.0
			for dy := -ri; dy <= ri; dy++ {
				yy := y + dy
				if yy < 0 || yy >= h {
					continue
				}
				for dx := -ri; dx <= ri; dx++ {
					xx := x + dx
					if xx < 0 || xx >= w {
						continue
					}
					wt := rad + 0.5 - math.Hypot(float64(dx), float64(dy))
					if wt <= 0 {
						continue
					}
					if v := a[yy*w+xx] * math.Min(wt, 1); v > m {
						m = v
					}
				}
			}
			out[y*w+x] = m
		}
	}
	return out
}

// extent measures a glyph at size 100: ink above and below the baseline, and ink width (px).
func extent(f *opentype.Font, r rune) (asc, desc, wid float64, ok bool) {
	const s = 300
	raw := renderRaw(f, r, 100, s, 100, 200)
	x0, y0, x1, y1, ok := inkBox(raw, s, s, 0.02)
	return float64(200 - y0), float64(y1 + 1 - 200), float64(x1 - x0 + 1), ok
}

// glyph renders r into one cell with the ink centred at midX and the baseline at base (both in the
// original cell's 64x64 area). The letter body keeps its shape; only what would leave the cell (margin m)
// is squeezed: the part above the cap height and the part below the baseline. A glyph wider than the
// cell is scaled down whole.
func glyph(f *opentype.Font, size, base, capPx, bold, midX, m float64, r rune) []float64 {
	asc, desc, wid, ok := extent(f, r)
	out := make([]float64, cell*cellH)
	if !ok {
		return out
	}
	base += float64(yoff)
	pad := bold + m
	sc := 1.0
	if w := wid * size / 100; w > float64(cell)-2*pad {
		sc = (float64(cell) - 2*pad) / w
	}
	k := size * sc / 100
	ascPx, descPx := asc*k, desc*k
	half := wid * k / 2
	midX = math.Max(pad+half, math.Min(float64(cell)-pad-half, midX))
	s := 3 * max(cell, cellH)
	raw := renderRaw(f, r, size*sc, s, float64(cell), float64(cellH)+base)
	x0, _, x1, _, _ := inkBox(raw, s, s, 0.02)
	dx := midX - float64(x0+x1+1)/2

	top, bot := pad, float64(cellH)-pad
	capLine := math.Max(top+1, base-capPx)
	src := func(y float64) float64 { // output row -> source row
		if y < capLine && base-ascPx < top {
			return capLine - (capLine-y)*(ascPx-(base-capLine))/(capLine-top)
		}
		if y > base && base+descPx > bot {
			return base + (y-base)*descPx/(bot-base)
		}
		return y
	}
	sample := func(x, y float64) float64 { // bilinear; canvas rows are offset by cellH
		y += float64(cellH)
		ix, iy := int(math.Floor(x)), int(math.Floor(y))
		tx, ty := x-float64(ix), y-float64(iy)
		at := func(xx, yy int) float64 {
			if xx < 0 || yy < 0 || xx >= s || yy >= s {
				return 0
			}
			return raw[yy*s+xx]
		}
		return (at(ix, iy)*(1-tx)+at(ix+1, iy)*tx)*(1-ty) + (at(ix, iy+1)*(1-tx)+at(ix+1, iy+1)*tx)*ty
	}
	const ss = 4 // vertical sub-samples per output pixel (squeezed rows average their source span)
	for y := 0; y < cellH; y++ {
		for x := 0; x < cell; x++ {
			v := 0.0
			for j := 0; j < ss; j++ {
				v += sample(float64(x)-dx, src(float64(y)+(float64(j)+0.5)/ss)-0.5)
			}
			out[y*cell+x] = v / ss
		}
	}
	return dilate(out, cell, cellH, bold)
}

// upscaleOrig enlarges an original 8x8 cell bilinearly and steepens the edge, for glyphs the font lacks.
func upscaleOrig(o []float64) []float64 {
	k := float64(cell) / ocell
	out := make([]float64, cell*cellH)
	at := func(x, y int) float64 {
		x, y = max(0, min(ocell-1, x)), max(0, min(ocell-1, y))
		return o[y*ocell+x]
	}
	for y := 0; y < cell; y++ {
		for x := 0; x < cell; x++ {
			fx, fy := (float64(x)+0.5)/k-0.5, (float64(y)+0.5)/k-0.5
			ix, iy := int(math.Floor(fx)), int(math.Floor(fy))
			tx, ty := fx-float64(ix), fy-float64(iy)
			v := (at(ix, iy)*(1-tx)+at(ix+1, iy)*tx)*(1-ty) + (at(ix, iy+1)*(1-tx)+at(ix+1, iy+1)*tx)*ty
			t := math.Max(0, math.Min(1, (v-0.3)/0.4))
			out[(y+yoff)*cell+x] = t * t * (3 - 2*t)
		}
	}
	return out
}

func main() {
	origFn := flag.String("orig", "../tex_2.png", "original 128x128 atlas")
	outBase := flag.String("out", "../font_hd", "output base name (.png, _preview.png, .dds)")
	fontFn := flag.String("font", `C:\Windows\Fonts\bahnschrift.ttf`, "TrueType font")
	cellF := flag.Int("cell", 64, "output cell width")
	tall := flag.Bool("tall", false, "cells twice as high as wide (the layer doubles the glyph quads' height)")
	boldEm := flag.Float64("bold", 0.025, "synthetic bold: stroke growth per side, in em")
	margin := flag.Float64("margin", 1, "free pixels at the cell edges")
	capO := flag.Float64("cap", 0, "cap height in original pixels (0: largest size at which descenders fit)")
	baseO := flag.Float64("base", 7, "baseline in original pixels (with -cap)")
	flag.Parse()
	cell, cellH = *cellF, *cellF
	if *tall {
		cellH, yoff = 2*cell, cell/2
	}
	k := float64(cell) / ocell

	orig := loadOrig(*origFn)
	b, err := os.ReadFile(*fontFn)
	if err != nil {
		panic(err)
	}
	f, err := opentype.Parse(b)
	if err != nil {
		panic(err)
	}

	maxAsc, maxDesc := 0.0, 0.0
	for _, r := range "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789" {
		if a, d, _, ok := extent(f, r); ok {
			maxAsc, maxDesc = math.Max(maxAsc, a), math.Max(maxDesc, d)
		}
	}
	capH, _, _, _ := extent(f, 'H')
	// default: the largest size at which letters fit the 8x8 area, solved from
	// cell = (asc+desc)*size/100 + 2*(bold+margin)
	size := 100 * (float64(cell) - 2**margin) / (maxAsc + maxDesc + 2*100**boldEm)
	bold := *boldEm * size
	base := *margin + bold + maxAsc*size/100
	if *capO > 0 { // cap height (with the bold stroke) and baseline as in the original
		size = *capO * k / (capH/100 + 2**boldEm)
		bold = *boldEm * size
		base = *baseO*k - bold
	}
	capPx := capH * size / 100
	fmt.Printf("size %.1f px, baseline %.1f, bold %.1f px, cap height %.2f original px (original: 6), cell %dx%d\n",
		size, base, bold, (capPx+2*bold)/k, cell, cellH)

	// cells 96+ hold the game's own selection of accented letters (read off the original glyphs)
	ext := map[int]rune{}
	for i, r := range []rune("ÇüéâäàåçêëèïîìÄÅÉ¨Æôöòûÿ ÖÜ¢£¥₧ƒ¿¡ ßÑñ ÓóÁáÜûÚú") {
		if r != ' ' {
			ext[96+i] = r
		}
	}
	W, H := grid*cell, grid*cellH
	atlas := make([]float64, W*H)
	for c := 0; c < grid*grid; c++ {
		if _, _, _, _, ok := inkBox(orig[c], ocell, ocell, 0.1); !ok {
			continue
		}
		r := rune(0)
		switch {
		case c < 95:
			r = rune(c + 32)
		case c == 95:
			r = '↑'
		default:
			r = ext[c]
		}
		var g []float64
		if r != 0 && hasGlyph(f, r) {
			mid := float64(cell)/2 + (inkMidX(orig[c], ocell)-ocell/2)*k
			g = glyph(f, size, base, capPx, bold, mid, *margin, r)
		} else {
			fmt.Printf("cell %d: U+%04X not in the font, original glyph upscaled\n", c, r)
			g = upscaleOrig(orig[c])
		}
		cx, cy := c%grid*cell, c/grid*cellH
		for y := 0; y < cellH; y++ {
			copy(atlas[(cy+y)*W+cx:(cy+y)*W+cx+cell], g[y*cell:(y+1)*cell])
		}
	}
	writeOutputs(*outBase, atlas, W, H)
}

func writeOutputs(base string, a []float64, W, H int) {
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	prev := image.NewRGBA(image.Rect(0, 0, W, H))
	for i, v := range a {
		al := uint8(math.Round(math.Min(1, v) * 255))
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = 255, 255, 255, al
		m := float64(al) / 255
		prev.Set(i%W, i/W, color.RGBA{uint8(160 + 95*m), uint8(255 * m), uint8(160 + 95*m), 255})
	}
	save := func(fn string, im image.Image) {
		f, _ := os.Create(fn)
		png.Encode(f, im)
		f.Close()
	}
	save(base+".png", img)
	save(base+"_preview.png", prev)
	writeDDS(base+".dds", a, W, H)
	fmt.Println("wrote", base+".png", base+".dds")
}

// writeDDS stores white BGRA with the coverage in alpha and a box-filtered mip chain (legacy header,
// 32-bit uncompressed, which the layer's loader reads as B8G8R8A8). W and H are powers of two.
func writeDDS(fn string, a []float64, W, H int) {
	var levels [][]float64
	cur, w, h := a, W, H
	for {
		levels = append(levels, cur)
		if w == 1 && h == 1 {
			break
		}
		nw, nh := max(1, w/2), max(1, h/2)
		nx := make([]float64, nw*nh)
		at := func(x, y int) float64 { return cur[min(y, h-1)*w+min(x, w-1)] }
		for y := 0; y < nh; y++ {
			for x := 0; x < nw; x++ {
				nx[y*nw+x] = (at(2*x, 2*y) + at(2*x+1, 2*y) + at(2*x, 2*y+1) + at(2*x+1, 2*y+1)) / 4
			}
		}
		cur, w, h = nx, nw, nh
	}
	hd := make([]byte, 128)
	copy(hd, "DDS ")
	le := binary.LittleEndian
	le.PutUint32(hd[4:], 124)
	le.PutUint32(hd[8:], 0x1|0x2|0x4|0x8|0x1000|0x20000)
	le.PutUint32(hd[12:], uint32(H))
	le.PutUint32(hd[16:], uint32(W))
	le.PutUint32(hd[20:], uint32(W*4))
	le.PutUint32(hd[28:], uint32(len(levels)))
	le.PutUint32(hd[76:], 32)
	le.PutUint32(hd[80:], 0x41)
	le.PutUint32(hd[88:], 32)
	le.PutUint32(hd[92:], 0x00ff0000)
	le.PutUint32(hd[96:], 0x0000ff00)
	le.PutUint32(hd[100:], 0x000000ff)
	le.PutUint32(hd[104:], 0xff000000)
	le.PutUint32(hd[108:], 0x1000|0x8|0x400000)
	out := hd
	for _, l := range levels {
		for _, v := range l {
			out = append(out, 255, 255, 255, byte(math.Round(math.Min(1, v)*255)))
		}
	}
	os.WriteFile(fn, out, 0o644)
}
