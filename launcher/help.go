// Hilfe-Reiter: der spielrelevante Teil des Handbuchs von clusterball.de,
// direkt im Launcher statt als Link auf die (langsame) Webseite. Die Texte
// liegen je Sprache in help_de.txt und help_en.txt, links stehen die Kapitel,
// rechts wird das gewaehlte Kapitel selbst gezeichnet und gescrollt.

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strconv"
	"strings"
	"syscall"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	xdraw "golang.org/x/image/draw"
)

//go:embed help_de.txt
var helpDE string

//go:embed help_en.txt
var helpEN string

// Bildschirmuebersicht aus dem Handbuch (608x478).
//
//go:embed help_interface.jpg
var helpInterfaceJPG []byte

//go:embed about_de.txt
var aboutDE string

//go:embed about_en.txt
var aboutEN string

// Bilder des Reiters "Über": je eine Szene aus Clusterball Re-Enhanced (docs/
// flight-antarctica.jpg) und aus ClusterForce (clusterforce-website/assets/
// shots/ocean_chain.webp), verkleinert; dazu die Logos der Kopfleiste.
//
//go:embed about_shot_cb.jpg
var aboutShotCBJPG []byte

//go:embed about_shot.jpg
var aboutShotJPG []byte

var helpImages = map[string][]byte{
	"help_interface.jpg": helpInterfaceJPG,
	"about_shot_cb.jpg":  aboutShotCBJPG,
	"logo_cb.png":        logoPNG,
	"logo_cf.png":        logoCFPNG,
	"about_shot.jpg":     aboutShotJPG,
}

const (
	hbPara = iota
	hbSub
	hbBullet
	hbTerm
	hbImage
	hbLink
	hbHero    // Titelkarte: Screenshot, Logo darauf, Zeile rechts unten
	hbButtons // Reihe von Knoepfen mit Links
)

type helpBlock struct {
	kind  int
	term  string // hbTerm: Begriff links
	text  string // Absatz, Ueberschrift, Beschreibung, Linktext, Bildname
	url   string // hbLink
	ball  byte   // hbTerm: 'r', 'b', 'y' fuer einen farbigen Ball vor dem Begriff
	width int    // hbImage: hoechstens so breit (logische Pixel), 0 = Seitenbreite
}

type helpChapter struct {
	title  string
	blocks []helpBlock
}

// parseHelp liest das Zeilenformat aus dem Kopf von help_de.txt.
func parseHelp(src string) []helpChapter {
	var out []helpChapter
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "##") {
			continue
		}
		if strings.HasPrefix(line, "= ") {
			out = append(out, helpChapter{title: strings.TrimSpace(line[2:])})
			continue
		}
		if len(out) == 0 {
			continue
		}
		b := helpBlock{kind: hbPara, text: line}
		switch {
		case strings.HasPrefix(line, "## "):
			b = helpBlock{kind: hbSub, text: strings.TrimSpace(line[3:])}
		case strings.HasPrefix(line, "- "):
			b = helpBlock{kind: hbBullet, text: strings.TrimSpace(line[2:])}
		case strings.HasPrefix(line, "[img ") && strings.HasSuffix(line, "]"):
			n, w, _ := strings.Cut(line[5:len(line)-1], "|")
			b = helpBlock{kind: hbImage, text: strings.TrimSpace(n)}
			b.width, _ = strconv.Atoi(strings.TrimSpace(w))
		case strings.HasPrefix(line, "[hero ") && strings.HasSuffix(line, "]"):
			// [hero bild|logo|fokus|zeile]: fokus 0-100 = welcher Streifen des Bildes (oben-unten)
			f := strings.SplitN(line[6:len(line)-1], "|", 4)
			for len(f) < 4 {
				f = append(f, "")
			}
			b = helpBlock{kind: hbHero, text: strings.TrimSpace(f[0]), term: strings.TrimSpace(f[1]), url: strings.TrimSpace(f[3])}
			b.width, _ = strconv.Atoi(strings.TrimSpace(f[2]))
		case strings.HasPrefix(line, "[buttons ") && strings.HasSuffix(line, "]"):
			// [buttons Text|URL|Text|URL ...]: der erste Knopf gelb, die anderen hell
			b = helpBlock{kind: hbButtons, text: line[9 : len(line)-1]}
		case strings.HasPrefix(line, "[link ") && strings.HasSuffix(line, "]"):
			t, u, _ := strings.Cut(line[6:len(line)-1], "|")
			b = helpBlock{kind: hbLink, text: strings.TrimSpace(t), url: strings.TrimSpace(u)}
		case strings.Contains(line, " :: "):
			t, d, _ := strings.Cut(line, " :: ")
			b = helpBlock{kind: hbTerm, term: strings.TrimSpace(t), text: strings.TrimSpace(d)}
			if len(b.term) > 3 && b.term[0] == '@' && b.term[2] == ' ' {
				b.ball, b.term = b.term[1], strings.TrimSpace(b.term[3:])
			}
		}
		c := &out[len(out)-1]
		c.blocks = append(c.blocks, b)
	}
	return out
}

