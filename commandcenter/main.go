package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

//go:embed logo_cb.png
var logoPNG []byte

const (
	appTitle   = "Clusterball Re-Enhanced"
	appVersion = "3.0"
)

// Weblinks. XDreamCC.ini fuehrte fuer die Registrierung noch register.php;
// hier bleibt die neuere Adresse aus dem bisherigen Launcher-Code.
const (
	registerURL = "http://www.clusterball.de/registrieren/"
	supportURL  = "http://www.clusterball.de/support.php"
	forceURL    = "https://www.clusterforce.net"
)

const (
	tabSpiel         = 0
	tabEinstellungen = 1
)

// Feste Hoehen der Kopfbereiche. Beide Composites werden auf exakt diese
// Hoehe genagelt, damit der Inhalt beim Tab-Wechsel nicht springt.
const (
	headerHeight = 76
	tabBarHeight = 40
)

// Logische Breite des Logos. Die PNG-Datei ist mit 706px absichtlich viel
// groesser - sie wird als High-DPI-Asset eingebunden (siehe loadLogo).
const logoWidth = 290

// Farbtiefe des Bildspeichers. Unter der Direct3D-Schicht nimmt das Spiel den
// OpenGL-Pfad, und dort findet es unter aktuellem Windows nur Modi mit 32 Bit -
// mit 16 Bit bricht es beim Start ab. Deshalb steht der Wert fest.
const gameDepth = 32

// Abstaende. Ein einziger Gutter fuer alle Aussenraender, damit nichts
// mehr an der Fensterkante klebt.
const (
	gutter = 14 // Aussenabstand zum Fensterrand
	gapM   = 10 // zwischen Bloecken
	gapS   = 6  // innerhalb eines Blocks
)

// Einheitliche Typo-Skala. Jedes Widget bekommt explizit eine dieser Fonts,
// damit nicht der System-Default (andere Familie/Groesse) dazwischenfunkt.
const fontFamily = "Segoe UI"

var (
	fontTitle    = Font{Family: fontFamily, PointSize: 14, Bold: true}
	fontSubtitle = Font{Family: fontFamily, PointSize: 8}
	fontCTA      = Font{Family: fontFamily, PointSize: 12, Bold: true}
	fontHeading  = Font{Family: fontFamily, PointSize: 9, Bold: true}
	fontBody     = Font{Family: fontFamily, PointSize: 9}
	fontBodyBold = Font{Family: fontFamily, PointSize: 9, Bold: true}
	fontHint     = Font{Family: fontFamily, PointSize: 8, Italic: true}
)

// Auswahllisten, deren Beschriftung uebersetzt wird, waehrend der gespeicherte
// Wert gleich bleibt. Die Werte sind die der clusterball_d3d.ini.
var (
	qualityValues = []int{0, 1, 2}
	displayValues = []bool{true, false} // Vollbild, Fenster
	resValues     = []string{"", "1280x720", "1600x900", "1920x1080", "2560x1440", "3840x2160"}
	msaaValues    = []int{1, 2, 4, 8}
	ssValues      = []int{1, 2}
	anisoValues   = []int{1, 2, 4, 8, 16}
	packValues    = []string{"", "hd", "hd16"}
)

func qualityLabels() []string { return []string{tr("set.qLow"), tr("set.qMid"), tr("set.qHigh")} }
func displayLabels() []string { return []string{tr("set.fullscreen"), tr("set.window")} }
func msaaLabels() []string    { return []string{tr("set.off"), "2×", "4×", "8×"} }
func ssLabels() []string      { return []string{tr("set.off"), "2×"} }
func anisoLabels() []string   { return []string{tr("set.off"), "2×", "4×", "8×", "16×"} }
func packLabels() []string {
	return []string{tr("set.texOrig"), tr("set.texHD4"), tr("set.texHD16")}
}

// resLabels beschriftet resValues; eine von Hand in die INI geschriebene
// Auflösung steht als zusätzlicher Eintrag am Ende.
func resLabels(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		if v == "" {
			out[i] = tr("set.resNative")
			continue
		}
		out[i] = strings.Replace(v, "x", " × ", 1)
	}
	return out
}

func indexOfBool(list []bool, v bool) int {
	for i, b := range list {
		if b == v {
			return i
		}
	}
	return 0
}

func indexOfStr(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return 0
}

func indexOfInt(list []int, v int) int {
	for i, n := range list {
		if n == v {
			return i
		}
	}
	return 0
}

// texts sammelt die Widgets, deren Beschriftung beim Sprachwechsel neu
// gesetzt wird. Getrennt vom Rest, damit beim Ergaenzen eines Textes sofort
// auffaellt, dass er auch hier eingetragen gehoert.
type texts struct {
	accPlayer   *walk.Label
	accPassword *walk.Label
	accHint     *walk.Label
	btnRegister *walk.PushButton

	btnStart     *walk.PushButton
	btnDedicated *walk.PushButton
	gbOwnIP      *walk.GroupBox
	btnDetectIP  *walk.PushButton

	lblTableHint *walk.Label

	setGraphics   *walk.Label
	setDisplay    *walk.Label
	setResolution *walk.Label
	setMSAA       *walk.Label
	setSS         *walk.Label
	setAniso      *walk.Label
	setTexPack    *walk.Label
	setSharpen    *walk.Label
	d3dHint       *walk.Label
	setGame       *walk.Label
	setQuality    *walk.Label
	setBrightness *walk.Label
	setMouse      *walk.Label
	setNetwork    *walk.Label
	setMaster     *walk.Label
	setLanguage   *walk.Label
	gfxHint       *walk.Label
	btnSave       *walk.PushButton

	sbBrand *walk.StatusBarItem
	sbForce *walk.StatusBarItem
}

type AppWindow struct {
	*walk.MainWindow
	cfg       AppConfig
	activeTab int

	// Panels
	panelSpiel         *walk.Composite
	panelEinstellungen *walk.Composite

	// Tab-Buttons
	btnTabSpiel         *walk.PushButton
	btnTabEinstellungen *walk.PushButton
	btnTabHilfe         *walk.PushButton
	fontTabActive       *walk.Font
	fontTabInactive     *walk.Font

	// Account (oben in Spiel-Tab)
	editAccount     *walk.LineEdit
	editAccPassword *walk.LineEdit

	// Serverliste
	hostListView *walk.TableView
	hostModel    *HostListModel
	lblServers   *walk.Label
	btnRefresh   *walk.PushButton
	refreshing   bool // nur vom UI-Thread angefasst
	lastHosts    int  // -1 = noch nicht geladen

	// Eigene IP
	editOwnIP     *walk.LineEdit
	chkExternalIP *walk.CheckBox

	// Einstellungen
	editServerIP *walk.LineEdit
	radioDeutsch *walk.RadioButton
	radioEnglish *walk.RadioButton

	// Direct3D-Schicht
	cmbDisplay *walk.ComboBox
	cmbRes     *walk.ComboBox
	resChoices []string // Werte hinter cmbRes, siehe resLabels
	cmbMSAA    *walk.ComboBox
	cmbSS      *walk.ComboBox
	cmbAniso   *walk.ComboBox
	cmbTexPack *walk.ComboBox
	sldSharpen *walk.Slider
	lblSharpen *walk.Label
	chkVSync   *walk.CheckBox
	chkFont    *walk.CheckBox
	chkAO      *walk.CheckBox

	// Spiel
	cmbQuality    *walk.ComboBox
	sldBrightness *walk.Slider
	lblBrightness *walk.Label
	sldMouse      *walk.Slider
	lblMouse      *walk.Label

	lblStatus *walk.StatusBarItem
	lblClock  *walk.StatusBarItem

	t texts

	beacon   *HostBeacon
	done     chan struct{} // beendet die Hintergrund-Goroutinen beim Schliessen
	doneOnce sync.Once
}

