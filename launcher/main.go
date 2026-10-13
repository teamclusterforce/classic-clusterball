package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	xdraw "golang.org/x/image/draw"
)

// Alle aus assets/cmd/launcherlogo.
//
//go:embed logo_cb.png
var logoPNG []byte

//go:embed logo_cf.png
var logoCFPNG []byte

//go:embed logo_cbde.png
var logoCBDEPNG []byte

const (
	appTitle   = "Clusterball Re-Enhanced"
	appVersion = "3.0"
)

// Weblinks. XDreamCC.ini fuehrte fuer die Registrierung noch register.php;
// hier bleibt die neuere Adresse aus dem bisherigen Launcher-Code.
const (
	registerURL = "http://www.clusterball.de/registrieren/"
	forceURL    = "https://www.clusterforce.net"
)

const (
	tabSpiel         = 0
	tabEinstellungen = 1
	tabHilfe         = 2
	tabUeber         = 3
	tabSteuerung     = 4
)

// Feste Hoehen der Kopfbereiche. Beide Composites werden auf exakt diese
// Hoehe genagelt, damit der Inhalt beim Tab-Wechsel nicht springt.
const (
	headerHeight = 80
	tabBarHeight = 38
)

// Logische Hoehe der Logos in der Kopfleiste. Die PNG-Dateien sind 240 px
// hoch - sie werden als High-DPI-Assets eingebunden (siehe loadLogo).
const (
	logoHeight     = 56
	logoCFHeight   = 40
	logoCBDEHeight = 40
)

const siteURL = "http://www.clusterball.de"

// Farbtiefe des Bildspeichers. Unter der Direct3D-Schicht nimmt das Spiel den
// OpenGL-Pfad, und dort findet es unter aktuellem Windows nur Modi mit 32 Bit -
// mit 16 Bit bricht es beim Start ab. Deshalb steht der Wert fest.
const gameDepth = 32

// Detailstufe des Spiels (graphicalQuality in dessen config.cfg). Das Spiel
// kennt fuenf Stufen, 0 = sehr niedrig bis 4 = sehr hoch (Local.mrg, Texte
// 147-151); 3 ist "Hoch". Sie steht fest und wird bei jedem Start gesetzt -
// frueher schrieb der Launcher 2, im Spiel "Mittel".
const gameQuality = 3

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
	fontCTA      = Font{Family: fontFamily, PointSize: 12, Bold: true}
	fontHeading  = Font{Family: fontFamily, PointSize: 9, Bold: true}
	fontBody     = Font{Family: fontFamily, PointSize: 9}
	fontBodyBold = Font{Family: fontFamily, PointSize: 9, Bold: true}
	fontHint     = Font{Family: fontFamily, PointSize: 8, Italic: true}

	// Einstellungen und Steuerung: dort ist Platz, die Schrift eine Stufe groesser.
	fontForm        = Font{Family: fontFamily, PointSize: 10}
	fontFormBold    = Font{Family: fontFamily, PointSize: 10, Bold: true}
	fontFormHeading = Font{Family: fontFamily, PointSize: 11, Bold: true}
	fontFormHint    = Font{Family: fontFamily, PointSize: 9, Italic: true}
)

// Auswahllisten, deren Beschriftung uebersetzt wird, waehrend der gespeicherte
// Wert gleich bleibt. Die Werte sind die der clusterball_d3d.ini.
var (
	displayValues = []bool{true, false} // Vollbild, Fenster
	resValues     = []string{"", "1280x720", "1600x900", "1920x1080", "2560x1440", "3840x2160"}
	msaaValues    = []int{1, 2, 4, 8}
	ssValues      = []int{1, 2}
	anisoValues   = []int{1, 2, 4, 8, 16}
	packValues    = []string{"", "hd"}
)

func displayLabels() []string { return []string{tr("set.fullscreen"), tr("set.window")} }
func msaaLabels() []string    { return []string{tr("set.off"), "2×", "4×", "8×"} }
func ssLabels() []string      { return []string{tr("set.off"), "2×"} }
func anisoLabels() []string   { return []string{tr("set.off"), "2×", "4×", "8×", "16×"} }
func packLabels() []string {
	return []string{tr("set.texOrig"), tr("set.texHD4")}
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

	btnStart     *walk.CustomWidget
	btnDedicated *walk.PushButton
	setOwnIP     *walk.Label
	btnDetectIP  *walk.PushButton

	lblTableHint *walk.Label
	emptyTitle   *walk.Label
	emptyHint    *walk.Label
	gfxTitle     *walk.Label
	gfxLink      *walk.LinkLabel

	setPreset   *walk.Label
	btnPresets  [4]*walk.PushButton
	btnDefaults *walk.PushButton

	setGraphics   *walk.Label
	setDisplay    *walk.Label
	setResolution *walk.Label
	setMSAA       *walk.Label
	setSS         *walk.Label
	setAniso      *walk.Label
	setTexPack    *walk.Label
	setSharpen    *walk.Label
	setVSync      *walk.Label
	setFontRep    *walk.Label
	setAO         *walk.Label
	setExternal   *walk.Label
	setSound      *walk.Label
	setMusic      *walk.Label
	setBall       *walk.Label
	btnPlayBall   *walk.PushButton
	d3dHint       *walk.Label
	setGame       *walk.Label
	setBrightness *walk.Label
	setMouse      *walk.Label
	setNetwork    *walk.Label
	gfxHint       *walk.Label
	btnSave       *walk.PushButton

	sbBrand *walk.StatusBarItem
}

