package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Hochaufgelöster Ersatz für die Menü- und HUD-Schrift des Spiels.
//
// Clusterball zeichnet seine kleine Schrift aus einem 128 × 128 großen Atlas
// mit 16 × 16 Zellen zu je 8 × 8 Pixeln. Die Direct3D-Schicht (opengl32.dll)
// erkennt diesen Atlas an seinem Hash und zeichnet stattdessen
// textures\font\<hash>.dds, wenn es die Datei gibt. Dieser Ersatz wird hier aus
// der Bahnschrift erzeugt, die mit Windows ausgeliefert wird - so liegen weder
// Spielgrafik noch eine fremde Schrift im Repository oder im Launcher.
//
// Die Zellen sind 64 Pixel breit und 128 hoch: der Bereich der Original-Zelle
// liegt in der Mitte, die Schicht zeichnet jede Glyphe doppelt so hoch. So
// haben Akzente und Unterlängen Platz, ohne dass sich das Layout des Spiels
// ändert.

// fontAtlasHash ist der Hash (Größe + Pixel, FNV-1a 64) des gefüllten
// Original-Atlas, unter dem die Schicht den Ersatz sucht.
const fontAtlasHash = "56f7e53aa0773be5"

const (
	fontGrid  = 16  // Zellen je Zeile
	fontOCell = 8   // Kantenlänge einer Original-Zelle
	fontCellW = 64  // Breite einer Ersatz-Zelle
	fontCellH = 128 // Höhe einer Ersatz-Zelle (doppelte Höhe, siehe oben)
	fontYOff  = 32  // Zeilen über dem Bereich der Original-Zelle

	fontCap    = 6.3  // Versalhöhe inklusive Fettung, in Original-Pixeln
	fontBase   = 7.0  // Grundlinie, in Original-Pixeln
	fontBoldEm = 0.04 // synthetische Fettung je Seite, in em
	fontMargin = 2.0  // freier Rand in der Zelle
)

// fontCellMid ist die Mitte der Glyphe jeder Original-Zelle in Original-Pixeln
// (-1 = leere Zelle), abgelesen aus dem Atlas des Spiels. Zeilen 0-5 sind
// ASCII 32-127, Zeile 6 folgt CP437 (Ç ü é …), Zeilen 7 und 8 sind eine eigene
// Auswahl des Spiels.
var fontCellMid = [256]float64{
	-1, 4.5, 4.5, 3, 4.5, 4, 3.5, 3, 3.5, 4.5, 4, 4, 4.5, 4, 4.5, 4,
	4, 4, 4.5, 4.5, 4.5, 5, 4.5, 4, 4.5, 4, 4, 4, 4, 4.5, 4, 5,
	3.5, 3.5, 4.5, 4, 4, 4, 4, 3.5, 4, 3.5, 3, 3.5, 4, 4, 4, 3.5,
	4.5, 4, 4, 4, 4, 4, 3.5, 4, 3.5, 3.5, 4, 4, 4, 3.5, 4, 4,
	2.5, 4, 4, 4, 4, 4, 4, 4, 4, 4.5, 4.5, 4, 4.5, 4, 3.5, 4,
	4, 4, 3, 3.5, 4, 3.5, 4, 4, 3.5, 4, 3.5, 4, 3.5, 5, 4, 4,
	3.5, 4.5, 3, 3.5, 3.5, 3.5, 3.5, 3.5, 4, 4, 4, 3.5, 4, 4, 3.5, 3.5,
	3, 3.5, 3.5, 4, 4, -1, -1, 3.5, -1, 4, 4, 3.5, 3, 3, 3, -1,
	4, 4.5, -1, 3, 4, 4, -1, 3.5, 3, 3.5, 4, 3.5, 3, 3.5, 2.5, -1,
}