func main() {
	dedicated := flag.Bool("d", false, "Als Dedicated Host starten")
	game := flag.Bool("g", false, "Spiel direkt starten")
	flag.Parse()
	cfg := LoadConfig()
	setLang(cfg.Language)

	if *dedicated {
		beacon := launchDedicatedHost(&cfg)
		if beacon == nil {
			return
		}
		// Ohne Fenster gibt es nichts, was den Prozess am Leben haelt - ohne
		// dieses Warten waere der Heartbeat nach einem einzigen Paket vorbei.
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		beacon.Stop()
		return
	}
	if *game {
		launchCB(&cfg, false)
		return
	}
	runGUI(&cfg)
}

func runGUI(cfg *AppConfig) {
	mw := &AppWindow{cfg: *cfg, done: make(chan struct{}), lastHosts: -1}
	if err := mw.build(); err != nil {
		walk.MsgBox(nil, tr("dlg.error"), err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
	mw.Run()
}

// loadLogo bindet das Logo als High-DPI-Bitmap ein: logisch logoWidth breit,
// gezeichnet aber aus der vollen PNG-Aufloesung.
//
// Der Umweg ueber die DPI-Angabe ist kein Selbstzweck. Ein ImageView ist in
// jedem Modus ausser ImageViewModeIdeal vertikal "greedy" (imageview.go:235).
// Diese Eigenschaft faerbt auf das umgebende Composite ab, der Header bekommt
// dadurch im Root-Layout einen Anteil am freien Platz zugeteilt, wird an
// seiner MaxSize abgeschnitten - und walk verteilt den abgeschnittenen Rest
// als Rand um *alle* Elemente (boxlayout.go:456). Genau das hat den Inhalt
// beim Tab-Wechsel springen lassen. Mit Ideal-Modus ist der Header nicht mehr
// dehnbar und der freie Platz landet vollstaendig im Inhaltsbereich.
func loadLogo() *walk.Bitmap {
	img, err := png.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		return nil
	}
	w := img.Bounds().Dx()
	if w <= 0 {
		return nil
	}
	dpi := int(math.Round(float64(w) * 96.0 / float64(logoWidth)))
	bmp, err := walk.NewBitmapFromImageForDPI(img, dpi)
	if err != nil {
		return nil
	}
	return bmp
}

func (mw *AppWindow) build() error {
	logoBitmap := loadLogo()

	mw.hostModel = NewHostListModel()

	// Aktiver/inaktiver Tab: gleiche Groesse, nur Bold wechselt, damit die
	// Leiste beim Umschalten ruhig bleibt.
	mw.fontTabActive, _ = walk.NewFont(fontFamily, 9, walk.FontBold)
	mw.fontTabInactive, _ = walk.NewFont(fontFamily, 9, 0)

	err := MainWindow{
		AssignTo: &mw.MainWindow,
		Title:    appTitle,
		MinSize:  Size{Width: 760, Height: 620},
		Size:     Size{Width: 790, Height: 640},
		// VNear: falls doch einmal Platz uebrig bleibt, sammelt er sich unten
		// statt sich als Rand um jedes Element zu legen.
		Layout: VBox{MarginsZero: true, SpacingZero: true, Alignment: AlignHNearVNear},
		// Summe der Breiten muss unter die Fensterbreite passen, sonst wird
		// das letzte Feld abgeschnitten.
		StatusBarItems: []StatusBarItem{
			{AssignTo: &mw.lblStatus, Text: tr("st.ready"), Width: 275},
			{AssignTo: &mw.lblClock, Text: "", Width: 125},
			{AssignTo: &mw.t.sbBrand, Text: tr("sb.brand"), Width: 215},
			{AssignTo: &mw.t.sbForce, Text: tr("sb.force"), Width: 115,
				ToolTipText: forceURL,
				OnClicked:   func() { openURL(forceURL) }},
		},
		Children: []Widget{
			mw.headerBar(logoBitmap),
			mw.tabBar(),

			// Gemeinsamer Container fuer beide Panels: er ist das einzige
			// dehnbare Kind des Root-Layouts, also bleiben Header und
			// Tab-Leiste beim Umschalten exakt an ihrem Platz.
			Composite{
				Layout: VBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					mw.spielPanel(),
					mw.einstellungenPanel(),
				},
			},
		},
	}.Create()

	if err != nil {
		return err
	}

	// Hintergrund-Goroutinen sauber beenden statt auf ein zerstoertes
	// Fenster-Handle zu pollen. sync.Once, weil Closing bei einem
	// abgebrochenen Schliessen erneut feuern kann.
	mw.MainWindow.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		mw.doneOnce.Do(func() { close(mw.done) })
		if mw.beacon != nil {
			mw.beacon.Stop()
		}
	})

	mw.applyConfig()
	mw.showTab(tabSpiel)

	// Der Launcher gehoert neben die Clusterball.exe. Liegt sie dort nicht,
	// gibt es nichts einzustellen - nur den Hinweis, wohin er gehoert.
	if !mw.cbExeOK() {
		walk.MsgBox(mw.MainWindow, tr("dlg.error"), fmt.Sprintf(tr("dlg.notFound"), mw.cfg.CBExePath),
			walk.MsgBoxIconWarning)
	}

	mw.refreshHostList()
	go mw.updateClock()
	go mw.autoRefreshHosts()
	return nil
}

// ── Kopfbereich ───────────────────────────────────────────────────────────────