// chapters liefert die Kapitel in der aktuellen Sprache.
func (h *helpView) chapters() []helpChapter {
	if c, ok := h.parsed[currentLang]; ok {
		return c
	}
	src := h.de
	if currentLang == "en" {
		src = h.en
	}
	c := parseHelp(src)
	h.parsed[currentLang] = c
	return c
}

// Farben der Hilfe.
var (
	colHelpText  = walk.RGB(40, 46, 58)
	colHelpTerm  = walk.RGB(18, 26, 40)
	colHelpSub   = walk.RGB(28, 39, 58)
	colHelpLink  = walk.RGB(0, 90, 180)
	colHelpTrack = walk.RGB(236, 239, 244)
	colHelpThumb = walk.RGB(178, 186, 200)
	colNavText   = walk.RGB(70, 80, 100)
	helpBallCols = map[byte]walk.Color{'r': walk.RGB(220, 40, 40), 'b': walk.RGB(40, 90, 220), 'y': walk.RGB(245, 190, 0)}
)

// helpItem ist ein fertig platziertes Stueck der Seite (in Pixeln, ohne Scroll).
type helpItem struct {
	r     walk.Rectangle
	text  string
	font  *walk.Font
	color walk.Color
	ball  walk.Color // != 0: Kreis statt Text
	img   *walk.Bitmap
	url   string
	fill  bool                // r ist eine Flaeche in color (Akzentlinie)
	align walk.DrawTextFormat // Text: Ausrichtung statt Umbruch (0 = umbrechen)
}

// helpView zeigt Seiten im Format von help_de.txt: die Hilfe mit
// Kapitelleiste, "Über" als einzelne Seite ohne Leiste.
type helpView struct {
	mw       *AppWindow
	de, en   string // Quelltexte je Sprache
	withNav  bool
	parsed   map[string][]helpChapter
	nav      *walk.CustomWidget // nil ohne Kapitelleiste
	page     *walk.CustomWidget
	chapter  int
	scroll   int // Pixel
	items    []helpItem
	height   int // Gesamthoehe der Seite in Pixeln
	layoutAt struct {
		width   int
		chapter int
		lang    string
	}
	drag      bool // Schieber wird gezogen
	dragStart int
	dragFrom  int

	fTitle, fSub, fBody, fTerm, fLink, fNav, fNavActive, fHero *walk.Font
	glyphs                                                     *glyphCache
	images                                                     map[string]*walk.Bitmap
}