type AppWindow struct {
	*walk.MainWindow
	cfg       AppConfig
	activeTab int

	// Panels
	panelSpiel         *walk.Composite
	panelEinstellungen *walk.Composite
	panelHilfe         *walk.Composite
	panelUeber         *walk.Composite
	panelSteuerung     *walk.Composite
	controls           *controlsView
	glider             *gliderView
	help               *helpView
	about              *helpView // Reiter "Über", gleiche Darstellung ohne Kapitelleiste

	// Reiter (ui.go) und Schriften der selbst gezeichneten Teile
	tabs            []*tabItem
	fontTabActive   *walk.Font
	fontTabInactive *walk.Font
	fontCTA         *walk.Font
	langToggle      *walk.CustomWidget // DE | EN rechts in der Reiterleiste

	imgCF         *walk.ImageView // Team ClusterForce, verlinkt
	imgCBDE       *walk.ImageView // clusterball.de, verlinkt
	emptyState    *walk.Composite // statt der Tabelle, solange kein Server online ist
	lblGfxSummary *walk.Label     // Grafik-Kurzuebersicht im Spiel-Reiter

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

	// Direct3D-Schicht
	cmbDisplay *walk.ComboBox
	cmbRes     *walk.ComboBox
	resChoices []string // Werte hinter cmbRes, siehe resLabels
	cmbMSAA    *walk.ComboBox
	cmbSS      *walk.ComboBox
	cmbAniso   *walk.ComboBox
	cmbTexPack *walk.ComboBox
	cmbMusic   *walk.ComboBox // Musik: Original oder ClusterForce (cfaudio.go)
	cmbBall    *walk.ComboBox // Ball-Sound, Werte ballSoundValues
	sldSharpen *walk.Slider
	lblSharpen *walk.Label
	chkVSync   *walk.CheckBox
	chkFont    *walk.CheckBox
	chkAO      *walk.CheckBox

	// Spiel
	sldBrightness *walk.Slider
	lblBrightness *walk.Label
	sldMouse      *walk.Slider
	lblMouse      *walk.Label

	lblStatus *walk.StatusBarItem

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
	installDocs(cbExeDir(&cfg)) // Readme, Lizenz, Notice (docs.go)
	runGUI(&cfg)
}

func runGUI(cfg *AppConfig) {
	// Abstuerze landen in ClusterballCC_crash.log neben dem Launcher - ohne
	// Konsole waere ein Go-Panic sonst spurlos. Bleibt die Datei leer, wird
	// sie beim Beenden wieder entfernt.
	crashPath := filepath.Join(appDir(), "ClusterballCC_crash.log")
	if f, err := os.OpenFile(crashPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		debug.SetCrashOutput(f, debug.CrashOptions{})
		defer func() {
			// Der Runtime gehoert eine eigene Kopie des Handles; solange sie offen
			// ist, laesst Windows die Datei nicht loeschen.
			debug.SetCrashOutput(nil, debug.CrashOptions{})
			f.Close()
			if st, err := os.Stat(crashPath); err == nil && st.Size() == 0 {
				os.Remove(crashPath)
			}
		}()
	}
	mw := &AppWindow{cfg: *cfg, done: make(chan struct{}), lastHosts: -1}
	if err := mw.build(); err != nil {
		walk.MsgBox(nil, tr("dlg.error"), err.Error(), walk.MsgBoxIconError)
		return
	}
	centerOnScreen(mw.MainWindow)
	mw.Show()
	mw.Run()
}

// centerOnScreen setzt das Fenster mittig in den Arbeitsbereich seines
// Bildschirms (ohne Taskleiste) - Windows legt neue Fenster sonst gestaffelt
// irgendwo an den Rand.
func centerOnScreen(w *walk.MainWindow) {
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if !win.GetMonitorInfo(win.MonitorFromWindow(w.Handle(), win.MONITOR_DEFAULTTONEAREST), &mi) {
		return
	}
	wa := mi.RcWork
	b := w.BoundsPixels()
	b.X = int(wa.Left) + (int(wa.Right-wa.Left)-b.Width)/2
	b.Y = int(wa.Top) + (int(wa.Bottom-wa.Top)-b.Height)/2
	b.X, b.Y = max(b.X, int(wa.Left)), max(b.Y, int(wa.Top))
	w.SetBoundsPixels(b)
}

// loadLogo bindet ein Logo als High-DPI-Bitmap ein: logisch height hoch,
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
//
// Das PNG ist rund fuenfmal so gross wie die Anzeige. Diese Verkleinerung
// uebernimmt nicht Windows beim Zeichnen (ohne Glaettung, das Logo wurde
// pixelig), sondern loadLogo vorab mit einem Catmull-Rom-Filter, genau auf
// die Pixelgroesse fuer die DPI des Bildschirms.
func loadLogo(data []byte, height int) *walk.Bitmap {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	if b.Dy() <= 0 {
		return nil
	}
	dpi := screenDPI()
	th := int(math.Round(float64(height) * float64(dpi) / 96))
	tw := int(math.Round(float64(th) * float64(b.Dx()) / float64(b.Dy())))
	small := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(small, small.Bounds(), img, b, xdraw.Src, nil)
	bmp, err := walk.NewBitmapFromImageForDPI(small, dpi)
	if err != nil {
		return nil
	}
	return bmp
}