func (mw *AppWindow) headerBar(logo *walk.Bitmap) Widget {
	return Composite{
		MinSize:    Size{Height: headerHeight},
		MaxSize:    Size{Height: headerHeight},
		Background: SolidColorBrush{Color: walk.RGB(22, 32, 48)},
		Layout:     HBox{Margins: Margins{Left: gutter, Top: 8, Right: gutter, Bottom: 8}},
		Children: []Widget{
			// Ideal-Modus: feste Groesse, nicht dehnbar - siehe loadLogo.
			ImageView{
				Image: logo,
				Mode:  ImageViewModeIdeal,
			},
			HSpacer{},
			Composite{
				Layout: VBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					// Neben dem Clusterball-Logo ergibt das den Namen
					// "Clusterball Re-Enhanced".
					Label{
						Text:      "Re-Enhanced",
						TextColor: walk.RGB(255, 200, 0),
						Font:      fontTitle,
					},
					Label{
						Text:      "Command Center · v" + appVersion + " · clusterball.de",
						TextColor: walk.RGB(130, 160, 200),
						Font:      fontSubtitle,
					},
				},
			},
		},
	}
}

func (mw *AppWindow) tabBar() Widget {
	return Composite{
		MinSize:    Size{Height: tabBarHeight},
		MaxSize:    Size{Height: tabBarHeight},
		Background: SolidColorBrush{Color: walk.RGB(245, 246, 248)},
		Layout:     HBox{Margins: Margins{Left: gutter, Top: 4, Right: gutter, Bottom: 4}, Spacing: gapS},
		Children: []Widget{
			PushButton{
				AssignTo:  &mw.btnTabSpiel,
				Text:      tr("tab.game"),
				Font:      fontBodyBold,
				MinSize:   Size{Width: 130, Height: 30},
				MaxSize:   Size{Width: 130, Height: 30},
				OnClicked: func() { mw.showTab(tabSpiel) },
			},
			PushButton{
				AssignTo:  &mw.btnTabEinstellungen,
				Text:      tr("tab.settings"),
				Font:      fontBody,
				MinSize:   Size{Width: 150, Height: 30},
				MaxSize:   Size{Width: 150, Height: 30},
				OnClicked: func() { mw.showTab(tabEinstellungen) },
			},
			// Kein Tab, sondern ein Link: oeffnet die Support-Seite im
			// Browser und nimmt deshalb nicht am Fontwechsel von showTab teil.
			PushButton{
				AssignTo:    &mw.btnTabHilfe,
				Text:        tr("tab.help"),
				Font:        fontBody,
				MinSize:     Size{Width: 100, Height: 30},
				MaxSize:     Size{Width: 100, Height: 30},
				ToolTipText: supportURL,
				OnClicked:   func() { openURL(supportURL) },
			},
			HSpacer{},
		},
	}
}

// ── Tab: Spiel ────────────────────────────────────────────────────────────────

func (mw *AppWindow) spielPanel() Widget {
	return Composite{
		AssignTo: &mw.panelSpiel,
		Layout:   VBox{Margins: Margins{Left: gutter, Top: 12, Right: gutter, Bottom: 10}, Spacing: gapM},
		Children: []Widget{
			mw.accountRow(),
			Composite{
				// HNear + Uebergewicht auf der Serverliste: die Aktionsspalte
				// hat feste Breite, ihre Buttons melden dem Layout aber
				// trotzdem Platzbedarf. Ohne das bekaeme sie beim Verbreitern
				// des Fensters einen Anteil zugeteilt, den sie nicht nutzen
				// kann - und der Rest landet als Luecke links von ihr.
				Layout: HBox{MarginsZero: true, Spacing: gapM, Alignment: AlignHNearVNear},
				Children: []Widget{
					mw.actionColumn(),
					mw.serverList(),
				},
			},
		},
	}
}

func (mw *AppWindow) accountRow() Widget {
	return Composite{
		Background: SolidColorBrush{Color: walk.RGB(235, 240, 250)},
		Layout:     HBox{Margins: Margins{Left: gapM, Top: gapS, Right: gapM, Bottom: gapS}, Spacing: gapS},
		Children: []Widget{
			Label{
				Text:      "\U0001f464",
				Font:      fontHeading,
				TextColor: walk.RGB(60, 100, 180),
			},
			Label{AssignTo: &mw.t.accPlayer, Text: tr("acc.player"), Font: fontBodyBold},
			LineEdit{
				AssignTo:  &mw.editAccount,
				Text:      mw.cfg.AccountName,
				Font:      fontBody,
				MinSize:   Size{Width: 150},
				MaxSize:   Size{Width: 150},
				CueBanner: tr("acc.playerCue"),
			},
			Label{AssignTo: &mw.t.accPassword, Text: tr("acc.password"), Font: fontBodyBold},
			LineEdit{
				AssignTo:     &mw.editAccPassword,
				Text:         mw.cfg.AccountPassword,
				Font:         fontBody,
				MinSize:      Size{Width: 130},
				MaxSize:      Size{Width: 130},
				PasswordMode: true,
				CueBanner:    tr("acc.passwordCue"),
			},
			// Feste Breiten oben sorgen dafuer, dass der Button hier links
			// stehen bleibt statt von wachsenden Feldern an den Rand
			// geschoben zu werden.
			PushButton{
				AssignTo:    &mw.t.btnRegister,
				Text:        tr("acc.register"),
				Font:        fontBodyBold,
				MinSize:     Size{Width: 145, Height: 26},
				MaxSize:     Size{Width: 145, Height: 26},
				ToolTipText: tr("acc.registerTip"),
				OnClicked:   func() { openURL(registerURL) },
			},
			HSpacer{},
			Label{
				AssignTo:  &mw.t.accHint,
				Text:      tr("acc.hint"),
				Font:      fontHint,
				TextColor: walk.RGB(120, 125, 135),
			},
		},
	}
}

func (mw *AppWindow) actionColumn() Widget {
	return Composite{
		MinSize: Size{Width: 175},
		MaxSize: Size{Width: 175},
		Layout:  VBox{MarginsZero: true, Spacing: gapS},
		Children: []Widget{
			PushButton{
				AssignTo:  &mw.t.btnStart,
				Text:      tr("act.start"),
				Font:      fontCTA,
				MinSize:   Size{Height: 46},
				MaxSize:   Size{Height: 46},
				OnClicked: func() { mw.startGame() },
			},
			PushButton{
				AssignTo:  &mw.t.btnDedicated,
				Text:      tr("act.dedicated"),
				Font:      fontBody,
				MinSize:   Size{Height: 32},
				MaxSize:   Size{Height: 32},
				OnClicked: func() { mw.startDedicated() },
			},
			VSpacer{Size: gapS},
			// Nur noch die eigene Adresse. Ein Feld fuer die Host-IP gab es
			// hier frueher auch - der Master-Server liest die Host-Adresse
			// aber ohnehin aus dem Absender des UDP-Heartbeats, das Feld war
			// also wirkungslos.
			GroupBox{
				AssignTo: &mw.t.gbOwnIP,
				Title:    tr("act.ownIP"),
				Font:     fontBody,
				Layout:   VBox{Margins: Margins{Left: gapS, Top: gapS, Right: gapS, Bottom: gapS}, Spacing: gapS},
				Children: []Widget{
					Composite{
						Layout: HBox{MarginsZero: true, SpacingZero: true},
						Children: []Widget{
							LineEdit{AssignTo: &mw.editOwnIP, Text: mw.cfg.OwnIP, Font: fontBody},
							PushButton{
								AssignTo:    &mw.t.btnDetectIP,
								Text:        "↻",
								Font:        fontBody,
								MinSize:     Size{Width: 24, Height: 22},
								MaxSize:     Size{Width: 24, Height: 22},
								ToolTipText: tr("act.detectTip"),
								OnClicked:   func() { mw.detectExternalIP() },
							},
						},
					},
					CheckBox{
						AssignTo: &mw.chkExternalIP,
						Text:     tr("act.external"),
						Font:     fontBody,
						OnClicked: func() {
							mw.cfg.UseExternalIP = mw.chkExternalIP.Checked()
							if mw.chkExternalIP.Checked() {
								mw.detectExternalIP()
							}
						},
					},
				},
			},
			VSpacer{},
		},
	}
}

