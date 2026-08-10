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
	appTitle   = "Clusterball.de Command Center"
	appVersion = "2.0"
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

// Hoechste Aufloesung, die das Spiel selbst kennt. Clusterball.exe enthaelt
// genau drei Modi - "640 x 480", "800 x 600" und "1024 x 768" - mehr bietet
// die Glide-Schnittstelle des Spiels nicht an. Der Launcher setzt deshalb
// immer das Maximum; schaerfer wird das Bild ueber die Renderaufloesung des
// Wrappers, nicht ueber diesen Wert.
const (
	gameResW = 1024
	gameResH = 768
)

// Farbtiefe des Bildspeichers. Glide arbeitet mit 16 Bit - eine andere Tiefe
// bietet die Schnittstelle nicht an, deshalb steht der Wert fest.
const gameDepth = 16

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

var renderers = []string{"3dfx", "Direct3D", "OpenGL", "Software"}

// Auswahllisten, deren Beschriftung uebersetzt wird, waehrend der gespeicherte
// Wert gleich bleibt. Die Glide-Werte sind die von nGlide dokumentierten
// Formen: "0"/"1" fuer die Vorgaben, sonst die Aufloesung als Text.
var (
	depthValues       = []int{gameDepth} // nur 16 Bit, siehe gameDepth
	qualityValues     = []int{0, 1, 2}
	glideResValues    = []string{"0", "1", "1024x768", "1280x960", "1600x1200", "1920x1080"}
	glideAspectValues = []string{"0", "1", "4:3"}
)

func depthLabels() []string   { return []string{tr("set.depth16")} }
func qualityLabels() []string { return []string{tr("set.qLow"), tr("set.qMid"), tr("set.qHigh")} }

func glideResLabels() []string {
	return []string{tr("set.glideResApp"), tr("set.glideResDsk"),
		"1024 × 768", "1280 × 960", "1600 × 1200", "1920 × 1080"}
}