// screenDPI ist die DPI des Bildschirms (96 = 100 %).
func screenDPI() int {
	hdc := win.GetDC(0)
	defer win.ReleaseDC(0, hdc)
	if d := int(win.GetDeviceCaps(hdc, win.LOGPIXELSX)); d > 0 {
		return d
	}
	return 96
}

func (mw *AppWindow) build() error {
	logoBitmap := loadLogo(logoPNG, logoHeight)
	logoCF := loadLogo(logoCFPNG, logoCFHeight)
	logoCBDE := loadLogo(logoCBDEPNG, logoCBDEHeight)

	mw.hostModel = NewHostListModel()
	mw.help = newHelpView(mw, helpDE, helpEN, true)
	mw.about = newHelpView(mw, aboutDE, aboutEN, false)
	mw.controls = newControlsView(mw)
	mw.glider = &gliderView{mw: mw}

	// Aktiver/inaktiver Reiter: gleiche Groesse, nur Bold wechselt, damit die
	// Leiste beim Umschalten ruhig bleibt.
	mw.fontTabActive, _ = walk.NewFont(fontFamily, 10, walk.FontBold)
	mw.fontTabInactive, _ = walk.NewFont(fontFamily, 10, 0)
	mw.fontCTA, _ = walk.NewFont(fontFamily, 13, walk.FontBold)
	mw.tabs = []*tabItem{
		{key: "tab.game", active: func() bool { return mw.activeTab == tabSpiel }, onClick: func() { mw.showTab(tabSpiel) }},
		{key: "tab.settings", active: func() bool { return mw.activeTab == tabEinstellungen },
			onClick: func() { mw.showTab(tabEinstellungen) }},
		{key: "tab.controls", active: func() bool { return mw.activeTab == tabSteuerung },
			onClick: func() { mw.showTab(tabSteuerung) }},
		// Das Handbuch im Launcher, siehe help.go.
		{key: "tab.help", active: func() bool { return mw.activeTab == tabHilfe }, onClick: func() { mw.showTab(tabHilfe) }},
		{key: "tab.about", active: func() bool { return mw.activeTab == tabUeber }, onClick: func() { mw.showTab(tabUeber) }},
	}

	err := MainWindow{
		AssignTo: &mw.MainWindow,
		Title:    appTitle,
		Visible:  false, // erst nach dem Zentrieren zeigen, siehe runGUI
		MinSize:  Size{Width: 820, Height: 680},
		Size:     Size{Width: 980, Height: 700}, // quer, nicht fast quadratisch
		// VNear: falls doch einmal Platz uebrig bleibt, sammelt er sich unten
		// statt sich als Rand um jedes Element zu legen.
		Layout: VBox{MarginsZero: true, SpacingZero: true, Alignment: AlignHNearVNear},
		// Summe der Breiten muss unter die Fensterbreite passen, sonst wird
		// das letzte Feld abgeschnitten.
		StatusBarItems: []StatusBarItem{
			{AssignTo: &mw.lblStatus, Text: tr("st.ready"), Width: 580},
			{Text: "v" + appVersion, Width: 70},
			{AssignTo: &mw.t.sbBrand, Text: tr("sb.brand"), Width: 150,
				ToolTipText: siteURL,
				OnClicked:   func() { openURL(siteURL) }},
		},
		Children: []Widget{
			mw.headerBar(logoBitmap, logoCBDE, logoCF),
			mw.tabBar(),

			// Gemeinsamer Container fuer die Panels: er ist das einzige
			// dehnbare Kind des Root-Layouts, also bleiben Header und
			// Tab-Leiste beim Umschalten exakt an ihrem Platz.
			// Die selbst gezeichneten Reiter melden walk immer "will wachsen"
			// (CustomWidget ist greedy). Damit die Reiterleiste davon keinen
			// Anteil am freien Platz bekommt - walk verteilt den an ihrer
			// MaxSize abgeschnittenen Rest sonst als Luecken -, geht er nach
			// Gewicht fast ganz an diesen Container.
			Composite{
				StretchFactor: 10000,
				Layout:        VBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					mw.spielPanel(),
					mw.einstellungenPanel(),
					mw.help.panel(&mw.panelHilfe),
					mw.about.panel(&mw.panelUeber),
					mw.controls.panel(&mw.panelSteuerung),
				},
			},
		},
	}.Create()

	if err != nil {
		return err
	}
	// Programm-Icon fuer Titelleiste und Taskleiste. walk sucht die Icon-Gruppe
	// unter der ID 7 (aeltere rsrc-Versionen); rsrc v0.10 vergibt nach dem
	// Manifest (1) die ID 2.
	if ic, err := walk.NewIconFromResourceId(2); err == nil {
		mw.MainWindow.SetIcon(ic)
	}
	// Klickbare Teile zeigen die Hand.
	for _, t := range mw.tabs {
		t.w.SetCursor(walk.CursorHand())
	}
	mw.t.btnStart.SetCursor(walk.CursorHand())
	for _, img := range []*walk.ImageView{mw.imgCF, mw.imgCBDE} {
		if img != nil {
			img.SetCursor(walk.CursorHand())
		}
	}
	mw.langToggle.SetCursor(walk.CursorHand())
	mw.help.attach()
	mw.about.attach()
	mw.controls.attach()
	mw.glider.run(cbExeDir(&mw.cfg))

	// Die Statusleiste zeichnet beim Breiterziehen nur den neuen Streifen; der
	// Bereich rechts vom letzten Feld behielt so jeden alten Groessengriff
	// (eine Spur aus Punkten). Bei jeder Groessenaenderung ganz neu zeichnen.
	mw.MainWindow.SizeChanged().Attach(func() {
		if sb := mw.MainWindow.StatusBar(); sb != nil {
			sb.Invalidate()
		}
	})

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
	mw.controls.ready = true
	mw.showTab(tabSpiel)

	// Der Launcher gehoert neben die Clusterball.exe. Liegt sie dort nicht,
	// gibt es nichts einzustellen - nur den Hinweis, wohin er gehoert.
	if !mw.cbExeOK() {
		walk.MsgBox(mw.MainWindow, tr("dlg.error"), fmt.Sprintf(tr("dlg.notFound"), mw.cfg.CBExePath),
			walk.MsgBoxIconWarning)
	}

	mw.refreshHostList()
	go mw.autoRefreshHosts()
	return nil
}