// fontCellRune liefert das Zeichen einer Zelle. Zelle 95 ist im Spiel ein
// Pfeil nach oben (0 = selbst gezeichnet, siehe drawArrow).
func fontCellRune(c int) rune {
	switch {
	case c < 95:
		return rune(c + 32)
	case c == 95:
		return 0
	}
	// Ab Zelle 96, je Zeile 16 Zeichen. ₧ (Zelle 126) kennt die Bahnschrift
	// nicht, dort steht ein P.
	ext := []rune("ÇüéâäàåçêëèïîìÄÅ" + "É¨Æôöòûÿ ÖÜ¢£¥Pƒ" + "¿¡ ßÑñ ÓóÁáÜûÚú")
	if i := c - 96; i >= 0 && i < len(ext) {
		return ext[i]
	}
	return ' '
}

// fontAtlasPath liefert den Pfad, unter dem die Schicht den Ersatz sucht.
func fontAtlasPath(gameDir string) string {
	return filepath.Join(gameDir, "textures", "font", fontAtlasHash+".dds")
}

// systemFontPath sucht eine Schrift im Windows-Schriftenordner.
func systemFontPath(name string) string {
	dir := os.Getenv("WINDIR")
	if dir == "" {
		dir = `C:\Windows`
	}
	return filepath.Join(dir, "Fonts", name)
}

// ensureFontAtlas erzeugt den Schrift-Ersatz, falls er noch fehlt. Ohne
// Bahnschrift (vor Windows 10) bleibt es bei der Original-Schrift.
func ensureFontAtlas(gameDir string) error {
	out := fontAtlasPath(gameDir)
	if _, err := os.Stat(out); err == nil {
		return nil
	}
	data, err := os.ReadFile(systemFontPath("bahnschrift.ttf"))
	if err != nil {
		return fmt.Errorf("Bahnschrift nicht gefunden: %w", err)
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return err
	}
	atlas := renderFontAtlas(f)
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	return os.WriteFile(out, encodeDDS(atlas, fontGrid*fontCellW, fontGrid*fontCellH), 0644)
}

// renderFontAtlas zeichnet alle belegten Zellen. Rückgabe: Deckung 0…1 je
// Pixel, zeilenweise, fontGrid*fontCellW breit.
func renderFontAtlas(f *opentype.Font) []float64 {
	k := float64(fontCellW) / fontOCell
	capH, _, _, _ := glyphExtent(f, 'H')
	size := fontCap * k / (capH/100 + 2*fontBoldEm)
	bold := fontBoldEm * size
	base := fontBase*k - bold
	capPx := capH * size / 100

	W := fontGrid * fontCellW
	atlas := make([]float64, W*fontGrid*fontCellH)
	var buf sfnt.Buffer
	for c := 0; c < fontGrid*fontGrid; c++ {
		if fontCellMid[c] < 0 {
			continue
		}
		mid := float64(fontCellW)/2 + (fontCellMid[c]-fontOCell/2)*k
		var g []float64
		if r := fontCellRune(c); r != 0 {
			if idx, err := f.GlyphIndex(&buf, r); err != nil || idx == 0 {
				r = '?'
			}
			g = renderGlyph(f, r, size, base, capPx, bold, mid)
		} else {
			g = drawArrow(base, capPx, bold)
		}
		cx, cy := c%fontGrid*fontCellW, c/fontGrid*fontCellH
		for y := 0; y < fontCellH; y++ {
			copy(atlas[(cy+y)*W+cx:(cy+y)*W+cx+fontCellW], g[y*fontCellW:(y+1)*fontCellW])
		}
	}
	return atlas
}

// rasterRune zeichnet r mit der Grundlinie bei (ox, oy) in eine s × s große Fläche.
func rasterRune(f *opentype.Font, r rune, size float64, s int, ox, oy float64) []float64 {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return make([]float64, s*s)
	}
	defer face.Close()
	dst := image.NewAlpha(image.Rect(0, 0, s, s))
	d := font.Drawer{Dst: dst, Src: image.Opaque, Face: face,
		Dot: fixed.Point26_6{X: fixed.Int26_6(ox * 64), Y: fixed.Int26_6(oy * 64)}}
	d.DrawString(string(r))
	out := make([]float64, s*s)
	for i := range out {
		out[i] = float64(dst.Pix[i]) / 255
	}
	return out
}