func newHelpView(mw *AppWindow, de, en string, withNav bool) *helpView {
	h := &helpView{mw: mw, de: de, en: en, withNav: withNav,
		parsed: map[string][]helpChapter{}, images: map[string]*walk.Bitmap{}}
	h.fTitle, _ = walk.NewFont(fontFamily, 16, walk.FontBold)
	h.fSub, _ = walk.NewFont(fontFamily, 11, walk.FontBold)
	h.fBody, _ = walk.NewFont(fontFamily, 10, 0)
	h.fTerm, _ = walk.NewFont(fontFamily, 10, walk.FontBold)
	h.fLink, _ = walk.NewFont(fontFamily, 10, walk.FontUnderline)
	h.fNav, _ = walk.NewFont(fontFamily, 10, 0)
	h.fNavActive, _ = walk.NewFont(fontFamily, 10, walk.FontBold)
	h.fHero, _ = walk.NewFont(fontFamily, 12, walk.FontBold)
	h.glyphs = newGlyphCache()
	return h
}

const (
	helpNavWidth = 190
	helpNavRow   = 30
	helpPad      = 22 // Innenabstand der Seite
	helpBar      = 8  // Breite der Scrollleiste
)

// panel baut den Reiter: Kapitelliste links (falls gewuenscht), Seite rechts.
func (h *helpView) panel(assign **walk.Composite) Widget {
	page := CustomWidget{
		AssignTo:            &h.page,
		StretchFactor:       10,
		PaintMode:           PaintBuffered, // DoubleBuffering wertet walk nicht aus; ohne das wird vor jedem Zeichnen geloescht (Flackern)
		InvalidatesOnResize: true,
		Paint:               func(c *walk.Canvas, _ walk.Rectangle) error { return h.paintPage(c) },
		OnMouseDown:         func(x, y int, b walk.MouseButton) { h.mouseDown(x, y, b) },
		OnMouseMove:         func(x, y int, b walk.MouseButton) { h.mouseMove(x, y) },
		OnMouseUp:           func(x, y int, b walk.MouseButton) { h.mouseUp(x, y, b) },
		OnKeyDown:           func(k walk.Key) { h.key(k) },
	}
	children := []Widget{page}
	if h.withNav {
		children = []Widget{h.navWidget(), page}
	}
	return Composite{
		AssignTo: assign,
		Visible:  false,
		Layout:   HBox{Margins: Margins{Left: gutter, Top: 12, Right: gutter, Bottom: 10}, Spacing: gapM},
		Children: children,
	}
}

func (h *helpView) navWidget() Widget {
	return CustomWidget{
		AssignTo:            &h.nav,
		MinSize:             Size{Width: helpNavWidth},
		MaxSize:             Size{Width: helpNavWidth},
		PaintMode:           PaintBuffered, // DoubleBuffering wertet walk nicht aus; ohne das wird vor jedem Zeichnen geloescht (Flackern)
		InvalidatesOnResize: true,
		Paint:               func(c *walk.Canvas, _ walk.Rectangle) error { return h.paintNav(c) },
		OnMouseUp: func(x, y int, b walk.MouseButton) {
			if b != walk.LeftButton {
				return
			}
			if i := (y - h.nav.IntFrom96DPI(6)) / h.nav.IntFrom96DPI(helpNavRow); i >= 0 && i < len(h.chapters()) {
				h.show(i)
			}
		},
	}
}

// attach haengt das Mausrad an; das geht erst, wenn das Widget existiert.
func (h *helpView) attach() {
	if h.nav != nil {
		h.nav.SetCursor(walk.CursorHand())
	}
	h.page.MouseWheel().Attach(func(x, y int, b walk.MouseButton) {
		h.scrollBy(-walk.MouseWheelEventDelta(b) * h.page.IntFrom96DPI(60) / 120)
	})
}

// show wechselt das Kapitel und springt an den Anfang.
func (h *helpView) show(i int) {
	h.chapter, h.scroll = i, 0
	if h.nav != nil {
		h.nav.Invalidate()
	}
	h.page.Invalidate()
}

// retranslate: nach dem Sprachwechsel dasselbe Kapitel in der neuen Sprache.
func (h *helpView) retranslate() {
	if n := len(h.chapters()); h.chapter >= n {
		h.chapter = 0
	}
	h.scroll = 0
	if h.nav != nil {
		h.nav.Invalidate()
	}
	h.page.Invalidate()
}