func (mw *AppWindow) serverList() Widget {
	return Composite{
		StretchFactor: 20, // zusaetzliche Fensterbreite geht in die Tabelle
		Layout:        VBox{MarginsZero: true, Spacing: gapS},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: gapS},
				Children: []Widget{
					Label{
						AssignTo: &mw.lblServers,
						Text:     tr("srv.title"),
						Font:     fontHeading,
					},
					HSpacer{},
					PushButton{
						AssignTo:  &mw.btnRefresh,
						Text:      tr("srv.refresh"),
						Font:      fontBody,
						MinSize:   Size{Width: 125, Height: 24},
						MaxSize:   Size{Width: 125, Height: 24},
						OnClicked: func() { mw.refreshHostList() },
					},
				},
			},
			TableView{
				AssignTo: &mw.hostListView,
				Font:     fontBody,
				Columns: []TableViewColumn{
					{Title: tr("srv.colName"), Width: 190},
					{Title: tr("srv.colPlayers"), Width: 60},
					{Title: tr("srv.colMap"), Width: 105},
					{Title: tr("srv.colType"), Width: 75},
				},
				LastColumnStretched: true,
				AlternatingRowBG:    true,
				Model:               mw.hostModel,
				OnItemActivated:     func() { mw.joinSelectedServer() },
			},
			Label{
				AssignTo:  &mw.t.lblTableHint,
				Text:      tr("srv.hint"),
				Font:      fontHint,
				TextColor: walk.RGB(120, 125, 135),
			},
		},
	}
}

// ── Tab: Einstellungen ────────────────────────────────────────────────────────

func (mw *AppWindow) einstellungenPanel() Widget {
	// combo baut eine Auswahlliste fester Breite, linksbuendig in ihrer Zelle.
	combo := func(assign **walk.ComboBox, model []string, tip string) Widget {
		return Composite{
			Layout: HBox{MarginsZero: true, SpacingZero: true},
			Children: []Widget{
				ComboBox{AssignTo: assign, Model: model, Font: fontBody, ToolTipText: tip,
					MinSize: Size{Width: 150}, MaxSize: Size{Width: 150}},
				HSpacer{},
			},
		}
	}
	// slider baut einen Regler 0 … max mit Zahlenanzeige.
	slider := func(assign **walk.Slider, label **walk.Label, lo, hi int) Widget {
		return Composite{
			Layout: HBox{MarginsZero: true, Spacing: gapS},
			Children: []Widget{
				Slider{
					AssignTo: assign,
					MinValue: lo,
					MaxValue: hi,
					MinSize:  Size{Width: 150},
					MaxSize:  Size{Width: 150},
					Tracking: true,
					OnValueChanged: func() {
						(*label).SetText(fmt.Sprintf("%d", (*assign).Value()))
					},
				},
				Label{AssignTo: label, Text: "0", Font: fontBody, MinSize: Size{Width: 28}},
				HSpacer{},
			},
		}
	}
	hint := func(assign **walk.Label, key string) Widget {
		return Label{AssignTo: assign, Text: tr(key), Font: fontHint, TextColor: walk.RGB(120, 125, 135)}
	}

	return Composite{
		AssignTo: &mw.panelEinstellungen,
		Visible:  false,
		Layout:   VBox{Margins: Margins{Left: gutter, Top: 12, Right: gutter, Bottom: 10}, Spacing: gapM},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 24},
				Children: []Widget{

					// Linke Spalte: Direct3D-Schicht. HNear, sonst zentriert walk
					// jede Zeile einzeln in der Spaltenbreite - Checkboxen stehen
					// dann versetzt zueinander statt bündig.
					Composite{
						Layout: VBox{MarginsZero: true, Spacing: gapS, Alignment: AlignHNearVNear},
						Children: []Widget{
							// Alles hier landet in der clusterball_d3d.ini,
							// siehe writeD3DSettings.
							Label{AssignTo: &mw.t.setGraphics, Text: tr("set.graphics"), Font: fontHeading},
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true, Spacing: gapS},
								Children: []Widget{
									Label{AssignTo: &mw.t.setDisplay, Text: tr("set.display"), Font: fontBody},
									combo(&mw.cmbDisplay, displayLabels(), ""),
									Label{AssignTo: &mw.t.setResolution, Text: tr("set.resolution"), Font: fontBody,
										ToolTipText: tr("set.resolutionTip")},
									combo(&mw.cmbRes, resLabels(resValues), tr("set.resolutionTip")),
									Label{AssignTo: &mw.t.setMSAA, Text: tr("set.msaa"), Font: fontBody},
									combo(&mw.cmbMSAA, msaaLabels(), ""),
									Label{AssignTo: &mw.t.setSS, Text: tr("set.ss"), Font: fontBody,
										ToolTipText: tr("set.ssTip")},
									combo(&mw.cmbSS, ssLabels(), tr("set.ssTip")),
									Label{AssignTo: &mw.t.setAniso, Text: tr("set.aniso"), Font: fontBody},
									combo(&mw.cmbAniso, anisoLabels(), ""),
									Label{AssignTo: &mw.t.setTexPack, Text: tr("set.texPack"), Font: fontBody,
										ToolTipText: tr("set.texPackTip")},
									combo(&mw.cmbTexPack, packLabels(), tr("set.texPackTip")),
									Label{AssignTo: &mw.t.setSharpen, Text: tr("set.sharpen"), Font: fontBody},
									slider(&mw.sldSharpen, &mw.lblSharpen, 0, 100),
								},
							},
							CheckBox{AssignTo: &mw.chkVSync, Text: tr("set.vsync"), Font: fontBody},
							CheckBox{AssignTo: &mw.chkFont, Text: tr("set.font"), Font: fontBody,
								ToolTipText: tr("set.fontTip")},
							CheckBox{AssignTo: &mw.chkAO, Text: tr("set.ao"), Font: fontBody,
								ToolTipText: tr("set.aoTip")},
							hint(&mw.t.d3dHint, "set.d3dHint"),
						},
					},

					// Rechte Spalte: Spiel, Netzwerk, Sprache
					Composite{
						Layout: VBox{MarginsZero: true, Spacing: gapS, Alignment: AlignHNearVNear},
						Children: []Widget{
							// Detailstufe und Helligkeit stehen in der config.cfg
							// des Spiels, die Maus in der clusterball_d3d.ini.
							Label{AssignTo: &mw.t.setGame, Text: tr("set.game"), Font: fontHeading},
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true, Spacing: gapS},
								Children: []Widget{
									Label{AssignTo: &mw.t.setQuality, Text: tr("set.quality"), Font: fontBody},
									combo(&mw.cmbQuality, qualityLabels(), ""),
									Label{AssignTo: &mw.t.setBrightness, Text: tr("set.brightness"), Font: fontBody},
									// Gamma des Renderers in Prozent: die Helligkeit des Spiels
									// selbst wirkt unter Direct3D nicht, siehe D3DSettings.Gamma.
									slider(&mw.sldBrightness, &mw.lblBrightness, 50, 150),
									Label{AssignTo: &mw.t.setMouse, Text: tr("set.mouse"), Font: fontBody,
										ToolTipText: tr("set.mouseTip")},
									slider(&mw.sldMouse, &mw.lblMouse, 5, 100),
								},
							},
							hint(&mw.t.gfxHint, "set.gfxHint"),

							VSpacer{Size: gapS},
							Label{AssignTo: &mw.t.setNetwork, Text: tr("set.network"), Font: fontHeading},
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true, Spacing: gapS},
								Children: []Widget{
									Label{AssignTo: &mw.t.setMaster, Text: tr("set.master"), Font: fontBody},
									LineEdit{AssignTo: &mw.editServerIP, Text: mw.cfg.ServerIP, Font: fontBody,
										MinSize: Size{Width: 150}, MaxSize: Size{Width: 150}},
								},
							},
							VSpacer{Size: gapS},
							Label{AssignTo: &mw.t.setLanguage, Text: tr("set.language"), Font: fontHeading},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 16},
								Children: []Widget{
									// Sprachwechsel greift sofort, ohne Speichern
									// und ohne Neustart.
									RadioButton{AssignTo: &mw.radioDeutsch, Text: "Deutsch", Font: fontBody, Value: "de",
										OnClicked: func() { mw.switchLanguage("de") }},
									RadioButton{AssignTo: &mw.radioEnglish, Text: "English", Font: fontBody, Value: "en",
										OnClicked: func() { mw.switchLanguage("en") }},
									HSpacer{},
								},
							},
						},
					},
				},
			},

			VSpacer{},

			Composite{
				Layout: HBox{MarginsZero: true, Spacing: gapS},
				Children: []Widget{
					HSpacer{},
					// Der Pfad gehoert in den Tooltip: als Label wuerde seine
					// Textbreite dem ganzen Fenster die Mindestbreite diktieren.
					PushButton{
						AssignTo:    &mw.t.btnSave,
						Text:        tr("set.save"),
						Font:        fontBodyBold,
						MinSize:     Size{Width: 200, Height: 32},
						MaxSize:     Size{Width: 200, Height: 32},
						ToolTipText: fmt.Sprintf(tr("set.saveTip"), configPath()),
						OnClicked:   func() { mw.saveSettings() },
					},
				},
			},
		},
	}
}