// ── Kopfbereich ───────────────────────────────────────────────────────────────

// Kopfleiste: links das Clusterball-Logo (mit "RE-ENHANCED" darunter, wie im
// Spiel), rechts die Community-Seite clusterball.de und Team ClusterForce,
// beide verlinkt und durch einen feinen Strich getrennt.
func (mw *AppWindow) headerBar(logo, logoCBDE, logoCF *walk.Bitmap) Widget {
	return Composite{
		MinSize:    Size{Height: headerHeight},
		MaxSize:    Size{Height: headerHeight},
		Background: SolidColorBrush{Color: colHeader},
		Layout:     HBox{Margins: Margins{Left: gutter, Top: 10, Right: gutter + 4, Bottom: 10}},
		Children: []Widget{
			// Ideal-Modus: feste Groesse, nicht dehnbar - siehe loadLogo.
			ImageView{Image: logo, Mode: ImageViewModeIdeal},
			HSpacer{},
			ImageView{
				AssignTo:    &mw.imgCBDE,
				Image:       logoCBDE,
				Mode:        ImageViewModeIdeal,
				ToolTipText: siteURL,
				OnMouseUp:   func(x, y int, b walk.MouseButton) { openURL(siteURL) },
			},
			HSpacer{Size: 14},
			Composite{
				// Ohne Layout bekaeme das leere Composite keine Groesse.
				Layout:     VBox{MarginsZero: true},
				MinSize:    Size{Width: 1, Height: 34},
				MaxSize:    Size{Width: 1, Height: 34},
				Background: SolidColorBrush{Color: walk.RGB(64, 80, 106)},
			},
			HSpacer{Size: 14},
			ImageView{
				AssignTo:    &mw.imgCF,
				Image:       logoCF,
				Mode:        ImageViewModeIdeal,
				ToolTipText: forceURL,
				OnMouseUp:   func(x, y int, b walk.MouseButton) { openURL(forceURL) },
			},
		},
	}
}