func (h *helpView) paintNav(c *walk.Canvas) error {
	b := h.nav.ClientBoundsPixels()
	c.FillRectanglePixels(brushOf(colAccountBar), b)
	px := h.nav.IntFrom96DPI
	y := px(6)
	for i, ch := range h.chapters() {
		r := walk.Rectangle{X: 0, Y: y, Width: b.Width, Height: px(helpNavRow)}
		font, col := h.fNav, colNavText
		if i == h.chapter {
			font, col = h.fNavActive, colHelpTerm
			c.FillRectanglePixels(brushOf(walk.RGB(255, 255, 255)), r)
			c.FillRectanglePixels(brushOf(colAccent), walk.Rectangle{X: 0, Y: r.Y, Width: px(4), Height: r.Height})
		}
		tr := walk.Rectangle{X: px(16), Y: r.Y, Width: r.Width - px(20), Height: r.Height}
		c.DrawTextPixels(ch.title, font, col, tr, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix)
		y += r.Height
	}
	return nil
}

// layout platziert das Kapitel fuer die Breite w (Pixel). Begriffe stehen in
// einer eigenen Spalte, deren Breite sich nach dem laengsten Begriff des
// jeweiligen Abschnitts richtet.
func (h *helpView) layout(c *walk.Canvas, w int) {
	h.layoutAt.width, h.layoutAt.chapter, h.layoutAt.lang = w, h.chapter, currentLang
	h.items = h.items[:0]
	px := h.page.IntFrom96DPI
	chs := h.chapters()
	if h.chapter >= len(chs) {
		h.height = 0
		return
	}
	ch := chs[h.chapter]
	x0, right := px(helpPad), w-px(helpPad)-px(helpBar)
	width := right - x0
	// Lesbare Zeilenlaenge, auch wenn das Fenster sehr breit ist.
	if max := px(760); width > max {
		width = max
	}
	measure := func(s string, f *walk.Font, wd int) int {
		r, _, err := c.MeasureTextPixels(s, f, walk.Rectangle{Width: wd, Height: 100000}, walk.TextWordbreak)
		if err != nil {
			return 0
		}
		return r.Height
	}
	// Breite einer Zeile. MeasureTextPixels taugt dafuer nicht: walk misst
	// ueber eine Metadatei und gibt als Breite die uebergebene zurueck.
	measureW := func(s string, f *walk.Font) int {
		return textWidth(c, s, f)
	}
	text := func(r walk.Rectangle, s string, f *walk.Font, col walk.Color) {
		h.items = append(h.items, helpItem{r: r, text: s, font: f, color: col})
	}

	y := px(helpPad) - px(4)
	th := measure(ch.title, h.fTitle, width)
	text(walk.Rectangle{X: x0, Y: y, Width: width, Height: th}, ch.title, h.fTitle, colHelpTerm)
	y += th + px(4)
	h.items = append(h.items, helpItem{r: walk.Rectangle{X: x0, Y: y, Width: px(44), Height: px(3)}, color: colAccent, fill: true})
	y += px(3) + px(14)

	prev := -1
	termCol := 0
	for i, b := range ch.blocks {
		// Abstand nach oben: Zeilen derselben Art stehen enger zusammen.
		switch {
		case prev == -1:
		case b.kind == hbHero:
			y += px(22)
		case b.kind == hbSub:
			y += px(16)
		case b.kind == prev && (b.kind == hbBullet || b.kind == hbTerm || b.kind == hbLink):
			y += px(5)
		default:
			y += px(10)
		}
		switch b.kind {
		case hbSub:
			hh := measure(b.text, h.fSub, width)
			text(walk.Rectangle{X: x0, Y: y, Width: width, Height: hh}, b.text, h.fSub, colHelpSub)
			y += hh
		case hbPara:
			hh := measure(b.text, h.fBody, width)
			text(walk.Rectangle{X: x0, Y: y, Width: width, Height: hh}, b.text, h.fBody, colHelpText)
			y += hh
		case hbBullet:
			ind := px(18)
			hh := measure(b.text, h.fBody, width-ind)
			text(walk.Rectangle{X: x0 + px(4), Y: y, Width: ind, Height: hh}, "•", h.fBody, colAccentDown)
			text(walk.Rectangle{X: x0 + ind, Y: y, Width: width - ind, Height: hh}, b.text, h.fBody, colHelpText)
			y += hh
		case hbLink:
			lw := measureW(b.text, h.fLink)
			hh := measure(b.text, h.fLink, width)
			h.items = append(h.items, helpItem{r: walk.Rectangle{X: x0, Y: y, Width: lw, Height: hh}, text: b.text,
				font: h.fLink, color: colHelpLink, url: b.url})
			y += hh
		case hbImage:
			maxW := width
			if b.width > 0 && px(b.width) < maxW {
				maxW = px(b.width)
			}
			bmp := h.image(b.text, maxW)
			if bmp == nil {
				break
			}
			s := bmp.Size() // Pixel, siehe image
			h.items = append(h.items, helpItem{r: walk.Rectangle{X: x0, Y: y, Width: s.Width, Height: s.Height}, img: bmp})
			y += s.Height
		case hbHero:
			hh := width * 36 / 100
			bmp := h.hero(b.text, b.term, float64(b.width)/100, width, hh)
			if bmp == nil {
				break
			}
			h.items = append(h.items, helpItem{r: walk.Rectangle{X: x0, Y: y, Width: width, Height: hh}, img: bmp})
			if b.url != "" {
				lh := measure(b.url, h.fHero, width)
				h.items = append(h.items, helpItem{r: walk.Rectangle{X: x0, Y: y + hh - px(16) - lh, Width: width - px(20), Height: lh},
					text: b.url, font: h.fHero, color: colAccent, align: walk.TextRight | walk.TextSingleLine})
			}
			y += hh
		case hbButtons:
			f := strings.Split(b.text, "|")
			x := x0
			bh := px(32)
			for k := 0; k+1 < len(f); k += 2 {
				label, url := strings.TrimSpace(f[k]), strings.TrimSpace(f[k+1])
				bw := measureW(label, h.fTerm) + px(32)
				face, edge, ink := colAccent, colAccentDown, colAccentText
				if k > 0 {
					face, edge, ink = walk.RGB(255, 255, 255), walk.RGB(176, 184, 198), colHelpTerm
				}
				r := walk.Rectangle{X: x, Y: y, Width: bw, Height: bh}
				h.items = append(h.items,
					helpItem{r: r, img: h.glyphs.pill(bw, bh, h.page.DPI(), face, edge, walk.RGB(255, 255, 255)), url: url},
					helpItem{r: r, text: label, font: h.fTerm, color: ink, url: url, align: walk.TextCenter | walk.TextVCenter | walk.TextSingleLine})
				x += bw + px(10)
			}
			y += bh
		case hbTerm:
			// Spaltenbreite einmal je zusammenhaengender Begriffsliste.
			if prev != hbTerm {
				termCol = 0
				for j := i; j < len(ch.blocks) && ch.blocks[j].kind == hbTerm; j++ {
					tw := measureW(ch.blocks[j].term, h.fTerm)
					if ch.blocks[j].ball != 0 {
						tw += px(22)
					}
					if tw > termCol {
						termCol = tw
					}
				}
				termCol += px(18)
				if max := width * 2 / 5; termCol > max {
					termCol = max
				}
			}
			tx := x0
			if col, ok := helpBallCols[b.ball]; ok {
				d := px(14)
				h.items = append(h.items, helpItem{r: walk.Rectangle{X: x0, Y: y + px(2), Width: d, Height: d}, ball: col})
				tx += px(22)
			}
			termW := x0 + termCol - tx - px(12)
			t1 := measure(b.term, h.fTerm, termW)
			t2 := measure(b.text, h.fBody, width-termCol)
			text(walk.Rectangle{X: tx, Y: y, Width: termW, Height: t1}, b.term, h.fTerm, colHelpTerm)
			text(walk.Rectangle{X: x0 + termCol, Y: y, Width: width - termCol, Height: t2}, b.text, h.fBody, colHelpText)
			if t1 > t2 {
				t2 = t1
			}
			y += t2
		}
		prev = b.kind
	}
	h.height = y + px(helpPad)
}