func glideAspectLabels() []string {
	return []string{tr("set.glideAspFit"), tr("set.glideAspApp"), tr("set.glideAsp43")}
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

	setProgram    *walk.Label
	setGraphics   *walk.Label
	setRenderer   *walk.Label
	setResolution *walk.Label
	resMaxHint    *walk.Label
	setDepth      *walk.Label
	setQuality    *walk.Label
	setBrightness *walk.Label
	setNetwork    *walk.Label
	setMaster     *walk.Label
	setLanguage   *walk.Label
	gfxHint       *walk.Label
	btnBrowse     *walk.PushButton
	btnSave       *walk.PushButton

	setGlide       *walk.Label
	setGlideRes    *walk.Label
	setGlideAspect *walk.Label
	glideHint      *walk.Label

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
	editCBPath   *walk.LineEdit
	editServerIP *walk.LineEdit
	cmbRenderer  *walk.ComboBox
	editWidth    *walk.LineEdit
	editHeight   *walk.LineEdit
	radioDeutsch *walk.RadioButton
	radioEnglish *walk.RadioButton

	// Grafik des Spiels
	cmbDepth      *walk.ComboBox
	cmbQuality    *walk.ComboBox
	sldBrightness *walk.Slider
	lblBrightness *walk.Label
	chkAntialias  *walk.CheckBox
	chkTrilinear  *walk.CheckBox
	chkTex32      *walk.CheckBox
	chkClamp      *walk.CheckBox

	// nGlide
	cmbGlideRes    *walk.ComboBox
	cmbGlideAspect *walk.ComboBox
	chkGlideVSync  *walk.CheckBox
	chkGlideSplash *walk.CheckBox

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

	// Ohne gueltige Clusterball.exe kann man nichts starten - dann direkt
	// dort hinfuehren, wo man das geradebiegt.
	if !mw.cbExeOK() {
		mw.showTab(tabEinstellungen)
		mw.setStatus(tr("st.exeMissing"))
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
					Label{
						Text:      "Command Center",
						TextColor: walk.RGB(255, 200, 0),
						Font:      fontTitle,
					},
					Label{
						Text:      "v" + appVersion + " · clusterball.de",
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
	return Composite{
		AssignTo: &mw.panelEinstellungen,
		Visible:  false,
		Layout:   VBox{Margins: Margins{Left: gutter, Top: 12, Right: gutter, Bottom: 10}, Spacing: gapM},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 24},
				Children: []Widget{

					// Linke Spalte. HNear, sonst zentriert walk jede Zeile
					// einzeln in der Spaltenbreite - Checkboxen stehen dann
					// versetzt zueinander statt bündig.
					Composite{
						Layout: VBox{MarginsZero: true, Spacing: gapS, Alignment: AlignHNearVNear},
						Children: []Widget{
							Label{AssignTo: &mw.t.setProgram, Text: tr("set.program"), Font: fontHeading},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 4},
								Children: []Widget{
									LineEdit{AssignTo: &mw.editCBPath, Text: mw.cfg.CBExePath, Font: fontBody},
									PushButton{
										AssignTo:    &mw.t.btnBrowse,
										Text:        tr("set.browse"),
										Font:        fontBody,
										MinSize:     Size{Width: 110, Height: 22},
										MaxSize:     Size{Width: 110, Height: 22},
										ToolTipText: tr("set.browseTip"),
										OnClicked:   func() { mw.browseCBExe() },
									},
								},
							},
							VSpacer{Size: gapS},
							Label{AssignTo: &mw.t.setGraphics, Text: tr("set.graphics"), Font: fontHeading},
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true, Spacing: gapS},
								Children: []Widget{
									Label{AssignTo: &mw.t.setRenderer, Text: tr("set.renderer"), Font: fontBody},
									Composite{
										Layout: HBox{MarginsZero: true, SpacingZero: true},
										Children: []Widget{
											ComboBox{
												AssignTo: &mw.cmbRenderer,
												Model:    renderers,
												Font:     fontBody,
												MinSize:  Size{Width: 130},
												MaxSize:  Size{Width: 130},
												// Die nGlide-Optionen greifen nur
												// im Glide-Pfad.
												OnCurrentIndexChanged: func() { mw.updateGlideEnabled() },
											},
											HSpacer{},
										},
									},
									Label{AssignTo: &mw.t.setResolution, Text: tr("set.resolution"), Font: fontBody,
										ToolTipText: tr("set.resolutionTip")},
									// Gesperrt: der Launcher setzt immer das Maximum,
									// das das Spiel kennt.
									Composite{
										Layout: HBox{MarginsZero: true, Spacing: gapS},
										Children: []Widget{
											LineEdit{AssignTo: &mw.editWidth, Font: fontBody,
												ReadOnly: true, Enabled: false,
												ToolTipText: tr("set.resolutionTip"),
												MinSize:     Size{Width: 58}, MaxSize: Size{Width: 58}},
											Label{Text: "×", Font: fontBody},
											LineEdit{AssignTo: &mw.editHeight, Font: fontBody,
												ReadOnly: true, Enabled: false,
												ToolTipText: tr("set.resolutionTip"),
												MinSize:     Size{Width: 58}, MaxSize: Size{Width: 58}},
											Label{AssignTo: &mw.t.resMaxHint, Text: tr("set.resolutionMax"),
												Font: fontHint, TextColor: walk.RGB(120, 125, 135)},
											HSpacer{},
										},
									},

									// Gesperrt: Glide kennt nur 16 Bit.
									Label{AssignTo: &mw.t.setDepth, Text: tr("set.depth"), Font: fontBody,
										ToolTipText: tr("set.depthTip")},
									Composite{
										Layout: HBox{MarginsZero: true, SpacingZero: true},
										Children: []Widget{
											ComboBox{
												AssignTo:    &mw.cmbDepth,
												Model:       depthLabels(),
												Font:        fontBody,
												Enabled:     false,
												ToolTipText: tr("set.depthTip"),
												MinSize:     Size{Width: 130},
												MaxSize:     Size{Width: 130},
											},
											HSpacer{},
										},
									},

									Label{AssignTo: &mw.t.setQuality, Text: tr("set.quality"), Font: fontBody},
									Composite{
										Layout: HBox{MarginsZero: true, SpacingZero: true},
										Children: []Widget{
											ComboBox{
												AssignTo: &mw.cmbQuality,
												Model:    qualityLabels(),
												Font:     fontBody,
												MinSize:  Size{Width: 130},
												MaxSize:  Size{Width: 130},
											},
											HSpacer{},
										},
									},

									Label{AssignTo: &mw.t.setBrightness, Text: tr("set.brightness"), Font: fontBody},
									Composite{
										Layout: HBox{MarginsZero: true, Spacing: gapS},
										Children: []Widget{
											Slider{
												AssignTo: &mw.sldBrightness,
												MinValue: 0,
												MaxValue: 100,
												MinSize:  Size{Width: 130},
												MaxSize:  Size{Width: 130},
												Tracking: true,
												OnValueChanged: func() {
													mw.lblBrightness.SetText(fmt.Sprintf("%d", mw.sldBrightness.Value()))
												},
											},
											Label{AssignTo: &mw.lblBrightness, Text: "0", Font: fontBody,
												MinSize: Size{Width: 28}},
											HSpacer{},
										},
									},
								},
							},

							CheckBox{AssignTo: &mw.chkAntialias, Text: tr("set.antialias"), Font: fontBody},
							CheckBox{AssignTo: &mw.chkTrilinear, Text: tr("set.trilinear"), Font: fontBody},
							CheckBox{AssignTo: &mw.chkTex32, Text: tr("set.tex32"), Font: fontBody},
							CheckBox{AssignTo: &mw.chkClamp, Text: tr("set.clamp"), Font: fontBody},
							Label{
								AssignTo:  &mw.t.gfxHint,
								Text:      tr("set.gfxHint"),
								Font:      fontHint,
								TextColor: walk.RGB(120, 125, 135),
							},
						},
					},

					// Rechte Spalte
					Composite{
						Layout: VBox{MarginsZero: true, Spacing: gapS, Alignment: AlignHNearVNear},
						Children: []Widget{
							// Das mitgelieferte glide2x.dll ist nGlide 2.1. Seine
							// Einstellungen gehen als Umgebungsvariablen an den
							// Spielprozess, siehe glideEnv.
							Label{AssignTo: &mw.t.setGlide, Text: tr("set.glide"), Font: fontHeading},
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true, Spacing: gapS},
								Children: []Widget{
									Label{AssignTo: &mw.t.setGlideRes, Text: tr("set.glideRes"), Font: fontBody,
										ToolTipText: tr("set.glideResTip")},
									ComboBox{
										AssignTo:    &mw.cmbGlideRes,
										Model:       glideResLabels(),
										Font:        fontBody,
										ToolTipText: tr("set.glideResTip"),
										MinSize:     Size{Width: 150}, MaxSize: Size{Width: 150},
									},
									Label{AssignTo: &mw.t.setGlideAspect, Text: tr("set.glideAspect"), Font: fontBody},
									ComboBox{
										AssignTo: &mw.cmbGlideAspect,
										Model:    glideAspectLabels(),
										Font:     fontBody,
										MinSize:  Size{Width: 150}, MaxSize: Size{Width: 150},
									},
								},
							},
							CheckBox{AssignTo: &mw.chkGlideVSync, Text: tr("set.glideVSync"), Font: fontBody},
							CheckBox{AssignTo: &mw.chkGlideSplash, Text: tr("set.glideSplash"), Font: fontBody},
							Label{
								AssignTo:  &mw.t.glideHint,
								Text:      tr("set.glideOnly3dfx"),
								Font:      fontHint,
								TextColor: walk.RGB(120, 125, 135),
							},

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

	mw.t.setProgram.SetText(tr("set.program"))
	mw.t.setGraphics.SetText(tr("set.graphics"))
	mw.t.setRenderer.SetText(tr("set.renderer"))
	mw.t.setResolution.SetText(tr("set.resolution"))
	mw.t.setDepth.SetText(tr("set.depth"))
	mw.t.setQuality.SetText(tr("set.quality"))
	mw.t.setBrightness.SetText(tr("set.brightness"))
	mw.chkAntialias.SetText(tr("set.antialias"))
	mw.chkTrilinear.SetText(tr("set.trilinear"))
	mw.chkTex32.SetText(tr("set.tex32"))
	mw.chkClamp.SetText(tr("set.clamp"))
	mw.t.gfxHint.SetText(tr("set.gfxHint"))

	mw.t.setResolution.SetToolTipText(tr("set.resolutionTip"))
	mw.t.resMaxHint.SetText(tr("set.resolutionMax"))
	mw.editWidth.SetToolTipText(tr("set.resolutionTip"))
	mw.editHeight.SetToolTipText(tr("set.resolutionTip"))

	mw.t.setGlide.SetText(tr("set.glide"))
	mw.t.setGlideRes.SetText(tr("set.glideRes"))
	mw.t.setGlideRes.SetToolTipText(tr("set.glideResTip"))
	mw.cmbGlideRes.SetToolTipText(tr("set.glideResTip"))
	mw.t.setGlideAspect.SetText(tr("set.glideAspect"))
	mw.chkGlideVSync.SetText(tr("set.glideVSync"))
	mw.chkGlideSplash.SetText(tr("set.glideSplash"))
	mw.t.glideHint.SetText(tr("set.glideOnly3dfx"))

	// Auswahllisten neu beschriften. SetModel setzt die Auswahl zurueck,
	// deshalb Index merken und wiederherstellen.
	refill := func(cb *walk.ComboBox, labels []string) {
		i := cb.CurrentIndex()
		cb.SetModel(labels)
		cb.SetCurrentIndex(i)
	}
	refill(mw.cmbDepth, depthLabels())
	refill(mw.cmbQuality, qualityLabels())
	refill(mw.cmbGlideRes, glideResLabels())
	refill(mw.cmbGlideAspect, glideAspectLabels())

	mw.t.setNetwork.SetText(tr("set.network"))
	mw.t.setMaster.SetText(tr("set.master"))
	mw.t.setLanguage.SetText(tr("set.language"))
	mw.t.btnBrowse.SetText(tr("set.browse"))
	mw.t.btnBrowse.SetToolTipText(tr("set.browseTip"))
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
	setText(mw.editCBPath, mw.cfg.CBExePath)
	setText(mw.editServerIP, mw.cfg.ServerIP)
	setText(mw.editOwnIP, mw.cfg.OwnIP)
	setText(mw.editAccount, mw.cfg.AccountName)
	setText(mw.editAccPassword, mw.cfg.AccountPassword)
	setText(mw.editWidth, fmt.Sprintf("%d", mw.cfg.Width))
	setText(mw.editHeight, fmt.Sprintf("%d", mw.cfg.Height))

	if mw.chkExternalIP != nil {
		mw.chkExternalIP.SetChecked(mw.cfg.UseExternalIP)
	}
	if mw.cmbRenderer != nil {
		idx := 0
		for i, r := range renderers {
			if r == mw.cfg.Renderer {
				idx = i
				break
			}
		}
		mw.cmbRenderer.SetCurrentIndex(idx)
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

	// Grafik des Spiels
	if mw.cmbDepth != nil {
		mw.cmbDepth.SetCurrentIndex(indexOfInt(depthValues, mw.cfg.Depth))
		mw.cmbQuality.SetCurrentIndex(indexOfInt(qualityValues, mw.cfg.GraphicalQuality))
		mw.sldBrightness.SetValue(clampInt(mw.cfg.Brightness, 0, 100))
		mw.lblBrightness.SetText(fmt.Sprintf("%d", mw.sldBrightness.Value()))
		mw.chkAntialias.SetChecked(mw.cfg.Antialias)
		mw.chkTrilinear.SetChecked(mw.cfg.TrilinearMipmap)
		mw.chkTex32.SetChecked(mw.cfg.Force32BitTex)
		mw.chkClamp.SetChecked(mw.cfg.ClampTexture)
	}

	// nGlide
	if mw.cmbGlideRes != nil {
		mw.cmbGlideRes.SetCurrentIndex(indexOfStr(glideResValues, mw.cfg.GlideResolution))
		mw.cmbGlideAspect.SetCurrentIndex(indexOfStr(glideAspectValues, mw.cfg.GlideAspect))
		mw.chkGlideVSync.SetChecked(mw.cfg.GlideVSync)
		mw.chkGlideSplash.SetChecked(mw.cfg.GlideSplash)
	}
	mw.updateGlideEnabled()
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

// currentRenderer liefert den in der Auswahl stehenden Renderer.
func (mw *AppWindow) currentRenderer() string {
	if mw.cmbRenderer != nil {
		if i := mw.cmbRenderer.CurrentIndex(); i >= 0 && i < len(renderers) {
			return renderers[i]
		}
	}
	return mw.cfg.Renderer
}

// updateGlideEnabled graut die nGlide-Optionen aus, wenn nicht ueber Glide
// gerendert wird - dann laufen sie ins Leere.
func (mw *AppWindow) updateGlideEnabled() {
	on := mw.currentRenderer() == "3dfx"
	if mw.cmbGlideRes == nil {
		return // Fenster noch im Aufbau
	}
	mw.cmbGlideRes.SetEnabled(on)
	mw.cmbGlideAspect.SetEnabled(on)
	mw.chkGlideVSync.SetEnabled(on)
	mw.chkGlideSplash.SetEnabled(on)
	mw.t.setGlideRes.SetEnabled(on)
	mw.t.setGlideAspect.SetEnabled(on)
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

func (mw *AppWindow) saveSettings() {
	mw.cfg.CBExePath = text(mw.editCBPath)
	mw.cfg.ServerIP = text(mw.editServerIP)

	if mw.cmbRenderer != nil {
		if idx := mw.cmbRenderer.CurrentIndex(); idx >= 0 && idx < len(renderers) {
			mw.cfg.Renderer = renderers[idx]
		}
	}
	// Auflösung wird nicht aus der Oberfläche gelesen - sie steht fest auf
	// dem Maximum des Spiels.
	mw.cfg.Width, mw.cfg.Height = gameResW, gameResH

	// Grafik des Spiels. Die Farbtiefe steht fest und wird nicht abgefragt.
	mw.cfg.Depth = gameDepth
	if mw.cmbQuality != nil {
		if i := mw.cmbQuality.CurrentIndex(); i >= 0 && i < len(qualityValues) {
			mw.cfg.GraphicalQuality = qualityValues[i]
		}
		mw.cfg.Brightness = mw.sldBrightness.Value()
		mw.cfg.Antialias = mw.chkAntialias.Checked()
		mw.cfg.TrilinearMipmap = mw.chkTrilinear.Checked()
		mw.cfg.Force32BitTex = mw.chkTex32.Checked()
		mw.cfg.ClampTexture = mw.chkClamp.Checked()
	}

	// nGlide
	if mw.cmbGlideRes != nil {
		if i := mw.cmbGlideRes.CurrentIndex(); i >= 0 && i < len(glideResValues) {
			mw.cfg.GlideResolution = glideResValues[i]
		}
		if i := mw.cmbGlideAspect.CurrentIndex(); i >= 0 && i < len(glideAspectValues) {
			mw.cfg.GlideAspect = glideAspectValues[i]
		}
		mw.cfg.GlideVSync = mw.chkGlideVSync.Checked()
		mw.cfg.GlideSplash = mw.chkGlideSplash.Checked()
	}

	mw.cfg.ServerMode = "arena"
	if mw.radioEnglish != nil && mw.radioEnglish.Checked() {
		mw.cfg.Language = "en"
	} else {
		mw.cfg.Language = "de"
	}
	mw.syncSpiel()

	if err := SaveConfig(&mw.cfg); err != nil {
		walk.MsgBox(mw.MainWindow, tr("dlg.error"), err.Error(), walk.MsgBoxIconError)
		return
	}
	// Die Grafikwerte gehoeren in die config.cfg des Spiels - sonst bleibt die
	// Einstellung wieder folgenlos.
	if err := writeGameSettings(&mw.cfg); err != nil {
		walk.MsgBox(mw.MainWindow, tr("dlg.error"), fmt.Sprintf(tr("dlg.gameCfg"), err), walk.MsgBoxIconWarning)
	}
	// Werte zurueckschreiben, damit korrigierte Eingaben (z.B. "abc" als
	// Breite) sofort sichtbar wieder auf dem gueltigen Stand stehen.
	mw.applyConfig()
	mw.setStatus(tr("st.saved"))
}

func (mw *AppWindow) browseCBExe() {
	dlg := new(walk.FileDialog)
	dlg.Title = tr("dlg.pickExe")
	dlg.Filter = tr("dlg.filterCB")
	if cur := text(mw.editCBPath); cur != "" {
		dlg.InitialDirPath = filepath.Dir(cur)
	}
	if ok, _ := dlg.ShowOpen(mw.MainWindow); ok {
		mw.editCBPath.SetText(dlg.FilePath)
	}
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
	mw.syncSpiel()
	if err := SaveConfig(&mw.cfg); err != nil {
		mw.setStatus(fmt.Sprintf(tr("st.cfgFailed"), err))
	}
	mw.setStatus(tr("st.starting"))
	go launchCB(&mw.cfg, false)
}

func (mw *AppWindow) startDedicated() {
	mw.syncSpiel()
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
	mw.syncSpiel()
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

	args := []string{"/host"}
	if dedicated {
		args = []string{"/dedicatedserver", "dedi1.cfg"}
	}
	cmd := exec.Command(exePath, args...)
	cmd.Dir = filepath.Dir(exePath)
	cmd.Env = glideEnv(cfg) // nGlide-Einstellungen nur fuer diesen Prozess
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