func inkBounds(a []float64, w, h int, thr float64) (x0, y0, x1, y1 int, ok bool) {
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

// glyphExtent misst eine Glyphe bei Größe 100: Höhe über und unter der
// Grundlinie sowie die Breite (Pixel).
func glyphExtent(f *opentype.Font, r rune) (asc, desc, wid float64, ok bool) {
	const s = 300
	raw := rasterRune(f, r, 100, s, 100, 200)
	x0, y0, x1, y1, ok := inkBounds(raw, s, s, 0.02)
	return float64(200 - y0), float64(y1 + 1 - 200), float64(x1 - x0 + 1), ok
}

// renderGlyph zeichnet eine Zelle: Glyphenmitte bei midX, Grundlinie bei base
// (beides im Bereich der Original-Zelle). Was trotz der hohen Zelle nicht
// passt, wird nur in seinem Überstand gestaucht; eine zu breite Glyphe wird
// als Ganzes verkleinert.
func renderGlyph(f *opentype.Font, r rune, size, base, capPx, bold, midX float64) []float64 {
	out := make([]float64, fontCellW*fontCellH)
	asc, desc, wid, ok := glyphExtent(f, r)
	if !ok {
		return out
	}
	base += fontYOff
	pad := bold + fontMargin
	sc := 1.0
	if w := wid * size / 100; w > fontCellW-2*pad {
		sc = (fontCellW - 2*pad) / w
	}
	k := size * sc / 100
	ascPx, descPx := asc*k, desc*k
	half := wid * k / 2
	midX = math.Max(pad+half, math.Min(fontCellW-pad-half, midX))
	s := 3 * fontCellH
	raw := rasterRune(f, r, size*sc, s, fontCellW, fontCellH+base)
	x0, _, x1, _, _ := inkBounds(raw, s, s, 0.02)
	dx := midX - float64(x0+x1+1)/2

	top, bot := pad, fontCellH-pad
	capLine := math.Max(top+1, base-capPx)
	src := func(y float64) float64 {
		if y < capLine && base-ascPx < top {
			return capLine - (capLine-y)*(ascPx-(base-capLine))/(capLine-top)
		}
		if y > base && base+descPx > bot {
			return base + (y-base)*descPx/(bot-base)
		}
		return y
	}
	sample := func(x, y float64) float64 {
		y += fontCellH
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
	const ss = 4
	for y := 0; y < fontCellH; y++ {
		for x := 0; x < fontCellW; x++ {
			v := 0.0
			for j := 0; j < ss; j++ {
				v += sample(float64(x)-dx, src(float64(y)+(float64(j)+0.5)/ss)-0.5)
			}
			out[y*fontCellW+x] = v / ss
		}
	}
	return dilate(out, fontCellW, fontCellH, bold)
}

// drawArrow zeichnet den Pfeil nach oben (Zelle 95), den die Bahnschrift nicht hat.
func drawArrow(base, capPx, bold float64) []float64 {
	base += fontYOff
	top := base - capPx + bold
	bot := base - bold
	mid := float64(fontCellW) / 2
	head := top + capPx*0.5
	z := vector.NewRasterizer(fontCellW, fontCellH)
	z.MoveTo(float32(mid), float32(top))
	z.LineTo(float32(mid+22), float32(head))
	z.LineTo(float32(mid+8), float32(head))
	z.LineTo(float32(mid+8), float32(bot))
	z.LineTo(float32(mid-8), float32(bot))
	z.LineTo(float32(mid-8), float32(head))
	z.LineTo(float32(mid-22), float32(head))
	z.ClosePath()
	dst := image.NewAlpha(image.Rect(0, 0, fontCellW, fontCellH))
	z.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	out := make([]float64, fontCellW*fontCellH)
	for i := range out {
		out[i] = float64(dst.Pix[i]) / 255
	}
	return out
}

// dilate verbreitert die Deckung um rad Pixel mit rundem, weichem Pinsel
// (synthetische Fettung - die Bahnschrift-Datei enthält nur den Normalschnitt).
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

// encodeDDS schreibt weiße BGRA-Pixel mit der Deckung im Alphakanal und einer
// Mipmap-Kette (unkomprimiert, 32 Bit - so liest sie die Schicht).
func encodeDDS(a []float64, W, H int) []byte {
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
	return out
}
