// Selbst gezeichnete Teile der Oberflaeche: die flachen Reiter in der dunklen
// Leiste und der gelbe Hauptknopf. walk zeichnet Buttons im Windows-Stil und
// laesst sich dort keine Farben geben, deshalb sind beide CustomWidgets.

package main

import (
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
		DoubleBuffering:     true,
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
		DoubleBuffering:     true,
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
