// Gleiter im Spiel-Reiter: das Schiff des Spiels (shipLOD358 aus Data.mrg mit
// der Skin Skins\Default\Blue.jpg), langsam drehend. Beides wird zur Laufzeit
// aus dem Spielordner gelesen - Modell und Skin bringt der Launcher nicht mit.
// Fehlt etwas, bleibt die Stelle leer.
//
// Gezeichnet wird mit einem kleinen Software-Rasterer: perspektivisch,
// Tiefenpuffer, Textur bilinear und perspektivisch korrekt, Licht von schraeg
// oben, 2 x 2 Abtastungen je Pixel fuer glatte Kanten. Bei 190 x 130 Pixeln
// sind das gut 1 ms je Bild; gezeichnet wird nur, solange der Spiel-Reiter
// zu sehen ist.

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// mrgEntry sucht eine Datei in einem Clusterball-Archiv (Data.mrg): Gruppen
// mit Name und Eintraegen (id, Offset, Laenge, Name), alles big-endian. Die
// Namen tragen die Nummer vorn ("31.shipLOD358.3GM").
func mrgEntry(b []byte, file string) ([]byte, error) {
	be := binary.BigEndian
	if len(b) < 8 {
		return nil, errors.New("mrg: zu kurz")
	}
	cstr := func(p int) (string, int) {
		s := p
		for p < len(b) && b[p] != 0 {
			p++
		}
		return string(b[s:p]), p + 1
	}
	want := strings.ToLower(file)
	nf := int(be.Uint32(b[4:]))
	p := 8
	for i := 0; i < nf && p+4 <= len(b); i++ {
		bl := int(be.Uint32(b[p:]))
		_, q := cstr(p + 4)
		if q+4 > len(b) {
			break
		}
		n := int(be.Uint32(b[q:]))
		q += 4
		for j := 0; j < n && q+12 <= len(b); j++ {
			off, ln := int(be.Uint32(b[q+4:])), int(be.Uint32(b[q+8:]))
			name, nq := cstr(q + 12)
			name = strings.ToLower(name)
			if (name == want || strings.HasSuffix(name, "."+want)) && off >= 0 && off+ln <= len(b) {
				return b[off : off+ln], nil
			}
			q = nq
		}
		if bl <= 0 {
			break
		}
		p += bl
	}
	return nil, errors.New("mrg: " + file + " nicht gefunden")
}

type gliderTri struct {
	v  [3]int
	uv [3][2]float64
}

type gliderMesh struct {
	verts      [][3]float64
	tris       []gliderTri
	tex        []uint8 // RGB, texW x texH
	texW, texH int

	// Zeichenpuffer, von Bild zu Bild wiederverwendet
	zbuf []float32
	col  []uint8
	bg   []uint8 // Hintergrund mit Bodenschatten, fuer die aktuelle Groesse
	bgW  int
	bgH  int
	bgC  [3]uint8
}