// image liefert ein Bild des Handbuchs, vorab mit Catmull-Rom auf hoechstens
// maxW Pixel Breite gebracht (wie die Logos, siehe loadLogo).
func (h *helpView) image(name string, maxW int) *walk.Bitmap {
	key := name + "@" + strconv.Itoa(maxW)
	if bmp, ok := h.images[key]; ok {
		return bmp
	}
	data, ok := helpImages[name]
	if !ok {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	tw := h.page.IntFrom96DPI(b.Dx())
	if tw > maxW {
		tw = maxW
	}
	th := tw * b.Dy() / b.Dx()
	small := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(small, small.Bounds(), img, b, xdraw.Src, nil)
	// Bei 96 DPI ist eine Bitmap-Einheit ein Pixel; gezeichnet wird ohnehin
	// in Pixeln (DrawImageStretchedPixels), also ohne erneutes Skalieren.
	bmp, err := walk.NewBitmapFromImageForDPI(small, 96)
	if err != nil {
		return nil
	}
	h.images[key] = bmp
	return bmp
}

func (h *helpView) maxScroll() int {
	v := h.height - h.page.ClientBoundsPixels().Height
	if v < 0 {
		return 0
	}
	return v
}

func (h *helpView) scrollBy(d int) {
	s := h.scroll + d
	if s > h.maxScroll() {
		s = h.maxScroll()
	}
	if s < 0 {
		s = 0
	}
	if s != h.scroll {
		h.scroll = s
		h.page.Invalidate()
	}
}

// thumb ist der Schieber der Scrollleiste; leer, wenn alles hineinpasst.
func (h *helpView) thumb() (track, thumb walk.Rectangle) {
	b := h.page.ClientBoundsPixels()
	px := h.page.IntFrom96DPI
	track = walk.Rectangle{X: b.Width - px(helpBar) - px(4), Y: px(4), Width: px(helpBar), Height: b.Height - px(8)}
	if h.height <= b.Height || h.height == 0 {
		return track, walk.Rectangle{}
	}
	th := track.Height * b.Height / h.height
	if th < px(30) {
		th = px(30)
	}
	ty := track.Y + (track.Height-th)*h.scroll/h.maxScroll()
	return track, walk.Rectangle{X: track.X, Y: ty, Width: track.Width, Height: th}
}

func (h *helpView) paintPage(c *walk.Canvas) error {
	b := h.page.ClientBoundsPixels()
	c.FillRectanglePixels(brushOf(walk.RGB(255, 255, 255)), b)
	if h.layoutAt.width != b.Width || h.layoutAt.chapter != h.chapter || h.layoutAt.lang != currentLang || h.items == nil {
		h.layout(c, b.Width)
	}
	if h.scroll > h.maxScroll() {
		h.scroll = h.maxScroll()
	}
	for _, it := range h.items {
		r := it.r
		r.Y -= h.scroll
		if r.Y+r.Height < 0 || r.Y > b.Height {
			continue
		}
		switch {
		case it.fill:
			c.FillRectanglePixels(brushOf(it.color), r)
		case it.ball != 0:
			c.FillEllipsePixels(brushOf(it.ball), r)
		case it.img != nil:
			c.DrawImageStretchedPixels(it.img, r)
		default:
			format := walk.TextWordbreak
			if it.align != 0 {
				format = it.align
			}
			c.DrawTextPixels(it.text, it.font, it.color, r, format|walk.TextNoPrefix)
		}
	}
	if track, th := h.thumb(); th.Height > 0 {
		c.FillRoundedRectanglePixels(brushOf(colHelpTrack), track, walk.Size{Width: track.Width, Height: track.Width})
		c.FillRoundedRectanglePixels(brushOf(colHelpThumb), th, walk.Size{Width: th.Width, Height: th.Width})
	}
	return nil
}

// linkAt liefert die URL unter dem Mauszeiger, sonst "".
func (h *helpView) linkAt(x, y int) string {
	for _, it := range h.items {
		if it.url == "" {
			continue
		}
		r := it.r
		r.Y -= h.scroll
		if x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height {
			return it.url
		}
	}
	return ""
}

func (h *helpView) mouseDown(x, y int, b walk.MouseButton) {
	h.page.SetFocus() // fuer Mausrad und Tasten
	if b != walk.LeftButton {
		return
	}
	track, th := h.thumb()
	if th.Height == 0 || x < track.X-h.page.IntFrom96DPI(4) {
		return
	}
	switch {
	case y >= th.Y && y < th.Y+th.Height:
		h.drag, h.dragStart, h.dragFrom = true, y, h.scroll
	case y < th.Y:
		h.scrollBy(-h.page.ClientBoundsPixels().Height * 9 / 10)
	default:
		h.scrollBy(h.page.ClientBoundsPixels().Height * 9 / 10)
	}
}

func (h *helpView) mouseMove(x, y int) {
	if h.drag {
		track, th := h.thumb()
		if free := track.Height - th.Height; free > 0 {
			h.scrollBy(h.dragFrom + (y-h.dragStart)*h.maxScroll()/free - h.scroll)
		}
		return
	}
	if h.linkAt(x, y) != "" {
		h.page.SetCursor(walk.CursorHand())
	} else {
		h.page.SetCursor(walk.CursorArrow())
	}
}

func (h *helpView) mouseUp(x, y int, b walk.MouseButton) {
	if h.drag {
		h.drag = false
		return
	}
	if b == walk.LeftButton {
		if u := h.linkAt(x, y); u != "" {
			openURL(u)
		}
	}
}

func (h *helpView) key(k walk.Key) {
	page := h.page.ClientBoundsPixels().Height * 9 / 10
	line := h.page.IntFrom96DPI(40)
	switch k {
	case walk.KeyDown:
		h.scrollBy(line)
	case walk.KeyUp:
		h.scrollBy(-line)
	case walk.KeyNext, walk.KeySpace:
		h.scrollBy(page)
	case walk.KeyPrior:
		h.scrollBy(-page)
	case walk.KeyHome:
		h.scrollBy(-h.scroll)
	case walk.KeyEnd:
		h.scrollBy(h.maxScroll())
	}
}

var widthFonts = map[[3]int]win.HFONT{}

// textWidth misst eine Zeile in Pixeln mit GetTextExtentPoint32. walk gibt
// den Font-Handle nicht heraus, also wird ein gleichwertiger erzeugt (Familie,
// Groesse, fett) und je DPI zwischengespeichert.
func textWidth(c *walk.Canvas, s string, f *walk.Font) int {
	dpi := c.DPI()
	bold := 0
	if f.Bold() {
		bold = 1
	}
	key := [3]int{f.PointSize(), bold, dpi}
	hf, ok := widthFonts[key]
	if !ok {
		lf := win.LOGFONT{LfHeight: -int32(f.PointSize() * dpi / 72), LfWeight: win.FW_NORMAL, LfCharSet: win.DEFAULT_CHARSET}
		if bold == 1 {
			lf.LfWeight = win.FW_BOLD
		}
		name := syscall.StringToUTF16(f.Family())
		copy(lf.LfFaceName[:len(lf.LfFaceName)-1], name)
		hf = win.CreateFontIndirect(&lf)
		widthFonts[key] = hf
	}
	hdc := c.HDC()
	old := win.SelectObject(hdc, win.HGDIOBJ(hf))
	defer win.SelectObject(hdc, old)
	var sz win.SIZE
	u := syscall.StringToUTF16(s)
	win.GetTextExtentPoint32(hdc, &u[0], int32(len(u)-1), &sz)
	return int(sz.CX)
}

// hero setzt eine Titelkarte w x hh Pixel zusammen: den Screenshot shot als
// Ausschnitt (focus 0 = oberer, 1 = unterer Rand), unten in das Dunkelblau
// der Kopfleiste verlaufend, darauf links unten das Logo (eins der hellen
// Kopfleisten-Logos), Ecken abgerundet auf den weissen Seitengrund.
func (h *helpView) hero(shot, logo string, focus float64, w, hh int) *walk.Bitmap {
	dpi := h.page.DPI()
	key := fmt.Sprintf("hero %s %s %.2f %d %d %d", shot, logo, focus, w, hh, dpi)
	if bmp, ok := h.images[key]; ok {
		return bmp
	}
	src, _, err := image.Decode(bytes.NewReader(helpImages[shot]))
	if err != nil {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, w, hh))
	// Ausschnitt mit dem Seitenverhaeltnis der Karte
	sb := src.Bounds()
	cw, ch := sb.Dx(), sb.Dx()*hh/w
	if ch > sb.Dy() {
		cw, ch = sb.Dy()*w/hh, sb.Dy()
	}
	cx := sb.Min.X + (sb.Dx()-cw)/2
	cy := sb.Min.Y + int(float64(sb.Dy()-ch)*math.Max(0, math.Min(1, focus)))
	xdraw.CatmullRom.Scale(out, out.Bounds(), src, image.Rect(cx, cy, cx+cw, cy+ch), xdraw.Src, nil)
	// Verlauf nach unten ins Dunkelblau
	dark := colHeader
	for y := 0; y < hh; y++ {
		t := (float64(y)/float64(hh) - 0.35) / 0.65
		if t <= 0 {
			continue
		}
		a := 0.88 * t * t * (3 - 2*t)
		for x := 0; x < w; x++ {
			o := out.PixOffset(x, y)
			out.Pix[o] = uint8(float64(out.Pix[o])*(1-a) + float64(dark.R())*a)
			out.Pix[o+1] = uint8(float64(out.Pix[o+1])*(1-a) + float64(dark.G())*a)
			out.Pix[o+2] = uint8(float64(out.Pix[o+2])*(1-a) + float64(dark.B())*a)
		}
	}
	// Logo links unten
	if data, ok := helpImages[logo]; ok {
		if lg, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			lb := lg.Bounds()
			lh := hh * 26 / 100
			lw := lh * lb.Dx() / lb.Dy()
			m := h.page.IntFrom96DPI(18)
			dst := image.Rect(m, hh-m*8/10-lh, m+lw, hh-m*8/10)
			xdraw.CatmullRom.Scale(out, dst, lg, lb, xdraw.Over, nil)
		}
	}
	// runde Ecken auf Weiss
	r := float64(h.page.IntFrom96DPI(10))
	for y := 0; y < hh; y++ {
		for x := 0; x < w; x++ {
			dx, dy := 0.0, 0.0
			if fx := float64(x) + 0.5; fx < r {
				dx = r - fx
			} else if fx > float64(w)-r {
				dx = fx - (float64(w) - r)
			}
			if fy := float64(y) + 0.5; fy < r {
				dy = r - fy
			} else if fy > float64(hh)-r {
				dy = fy - (float64(hh) - r)
			}
			if dx == 0 || dy == 0 {
				continue
			}
			cover := math.Max(0, math.Min(1, r-math.Sqrt(dx*dx+dy*dy)+0.5))
			o := out.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				out.Pix[o+c] = uint8(float64(out.Pix[o+c])*cover + 255*(1-cover))
			}
		}
	}
	bmp, err := walk.NewBitmapFromImageForDPI(out, 96)
	if err != nil {
		return nil
	}
	h.images[key] = bmp
	return bmp
}