// Reiterleiste: flache Reiter, der aktive weiss mit gelber Unterlinie.
func (mw *AppWindow) tabBar() Widget {
	return Composite{
		MinSize:    Size{Height: tabBarHeight},
		MaxSize:    Size{Height: tabBarHeight},
		Background: SolidColorBrush{Color: colTabBar},
		Layout:     HBox{Margins: Margins{Left: gutter - 6}, SpacingZero: true},
		Children: []Widget{
			mw.tabWidget(mw.tabs[0], 110),
			mw.tabWidget(mw.tabs[1], 140),
			mw.tabWidget(mw.tabs[2], 120),
			mw.tabWidget(mw.tabs[3], 90),
			mw.tabWidget(mw.tabs[4], 80),
			HSpacer{},
			mw.langWidget(),
			HSpacer{Size: gutter - 6},
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
		Background: SolidColorBrush{Color: colAccountBar},
		Layout:     HBox{Margins: Margins{Left: gapM, Top: 8, Right: gapM + 2, Bottom: 8}, Spacing: gapS},
		Children: []Widget{
			Label{AssignTo: &mw.t.accPlayer, Text: tr("acc.player"), Font: fontBodyBold},
			LineEdit{
				AssignTo:      &mw.editAccount,
				Text:          mw.cfg.AccountName,
				Font:          fontBody,
				MinSize:       Size{Width: 150},
				MaxSize:       Size{Width: 150},
				CueBanner:     tr("acc.playerCue"),
				OnTextChanged: func() { mw.updateAccountState() },
			},
			Label{AssignTo: &mw.t.accPassword, Text: tr("acc.password"), Font: fontBodyBold},
			LineEdit{
				AssignTo:      &mw.editAccPassword,
				Text:          mw.cfg.AccountPassword,
				Font:          fontBody,
				MinSize:       Size{Width: 130},
				MaxSize:       Size{Width: 130},
				PasswordMode:  true,
				CueBanner:     tr("acc.passwordCue"),
				OnTextChanged: func() { mw.updateAccountState() },
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
			// "● Nur LAN-Spiele" / "● Online-Konto", siehe updateAccountState.
			Label{AssignTo: &mw.t.accHint, Text: tr("acc.lan"), Font: fontBodyBold, TextColor: colMuted},
		},
	}
}

// updateAccountState zeigt, ob mit den eingetragenen Daten online gespielt
// werden kann (Name und Passwort) oder nur im LAN.
func (mw *AppWindow) updateAccountState() {
	if mw.t.accHint == nil || mw.editAccount == nil || mw.editAccPassword == nil {
		return
	}
	if strings.TrimSpace(mw.editAccount.Text()) != "" && mw.editAccPassword.Text() != "" {
		mw.t.accHint.SetText(tr("acc.online"))
		mw.t.accHint.SetTextColor(colOK)
	} else {
		mw.t.accHint.SetText(tr("acc.lan"))
		mw.t.accHint.SetTextColor(colMuted)
	}
}

func (mw *AppWindow) actionColumn() Widget {
	// Starten ist das, was fast jeder will: der grosse gelbe Knopf. Einen
	// Server hosten steht als zweiter Knopf darunter; die eigene IP dafuer
	// liegt in den Einstellungen unter Netzwerk.
	return Composite{
		MinSize: Size{Width: 190},
		MaxSize: Size{Width: 190},
		Layout:  VBox{MarginsZero: true, Spacing: gapS},
		Children: []Widget{
			VSpacer{Size: 28}, // auf Hoehe der Tabelle neben der Ueberschrift
			mw.ctaButton(&mw.t.btnStart, "act.start", 58, func() { mw.startGame() }),
			PushButton{
				AssignTo:  &mw.t.btnDedicated,
				Text:      tr("act.dedicated"),
				Font:      fontBody,
				MinSize:   Size{Height: 32},
				MaxSize:   Size{Height: 32},
				OnClicked: func() { mw.startDedicated() },
			},
			VSpacer{Size: gapM},
			// Kurzuebersicht der Grafik, mit Sprung in die Einstellungen.
			Composite{
				MinSize:    Size{Width: 190},
				Background: SolidColorBrush{Color: colAccountBar},
				Layout:     VBox{Margins: Margins{Left: gapM, Top: 8, Right: gapM, Bottom: 8}, Spacing: 2, Alignment: AlignHNearVNear},
				Children: []Widget{
					// HSpacer: macht den Kasten so breit wie die Spalte.
					Composite{
						Layout: HBox{MarginsZero: true, SpacingZero: true},
						Children: []Widget{
							Label{AssignTo: &mw.t.gfxTitle, Text: tr("sum.title"), Font: fontBodyBold},
							HSpacer{},
						},
					},
					Label{AssignTo: &mw.lblGfxSummary, Text: "", Font: fontBody, TextColor: walk.RGB(70, 78, 92)},
					LinkLabel{
						AssignTo:        &mw.t.gfxLink,
						Text:            `<a id="s">` + tr("sum.change") + `</a>`,
						Font:            fontBody,
						OnLinkActivated: func(*walk.LinkLabelLink) { mw.showTab(tabEinstellungen) },
					},
				},
			},
			VSpacer{},
			// Der Gleiter des Spiels, langsam drehend (glider.go).
			mw.glider.widget(),
		},
	}
}

// updateGfxSummary fasst die Grafikeinstellungen fuer den Spiel-Reiter
// zusammen: Anzeige und Aufloesung, Kantenglaettung, Texturen.
func (mw *AppWindow) updateGfxSummary() {
	if mw.lblGfxSummary == nil {
		return
	}
	d := mw.cfg.D3D
	disp := tr("set.fullscreen")
	if !d.Fullscreen {
		disp = tr("set.window")
	}
	res := resLabels([]string{d.Resolution})[0]
	aa := tr("set.off")
	if d.MSAA > 1 {
		aa = fmt.Sprintf("%d×", d.MSAA)
	}
	if d.SuperSample > 1 {
		aa += fmt.Sprintf(" + SS %d×", d.SuperSample)
	}
	tex := tr("set.texOrig")
	if d.TexturePack != "" {
		tex = tr("set.texHD4")
	}
	mw.lblGfxSummary.SetText(fmt.Sprintf(tr("sum.text"), disp, res, aa, tex))
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
				Visible:  false, // bis Server da sind, steht dort emptyState
				Columns: []TableViewColumn{
					{Title: tr("srv.colName"), Width: 210},
					{Title: tr("srv.colPlayers"), Width: 64},
					{Title: tr("srv.colMap"), Width: 120},
					{Title: tr("srv.colType"), Width: 80},
				},
				LastColumnStretched: true,
				AlternatingRowBG:    true,
				Model:               mw.hostModel,
				OnItemActivated:     func() { mw.joinSelectedServer() },
			},
			// Leere Liste: statt einer weissen Tabelle ohne Zeilen ein Hinweis,
			// was man jetzt tun kann.
			Composite{
				AssignTo:   &mw.emptyState,
				Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)},
				Layout:     VBox{Margins: Margins{Left: 24, Top: 24, Right: 24, Bottom: 24}, Spacing: gapS},
				Children: []Widget{
					VSpacer{},
					Label{AssignTo: &mw.t.emptyTitle, Text: tr("srv.emptyTitle"), Font: fontCTA,
						TextColor: walk.RGB(60, 70, 90), TextAlignment: AlignCenter},
					Label{AssignTo: &mw.t.emptyHint, Text: tr("srv.emptyHint"), Font: fontBody,
						TextColor: colMuted, TextAlignment: AlignCenter},
					VSpacer{},
				},
			},
			Label{
				AssignTo:  &mw.t.lblTableHint,
				Text:      tr("srv.hint"),
				Font:      fontHint,
				TextColor: colMuted,
			},
		},
	}
}

// ── Tab: Einstellungen ────────────────────────────────────────────────────────