// ── Sprache ───────────────────────────────────────────────────────────────────

// switchLanguage stellt die Oberflaeche sofort um. Die Beschriftungen werden
// im laufenden Fenster ersetzt - kein Neustart, keine ungespeicherten
// Eingaben gehen verloren.
func (mw *AppWindow) switchLanguage(lang string) {
	if lang == mw.cfg.Language {
		return
	}
	mw.cfg.Language = lang
	setLang(lang)
	mw.retranslate()

	// Nur die Sprache wandert sofort auf die Platte. Alles andere im
	// Einstellungen-Tab bleibt bis zum Speichern-Button ungespeichert.
	if err := SaveConfig(&mw.cfg); err != nil {
		mw.setStatus(fmt.Sprintf(tr("st.cfgFailed"), err))
	}
}

// retranslate setzt jede Beschriftung neu. Wer hier einen Eintrag vergisst,
// sieht ihn nach dem Umschalten in der alten Sprache stehen.
func (mw *AppWindow) retranslate() {
	mw.SetSuspended(true)
	defer mw.SetSuspended(false)

	mw.btnTabSpiel.SetText(tr("tab.game"))
	mw.btnTabEinstellungen.SetText(tr("tab.settings"))
	mw.btnTabHilfe.SetText(tr("tab.help"))

	mw.t.accPlayer.SetText(tr("acc.player"))
	mw.t.accPassword.SetText(tr("acc.password"))
	mw.t.accHint.SetText(tr("acc.hint"))
	mw.t.btnRegister.SetText(tr("acc.register"))
	mw.t.btnRegister.SetToolTipText(tr("acc.registerTip"))
	mw.editAccount.SetCueBanner(tr("acc.playerCue"))
	mw.editAccPassword.SetCueBanner(tr("acc.passwordCue"))

	mw.t.btnStart.SetText(tr("act.start"))
	mw.t.btnDedicated.SetText(tr("act.dedicated"))
	mw.t.gbOwnIP.SetTitle(tr("act.ownIP"))
	mw.t.btnDetectIP.SetToolTipText(tr("act.detectTip"))
	mw.chkExternalIP.SetText(tr("act.external"))

	mw.btnRefresh.SetText(tr("srv.refresh"))
	mw.t.lblTableHint.SetText(tr("srv.hint"))
	mw.updateServerLabel()

	cols := mw.hostListView.Columns()
	titles := []string{tr("srv.colName"), tr("srv.colPlayers"), tr("srv.colMap"), tr("srv.colType")}
	for i := 0; i < cols.Len() && i < len(titles); i++ {
		cols.At(i).SetTitle(titles[i])
	}
	// Die Typ-Spalte enthaelt uebersetzten Text ("Dedicated"), also neu zeichnen.
	mw.hostModel.PublishRowsReset()

	mw.t.setGraphics.SetText(tr("set.graphics"))
	mw.t.setDisplay.SetText(tr("set.display"))
	mw.t.setResolution.SetText(tr("set.resolution"))
	mw.t.setResolution.SetToolTipText(tr("set.resolutionTip"))
	mw.cmbRes.SetToolTipText(tr("set.resolutionTip"))
	mw.t.setMSAA.SetText(tr("set.msaa"))
	mw.t.setSS.SetText(tr("set.ss"))
	mw.t.setSS.SetToolTipText(tr("set.ssTip"))
	mw.cmbSS.SetToolTipText(tr("set.ssTip"))
	mw.t.setAniso.SetText(tr("set.aniso"))
	mw.t.setTexPack.SetText(tr("set.texPack"))
	mw.t.setTexPack.SetToolTipText(tr("set.texPackTip"))
	mw.cmbTexPack.SetToolTipText(tr("set.texPackTip"))
	mw.t.setSharpen.SetText(tr("set.sharpen"))
	mw.chkVSync.SetText(tr("set.vsync"))
	mw.chkFont.SetText(tr("set.font"))
	mw.chkFont.SetToolTipText(tr("set.fontTip"))
	mw.chkAO.SetText(tr("set.ao"))
	mw.chkAO.SetToolTipText(tr("set.aoTip"))
	mw.t.d3dHint.SetText(tr("set.d3dHint"))

	mw.t.setGame.SetText(tr("set.game"))
	mw.t.setQuality.SetText(tr("set.quality"))
	mw.t.setBrightness.SetText(tr("set.brightness"))
	mw.t.setMouse.SetText(tr("set.mouse"))
	mw.t.setMouse.SetToolTipText(tr("set.mouseTip"))
	mw.t.gfxHint.SetText(tr("set.gfxHint"))

	// Auswahllisten neu beschriften. SetModel setzt die Auswahl zurueck,
	// deshalb Index merken und wiederherstellen.
	refill := func(cb *walk.ComboBox, labels []string) {
		i := cb.CurrentIndex()
		cb.SetModel(labels)
		cb.SetCurrentIndex(i)
	}
	refill(mw.cmbDisplay, displayLabels())
	refill(mw.cmbRes, resLabels(mw.resChoices))
	refill(mw.cmbMSAA, msaaLabels())
	refill(mw.cmbSS, ssLabels())
	refill(mw.cmbAniso, anisoLabels())
	refill(mw.cmbTexPack, packLabels())
	refill(mw.cmbQuality, qualityLabels())

	mw.t.setNetwork.SetText(tr("set.network"))
	mw.t.setMaster.SetText(tr("set.master"))
	mw.t.setLanguage.SetText(tr("set.language"))
	mw.t.btnSave.SetText(tr("set.save"))
	mw.t.btnSave.SetToolTipText(fmt.Sprintf(tr("set.saveTip"), configPath()))

	mw.t.sbBrand.SetText(tr("sb.brand"))
	mw.t.sbForce.SetText(tr("sb.force"))
	mw.updateServerStatus()
}

