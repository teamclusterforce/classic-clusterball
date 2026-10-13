// Geglaettete Symbole fuer den Reiter "Steuerung": Tastenkappe und Maus.
// GDI zeichnet Rundungen ohne Kantenglaettung - bei 24 Pixeln hohen Symbolen
// sieht das treppig aus. Deshalb werden sie hier mit x/image/vector in der
// Pixelgroesse des Bildschirms gerastert und als Bitmap gezeichnet.

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/lxn/walk"
	"golang.org/x/image/vector"
)

type glyphCache struct{ m map[string]*walk.Bitmap }

func newGlyphCache() *glyphCache { return &glyphCache{m: map[string]*walk.Bitmap{}} }

func nrgba(c walk.Color) color.NRGBA {
	return color.NRGBA{c.R(), c.G(), c.B(), 255}
}

// roundRect legt einen Pfad mit runden Ecken (Radius r) in z.
func roundRect(z *vector.Rasterizer, x0, y0, x1, y1, r float32) {
	k := float32(0.5523) * r // Kreisbogen als kubische Bezierkurve
	z.MoveTo(x0+r, y0)
	z.LineTo(x1-r, y0)
	z.CubeTo(x1-r+k, y0, x1, y0+r-k, x1, y0+r)
	z.LineTo(x1, y1-r)
	z.CubeTo(x1, y1-r+k, x1-r+k, y1, x1-r, y1)
	z.LineTo(x0+r, y1)
	z.CubeTo(x0+r-k, y1, x0, y1-r+k, x0, y1-r)
	z.LineTo(x0, y0+r)
	z.CubeTo(x0, y0+r-k, x0+r-k, y0, x0+r, y0)
	z.ClosePath()
}

// fill rastert eine Form (shape legt den Pfad) in Farbe c auf dst.
func fill(dst *image.RGBA, c color.NRGBA, shape func(z *vector.Rasterizer)) {
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	shape(z)
	z.Draw(dst, b, image.NewUniform(c), image.Point{})
}

func (g *glyphCache) bitmap(key string, img image.Image) *walk.Bitmap {
	if bmp, ok := g.m[key]; ok {
		return bmp
	}
	// 96 DPI: eine Bitmap-Einheit ist ein Pixel, gezeichnet wird in Pixeln.
	bmp, err := walk.NewBitmapFromImageForDPI(img, 96)
	if err != nil {
		return nil
	}
	g.m[key] = bmp
	return bmp
}

// keycap: Tastenkappe w x h Pixel mit Schattenkante unten; active = gelb
// (wartet auf eine Taste).
func (g *glyphCache) keycap(w, h, dpi int, active bool) *walk.Bitmap {
	key := fmt.Sprintf("cap %d %d %d %v", w, h, dpi, active)
	if bmp, ok := g.m[key]; ok {
		return bmp
	}
	s := float32(dpi) / 96
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(nrgba(colCard)), image.Point{}, draw.Src)
	edge, face := walk.RGB(168, 177, 192), walk.RGB(255, 255, 255)
	if active {
		edge, face = colAccentDown, walk.RGB(255, 246, 214)
	}
	fw, fh, r := float32(w), float32(h), 6*s
	// Kante (inkl. Schatten unten), darauf die Flaeche.
	fill(img, nrgba(edge), func(z *vector.Rasterizer) { roundRect(z, 0.5, 0.5, fw-0.5, fh-0.5, r) })
	fill(img, nrgba(face), func(z *vector.Rasterizer) { roundRect(z, 0.5+s, 0.5+s, fw-0.5-s, fh-0.5-3*s, r-s) })
	return g.bitmap(key, img)
}

