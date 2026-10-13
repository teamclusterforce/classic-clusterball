// Selbst gezeichnete Teile der Oberflaeche: die flachen Reiter in der dunklen
// Leiste und der gelbe Hauptknopf. walk zeichnet Buttons im Windows-Stil und
// laesst sich dort keine Farben geben, deshalb sind beide CustomWidgets.

package main

import (
	"strconv"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// Farben. Dunkelblau und Gelb wie die Menues des Spiels.
var (
	colHeader     = walk.RGB(18, 26, 40)
	colTabBar     = walk.RGB(28, 39, 58)
	colTabText    = walk.RGB(150, 165, 190)
	colTabActive  = walk.RGB(255, 255, 255)
	colAccent     = walk.RGB(245, 184, 0) // die gelbe Leiste im Hauptmenue
	colAccentDown = walk.RGB(214, 158, 0)
	colAccentText = walk.RGB(20, 24, 32)
	colMuted      = walk.RGB(120, 125, 135)
	colOK         = walk.RGB(34, 140, 64)
	colAccountBar = walk.RGB(232, 237, 246)
)

// colPanel ist der Hintergrund der Seiten (Systemfarbe der Dialoge).
func colPanel() walk.Color { return walk.Color(win.GetSysColor(win.COLOR_BTNFACE)) }

var brushCache = map[walk.Color]*walk.SolidColorBrush{}

func brushOf(c walk.Color) *walk.SolidColorBrush {
	if b, ok := brushCache[c]; ok {
		return b
	}
	b, _ := walk.NewSolidColorBrush(c)
	brushCache[c] = b
	return b
}

// tabItem ist ein Reiter der dunklen Leiste. Der Text kommt bei jedem
// Zeichnen aus tr(), ein Sprachwechsel braucht also nur Invalidate.
type tabItem struct {
	key     string
	active  func() bool
	onClick func()
	w       *walk.CustomWidget
}

func (mw *AppWindow) tabWidget(t *tabItem, width int) Widget {
	return CustomWidget{
		AssignTo:            &t.w,
		MinSize:             Size{Width: width, Height: tabBarHeight},
		MaxSize:             Size{Width: width, Height: tabBarHeight},
		PaintMode:           PaintBuffered, // DoubleBuffering wertet walk nicht aus; ohne das wird vor jedem Zeichnen geloescht (Flackern)
		InvalidatesOnResize: true,
		Paint: func(c *walk.Canvas, _ walk.Rectangle) error {
			b := t.w.ClientBounds()
			c.FillRectangle(brushOf(colTabBar), b)
			font, col := mw.fontTabInactive, colTabText
			if t.active() {
				font, col = mw.fontTabActive, colTabActive
				c.FillRectangle(brushOf(colAccent), walk.Rectangle{X: b.X + 10, Y: b.Y + b.Height - 3, Width: b.Width - 20, Height: 3})
			}
			return c.DrawText(tr(t.key), font, col, b, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		},
		OnMouseUp: func(x, y int, button walk.MouseButton) {
			if button == walk.LeftButton {
				t.onClick()
			}
		},
	}
}

// ctaButton ist der grosse gelbe Hauptknopf.
func (mw *AppWindow) ctaButton(assign **walk.CustomWidget, key string, height int, onClick func()) Widget {
	pressed := false
	return CustomWidget{
		AssignTo:            assign,
		MinSize:             Size{Height: height},
		MaxSize:             Size{Height: height},
		PaintMode:           PaintBuffered, // DoubleBuffering wertet walk nicht aus; ohne das wird vor jedem Zeichnen geloescht (Flackern)
		InvalidatesOnResize: true,
		Paint: func(c *walk.Canvas, _ walk.Rectangle) error {
			b := (*assign).ClientBounds()
			c.FillRectangle(brushOf(colPanel()), b)
			col := colAccent
			if pressed {
				col = colAccentDown
			}
			c.FillRoundedRectangle(brushOf(col), b, walk.Size{Width: 10, Height: 10})
			return c.DrawText(tr(key), mw.fontCTA, colAccentText, b, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		},
		OnMouseDown: func(x, y int, button walk.MouseButton) {
			if button == walk.LeftButton {
				pressed = true
				(*assign).Invalidate()
			}
		},
		OnMouseUp: func(x, y int, button walk.MouseButton) {
			if !pressed {
				return
			}
			pressed = false
			(*assign).Invalidate()
			onClick()
		},
	}
}

// langWidget ist der Sprachumschalter rechts in der Reiterleiste: "DE  EN",
// die aktive Sprache weiss und fett. Ein Klick auf die linke Haelfte waehlt
// Deutsch, auf die rechte Englisch.
func (mw *AppWindow) langWidget() Widget {
	const width = 76
	return CustomWidget{
		AssignTo:            &mw.langToggle,
		MinSize:             Size{Width: width, Height: tabBarHeight},
		MaxSize:             Size{Width: width, Height: tabBarHeight},
		PaintMode:           PaintBuffered, // DoubleBuffering wertet walk nicht aus; ohne das wird vor jedem Zeichnen geloescht (Flackern)
		InvalidatesOnResize: true,
		ToolTipText:         "Sprache / Language",
		Paint: func(c *walk.Canvas, _ walk.Rectangle) error {
			b := mw.langToggle.ClientBounds()
			c.FillRectangle(brushOf(colTabBar), b)
			half := b.Width / 2
			for i, l := range []string{"de", "en"} {
				r := walk.Rectangle{X: b.X + i*half, Y: b.Y, Width: half, Height: b.Height}
				font, col := mw.fontTabInactive, colTabText
				if currentLang == l {
					font, col = mw.fontTabActive, colTabActive
					c.FillRectangle(brushOf(colAccent), walk.Rectangle{X: r.X + 8, Y: r.Y + r.Height - 3, Width: r.Width - 16, Height: 3})
				}
				c.DrawText(strings.ToUpper(l), font, col, r, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
			}
			// feiner Trennstrich zwischen beiden
			return c.FillRectangle(brushOf(colTabText), walk.Rectangle{X: b.X + half, Y: b.Y + 13, Width: 1, Height: b.Height - 26})
		},
		OnMouseUp: func(x, y int, button walk.MouseButton) {
			if button != walk.LeftButton {
				return
			}
			if x < mw.langToggle.ClientBoundsPixels().Width/2 {
				mw.switchLanguage("de")
			} else {
				mw.switchLanguage("en")
			}
		},
	}
}

// ── Formular-Karten (Einstellungen und Steuerung) ────────────────────────────
//
// Beide Seiten bauen auf denselben Teilen auf, damit sie gleich aussehen:
// weisse Karten mit Ueberschrift und gelbem Strich, darin Zeilen fester Hoehe
// mit gleich breiter Beschriftungsspalte, darunter eine Fussleiste mit
// Hinweis links und Knoepfen rechts.

const (
	formRowHeight  = 30
	formLabelWidth = 180
	formCardPad    = 14
)

var (
	colCard     = walk.RGB(255, 255, 255) // Kartengrund; auch der Grund der Tastensymbole
	colCardEdge = walk.RGB(214, 220, 230)
)

// formRow: Beschriftung links (feste Breite), Bedienelement rechts, beides
// vertikal mittig in einer Zeile fester Hoehe.
func formRow(label **walk.Label, key, tip string, control Widget) Widget {
	return Composite{
		MinSize: Size{Height: formRowHeight},
		MaxSize: Size{Height: formRowHeight},
		Layout:  HBox{MarginsZero: true, Spacing: gapM, Alignment: AlignHNearVCenter},
		Children: []Widget{
			Label{AssignTo: label, Text: tr(key), Font: fontForm, ToolTipText: tip,
				MinSize: Size{Width: formLabelWidth}, MaxSize: Size{Width: formLabelWidth}},
			control,
			HSpacer{},
		},
	}
}

// formCard: weisse Karte mit 1 px Rand, Ueberschrift, gelbem Strich und
// den Zeilen darunter.
func formCard(title **walk.Label, key string, rows ...Widget) Widget {
	children := []Widget{
		Label{AssignTo: title, Text: tr(key), Font: fontFormHeading},
		Composite{
			Layout: HBox{MarginsZero: true, SpacingZero: true},
			Children: []Widget{
				Composite{
					MinSize:    Size{Width: 36, Height: 3},
					MaxSize:    Size{Width: 36, Height: 3},
					Layout:     HBox{MarginsZero: true},
					Background: SolidColorBrush{Color: colAccent},
				},
				HSpacer{},
			},
		},
		VSpacer{Size: gapS},
	}
	children = append(children, rows...)
	// Feste Hoehe: Tastensymbole sind CustomWidgets und melden immer "will
	// wachsen" - ohne Deckel wuerde die Karte auf die Hoehe der Nachbarspalte
	// gestreckt und ihre Zeilen auseinandergezogen.
	h := 2 + 20 + 20 + 3 + gapS + len(rows)*formRowHeight + 2
	return Composite{
		MinSize:    Size{Height: h},
		MaxSize:    Size{Height: h},
		Background: SolidColorBrush{Color: colCardEdge},
		Layout:     VBox{Margins: Margins{Left: 1, Top: 1, Right: 1, Bottom: 1}},
		Children: []Widget{
			Composite{
				Background: SolidColorBrush{Color: colCard},
				Layout: VBox{Margins: Margins{Left: formCardPad, Top: 10, Right: formCardPad, Bottom: 10},
					SpacingZero: true, Alignment: AlignHNearVNear},
				Children: children,
			},
		},
	}
}

// formColumn: Karten untereinander, oben ausgerichtet.
func formColumn(cards ...Widget) Widget {
	return Composite{
		Layout:   VBox{MarginsZero: true, Spacing: gapM, Alignment: AlignHNearVNear},
		Children: append(cards, VSpacer{}),
	}
}

// formFooter: Hinweise links (muted, kursiv), Knoepfe rechts.
func formFooter(hints []Widget, buttons ...Widget) Widget {
	children := []Widget{
		Composite{Layout: VBox{MarginsZero: true, SpacingZero: true, Alignment: AlignHNearVCenter}, Children: hints},
		HSpacer{},
	}
	children = append(children, buttons...)
	return Composite{
		MinSize:  Size{Height: 34},
		Layout:   HBox{MarginsZero: true, Spacing: gapS, Alignment: AlignHNearVCenter},
		Children: children,
	}
}

// formHint ist eine Hinweiszeile fuer formFooter.
func formHint(assign **walk.Label, key string) Widget {
	return Label{AssignTo: assign, Text: tr(key), Font: fontFormHint, TextColor: colMuted}
}

// presetBar: die Leiste mit den Voreinstellungen oben auf beiden Seiten.
func presetBar(label **walk.Label, buttons ...Widget) Widget {
	children := []Widget{Label{AssignTo: label, Text: tr("set.preset"), Font: fontFormBold}}
	children = append(children, buttons...)
	children = append(children, HSpacer{})
	return Composite{
		Background: SolidColorBrush{Color: colAccountBar},
		Layout:     HBox{Margins: Margins{Left: gapM, Top: 8, Right: gapM, Bottom: 8}, Spacing: gapS},
		Children:   children,
	}
}

// valueSlider: Regler lo … hi mit Zahl dahinter.
func valueSlider(assign **walk.Slider, label **walk.Label, lo, hi int, onChange func()) Widget {
	return Composite{
		Layout: HBox{MarginsZero: true, Spacing: gapS, Alignment: AlignHNearVCenter},
		Children: []Widget{
			Slider{
				AssignTo: assign,
				MinValue: lo,
				MaxValue: hi,
				MinSize:  Size{Width: 150},
				MaxSize:  Size{Width: 150},
				Tracking: true,
				OnValueChanged: func() {
					(*label).SetText(strconv.Itoa((*assign).Value()))
					if onChange != nil {
						onChange()
					}
				},
			},
			Label{AssignTo: label, Text: "0", Font: fontForm, MinSize: Size{Width: 28}},
		},
	}
}