// updateServerLabel schreibt die Ueberschrift der Serverliste passend zum
// zuletzt geladenen Stand - auch nach einem Sprachwechsel.
func (mw *AppWindow) updateServerLabel() {
	if mw.lblServers == nil {
		return
	}
	switch {
	case mw.lastHosts < 0:
		mw.lblServers.SetText(tr("srv.title"))
	case mw.lastHosts == 0:
		mw.lblServers.SetText(tr("srv.titleNone"))
	default:
		mw.lblServers.SetText(fmt.Sprintf(tr("srv.titleN"), mw.lastHosts))
	}
}

// updateServerStatus leitet die Statusmeldung aus dem letzten Ladestand ab,
// statt sie nur beim Laden zu setzen - so ueberlebt sie den Sprachwechsel.
func (mw *AppWindow) updateServerStatus() {
	switch {
	case mw.lastHosts < 0:
		mw.setStatus(tr("st.ready"))
	case mw.lastHosts == 0:
		mw.setStatus(tr("st.noServers"))
	case mw.lastHosts == 1:
		mw.setStatus(tr("st.nServers1"))
	default:
		mw.setStatus(fmt.Sprintf(tr("st.nServers"), mw.lastHosts))
	}
}

// ── Tab-Umschaltung ───────────────────────────────────────────────────────────

func (mw *AppWindow) showTab(tab int) {
	mw.activeTab = tab

	// SetSuspended klammert Ein- und Ausblenden zu einem einzigen
	// Layout-Durchgang, sonst flackert es.
	mw.MainWindow.SetSuspended(true)
	defer mw.MainWindow.SetSuspended(false)

	mw.panelSpiel.SetVisible(tab == tabSpiel)
	mw.panelEinstellungen.SetVisible(tab == tabEinstellungen)

	if mw.fontTabActive != nil && mw.fontTabInactive != nil {
		if tab == tabSpiel {
			mw.btnTabSpiel.SetFont(mw.fontTabActive)
			mw.btnTabEinstellungen.SetFont(mw.fontTabInactive)
		} else {
			mw.btnTabSpiel.SetFont(mw.fontTabInactive)
			mw.btnTabEinstellungen.SetFont(mw.fontTabActive)
		}
	}
}

// ── Konfiguration ↔ Oberflaeche ───────────────────────────────────────────────