// decodeShip liest eine .3GM-Form (Formatbeschreibung: cbport/gm.go im
// ClusterForce-Repo). Zwei Fassungen: "Dot2" mit 16-Bit-Feldern und
// int32-Eckpunkten, und "3DGM" (so das Schiff) mit 32-Bit-Feldern,
// float32-Eckpunkten ("FDot") und Polygonen im "Prim"-Block. In beiden stehen
// die UVs in 1/256 Kachel, getrennt als u- und v-Block, dann zwei Eckenlisten.
func decodeShip(b []byte) (*gliderMesh, error) {
	be := binary.BigEndian
	wide := len(b) >= 4 && string(b[0:4]) == "3DGM"
	fs := 2 // Feldbreite
	if wide {
		fs = 4
	}
	field := func(o int) int {
		if o < 0 || o+fs > len(b) {
			return -1
		}
		if wide {
			return int(be.Uint32(b[o:]))
		}
		return int(be.Uint16(b[o:]))
	}
	signed := func(o int) int {
		if wide {
			return int(int32(be.Uint32(b[o:])))
		}
		return int(int16(be.Uint16(b[o:])))
	}
	m := &gliderMesh{}
	floats := false
	vb := bytes.Index(b, []byte("FDot"))
	if vb >= 0 {
		floats = true
	} else if vb = bytes.Index(b, []byte("Dot2")); vb < 0 {
		return nil, errors.New("3gm: keine Eckpunkte")
	}
	size := int(be.Uint32(b[vb+4:]))
	if !floats {
		size -= 4 // bei Dot2 zaehlt die Groesse sich selbst mit
	}
	n := size / 12
	if n <= 0 || vb+8+n*12 > len(b) {
		return nil, errors.New("3gm: Eckpunkte passen nicht")
	}
	for i := 0; i < n; i++ {
		p := vb + 8 + i*12
		var v [3]float64
		for a := 0; a < 3; a++ {
			w := be.Uint32(b[p+4*a:])
			if floats {
				v[a] = float64(math.Float32frombits(w))
			} else {
				v[a] = float64(int32(w))
			}
		}
		m.verts = append(m.verts, v)
	}
	tag, endList, endRec := "Line", 0x6000, 0x7000
	if wide {
		tag, endList, endRec = "Prim", 0xfffffffe, 0xffffffff
	}
	pb := bytes.Index(b, []byte(tag))
	if pb < 0 {
		return nil, errors.New("3gm: keine Polygone")
	}
	head := 4 * fs // Flags, Ecken, Schattierung, Textur
	for p := pb + 8; field(p) != endList; {
		if p < 0 || p+head > len(b) {
			return nil, errors.New("3gm: Polygonliste laeuft ueber das Ende")
		}
		flags, k := field(p), field(p+fs)&0xffff
		extra := 0
		if flags&2 != 0 {
			extra = 1
		}
		q := p + head + fs*(4*k+extra)
		if field(q) != endRec {
			return nil, errors.New("3gm: Polygon endet nicht wie erwartet")
		}
		d := p + head + fs*extra
		uv := make([][2]float64, k)
		for c := 0; c < k; c++ {
			uv[c] = [2]float64{float64(signed(d+fs*c)) / 256, float64(signed(d+fs*(k+c))) / 256}
		}
		d += 2 * fs * k
		// erste Eckenliste, sonst die zweite (manche Formen markieren die erste ungueltig)
		var idx []int
		for l := 0; l < 2 && idx == nil; l++ {
			cand := make([]int, k)
			ok := k >= 3
			for c := 0; c < k; c++ {
				cand[c] = field(d + fs*(l*k+c))
				if cand[c] < 0 || cand[c] >= n {
					ok = false
				}
			}
			if ok {
				idx = cand
			}
		}
		for c := 1; idx != nil && c+1 < k; c++ { // Faecher aus Dreiecken
			m.tris = append(m.tris, gliderTri{v: [3]int{idx[0], idx[c], idx[c+1]}, uv: [3][2]float64{uv[0], uv[c], uv[c+1]}})
		}
		p = q + fs
	}
	if len(m.tris) == 0 {
		return nil, errors.New("3gm: keine Dreiecke")
	}
	m.center()
	return m, nil
}

// center verschiebt das Modell in den Ursprung und normiert es auf Radius 1.
func (m *gliderMesh) center() {
	var lo, hi [3]float64
	for i, v := range m.verts {
		for a := 0; a < 3; a++ {
			if i == 0 || v[a] < lo[a] {
				lo[a] = v[a]
			}
			if i == 0 || v[a] > hi[a] {
				hi[a] = v[a]
			}
		}
	}
	r := 0.0
	for i := range m.verts {
		for a := 0; a < 3; a++ {
			m.verts[i][a] -= (lo[a] + hi[a]) / 2
		}
		v := m.verts[i]
		r = math.Max(r, math.Sqrt(v[0]*v[0]+v[1]*v[1]+v[2]*v[2]))
	}
	for i := range m.verts {
		for a := 0; a < 3; a++ {
			m.verts[i][a] /= r
		}
	}
}