func (mw *AppWindow) einstellungenPanel() Widget {
	// combo baut eine Auswahlliste fester Breite.
	combo := func(assign **walk.ComboBox, model []string, tip string) Widget {
		return ComboBox{AssignTo: assign, Model: model, Font: fontForm, ToolTipText: tip,
			MinSize: Size{Width: 170}, MaxSize: Size{Width: 170}}
	}
	check := func(assign **walk.CheckBox, tip string) Widget {
		return CheckBox{AssignTo: assign, Font: fontForm, ToolTipText: tip}
	}

	return Composite{
		AssignTo: &mw.panelEinstellungen,
		Visible:  false,
		Layout:   VBox{Margins: Margins{Left: gutter, Top: 12, Right: gutter, Bottom: 10}, Spacing: gapM},
		Children: []Widget{
			// Voreinstellungen fuer die Grafik: setzen Kantenglaettung,
			// Supersampling, Filterung und Umgebungsverdeckung auf einmal.
			presetBar(&mw.t.setPreset, mw.presetButton(0), mw.presetButton(1), mw.presetButton(2), mw.presetButton(3)),
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: gapM, Alignment: AlignHNearVNear},
				Children: []Widget{
					// Alles hier landet in der clusterball_d3d.ini, siehe
					// writeD3DSettings. Die Detailstufe des Spiels steht fest
					// (gameQuality), die Mausempfindlichkeit steht unter Steuerung.
					formColumn(
						formCard(&mw.t.setGraphics, "set.graphics",
							formRow(&mw.t.setDisplay, "set.display", "", combo(&mw.cmbDisplay, displayLabels(), "")),
							formRow(&mw.t.setResolution, "set.resolution", tr("set.resolutionTip"),
								combo(&mw.cmbRes, resLabels(resValues), tr("set.resolutionTip"))),
							formRow(&mw.t.setTexPack, "set.texPack", tr("set.texPackTip"),
								combo(&mw.cmbTexPack, packLabels(), tr("set.texPackTip"))),
							formRow(&mw.t.setMSAA, "set.msaa", "", combo(&mw.cmbMSAA, msaaLabels(), "")),
							formRow(&mw.t.setSS, "set.ss", tr("set.ssTip"), combo(&mw.cmbSS, ssLabels(), tr("set.ssTip"))),
							formRow(&mw.t.setAniso, "set.aniso", "", combo(&mw.cmbAniso, anisoLabels(), "")),
							formRow(&mw.t.setVSync, "set.vsync", "", check(&mw.chkVSync, "")),
						),
						mw.networkCard(),
					),
					formColumn(
						// Gamma des Renderers in Prozent: die Helligkeit des Spiels
						// selbst wirkt unter Direct3D nicht, siehe D3DSettings.Gamma.
						formCard(&mw.t.setGame, "set.image",
							formRow(&mw.t.setSharpen, "set.sharpen", "", valueSlider(&mw.sldSharpen, &mw.lblSharpen, 0, 100, nil)),
							formRow(&mw.t.setBrightness, "set.brightness", "",
								valueSlider(&mw.sldBrightness, &mw.lblBrightness, 50, 150, nil)),
							formRow(&mw.t.setAO, "set.ao", tr("set.aoTip"), check(&mw.chkAO, tr("set.aoTip"))),
							formRow(&mw.t.setFontRep, "set.font", tr("set.fontTip"), check(&mw.chkFont, tr("set.fontTip"))),
						),
						// Musik und Ball-Sound von ClusterForce statt der des
						// Spiels, siehe cfaudio.go.
						formCard(&mw.t.setSound, "snd.title",
							formRow(&mw.t.setMusic, "snd.music", tr("snd.musicTip"),
								combo(&mw.cmbMusic, musicLabels(), tr("snd.musicTip"))),
							formRow(&mw.t.setBall, "snd.ball", tr("snd.ballTip"), Composite{
								Layout: HBox{MarginsZero: true, Spacing: 4, Alignment: AlignHNearVCenter},
								Children: []Widget{
									combo(&mw.cmbBall, ballSoundLabels(), tr("snd.ballTip")),
									PushButton{
										AssignTo:    &mw.t.btnPlayBall,
										Text:        "▶",
										Font:        fontForm,
										MinSize:     Size{Width: 26, Height: 23},
										MaxSize:     Size{Width: 26, Height: 23},
										ToolTipText: tr("snd.preview"),
										OnClicked:   func() { mw.previewBallSound() },
									},
									HSpacer{},
								},
							}),
						),
					),
				},
			},
			VSpacer{},
			formFooter(
				[]Widget{formHint(&mw.t.gfxHint, "set.gfxHint"), formHint(&mw.t.d3dHint, "set.d3dHint")},
				PushButton{
					AssignTo:  &mw.t.btnDefaults,
					Text:      tr("set.defaults"),
					Font:      fontForm,
					MinSize:   Size{Width: 210, Height: 32},
					MaxSize:   Size{Width: 210, Height: 32},
					OnClicked: func() { mw.restoreDefaults() },
				},
				// Der Pfad gehoert in den Tooltip: als Label wuerde seine
				// Textbreite dem ganzen Fenster die Mindestbreite diktieren.
				PushButton{
					AssignTo:    &mw.t.btnSave,
					Text:        tr("set.save"),
					Font:        fontFormBold,
					MinSize:     Size{Width: 210, Height: 32},
					MaxSize:     Size{Width: 210, Height: 32},
					ToolTipText: fmt.Sprintf(tr("set.saveTip"), configPath()),
					OnClicked:   func() { mw.saveSettings() },
				},
			),
		},
	}
}

