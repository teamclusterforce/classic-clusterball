package main

import "syscall"

// Sprachtabellen der Oberfläche.
//
// currentLang ist bewusst paketweit: auch launchCB und launchDedicatedHost
// zeigen Meldungen an, laufen aber ohne Fensterreferenz (z.B. beim Start mit
// -d oder -g).
//
// Fehlt ein Schlüssel in textsEN, greift automatisch der deutsche Text – eine
// unvollständige Übersetzung führt also nie zu leeren Beschriftungen.
var currentLang = "de"

func setLang(lang string) {
	if lang == "en" {
		currentLang = "en"
		return
	}
	currentLang = "de"
}

func tr(key string) string {
	if currentLang == "en" {
		if s, ok := textsEN[key]; ok {
			return s
		}
	}
	if s, ok := textsDE[key]; ok {
		return s
	}
	return key
}

var procGetUserDefaultUILanguage = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// systemLanguage ist "de", wenn die Windows-Oberflaeche deutsch ist, sonst "en".
func systemLanguage() string {
	id, _, _ := procGetUserDefaultUILanguage.Call()
	return langOfLANGID(uint16(id))
}

// langOfLANGID: die unteren 10 Bit einer LANGID sind die Hauptsprache,
// LANG_GERMAN ist 0x07 (de-DE, de-AT, de-CH, ...).
func langOfLANGID(id uint16) string {
	if id&0x3ff == 0x07 {
		return "de"
	}
	return "en"
}