// mouse: Maus 16 x 24 (logisch) mit gelb markierter Taste (part 1 links,
// 2 rechts, 3 Mitte, 4 Seitentaste) bzw. dem Rad und einem Pfeil daneben
// (kind capWheel, part +1 hoch / -1 runter).
func (g *glyphCache) mouse(kind, part, dpi int) *walk.Bitmap {
	key := fmt.Sprintf("mouse %d %d %d", kind, part, dpi)
	if bmp, ok := g.m[key]; ok {
		return bmp
	}
	s := float32(dpi) / 96
	mw, mh := 16*s, 24*s
	w := int(math.Ceil(float64(mw + 2*s)))
	if kind == capWheel {
		w = int(math.Ceil(float64(mw + 11*s)))
	}
	h := int(math.Ceil(float64(mh + 1)))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(nrgba(colCard)), image.Point{}, draw.Src)
	ink, accent, white := nrgba(walk.RGB(60, 70, 90)), nrgba(colAccent), color.NRGBA{255, 255, 255, 255}
	x0, y0 := float32(0.5), float32(0.5)
	x1, y1 := x0+mw, y0+mh
	r := mw / 2
	lw := 1.4 * s
	split := y0 + mh*0.42 // Unterkante der Tasten
	mid := (x0 + x1) / 2

	fill(img, ink, func(z *vector.Rasterizer) { roundRect(z, x0, y0, x1, y1, r) })
	fill(img, white, func(z *vector.Rasterizer) { roundRect(z, x0+lw, y0+lw, x1-lw, y1-lw, r-lw) })
	// Belegte Taste: die obere Haelfte links/rechts, an der Rundung beschnitten.
	if kind == capMouse && (part == 1 || part == 2) {
		tmp := image.NewRGBA(img.Bounds())
		fill(tmp, accent, func(z *vector.Rasterizer) { roundRect(z, x0+lw, y0+lw, x1-lw, y1-lw, r-lw) })
		left, right := x0, mid
		if part == 2 {
			left, right = mid, x1
		}
		clip := image.Rect(int(left), 0, int(math.Ceil(float64(right))), int(split))
		draw.Draw(img, clip, tmp, clip.Min, draw.Over)
	}
	// Trennlinien: waagerecht unter den Tasten, senkrecht zwischen ihnen.
	fill(img, ink, func(z *vector.Rasterizer) {
		z.MoveTo(x0+lw, split-lw/2)
		z.LineTo(x1-lw, split-lw/2)
		z.LineTo(x1-lw, split+lw/2)
		z.LineTo(x0+lw, split+lw/2)
		z.ClosePath()
		z.MoveTo(mid-lw/2, y0+lw)
		z.LineTo(mid+lw/2, y0+lw)
		z.LineTo(mid+lw/2, split)
		z.LineTo(mid-lw/2, split)
		z.ClosePath()
	})
	// Rad bzw. mittlere Taste.
	if kind == capWheel || (kind == capMouse && part == 3) {
		wx, wy := mid-2.2*s, y0+mh*0.12
		fill(img, ink, func(z *vector.Rasterizer) { roundRect(z, wx-0.8*s, wy-0.8*s, wx+5.2*s, wy+8.8*s, 3*s) })
		fill(img, accent, func(z *vector.Rasterizer) { roundRect(z, wx, wy, wx+4.4*s, wy+8*s, 2.2*s) })
	}
	if kind == capMouse && part == 4 {
		fill(img, accent, func(z *vector.Rasterizer) { roundRect(z, x0, split+3*s, x0+3*s, split+9*s, 1.2*s) })
	}
	// Pfeil fuer die Drehrichtung des Rads.
	if kind == capWheel {
		ax, ay := x1+5.5*s, y0+mh*0.3
		d := float32(part) // +1 hoch, -1 runter
		fill(img, ink, func(z *vector.Rasterizer) {
			z.MoveTo(ax, ay-5*s*d)
			z.LineTo(ax+4*s, ay)
			z.LineTo(ax+1.3*s, ay)
			z.LineTo(ax+1.3*s, ay+5*s*d)
			z.LineTo(ax-1.3*s, ay+5*s*d)
			z.LineTo(ax-1.3*s, ay)
			z.LineTo(ax-4*s, ay)
			z.ClosePath()
		})
	}
	return g.bitmap(key, img)
}

// arrow: Pfeil fuer die Pfeiltasten (dir 0 hoch, 1 rechts, 2 runter, 3 links),
// 14 x 14 logisch. Als Schrift bekommt der duenne Schaft ClearType-Farbsaeume.
func (g *glyphCache) arrow(dir, dpi int, bg walk.Color) *walk.Bitmap {
	key := fmt.Sprintf("arrow %d %d %d", dir, dpi, bg)
	if bmp, ok := g.m[key]; ok {
		return bmp
	}
	s := float32(dpi) / 96
	n := int(math.Ceil(float64(14 * s)))
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	draw.Draw(img, img.Bounds(), image.NewUniform(nrgba(bg)), image.Point{}, draw.Src)
	c := float32(n) / 2
	// Pfeil nach oben um die Mitte, fuer die anderen Richtungen gedreht.
	pts := [][2]float32{{0, -6}, {5.5, -0.5}, {2, -0.5}, {2, 6}, {-2, 6}, {-2, -0.5}, {-5.5, -0.5}}
	rot := func(p [2]float32) (float32, float32) {
		x, y := p[0]*s, p[1]*s
		for i := 0; i < dir; i++ {
			x, y = -y, x
		}
		return c + x, c + y
	}
	fill(img, nrgba(colHelpTerm), func(z *vector.Rasterizer) {
		z.MoveTo(rot(pts[0]))
		for _, p := range pts[1:] {
			z.LineTo(rot(p))
		}
		z.ClosePath()
	})
	return g.bitmap(key, img)
}

// pill: Knopf w x h Pixel mit runden Enden, Flaeche face, Rand edge, auf bg.
func (g *glyphCache) pill(w, h, dpi int, face, edge, bg walk.Color) *walk.Bitmap {
	key := fmt.Sprintf("pill %d %d %d %d %d %d", w, h, dpi, face, edge, bg)
	if bmp, ok := g.m[key]; ok {
		return bmp
	}
	s := float32(dpi) / 96
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(nrgba(bg)), image.Point{}, draw.Src)
	fw, fh := float32(w), float32(h)
	r := fh / 2
	fill(img, nrgba(edge), func(z *vector.Rasterizer) { roundRect(z, 0.5, 0.5, fw-0.5, fh-0.5, r-0.5) })
	fill(img, nrgba(face), func(z *vector.Rasterizer) { roundRect(z, 0.5+s, 0.5+s, fw-0.5-s, fh-0.5-s, r-0.5-s) })
	return g.bitmap(key, img)
}