// networkCard: Eigene IP fuer den Dedicated Host. Die Adresse des Master-Servers steht
// nur in der clusterball.ini (server_ip); ein Feld fuer die Host-IP gibt es nicht, der
// Master-Server liest sie aus dem Absender des UDP-Heartbeats.
func (mw *AppWindow) networkCard() Widget {
	return formCard(&mw.t.setNetwork, "set.network",
		formRow(&mw.t.setOwnIP, "act.ownIP", "", Composite{
			Layout: HBox{MarginsZero: true, Spacing: 4, Alignment: AlignHNearVCenter},
			Children: []Widget{
				LineEdit{AssignTo: &mw.editOwnIP, Text: mw.cfg.OwnIP, Font: fontForm,
					MinSize: Size{Width: 130}, MaxSize: Size{Width: 130}},
				PushButton{
					AssignTo:    &mw.t.btnDetectIP,
					Text:        "↻",
					Font:        fontForm,
					MinSize:     Size{Width: 26, Height: 23},
					MaxSize:     Size{Width: 26, Height: 23},
					ToolTipText: tr("act.detectTip"),
					OnClicked:   func() { mw.detectExternalIP() },
				},
				HSpacer{},
			},
		}),
		formRow(&mw.t.setExternal, "act.external", "", CheckBox{
			AssignTo: &mw.chkExternalIP,
			Font:     fontForm,
			OnClicked: func() {
				mw.cfg.UseExternalIP = mw.chkExternalIP.Checked()
				if mw.chkExternalIP.Checked() {
					mw.detectExternalIP()
				}
			},
		}),
	)
}

// Grafik-Voreinstellungen: Kantenglaettung, Supersampling, anisotrope
// Filterung, Umgebungsverdeckung. Aufloesung, Anzeige und Texturen bleiben.
var graphicsPresets = []struct {
	key             string
	msaa, ss, aniso int
	ao              bool
}{
	{"set.pLow", 1, 1, 4, false},
	{"set.pMid", 4, 1, 8, false},
	{"set.pHigh", 8, 1, 16, true},
	{"set.pMax", 8, 2, 16, true},
}

func (mw *AppWindow) presetButton(i int) Widget {
	return PushButton{
		AssignTo:  &mw.t.btnPresets[i],
		Text:      tr(graphicsPresets[i].key),
		Font:      fontForm,
		MinSize:   Size{Width: 100, Height: 28},
		MaxSize:   Size{Width: 100, Height: 28},
		OnClicked: func() { mw.applyPreset(i) },
	}
}

// applyPreset stellt die Auswahllisten ein; gespeichert wird wie sonst auch
// erst mit "Einstellungen speichern" (oder beim Spielstart).
func (mw *AppWindow) applyPreset(i int) {
	p := graphicsPresets[i]
	mw.cmbMSAA.SetCurrentIndex(indexOfInt(msaaValues, p.msaa))
	mw.cmbSS.SetCurrentIndex(indexOfInt(ssValues, p.ss))
	mw.cmbAniso.SetCurrentIndex(indexOfInt(anisoValues, p.aniso))
	mw.chkAO.SetChecked(p.ao)
	mw.setStatus(fmt.Sprintf(tr("st.preset"), tr(p.key)))
}

// restoreDefaults setzt die Grafikeinstellungen auf die Vorgaben zurueck.
func (mw *AppWindow) restoreDefaults() {
	mw.showD3D(defaultD3D)
	mw.sldBrightness.SetValue(clampInt(defaultD3D.Gamma, 50, 150))
	mw.lblBrightness.SetText(fmt.Sprintf("%d", mw.sldBrightness.Value()))
	mw.setStatus(tr("st.defaults"))
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

	for _, t := range mw.tabs {
		t.w.Invalidate() // die Reiter holen ihren Text beim Zeichnen
	}
	mw.langToggle.Invalidate()
	mw.help.retranslate()
	mw.about.retranslate()
	mw.controls.retranslate()

	mw.t.accPlayer.SetText(tr("acc.player"))
	mw.t.accPassword.SetText(tr("acc.password"))
	mw.updateAccountState()
	mw.updateGfxSummary()
	mw.t.btnRegister.SetText(tr("acc.register"))
	mw.t.btnRegister.SetToolTipText(tr("acc.registerTip"))
	mw.editAccount.SetCueBanner(tr("acc.playerCue"))
	mw.editAccPassword.SetCueBanner(tr("acc.passwordCue"))

	mw.t.btnStart.Invalidate()
	mw.t.btnDedicated.SetText(tr("act.dedicated"))
	mw.t.setOwnIP.SetText(tr("act.ownIP"))
	mw.t.btnDetectIP.SetToolTipText(tr("act.detectTip"))
	mw.t.setExternal.SetText(tr("act.external"))

	mw.btnRefresh.SetText(tr("srv.refresh"))
	mw.t.lblTableHint.SetText(tr("srv.hint"))
	mw.t.emptyHint.SetText(tr("srv.emptyHint"))
	mw.updateServerLabel()

	mw.t.setPreset.SetText(tr("set.preset"))
	for i, b := range mw.t.btnPresets {
		b.SetText(tr(graphicsPresets[i].key))
	}
	mw.t.btnDefaults.SetText(tr("set.defaults"))
	mw.t.gfxTitle.SetText(tr("sum.title"))
	mw.t.gfxLink.SetText(`<a id="s">` + tr("sum.change") + `</a>`)

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
	mw.t.setVSync.SetText(tr("set.vsync"))
	mw.t.setFontRep.SetText(tr("set.font"))
	mw.t.setFontRep.SetToolTipText(tr("set.fontTip"))
	mw.chkFont.SetToolTipText(tr("set.fontTip"))
	mw.t.setAO.SetText(tr("set.ao"))
	mw.t.setAO.SetToolTipText(tr("set.aoTip"))
	mw.chkAO.SetToolTipText(tr("set.aoTip"))
	mw.t.d3dHint.SetText(tr("set.d3dHint"))

	mw.t.setGame.SetText(tr("set.image"))
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
	refill(mw.cmbMusic, musicLabels())
	refill(mw.cmbBall, ballSoundLabels())
	mw.t.setSound.SetText(tr("snd.title"))
	mw.t.setMusic.SetText(tr("snd.music"))
	mw.t.setMusic.SetToolTipText(tr("snd.musicTip"))
	mw.cmbMusic.SetToolTipText(tr("snd.musicTip"))
	mw.t.setBall.SetText(tr("snd.ball"))
	mw.t.setBall.SetToolTipText(tr("snd.ballTip"))
	mw.cmbBall.SetToolTipText(tr("snd.ballTip"))
	mw.t.btnPlayBall.SetToolTipText(tr("snd.preview"))

	mw.t.setNetwork.SetText(tr("set.network"))
	mw.t.btnSave.SetText(tr("set.save"))
	mw.t.btnSave.SetToolTipText(fmt.Sprintf(tr("set.saveTip"), configPath()))

	mw.t.sbBrand.SetText(tr("sb.brand"))
	mw.updateServerStatus()
}

