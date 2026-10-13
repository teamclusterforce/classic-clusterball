// Reiter "Steuerung": die Tastenbelegung des Spiels (config.cfg) mit
// gezeichneten Tasten, Voreinstellungen "WASD + Maus" und "Klassisch" und dem
// Mausrad.
//
// Das Spiel speichert jede Belegung als Tastennamen in seiner eigenen Sprache:
// Buchstaben und Ziffern als Zeichen ("w", "1"), F-Tasten als "F1", alles
// andere aus seiner Namenstabelle (Local.mrg, Texte 274-344, z.B. "Leertaste",
// "Linke Maustaste"). Das Mausrad kennt es nicht: der Renderer macht aus jeder
// Raste einen Druck auf F11 (hoch) bzw. F12 (runter), siehe WheelUp/WheelDown
// in der clusterball_d3d.ini. Eine Aktion "auf dem Mausrad" ist im Spiel also
// auf F11/F12 gelegt; F11 und F12 sind deshalb sonst nicht belegbar.

package main

import (
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

const (
	wheelUpName   = "F11"
	wheelDownName = "F12"
	wheelUpDIK    = 0x57 // DirectInput-Scancodes von F11/F12 fuer den Renderer
	wheelDownDIK  = 0x58
)

// ctrlActions: die Aktionen in der Reihenfolge des Reiters (Schluessel der
// config.cfg, Textschluessel "ctl.<name>").
var ctrlActions = []string{
	"accelerate", "brake", "turn_left", "turn_right", "pitch_up", "pitch_down", "roll", "superboost",
	"fire", "next_weapon", "prev_weapon", "change_target", "release_balls", "talk", "toggle_scoreboard", "change_view",
}

// gameKey ist ein Eintrag der Namenstabelle des Spiels. dik ist der
// DirectInput-Scancode, mouse die Maustaste (1-4) bei Maus-Eintraegen.
type gameKey struct {
	dik    int
	mouse  int
	de, en string // Bytes wie im Spiel (Latin-1)
}

// gameKeys: Local.mrg, Texte 274-344, ohne die japanischen Sondertasten und
// "Zehnerblock Punkt " (mit Leerzeichen am Ende, das die config.cfg nicht
// sauber traegt).
var gameKeys = []gameKey{
	{dik: 0x01, de: "Escape", en: "Escape"},
	{dik: 0x0C, de: "Minus", en: "Minus"},
	{dik: 0x0D, de: "Gleich", en: "Equals"},
	{dik: 0x0E, de: "R\xfccktaste", en: "Backspace"},
	{dik: 0x0F, de: "Tabulator", en: "Tab"},
	{dik: 0x1A, de: "Linke Klammer", en: "Left bracket"},
	{dik: 0x1B, de: "Rechte Klammer", en: "Right bracket"},
	{dik: 0x1C, de: "Enter", en: "Return"},
	{dik: 0x1D, de: "STRG-Links", en: "Left Control"},
	{dik: 0x27, de: "Semikolon", en: "Semi colon"},
	{dik: 0x28, de: "Apostroph", en: "Apostrophe"},
	{dik: 0x29, de: "Hochkomma", en: "Grave accent"},
	{dik: 0x2A, de: "Shift-Links", en: "Left shift"},
	{dik: 0x2B, de: "Backslash", en: "Backslash"},
	{dik: 0x56, de: "Gr\xf6sser/Kleiner als", en: "Bigger/Smaller than"},
	{dik: 0x33, de: "Komma", en: "Comma"},
	{dik: 0x34, de: "Punkt", en: "Period"},
	{dik: 0x35, de: "Slash", en: "Slash"},
	{dik: 0x36, de: "Shift-Rechts", en: "Right shift"},
	{dik: 0x37, de: "Num. Multiplizieren", en: "Numpad multiply"},
	{dik: 0x38, de: "Alt-Links", en: "Left alt"},
	{dik: 0x39, de: "Leertaste", en: "Space"},
	{dik: 0x3A, de: "Feststelltaste", en: "Caps lock"},
	{dik: 0x45, de: "Num-Lock", en: "Num lock"},
	{dik: 0x47, de: "Zehnerblock 7", en: "Numpad 7"},
	{dik: 0x48, de: "Zehnerblock 8", en: "Numpad 8"},
	{dik: 0x49, de: "Zehnerblock 9", en: "Numpad 9"},
	{dik: 0x4A, de: "Zehnerblock Minus", en: "Numpad minus"},
	{dik: 0x4B, de: "Zehnerblock 4", en: "Numpad 4"},
	{dik: 0x4C, de: "Zehnerblock 5", en: "Numpad 5"},
	{dik: 0x4D, de: "Zehnerblock 6", en: "Numpad 6"},
	{dik: 0x4E, de: "Zehnerblock Plus", en: "Numpad plus"},
	{dik: 0x4F, de: "Zehnerblock 1", en: "Numpad 1"},
	{dik: 0x50, de: "Zehnerblock 2", en: "Numpad 2"},
	{dik: 0x51, de: "Zehnerblock 3", en: "Numpad 3"},
	{dik: 0x52, de: "Zehnerblock 0", en: "Numpad 0"},
	{dik: 0x9C, de: "Zehnerblock Enter", en: "Numpad enter"},
	{dik: 0x9D, de: "STRG-Rechts", en: "Right control"},
	{dik: 0xB5, de: "Zehnerblock Div.", en: "Numpad divide"},
	{dik: 0xB8, de: "Alt-Rechts", en: "Right alt"},
	{dik: 0xC7, de: "Pos1", en: "Home"},
	{dik: 0xC8, de: "Pfeil Hoch", en: "Up arrow"},
	{dik: 0xC9, de: "Seite Hoch", en: "Page up"},
	{dik: 0xCB, de: "Linker Pfeil", en: "Left arrow"},
	{dik: 0xCD, de: "Rechter Pfeil", en: "Right arrow"},
	{dik: 0xCF, de: "Ende", en: "End"},
	{dik: 0xD0, de: "Pfeil runter", en: "Down arrow"},
	{dik: 0xD1, de: "Seite runter", en: "Page down"},
	{dik: 0xD2, de: "Einfg.", en: "Insert"},
	{dik: 0xD3, de: "Entf.", en: "Delete"},
	{mouse: 1, de: "Linke Maustaste", en: "Left mouse btn"},
	{mouse: 2, de: "Rechte Maustaste", en: "Right mouse btn"},
	{mouse: 3, de: "Mittlere Maustaste", en: "Middle mouse btn"},
	{mouse: 4, de: "Maustaste 4", en: "Mouse btn 4"},
}

// Voreinstellungen, mit den deutschen Namen des Spiels. "Klassisch" ist die
// Belegung, mit der Clusterball ausgeliefert wurde.
var (
	presetClassic = map[string]string{
		"accelerate": "w", "brake": "s", "turn_left": "Linker Pfeil", "turn_right": "Rechter Pfeil",
		"pitch_up": "Pfeil runter", "pitch_down": "Pfeil Hoch", "roll": "e", "superboost": "Rechte Maustaste",
		"fire": "Linke Maustaste", "next_weapon": "d", "prev_weapon": "a", "change_target": "q",
		"release_balls": "Leertaste", "talk": "Enter", "toggle_scoreboard": "t", "change_view": "v",
	}
	presetWASD = map[string]string{
		"accelerate": "w", "brake": "s", "turn_left": "a", "turn_right": "d",
		"pitch_up": "Pfeil runter", "pitch_down": "Pfeil Hoch", "roll": "e", "superboost": "Rechte Maustaste",
		"fire": "Linke Maustaste", "next_weapon": wheelUpName, "prev_weapon": wheelDownName, "change_target": "q",
		"release_balls": "Leertaste", "talk": "Enter", "toggle_scoreboard": "t", "change_view": "v",
	}
)

func findGameKey(name string) (gameKey, bool) {
	for _, k := range gameKeys {
		if strings.EqualFold(name, k.de) || strings.EqualFold(name, k.en) {
			return k, true
		}
	}
	return gameKey{}, false
}

// bindingsLang erkennt die Sprache der Tastennamen in der config.cfg: "en",
// sobald ein Name nur in der englischen Spalte steht, sonst "de".
func bindingsLang(b map[string]string) string {
	for _, v := range b {
		if k, ok := findGameKey(v); ok && k.de != k.en && strings.EqualFold(v, k.en) {
			return "en"
		}
	}
	return "de"
}

// inLang gibt einen Tastennamen in der Sprache lang des Spiels zurueck.
func inLang(name, lang string) string {
	k, ok := findGameKey(name)
	if !ok {
		return name
	}
	if lang == "en" {
		return k.en
	}
	return k.de
}

// presetIn liefert eine Voreinstellung mit Namen in der Sprache lang.
func presetIn(p map[string]string, lang string) map[string]string {
	out := make(map[string]string, len(p))
	for a, n := range p {
		out[a] = inLang(n, lang)
	}
	return out
}

// sameBindings vergleicht zwei Belegungen ueber alle Aktionen, unabhaengig
// von Sprache und Gross-/Kleinschreibung.
func sameBindings(a, b map[string]string) bool {
	for _, act := range ctrlActions {
		if !strings.EqualFold(inLang(a[act], "de"), inLang(b[act], "de")) {
			return false
		}
	}
	return true
}

// readBindings liest die Belegung aus der config.cfg.
func readBindings(g *GameConfig) map[string]string {
	out := map[string]string{}
	for _, act := range ctrlActions {
		if v, ok := g.Get(act); ok {
			out[act] = strings.TrimSpace(v)
		}
	}
	return out
}

// writeBindings traegt die Belegung in die config.cfg ein.
func writeBindings(g *GameConfig, b map[string]string) {
	for _, act := range ctrlActions {
		if v := b[act]; v != "" {
			g.Set(act, v)
		}
	}
}

// saveBindings schreibt nur die Belegung, sofort nach jeder Aenderung im Reiter.
func saveBindings(cfg *AppConfig) error {
	path := gameConfigPath(cfg)
	if path == "" || len(cfg.Bindings) == 0 {
		return nil
	}
	g, err := LoadGameConfig(path)
	if err != nil {
		return err
	}
	writeBindings(g, cfg.Bindings)
	return g.Save(path)
}

// ── Anzeige ──────────────────────────────────────────────────────────────────

const (
	capKey = iota
	capMouse
	capWheel
)

// capOf beschreibt, wie eine Belegung gezeichnet wird: Art, Beschriftung und
// bei der Maus die Taste (1 links, 2 rechts, 3 Mitte, 4) bzw. beim Rad +1/-1.
func capOf(name string) (kind int, label string, part int) {
	switch {
	case strings.EqualFold(name, wheelUpName):
		return capWheel, tr("ctl.wheelUp"), 1
	case strings.EqualFold(name, wheelDownName):
		return capWheel, tr("ctl.wheelDown"), -1
	}
	if k, ok := findGameKey(name); ok {
		if k.mouse > 0 {
			return capMouse, tr([]string{"", "ctl.lmb", "ctl.rmb", "ctl.mmb", "ctl.mb4"}[k.mouse]), k.mouse
		}
		switch k.dik {
		case 0xC8:
			return capKey, "↑", 0
		case 0xD0:
			return capKey, "↓", 0
		case 0xCB:
			return capKey, "←", 0
		case 0xCD:
			return capKey, "→", 0
		}
		label := k.de
		if currentLang == "en" {
			label = k.en
		}
		return capKey, latin1ToUTF8(label), 0
	}
	if name == "" {
		return capKey, "–", 0
	}
	return capKey, strings.ToUpper(latin1ToUTF8(name)), 0
}

// latin1ToUTF8: die Namen des Spiels sind Latin-1 (z.B. "R\xfccktaste").
func latin1ToUTF8(s string) string {
	r := make([]rune, 0, len(s))
	for i := 0; i < len(s); i++ {
		r = append(r, rune(s[i]))
	}
	return string(r)
}

// ── Erfassen einer neuen Taste ───────────────────────────────────────────────

// Virtuelle Tasten, die beim Erfassen abgefragt werden: links/rechts
// getrennt (VK_LSHIFT statt VK_SHIFT), Maustasten 1-4.
var captureVKs = func() []int {
	v := []int{win.VK_LBUTTON, win.VK_RBUTTON, win.VK_MBUTTON, win.VK_XBUTTON1,
		win.VK_BACK, win.VK_TAB, win.VK_RETURN, win.VK_CAPITAL, win.VK_ESCAPE, win.VK_SPACE,
		win.VK_PRIOR, win.VK_NEXT, win.VK_END, win.VK_HOME, win.VK_LEFT, win.VK_UP, win.VK_RIGHT, win.VK_DOWN,
		win.VK_INSERT, win.VK_DELETE, win.VK_LSHIFT, win.VK_RSHIFT, win.VK_LCONTROL, win.VK_RCONTROL,
		win.VK_LMENU, win.VK_RMENU, win.VK_NUMLOCK, win.VK_MULTIPLY, win.VK_ADD, win.VK_SUBTRACT, win.VK_DIVIDE}
	for c := '0'; c <= '9'; c++ {
		v = append(v, int(c))
	}
	for c := 'A'; c <= 'Z'; c++ {
		v = append(v, int(c))
	}
	for i := 0; i < 10; i++ {
		v = append(v, win.VK_NUMPAD0+i, win.VK_F1+i)
	}
	v = append(v, win.VK_F11, win.VK_F12)
	for _, k := range []int{win.VK_OEM_1, win.VK_OEM_PLUS, win.VK_OEM_COMMA, win.VK_OEM_MINUS, win.VK_OEM_PERIOD,
		win.VK_OEM_2, win.VK_OEM_3, win.VK_OEM_4, win.VK_OEM_5, win.VK_OEM_6, win.VK_OEM_7, win.VK_OEM_102} {
		v = append(v, k)
	}
	return v
}()

var procMapVirtualKey = syscall.NewLazyDLL("user32.dll").NewProc("MapVirtualKeyW")

// vkDIK: Tasten, deren Scancode MapVirtualKey nicht als "erweitert" meldet.
var vkDIK = map[int]int{
	win.VK_PRIOR: 0xC9, win.VK_NEXT: 0xD1, win.VK_END: 0xCF, win.VK_HOME: 0xC7,
	win.VK_LEFT: 0xCB, win.VK_UP: 0xC8, win.VK_RIGHT: 0xCD, win.VK_DOWN: 0xD0,
	win.VK_INSERT: 0xD2, win.VK_DELETE: 0xD3, win.VK_RCONTROL: 0x9D, win.VK_RMENU: 0xB8,
	win.VK_DIVIDE: 0xB5, win.VK_NUMLOCK: 0x45,
}

// nameOfVK uebersetzt eine gedrueckte Taste in den Namen, den das Spiel in der
// config.cfg erwartet (Sprache lang). "" heisst: kennt das Spiel nicht.
func nameOfVK(vk int, lang string) string {
	pick := func(k gameKey) string {
		if lang == "en" {
			return k.en
		}
		return k.de
	}
	switch {
	case vk >= 'A' && vk <= 'Z':
		return strings.ToLower(string(rune(vk)))
	case vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= win.VK_F1 && vk <= win.VK_F10:
		return "F" + strconv.Itoa(vk-win.VK_F1+1)
	case vk == win.VK_F11, vk == win.VK_F12:
		return "" // dem Mausrad vorbehalten
	}
	for i, m := range []int{win.VK_LBUTTON, win.VK_RBUTTON, win.VK_MBUTTON, win.VK_XBUTTON1} {
		if vk == m {
			for _, k := range gameKeys {
				if k.mouse == i+1 {
					return pick(k)
				}
			}
		}
	}
	dik, ok := vkDIK[vk]
	if !ok {
		sc, _, _ := procMapVirtualKey.Call(uintptr(vk), 0) // MAPVK_VK_TO_VSC
		dik = int(sc)
	}
	for _, k := range gameKeys {
		if k.mouse == 0 && k.dik == dik {
			return pick(k)
		}
	}
	return ""
}

// ── Reiter ───────────────────────────────────────────────────────────────────

type controlsView struct {
	mw      *AppWindow
	caps    []*walk.CustomWidget
	labels  []*walk.Label
	capture int // Index der Aktion, die gerade eine Taste erwartet, sonst -1
	stop    chan struct{}
	hint    *walk.Label
	presets [2]*walk.PushButton
	preset  *walk.Label
	fCap    *walk.Font
	fSmall  *walk.Font
	glyphs  *glyphCache
	mu      sync.Mutex
}

func newControlsView(mw *AppWindow) *controlsView {
	c := &controlsView{mw: mw, capture: -1, caps: make([]*walk.CustomWidget, len(ctrlActions)),
		labels: make([]*walk.Label, len(ctrlActions))}
	c.fCap, _ = walk.NewFont(fontFamily, 9, walk.FontBold)
	c.fSmall, _ = walk.NewFont(fontFamily, 8, 0)
	c.glyphs = newGlyphCache()
	return c
}

func (c *controlsView) panel(assign **walk.Composite) Widget {
	half := (len(ctrlActions) + 1) / 2
	column := func(from, to int) Widget {
		var ch []Widget
		for i := from; i < to; i++ {
			i := i
			ch = append(ch,
				Label{AssignTo: &c.labels[i], Text: tr("ctl." + ctrlActions[i]), Font: fontBody},
				c.capWidget(i))
		}
		return Composite{Layout: Grid{Columns: 2, MarginsZero: true, Spacing: gapS}, Children: ch}
	}
	return Composite{
		AssignTo: assign,
		Visible:  false,
		Layout:   VBox{Margins: Margins{Left: gutter, Top: 12, Right: gutter, Bottom: 10}, Spacing: gapM},
		Children: []Widget{
			Composite{
				Background: SolidColorBrush{Color: colAccountBar},
				Layout:     HBox{Margins: Margins{Left: gapM, Top: 8, Right: gapM, Bottom: 8}, Spacing: gapS},
				Children: []Widget{
					Label{AssignTo: &c.preset, Text: tr("set.preset"), Font: fontBodyBold},
					PushButton{AssignTo: &c.presets[0], Text: tr("ctl.presetWASD"), Font: fontBody,
						MinSize: Size{Width: 130, Height: 26}, MaxSize: Size{Width: 130, Height: 26},
						OnClicked: func() { c.applyPreset(presetWASD) }},
					PushButton{AssignTo: &c.presets[1], Text: tr("ctl.presetClassic"), Font: fontBody,
						MinSize: Size{Width: 130, Height: 26}, MaxSize: Size{Width: 130, Height: 26},
						OnClicked: func() { c.applyPreset(presetClassic) }},
					HSpacer{},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 40, Alignment: AlignHNearVNear},
				Children: []Widget{
					column(0, half),
					column(half, len(ctrlActions)),
					HSpacer{},
				},
			},
			Label{AssignTo: &c.hint, Text: tr("ctl.hint"), Font: fontHint, TextColor: colMuted},
			VSpacer{},
		},
	}
}

const capWidth, capHeight = 170, 30

func (c *controlsView) capWidget(i int) Widget {
	return CustomWidget{
		AssignTo:            &c.caps[i],
		MinSize:             Size{Width: capWidth, Height: capHeight},
		MaxSize:             Size{Width: capWidth, Height: capHeight},
		DoubleBuffering:     true,
		InvalidatesOnResize: true,
		Paint:               func(cv *walk.Canvas, _ walk.Rectangle) error { return c.paintCap(cv, i) },
		OnMouseUp: func(x, y int, b walk.MouseButton) {
			if b == walk.LeftButton && c.capture != i {
				c.startCapture(i)
			}
		},
	}
}

// attach: Handzeiger und Mausrad, sobald die Widgets existieren.
func (c *controlsView) attach() {
	for i, w := range c.caps {
		i := i
		w.SetCursor(walk.CursorHand())
		w.MouseWheel().Attach(func(x, y int, b walk.MouseButton) {
			if c.capture != i {
				return
			}
			if walk.MouseWheelEventDelta(b) > 0 {
				c.assign(i, wheelUpName)
			} else {
				c.assign(i, wheelDownName)
			}
		})
	}
}

// paintCap zeichnet die Belegung einer Aktion: Tasten als Tastenkappe (so
// breit wie ihre Beschriftung), Maustasten und Mausrad als Maus mit der
// belegten Taste in Gelb, daneben der Name. Waehrend des Erfassens steht dort
// ein gelber Rahmen mit "Taste druecken". Kappen und Maus sind geglaettete
// Bilder (glyphs.go), die Schrift zeichnet GDI.
func (c *controlsView) paintCap(cv *walk.Canvas, i int) error {
	w := c.caps[i]
	b := w.ClientBoundsPixels()
	px := w.IntFrom96DPI
	cv.FillRectanglePixels(brushOf(colPanel()), b)
	dpi := w.DPI()
	if c.capture == i {
		cv.DrawImageStretchedPixels(c.glyphs.keycap(b.Width, b.Height, dpi, true), b)
		return cv.DrawTextPixels(tr("ctl.press"), c.fSmall, colAccentText, b, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	}
	kind, label, part := capOf(c.mw.cfg.Bindings[ctrlActions[i]])
	if kind == capKey {
		tw := textWidth(cv, label, c.fCap)
		kw := tw + px(22)
		if kw < b.Height {
			kw = b.Height // mindestens quadratisch
		}
		if kw > b.Width {
			kw = b.Width
		}
		r := walk.Rectangle{X: b.X, Y: b.Y, Width: kw, Height: b.Height}
		cv.DrawImageStretchedPixels(c.glyphs.keycap(r.Width, r.Height, dpi, false), r)
		r.Height -= px(3) // Schrift auf die Kappe, nicht auf deren Kante
		if dir := strings.Index("↑→↓←", label); dir >= 0 && len(label) == len("↑") {
			a := c.glyphs.arrow(dir/len("↑"), dpi, walk.RGB(255, 255, 255))
			s := a.Size()
			return cv.DrawImageStretchedPixels(a, walk.Rectangle{X: r.X + (r.Width-s.Width)/2, Y: r.Y + (r.Height-s.Height)/2, Width: s.Width, Height: s.Height})
		}
		return cv.DrawTextPixels(label, c.fCap, colHelpTerm, r, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix)
	}
	icon := c.glyphs.mouse(kind, part, dpi)
	s := icon.Size()
	ir := walk.Rectangle{X: b.X + px(4), Y: b.Y + (b.Height-s.Height)/2, Width: s.Width, Height: s.Height}
	cv.DrawImageStretchedPixels(icon, ir)
	t := walk.Rectangle{X: ir.X + ir.Width + px(8), Y: b.Y, Width: b.Width - (ir.Width + px(12)), Height: b.Height}
	return cv.DrawTextPixels(label, c.fCap, colHelpTerm, t, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
}

// startCapture wartet auf die naechste Taste, Maustaste oder Raddrehung. Die
// Tastatur wird reihum abgefragt statt ueber Nachrichten - so kommen auch
// Tab, Enter, Pfeile und Alt an, die walk sonst fuer die Dialognavigation
// verbraucht.
func (c *controlsView) startCapture(i int) {
	c.stopCapture()
	c.capture = i
	c.caps[i].SetFocus()
	c.invalidateAll()
	stop := make(chan struct{})
	c.mu.Lock()
	c.stop = stop
	c.mu.Unlock()
	go func() {
		down := map[int]bool{}
		for _, vk := range captureVKs {
			down[vk] = keyDown(vk)
		}
		t := time.NewTicker(15 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-c.mw.done:
				return
			case <-t.C:
			}
			for _, vk := range captureVKs {
				now := keyDown(vk)
				if now && !down[vk] {
					vk := vk
					c.mw.sync(func() { c.pressed(i, vk) })
					return
				}
				down[vk] = now
			}
		}
	}()
}

func (c *controlsView) stopCapture() {
	c.mu.Lock()
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
	c.mu.Unlock()
	if c.capture >= 0 {
		c.capture = -1
		c.invalidateAll()
	}
}

func (c *controlsView) pressed(i, vk int) {
	if c.capture != i {
		return
	}
	if vk == win.VK_ESCAPE {
		c.stopCapture()
		c.mw.setStatus(tr("ctl.canceled"))
		return
	}
	name := nameOfVK(vk, bindingsLang(c.mw.cfg.Bindings))
	if name == "" {
		c.stopCapture()
		c.mw.setStatus(tr("ctl.unsupported"))
		return
	}
	c.assign(i, name)
}

// assign belegt Aktion i mit name. Liegt name schon auf einer anderen Aktion,
// tauschen beide - das Spiel erlaubt jede Taste nur einmal.
func (c *controlsView) assign(i int, name string) {
	c.stopCapture()
	b := c.mw.cfg.Bindings
	if b == nil {
		b = map[string]string{}
		c.mw.cfg.Bindings = b
	}
	act := ctrlActions[i]
	old := b[act]
	for _, other := range ctrlActions {
		if other != act && strings.EqualFold(b[other], name) {
			b[other] = old
		}
	}
	b[act] = name
	c.save()
	c.invalidateAll()
}

func (c *controlsView) applyPreset(p map[string]string) {
	c.stopCapture()
	c.mw.cfg.Bindings = presetIn(p, bindingsLang(c.mw.cfg.Bindings))
	c.save()
	c.invalidateAll()
}

func (c *controlsView) save() {
	if err := saveBindings(&c.mw.cfg); err != nil {
		c.mw.setStatus(err.Error())
		return
	}
	c.mw.setStatus(tr("ctl.saved"))
}

func (c *controlsView) invalidateAll() {
	for _, w := range c.caps {
		if w != nil {
			w.Invalidate()
		}
	}
}

func (c *controlsView) retranslate() {
	for i, l := range c.labels {
		l.SetText(tr("ctl." + ctrlActions[i]))
	}
	c.preset.SetText(tr("set.preset"))
	c.presets[0].SetText(tr("ctl.presetWASD"))
	c.presets[1].SetText(tr("ctl.presetClassic"))
	c.hint.SetText(tr("ctl.hint"))
	c.invalidateAll()
}

var procGetAsyncKeyState = syscall.NewLazyDLL("user32.dll").NewProc("GetAsyncKeyState")

// keyDown: ist die virtuelle Taste vk gerade gedrueckt (hoechstes Bit)?
func keyDown(vk int) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}