// setTexture uebernimmt die Skin als RGB-Feld.
func (m *gliderMesh) setTexture(img image.Image) {
	b := img.Bounds()
	m.texW, m.texH = b.Dx(), b.Dy()
	m.tex = make([]uint8, 3*m.texW*m.texH)
	for y := 0; y < m.texH; y++ {
		for x := 0; x < m.texW; x++ {
			r, g, bb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := 3 * (y*m.texW + x)
			m.tex[i], m.tex[i+1], m.tex[i+2] = uint8(r>>8), uint8(g>>8), uint8(bb>>8)
		}
	}
}

// loadGlider liest Schiff und Skin aus dem Spielordner.
func loadGlider(gameDir string) (*gliderMesh, error) {
	data, err := os.ReadFile(filepath.Join(gameDir, "Data.mrg"))
	if err != nil {
		return nil, err
	}
	shape, err := mrgEntry(data, "shipLOD358.3GM")
	if err != nil {
		return nil, err
	}
	m, err := decodeShip(shape)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(gameDir, "Skins", "Default", "Blue.jpg"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	m.setTexture(img)
	return m, nil
}

// background legt den Hintergrund (Farbe bg, darunter ein weicher Schatten
// auf dem "Boden") fuer w x h Abtastungen an - einmal je Groesse.
func (m *gliderMesh) background(w, h int, bg [3]uint8) {
	if m.bgW == w && m.bgH == h && m.bgC == bg && m.bg != nil {
		return
	}
	m.bgW, m.bgH, m.bgC = w, h, bg
	m.bg = make([]uint8, 3*w*h)
	cx, cy := float64(w)/2, float64(h)*0.80
	rx, ry := float64(w)*0.36, float64(h)*0.07
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			d := dx*dx + dy*dy
			s := 0.16 * math.Exp(-d*1.6) // weicher Rand
			i := 3 * (y*w + x)
			for c := 0; c < 3; c++ {
				m.bg[i+c] = uint8(float64(bg[c]) * (1 - s))
			}
		}
	}
}