var textsDE = map[string]string{
	"tab.game":     "Spiel",
	"tab.settings": "Einstellungen",
	"tab.controls": "Steuerung",
	"tab.help":     "Hilfe",
	"tab.about":    "Über",

	"snd.title":         "Ton",
	"snd.music":         "Musik",
	"snd.musicTip":      "ClusterForce: zu jeder Venue ein passender 30-Sekunden-Ausschnitt aus der Musik von ClusterForce, dazu Intro und Outro. Musik läuft in Clusterball nur in den Menüs und beim Laden.",
	"snd.musicOriginal": "Original",
	"snd.musicCF":       "ClusterForce",
	"snd.ball":          "Ball-Sound",
	"snd.ballTip":       "Der Ton beim Einsammeln eines Balls: der von Clusterball oder eine der drei Varianten aus ClusterForce.",
	"snd.ballOriginal":  "Original",
	"snd.ballStandard":  "CF: Standard",
	"snd.ballComputer":  "CF: Bordcomputer",
	"snd.ballAnnouncer": "CF: Sprecher",
	"snd.preview":       "Anhören",

	"ctl.accelerate":        "Beschleunigen",
	"ctl.brake":             "Bremsen",
	"ctl.turn_left":         "Nach links",
	"ctl.turn_right":        "Nach rechts",
	"ctl.pitch_up":          "Nach oben",
	"ctl.pitch_down":        "Nach unten",
	"ctl.roll":              "Rollen",
	"ctl.superboost":        "Superboost",
	"ctl.fire":              "Feuern",
	"ctl.next_weapon":       "Nächste Waffe",
	"ctl.prev_weapon":       "Vorherige Waffe",
	"ctl.change_target":     "Ziel wechseln",
	"ctl.release_balls":     "Köderball",
	"ctl.talk":              "Spielchat",
	"ctl.toggle_scoreboard": "Punkteübersicht",
	"ctl.change_view":       "Sicht wechseln",
	"ctl.cardFly":           "Fliegen",
	"ctl.cardMore":          "Kampf und mehr",
	"ctl.presetWASD":        "WASD + Maus",
	"ctl.presetClassic":     "Klassisch",
	"ctl.hint":              "Klick auf eine Taste, dann die neue Taste, Maustaste oder das Mausrad betätigen (Esc bricht ab).\nDie Maus lenkt außerdem immer mit. Waffen 1–9 und Schnell-Chat F1–F10 bleiben, wie sie sind.",
	"ctl.press":             "Taste drücken …",
	"ctl.wheelUp":           "Mausrad hoch",
	"ctl.wheelDown":         "Mausrad runter",
	"ctl.lmb":               "Linke Maustaste",
	"ctl.rmb":               "Rechte Maustaste",
	"ctl.mmb":               "Mittlere Maustaste",
	"ctl.mb4":               "Maustaste 4",
	"ctl.saved":             "Steuerung gespeichert - gilt ab dem nächsten Spielstart",
	"ctl.canceled":          "Belegung abgebrochen",
	"ctl.unsupported":       "Diese Taste kennt das Spiel nicht",

	"acc.player":      "Spieler:",
	"acc.playerCue":   "clusterball.de Account",
	"acc.password":    "Passwort:",
	"acc.passwordCue": "Passwort",
	"acc.register":    "Jetzt registrieren",
	"acc.registerTip": "Kostenlosen clusterball.de-Account anlegen – ohne Account sind nur LAN-Spiele möglich",
	"acc.lan":         "●  Nur LAN-Spiele",
	"acc.online":      "●  Online-Konto",

	"act.start":     "SPIEL STARTEN",
	"act.dedicated": "Server hosten (Dedicated)",
	"act.ownIP":     "Eigene IP",
	"act.detectTip": "Externe IP jetzt neu erkennen",
	"act.external":  "Externe IP verwenden",

	"srv.title":        "ONLINE SERVER",
	"srv.titleN":       "ONLINE SERVER  (%d)",
	"srv.refresh":      "↺ Aktualisieren",
	"srv.colName":      "Server-Name",
	"srv.colPlayers":   "Spieler",
	"srv.colMap":       "Karte",
	"srv.colType":      "Typ",
	"srv.hint":         "Doppelklick zum Beitreten  ·  Auto-Refresh alle 10s",
	"srv.emptyTitle":   "Gerade ist kein Server online",
	"sum.title":        "Grafik",
	"sum.text":         "%s · %s\nKantenglättung %s\nTexturen: %s",
	"sum.change":       "Einstellungen ändern",
	"srv.loadingTitle": "Serverliste wird geladen …",
	"srv.emptyHint":    "Starte ein LAN-Spiel oder hoste einen Server für alle.\nDie Liste aktualisiert sich alle 10 Sekunden.",
	"srv.dedicated":    "Dedicated",

	"set.graphics": "Grafik (Direct3D 11)",
	"set.network":  "Netzwerk",
	"set.language": "Sprache",
	"set.save":     "Einstellungen speichern",
	"set.defaults": "Standard wiederherstellen",
	"set.preset":   "Voreinstellung:",
	"set.pLow":     "Niedrig",
	"set.pMid":     "Mittel",
	"set.pHigh":    "Hoch",
	"set.pMax":     "Maximum",
	"set.saveTip":  "Speichert nach %s",

	"set.display":       "Anzeige",
	"set.fullscreen":    "Vollbild",
	"set.window":        "Fenster",
	"set.resolution":    "Auflösung",
	"set.resolutionTip": "Auflösung, in der gezeichnet wird. „Nativ“ nimmt die des Monitors und ist am schärfsten; eine kleinere wird im Vollbild auf den Monitor gestreckt.",
	"set.resNative":     "Nativ (Monitor)",
	"set.msaa":          "Kantenglättung",
	"set.ss":            "Supersampling",
	"set.ssTip":         "Zeichnet in doppelter Auflösung und rechnet herunter – sehr glatte Kanten, braucht aber deutlich mehr Grafikleistung.",
	"set.aniso":         "Anisotrope Filterung",
	"set.off":           "aus",
	"set.texPack":       "Texturen",
	"set.texPackTip":    "Das hochaufgelöste Texturpaket liegt im Spielordner unter textures\\hd.",
	"set.texOrig":       "Original",
	"set.texHD4":        "HD (4-fach)",
	"set.sharpen":       "Schärfe",
	"set.vsync":         "Vertikale Synchronisation",
	"set.font":          "Scharfe Schrift",
	"set.fontTip":       "Ersetzt die 8 × 8 Pixel kleine Schrift des Spiels durch eine hochaufgelöste aus der Bahnschrift von Windows.",
	"set.ao":            "Umgebungsverdeckung",
	"set.aoTip":         "Schattiert Ecken und Kanten. Zeigt auf dem groben Gelände dessen Facetten, deshalb standardmäßig aus.",
	"set.d3dHint":       "Alt+Enter wechselt im Spiel zwischen Vollbild und Fenster.",

	"set.image":      "Bild",
	"set.brightness": "Helligkeit",
	"set.mouse":      "Mausempfindlichkeit",
	"set.mouseTip":   "Empfindlichkeit der Maus im Spiel; die Zeigergeschwindigkeit von Windows kommt hinzu.",
	"set.gfxHint":    "Wirkt beim nächsten Spielstart.",

	"st.ready":       "Bereit",
	"st.starting":    "Clusterball wird gestartet …",
	"st.dediStart":   "Dedicated Host wird gestartet …",
	"st.dediRunning": "Dedicated Host läuft – Heartbeat aktiv",
	"st.loading":     "Serverliste wird geladen …",
	"st.listError":   "Serverliste nicht erreichbar: %s",
	"st.noServers":   "Keine Server online",
	"st.nServers1":   "1 Server online",
	"st.nServers":    "%d Server online",
	"st.saved":       "Einstellungen gespeichert",
	"st.preset":      "Voreinstellung „%s“ gewählt – wirkt nach dem Speichern oder beim Spielstart",
	"st.defaults":    "Standardwerte eingesetzt – wirken nach dem Speichern oder beim Spielstart",
	"st.ipDetect":    "Externe IP wird ermittelt …",
	"st.ipFailed":    "IP-Erkennung fehlgeschlagen: %s",
	"st.ipOK":        "Externe IP: %s",
	"st.joining":     "Starte für „%s“ …",
	"st.cfgFailed":   "Konfiguration nicht gespeichert: %s",

	"dlg.error":     "Fehler",
	"dlg.noPath":    "Der Spielordner konnte nicht ermittelt werden.",
	"dlg.notFound":  "Clusterball.exe nicht gefunden:\n%s\n\nDer Launcher gehört in den Spielordner, neben die Clusterball.exe.",
	"dlg.heartbeat": "Heartbeat-Fehler",
	"dlg.gameCfg":   "config.cfg des Spiels konnte nicht geschrieben werden:\n%s",
	"dlg.d3dCfg":    "clusterball_d3d.ini konnte nicht geschrieben werden:\n%s",
	"dlg.layer":     "Die Direct3D-Schicht (opengl32.dll) konnte nicht ins Spielverzeichnis gelegt werden - läuft das Spiel noch?\n%s",
	"dlg.font":      "Die scharfe Schrift konnte nicht erzeugt werden, das Spiel nutzt seine eigene:\n%s",

	"sb.brand": "clusterball.de",
}

