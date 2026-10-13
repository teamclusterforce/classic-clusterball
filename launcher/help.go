// Hilfe-Reiter: der spielrelevante Teil des Handbuchs von clusterball.de,
// direkt im Launcher statt als Link auf die (langsame) Webseite. Die Texte
// liegen je Sprache in help_de.txt und help_en.txt, links stehen die Kapitel,
// rechts wird das gewaehlte Kapitel selbst gezeichnet und gescrollt.

package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/jpeg"
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

var helpImages = map[string][]byte{"help_interface.jpg": helpInterfaceJPG}

const (
	hbPara = iota
	hbSub
	hbBullet
	hbTerm
	hbImage
	hbLink
)

type helpBlock struct {
	kind int
	term string // hbTerm: Begriff links
	text string // Absatz, Ueberschrift, Beschreibung, Linktext, Bildname
	url  string // hbLink
	ball byte   // hbTerm: 'r', 'b', 'y' fuer einen farbigen Ball vor dem Begriff
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
			b = helpBlock{kind: hbImage, text: strings.TrimSpace(line[5 : len(line)-1])}
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

var helpParsed = map[string][]helpChapter{}

// helpChapters liefert die Kapitel in der aktuellen Sprache.
func helpChapters() []helpChapter {
	if c, ok := helpParsed[currentLang]; ok {
		return c
	}
	src := helpDE
	if currentLang == "en" {
		src = helpEN
	}
	c := parseHelp(src)
	helpParsed[currentLang] = c
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
	fill  bool // r ist eine Flaeche in color (Akzentlinie)
}

type helpView struct {
	mw       *AppWindow
	nav      *walk.CustomWidget
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

	fTitle, fSub, fBody, fTerm, fLink, fNav, fNavActive *walk.Font
	images                                              map[string]*walk.Bitmap
}

func newHelpView(mw *AppWindow) *helpView {
	h := &helpView{mw: mw, images: map[string]*walk.Bitmap{}}
	h.fTitle, _ = walk.NewFont(fontFamily, 16, walk.FontBold)
	h.fSub, _ = walk.NewFont(fontFamily, 11, walk.FontBold)
	h.fBody, _ = walk.NewFont(fontFamily, 10, 0)
	h.fTerm, _ = walk.NewFont(fontFamily, 10, walk.FontBold)
	h.fLink, _ = walk.NewFont(fontFamily, 10, walk.FontUnderline)
	h.fNav, _ = walk.NewFont(fontFamily, 10, 0)
	h.fNavActive, _ = walk.NewFont(fontFamily, 10, walk.FontBold)
	return h
}

const (
	helpNavWidth = 190
	helpNavRow   = 30
	helpPad      = 22 // Innenabstand der Seite
	helpBar      = 8  // Breite der Scrollleiste
)

// panel baut den Reiter: Kapitelliste links, Seite rechts.
func (h *helpView) panel(assign **walk.Composite) Widget {
	return Composite{
		AssignTo: assign,
		Visible:  false,
		Layout:   HBox{Margins: Margins{Left: gutter, Top: 12, Right: gutter, Bottom: 10}, Spacing: gapM},
		Children: []Widget{
			CustomWidget{
				AssignTo:            &h.nav,
				MinSize:             Size{Width: helpNavWidth},
				MaxSize:             Size{Width: helpNavWidth},
				DoubleBuffering:     true,
				InvalidatesOnResize: true,
				Paint:               func(c *walk.Canvas, _ walk.Rectangle) error { return h.paintNav(c) },
				OnMouseUp: func(x, y int, b walk.MouseButton) {
					if b != walk.LeftButton {
						return
					}
					if i := (y - h.nav.IntFrom96DPI(6)) / h.nav.IntFrom96DPI(helpNavRow); i >= 0 && i < len(helpChapters()) {
						h.show(i)
					}
				},
			},
			CustomWidget{
				AssignTo:            &h.page,
				StretchFactor:       10,
				DoubleBuffering:     true,
				InvalidatesOnResize: true,
				Paint:               func(c *walk.Canvas, _ walk.Rectangle) error { return h.paintPage(c) },
				OnMouseDown:         func(x, y int, b walk.MouseButton) { h.mouseDown(x, y, b) },
				OnMouseMove:         func(x, y int, b walk.MouseButton) { h.mouseMove(x, y) },
				OnMouseUp:           func(x, y int, b walk.MouseButton) { h.mouseUp(x, y, b) },
				OnKeyDown:           func(k walk.Key) { h.key(k) },
			},
		},
	}
}

// attach haengt das Mausrad an; das geht erst, wenn das Widget existiert.
func (h *helpView) attach() {
	h.nav.SetCursor(walk.CursorHand())
	h.page.MouseWheel().Attach(func(x, y int, b walk.MouseButton) {
		h.scrollBy(-walk.MouseWheelEventDelta(b) * h.page.IntFrom96DPI(60) / 120)
	})
}

// show wechselt das Kapitel und springt an den Anfang.
func (h *helpView) show(i int) {
	h.chapter, h.scroll = i, 0
	h.nav.Invalidate()
	h.page.Invalidate()
}

// retranslate: nach dem Sprachwechsel dasselbe Kapitel in der neuen Sprache.
func (h *helpView) retranslate() {
	if n := len(helpChapters()); h.chapter >= n {
		h.chapter = 0
	}
	h.scroll = 0
	h.nav.Invalidate()
	h.page.Invalidate()
}

func (h *helpView) paintNav(c *walk.Canvas) error {
	b := h.nav.ClientBoundsPixels()
	c.FillRectanglePixels(brushOf(colAccountBar), b)
	px := h.nav.IntFrom96DPI
	y := px(6)
	for i, ch := range helpChapters() {
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
	chs := helpChapters()
	if h.chapter >= len(chs) {
		h.height = 0
		return
	}
	ch := chs[h.chapter]
	x0, right := px(helpPad), w-px(helpPad)-px(helpBar)
	width := right - x0
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
			bmp := h.image(b.text, width)
			if bmp == nil {
				break
			}
			s := bmp.Size() // Pixel, siehe image
			h.items = append(h.items, helpItem{r: walk.Rectangle{X: x0, Y: y, Width: s.Width, Height: s.Height}, img: bmp})
			y += s.Height
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
	img, err := jpeg.Decode(bytes.NewReader(data))
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
			c.DrawTextPixels(it.text, it.font, it.color, r, walk.TextWordbreak|walk.TextNoPrefix)
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