func (mw *AppWindow) applyConfig() {
	setText(mw.editServerIP, mw.cfg.ServerIP)
	setText(mw.editOwnIP, mw.cfg.OwnIP)
	setText(mw.editAccount, mw.cfg.AccountName)
	setText(mw.editAccPassword, mw.cfg.AccountPassword)

	if mw.chkExternalIP != nil {
		mw.chkExternalIP.SetChecked(mw.cfg.UseExternalIP)
	}
	// SetChecked loest kein OnClicked aus, der Sprachwechsel wird hier also
	// nicht versehentlich angestossen.
	if mw.radioDeutsch != nil && mw.radioEnglish != nil {
		if mw.cfg.Language == "en" {
			mw.radioEnglish.SetChecked(true)
		} else {
			mw.radioDeutsch.SetChecked(true)
		}
	}

	// Direct3D-Schicht
	if mw.cmbDisplay != nil {
		d := mw.cfg.D3D
		mw.cmbDisplay.SetCurrentIndex(indexOfBool(displayValues, d.Fullscreen))
		// Eine von Hand eingetragene Auflösung bleibt auswählbar.
		mw.resChoices = append([]string(nil), resValues...)
		if indexOfStr(mw.resChoices, d.Resolution) == 0 && d.Resolution != "" {
			mw.resChoices = append(mw.resChoices, d.Resolution)
		}
		mw.cmbRes.SetModel(resLabels(mw.resChoices))
		mw.cmbRes.SetCurrentIndex(indexOfStr(mw.resChoices, d.Resolution))
		mw.cmbMSAA.SetCurrentIndex(indexOfInt(msaaValues, d.MSAA))
		mw.cmbSS.SetCurrentIndex(indexOfInt(ssValues, d.SuperSample))
		mw.cmbAniso.SetCurrentIndex(indexOfInt(anisoValues, d.Anisotropy))
		mw.cmbTexPack.SetCurrentIndex(indexOfStr(packValues, d.TexturePack))
		mw.sldSharpen.SetValue(clampInt(d.Sharpen, 0, 100))
		mw.lblSharpen.SetText(fmt.Sprintf("%d", mw.sldSharpen.Value()))
		mw.chkVSync.SetChecked(d.VSync)
		mw.chkFont.SetChecked(d.FontReplace)
		mw.chkAO.SetChecked(d.AmbientOcc)
		mw.sldMouse.SetValue(clampInt(int(d.MouseScale*100+0.5), 5, 100))
		mw.lblMouse.SetText(fmt.Sprintf("%d", mw.sldMouse.Value()))
	}

	// Spiel
	if mw.cmbQuality != nil {
		mw.cmbQuality.SetCurrentIndex(indexOfInt(qualityValues, mw.cfg.GraphicalQuality))
		mw.sldBrightness.SetValue(clampInt(mw.cfg.D3D.Gamma, 50, 150))
		mw.lblBrightness.SetText(fmt.Sprintf("%d", mw.sldBrightness.Value()))
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func setText(le *walk.LineEdit, s string) {
	if le != nil {
		le.SetText(s)
	}
}

func text(le *walk.LineEdit) string {
	if le == nil {
		return ""
	}
	return le.Text()
}

// syncSpiel liest Account + Host-IP aus dem Spiel-Tab.
func (mw *AppWindow) syncSpiel() {
	mw.cfg.AccountName = text(mw.editAccount)
	mw.cfg.AccountPassword = text(mw.editAccPassword)
	mw.cfg.OwnIP = text(mw.editOwnIP)
	if mw.chkExternalIP != nil {
		mw.cfg.UseExternalIP = mw.chkExternalIP.Checked()
	}
}

// readSettings uebernimmt alles, was in der Oberflaeche eingestellt ist, in
// die Konfiguration. Der Spielstart ruft es ebenfalls auf: wer im
// Einstellungen-Tab etwas aendert und sofort startet, bekommt genau das, auch
// ohne vorher zu speichern.
func (mw *AppWindow) readSettings() {
	mw.cfg.ServerIP = text(mw.editServerIP)

	// Direct3D-Schicht
	if mw.cmbDisplay != nil {
		d := &mw.cfg.D3D
		pick := func(cb *walk.ComboBox, n int) (int, bool) {
			i := cb.CurrentIndex()
			return i, i >= 0 && i < n
		}
		if i, ok := pick(mw.cmbDisplay, len(displayValues)); ok {
			d.Fullscreen = displayValues[i]
		}
		if i, ok := pick(mw.cmbRes, len(mw.resChoices)); ok {
			d.Resolution = mw.resChoices[i]
		}
		if i, ok := pick(mw.cmbMSAA, len(msaaValues)); ok {
			d.MSAA = msaaValues[i]
		}
		if i, ok := pick(mw.cmbSS, len(ssValues)); ok {
			d.SuperSample = ssValues[i]
		}
		if i, ok := pick(mw.cmbAniso, len(anisoValues)); ok {
			d.Anisotropy = anisoValues[i]
		}
		if i, ok := pick(mw.cmbTexPack, len(packValues)); ok {
			d.TexturePack = packValues[i]
		}
		d.Sharpen = mw.sldSharpen.Value()
		d.VSync = mw.chkVSync.Checked()
		d.FontReplace = mw.chkFont.Checked()
		d.AmbientOcc = mw.chkAO.Checked()
		d.MouseScale = float64(mw.sldMouse.Value()) / 100
	}

	// Spiel. Die Farbtiefe steht fest und wird nicht abgefragt.
	mw.cfg.Depth = gameDepth
	if mw.cmbQuality != nil {
		if i := mw.cmbQuality.CurrentIndex(); i >= 0 && i < len(qualityValues) {
			mw.cfg.GraphicalQuality = qualityValues[i]
		}
		mw.cfg.D3D.Gamma = mw.sldBrightness.Value()
	}

	mw.cfg.ServerMode = "arena"
	if mw.radioEnglish != nil && mw.radioEnglish.Checked() {
		mw.cfg.Language = "en"
	} else {
		mw.cfg.Language = "de"
	}
	mw.syncSpiel()
}

func (mw *AppWindow) saveSettings() {
	mw.readSettings()
	if err := SaveConfig(&mw.cfg); err != nil {
		walk.MsgBox(mw.MainWindow, tr("dlg.error"), err.Error(), walk.MsgBoxIconError)
		return
	}
	// Die Grafikwerte gehoeren in die config.cfg des Spiels - sonst bleibt die
	// Einstellung wieder folgenlos.
	if err := writeGameSettings(&mw.cfg); err != nil {
		walk.MsgBox(mw.MainWindow, tr("dlg.error"), fmt.Sprintf(tr("dlg.gameCfg"), err), walk.MsgBoxIconWarning)
	}
	// Ebenso die Einstellungen der Direct3D-Schicht in deren INI.
	if err := writeD3DSettings(&mw.cfg); err != nil {
		walk.MsgBox(mw.MainWindow, tr("dlg.error"), fmt.Sprintf(tr("dlg.d3dCfg"), err), walk.MsgBoxIconWarning)
	}
	// Werte zurueckschreiben, damit korrigierte Eingaben sofort sichtbar
	// wieder auf dem gueltigen Stand stehen.
	mw.applyConfig()
	mw.setStatus(tr("st.saved"))
}

func (mw *AppWindow) cbExeOK() bool {
	if mw.cfg.CBExePath == "" {
		return false
	}
	_, err := os.Stat(mw.cfg.CBExePath)
	return err == nil
}

// ── Aktionen ──────────────────────────────────────────────────────────────────

func (mw *AppWindow) startGame() {
	mw.readSettings()
	if err := SaveConfig(&mw.cfg); err != nil {
		mw.setStatus(fmt.Sprintf(tr("st.cfgFailed"), err))
	}
	mw.setStatus(tr("st.starting"))
	go launchCB(&mw.cfg, false)
}

func (mw *AppWindow) startDedicated() {
	mw.readSettings()
	if err := SaveConfig(&mw.cfg); err != nil {
		mw.setStatus(fmt.Sprintf(tr("st.cfgFailed"), err))
	}
	mw.setStatus(tr("st.dediStart"))
	go func() {
		beacon := launchDedicatedHost(&mw.cfg)
		mw.sync(func() {
			mw.beacon = beacon
			if beacon == nil {
				return
			}
			mw.setStatus(tr("st.dediRunning"))
		})
	}()
}

func (mw *AppWindow) refreshHostList() {
	if mw.refreshing {
		return // laeuft schon - kein zweiter Request obendrauf
	}
	mw.refreshing = true
	if mw.btnRefresh != nil {
		mw.btnRefresh.SetEnabled(false)
	}
	mw.setStatus(tr("st.loading"))

	serverIP := mw.cfg.ServerIP
	go func() {
		hosts, err := NewMasterClient(serverIP).GetHostList()
		mw.sync(func() {
			mw.refreshing = false
			if mw.btnRefresh != nil {
				mw.btnRefresh.SetEnabled(true)
			}
			if err != nil {
				mw.setStatus(fmt.Sprintf(tr("st.listError"), err))
				return
			}
			mw.hostModel.setHosts(hosts)
			mw.lastHosts = len(hosts)
			mw.updateServerLabel()
			mw.updateServerStatus()
		})
	}()
}

func (mw *AppWindow) joinSelectedServer() {
	idx := mw.hostListView.CurrentIndex()
	host, ok := mw.hostModel.at(idx)
	if !ok {
		return
	}
	mw.readSettings()
	// Die Hostliste liefert nicht bei jedem Eintrag eine Adresse. Wenn eine
	// drinsteht, wird sie als zuletzt gewaehlter Host vermerkt - mehr kann
	// damit derzeit nicht passieren, weil das Spiel beim Start keinen
	// Zielhost als Argument entgegennimmt (siehe launchCB).
	if ip := strings.TrimSpace(host.IP); net.ParseIP(ip) != nil {
		mw.cfg.HostIP = ip
	}
	mw.setStatus(fmt.Sprintf(tr("st.joining"), host.Name))
	go launchCB(&mw.cfg, false)
}

func (mw *AppWindow) detectExternalIP() {
	mw.setStatus(tr("st.ipDetect"))
	go func() {
		ip, err := fetchExternalIP()
		mw.sync(func() {
			if err != nil {
				mw.setStatus(fmt.Sprintf(tr("st.ipFailed"), err))
				return
			}
			setText(mw.editOwnIP, ip)
			mw.cfg.OwnIP = ip
			mw.setStatus(fmt.Sprintf(tr("st.ipOK"), ip))
		})
	}()
}

func (mw *AppWindow) setStatus(msg string) {
	if mw.lblStatus != nil {
		mw.lblStatus.SetText(msg)
	}
}

// sync fuehrt fn im UI-Thread aus, aber nur solange das Fenster noch lebt.
func (mw *AppWindow) sync(fn func()) {
	select {
	case <-mw.done:
		return
	default:
	}
	mw.MainWindow.Synchronize(fn)
}

// ── Hintergrund ───────────────────────────────────────────────────────────────

func (mw *AppWindow) autoRefreshHosts() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-mw.done:
			return
		case <-ticker.C:
			mw.sync(func() { mw.refreshHostList() })
		}
	}
}