// render zeichnet den Gleiter, um angle (Bogenmass) um die Hochachse gedreht,
// als BGRA-Pixel (oben beginnend) in dst; dst hat w*h*4 Bytes.
func (m *gliderMesh) render(dst []uint8, w, h int, bg [3]uint8, angle float64) {
	const ss = 2
	sw, sh := w*ss, h*ss
	if len(m.zbuf) != sw*sh {
		m.zbuf = make([]float32, sw*sh)
		m.col = make([]uint8, 3*sw*sh)
	}
	m.background(sw, sh, bg)
	copy(m.col, m.bg)
	for i := range m.zbuf {
		m.zbuf[i] = 0
	}

	pitch := 22 * math.Pi / 180 // Kamera leicht von oben
	sa, ca := math.Sin(angle), math.Cos(angle)
	sp, cp := math.Sin(pitch), math.Cos(pitch)
	const dist = 3.0
	// Brennweite so, dass der Gleiter in jeder Drehlage mit Rand hineinpasst
	// (Radius 1, naechster Punkt in Tiefe dist-1).
	focal := math.Min(float64(sw)*0.5, float64(sh)*1.0) * (dist - 1)
	cy := float64(sh) * 0.46

	n := len(m.verts)
	type pv struct{ x, y, iz, vx, vy, vz float64 }
	pts := make([]pv, n)
	for i, v := range m.verts {
		// Modell: x Spannweite, y Laenge, z hoch
		x, y := v[0]*ca-v[1]*sa, v[0]*sa+v[1]*ca
		vy := v[2]*cp + y*sp
		vz := y*cp - v[2]*sp + dist
		pts[i] = pv{float64(sw)/2 + x*focal/vz, cy - vy*focal/vz, 1 / vz, x, vy, vz}
	}
	lx, ly, lz := -0.45, 0.7, -0.55
	ll := math.Sqrt(lx*lx + ly*ly + lz*lz)
	lx, ly, lz = lx/ll, ly/ll, lz/ll

	tw, th := float64(m.texW), float64(m.texH)
	for _, t := range m.tris {
		a, b, c := pts[t.v[0]], pts[t.v[1]], pts[t.v[2]]
		e1x, e1y, e1z := b.vx-a.vx, b.vy-a.vy, b.vz-a.vz
		e2x, e2y, e2z := c.vx-a.vx, c.vy-a.vy, c.vz-a.vz
		nx, ny, nz := e1y*e2z-e1z*e2y, e1z*e2x-e1x*e2z, e1x*e2y-e1y*e2x
		nl := math.Sqrt(nx*nx + ny*ny + nz*nz)
		if nl == 0 {
			continue
		}
		nx, ny, nz = nx/nl, ny/nl, nz/nl
		if nx*a.vx+ny*a.vy+nz*a.vz > 0 { // zweiseitig: zur Kamera drehen
			nx, ny, nz = -nx, -ny, -nz
		}
		shade := 0.5 + 0.65*math.Max(0, nx*lx+ny*ly+nz*lz)
		area := (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
		if area == 0 {
			continue
		}
		minx := int(math.Max(0, math.Floor(math.Min(a.x, math.Min(b.x, c.x)))))
		maxx := int(math.Min(float64(sw-1), math.Ceil(math.Max(a.x, math.Max(b.x, c.x)))))
		miny := int(math.Max(0, math.Floor(math.Min(a.y, math.Min(b.y, c.y)))))
		maxy := int(math.Min(float64(sh-1), math.Ceil(math.Max(a.y, math.Max(b.y, c.y)))))
		// u/z und v/z je Ecke fuer perspektivisch korrekte Texturkoordinaten
		ua, va := t.uv[0][0]*a.iz, t.uv[0][1]*a.iz
		ub, vb := t.uv[1][0]*b.iz, t.uv[1][1]*b.iz
		uc, vc := t.uv[2][0]*c.iz, t.uv[2][1]*c.iz
		for y := miny; y <= maxy; y++ {
			py := float64(y) + 0.5
			for x := minx; x <= maxx; x++ {
				px := float64(x) + 0.5
				w0 := ((b.x-px)*(c.y-py) - (b.y-py)*(c.x-px)) / area
				w1 := ((c.x-px)*(a.y-py) - (c.y-py)*(a.x-px)) / area
				w2 := 1 - w0 - w1
				if w0 < 0 || w1 < 0 || w2 < 0 {
					continue
				}
				iz := w0*a.iz + w1*b.iz + w2*c.iz
				k := y*sw + x
				if float32(iz) <= m.zbuf[k] {
					continue
				}
				m.zbuf[k] = float32(iz)
				u := (w0*ua + w1*ub + w2*uc) / iz
				v := (w0*va + w1*vb + w2*vc) / iz
				// bilinear, wiederholt
				fxp := (u-math.Floor(u))*tw - 0.5
				fyp := (v-math.Floor(v))*th - 0.5
				x0, y0 := int(math.Floor(fxp)), int(math.Floor(fyp))
				fx, fy := fxp-float64(x0), fyp-float64(y0)
				xa, xb := (x0+m.texW)%m.texW, (x0+1+m.texW)%m.texW
				ya, yb := (y0+m.texH)%m.texH, (y0+1+m.texH)%m.texH
				for ch := 0; ch < 3; ch++ {
					p00 := float64(m.tex[3*(ya*m.texW+xa)+ch])
					p10 := float64(m.tex[3*(ya*m.texW+xb)+ch])
					p01 := float64(m.tex[3*(yb*m.texW+xa)+ch])
					p11 := float64(m.tex[3*(yb*m.texW+xb)+ch])
					val := ((p00*(1-fx)+p10*fx)*(1-fy) + (p01*(1-fx)+p11*fx)*fy) * shade
					if val > 255 {
						val = 255
					}
					m.col[3*k+ch] = uint8(val)
				}
			}
		}
	}
	// 2 x 2 Abtastungen zu einem Pixel, als BGRA
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s [3]int
			for j := 0; j < ss; j++ {
				for i := 0; i < ss; i++ {
					k := 3 * ((y*ss+j)*sw + x*ss + i)
					s[0] += int(m.col[k])
					s[1] += int(m.col[k+1])
					s[2] += int(m.col[k+2])
				}
			}
			o := 4 * (y*w + x)
			dst[o], dst[o+1], dst[o+2], dst[o+3] = uint8(s[2]/4), uint8(s[1]/4), uint8(s[0]/4), 255
		}
	}
}