// updateServerLabel schreibt die Ueberschrift der Serverliste passend zum
// zuletzt geladenen Stand - auch nach einem Sprachwechsel.
func (mw *AppWindow) updateServerLabel() {
	if mw.lblServers == nil {
		return
	}
	// Ohne Server steht statt der leeren Tabelle der Hinweis.
	if mw.emptyState != nil {
		mw.emptyState.SetVisible(mw.lastHosts <= 0)
		mw.hostListView.SetVisible(mw.lastHosts > 0)
		if mw.lastHosts < 0 {
			mw.t.emptyTitle.SetText(tr("srv.loadingTitle"))
		} else {
			mw.t.emptyTitle.SetText(tr("srv.emptyTitle"))
		}
	}
	switch {
	case mw.lastHosts < 0:
		mw.lblServers.SetText(tr("srv.title"))
	case mw.lastHosts == 0:
		mw.lblServers.SetText(tr("srv.title")) // "keine" sagt schon der Hinweis in der Liste
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
	mw.panelHilfe.SetVisible(tab == tabHilfe)
	mw.panelUeber.SetVisible(tab == tabUeber)
	mw.panelSteuerung.SetVisible(tab == tabSteuerung)
	if tab != tabSteuerung {
		mw.controls.stopCapture()
	}
	for _, t := range mw.tabs {
		t.w.Invalidate()
	}
}

// ── Konfiguration ↔ Oberflaeche ───────────────────────────────────────────────

func (mw *AppWindow) applyConfig() {
	setText(mw.editOwnIP, mw.cfg.OwnIP)
	setText(mw.editAccount, mw.cfg.AccountName)
	setText(mw.editAccPassword, mw.cfg.AccountPassword)

	if mw.chkExternalIP != nil {
		mw.chkExternalIP.SetChecked(mw.cfg.UseExternalIP)
	}

	if mw.cmbDisplay != nil {
		mw.showD3D(mw.cfg.D3D)
	}

	// Spiel
	if mw.sldBrightness != nil {
		mw.sldBrightness.SetValue(clampInt(mw.cfg.D3D.Gamma, 50, 150))
		mw.lblBrightness.SetText(fmt.Sprintf("%d", mw.sldBrightness.Value()))
	}
	mw.updateAccountState()
	mw.updateGfxSummary()
}

// showD3D stellt die Auswahllisten der Direct3D-Schicht auf d.
func (mw *AppWindow) showD3D(d D3DSettings) {
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
	if d.CFMusic {
		mw.cmbMusic.SetCurrentIndex(1)
	} else {
		mw.cmbMusic.SetCurrentIndex(0)
	}
	mw.cmbBall.SetCurrentIndex(indexOfStr(ballSoundValues, d.BallSound))
	mw.sldMouse.SetValue(clampInt(int(d.MouseScale*100+0.5), 5, 100))
	mw.lblMouse.SetText(fmt.Sprintf("%d", mw.sldMouse.Value()))
	mw.controls.chkInvert.SetChecked(mw.cfg.InvertAxis)
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
		d.CFMusic = mw.cmbMusic.CurrentIndex() == 1
		if i, ok := pick(mw.cmbBall, len(ballSoundValues)); ok {
			d.BallSound = ballSoundValues[i]
		}
		d.MouseScale = float64(mw.sldMouse.Value()) / 100
	}

	// Spiel. Farbtiefe und Detailstufe stehen fest und werden nicht abgefragt.
	mw.cfg.Depth = gameDepth
	mw.cfg.GraphicalQuality = gameQuality
	if mw.sldBrightness != nil {
		mw.cfg.D3D.Gamma = mw.sldBrightness.Value()
	}

	// Die Sprache setzt switchLanguage direkt in mw.cfg.
	mw.cfg.ServerMode = "arena"
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