var textsEN = map[string]string{
	"tab.game":     "Play",
	"tab.settings": "Settings",
	"tab.controls": "Controls",
	"tab.help":     "Help",
	"tab.about":    "About",

	"snd.title":         "Sound",
	"snd.music":         "Music",
	"snd.musicTip":      "ClusterForce: a matching 30-second excerpt of ClusterForce's music for every venue, plus intro and outro. Clusterball plays music only in the menus and while loading.",
	"snd.musicOriginal": "Original",
	"snd.musicCF":       "ClusterForce",
	"snd.ball":          "Ball sound",
	"snd.ballTip":       "The sound when you pick up a ball: Clusterball's own or one of the three variants from ClusterForce.",
	"snd.ballOriginal":  "Original",
	"snd.ballStandard":  "CF: Standard",
	"snd.ballComputer":  "CF: Board computer",
	"snd.ballAnnouncer": "CF: Announcer",
	"snd.preview":       "Listen",

	"ctl.accelerate":        "Accelerate",
	"ctl.brake":             "Brake",
	"ctl.turn_left":         "Turn left",
	"ctl.turn_right":        "Turn right",
	"ctl.pitch_up":          "Pitch up",
	"ctl.pitch_down":        "Pitch down",
	"ctl.roll":              "Roll",
	"ctl.superboost":        "Superboost",
	"ctl.fire":              "Fire",
	"ctl.next_weapon":       "Next weapon",
	"ctl.prev_weapon":       "Previous weapon",
	"ctl.change_target":     "Change target",
	"ctl.release_balls":     "Decoy ball",
	"ctl.talk":              "Chat",
	"ctl.toggle_scoreboard": "Scoreboard",
	"ctl.change_view":       "Change view",
	"ctl.cardFly":           "Flying",
	"ctl.cardMore":          "Combat and more",
	"ctl.presetWASD":        "WASD + mouse",
	"ctl.presetClassic":     "Classic",
	"ctl.hint":              "Click a key, then press the new key or mouse button, or turn the mouse wheel (Esc cancels).\nThe mouse always steers as well. Weapons 1–9 and quick chat F1–F10 stay as they are.",
	"ctl.press":             "Press a key …",
	"ctl.wheelUp":           "Wheel up",
	"ctl.wheelDown":         "Wheel down",
	"ctl.lmb":               "Left mouse button",
	"ctl.rmb":               "Right mouse button",
	"ctl.mmb":               "Middle mouse button",
	"ctl.mb4":               "Mouse button 4",
	"ctl.saved":             "Controls saved - they apply from the next game start",
	"ctl.canceled":          "Binding canceled",
	"ctl.unsupported":       "The game does not know this key",

	"acc.player":      "Player:",
	"acc.playerCue":   "clusterball.de account",
	"acc.password":    "Password:",
	"acc.passwordCue": "Password",
	"acc.register":    "Create account",
	"acc.registerTip": "Create a free clusterball.de account – without one, only LAN games are possible",
	"acc.lan":         "●  LAN games only",
	"acc.online":      "●  Online account",

	"act.start":     "START GAME",
	"act.dedicated": "Host a server (dedicated)",
	"act.ownIP":     "Own IP",
	"act.detectTip": "Detect external IP again",
	"act.external":  "Use external IP",

	"srv.title":        "ONLINE SERVERS",
	"srv.titleN":       "ONLINE SERVERS  (%d)",
	"srv.refresh":      "↺ Refresh",
	"srv.colName":      "Server name",
	"srv.colPlayers":   "Players",
	"srv.colMap":       "Map",
	"srv.colType":      "Type",
	"srv.hint":         "Double-click to join  ·  auto-refresh every 10s",
	"srv.emptyTitle":   "No server is online right now",
	"sum.title":        "Graphics",
	"sum.text":         "%s · %s\nAnti-aliasing %s\nTextures: %s",
	"sum.change":       "Change settings",
	"srv.loadingTitle": "Loading the server list …",
	"srv.emptyHint":    "Start a LAN game or host a server for everyone.\nThe list refreshes every 10 seconds.",
	"srv.dedicated":    "Dedicated",

	"set.graphics": "Graphics (Direct3D 11)",
	"set.network":  "Network",
	"set.language": "Language",
	"set.save":     "Save settings",
	"set.defaults": "Restore defaults",
	"set.preset":   "Preset:",
	"set.pLow":     "Low",
	"set.pMid":     "Medium",
	"set.pHigh":    "High",
	"set.pMax":     "Maximum",
	"set.saveTip":  "Saves to %s",

	"set.display":       "Display",
	"set.fullscreen":    "Fullscreen",
	"set.window":        "Window",
	"set.resolution":    "Resolution",
	"set.resolutionTip": "The resolution the game is drawn at. “Native” uses the monitor's and is the sharpest; a smaller one is stretched to the monitor in fullscreen.",
	"set.resNative":     "Native (monitor)",
	"set.msaa":          "Anti-aliasing",
	"set.ss":            "Supersampling",
	"set.ssTip":         "Draws at twice the resolution and scales down – very smooth edges, but needs much more graphics power.",
	"set.aniso":         "Anisotropic filtering",
	"set.off":           "off",
	"set.texPack":       "Textures",
	"set.texPackTip":    "The high-resolution texture pack lives in the game folder under textures\\hd.",
	"set.texOrig":       "Original",
	"set.texHD4":        "HD (4x)",
	"set.sharpen":       "Sharpness",
	"set.vsync":         "Vertical synchronisation",
	"set.font":          "Sharp font",
	"set.fontTip":       "Replaces the game's tiny 8 × 8 pixel font with a high-resolution one made from Windows' Bahnschrift.",
	"set.ao":            "Ambient occlusion",
	"set.aoTip":         "Shades corners and creases. On the coarse terrain it shows the facets, hence off by default.",
	"set.d3dHint":       "Alt+Enter switches between fullscreen and window in the game.",

	"set.image":      "Picture",
	"set.brightness": "Brightness",
	"set.mouse":      "Mouse sensitivity",
	"set.mouseTip":   "Mouse sensitivity in the game; the Windows pointer speed is applied on top.",
	"set.gfxHint":    "Applies on the next game start.",

	"st.ready":       "Ready",
	"st.starting":    "Starting Clusterball …",
	"st.dediStart":   "Starting dedicated host …",
	"st.dediRunning": "Dedicated host running – heartbeat active",
	"st.loading":     "Loading server list …",
	"st.listError":   "Server list unreachable: %s",
	"st.noServers":   "No servers online",
	"st.nServers1":   "1 server online",
	"st.nServers":    "%d servers online",
	"st.saved":       "Settings saved",
	"st.preset":      "Preset “%s” chosen – applies once saved or when the game starts",
	"st.defaults":    "Defaults restored – apply once saved or when the game starts",
	"st.ipDetect":    "Detecting external IP …",
	"st.ipFailed":    "IP detection failed: %s",
	"st.ipOK":        "External IP: %s",
	"st.joining":     "Starting for “%s” …",
	"st.cfgFailed":   "Could not save configuration: %s",

	"dlg.error":     "Error",
	"dlg.noPath":    "Could not determine the game folder.",
	"dlg.notFound":  "Clusterball.exe not found:\n%s\n\nThe launcher belongs in the game folder, next to Clusterball.exe.",
	"dlg.heartbeat": "Heartbeat error",
	"dlg.gameCfg":   "Could not write the game's config.cfg:\n%s",
	"dlg.d3dCfg":    "Could not write clusterball_d3d.ini:\n%s",
	"dlg.layer":     "Could not place the Direct3D layer (opengl32.dll) in the game folder - is the game still running?\n%s",
	"dlg.font":      "Could not create the sharp font, the game uses its own:\n%s",

	"sb.brand": "clusterball.de",
}