// ── Anzeige ──────────────────────────────────────────────────────────────────

const gliderTurn = 16.0 // Sekunden je Umdrehung

var procSetDIBitsToDevice = syscall.NewLazyDLL("gdi32.dll").NewProc("SetDIBitsToDevice")

type gliderView struct {
	mw    *AppWindow
	w     *walk.CustomWidget
	mesh  *gliderMesh // nil, bis geladen (oder wenn das Spiel fehlt)
	buf   []uint8
	start time.Time
}

func (g *gliderView) widget() Widget {
	return CustomWidget{
		AssignTo:            &g.w,
		MinSize:             Size{Width: 190, Height: 80}, // darf schrumpfen, das Fenster waechst nicht
		MaxSize:             Size{Width: 190, Height: 150},
		PaintMode:           PaintBuffered, // DoubleBuffering wertet walk nicht aus; ohne das wird vor jedem Zeichnen geloescht (Flackern)
		InvalidatesOnResize: true,
		Paint:               func(c *walk.Canvas, _ walk.Rectangle) error { return g.paint(c) },
	}
}

func (g *gliderView) paint(c *walk.Canvas) error {
	b := g.w.ClientBoundsPixels()
	if g.mesh == nil || b.Width <= 0 || b.Height <= 0 {
		return c.FillRectanglePixels(brushOf(colPanel()), b)
	}
	w, h := b.Width, b.Height
	if len(g.buf) != 4*w*h {
		g.buf = make([]uint8, 4*w*h)
	}
	p := colPanel()
	angle := 2 * math.Pi * math.Mod(time.Since(g.start).Seconds()/gliderTurn, 1)
	g.mesh.render(g.buf, w, h, [3]uint8{p.R(), p.G(), p.B()}, angle)
	// BITMAPINFOHEADER, negative Hoehe = Zeilen von oben nach unten
	var bi struct {
		size                 uint32
		width, height        int32
		planes, bitCount     uint16
		compression, sizeImg uint32
		xppm, yppm           int32
		clrUsed, clrImp      uint32
	}
	bi.size = uint32(unsafe.Sizeof(bi))
	bi.width, bi.height = int32(w), -int32(h)
	bi.planes, bi.bitCount = 1, 32
	procSetDIBitsToDevice.Call(uintptr(c.HDC()), uintptr(b.X), uintptr(b.Y), uintptr(w), uintptr(h), 0, 0, 0,
		uintptr(h), uintptr(unsafe.Pointer(&g.buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	return nil
}

// run laedt das Schiff im Hintergrund und dreht es dann, solange das Fenster
// lebt - neu gezeichnet wird nur bei sichtbarem Spiel-Reiter.
func (g *gliderView) run(gameDir string) {
	go func() {
		m, err := loadGlider(gameDir)
		if err != nil {
			return
		}
		g.mw.sync(func() {
			g.mesh = m
			g.start = time.Now()
			g.w.Invalidate()
		})
		t := time.NewTicker(time.Second / 12)
		defer t.Stop()
		for {
			select {
			case <-g.mw.done:
				return
			case <-t.C:
				g.mw.sync(func() {
					if g.mw.activeTab == tabSpiel && !win.IsIconic(g.mw.Handle()) {
						g.w.Invalidate()
					}
				})
			}
		}
	}()
}