// updateClock schreibt in ein eigenes Statusleisten-Feld. Frueher lief das
// auf demselben Feld wie die Statusmeldungen und hat jede Meldung nach
// spaetestens einer Sekunde wieder ueberschrieben.
func (mw *AppWindow) updateClock() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-mw.done:
			return
		case now := <-ticker.C:
			secs := now.Hour()*3600 + now.Minute()*60 + now.Second()
			beats := float64(secs) / 86.4
			txt := fmt.Sprintf("%s · @%06.2f", now.Format("02.01.06"), beats)
			mw.sync(func() {
				if mw.lblClock != nil {
					mw.lblClock.SetText(txt)
				}
			})
		}
	}
}

func fetchExternalIP() (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(data))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("unerwartete Antwort: %q", ip)
	}
	return ip, nil
}

// openURL oeffnet eine URL im Standardbrowser. rundll32 statt "cmd /c start",
// weil start das erste Argument in Anfuehrungszeichen als Fenstertitel
// missversteht und an & in der URL scheitert.
func openURL(url string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// ── Spielstart ────────────────────────────────────────────────────────────────

func launchDedicatedHost(cfg *AppConfig) *HostBeacon {
	if dir := cbExeDir(cfg); dir != "" {
		updateDediCfg(filepath.Join(dir, "dedi1.cfg"), cfg)
	}
	launchCB(cfg, true)

	beacon := NewHostBeacon("Egypt", true)
	if err := beacon.Start(); err != nil {
		walk.MsgBox(nil, tr("dlg.heartbeat"), err.Error(), walk.MsgBoxIconWarning)
		return nil
	}
	return beacon
}

func updateDediCfg(dediPath string, cfg *AppConfig) {
	data, err := os.ReadFile(dediPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.SplitN(strings.TrimSpace(line), " ", 2)[0] == "network_speed" {
			if cfg.UseExternalIP {
				lines[i] = "network_speed 1.000000"
			} else {
				lines[i] = "network_speed 2.000000"
			}
		}
	}
	os.WriteFile(dediPath, []byte(strings.Join(lines, "\n")), 0644)
}

func launchCB(cfg *AppConfig, dedicated bool) {
	exePath := cfg.CBExePath
	if exePath == "" {
		walk.MsgBox(nil, tr("dlg.error"), tr("dlg.noPath"), walk.MsgBoxIconError)
		return
	}
	if _, err := os.Stat(exePath); err != nil {
		walk.MsgBox(nil, tr("dlg.error"), fmt.Sprintf(tr("dlg.notFound"), exePath), walk.MsgBoxIconError)
		return
	}

	// Zugangsdaten und Grafikwerte in die config.cfg des Spiels uebertragen.
	if err := writeGameSettings(cfg); err != nil {
		walk.MsgBox(nil, tr("dlg.error"), fmt.Sprintf(tr("dlg.gameCfg"), err), walk.MsgBoxIconWarning)
	}
	// Direct3D-Schicht, ihre Einstellungen und der Schrift-Ersatz. Ohne
	// Schicht startet das Spiel nicht - dann lieber gar nicht.
	fontErr, err := prepareGame(cfg)
	if err != nil {
		walk.MsgBox(nil, tr("dlg.error"), fmt.Sprintf(tr("dlg.layer"), err), walk.MsgBoxIconError)
		return
	}
	if fontErr != nil {
		walk.MsgBox(nil, tr("dlg.error"), fmt.Sprintf(tr("dlg.font"), fontErr), walk.MsgBoxIconWarning)
	}

	args := []string{"/host"}
	if dedicated {
		args = []string{"/dedicatedserver", "dedi1.cfg"}
	}
	cmd := exec.Command(exePath, args...)
	cmd.Dir = filepath.Dir(exePath)
	if err := cmd.Start(); err != nil {
		walk.MsgBox(nil, tr("dlg.error"), err.Error(), walk.MsgBoxIconError)
	}
}

// ── HostListModel ─────────────────────────────────────────────────────────────

type HostListModel struct {
	walk.TableModelBase
	hosts []HostEntry
}

func NewHostListModel() *HostListModel { return &HostListModel{} }
func (m *HostListModel) RowCount() int { return len(m.hosts) }

func (m *HostListModel) at(row int) (HostEntry, bool) {
	if row < 0 || row >= len(m.hosts) {
		return HostEntry{}, false
	}
	return m.hosts[row], true
}

func (m *HostListModel) Value(row, col int) interface{} {
	h, ok := m.at(row)
	if !ok {
		return ""
	}
	switch col {
	case 0:
		return h.Name
	case 1:
		return h.PlayersStr
	case 2:
		return h.Map
	case 3:
		if h.HostType == "D" {
			if h.GameType != "" {
				return "Dedi · " + h.GameType
			}
			return tr("srv.dedicated")
		}
		return h.GameType
	}
	return ""
}

func (m *HostListModel) setHosts(hosts []HostEntry) {
	m.hosts = hosts
	m.PublishRowsReset()
}
